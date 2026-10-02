package service

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"horarios/migracion/internal/repository"
)

func getSQSCommandURL() string {
	// Usually running on localhost:8081 locally
	return "http://localhost:8081/queue/send?queue_name=commands"
}

func sendCommandToSQS(command, taskID string) error {
	url := getSQSCommandURL()
	payload := fmt.Sprintf(`{"command":"%s","task_id":"%s"}`, command, taskID)
	
	log.Printf("[Go] Enviando comando %s a SQS: %s", command, url)
	resp, err := http.Post(url, "application/json", strings.NewReader(payload))
	if err != nil {
		log.Printf("[Go] Error enviando comando %s a SQS: %v", command, err)
		return err
	}
	defer resp.Body.Close()
	log.Printf("[Go] Comando %s encolado en SQS con estado: %s", command, resp.Status)
	return nil
}

func TriggerSolver(taskID, dbDir string) {
	appEnv := os.Getenv("APP_ENV")
	if appEnv == "" || appEnv == "local" {
		err := sendCommandToSQS("SOLVE", taskID)
		if err != nil {
			dbPath := filepath.Join(dbDir, taskID+".db") + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)"
			db, dbErr := sql.Open("sqlite", dbPath)
			if dbErr == nil {
				defer db.Close()
				repository.SetTaskError(db, fmt.Sprintf("Error enviando comando a SQS: %v", err))
			}
		}
	}
}

func StopSolver(taskID string) error {
	appEnv := os.Getenv("APP_ENV")
	if appEnv == "" || appEnv == "local" {
		return sendCommandToSQS("STOP", taskID)
	}
	return nil
}
