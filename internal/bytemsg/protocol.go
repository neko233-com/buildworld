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
	Major uint32 = 3
	Minor uint32 = 0

	// LegacyVersion stays on the wire during the compatibility period so a
	// server upgrade can still identify an older v3 worker explicitly.
	LegacyVersion = "bytemsg233/v3"

	protocolMetadataKey = "x-buildworld-bytemsg"
	dispatchMetadataKey = "x-buildworld-dispatch-token"
)

var capabilities = []string{
	"markdown-pipeline",
	"platform-additions",
	"node-outputs",
	"artifact-stream-v1",
	"go-binary-plugins-v1",
}

// NewProtocolInfo returns a fresh protobuf message so callers can safely pass
// it to a request or response without sharing mutable slices.
func NewProtocolInfo() *pb.ProtocolInfo {
	return &pb.ProtocolInfo{Name: Name, Major: Major, Minor: Minor, Capabilities: append([]string(nil), capabilities...)}
}

// Validate rejects an incompatible major before a worker executes a pipeline.
// A missing structured field is accepted only for the explicit v3 legacy
// value, allowing a rolling upgrade without treating arbitrary strings as v3.
func Validate(info *pb.ProtocolInfo, legacy string) error {
	if info == nil {
		if legacy == LegacyVersion {
			return nil
		}
		return fmt.Errorf("unsupported worker protocol: %s", legacy)
	}
	if info.Name != Name || info.Major != Major {
		return fmt.Errorf("unsupported worker protocol: %s/v%d", info.Name, info.Major)
	}
	return nil
}

// WithDispatchCredential attaches both the protocol marker and the shared
// enrollment credential to an outgoing gRPC context.
func WithDispatchCredential(ctx context.Context, credential string) context.Context {
	return metadata.AppendToOutgoingContext(ctx, protocolMetadataKey, LegacyVersion, dispatchMetadataKey, credential)
}

// ValidateDispatchCredential protects a worker's gRPC execution endpoint.
// An empty expected credential deliberately disables remote dispatch rather
// than exposing an unauthenticated command execution port.
func ValidateDispatchCredential(ctx context.Context, expected string) error {
	if expected == "" {
		return fmt.Errorf("remote worker dispatch is disabled: enrollment token is empty")
	}
	metadataIn, ok := metadata.FromIncomingContext(ctx)
	if !ok || first(metadataIn.Get(protocolMetadataKey)) != LegacyVersion {
		return fmt.Errorf("unsupported worker transport protocol")
	}
	if first(metadataIn.Get(dispatchMetadataKey)) != expected {
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
