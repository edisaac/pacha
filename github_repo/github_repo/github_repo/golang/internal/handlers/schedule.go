package handlers

import (
	"database/sql"
	"fmt"
	"net/http"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"horarios/migracion/internal/models"
	"horarios/migracion/internal/repository"
	"horarios/migracion/internal/service"
)

// ── HandleView ────────────────────────────────────────────────────────────────

func HandleView(dbDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		taskID := strings.TrimPrefix(r.URL.Path, "/planing/")
		renderScheduleView(w, r, taskID, dbDir, "")
	}
}

// ── HandleStop ────────────────────────────────────────────────────────────────

func HandleStop(dbDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		taskID := strings.TrimPrefix(r.URL.Path, "/stop/")
		_ = service.StopSolver(taskID)

		dbPath := filepath.Join(dbDir, taskID+".db") + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)"
		db, err := sql.Open("sqlite", dbPath)
		if err != nil {
			sendErrorHTML(w, "DB error: "+err.Error())
			return
		}
		defer db.Close()

		if err := repository.SetTaskStopped(db); err != nil {
			sendErrorHTML(w, "Error marcando tarea como detenida: "+err.Error())
			return
		}
		_ = repository.RecordSolverEvent(db, "STOP")

		renderScheduleView(w, r, taskID, dbDir, "STOPPED")
	}
}

// ── HandleRestart ─────────────────────────────────────────────────────────────

func HandleRestart(dbDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		taskID := strings.TrimPrefix(r.URL.Path, "/restart/")

		dbPath := filepath.Join(dbDir, taskID+".db") + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)"
		db, err := sql.Open("sqlite", dbPath)
		if err != nil {
			sendErrorHTML(w, "DB error: "+err.Error())
			return
		}
		defer db.Close()

		if err := repository.ResetTaskMeta(db); err != nil {
			sendErrorHTML(w, "Error reseteando tarea: "+err.Error())
			return
		}
		_ = repository.RecordSolverEvent(db, "START")

		go service.TriggerSolver(taskID, dbDir)
		renderScheduleView(w, r, taskID, dbDir, "RUNNING")
	}
}

// ── HandleStart ───────────────────────────────────────────────────────────────

func HandleStart(dbDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		taskID := strings.TrimPrefix(r.URL.Path, "/start/")
		
		dbPath := filepath.Join(dbDir, taskID+".db") + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)"
		if db, err := sql.Open("sqlite", dbPath); err == nil {
			_ = repository.ResetTaskMeta(db)
			_ = repository.RecordSolverEvent(db, "START")
			db.Close()
		}

		go service.TriggerSolver(taskID, dbDir)
		renderScheduleView(w, r, taskID, dbDir, "RUNNING")
	}
}

// ── HandleStatus ──────────────────────────────────────────────────────────────

func HandleStatus(dbDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		taskID := strings.TrimPrefix(r.URL.Path, "/status/")
		dbPath := filepath.Join(dbDir, taskID+".db") + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)"
		db, err := sql.Open("sqlite", dbPath)
		if err != nil {
			sendErrorHTML(w, "DB error")
			return
		}
		defer db.Close()

		meta, err := repository.GetTaskMeta(db)
		if err != nil && err != sql.ErrNoRows {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		stats, _ := repository.GetTaskStats(db)

		// Let Kotlin handle the status changes in task_meta.
		if meta.SolverStatus == "" {
			meta.SolverStatus = "NEW"
		}

		pct := 0
		if stats.TotalLessons > 0 {
			pct = (stats.CleanLessons * 100) / stats.TotalLessons
		}

		data := map[string]interface{}{
			"TaskID":    taskID,
			"Status":    meta.SolverStatus,
			"Score":     meta.Score,
			"ErrorMsg":  meta.ErrorMsg,
			"Stats":     stats,
			"Percent":   pct,
			"IsPolling": true,
		}

		if meta.SolverStatus == "COMPLETED" || meta.SolverStatus == "ERROR" || meta.SolverStatus == "STOPPED" {
			w.Header().Set("HX-Trigger", `{"solverdone":true}`)
		} else if meta.SolverStatus == "RUNNING" {
			w.Header().Set("HX-Trigger", `{"refreshgrid":true}`)
		}

		RenderTemplate(w, "polling_response", "internal/presentation/components/dashboard_view.html", data)
	}
}

// ── HandleLessonAction ────────────────────────────────────────────────────────

