package handlers

import (
	"database/sql"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"horarios/migracion/internal/models"
	"horarios/migracion/internal/repository"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

func HandleDetails(dbDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/planing/"), "/")
		// parts[0] = taskID, parts[1] = "details", parts[2] = entity
		if len(parts) < 3 || parts[1] != "details" {
			http.Error(w, "Invalid path", http.StatusBadRequest)
			return
		}

		taskID := parts[0]
		entity := parts[2]
		mode := r.URL.Query().Get("mode") // "selector" or "" (default = modal)

		dbPath := filepath.Join(dbDir, taskID+".db") + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)"
		db, err := sql.Open("sqlite", dbPath)
		if err != nil {
			http.Error(w, "DB Error", http.StatusInternalServerError)
			return
		}
		defer db.Close()

		q := r.URL.Query().Get("q")
		pageStr := r.URL.Query().Get("page")
		page := 1
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
		}
		limit := 20
		offset := (page - 1) * limit

		meta, _ := repository.GetTaskMeta(db)
		data := map[string]interface{}{
			"TaskID":      taskID,
			"Status":      meta.SolverStatus,
			"EntityTitle": cases.Title(language.Spanish).String(entity),
			"Entity":      entity,
			"Mode":        mode,
			"SearchQuery": q,
			"CurrentPage": page,
			"Limit":       limit,
		}

		switch entity {
		case "rooms":
			data["EntityTitle"] = "Aulas"
			rooms, total, _ := repository.GetRoomsPaginated(db, q, limit, offset)
			if mode == "selector" {
				counts, _ := repository.GetConflictCountByEntity(db)
				for i := range rooms {
					rooms[i].ConflictCount = counts["room"][rooms[i].ID]
				}
			}
			data["Rooms"] = rooms
			data["TotalItems"] = total
		case "teachers":
			data["EntityTitle"] = "Profesores"
			teachers, total, _ := repository.GetTeachersPaginated(db, q, limit, offset)
			if mode == "selector" {
				counts, _ := repository.GetConflictCountByEntity(db)
				for i := range teachers {
					teachers[i].ConflictCount = counts["teacher"][teachers[i].ID]
				}
			}
			data["Teachers"] = teachers
			data["TotalItems"] = total
		case "subjects":
			data["EntityTitle"] = "Cursos"
			data["Subjects"], data["TotalItems"], _ = repository.GetSubjectsPaginated(db, q, limit, offset)
		case "groups":
			data["EntityTitle"] = "Grupos"
			groups, total, _ := repository.GetRootGroupsPaginated(db, q, limit, offset)
			if mode == "selector" {
				counts, _ := repository.GetConflictCountByEntity(db)
				for i := range groups {
					groups[i].ConflictCount = counts["group"][groups[i].ID]
				}
			}
			data["Groups"] = groups
			data["TotalItems"] = total
		case "intervals":
			data["EntityTitle"] = "Intervalos"
			data["Intervals"], data["TotalItems"], _ = repository.GetIntervalsPaginated(db, q, limit, offset)
		case "lessons":
			data["EntityTitle"] = "Lecciones"
			filter := models.LessonFilter{
				Search:     q,
				Status:     r.URL.Query().Get("status"),
				SubjectID:    r.URL.Query().Get("subject_id"),
				GroupID:      r.URL.Query().Get("group_id"),
				TeacherID:    r.URL.Query().Get("teacher_id"),
				RoomID:       r.URL.Query().Get("room_id"),
				TimeslotID:   r.URL.Query().Get("timeslot_id"),
			}
			repository.PopulateFilterNames(db, &filter)
			data["Filter"] = filter
			data["Lessons"], data["TotalItems"], _ = repository.GetLessonsDetailPaginated(db, filter, limit, offset)
		case "teacher_subjects":
			if len(parts) > 3 {
				teacherIDStr := parts[3]
				teacherID, _ := strconv.Atoi(teacherIDStr)
				
				// Find teacher name
				teachers, _ := repository.GetTeachers(db)
				teacherName := "Desconocido"
				for _, t := range teachers {
					if t.ID == teacherID {
						teacherName = t.Name
						break
					}
				}
				
				data["EntityTitle"] = "Cursos de " + teacherName
				data["TeacherName"] = teacherName
				data["Subjects"], _ = repository.GetSubjectsByTeacher(db, teacherID)
			}
		case "subject_teachers":
			if len(parts) > 3 {
				subjectIDStr := parts[3]
				subjectID, _ := strconv.Atoi(subjectIDStr)
				
				// Find subject name
				subjects, _ := repository.GetSubjects(db)
				subjectName := "Desconocido"
				for _, s := range subjects {
					if s.ID == subjectID {
						subjectName = s.Name
						break
					}
				}
				
				data["EntityTitle"] = "Docentes para " + subjectName
				data["SubjectName"] = subjectName
				data["Teachers"], _ = repository.GetTeachersBySubject(db, subjectID)
			}
		case "group_subgroups":
			if len(parts) > 3 {
				groupIDStr := parts[3]
				groupID, _ := strconv.Atoi(groupIDStr)
				
				// Find group name
				groups, _ := repository.GetRootGroups(db)
				groupName := "Desconocido"
				for _, g := range groups {
					if g.ID == groupID {
						groupName = g.Name
						break
					}
				}
				
				data["EntityTitle"] = "Subgrupos de " + groupName
				data["GroupName"] = groupName
				data["SubGroups"], _ = repository.GetSubGroups(db, groupID)
			}
		default:
			http.Error(w, "Entity not found", http.StatusNotFound)
			return
		}

		if totalItems, ok := data["TotalItems"].(int); ok {
			totalPages := (totalItems + limit - 1) / limit
			if totalPages == 0 {
				totalPages = 1
			}
			data["TotalPages"] = totalPages
			data["HasNext"] = page < totalPages
			data["HasPrev"] = page > 1
			data["NextPage"] = page + 1
			data["PrevPage"] = page - 1
		}

		// Embed mode: render just the table directly
		if mode == "embed" {
			if entity == "rooms" {
				RenderTemplate(w, "room_layout", "internal/presentation/components/rooms/room_layout.html", data)
				return
			}
			if entity == "teachers" {
				RenderTemplate(w, "teacher_layout", "internal/presentation/components/teachers/teacher_layout.html", data)
				return
			}
			if entity == "groups" {
				RenderTemplate(w, "group_layout", "internal/presentation/components/groups/group_layout.html", data)
				return
			}
			if entity == "lessons" {
				RenderTemplate(w, "lesson_layout", "internal/presentation/components/lessons/lesson_layout.html", data)
				return
			}
			if entity == "intervals" {
				RenderTemplate(w, "interval_layout", "internal/presentation/components/intervals/interval_layout.html", data)
				return
			}
			if entity == "subjects" {
				RenderTemplate(w, "subject_layout", "internal/presentation/components/subjects/subject_layout.html", data)
				return
			}
			RenderTemplate(w, "entity_table", "internal/presentation/components/entity_table.html", data)
			return
		}

		if mode == "list" && entity == "rooms" {
			RenderTemplate(w, "room_list", "internal/presentation/components/rooms/room_list.html", data)
			return
		}
		if mode == "list" && entity == "teachers" {
			RenderTemplate(w, "teacher_list", "internal/presentation/components/teachers/teacher_list.html", data)
			return
		}
		if mode == "list" && entity == "groups" {
			RenderTemplate(w, "group_list", "internal/presentation/components/groups/group_list.html", data)
			return
		}
		if mode == "list" && entity == "lessons" {
			RenderTemplate(w, "lesson_list", "internal/presentation/components/lessons/lesson_list.html", data)
			return
		}
		if mode == "list" && entity == "intervals" {
			RenderTemplate(w, "interval_list", "internal/presentation/components/intervals/interval_list.html", data)
			return
		}
		if mode == "list" && entity == "subjects" {
			RenderTemplate(w, "subject_list", "internal/presentation/components/subjects/subject_list.html", data)
			return
		}

		if mode == "metrics" && entity == "rooms" {
			RenderTemplate(w, "room_metrics", "internal/presentation/components/rooms/room_metrics.html", data)
			return
		}
		if mode == "metrics" && entity == "teachers" {
			RenderTemplate(w, "teacher_metrics", "internal/presentation/components/teachers/teacher_metrics.html", data)
			return
		}
		if mode == "metrics" && entity == "groups" {
			RenderTemplate(w, "group_metrics", "internal/presentation/components/groups/group_metrics.html", data)
			return
		}
		if mode == "metrics" && entity == "lessons" {
			RenderTemplate(w, "lesson_metrics", "internal/presentation/components/lessons/lesson_metrics.html", data)
			return
		}
		if mode == "metrics" && entity == "intervals" {
			RenderTemplate(w, "interval_metrics", "internal/presentation/components/intervals/interval_metrics.html", data)
			return
		}
		if mode == "metrics" && entity == "subjects" {
			RenderTemplate(w, "subject_metrics", "internal/presentation/components/subjects/subject_metrics.html", data)
			return
		}

		// Modal for teacher subjects
		if entity == "teacher_subjects" {
			RenderTemplate(w, "teacher_subjects_modal", "internal/presentation/components/teachers/teacher_subjects_modal.html", data)
			return
		}
		
		// Modal for subject teachers
		if entity == "subject_teachers" {
			RenderTemplate(w, "subject_teachers_modal", "internal/presentation/components/subjects/subject_teachers_modal.html", data)
			return
		}
		
		// Modal for group subgroups
		if entity == "group_subgroups" {
			RenderTemplate(w, "group_subgroups_modal", "internal/presentation/components/groups/group_subgroups_modal.html", data)
			return
		}

		// Selector mode: render pills for tab-based schedule navigation
		if mode == "selector" {
			RenderTemplate(w, "entity_pills", "internal/presentation/components/entity_pills.html", data)
			return
		}

		// Default: modal
		RenderTemplate(w, "details_modal.html", "internal/presentation/details_modal.html", data)
	}
}


