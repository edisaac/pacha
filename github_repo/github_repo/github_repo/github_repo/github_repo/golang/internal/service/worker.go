package service

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"path/filepath"
	"time"

	"horarios/migracion/internal/repository"
)

// TelemetryDelta represents an assignment change for a lesson
type TelemetryDelta struct {
	LessonID   int  `json:"lesson_id"`
	RoomID     *int `json:"room_id"`
	TimeslotID *int `json:"timeslot_id"`
	TeacherID  *int `json:"teacher_id"`
}

// TelemetryPayload represents the incoming message from Kotlin
type TelemetryPayload struct {
	TaskID                string           `json:"task_id"`
	Score                 string           `json:"score"`
	TimeMillis            int64            `json:"time_millis"`
	HardScore             int              `json:"hard_score"`
	SoftScore             int              `json:"soft_score"`
	Status                string           `json:"status"` // RUNNING, COMPLETED, ERROR, RUNNING_HEARTBEAT
	Phase                 string           `json:"phase"`
	UnassignedLessons     int              `json:"unassigned_lessons"`
	CombinationsEvaluated int64            `json:"combinations_evaluated"`
	EvaluationsPerSecond  int64            `json:"evaluations_per_second"`
	Cambios               []TelemetryDelta `json:"cambios"`
	ErrorMsg              string           `json:"error_msg"`
}