func HandleLessonAction(dbDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// URL format: /planing/{taskID}/lesson/{lessonID}/{action}
		parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/planing/"), "/")
		if len(parts) < 3 || parts[1] != "lesson" {
			http.Error(w, "Invalid path", http.StatusBadRequest)
			return
		}
		taskID := parts[0]
		var lessonIDStr, action string
		if len(parts) == 3 {
			lessonIDStr = "0"
			action = parts[2]
		} else {
			lessonIDStr = parts[2]
			action = parts[3]
		}

		if action != "add-modal" && r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var lessonID int
		if lessonIDStr != "add-modal" {
			var err error
			lessonID, err = strconv.Atoi(lessonIDStr)
			if err != nil {
				http.Error(w, "Invalid lesson ID", http.StatusBadRequest)
				return
			}
		}

		dbPath := filepath.Join(dbDir, taskID+".db") + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)"
		db, err := sql.Open("sqlite", dbPath)
		if err != nil {
			http.Error(w, "DB error: "+err.Error(), http.StatusInternalServerError)
			return
		}
		defer db.Close()

		meta, err := repository.GetTaskMeta(db)
		if err == nil && meta.SolverStatus == "RUNNING" {
			http.Error(w, "Cannot modify lessons while solver is running", http.StatusBadRequest)
			return
		}

		switch action {
		case "pin":
			err = repository.SetLessonPinned(db, lessonID, true)
		case "unpin":
			err = repository.SetLessonPinned(db, lessonID, false)
		case "unassign":
			err = repository.UnassignLesson(db, lessonID)
		case "add-modal":
			if r.Method != http.MethodGet {
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
				return
			}
			timeslotIDStr := r.URL.Query().Get("timeslot_id")
			timeslotID, _ := strconv.Atoi(timeslotIDStr)
			
			var pendingLessons []models.LessonDetail
			var selectedLessonID int

			entityType := r.URL.Query().Get("entity_type")
			entityID := r.URL.Query().Get("entity_id")
			
			filter := models.LessonFilter{Status: "unassigned"}
			if entityType == "group" {
				filter.GroupID = entityID
			} else if entityType == "teacher" {
				filter.TeacherID = entityID
			} else if entityType == "room" {
				filter.RoomID = entityID
			}
			pendingLessons, _, _ = repository.GetLessonsDetailPaginated(db, filter, 1000, 0)
			
			selectedLessonID, _ = strconv.Atoi(r.URL.Query().Get("lesson_id"))
			if selectedLessonID == 0 && len(pendingLessons) > 0 {
				selectedLessonID = pendingLessons[0].ID
			}

			var data models.LessonEditData
			if selectedLessonID > 0 {
				var err error
				data, err = repository.GetLessonForEdit(db, selectedLessonID)
				if err != nil {
					http.Error(w, "Error fetching lesson: "+err.Error(), http.StatusInternalServerError)
					return
				}
				
				if timeslotID > 0 {
					data.CurrentTimeslotID = timeslotID
					repository.MarkOccupancyForTimeslot(db, &data, timeslotID, selectedLessonID)
					
					// Update IsCurrent for the timeslots array so the UI selects it
					for i := range data.Timeslots {
						if data.Timeslots[i].ID == timeslotID {
							data.Timeslots[i].IsCurrent = true
						} else {
							data.Timeslots[i].IsCurrent = false
						}
					}

					// Smart Filtering: remove occupied teachers and rooms
					filteredTeachers := make([]models.TeacherOption, 0)
					for _, t := range data.Teachers {
						if t.OccupiedBy == "" || t.IsCurrent {
							filteredTeachers = append(filteredTeachers, t)
						}
					}
					data.Teachers = filteredTeachers

					filteredRooms := make([]models.RoomOption, 0)
					for _, rm := range data.Rooms {
						if rm.OccupiedBy == "" || rm.IsCurrent {
							filteredRooms = append(filteredRooms, rm)
						}
					}
					data.Rooms = filteredRooms
				}
			}
			
			RenderTemplate(w, "add_lesson_modal", "internal/presentation/components/add_lesson_modal.html", map[string]interface{}{
				"TaskID": taskID,
				"Lesson": data,
				"PendingLessons": pendingLessons,
				"SelectedLessonID": selectedLessonID,
				"TimeslotID": timeslotID,
				"EntityType": entityType,
				"EntityID": entityID,
			})
			return
		case "update":
			r.ParseForm()
			timeslotID, _ := strconv.Atoi(r.FormValue("timeslot_id"))
			roomID, _ := strconv.Atoi(r.FormValue("room_id"))
			teacherID, _ := strconv.Atoi(r.FormValue("teacher_id"))

			if timeslotID == 0 || roomID == 0 || teacherID == 0 {
				w.Header().Set("HX-Retarget", "#modal-error-message")
				w.Header().Set("HX-Reswap", "innerHTML")
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(`<div class="bg-red-50 text-red-600 p-3 rounded-md text-sm mb-4 border border-red-200 flex items-center gap-2"><span class="material-symbols-outlined">error</span> Debes seleccionar bloque horario, docente y aula para poder asignar la sesión.</div>`))
				return
			}

			// Validation: Hard Constraints Check
			conflict, err := repository.CheckHardConflicts(db, lessonID, timeslotID, roomID, teacherID)
			if err != nil {
				http.Error(w, "Error validating conflicts", http.StatusInternalServerError)
				return
			}

			if conflict != "" {
				w.Header().Set("HX-Retarget", "#modal-error-message")
				w.Header().Set("HX-Reswap", "innerHTML")
				w.WriteHeader(http.StatusOK) // Use 200 OK so HTMX renders the error message
				w.Write([]byte(fmt.Sprintf(`<div class="bg-red-50 text-red-600 p-3 rounded-md text-sm mb-4 border border-red-200 flex items-center gap-2"><span class="material-symbols-outlined">error</span> %s</div>`, conflict)))
				return
			}

			err = repository.UpdateLesson(db, lessonID, timeslotID, roomID, teacherID)
			if err == nil {
				w.Header().Set("HX-Trigger", `{"refreshgrid":true, "closemodal":true}`)
			}
		default:
			http.Error(w, "Unknown action", http.StatusBadRequest)
			return
		}

		if err != nil {
			http.Error(w, "Error updating lesson: "+err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("HX-Trigger", `{"refreshgrid":true}`)
		w.WriteHeader(http.StatusOK)
	}
}

