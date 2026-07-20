package ws

import (
	"net/http/httptest"
	"testing"
)

func TestSameOrigin(t *testing.T) {
	for _, tt := range []struct {
		name   string
		origin string
		want   bool
	}{
		{name: "native client", want: true},
		{name: "same origin", origin: "http://buildworld.example:7777", want: true},
		{name: "same origin https", origin: "https://buildworld.example:7777", want: true},
		{name: "foreign origin", origin: "https://attacker.example", want: false},
		{name: "invalid origin", origin: "://", want: false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "http://buildworld.example:7777/ws", nil)
			if tt.origin != "" {
				req.Header.Set("Origin", tt.origin)
			}
			if got := sameOrigin(req); got != tt.want {
				t.Fatalf("sameOrigin() = %v, want %v", got, tt.want)
			}
		})
	}
}
