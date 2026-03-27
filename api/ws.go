package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // Rely on Gin's CORS middleware for origins
	},
}

// WsMessage defines the JSON structure for WebSocket communication
type WsMessage struct {
	Type    string      `json:"type"` // e.g., "stats", "status_update", "error"
	Project string      `json:"project,omitempty"`
	Data    interface{} `json:"data"`
}

// WsClient represents a single connected client
type WsClient struct {
	hub      *WsHub
	conn     *websocket.Conn
	send     chan []byte
	projects map[string]bool // Project refs this client is subscribed to
	mu       sync.RWMutex
}

// WsHub manages all active WebSocket connections
type WsHub struct {
	clients    map[*WsClient]bool
	broadcast  chan WsMessage
	register   chan *WsClient
	unregister chan *WsClient
	logger     *slog.Logger
	mu        sync.RWMutex
}

func NewWsHub(logger *slog.Logger) *WsHub {
	return &WsHub{
		clients:    make(map[*WsClient]bool),
		broadcast:  make(chan WsMessage),
		register:   make(chan *WsClient),
		unregister: make(chan *WsClient),
		logger:     logger,
	}
}

func (h *WsHub) Run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			h.mu.Unlock()
			h.logger.Debug("WS client registered")

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
			}
			h.mu.Unlock()
			h.logger.Debug("WS client unregistered")

		case message := <-h.broadcast:
			payload, err := json.Marshal(message)
			if err != nil {
				h.logger.Error("Failed to marshal WS message", "error", err)
				continue
			}

			h.mu.RLock()
			for client := range h.clients {
				client.mu.RLock()
				subscribed := message.Project == "" || client.projects[message.Project]
				client.mu.RUnlock()

				if subscribed {
					select {
					case client.send <- payload:
					default:
						close(client.send)
						delete(h.clients, client)
					}
				}
			}
			h.mu.RUnlock()
		}
	}
}

func (a *Api) wsHandler(c *gin.Context) {
	// 1. Authenticate (handled by Gin middleware? No, we need it here for the upgrade)
	// We'll reuse the GetAccountFromRequest logic but the upgrade needs to happen first
	account, err := a.GetAccountFromRequest(c)
	if err != nil {
		a.logger.Warn("WS authentication failed", "error", err)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		a.logger.Error("Failed to upgrade to WebSocket", "error", err)
		return
	}

	client := &WsClient{
		hub:      a.wsHub,
		conn:     conn,
		send:     make(chan []byte, 256),
		projects: make(map[string]bool),
	}
	client.hub.register <- client

	// Start reader and writer
	go client.writePump()
	go client.readPump(account.GotrueID)
}

func (c *WsClient) readPump(accountID string) {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			break
		}

		var msg struct {
			Action  string `json:"action"` // e.g., "subscribe", "unsubscribe"
			Project string `json:"project"`
		}
		if err := json.Unmarshal(message, &msg); err != nil {
			continue
		}

		c.mu.Lock()
		if msg.Action == "subscribe" {
			c.projects[msg.Project] = true
			c.hub.logger.Debug("WS client subscribed", "account", accountID, "project", msg.Project)
		} else if msg.Action == "unsubscribe" {
			delete(c.projects, msg.Project)
			c.hub.logger.Debug("WS client unsubscribed", "account", accountID, "project", msg.Project)
		}
		c.mu.Unlock()
	}
}

func (c *WsClient) writePump() {
	defer func() {
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
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
		}
	}
}

// BroadcastStats sends a stats message to all subscribed clients
func (h *WsHub) BroadcastStats(projectRef string, stats interface{}) {
	h.broadcast <- WsMessage{
		Type:    "stats",
		Project: projectRef,
		Data:    stats,
	}
}

// BroadcastStatus send a status update message to all subscribed clients
func (h *WsHub) BroadcastStatus(projectRef string, status string) {
	h.broadcast <- WsMessage{
		Type:    "status_update",
		Project: projectRef,
		Data:    gin.H{"status": status},
	}
}
