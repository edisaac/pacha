package handlers

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"path/filepath"
	"strings"

	"horarios/migracion/internal/service"
)

// HandleExport procesa la petición GET /export/{task_id} y genera la descarga del archivo Excel procesado.
func HandleExport(dbDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		taskID := strings.TrimPrefix(r.URL.Path, "/export/")
		if taskID == "" || strings.Contains(taskID, "/") {
			http.Error(w, "Task ID inválido", http.StatusBadRequest)
			return
		}

		dbPath := filepath.Join(dbDir, taskID+".db") + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)"
		db, err := sql.Open("sqlite", dbPath)
		if err != nil {
			log.Printf("Error abriendo BD en export para tarea %s: %v", taskID, err)
			http.Error(w, "Error al abrir la base de datos", http.StatusInternalServerError)
			return
		}
		defer db.Close()

		file, err := service.ExportToExcel(db)
		if err != nil {
			log.Printf("Error generando Excel para tarea %s: %v", taskID, err)
			http.Error(w, "Error al generar el archivo Excel", http.StatusInternalServerError)
			return
		}

		filename := fmt.Sprintf("%s.xlsx", taskID)
		w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))

		if err := file.Write(w); err != nil {
			log.Printf("Error escribiendo Excel al response para tarea %s: %v", taskID, err)
		}
	}
}
