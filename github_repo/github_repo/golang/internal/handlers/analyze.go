package handlers

import (
	"database/sql"
	"net/http"
	"path/filepath"
	"strings"

	"horarios/migracion/internal/repository"
)

func HandleAnalyze(dbDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		taskID := strings.TrimPrefix(r.URL.Path, "/analyze/")
		
		w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Expires", "0")
		
		dbPath := filepath.Join(dbDir, taskID+".db") + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)"
		db, err := sql.Open("sqlite", dbPath)
		if err != nil {
			sendErrorHTML(w, "DB error")
			return
		}
		defer db.Close()

		meta, _ := repository.GetTaskMeta(db)
		constraints, _ := repository.GetConstraints(db)

		scoreToDisplay := meta.Score
		if scoreToDisplay == "" {
			scoreToDisplay = "0hard/0soft"
		}

		data := map[string]interface{}{
			"TaskID":      taskID,
			"Score":       scoreToDisplay,
			"Status":      meta.SolverStatus,
			"Constraints": constraints,
		}

		RenderTemplate(w, "analyze_panel", "internal/presentation/components/analyze_panel.html", data)
	}
}