// ── HandleTopbarUnassigned ───────────────────────────────────────────────────

func HandleTopbarUnassigned(dbDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/planing/"), "/")
		if len(parts) < 2 {
			http.Error(w, "Invalid path", http.StatusBadRequest)
			return
		}
		taskID := parts[0]

		dbPath := filepath.Join(dbDir, taskID+".db") + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)"
		db, err := sql.Open("sqlite", dbPath)
		if err != nil {
			http.Error(w, "DB error: "+err.Error(), http.StatusInternalServerError)
			return
		}
		defer db.Close()

		filter := models.LessonFilter{Status: "unassigned"}
		if groupID := r.URL.Query().Get("group_id"); groupID != "" {
			filter.GroupID = groupID
		}
		if teacherID := r.URL.Query().Get("teacher_id"); teacherID != "" {
			filter.TeacherID = teacherID
		}
		if roomID := r.URL.Query().Get("room_id"); roomID != "" {
			filter.RoomID = roomID
		}

		lessons, _, err := repository.GetLessonsDetailPaginated(db, filter, 1000, 0)
		if err != nil {
			http.Error(w, "Error fetching unassigned lessons", http.StatusInternalServerError)
			return
		}

		data := map[string]interface{}{
			"TaskID":  taskID,
			"Lessons": lessons,
		}

		RenderTemplate(w, "unassigned_topbar", "internal/presentation/components/unassigned_topbar.html", data)
	}
}

// ── HandlePinEntityAction ──────────────────────────────────────────────────────

func HandlePinEntityAction(dbDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/planing/"), "/")
		if len(parts) < 4 || parts[1] != "pin-entity" {
			http.Error(w, "Invalid path", http.StatusBadRequest)
			return
		}
		
		taskID := parts[0]
		entityType := parts[2]
		entityIDStr := parts[3]

		entityID, err := strconv.Atoi(entityIDStr)
		if err != nil {
			http.Error(w, "Invalid entity ID", http.StatusBadRequest)
			return
		}

		dbPath := filepath.Join(dbDir, taskID+".db") + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)"
		db, err := sql.Open("sqlite", dbPath)
		if err != nil {
			http.Error(w, "DB error: "+err.Error(), http.StatusInternalServerError)
			return
		}
		defer db.Close()

		meta, err := repository.GetTaskMeta(db)
		if err == nil && meta.SolverStatus == "RUNNING" {
			http.Error(w, "Cannot modify lessons while solver is running", http.StatusBadRequest)
			return
		}

		fmt.Printf("HandlePinEntityAction called for %s %d\n", entityType, entityID)

		success, err := repository.PinEntityIfNoConflicts(db, entityType, entityID)
		if err != nil {
			fmt.Printf("Error checking conflicts: %v\n", err)
			http.Error(w, "Error checking conflicts: "+err.Error(), http.StatusInternalServerError)
			return
		}

		fmt.Printf("PinEntityIfNoConflicts returned %v\n", success)

		if success {
			w.Header().Set("HX-Trigger", `{"refreshgrid":true}`)
			w.WriteHeader(http.StatusOK)
		} else {
			msg := fmt.Sprintf("No se puede fijar el horario para este %s porque existen conflictos.", entityType)
			w.Header().Set("HX-Trigger", fmt.Sprintf(`{"conflict-alert": {"msg": "%s"}}`, msg))
			w.WriteHeader(http.StatusOK)
		}
	}
}

