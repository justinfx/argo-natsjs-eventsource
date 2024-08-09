# Argo NATS Jetstream EventSource

[![Docker Image Version](https://img.shields.io/docker/v/justinisrael/argo-natsjs-eventsource?label=Docker)](https://hub.docker.com/r/justinisrael/argo-natsjs-eventsource/tags)

A "generic" Argo EventSource implementation supporting NATS Jetstream

See: https://argoproj.github.io/argo-events/eventsources/generic/

## Purpose

At the time this was implemented, Argo Events does not provide an EventSource implementation that supports 
Nats Jetstream. It only supports Nats core messaging. This generic EventSource will be maintained until hopefully
the implementation can be adopted as a 1st-class EventSource.

https://github.com/argoproj/argo-events/issues/3160

## Implementation

An instance of this service is referenced by the generic EventSource definition url. A config is then also specified 
to be passed to the service when the EventSource proxy is created and requests to begin streaming messages. For each
EventSource that is defined, a new Nats connection is started, and a Consumer is created/updated based on the config. 
When the stream ends, the Nats connection is closed.

## Development

In order to re-generate the EventSource generic.proto (if it changes), your environment will need:

* `protoc` (Protocol Buffers compiler)
* `protoc-gen-go` plugin
* `protoc-gen-go-grpc` plugin

Re-generate and commit the protobuf/grpc code:

```bash
go generate
```
