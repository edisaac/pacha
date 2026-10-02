package service

import (
	"fmt"
	"net/http"
	"sync"
)

// SSEBroker manages SSE connections for different tasks
type SSEBroker struct {
	mu      sync.RWMutex
	clients map[string]map[chan string]bool
}

var GlobalSSEBroker = &SSEBroker{
	clients: make(map[string]map[chan string]bool),
}

// AddClient registers a new SSE client for a specific task
func (b *SSEBroker) AddClient(taskID string, ch chan string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.clients[taskID] == nil {
		b.clients[taskID] = make(map[chan string]bool)
	}
	b.clients[taskID][ch] = true
}

// RemoveClient removes an SSE client
func (b *SSEBroker) RemoveClient(taskID string, ch chan string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if clientsForTask, ok := b.clients[taskID]; ok {
		delete(clientsForTask, ch)
		if len(clientsForTask) == 0 {
			delete(b.clients, taskID)
		}
	}
}

// Broadcast sends a message to all clients listening to a specific task
func (b *SSEBroker) Broadcast(taskID string, message string) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if clientsForTask, ok := b.clients[taskID]; ok {
		for ch := range clientsForTask {
			// Non-blocking send to avoid getting stuck on a slow client
			select {
			case ch <- message:
			default:
			}
		}
	}
}

// HandleSSE is the HTTP handler for SSE connections
func HandleSSE(w http.ResponseWriter, r *http.Request, taskID string) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// Ensure the response writer supports flushing
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported!", http.StatusInternalServerError)
		return
	}

	// Create a channel for this client
	ch := make(chan string, 10)
	GlobalSSEBroker.AddClient(taskID, ch)

	// Clean up when the client disconnects
	defer func() {
		GlobalSSEBroker.RemoveClient(taskID, ch)
		close(ch)
	}()

	// Send an initial connected message
	fmt.Fprintf(w, "event: connected\ndata: {\"status\": \"connected\"}\n\n")
	flusher.Flush()

	// Listen for messages or client disconnection
	for {
		select {
		case msg := <-ch:
			// HTMX SSE extension expects the event name. We can just use "message" or "refreshgrid".
			// Here we assume msg contains the event name and data if needed, or we just send an event.
			// Format:
			// event: eventName
			// data: eventData
			//
			fmt.Fprint(w, msg)
			flusher.Flush()
		case <-r.Context().Done():
			// Client closed connection
			return
		}
	}
}