// ── HandleScheduleGrid ────────────────────────────────────────────────────────

func HandleScheduleGrid(dbDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		after := strings.TrimPrefix(r.URL.Path, "/planing/")
		parts := strings.SplitN(after, "/schedule", 2)
		taskID := parts[0]
		suffix := ""
		if len(parts) > 1 {
			suffix = parts[1]
		}

		switch {
		case strings.HasPrefix(suffix, "/room/"):
			idStr := strings.TrimPrefix(suffix, "/room/")
			id, _ := strconv.Atoi(idStr)
			renderScheduleByEntity(w, taskID, dbDir, "room", id)
		case strings.HasPrefix(suffix, "/teacher/"):
			idStr := strings.TrimPrefix(suffix, "/teacher/")
			id, _ := strconv.Atoi(idStr)
			renderScheduleByEntity(w, taskID, dbDir, "teacher", id)
		case strings.HasPrefix(suffix, "/group/"):
			idStr := strings.TrimPrefix(suffix, "/group/")
			id, _ := strconv.Atoi(idStr)
			renderScheduleByEntity(w, taskID, dbDir, "group", id)
		case strings.HasPrefix(suffix, "/interval/"):
			idStr := strings.TrimPrefix(suffix, "/interval/")
			id, _ := strconv.Atoi(idStr)
			renderScheduleByEntity(w, taskID, dbDir, "interval", id)
		default:
			data := buildSchedulePageData(w, taskID, dbDir, "")
			if data != nil {
				RenderTemplate(w, "schedule_grid", "internal/presentation/components/schedule_grid.html", data)
			}
		}
	}
}

// ── HandleOcupabilidadGrid ────────────────────────────────────────────────────

func HandleOcupabilidadGrid(dbDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		after := strings.TrimPrefix(r.URL.Path, "/planing/")
		parts := strings.SplitN(after, "/ocupabilidad", 2)
		taskID := parts[0]

		data := buildSchedulePageData(w, taskID, dbDir, "")
		if data != nil {
			RenderTemplate(w, "ocupabilidad_grid", "internal/presentation/components/ocupabilidad_grid.html", data)
		}
	}
}

// ── renderScheduleByEntity ────────────────────────────────────────────────────

func renderScheduleByEntity(w http.ResponseWriter, taskID, dbDir, viewType string, entityID int) {
	dbPath := filepath.Join(dbDir, taskID+".db") + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)"
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		sendErrorHTML(w, "DB error: "+err.Error())
		return
	}
	defer db.Close()

	days, _ := repository.GetDays(db)
	conflictIDs, _ := repository.GetConflictLessonIDs(db)
	taskMeta, _ := repository.GetTaskMeta(db)

	var lessons []models.LessonData
	var entityName string

	switch viewType {
	case "room":
		rooms, _ := repository.GetRooms(db)
		for _, room := range rooms {
			if room.ID == entityID {
				entityName = room.Name
				break
			}
		}
		lessons, _ = repository.GetAssignedLessonsByRoom(db, entityID, conflictIDs)
	case "teacher":
		teachers, _ := repository.GetTeachers(db)
		for _, t := range teachers {
			if t.ID == entityID {
				entityName = t.Name
				break
			}
		}
		lessons, _ = repository.GetAssignedLessonsByTeacher(db, entityID, conflictIDs)
	case "group":
		groups, _ := repository.GetGroups(db)
		for _, g := range groups {
			if g.ID == entityID {
				entityName = g.Name
				break
			}
		}
		lessons, _ = repository.GetAssignedLessonsByGroup(db, entityID, conflictIDs)
	case "interval":
		intervals, _ := repository.GetIntervals(db)
		for _, i := range intervals {
			if i.ID == entityID {
				entityName = i.NameOfDay + " " + i.StartTime + " - " + i.EndTime
				break
			}
		}
		lessons, _ = repository.GetAssignedLessonsByInterval(db, entityID, conflictIDs)
	}

	intervals, _ := repository.GetIntervals(db)
	timeGroups := buildTimeGroups(lessons, intervals...)

	data := &models.ScheduleByEntityData{
		TaskID:     taskID,
		Status:     taskMeta.SolverStatus,
		ViewType:   viewType,
		EntityID:   entityID,
		EntityName: entityName,
		Days:       days,
		TimeGroups: timeGroups,
	}

	RenderTemplate(w, "schedule_entity_grid", "internal/presentation/components/schedule_entity_grid.html", data)
}

