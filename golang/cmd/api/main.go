package main

import (
	"log"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"horarios/migracion/internal/handlers"
	"horarios/migracion/internal/service"
	_ "modernc.org/sqlite"
)

var (
	SharedDataDir string
	UploadsDir    string
	DBDir         string
)

func init() {
	SharedDataDir = os.Getenv("SHARED_DATA_DIR")
	if SharedDataDir == "" {
		SharedDataDir = "../shared-data"
	}
	UploadsDir = filepath.Join(SharedDataDir, "originales")
	DBDir = filepath.Join(SharedDataDir, "db")

	appEnv := os.Getenv("APP_ENV")
	if appEnv == "" || appEnv == "local" {
		os.MkdirAll(UploadsDir, 0755)
		os.MkdirAll(DBDir, 0755)
	}

	// Configure slog based on LOGLEVEL env var
	logLevel := os.Getenv("LOGLEVEL")
	var level slog.Level
	switch strings.ToUpper(logLevel) {
	case "DEBUG":
		level = slog.LevelDebug
	case "WARN":
		level = slog.LevelWarn
	case "ERROR":
		level = slog.LevelError
	default:
		level = slog.LevelInfo // Default to INFO
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: level,
	}))
	slog.SetDefault(logger)
}

func main() {
	mux := http.NewServeMux()

	// Static assets
	fs := http.FileServer(http.Dir("./static"))
	mux.Handle("/static/", http.StripPrefix("/static/", fs))

	// Redirect root to /home
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			http.Redirect(w, r, "/home", http.StatusSeeOther)
			return
		}
		http.NotFound(w, r)
	})

	// Upload form + upload action
	mux.HandleFunc("/home", handlers.ServeUploadForm(DBDir))
	mux.HandleFunc("/upload", handlers.HandleUpload(DBDir))
	mux.HandleFunc("/use-template", handlers.HandleUseTemplate(DBDir))
	mux.HandleFunc("/dbs", handlers.HandleDBList(DBDir))

	// Delete a task DB
	mux.HandleFunc("/delete/", handlers.HandleDelete(DBDir))

	// Export task DB to Excel without IDs
	mux.HandleFunc("/export/", handlers.HandleExport(DBDir))
	// Start solver (first run)
	mux.HandleFunc("/start/", handlers.HandleStart(DBDir))
	// Stop solver (terminate early → STOPPED)
	mux.HandleFunc("/stop/", handlers.HandleStop(DBDir))
	// Restart solver from current best solution (warm start)
	mux.HandleFunc("/restart/", handlers.HandleRestart(DBDir))

	// Solver status polling (HTMX every 4s)
	mux.HandleFunc("/status/", handlers.HandleStatus(DBDir))

	// Score / constraint analysis panel
	mux.HandleFunc("/analyze/", handlers.HandleAnalyze(DBDir))

	// Save rule configuration
	mux.HandleFunc("/save-rules/", handlers.HandleSaveRules(DBDir))

	// Schedule views — all /planing/ prefixed routes dispatched here
	mux.HandleFunc("/planing/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case strings.Contains(path, "/details/"):
			handlers.HandleDetails(DBDir)(w, r)
		case strings.Contains(path, "/rules"):
			handlers.HandleRulesView(DBDir)(w, r)
		case strings.Contains(path, "/ocupabilidad"):
			handlers.HandleOcupabilidadGrid(DBDir)(w, r)
		case strings.Contains(path, "/schedule"):
			handlers.HandleScheduleGrid(DBDir)(w, r)
		case strings.Contains(path, "/lesson/"):
			handlers.HandleLessonAction(DBDir)(w, r)
		case strings.Contains(path, "/topbar/unassigned"):
			handlers.HandleTopbarUnassigned(DBDir)(w, r)
		case strings.Contains(path, "/topbar/conflict"):
			handlers.HandleTopbarConflict(DBDir)(w, r)
		case strings.Contains(path, "/pin-entity/"):
			handlers.HandlePinEntityAction(DBDir)(w, r)
		default:
			handlers.HandleView(DBDir)(w, r)
		}
	})

	// Score history for charts
	mux.HandleFunc("/api/task/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		// Extract taskID from /api/task/{taskID}/something
		parts := strings.Split(strings.TrimPrefix(path, "/api/task/"), "/")
		if len(parts) < 2 {
			http.NotFound(w, r)
			return
		}
		taskID := parts[0]
		action := parts[1]

		if action == "stream" {
			service.HandleSSE(w, r, taskID)
		} else if action == "score-history" {
			handlers.HandleScoreHistory(DBDir, taskID)(w, r)
		} else if action == "conflicts-history" {
			handlers.HandleConflictsHistory(DBDir, taskID)(w, r)
		} else {
			http.NotFound(w, r)
		}
	})

	port := os.Getenv("API_PORT")
	if port == "" {
		port = "8080"
	}

	// Start Mock SQS Queue Server on port 8081
	sqsPort := "8081"
	service.StartSQSMockServer(sqsPort)

	// Start the Go Telemetry Worker to consume the queue
	service.StartTelemetryWorker(DBDir, "http://localhost:"+sqsPort)

	log.Printf("Iniciando servidor HTMX en el puerto %s...", port)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatal(err)
	}
}
