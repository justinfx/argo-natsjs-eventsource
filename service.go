package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/araddon/dateparse"
	"github.com/argoproj/argo-events/common"
	apisCommon "github.com/argoproj/argo-events/pkg/apis/common"
	"github.com/argoproj/argo-events/pkg/apis/eventsource/v1alpha1"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"gopkg.in/yaml.v3"

	"github.com/justinfx/args-natsjs-eventsource/proto"
)

// NATSJSEventSourceCfg defines the configuration for a single EventSource start request
type NATSJSEventSourceCfg struct {
	URL               string                `yaml:"url"`
	Stream            string                `yaml:"stream"`
	FilterSubjects    []string              `yaml:"subjects,omitempty"`
	Durable           string                `yaml:"durable,omitempty"`
	JSONBody          bool                  `yaml:"jsonBody,omitempty"`
	ConnectionBackoff *apisCommon.Backoff   `yaml:"connection_backoff,omitempty"`
	DeliverStartSeq   uint64                `yaml:"deliver_start_seq,omitempty"`
	DeliverStartTime  string                `yaml:"deliver_start_time,omitempty"`
	TLS               *apisCommon.TLSConfig `yaml:"tls,omitempty"`
	Metadata          map[string]string     `yaml:"metadata,omitempty"`
	Auth              *v1alpha1.NATSAuth    `yaml:"auth,omitempty"`
}

// NATSJSEventData is the data returns for each Nats event
type NATSJSEventData struct {
	// Name of the subject.
	Subject string `json:"subject"`
	// Message data.
	Body interface{} `json:"body"`
	// Header represents the optional Header for a NATS message, based on the implementation of http.Header.
	Header map[string][]string `json:"header,omitempty"`
	// Metadata holds the user defined metadata which will passed along the event payload.
	Metadata map[string]string `json:"metadata,omitempty"`
	// The name of the stream
	Stream string `json:"stream"`
	// The name of the stream consumer
	Consumer string `json:"consumer"`
	// The position of the message in the stream
	SeqId uint64 `json:"seq_id"`
	// The timestamp of the message in the stream
	Timestamp time.Time `json:"timestamp"`
}

// NATSJSEventSource implements the generic EventSource proto service interface, where
// each request to StartEventSource will create a new Nats connection, pull from a Consumer,
// and push messages to the client
type NATSJSEventSource struct {
	cfg ServiceConfig
	proto.UnimplementedEventingServer
}

func NewNatsJSEventSource(cfg ServiceConfig) *NATSJSEventSource {
	if cfg.logger == nil {
		cfg.logger = slog.Default()
	}
	return &NATSJSEventSource{cfg: cfg}
}

