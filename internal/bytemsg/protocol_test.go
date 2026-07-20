package bytemsg

import (
	"context"
	"strings"
	"testing"

	pb "github.com/neko233-com/buildworld/internal/rpc/generated"
	"google.golang.org/grpc/metadata"
)

func TestValidateProtocolAcceptsCurrentAndLegacyV3(t *testing.T) {
	if err := Validate(NewProtocolInfo(), LegacyVersion); err != nil {
		t.Fatal(err)
	}
	if err := Validate(nil, LegacyVersion); err != nil {
		t.Fatal(err)
	}
}

func TestValidateProtocolRejectsDifferentMajor(t *testing.T) {
	err := Validate(&pb.ProtocolInfo{Name: Name, Major: Major - 1}, "")
	if err == nil || !strings.Contains(err.Error(), "unsupported worker protocol") {
		t.Fatalf("error = %v", err)
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
	if err := ValidateDispatchCredential(metadata.NewIncomingContext(context.Background(), metadataOut), "shared-secret"); err != nil {
		t.Fatal(err)
	}
	if err := ValidateDispatchCredential(metadata.NewIncomingContext(context.Background(), metadataOut), "other"); err == nil {
		t.Fatal("expected invalid credential")
	}
}
