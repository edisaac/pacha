package handlers

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/xuri/excelize/v2"

	"horarios/migracion/internal/repository"
	"horarios/migracion/internal/service"
)

type DBInfo struct {
	Name         string
	Status       string
	LastModified string
	ModTimeRaw   time.Time
}

func getDatabaseList(dbDir string) []DBInfo {
	files, err := os.ReadDir(dbDir)
	var dbs []DBInfo
	if err == nil {
		for _, f := range files {
			if !f.IsDir() && strings.HasSuffix(f.Name(), ".db") {
				dbName := strings.TrimSuffix(f.Name(), ".db")
				dbPath := filepath.Join(dbDir, f.Name()) + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)"
				status := "UNKNOWN"

				info, err := f.Info()
				var modTimeRaw time.Time
				lastModStr := ""
				if err == nil {
					modTimeRaw = info.ModTime()
					lastModStr = modTimeRaw.Format("02/01/2006 15:04")
				}

				db, err := sql.Open("sqlite", dbPath)
				if err == nil {
					meta, metaErr := repository.GetTaskMeta(db)
					if metaErr == nil {
						status = meta.SolverStatus
					}
					db.Close()
				}
				dbs = append(dbs, DBInfo{
					Name:         dbName,
					Status:       status,
					LastModified: lastModStr,
					ModTimeRaw:   modTimeRaw,
				})
			}
		}
	}

	sort.Slice(dbs, func(i, j int) bool {
		return dbs[i].ModTimeRaw.After(dbs[j].ModTimeRaw)
	})

	return dbs
}

func ServeUploadForm(dbDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		dbs := getDatabaseList(dbDir)

		var templates []string
		templateFiles, err := os.ReadDir("./static/templates")
		if err == nil {
			for _, f := range templateFiles {
				if !f.IsDir() && strings.HasSuffix(f.Name(), ".xlsx") {
					templates = append(templates, f.Name())
				}
			}
		}

		data := struct {
			Databases []DBInfo
			Templates []string
		}{
			Databases: dbs,
			Templates: templates,
		}

		RenderTemplate(w, "layout", "internal/presentation/upload.html", data)
	}
}

func HandleDBList(dbDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		dbs := getDatabaseList(dbDir)
		data := struct {
			Databases []DBInfo
		}{
			Databases: dbs,
		}
		// render just the partial
		RenderTemplate(w, "db_list", "internal/presentation/components/db_list.html", data)
	}
}

func HandleUpload(dbDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r.ParseMultipartForm(10 << 20)
		file, handler, err := r.FormFile("file")
		if err != nil {
			sendErrorHTML(w, "Error al recuperar el archivo.")
			return
		}
		defer file.Close()

		ext := filepath.Ext(handler.Filename)
		baseName := strings.TrimSuffix(handler.Filename, ext)
		baseName = strings.ReplaceAll(baseName, " ", "_")
		b := make([]byte, 3)
		rand.Read(b)
		randPart := hex.EncodeToString(b)
		taskID := baseName + "_" + randPart

		log.Printf("[Go] Archivo recibido: %s, asignando Task ID: %s", handler.Filename, taskID)

		if ext != ".xlsx" && ext != ".csv" {
			sendErrorHTML(w, "Solo se permite .xlsx o .csv")
			return
		}

		f, err := excelize.OpenReader(file)
		if err != nil {
			sendErrorHTML(w, "No se pudo leer Excel.")
			return
		}
		defer f.Close()

		dbPath := filepath.Join(dbDir, taskID+".db") + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)"
		db, err := sql.Open("sqlite", dbPath)
		if err != nil {
			sendErrorHTML(w, "Error DB.")
			return
		}
		defer db.Close()

		db.Exec("PRAGMA journal_mode=WAL; PRAGMA synchronous=NORMAL;")
		repository.CreateTables(db) // also creates task_meta with NEW row

		log.Printf("[Go] Iniciando llenado de base de datos SQLite para la tarea: %s", taskID)
		err = service.GenerateData(db, f)
		if err != nil {
			sendErrorHTML(w, "Error procesando el Excel: "+err.Error())
			return
		}

		log.Printf("[Go] Finalizado el llenado de DB para la tarea: %s", taskID)
		w.Header().Set("HX-Push-Url", "/planing/"+taskID)
		renderScheduleView(w, r, taskID, dbDir, "")
	}
}

func HandleUseTemplate(dbDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		templateName := r.URL.Query().Get("name")
		if templateName == "" {
			sendErrorHTML(w, "Nombre de plantilla no proporcionado.")
			return
		}

		filePath := filepath.Join("./static/templates", templateName)
		file, err := os.Open(filePath)
		if err != nil {
			log.Printf("[Go] Error abriendo plantilla %s: %v", filePath, err)
			sendErrorHTML(w, "Error al cargar la plantilla.")
			return
		}
		defer file.Close()

		ext := filepath.Ext(templateName)
		baseName := strings.TrimSuffix(templateName, ext)
		baseName = strings.ReplaceAll(baseName, " ", "_")
		b := make([]byte, 3)
		rand.Read(b)
		randPart := hex.EncodeToString(b)
		taskID := baseName + "_" + randPart

		log.Printf("[Go] Plantilla seleccionada: %s, asignando Task ID: %s", templateName, taskID)

		if ext != ".xlsx" && ext != ".csv" {
			sendErrorHTML(w, "Solo se permite .xlsx o .csv")
			return
		}

		f, err := excelize.OpenReader(file)
		if err != nil {
			sendErrorHTML(w, "No se pudo leer Excel de la plantilla.")
			return
		}
		defer f.Close()

		dbPath := filepath.Join(dbDir, taskID+".db") + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)"
		db, err := sql.Open("sqlite", dbPath)
		if err != nil {
			sendErrorHTML(w, "Error DB.")
			return
		}
		defer db.Close()

		db.Exec("PRAGMA journal_mode=WAL; PRAGMA synchronous=NORMAL;")
		repository.CreateTables(db)

		log.Printf("[Go] Iniciando llenado de base de datos SQLite para la tarea (desde plantilla): %s", taskID)
		err = service.GenerateData(db, f)
		if err != nil {
			sendErrorHTML(w, "Error procesando el Excel de la plantilla: "+err.Error())
			return
		}

		log.Printf("[Go] Finalizado el llenado de DB para la tarea: %s", taskID)
		w.Header().Set("HX-Push-Url", "/planing/"+taskID)
		renderScheduleView(w, r, taskID, dbDir, "")
	}
}

func sendErrorHTML(w http.ResponseWriter, msg string) {
	fmt.Fprintf(w, `<div class="bg-red-50 p-4 text-red-700 rounded-lg">%s<br><button onclick="window.location.reload()" class="mt-2 text-sm underline">Reintentar</button></div>`, msg)
}

func HandleDelete(dbDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		taskID := strings.TrimPrefix(r.URL.Path, "/delete/")
		if taskID == "" {
			http.Error(w, "Task ID required", http.StatusBadRequest)
			return
		}

		dbPath := filepath.Join(dbDir, taskID+".db")
		err := os.Remove(dbPath)
		if err != nil {
			log.Printf("[Go] Error deleting DB %s: %v", dbPath, err)
			http.Error(w, "Error deleting file", http.StatusInternalServerError)
			return
		}

		log.Printf("[Go] DB deleted: %s", dbPath)
		w.WriteHeader(http.StatusOK)
	}
}