func (n *NATSJSEventSource) StartEventSource(src *proto.EventSource, svr proto.Eventing_StartEventSourceServer) error {
	logger := n.cfg.logger.WithGroup("EventSource").With(slog.String("name", src.Name))

	logErr := func(err error) error {
		logger.Error(err.Error())
		return err
	}

	var esCfg NATSJSEventSourceCfg
	if err := yaml.Unmarshal(src.Config, &esCfg); err != nil {
		return logErr(fmt.Errorf("failed to parse yaml config: %w", err))
	}
	logger.Debug("Received StartEventSource request",
		// TODO: need to redact auth secrets, before logging
		//"config", esCfg,
		slog.String("stream", esCfg.Stream),
	)

	var opt []nats.Option
	if err := n.addTLSOptions(&esCfg, &opt); err != nil {
		return logErr(fmt.Errorf("failed to set the tls options, %w", err))
	}
	if err := n.addAuthOptions(&esCfg, &opt); err != nil {
		return logErr(fmt.Errorf("failed to set the auth options, %w", err))
	}

	// Start with a no-op reconnect handler, which we will update at the end
	reconnectFn := func() {}
	opt = append(opt, nats.ReconnectHandler(func(_ *nats.Conn) { reconnectFn() }))

	var startTime time.Time
	if esCfg.DeliverStartTime != "" {
		dt, err := dateparse.ParseLocal(esCfg.DeliverStartTime)
		if err != nil {
			return logErr(fmt.Errorf("bad config DeliverStartTime format: %v", err))
		}
		startTime = dt
	}

	if esCfg.JSONBody {
		logger.Info("Assuming all events have a json body")
	}

	var conn *nats.Conn
	logger.Info("Connecting to nats cluster")
	if err := common.DoWithRetry(esCfg.ConnectionBackoff, func() error {
		var err error
		if conn, err = nats.Connect(esCfg.URL, opt...); err != nil {
			return err
		}
		return nil
	}); err != nil {
		return logErr(fmt.Errorf("failed to connect to nats server for event source %s, %w", src.Name, err))
	}
	defer conn.Close()

	js, err := jetstream.New(conn)
	if err != nil {
		return logErr(fmt.Errorf("failed to create nats jetstream conn for event source %s, %w", src.Name, err))
	}

	ctx, consumerDone := context.WithCancel(svr.Context())
	defer consumerDone()

	// check if the configured stream exists
	_, err = js.Stream(ctx, esCfg.Stream)
	if err != nil {
		if errors.Is(err, jetstream.ErrStreamNotFound) {
			return logErr(fmt.Errorf(
				"failed to create nats jetstream consumer for event source %s, "+
					"configured stream \"%s\" does not exist on server \"%s\", %w",
				src.Name, esCfg.Stream, esCfg.URL, err))
		}
	}

	var (
		lastId         uint64
		consumer       jetstream.Consumer
		consumerErr    error
		createConsumer func(seqId uint64) (jetstream.Consumer, error)
	)

	if esCfg.Durable == "" {
		// create an ephemeral pull consumer
		createConsumer = func(seqId uint64) (jetstream.Consumer, error) {
			// handle possible reconnect
			if consumer != nil {
				if seqId == 0 {
					seqId = atomic.LoadUint64(&lastId)
					if seqId > 0 {
						seqId++
					}
				}
				info := consumer.CachedInfo()
				// The consumer seems to be cleaned up automatically, but just
				// to be sure, attempt to delete it manually
				_ = js.DeleteConsumer(ctx, info.Stream, info.Name)
			}

			cfg := jetstream.OrderedConsumerConfig{
				FilterSubjects:    esCfg.FilterSubjects,
				DeliverPolicy:     jetstream.DeliverNewPolicy,
				InactiveThreshold: 1 * time.Hour,
			}
			if seqId > 0 {
				cfg.OptStartSeq = seqId
				cfg.DeliverPolicy = jetstream.DeliverByStartSequencePolicy
				logger.Info("Using start seq id", "id", seqId)
			} else if !startTime.IsZero() {
				cfg.OptStartTime = &startTime
				cfg.DeliverPolicy = jetstream.DeliverByStartTimePolicy
				logger.Info("Using start date", "data", startTime)
			}
			logger.Debug("creating an ephemeral ordered consumer",
				"stream", esCfg.Stream, "subjects", esCfg.FilterSubjects)
			consumer, err = js.OrderedConsumer(ctx, esCfg.Stream, cfg)
			if err != nil {
				return nil, fmt.Errorf("error setting up ephemeral ordered consumer: %v", err)
			}
			return consumer, nil
		}
	} else {
		// create a durable named pull consumer
		createConsumer = func(seqId uint64) (jetstream.Consumer, error) {
			cfg := jetstream.ConsumerConfig{
				Name:              esCfg.Durable,
				Durable:           esCfg.Durable,
				Description:       fmt.Sprintf("argo-events generic NatsJSEventSouce %q", src.Name),
				DeliverPolicy:     jetstream.DeliverNewPolicy,
				AckPolicy:         jetstream.AckExplicitPolicy,
				MaxDeliver:        30,
				InactiveThreshold: 24 * time.Hour,
			}
			// Handle older nats server not supporting multiple filter topics
			if len(esCfg.FilterSubjects) == 1 {
				cfg.FilterSubject = esCfg.FilterSubjects[0]
			} else {
				cfg.FilterSubjects = esCfg.FilterSubjects
			}
			if seqId > 0 {
				cfg.OptStartSeq = seqId
				cfg.DeliverPolicy = jetstream.DeliverByStartSequencePolicy
				logger.Info("Using start seq id", "id", seqId)
			} else if !startTime.IsZero() {
				cfg.OptStartTime = &startTime
				cfg.DeliverPolicy = jetstream.DeliverByStartTimePolicy
				logger.Info("Using start date", "data", startTime)
			}

			// check if consumer exists
			if consumer, err = js.Consumer(ctx, esCfg.Stream, cfg.Name); err != nil {
				if !errors.Is(err, jetstream.ErrConsumerNotFound) {
					return nil, fmt.Errorf("error looking up existing Nats Jetstream consumer: %v", err)
				}
				logger.Debug("creating a durable consumer",
					"durable", cfg.Durable, "stream", esCfg.Stream, "subjects", esCfg.FilterSubjects)

			} else {
				cInfo, err := consumer.Info(ctx)
				if err != nil {
					return nil, fmt.Errorf("error looking up existing Nats Jetstream consumer info: %v", err)
				}
				// only certain fields can be updated on existing consumer
				cInfo.Config.FilterSubject = cfg.FilterSubject
				cInfo.Config.FilterSubjects = cfg.FilterSubjects
				cfg = cInfo.Config
				logger.Debug("using existing durable consumer",
					"durable", cfg.Durable, "stream", esCfg.Stream, "subjects", esCfg.FilterSubjects)
			}

			consumer, err = js.CreateOrUpdateConsumer(ctx, esCfg.Stream, cfg)
			if err != nil {
				return nil, fmt.Errorf("error setting up Nats Jetstream durable consumer: %v", err)
			}
			return consumer, nil
		}
	}

	if consumer, err = createConsumer(esCfg.DeliverStartSeq); err != nil {
		return err
	}

	event := proto.Event{Name: src.Name}

	// handler function will be used for our initial consume,
	// and for any reconnection attempts
	handler := func(m jetstream.Msg) {
		if ctx.Err() != nil {
			consumerErr = ctx.Err()
			consumerDone()
			return
		}

		jsMeta, err := m.Metadata()
		if err != nil {
			consumerErr = fmt.Errorf("error reading nats msg metadata: %w", err)
			consumerDone() // TODO: fatal to client? or just log and skip?
			return
		}
		eventData := NATSJSEventData{
			Subject:   m.Subject(),
			Header:    m.Headers(),
			Metadata:  esCfg.Metadata,
			Stream:    jsMeta.Stream,
			Consumer:  jsMeta.Consumer,
			SeqId:     jsMeta.Sequence.Stream,
			Timestamp: jsMeta.Timestamp,
		}

		if esCfg.JSONBody {
			data := m.Data()
			eventData.Body = (*json.RawMessage)(&data)
		} else {
			eventData.Body = m.Data()
		}

		event.Payload, err = json.Marshal(eventData)
		logger.Debug("sending message", "payload", string(event.Payload), "body", eventData.Body)
		if err != nil {
			consumerErr = logErr(fmt.Errorf("failed to marshal event data (rejecting event): %w", err))
			consumerDone() // TODO: fatal to client? or just log and skip?
			return
		}

		err = svr.Send(&event)
		if err != nil {
			consumerErr = logErr(fmt.Errorf("failed to send NATS Jetstream event to client: %w", err))
			consumerDone()
			return
		}
		if meta, err := m.Metadata(); err == nil {
			atomic.StoreUint64(&lastId, meta.Sequence.Stream)
		}
		if err := m.Ack(); err != nil {
			consumerErr = logErr(fmt.Errorf("failed acknowledging last Nats Jetstream message: %w", err))
			consumerDone()
			return
		}
	}

	consumerCtx, err := consumer.Consume(handler)
	if err != nil {
		return logErr(fmt.Errorf("error consuming messages: %w", err))
	}
	defer func() {
		// stop the later consumer, even if it is replaced via reconnection callback
		if consumerCtx != nil {
			consumerCtx.Stop()
		}
	}()

	// The implementation of the Nats OrderedConsumer doesn't properly handle
	// recreating the consumer and position when the server disappears.
	// We have to detect reconnections and manually restart the consumer
	// from our own tracked message id position.
	reconnectFn = func() {
		logger.Info("Nats reconnected: resuming consumer")
		consumer, err := createConsumer(0)
		if err != nil {
			consumerErr = logErr(fmt.Errorf("error recreating consumer after reconnection: %w", err))
			return
		}
		consumerCtx.Stop()
		consumerCtx, err = consumer.Consume(handler)
		if err != nil {
			consumerErr = logErr(fmt.Errorf("error consuming messages: %w", err))
		}
	}

	<-ctx.Done()
	return consumerErr
}

