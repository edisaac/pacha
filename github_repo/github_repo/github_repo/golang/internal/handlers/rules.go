package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"path/filepath"
	"strings"

	"horarios/migracion/internal/repository"
)

type Rule struct {
	Name        string `json:"name"`
	Weight      int    `json:"weight"`
	Label       string `json:"label"`
	Description string `json:"description"`
	ScoreType   string `json:"score_type"`
}

type RulesData struct {
	TaskID string
	Status string
	Rules  []Rule
}

func HandleRulesView(dbDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Split(r.URL.Path, "/")
		if len(parts) < 4 {
			http.Error(w, "Invalid URL", http.StatusBadRequest)
			return
		}
		taskID := parts[2]
		dbPath := filepath.Join(dbDir, taskID+".db") + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)"

		db, err := sql.Open("sqlite", dbPath)
		if err != nil {
			http.Error(w, "Error opening database", http.StatusInternalServerError)
			return
		}
		defer db.Close()

		status := r.URL.Query().Get("status")
		if status == "" {
			status = "UNKNOWN"
			if meta, err := repository.GetTaskMeta(db); err == nil {
				status = meta.SolverStatus
			}
		}

		// Read rules from rule_configuration table
		rows, err := db.Query("SELECT rule_name, weight, COALESCE(label, rule_name), COALESCE(description, ''), COALESCE(score_type, 'soft') FROM rule_configuration")
		var rules []Rule
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var rule Rule
				var weight sql.NullInt64 // use NullInt64 in case hard constraints have NULL weight
				if err := rows.Scan(&rule.Name, &weight, &rule.Label, &rule.Description, &rule.ScoreType); err != nil {
					log.Printf("Error scanning rule: %v", err)
					continue
				}
				if weight.Valid {
					rule.Weight = int(weight.Int64)
				}
				rules = append(rules, rule)
			}
		}

		if len(rules) == 0 {
			rules = []Rule{
				{Name: "studentGroupSubjectVariety", Weight: 10},
				{Name: "teacherTimeEfficiency", Weight: 8},
				{Name: "teacherRoomStability", Weight: 6},
				{Name: "penalizeWeekends", Weight: 4},
				{Name: "penalizeSunday", Weight: 0},
				{Name: "studentTimeEfficiency", Weight: 0},
			}
		}

		data := RulesData{
			TaskID: taskID,
			Status: status,
			Rules:  rules,
		}

		tmpl := template.New("rules")
		tmpl = tmpl.Funcs(template.FuncMap{
			"dict": func(values ...interface{}) (map[string]interface{}, error) {
				if len(values)%2 != 0 {
					return nil, fmt.Errorf("invalid dict call")
				}
				dict := make(map[string]interface{}, len(values)/2)
				for i := 0; i < len(values); i += 2 {
					key, ok := values[i].(string)
					if !ok {
						return nil, fmt.Errorf("dict keys must be strings")
					}
					dict[key] = values[i+1]
				}
				return dict, nil
			},
		})

		tmpl, err = tmpl.ParseFiles(
			"internal/presentation/components/rules_config.html",
		)
		if err != nil {
			log.Printf("Error parsing template: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		if err := tmpl.ExecuteTemplate(w, "rules_config", data); err != nil {
			log.Printf("Error executing template: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}
}

func HandleSaveRules(dbDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Split(r.URL.Path, "/")
		if len(parts) < 3 {
			http.Error(w, "Invalid URL", http.StatusBadRequest)
			return
		}
		taskID := parts[2]
		dbPath := filepath.Join(dbDir, taskID+".db") + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)"

		if err := r.ParseForm(); err != nil {
			http.Error(w, "Error parsing form", http.StatusBadRequest)
			return
		}

		rulesDataStr := r.FormValue("rulesData")
		var rules []Rule
		if err := json.Unmarshal([]byte(rulesDataStr), &rules); err != nil {
			log.Printf("Error unmarshaling rules data: %v", err)
			http.Error(w, "Invalid rules data", http.StatusBadRequest)
			return
		}

		db, err := sql.Open("sqlite", dbPath)
		if err != nil {
			http.Error(w, "Error opening database", http.StatusInternalServerError)
			return
		}
		defer db.Close()

		// Optional: Verify that solver is not running
		meta, err := repository.GetTaskMeta(db)
		if err == nil && meta.SolverStatus != "NEW" {
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte(`<span class="text-red-600">Error: No se pueden modificar las reglas después de iniciar la optimización.</span>`))
			return
		}

		tx, err := db.Begin()
		if err != nil {
			http.Error(w, "Error starting transaction", http.StatusInternalServerError)
			return
		}

		// Set all to 0 first (or maybe just update what we received and zero the others)
		_, err = tx.Exec("UPDATE rule_configuration SET weight = 0")
		if err != nil {
			tx.Rollback()
			http.Error(w, "Error updating rules", http.StatusInternalServerError)
			return
		}

		for _, rule := range rules {
			_, err = tx.Exec("INSERT OR REPLACE INTO rule_configuration (rule_name, weight) VALUES (?, ?)", rule.Name, rule.Weight)
			if err != nil {
				tx.Rollback()
				log.Printf("Error inserting rule %s: %v", rule.Name, err)
				http.Error(w, "Error saving rules", http.StatusInternalServerError)
				return
			}
		}

		if err := tx.Commit(); err != nil {
			http.Error(w, "Error committing transaction", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte("Reglas guardadas correctamente."))
	}
}
