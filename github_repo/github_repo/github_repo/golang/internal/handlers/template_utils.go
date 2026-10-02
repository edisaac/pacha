package handlers

import (
	"errors"
	"html/template"
	"log"
	"net/http"
	"path/filepath"
	"reflect"

	"horarios/migracion/internal/models"
)

var funcMap = template.FuncMap{
	"dict": func(values ...interface{}) (map[string]interface{}, error) {
		if len(values)%2 != 0 {
			return nil, errors.New("invalid dict call")
		}
		dict := make(map[string]interface{}, len(values)/2)
		for i := 0; i < len(values); i += 2 {
			key, ok := values[i].(string)
			if !ok {
				return nil, errors.New("dict keys must be strings")
			}
			dict[key] = values[i+1]
		}
		return dict, nil
	},
	"not": func(v interface{}) bool {
		if v == nil {
			return true
		}
		rv := reflect.ValueOf(v)
		switch rv.Kind() {
		case reflect.Bool:
			return !rv.Bool()
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			return rv.Int() == 0
		case reflect.String:
			return rv.String() == ""
		case reflect.Slice, reflect.Array, reflect.Map:
			return rv.Len() == 0
		case reflect.Ptr, reflect.Interface:
			return rv.IsNil()
		}
		return false
	},
	"gt": func(a, b int) bool { return a > b },
	// computePct returns integer percentage (0-100), safe against division by zero.
	"computePct": func(assigned, total int) int {
		if total == 0 {
			return 0
		}
		return (assigned * 100) / total
	},
	// isNil returns true if v is nil OR a nil pointer — works correctly with *int, *string, etc.
	"isNil": func(v interface{}) bool {
		if v == nil {
			return true
		}
		rv := reflect.ValueOf(v)
		return rv.Kind() == reflect.Ptr && rv.IsNil()
	},
	"countConflicts": func(v interface{}) int {
		if v == nil {
			return 0
		}
		count := 0
		if lessons, ok := v.([]models.LessonData); ok {
			for _, l := range lessons {
				if l.HasConflict {
					count++
				}
			}
		}
		return count
	},
}

// RenderTemplate parses the main template file plus all component partials,
// then executes the named template.
func RenderTemplate(w http.ResponseWriter, templateName string, mainFile string, data interface{}) {
	components, err := filepath.Glob("internal/presentation/components/*.html")
	if err != nil {
		http.Error(w, "Error cargando componentes", http.StatusInternalServerError)
		log.Printf("Glob error: %v", err)
		return
	}

	roomComponents, err := filepath.Glob("internal/presentation/components/rooms/*.html")
	if err != nil {
		http.Error(w, "Error cargando componentes de rooms", http.StatusInternalServerError)
		log.Printf("Glob error (rooms): %v", err)
		return
	}
	components = append(components, roomComponents...)

	teacherComponents, err := filepath.Glob("internal/presentation/components/teachers/*.html")
	if err != nil {
		http.Error(w, "Error cargando componentes de teachers", http.StatusInternalServerError)
		log.Printf("Glob error (teachers): %v", err)
		return
	}
	components = append(components, teacherComponents...)

	groupComponents, err := filepath.Glob("internal/presentation/components/groups/*.html")
	if err != nil {
		http.Error(w, "Error cargando componentes de groups", http.StatusInternalServerError)
		log.Printf("Glob error (groups): %v", err)
		return
	}
	components = append(components, groupComponents...)

	lessonComponents, err := filepath.Glob("internal/presentation/components/lessons/*.html")
	if err != nil {
		http.Error(w, "Error cargando componentes de lessons", http.StatusInternalServerError)
		log.Printf("Glob error (lessons): %v", err)
		return
	}
	components = append(components, lessonComponents...)

	intervalComponents, err := filepath.Glob("internal/presentation/components/intervals/*.html")
	if err != nil {
		http.Error(w, "Error cargando componentes de intervals", http.StatusInternalServerError)
		log.Printf("Glob error (intervals): %v", err)
		return
	}
	components = append(components, intervalComponents...)

	subjectComponents, err := filepath.Glob("internal/presentation/components/subjects/*.html")
	if err != nil {
		http.Error(w, "Error cargando componentes de subjects", http.StatusInternalServerError)
		log.Printf("Glob error (subjects): %v", err)
		return
	}
	components = append(components, subjectComponents...)

	files := append([]string{mainFile}, components...)

	// Always include the base layout so any template can be rendered as a full page
	files = append(files, "internal/presentation/layout.html")

	tmpl, err := template.New(filepath.Base(mainFile)).Funcs(funcMap).ParseFiles(files...)
	if err != nil {
		errMsg := "Error parseando plantillas: " + err.Error()
		http.Error(w, errMsg, http.StatusInternalServerError)
		log.Printf("Parse error for template %q: %v\nArchivos intentados: %v", templateName, err, files)
		return
	}

	if err = tmpl.ExecuteTemplate(w, templateName, data); err != nil {
		log.Printf("Execute error for template %q: %v", templateName, err)
	}
}
