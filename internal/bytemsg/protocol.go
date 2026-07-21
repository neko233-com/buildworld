// Package bytemsg owns the versioned binary contract used between the
// Buildworld control plane and remote workers.
package bytemsg

import (
	"context"
	"fmt"

	pb "github.com/neko233-com/buildworld/internal/rpc/generated"
	"google.golang.org/grpc/metadata"
)

const (
	Name         = "bytemsg233"
	Major uint32 = 1
	Minor uint32 = 0

	dispatchMetadataKey = "x-buildworld-dispatch-token"
)

var requiredCapabilities = []string{
	"typescript-pipeline",
	"jobs-yaml",
	"platform-additions",
	"node-outputs",
	"artifact-stream-v1",
	"go-binary-plugins-v1",
}

// NewProtocolInfo returns a fresh protobuf message so callers can safely pass
// it to a request or response without sharing mutable slices.
func NewProtocolInfo() *pb.ProtocolInfo {
	return &pb.ProtocolInfo{Name: Name, Major: Major, Minor: Minor, Capabilities: append([]string(nil), requiredCapabilities...)}
}

// Validate requires the current structured protocol and every execution
// capability used by the control plane before a worker executes a pipeline.
func Validate(info *pb.ProtocolInfo) error {
	if info == nil {
		return fmt.Errorf("unsupported worker protocol: missing structured protocol info")
	}
	if info.Name != Name || info.Major != Major {
		return fmt.Errorf("unsupported worker protocol: %s/v%d", info.Name, info.Major)
	}
	provided := make(map[string]struct{}, len(info.Capabilities))
	for _, capability := range info.Capabilities {
		provided[capability] = struct{}{}
	}
	for _, capability := range requiredCapabilities {
		if _, ok := provided[capability]; !ok {
			return fmt.Errorf("unsupported worker protocol: missing capability %q", capability)
		}
	}
	return nil
}

// WithDispatchCredential attaches the shared enrollment credential to an
// outgoing gRPC context. Protocol negotiation lives in the protobuf message.
func WithDispatchCredential(ctx context.Context, credential string) context.Context {
	return metadata.AppendToOutgoingContext(ctx, dispatchMetadataKey, credential)
}

// ValidateDispatchCredential protects a worker's gRPC execution endpoint.
// An empty expected credential deliberately disables remote dispatch rather
// than exposing an unauthenticated command execution port.
func ValidateDispatchCredential(ctx context.Context, expected string) error {
	if expected == "" {
		return fmt.Errorf("remote worker dispatch is disabled: enrollment token is empty")
	}
	metadataIn, ok := metadata.FromIncomingContext(ctx)
	if !ok || first(metadataIn.Get(dispatchMetadataKey)) != expected {
		return fmt.Errorf("invalid worker dispatch credential")
	}
	return nil
}

func first(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[0]
}
