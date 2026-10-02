package service

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"
)

// SQSMockServer acts as a lightweight local queue imitating AWS SQS.
type SQSMockServer struct {
	mu       sync.Mutex
	messages map[string][]Message
	msgID    int
}

type Message struct {
	ID            string `json:"id"`
	ReceiptHandle string `json:"receipt_handle"`
	Body          string `json:"body"` // JSON payload
}

var GlobalSQS = &SQSMockServer{
	messages: make(map[string][]Message),
	msgID:    0,
}

func getQueueName(r *http.Request) string {
	q := r.URL.Query().Get("queue_name")
	if q == "" {
		return "telemetry" // default for backward compatibility
	}
	return q
}

// StartSQSMockServer starts the queue server on a separate port
func StartSQSMockServer(port string) {
	mux := http.NewServeMux()

	// POST /queue/send
	mux.HandleFunc("/queue/send", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		qName := getQueueName(r)

		var payload map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}

		bodyBytes, _ := json.Marshal(payload)

		GlobalSQS.mu.Lock()
		GlobalSQS.msgID++
		msg := Message{
			ID:            fmt.Sprintf("msg-%d", GlobalSQS.msgID),
			ReceiptHandle: fmt.Sprintf("rh-%d", GlobalSQS.msgID),
			Body:          string(bodyBytes),
		}
		if GlobalSQS.messages[qName] == nil {
			GlobalSQS.messages[qName] = make([]Message, 0)
		}
		GlobalSQS.messages[qName] = append(GlobalSQS.messages[qName], msg)
		GlobalSQS.mu.Unlock()

		log.Printf("[SQS Mock] Recibido mensaje en cola '%s' (MsgID: %s)", qName, msg.ID)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"MessageId": msg.ID})
	})

	// GET /queue/receive
	mux.HandleFunc("/queue/receive", func(w http.ResponseWriter, r *http.Request) {
		qName := getQueueName(r)
		
		// Long polling simulation: wait up to 2 seconds if queue is empty
		for i := 0; i < 20; i++ {
			GlobalSQS.mu.Lock()
			if qMsgs, ok := GlobalSQS.messages[qName]; ok && len(qMsgs) > 0 {
				msg := qMsgs[0]
				GlobalSQS.mu.Unlock()

				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string][]Message{"Messages": {msg}})
				return
			}
			GlobalSQS.mu.Unlock()
			time.Sleep(100 * time.Millisecond)
		}
		
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"Messages": []}`))
	})

	// DELETE /queue/delete
	mux.HandleFunc("/queue/delete", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		
		qName := getQueueName(r)
		receiptHandle := r.URL.Query().Get("receipt_handle")
		if receiptHandle == "" {
			http.Error(w, "receipt_handle is required", http.StatusBadRequest)
			return
		}

		GlobalSQS.mu.Lock()
		if qMsgs, ok := GlobalSQS.messages[qName]; ok {
			for i, msg := range qMsgs {
				if msg.ReceiptHandle == receiptHandle {
					// Remove message
					GlobalSQS.messages[qName] = append(qMsgs[:i], qMsgs[i+1:]...)
					break
				}
			}
		}
		GlobalSQS.mu.Unlock()

		w.WriteHeader(http.StatusOK)
	})

	log.Printf("[SQS Mock] Started local queue server on port %s", port)
	go func() {
		if err := http.ListenAndServe(":"+port, mux); err != nil {
			log.Fatalf("SQS Mock Server failed: %v", err)
		}
	}()
}