func (n *NATSJSEventSource) addTLSOptions(cfg *NATSJSEventSourceCfg, opt *[]nats.Option) error {
	if cfg.TLS == nil {
		return nil
	}
	if cfg.TLS.InsecureSkipVerify ||
		(cfg.TLS.ClientCertSecret == nil && cfg.TLS.ClientKeySecret == nil && cfg.TLS.CACertSecret == nil) {
		return nil
	}
	tlsConfig, err := common.GetTLSConfig(cfg.TLS)
	if err != nil {
		return err
	}
	*opt = append(*opt, nats.Secure(tlsConfig))
	return nil
}

func (n *NATSJSEventSource) addAuthOptions(cfg *NATSJSEventSourceCfg, opt *[]nats.Option) error {
	if cfg.Auth == nil {
		return nil
	}
	switch {
	case cfg.Auth.Basic != nil:
		username, err := common.GetSecretFromVolume(cfg.Auth.Basic.Username)
		if err != nil {
			return err
		}
		password, err := common.GetSecretFromVolume(cfg.Auth.Basic.Password)
		if err != nil {
			return err
		}
		*opt = append(*opt, nats.UserInfo(username, password))
	case cfg.Auth.Token != nil:
		token, err := common.GetSecretFromVolume(cfg.Auth.Token)
		if err != nil {
			return err
		}
		*opt = append(*opt, nats.Token(token))
	case cfg.Auth.NKey != nil:
		nkeyFile, err := common.GetSecretVolumePath(cfg.Auth.NKey)
		if err != nil {
			return err
		}
		o, err := nats.NkeyOptionFromSeed(nkeyFile)
		if err != nil {
			return fmt.Errorf("failed to get NKey, %w", err)
		}
		*opt = append(*opt, o)
	case cfg.Auth.Credential != nil:
		cFile, err := common.GetSecretVolumePath(cfg.Auth.Credential)
		if err != nil {
			return err
		}
		*opt = append(*opt, nats.UserCredentials(cFile))
	}
	return nil
}
