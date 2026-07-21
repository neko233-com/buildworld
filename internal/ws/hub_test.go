package ws

import (
	"net/http/httptest"
	"testing"
	"time"
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

func TestBroadcastToRoomDropsFullClientWithoutBlocking(t *testing.T) {
	hub := NewHub()
	client := &Client{hub: hub, send: make(chan []byte, 1), room: "build:1"}
	hub.clients[client] = true
	hub.rooms[client.room] = map[*Client]bool{client: true}
	client.send <- []byte("already queued")

	done := make(chan struct{})
	go func() {
		hub.BroadcastToRoom(client.room, []byte("new output"))
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("BroadcastToRoom blocked on a slow client")
	}
	hub.mu.RLock()
	_, inClients := hub.clients[client]
	_, inRoom := hub.rooms[client.room][client]
	hub.mu.RUnlock()
	if inClients || inRoom {
		t.Fatal("full client was not removed")
	}
	if _, open := <-client.send; !open {
		// The buffered message is returned before the closed state.
		t.Fatal("buffered message unexpectedly missing")
	}
	if _, open := <-client.send; open {
		t.Fatal("full client send channel was not closed")
	}
}
