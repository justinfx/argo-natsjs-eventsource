//go:generate protoc -Iproto --go_out=paths=source_relative:./proto --go-grpc_out=paths=source_relative:./proto ./proto/generic.proto

package main

import (
	"fmt"
	"log"
	"log/slog"
	"net"
	"os"

	flag "github.com/spf13/pflag"
	"google.golang.org/grpc"

	"github.com/justinfx/args-natsjs-eventsource/proto"
)

var Version = "dev"

type ServiceConfig struct {
	Port   int
	logger *slog.Logger
}

var logLevel = &slog.LevelVar{}

func main() {
	logLevel.Set(slog.LevelDebug)
	cfg := ServiceConfig{
		logger: slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: logLevel})),
	}
	handleFlags(&cfg)
	serve(cfg)
}

func handleFlags(cfg *ServiceConfig) {
	help := flag.BoolP("help", "h", false, "")
	flag.IntVarP(&cfg.Port, "port", "p", 8080, "The service listener port")
	flag.Parse()
	if *help {
		flag.Usage()
		os.Exit(0)
	}
}

func serve(cfg ServiceConfig) {
	host := fmt.Sprintf("0.0.0.0:%d", cfg.Port)
	lis, err := net.Listen("tcp", host)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	var opts []grpc.ServerOption
	grpcServer := grpc.NewServer(opts...)
	proto.RegisterEventingServer(grpcServer, NewNatsJSEventSource(cfg))

	cfg.logger.Info("listening", "host", host)
	if err := grpcServer.Serve(lis); err != nil {
		cfg.logger.Error(fmt.Sprintf("failed to serve: %v", err))
		os.Exit(1)
	}
}
