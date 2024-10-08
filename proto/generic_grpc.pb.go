// https://github.com/argoproj/argo-events/blob/master/eventsources/sources/generic/generic.proto

// Code generated by protoc-gen-go-grpc. DO NOT EDIT.
// versions:
// - protoc-gen-go-grpc v1.5.1
// - protoc             v5.27.3
// source: generic.proto

package proto

import (
	context "context"
	grpc "google.golang.org/grpc"
	codes "google.golang.org/grpc/codes"
	status "google.golang.org/grpc/status"
)

// This is a compile-time assertion to ensure that this generated file
// is compatible with the grpc package it is being compiled against.
// Requires gRPC-Go v1.64.0 or later.
const _ = grpc.SupportPackageIsVersion9

const (
	Eventing_StartEventSource_FullMethodName = "/generic.Eventing/StartEventSource"
)

// EventingClient is the client API for Eventing service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://pkg.go.dev/google.golang.org/grpc/?tab=doc#ClientConn.NewStream.
type EventingClient interface {
	StartEventSource(ctx context.Context, in *EventSource, opts ...grpc.CallOption) (grpc.ServerStreamingClient[Event], error)
}

type eventingClient struct {
	cc grpc.ClientConnInterface
}

func NewEventingClient(cc grpc.ClientConnInterface) EventingClient {
	return &eventingClient{cc}
}

func (c *eventingClient) StartEventSource(ctx context.Context, in *EventSource, opts ...grpc.CallOption) (grpc.ServerStreamingClient[Event], error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	stream, err := c.cc.NewStream(ctx, &Eventing_ServiceDesc.Streams[0], Eventing_StartEventSource_FullMethodName, cOpts...)
	if err != nil {
		return nil, err
	}
	x := &grpc.GenericClientStream[EventSource, Event]{ClientStream: stream}
	if err := x.ClientStream.SendMsg(in); err != nil {
		return nil, err
	}
	if err := x.ClientStream.CloseSend(); err != nil {
		return nil, err
	}
	return x, nil
}

// This type alias is provided for backwards compatibility with existing code that references the prior non-generic stream type by name.
type Eventing_StartEventSourceClient = grpc.ServerStreamingClient[Event]

// EventingServer is the server API for Eventing service.
// All implementations must embed UnimplementedEventingServer
// for forward compatibility.
type EventingServer interface {
	StartEventSource(*EventSource, grpc.ServerStreamingServer[Event]) error
	mustEmbedUnimplementedEventingServer()
}

// UnimplementedEventingServer must be embedded to have
// forward compatible implementations.
//
// NOTE: this should be embedded by value instead of pointer to avoid a nil
// pointer dereference when methods are called.
type UnimplementedEventingServer struct{}

func (UnimplementedEventingServer) StartEventSource(*EventSource, grpc.ServerStreamingServer[Event]) error {
	return status.Errorf(codes.Unimplemented, "method StartEventSource not implemented")
}
func (UnimplementedEventingServer) mustEmbedUnimplementedEventingServer() {}
func (UnimplementedEventingServer) testEmbeddedByValue()                  {}

// UnsafeEventingServer may be embedded to opt out of forward compatibility for this service.
// Use of this interface is not recommended, as added methods to EventingServer will
// result in compilation errors.
type UnsafeEventingServer interface {
	mustEmbedUnimplementedEventingServer()
}

func RegisterEventingServer(s grpc.ServiceRegistrar, srv EventingServer) {
	// If the following call pancis, it indicates UnimplementedEventingServer was
	// embedded by pointer and is nil.  This will cause panics if an
	// unimplemented method is ever invoked, so we test this at initialization
	// time to prevent it from happening at runtime later due to I/O.
	if t, ok := srv.(interface{ testEmbeddedByValue() }); ok {
		t.testEmbeddedByValue()
	}
	s.RegisterService(&Eventing_ServiceDesc, srv)
}

func _Eventing_StartEventSource_Handler(srv interface{}, stream grpc.ServerStream) error {
	m := new(EventSource)
	if err := stream.RecvMsg(m); err != nil {
		return err
	}
	return srv.(EventingServer).StartEventSource(m, &grpc.GenericServerStream[EventSource, Event]{ServerStream: stream})
}

// This type alias is provided for backwards compatibility with existing code that references the prior non-generic stream type by name.
type Eventing_StartEventSourceServer = grpc.ServerStreamingServer[Event]

// Eventing_ServiceDesc is the grpc.ServiceDesc for Eventing service.
// It's only intended for direct use with grpc.RegisterService,
// and not to be introspected or modified (even as a copy)
var Eventing_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "generic.Eventing",
	HandlerType: (*EventingServer)(nil),
	Methods:     []grpc.MethodDesc{},
	Streams: []grpc.StreamDesc{
		{
			StreamName:    "StartEventSource",
			Handler:       _Eventing_StartEventSource_Handler,
			ServerStreams: true,
		},
	},
	Metadata: "generic.proto",
}
