package handlers

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"path/filepath"
)

// HandleScoreHistory returns the score history for the chart
func HandleScoreHistory(dbDir string, taskID string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		dbPath := filepath.Join(dbDir, taskID+".db") + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)"
		db, err := sql.Open("sqlite", dbPath)
		if err != nil {
			http.Error(w, "Failed to open DB", http.StatusInternalServerError)
			return
		}
		defer db.Close()

		rows, err := db.Query("SELECT time_millis, hard_score, soft_score FROM score_history ORDER BY time_millis ASC")
		if err != nil {
			// Might not exist yet if solver hasn't written
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"tiempos":[], "hard":[], "soft":[]}`))
			return
		}
		defer rows.Close()

		var tiempos []int64
		var hard []int
		var soft []int

		for rows.Next() {
			var t int64
			var h, s int
			if err := rows.Scan(&t, &h, &s); err == nil {
				tiempos = append(tiempos, t)
				hard = append(hard, h)
				soft = append(soft, s)
			}
		}

		var startTimeMillis int64
		_ = db.QueryRow("SELECT CAST(strftime('%s', started_at) AS INTEGER) * 1000 FROM task_meta WHERE started_at IS NOT NULL AND started_at != '' LIMIT 1").Scan(&startTimeMillis)

		type SolverEvent struct {
			Type string `json:"type"`
			Time int64  `json:"time_millis"`
		}
		var events []SolverEvent
		if eRows, err := db.Query("SELECT event_type, time_millis FROM solver_events ORDER BY time_millis ASC"); err == nil {
			defer eRows.Close()
			for eRows.Next() {
				var ev SolverEvent
				if err := eRows.Scan(&ev.Type, &ev.Time); err == nil {
					events = append(events, ev)
				}
			}
		}

		response := map[string]interface{}{
			"tiempos":           tiempos,
			"hard":              hard,
			"soft":              soft,
			"start_time_millis": startTimeMillis,
			"events":            events,
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(response); err != nil {
			log.Printf("Failed to encode score history: %v\n", err)
		}
	}
}