// ── renderScheduleView ────────────────────────────────────────────────────────

func renderScheduleView(w http.ResponseWriter, r *http.Request, taskID string, dbDir string, forcedStatus string) {
	data := buildSchedulePageData(w, taskID, dbDir, forcedStatus)
	if data != nil {
		if r.Header.Get("HX-Request") == "true" {
			RenderTemplate(w, "schedule_results", "internal/presentation/schedule_results.html", data)
		} else {
			RenderTemplate(w, "layout", "internal/presentation/schedule_results.html", data)
		}
	}
}

// ── buildSchedulePageData ─────────────────────────────────────────────────────

func buildSchedulePageData(w http.ResponseWriter, taskID string, dbDir string, forcedStatus string) *models.SchedulePageData {
	dbPath := filepath.Join(dbDir, taskID+".db") + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)"
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		sendErrorHTML(w, "Error DB: "+err.Error())
		return nil
	}
	defer db.Close()

	stats, _ := repository.GetTaskStats(db)
	meta, _ := repository.GetTaskMeta(db)
	days, _ := repository.GetDays(db)
	timeslots, _ := repository.GetIntervals(db)
	diagnostics, _ := repository.GetDiagnostics(db)

	status := forcedStatus
	if status == "" {
		status = meta.SolverStatus
		if status == "" {
			status = "NEW"
		}
	}

	conflictIDs, _ := repository.GetConflictLessonIDs(db)
	lessons, _ := repository.GetAssignedLessons(db, conflictIDs)
	timeGroups := buildTimeGroups(lessons, timeslots...)

	pct := 0
	if stats.TotalLessons > 0 {
		pct = (stats.CleanLessons * 100) / stats.TotalLessons
	}

	return &models.SchedulePageData{
		TaskID:     taskID,
		Status:     status,
		Score:      meta.Score,
		ErrorMsg:   meta.ErrorMsg,
		Percent:    pct,
		Days:       days,
		TimeGroups: timeGroups,
		Stats:      stats,
		Diagnostics: diagnostics,
	}
}

// ── buildTimeGroups ───────────────────────────────────────────────────────────

func buildTimeGroups(lessons []models.LessonData, allTimeslots ...models.Timeslot) []models.TimeGroup {
	timeGroupMap := make(map[string]*models.TimeGroup)
	var ordered []string

	for _, ts := range allTimeslots {
		timeRange := ts.StartTime + " - " + ts.EndTime
		if _, exists := timeGroupMap[timeRange]; !exists {
			timeGroupMap[timeRange] = &models.TimeGroup{
				TimeRange:   timeRange,
				StartTime:   ts.StartTime,
				EndTime:     ts.EndTime,
				Days:        make(map[int][]models.LessonData),
				TimeslotIDs: make(map[int]int),
			}
			ordered = append(ordered, timeRange)
		}
		timeGroupMap[timeRange].TimeslotIDs[ts.DayOfWeek] = ts.ID
	}

	for _, lesson := range lessons {
		timeRange := lesson.StartTime + " - " + lesson.EndTime
		tg, exists := timeGroupMap[timeRange]
		if !exists {
			tg = &models.TimeGroup{
				TimeRange:   timeRange,
				StartTime:   lesson.StartTime,
				EndTime:     lesson.EndTime,
				Days:        make(map[int][]models.LessonData),
				TimeslotIDs: make(map[int]int),
			}
			timeGroupMap[timeRange] = tg
			ordered = append(ordered, timeRange)
		}
		tg.Days[lesson.DayOfWeek] = append(tg.Days[lesson.DayOfWeek], lesson)
	}

	sort.Slice(ordered, func(i, j int) bool {
		return timeGroupMap[ordered[i]].StartTime < timeGroupMap[ordered[j]].StartTime
	})

	result := make([]models.TimeGroup, 0, len(ordered))
	for _, k := range ordered {
		result = append(result, *timeGroupMap[k])
	}
	return result
}
