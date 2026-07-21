package bytemsg

import (
	"context"
	"slices"
	"strings"
	"testing"

	pb "github.com/neko233-com/buildworld/internal/rpc/generated"
	"google.golang.org/grpc/metadata"
)

func TestNewProtocolInfoDeclaresCurrentExecutionCapabilities(t *testing.T) {
	info := NewProtocolInfo()
	if info.Name != Name || info.Major != Major || info.Minor != Minor {
		t.Fatalf("protocol = %#v", info)
	}
	want := []string{"typescript-pipeline", "jobs-yaml", "platform-additions", "node-outputs", "artifact-stream-v1", "go-binary-plugins-v1"}
	if !slices.Equal(info.Capabilities, want) {
		t.Fatalf("capabilities = %v, want %v", info.Capabilities, want)
	}
	info.Capabilities[0] = "mutated"
	if NewProtocolInfo().Capabilities[0] != want[0] {
		t.Fatal("NewProtocolInfo shared a mutable capability slice")
	}
}

func TestValidateProtocolAcceptsCurrentStructuredInfo(t *testing.T) {
	if err := Validate(NewProtocolInfo()); err != nil {
		t.Fatal(err)
	}
	info := NewProtocolInfo()
	info.Minor++
	info.Capabilities = append(info.Capabilities, "future-optional-capability")
	if err := Validate(info); err != nil {
		t.Fatalf("compatible newer minor = %v", err)
	}
}

func TestValidateProtocolRejectsMissingOrIncompatibleInfo(t *testing.T) {
	tests := []struct {
		name string
		info *pb.ProtocolInfo
		want string
	}{
		{name: "missing", want: "missing structured protocol info"},
		{name: "different name", info: &pb.ProtocolInfo{Name: "other", Major: Major}, want: "unsupported worker protocol"},
		{name: "different major", info: &pb.ProtocolInfo{Name: Name, Major: Major + 1}, want: "unsupported worker protocol"},
		{name: "missing capability", info: func() *pb.ProtocolInfo {
			info := NewProtocolInfo()
			info.Capabilities = info.Capabilities[1:]
			return info
		}(), want: `missing capability "typescript-pipeline"`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := Validate(test.info)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want substring %q", err, test.want)
			}
		})
	}
}

func TestDispatchCredentialRoundTrip(t *testing.T) {
	ctx := WithDispatchCredential(context.Background(), "shared-secret")
	// gRPC exposes outgoing metadata to the receiving server as incoming
	// metadata; construct that context here to unit-test the guard directly.
	metadataOut, ok := metadata.FromOutgoingContext(ctx)
	if !ok {
		t.Fatal("missing outgoing metadata")
	}
	if marker := metadataOut.Get("x-buildworld-bytemsg"); len(marker) != 0 {
		t.Fatalf("legacy protocol marker remains: %v", marker)
	}
	if err := ValidateDispatchCredential(metadata.NewIncomingContext(context.Background(), metadataOut), "shared-secret"); err != nil {
		t.Fatal(err)
	}
	if err := ValidateDispatchCredential(metadata.NewIncomingContext(context.Background(), metadataOut), "other"); err == nil {
		t.Fatal("expected invalid credential")
	}
	legacyOnly := metadata.NewIncomingContext(context.Background(), metadata.Pairs("x-buildworld-bytemsg", "bytemsg233/v3"))
	if err := ValidateDispatchCredential(legacyOnly, "shared-secret"); err == nil {
		t.Fatal("legacy protocol marker authenticated dispatch without a credential")
	}
}
