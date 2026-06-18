package ws

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"
	
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins for development
	},
}

type Client struct {
	hub    *Hub
	conn   *websocket.Conn
	send   chan []byte
	room   string
}

type Hub struct {
	clients    map[*Client]bool
	broadcast  chan []byte
	register   chan *Client
	unregister chan *Client
	rooms      map[string]map[*Client]bool
	mu         sync.RWMutex
}

type Message struct {
	Type    string      `json:"type"`
	Room    string      `json:"room,omitempty"`
	Payload interface{} `json:"payload"`
}

func NewHub() *Hub {
	return &Hub{
		clients:    make(map[*Client]bool),
		broadcast:  make(chan []byte, 256),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		rooms:      make(map[string]map[*Client]bool),
	}
}

func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			if client.room != "" {
				if h.rooms[client.room] == nil {
					h.rooms[client.room] = make(map[*Client]bool)
				}
				h.rooms[client.room][client] = true
			}
			h.mu.Unlock()
			log.Printf("Client connected to room: %s", client.room)
			
		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
				if client.room != "" {
					delete(h.rooms[client.room], client)
				}
			}
			h.mu.Unlock()
			log.Printf("Client disconnected from room: %s", client.room)
			
		case message := <-h.broadcast:
			h.mu.RLock()
			for client := range h.clients {
				select {
				case client.send <- message:
				default:
					close(client.send)
					delete(h.clients, client)
				}
			}
			h.mu.RUnlock()
		}
	}
}

func (h *Hub) BroadcastToRoom(room string, message []byte) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	
	if clients, ok := h.rooms[room]; ok {
		for client := range clients {
			select {
			case client.send <- message:
			default:
				close(client.send)
				delete(h.clients, client)
				delete(clients, client)
			}
		}
	}
}

func HandleWebSocket(hub *Hub, w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}
	
	room := r.URL.Query().Get("room")
	if room == "" {
		room = "default"
	}
	
	client := &Client{
		hub:  hub,
		conn: conn,
		send: make(chan []byte, 256),
		room: room,
	}
	
	hub.register <- client
	
	go client.writePump()
	go client.readPump()
}

func (c *Client) readPump() {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()
	
	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			break
		}
		
		var msg Message
		if err := json.Unmarshal(message, &msg); err == nil {
			if msg.Type == "subscribe" && msg.Room != "" {
				c.hub.mu.Lock()
				if c.room != "" {
					delete(c.hub.rooms[c.room], c)
				}
				c.room = msg.Room
				if c.hub.rooms[c.room] == nil {
					c.hub.rooms[c.room] = make(map[*Client]bool)
				}
				c.hub.rooms[c.room][c] = true
				c.hub.mu.Unlock()
			}
		}
	}
}

func (c *Client) writePump() {
	ticker := time.NewTicker(30 * time.Second)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()
	
	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			
			w, err := c.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			w.Write(message)
			
			if err := w.Close(); err != nil {
				return
			}
			
		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// BroadcastBuildLog sends build log to specific build room
func BroadcastBuildLog(hub *Hub, buildID string, logLine map[string]interface{}) {
	msg := Message{
		Type:    "build:log",
		Room:    "build:" + buildID,
		Payload: logLine,
	}
	data, _ := json.Marshal(msg)
	hub.BroadcastToRoom("build:"+buildID, data)
}

// BroadcastBuildStatus sends build status update
func BroadcastBuildStatus(hub *Hub, buildID string, status map[string]interface{}) {
	msg := Message{
		Type:    "build:status",
		Room:    "build:" + buildID,
		Payload: status,
	}
	data, _ := json.Marshal(msg)
	hub.BroadcastToRoom("build:"+buildID, data)
}