// StartTelemetryWorker starts a background worker that polls the local SQS mock
func StartTelemetryWorker(dbDir string, sqsURL string) {
	go func() {
		log.Println("[Go Worker] Started polling local SQS for telemetry...")
		for {
			// Poll SQS
			resp, err := http.Get(sqsURL + "/queue/receive")
			if err != nil {
				time.Sleep(2 * time.Second)
				continue
			}

			var result struct {
				Messages []Message `json:"Messages"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&result); err == nil && len(result.Messages) > 0 {
				msg := result.Messages[0]
				resp.Body.Close()

				// Parse payload
				var payload TelemetryPayload
				if err := json.Unmarshal([]byte(msg.Body), &payload); err == nil {
					processTelemetry(payload, dbDir)
				} else {
					log.Printf("[Go Worker] Error decodificando payload SQS: %v\n", err)
				}

				// Delete from queue
				req, _ := http.NewRequest(http.MethodDelete, fmt.Sprintf("%s/queue/delete?receipt_handle=%s", sqsURL, msg.ReceiptHandle), nil)
				http.DefaultClient.Do(req)
			} else {
				resp.Body.Close()
			}
			
			// Short sleep to prevent busy spinning if no messages
			time.Sleep(100 * time.Millisecond)
		}
	}()
}

func processTelemetry(payload TelemetryPayload, dbDir string) {
	log.Printf("[Go Worker] Procesando telemetría para la tarea: %s (Status: %s)", payload.TaskID, payload.Status)

	if payload.Status == "STATS_UPDATE" {
		sseData := map[string]interface{}{
			"score":                  payload.Score,
			"hard_score":             payload.HardScore,
			"soft_score":             payload.SoftScore,
			"phase":                  payload.Phase,
			"unassigned_lessons":     payload.UnassignedLessons,
			"combinations_evaluated": payload.CombinationsEvaluated,
			"evaluations_per_second": payload.EvaluationsPerSecond,
			"status":                 "STATS_UPDATE",
		}
		sseBytes, _ := json.Marshal(sseData)
		sseMessage := fmt.Sprintf("event: telemetry\ndata: %s\n\n", string(sseBytes))
		GlobalSSEBroker.Broadcast(payload.TaskID, sseMessage)
		return
	}

	dbPath := filepath.Join(dbDir, payload.TaskID+".db") + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)"
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		log.Printf("[Go Worker] Error opening DB for task %s: %v\n", payload.TaskID, err)
		return
	}
	defer db.Close()

	// 1. Ensure conflict_history exists (Schema patch for existing DBs)
	_, _ = db.Exec(`
		CREATE TABLE IF NOT EXISTS conflict_history (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			task_id TEXT NOT NULL,
			time_millis INTEGER NOT NULL,
			rule_name TEXT NOT NULL,
			count INTEGER NOT NULL
		)
	`)

	// 2. Apply Deltas (Batch update)
	if len(payload.Cambios) > 0 {
		tx, err := db.Begin()
		if err == nil {
			stmt, err := tx.Prepare("UPDATE lesson SET room_id=?, timeslot_id=?, teacher_id=? WHERE id=?")
			if err == nil {
				for _, c := range payload.Cambios {
					_, _ = stmt.Exec(c.RoomID, c.TimeslotID, c.TeacherID, c.LessonID)
				}
				stmt.Close()
			}
			tx.Commit()
		}
	}

	// 3. Record Violations History
	rows, err := db.Query(`
		SELECT v.rule_name, COUNT(*) 
		FROM v_constraint_violations v
		LEFT JOIN rule_configuration rc ON v.rule_name = rc.rule_name
		WHERE v.score_type = 'hard' OR IFNULL(rc.weight, 1) > 0
		GROUP BY v.rule_name
	`)
	if err == nil {
		tx, _ := db.Begin()
		stmt, _ := tx.Prepare("INSERT INTO conflict_history (task_id, time_millis, rule_name, count) VALUES (?, ?, ?, ?)")
		for rows.Next() {
			var ruleName string
			var count int
			if err := rows.Scan(&ruleName, &count); err == nil {
				_, _ = stmt.Exec(payload.TaskID, payload.TimeMillis, ruleName, count)
			}
		}
		stmt.Close()
		rows.Close()
		tx.Commit()
	} else {
		log.Printf("[Go Worker] Error querying v_constraint_violations: %v\n", err)
	}

	// 4. Write Score History
	_, err = db.Exec(`
		INSERT INTO score_history (task_id, time_millis, hard_score, soft_score) 
		VALUES (?, ?, ?, ?)`,
		payload.TaskID, payload.TimeMillis, payload.HardScore, payload.SoftScore,
	)
	if err != nil {
		log.Printf("[Go Worker] Error inserting score_history: %v\n", err)
	}

	// 3. Update Task Meta
	if payload.Status == "COMPLETED" {
		_, err := db.Exec("UPDATE task_meta SET solver_status=?, score=?, finished_at=datetime('now') WHERE solver_status != 'COMPLETED' AND solver_status != 'STOPPED'", payload.Status, payload.Score)
		if err != nil {
			log.Printf("[Go Worker] Error updating task_meta to %s: %v\n", payload.Status, err)
		}
		if err := repository.RecordSolverEvent(db, "STOP"); err != nil {
			log.Printf("[Go Worker] Error recording STOP event: %v\n", err)
		}
	} else if payload.Status == "ERROR" {
		_, err := db.Exec("UPDATE task_meta SET solver_status=?, error_msg=?, finished_at=datetime('now') WHERE solver_status != 'COMPLETED' AND solver_status != 'STOPPED'", payload.Status, payload.ErrorMsg)
		if err != nil {
			log.Printf("[Go Worker] Error updating task_meta to %s: %v\n", payload.Status, err)
		}
		if err := repository.RecordSolverEvent(db, "STOP"); err != nil {
			log.Printf("[Go Worker] Error recording STOP event: %v\n", err)
		}
	} else if payload.Status != "" {
		dbStatus := payload.Status
		if dbStatus == "NEW_BEST_SOLUTION" {
			dbStatus = "RUNNING"
		}
		_, err := db.Exec("UPDATE task_meta SET solver_status=?, score=? WHERE solver_status != 'COMPLETED' AND solver_status != 'STOPPED'", dbStatus, payload.Score)
		if err != nil {
			log.Printf("[Go Worker] Error updating task_meta to %s: %v\n", dbStatus, err)
		}
	}

	// 4. Broadcast SSE to UI
	sseData := map[string]interface{}{
		"score": payload.Score,
		"hard_score": payload.HardScore,
		"soft_score": payload.SoftScore,
		"phase": payload.Phase,
		"unassigned_lessons": payload.UnassignedLessons,
		"combinations_evaluated": payload.CombinationsEvaluated,
		"evaluations_per_second": payload.EvaluationsPerSecond,
		"status": payload.Status,
	}
	sseBytes, _ := json.Marshal(sseData)
	sseMessage := fmt.Sprintf("event: telemetry\ndata: %s\n\n", string(sseBytes))
	GlobalSSEBroker.Broadcast(payload.TaskID, sseMessage)
	
	if payload.Status == "COMPLETED" || payload.Status == "ERROR" || payload.Status == "STOPPED" {
		time.Sleep(100 * time.Millisecond) // Slight delay
		doneMessage := fmt.Sprintf("event: solverdone\ndata: {\"status\": \"%s\"}\n\n", payload.Status)
		GlobalSSEBroker.Broadcast(payload.TaskID, doneMessage)
	}
}
