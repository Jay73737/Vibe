package server

import (
	"encoding/json"
	"log"
	"sync"

	"github.com/gorilla/websocket"
)

// Event represents a push notification to connected clients.
type Event struct {
	Type    string      `json:"type"`    // "commit", "branch", "sync"
	Branch  string      `json:"branch,omitempty"`
	Hash    string      `json:"hash,omitempty"`
	Message string      `json:"message,omitempty"`
	Data    interface{} `json:"data,omitempty"`
}

// Hub manages WebSocket connections and broadcasts events.
type Hub struct {
	mu      sync.RWMutex
	clients map[*Client]bool
}

type Client struct {
	conn *websocket.Conn
	send chan []byte
	hub  *Hub
}

func NewHub() *Hub {
	return &Hub{
		clients: make(map[*Client]bool),
	}
}

// Register adds a new client connection.
func (h *Hub) Register(conn *websocket.Conn) *Client {
	client := &Client{
		conn: conn,
		send: make(chan []byte, 64),
		hub:  h,
	}
	h.mu.Lock()
	h.clients[client] = true
	h.mu.Unlock()

	// Start write pump
	go client.writePump()

	return client
}

// Unregister removes a client.
func (h *Hub) Unregister(c *Client) {
	h.mu.Lock()
	if _, ok := h.clients[c]; ok {
		delete(h.clients, c)
		close(c.send)
	}
	h.mu.Unlock()
}

// Broadcast sends an event to all connected clients.
func (h *Hub) Broadcast(event *Event) {
	data, err := json.Marshal(event)
	if err != nil {
		log.Printf("hub: marshal event: %v", err)
		return
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	for client := range h.clients {
		select {
		case client.send <- data:
		default:
			// Client too slow, disconnect
			go h.Unregister(client)
		}
	}
}

// ClientCount returns the number of connected clients.
func (h *Hub) ClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

func (c *Client) writePump() {
	defer c.conn.Close()
	for msg := range c.send {
		if err := c.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
			return
		}
	}
}

// ReadPump reads messages from the client (required to detect disconnects).
func (c *Client) ReadPump() {
	defer c.hub.Unregister(c)
	for {
		_, _, err := c.conn.ReadMessage()
		if err != nil {
			return
		}
	}
}
