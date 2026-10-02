package handlers

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"path/filepath"
)

// HandleConflictsHistory returns the history of constraint conflicts over time
func HandleConflictsHistory(dbDir string, taskID string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		dbPath := filepath.Join(dbDir, taskID+".db") + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)"
		db, err := sql.Open("sqlite", dbPath)
		if err != nil {
			http.Error(w, "Failed to open DB", http.StatusInternalServerError)
			return
		}
		defer db.Close()

		rows, err := db.Query("SELECT time_millis, rule_name, count FROM conflict_history ORDER BY time_millis ASC")
		if err != nil {
			// Might not exist yet
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"tiempos":[], "rules":{}}`))
			return
		}
		defer rows.Close()

		var tiemposList []int64
		tiemposSet := make(map[int64]bool)
		rulesData := make(map[string]map[int64]int) // rule_name -> {time_millis -> count}

		for rows.Next() {
			var t int64
			var ruleName string
			var count int
			if err := rows.Scan(&t, &ruleName, &count); err == nil {
				if !tiemposSet[t] {
					tiemposSet[t] = true
					tiemposList = append(tiemposList, t)
				}
				if rulesData[ruleName] == nil {
					rulesData[ruleName] = make(map[int64]int)
				}
				rulesData[ruleName][t] = count
			}
		}

		// Normalize data: each rule should have an array of counts corresponding to tiemposList
		responseRules := make(map[string][]int)
		for rule, timeMap := range rulesData {
			var counts []int
			for _, t := range tiemposList {
				counts = append(counts, timeMap[t]) // 0 if not exists (which is correct if no violation)
			}
			responseRules[rule] = counts
		}

		var startTimeMillis int64
		_ = db.QueryRow("SELECT CAST(strftime('%s', started_at) AS INTEGER) * 1000 FROM task_meta WHERE started_at IS NOT NULL AND started_at != '' LIMIT 1").Scan(&startTimeMillis)

		response := map[string]interface{}{
			"tiempos":           tiemposList,
			"rules":             responseRules,
			"start_time_millis": startTimeMillis,
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(response); err != nil {
			log.Printf("Failed to encode conflicts history: %v\n", err)
		}
	}
}
