package repository

import (
	"database/sql"
	_ "embed"
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	"horarios/migracion/internal/models"
)

//go:embed schema.sql
var schemaSQL string

func CreateTables(db *sql.DB) {
	_, err := db.Exec(schemaSQL)
	if err != nil {
		slog.Error("Failed to initialize database schema", "error", err)
	}
	// Migration: add is_pinned if not exists
	_, _ = db.Exec("ALTER TABLE lesson ADD COLUMN is_pinned BOOLEAN DEFAULT 0")
	// Migration: add parent_id for double lessons
	_, _ = db.Exec("ALTER TABLE lesson ADD COLUMN parent_id INTEGER REFERENCES lesson(id) ON DELETE CASCADE")
}

// ── task_meta ────────────────────────────────────────────────────────────────

func GetTaskMeta(db *sql.DB) (models.TaskMeta, error) {
	var m models.TaskMeta
	err := db.QueryRow(`
		SELECT solver_status, IFNULL(score,''), IFNULL(score_detail,''),
		       IFNULL(started_at,''), IFNULL(finished_at,''), IFNULL(error_msg,'')
		FROM task_meta LIMIT 1
	`).Scan(&m.SolverStatus, &m.Score, &m.ScoreDetail, &m.StartedAt, &m.FinishedAt, &m.ErrorMsg)
	return m, err
}

func GetConstraints(db *sql.DB) ([]models.ConstraintResult, error) {
	rows, err := db.Query(`
		SELECT v.rule_name, v.score_type, v.description, COALESCE(rc.label, v.rule_name) as label
		FROM v_constraint_violations v
		LEFT JOIN rule_configuration rc ON v.rule_name = rc.rule_name
		WHERE v.score_type = 'hard' OR IFNULL(rc.weight, 1) > 0
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	grouped := make(map[string]*models.ConstraintResult)
	for rows.Next() {
		var ruleName, scoreType, description, label string
		if err := rows.Scan(&ruleName, &scoreType, &description, &label); err != nil {
			return nil, err
		}

		if _, exists := grouped[ruleName]; !exists {
			grouped[ruleName] = &models.ConstraintResult{
				Name:       ruleName,
				Label:      label,
				Type:       scoreType,
				Matches:    0,
				Violations: []string{},
			}
		}

		grouped[ruleName].Matches++
		grouped[ruleName].Violations = append(grouped[ruleName].Violations, description)
	}

	var constraints []models.ConstraintResult
	for _, c := range grouped {
		constraints = append(constraints, *c)
	}
	return constraints, nil
}

// ResetTaskMeta clears solver results so the task can be re-solved from the
// current lesson assignments (Timefold warm-starts from existing values).
func ResetTaskMeta(db *sql.DB) error {
	_, err := db.Exec(`
		UPDATE task_meta SET
			solver_status = 'RUNNING',
			score         = '',
			score_detail  = '',
			started_at    = '',
			finished_at   = '',
			error_msg     = ''
	`)
	return err
}

// SetTaskStopped marks the task as STOPPED so the UI reflects the user's action.
// The Kotlin solver is terminated separately via StopSolver; this call ensures
// the DB state is consistent even if the solver had already finished.
func SetTaskStopped(db *sql.DB) error {
	_, err := db.Exec(`UPDATE task_meta SET solver_status='STOPPED', finished_at=datetime('now')`)
	return err
}

// SetTaskError marks the task as ERROR with the given message.
func SetTaskError(db *sql.DB, msg string) error {
	_, err := db.Exec(`UPDATE task_meta SET solver_status='ERROR', error_msg=?, finished_at=datetime('now')`, msg)
	return err
}

// RecordSolverEvent records a START or STOP event for the solver chart timeline.
func RecordSolverEvent(db *sql.DB, eventType string) error {
	var timeMillis int64
	db.QueryRow("SELECT CAST(strftime('%s', 'now') AS INTEGER) * 1000").Scan(&timeMillis)
	_, err := db.Exec(`INSERT INTO solver_events (event_type, time_millis) VALUES (?, ?)`, eventType, timeMillis)
	return err
}

// ── conflict detection ────────────────────────────────────────────────────────

func GetConflictLessonIDs(db *sql.DB) (map[int]models.ConflictTypes, error) {
	rows, err := db.Query(`SELECT lesson_id, type FROM v_lesson_conflict`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make(map[int]models.ConflictTypes)
	for rows.Next() {
		var id int
		var typ string
		if err := rows.Scan(&id, &typ); err == nil {
			c := result[id]
			if typ == "group" {
				c.Group = true
			} else if typ == "room" {
				c.Room = true
			} else if typ == "teacher" {
				c.Teacher = true
			} else if typ == "teacherConsistency" {
				c.TeacherConsistency = true
			}
			result[id] = c
		}
	}
	return result, nil
}

// GetConflictCountByEntity returns a two-level map: type → entityID → count.
func GetConflictCountByEntity(db *sql.DB) (map[string]map[int]int, error) {
	rows, err := db.Query(
		`SELECT type, entity_id, SUM(conflict_count) / 2 FROM v_lesson_conflict GROUP BY type, entity_id`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := map[string]map[int]int{
		"room":    {},
		"teacher": {},
		"group":   {},
	}
	for rows.Next() {
		var typ string
		var entityID, count int
		if err := rows.Scan(&typ, &entityID, &count); err != nil {
			continue
		}
		if _, ok := result[typ]; ok {
			result[typ][entityID] = count
		}
	}
	return result, nil
}

// ── stats ────────────────────────────────────────────────────────────────────

func GetTaskStats(db *sql.DB) (models.TaskStats, error) {
	var stats models.TaskStats

	db.QueryRow("SELECT COUNT(*) FROM room").Scan(&stats.TotalRooms)
	db.QueryRow("SELECT COUNT(*) FROM teacher").Scan(&stats.TotalTeachers)
	db.QueryRow("SELECT COUNT(*) FROM timeslot").Scan(&stats.TotalIntervals)
	db.QueryRow("SELECT COUNT(*) FROM subject").Scan(&stats.TotalSubjects)
	db.QueryRow("SELECT COUNT(*) FROM student_group").Scan(&stats.TotalGroups)
	db.QueryRow("SELECT COUNT(*) FROM teacher_subject").Scan(&stats.TotalTeacherSubjects)

	db.QueryRow("SELECT COUNT(*) FROM lesson").Scan(&stats.TotalLessons)
	db.QueryRow("SELECT COUNT(*) FROM lesson WHERE timeslot_id IS NOT NULL AND room_id IS NOT NULL AND teacher_id IS NOT NULL").Scan(&stats.AssignedLessons)
	stats.UnassignedLessons = stats.TotalLessons - stats.AssignedLessons
	db.QueryRow("SELECT COUNT(DISTINCT lesson_id) FROM v_lesson_conflict").Scan(&stats.ConflictLessons)

	db.QueryRow(`
		SELECT COUNT(*) FROM lesson l
		WHERE l.timeslot_id IS NOT NULL AND l.room_id IS NOT NULL AND l.teacher_id IS NOT NULL
		  AND l.id NOT IN (SELECT lesson_id FROM v_lesson_conflict)
	`).Scan(&stats.CleanLessons)

	return stats, nil
}

// ── diagnostics ──────────────────────────────────────────────────────────────

func GetDiagnostics(db *sql.DB) (models.Diagnostics, error) {
	var diag models.Diagnostics
	var totalIntervals int
	var totalRooms int
	var totalLessons int

	db.QueryRow("SELECT COUNT(*) FROM timeslot").Scan(&totalIntervals)
	db.QueryRow("SELECT COUNT(*) FROM room").Scan(&totalRooms)
	db.QueryRow("SELECT COUNT(*) FROM lesson").Scan(&totalLessons)

	if totalIntervals == 0 {
		return diag, nil
	}

	// 1. Global Room Shortage
	maxCapacity := totalIntervals * totalRooms
	if totalLessons > maxCapacity {
		diag.GlobalRoomShortage = &models.DiagnosticWarning{
			EntityName: "Global",
			Required:   totalLessons,
			Available:  maxCapacity,
		}
		diag.HasWarnings = true
	}

	// 2. Teacher Shortages per Subject
	// Para cada curso (subject), la cantidad de clases requeridas no puede exceder
	// la capacidad máxima de todos los profesores habilitados para dictar ese curso.
	// Capacidad de 1 profesor = total de intervalos disponibles.
	tRows, err := db.Query(`
		SELECT s.name, COUNT(l.id) as required_lessons, 
		       (SELECT COUNT(ts.teacher_id) FROM teacher_subject ts WHERE ts.subject_id = s.id) * ? as max_capacity
		FROM subject s
		JOIN lesson l ON l.subject_id = s.id
		GROUP BY s.id
		HAVING required_lessons > max_capacity
	`, totalIntervals)
	if err == nil {
		defer tRows.Close()
		for tRows.Next() {
			var w models.DiagnosticWarning
			tRows.Scan(&w.EntityName, &w.Required, &w.Available)
			diag.TeacherShortages = append(diag.TeacherShortages, w)
			diag.HasWarnings = true
		}
	}

	// 3. Group Overloads
	// Para cada grupo hoja, la suma de las clases asignadas a él y a sus padres no puede exceder el total de intervalos.
	gRows, err := db.Query(`
		SELECT leaf.name, COUNT(l.id) as required
		FROM student_group_leaf sgl
		JOIN lesson l ON l.student_group_id = sgl.group_id
		JOIN student_group leaf ON leaf.id = sgl.leaf_group_id
		GROUP BY sgl.leaf_group_id
		HAVING required > ?
	`, totalIntervals)
	if err == nil {
		defer gRows.Close()
		for gRows.Next() {
			var w models.DiagnosticWarning
			gRows.Scan(&w.EntityName, &w.Required)
			w.Available = totalIntervals
			diag.GroupOverloads = append(diag.GroupOverloads, w)
			diag.HasWarnings = true
		}
	}

	return diag, nil
}

// ── entity lists ─────────────────────────────────────────────────────────────

func GetRooms(db *sql.DB) ([]models.Room, error) {
	rows, err := db.Query("SELECT id, cod, name FROM room ORDER BY name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var res []models.Room
	for rows.Next() {
		var r models.Room
		rows.Scan(&r.ID, &r.Cod, &r.Name)
		res = append(res, r)
	}
	return res, nil
}

func GetTeachers(db *sql.DB) ([]models.Teacher, error) {
	rows, err := db.Query("SELECT id, cod, name FROM teacher ORDER BY name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var res []models.Teacher
	for rows.Next() {
		var t models.Teacher
		rows.Scan(&t.ID, &t.Cod, &t.Name)
		res = append(res, t)
	}
	return res, nil
}

// GetSubjectsByTeacher returns all subjects that a specific teacher is qualified to teach.
func GetSubjectsByTeacher(db *sql.DB, teacherID int) ([]models.Subject, error) {
	query := `
		SELECT s.id, s.cod, s.name, IFNULL(s.color, '#8AE234'), IFNULL(s.pattern, 'bg-solid')
		FROM subject s
		JOIN teacher_subject ts ON ts.subject_id = s.id
		WHERE ts.teacher_id = ?
		ORDER BY s.name
	`
	rows, err := db.Query(query, teacherID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var subjects []models.Subject
	for rows.Next() {
		var s models.Subject
		if err := rows.Scan(&s.ID, &s.Cod, &s.Name, &s.Color, &s.Pattern); err != nil {
			return nil, err
		}
		subjects = append(subjects, s)
	}
	return subjects, nil
}

// GetTeachersBySubject returns all teachers that are qualified to teach a specific subject.
func GetTeachersBySubject(db *sql.DB, subjectID int) ([]models.Teacher, error) {
	query := `
		SELECT t.id, t.cod, t.name
		FROM teacher t
		JOIN teacher_subject ts ON ts.teacher_id = t.id
		WHERE ts.subject_id = ?
		ORDER BY t.name
	`
	rows, err := db.Query(query, subjectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var teachers []models.Teacher
	for rows.Next() {
		var t models.Teacher
		if err := rows.Scan(&t.ID, &t.Cod, &t.Name); err != nil {
			return nil, err
		}
		teachers = append(teachers, t)
	}
	return teachers, nil
}

func GetSubjects(db *sql.DB) ([]models.Subject, error) {
	rows, err := db.Query("SELECT id, cod, name, IFNULL(color, '#8AE234'), IFNULL(pattern, 'bg-solid') FROM subject ORDER BY name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var res []models.Subject
	for rows.Next() {
		var s models.Subject
		rows.Scan(&s.ID, &s.Cod, &s.Name, &s.Color, &s.Pattern)
		res = append(res, s)
	}
	return res, nil
}

func GetGroups(db *sql.DB) ([]models.StudentGroup, error) {
	rows, err := db.Query("SELECT id, cod, name, parent_id, parent_cod FROM student_group ORDER BY name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var res []models.StudentGroup
	for rows.Next() {
		var g models.StudentGroup
		var pid sql.NullInt64
		var pcod sql.NullString
		rows.Scan(&g.ID, &g.Cod, &g.Name, &pid, &pcod)
		if pid.Valid {
			idVal := int(pid.Int64)
			g.ParentID = &idVal
		}
		if pcod.Valid {
			g.ParentCod = pcod.String
		}
		res = append(res, g)
	}
	return res, nil
}

func GetRootGroups(db *sql.DB) ([]models.StudentGroup, error) {
	rows, err := db.Query("SELECT id, cod, name, parent_id, parent_cod FROM student_group WHERE parent_id IS NULL ORDER BY name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var res []models.StudentGroup
	for rows.Next() {
		var g models.StudentGroup
		var pid sql.NullInt64
		var pcod sql.NullString
		rows.Scan(&g.ID, &g.Cod, &g.Name, &pid, &pcod)
		if pid.Valid {
			idVal := int(pid.Int64)
			g.ParentID = &idVal
		}
		if pcod.Valid {
			g.ParentCod = pcod.String
		}
		res = append(res, g)
	}
	return res, nil
}

func GetSubGroups(db *sql.DB, parentID int) ([]models.StudentGroup, error) {
	query := `
		WITH RECURSIVE
			subgroups(id, cod, name, parent_id, parent_cod, level) AS (
				SELECT id, cod, name, parent_id, parent_cod, 1 as level
				FROM student_group
				WHERE parent_id = ?
				UNION ALL
				SELECT sg.id, sg.cod, sg.name, sg.parent_id, sg.parent_cod, s.level + 1
				FROM student_group sg
				JOIN subgroups s ON sg.parent_id = s.id
			)
		SELECT id, cod, name, parent_id, parent_cod, level
		FROM subgroups
		ORDER BY level, name
	`
	rows, err := db.Query(query, parentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var res []models.StudentGroup
	for rows.Next() {
		var g models.StudentGroup
		var pid sql.NullInt64
		var pcod sql.NullString
		rows.Scan(&g.ID, &g.Cod, &g.Name, &pid, &pcod, &g.Level)
		if pid.Valid {
			idVal := int(pid.Int64)
			g.ParentID = &idVal
		}
		if pcod.Valid {
			g.ParentCod = pcod.String
		}
		res = append(res, g)
	}
	return res, nil
}

func GetIntervals(db *sql.DB) ([]models.Timeslot, error) {
	rows, err := db.Query("SELECT id, cod, day_of_week, name_of_day, start_time, end_time FROM timeslot ORDER BY day_of_week, start_time")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var res []models.Timeslot
	for rows.Next() {
		var t models.Timeslot
		rows.Scan(&t.ID, &t.Cod, &t.DayOfWeek, &t.NameOfDay, &t.StartTime, &t.EndTime)
		res = append(res, t)
	}
	return res, nil
}

func GetLessonsDetail(db *sql.DB) ([]models.LessonDetail, error) {
	rows, err := db.Query(`
		SELECT l.id,
		       IFNULL(s.name, ''),
		       IFNULL(t.name, ''),
		       IFNULL(sg.name, ''),
		       IFNULL(r.name, ''),
		       CASE WHEN l.timeslot_id IS NOT NULL
		            THEN ts.name_of_day || ' ' || ts.start_time
		            ELSE '' END,
		       IFNULL(s.pattern, 'bg-solid')
		FROM lesson l
		LEFT JOIN subject s       ON l.subject_id       = s.id
		LEFT JOIN teacher t       ON l.teacher_id       = t.id
		LEFT JOIN student_group sg ON l.student_group_id = sg.id
		LEFT JOIN room r          ON l.room_id          = r.id
		LEFT JOIN timeslot ts     ON l.timeslot_id      = ts.id
		ORDER BY s.name, sg.name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var res []models.LessonDetail
	for rows.Next() {
		var l models.LessonDetail
		rows.Scan(&l.ID, &l.Subject, &l.Teacher, &l.Group, &l.Room, &l.Timeslot, &l.Pattern)
		res = append(res, l)
	}
	return res, nil
}

// ── paginated entity lists ───────────────────────────────────────────────────

func GetRoomsPaginated(db *sql.DB, search string, limit, offset int) ([]models.Room, int, error) {
	var total int
	countQuery := "SELECT COUNT(*) FROM room WHERE name LIKE ? OR cod LIKE ?"
	err := db.QueryRow(countQuery, "%"+search+"%", "%"+search+"%").Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	query := "SELECT id, cod, name FROM room WHERE name LIKE ? OR cod LIKE ? ORDER BY name LIMIT ? OFFSET ?"
	rows, err := db.Query(query, "%"+search+"%", "%"+search+"%", limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var res []models.Room
	for rows.Next() {
		var r models.Room
		rows.Scan(&r.ID, &r.Cod, &r.Name)
		res = append(res, r)
	}
	return res, total, nil
}

func GetTeachersPaginated(db *sql.DB, search string, limit, offset int) ([]models.Teacher, int, error) {
	var total int
	countQuery := "SELECT COUNT(*) FROM teacher WHERE name LIKE ? OR cod LIKE ?"
	err := db.QueryRow(countQuery, "%"+search+"%", "%"+search+"%").Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	query := "SELECT id, cod, name FROM teacher WHERE name LIKE ? OR cod LIKE ? ORDER BY name LIMIT ? OFFSET ?"
	rows, err := db.Query(query, "%"+search+"%", "%"+search+"%", limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var res []models.Teacher
	for rows.Next() {
		var t models.Teacher
		rows.Scan(&t.ID, &t.Cod, &t.Name)
		res = append(res, t)
	}
	return res, total, nil
}

func GetSubjectsPaginated(db *sql.DB, search string, limit, offset int) ([]models.Subject, int, error) {
	var total int
	countQuery := "SELECT COUNT(*) FROM subject WHERE name LIKE ? OR cod LIKE ?"
	err := db.QueryRow(countQuery, "%"+search+"%", "%"+search+"%").Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	query := "SELECT id, cod, name, IFNULL(color, '#8AE234'), IFNULL(pattern, 'bg-solid') FROM subject WHERE name LIKE ? OR cod LIKE ? ORDER BY name LIMIT ? OFFSET ?"
	rows, err := db.Query(query, "%"+search+"%", "%"+search+"%", limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var res []models.Subject
	for rows.Next() {
		var s models.Subject
		rows.Scan(&s.ID, &s.Cod, &s.Name, &s.Color, &s.Pattern)
		res = append(res, s)
	}
	return res, total, nil
}

func GetRootGroupsPaginated(db *sql.DB, search string, limit, offset int) ([]models.StudentGroup, int, error) {
	var total int
	countQuery := "SELECT COUNT(*) FROM student_group WHERE parent_id IS NULL AND (name LIKE ? OR cod LIKE ?)"
	err := db.QueryRow(countQuery, "%"+search+"%", "%"+search+"%").Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	query := "SELECT id, cod, name, parent_id, parent_cod FROM student_group WHERE parent_id IS NULL AND (name LIKE ? OR cod LIKE ?) ORDER BY name LIMIT ? OFFSET ?"
	rows, err := db.Query(query, "%"+search+"%", "%"+search+"%", limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var res []models.StudentGroup
	for rows.Next() {
		var g models.StudentGroup
		var pid sql.NullInt64
		var pcod sql.NullString
		rows.Scan(&g.ID, &g.Cod, &g.Name, &pid, &pcod)
		if pid.Valid {
			idVal := int(pid.Int64)
			g.ParentID = &idVal
		}
		if pcod.Valid {
			g.ParentCod = pcod.String
		}
		res = append(res, g)
	}
	return res, total, nil
}

func GetIntervalsPaginated(db *sql.DB, search string, limit, offset int) ([]models.Timeslot, int, error) {
	var total int
	countQuery := "SELECT COUNT(*) FROM timeslot WHERE cod LIKE ? OR name_of_day LIKE ? OR start_time LIKE ? OR end_time LIKE ?"
	err := db.QueryRow(countQuery, "%"+search+"%", "%"+search+"%", "%"+search+"%", "%"+search+"%").Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	query := "SELECT id, cod, day_of_week, name_of_day, start_time, end_time FROM timeslot WHERE cod LIKE ? OR name_of_day LIKE ? OR start_time LIKE ? OR end_time LIKE ? ORDER BY day_of_week, start_time LIMIT ? OFFSET ?"
	rows, err := db.Query(query, "%"+search+"%", "%"+search+"%", "%"+search+"%", "%"+search+"%", limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var res []models.Timeslot
	for rows.Next() {
		var t models.Timeslot
		rows.Scan(&t.ID, &t.Cod, &t.DayOfWeek, &t.NameOfDay, &t.StartTime, &t.EndTime)
		res = append(res, t)
	}
	return res, total, nil
}

func GetLessonsDetailPaginated(db *sql.DB, filter models.LessonFilter, limit, offset int) ([]models.LessonDetail, int, error) {
	var total int
	isUnassignedSearch := filter.Search == ":unassigned" || filter.Search == "pending" || filter.Search == "sin asignar" || filter.Status == "unassigned"
	isAssignedSearch := filter.Status == "assigned"
	isConflictSearch := filter.Status == "conflict"

	whereClauses := []string{}
	args := []interface{}{}

	if filter.Search != "" && !isUnassignedSearch && !isAssignedSearch && !isConflictSearch {
		whereClauses = append(whereClauses, "(IFNULL(s.name, '') LIKE ? OR IFNULL(t.name, '') LIKE ? OR IFNULL(sg.name, '') LIKE ? OR IFNULL(r.name, '') LIKE ?)")
		args = append(args, "%"+filter.Search+"%", "%"+filter.Search+"%", "%"+filter.Search+"%", "%"+filter.Search+"%")
	}

	if isUnassignedSearch {
		whereClauses = append(whereClauses, "(l.timeslot_id IS NULL OR l.room_id IS NULL OR l.teacher_id IS NULL)")
	} else if isAssignedSearch {
		whereClauses = append(whereClauses, "(l.timeslot_id IS NOT NULL AND l.room_id IS NOT NULL AND l.teacher_id IS NOT NULL)")
	} else if isConflictSearch {
		whereClauses = append(whereClauses, "l.id IN (SELECT lesson_id FROM v_lesson_conflict)")
	}

	if filter.SubjectID != "" {
		whereClauses = append(whereClauses, "s.id = ?")
		args = append(args, filter.SubjectID)
	}
	if filter.GroupID != "" {
		groupIDInt, err := strconv.Atoi(filter.GroupID)
		if err == nil {
			ids, err := getAllGroupIDs(db, groupIDInt)
			if err == nil && len(ids) > 0 {
				placeholders := "?"
				args = append(args, ids[0])
				for _, id := range ids[1:] {
					placeholders += ", ?"
					args = append(args, id)
				}
				whereClauses = append(whereClauses, "sg.id IN ("+placeholders+")")
			} else {
				whereClauses = append(whereClauses, "sg.id = ?")
				args = append(args, filter.GroupID)
			}
		} else {
			whereClauses = append(whereClauses, "sg.id = ?")
			args = append(args, filter.GroupID)
		}
	}
	if filter.TeacherID != "" {
		whereClauses = append(whereClauses, "t.id = ?")
		args = append(args, filter.TeacherID)
	}
	if filter.RoomID != "" {
		whereClauses = append(whereClauses, "r.id = ?")
		args = append(args, filter.RoomID)
	}
	if filter.TimeslotID != "" {
		whereClauses = append(whereClauses, "ts.id = ?")
		args = append(args, filter.TimeslotID)
	}

	whereSQL := ""
	if len(whereClauses) > 0 {
		whereSQL = " WHERE " + strings.Join(whereClauses, " AND ")
	}

	countQuery := `
		SELECT COUNT(*) 
		FROM lesson l
		LEFT JOIN subject s       ON l.subject_id       = s.id
		LEFT JOIN teacher t       ON l.teacher_id       = t.id
		LEFT JOIN student_group sg ON l.student_group_id = sg.id
		LEFT JOIN room r          ON l.room_id          = r.id
		LEFT JOIN timeslot ts     ON l.timeslot_id      = ts.id
		` + whereSQL
	err := db.QueryRow(countQuery, args...).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	query := `
		SELECT l.id,
		       IFNULL(s.id, 0), IFNULL(s.name, ''),
		       IFNULL(t.id, 0), IFNULL(t.name, ''),
		       IFNULL(sg.id, 0), IFNULL(sg.name, ''),
		       IFNULL(r.id, 0), IFNULL(r.name, ''),
		       IFNULL(ts.id, 0),
		       CASE WHEN l.timeslot_id IS NOT NULL
		            THEN ts.name_of_day || ' ' || ts.start_time
		            ELSE '' END,
		       IFNULL(s.pattern, 'bg-solid'),
		       IFNULL(s.color, '#8AE234')
		FROM lesson l
		LEFT JOIN subject s       ON l.subject_id       = s.id
		LEFT JOIN teacher t       ON l.teacher_id       = t.id
		LEFT JOIN student_group sg ON l.student_group_id = sg.id
		LEFT JOIN room r          ON l.room_id          = r.id
		LEFT JOIN timeslot ts     ON l.timeslot_id      = ts.id
		` + whereSQL + `
		ORDER BY s.name, sg.name
		LIMIT ? OFFSET ?
	`
	
	args = append(args, limit, offset)
	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	conflictIDs, _ := GetConflictLessonIDs(db)

	var res []models.LessonDetail
	for rows.Next() {
		var l models.LessonDetail
		err := rows.Scan(
			&l.ID,
			&l.SubjectID, &l.Subject,
			&l.TeacherID, &l.Teacher,
			&l.GroupID, &l.Group,
			&l.RoomID, &l.Room,
			&l.TimeslotID, &l.Timeslot,
			&l.Pattern, &l.BgColor,
		)
		if err != nil {
			return nil, 0, err
		}

		// Compute text color based on background
		l.TextColor = "#ffffff"
		hexStr := l.BgColor
		if len(hexStr) == 7 && hexStr[0] == '#' {
			rVal, _ := strconv.ParseInt(hexStr[1:3], 16, 64)
			gVal, _ := strconv.ParseInt(hexStr[3:5], 16, 64)
			bVal, _ := strconv.ParseInt(hexStr[5:7], 16, 64)
			brightness := (rVal*299 + gVal*587 + bVal*114) / 1000
			if brightness > 128 {
				l.TextColor = "#000000"
			}
		}

		if c, ok := conflictIDs[l.ID]; ok {
			l.HasConflict = c.Group || c.Room || c.Teacher || c.TeacherConsistency
			l.Conflicts = c
		}

		res = append(res, l)
	}
	return res, total, nil
}
// ── schedule queries (all assigned lessons) ──────────────────────────────────

func PopulateFilterNames(db *sql.DB, filter *models.LessonFilter) {
	if filter.SubjectID != "" {
		db.QueryRow("SELECT name FROM subject WHERE id = ?", filter.SubjectID).Scan(&filter.SubjectName)
	}
	if filter.TeacherID != "" {
		db.QueryRow("SELECT name FROM teacher WHERE id = ?", filter.TeacherID).Scan(&filter.TeacherName)
	}
	if filter.GroupID != "" {
		db.QueryRow("SELECT name FROM student_group WHERE id = ?", filter.GroupID).Scan(&filter.GroupName)
	}
	if filter.RoomID != "" {
		db.QueryRow("SELECT name FROM room WHERE id = ?", filter.RoomID).Scan(&filter.RoomName)
	}
	if filter.TimeslotID != "" {
		db.QueryRow("SELECT name_of_day || ' ' || start_time FROM timeslot WHERE id = ?", filter.TimeslotID).Scan(&filter.TimeslotName)
	}
}

const assignedLessonsSQL = `
	SELECT l.id, IFNULL(t.name,''), s.name, IFNULL(sg.name,''), IFNULL(r.name,''),
	       ts.day_of_week, ts.name_of_day, ts.start_time, ts.end_time, IFNULL(s.color, '#8AE234'), IFNULL(s.pattern, 'bg-solid'), l.is_pinned, l.parent_id
	FROM lesson l
	LEFT JOIN teacher t        ON l.teacher_id       = t.id
	JOIN  subject s            ON l.subject_id       = s.id
	JOIN  student_group sg     ON l.student_group_id = sg.id
	LEFT JOIN room r           ON l.room_id          = r.id
	JOIN  timeslot ts          ON l.timeslot_id      = ts.id
	ORDER BY ts.start_time, ts.day_of_week
`

func GetAssignedLessons(db *sql.DB, conflictIDs map[int]models.ConflictTypes) ([]models.LessonData, error) {
	return scanLessons(db, conflictIDs, assignedLessonsSQL)
}

func GetAssignedLessonsByRoom(db *sql.DB, roomID int, conflictIDs map[int]models.ConflictTypes) ([]models.LessonData, error) {
	q := `
		SELECT l.id, IFNULL(t.name,''), s.name, IFNULL(sg.name,''), IFNULL(r.name,''),
		       ts.day_of_week, ts.name_of_day, ts.start_time, ts.end_time, IFNULL(s.color, '#8AE234'), IFNULL(s.pattern, 'bg-solid'), l.is_pinned, l.parent_id
		FROM lesson l
		LEFT JOIN teacher t        ON l.teacher_id       = t.id
		JOIN  subject s            ON l.subject_id       = s.id
		JOIN  student_group sg     ON l.student_group_id = sg.id
		LEFT JOIN room r           ON l.room_id          = r.id
		JOIN  timeslot ts          ON l.timeslot_id      = ts.id
		WHERE l.room_id = ?
		ORDER BY ts.start_time, ts.day_of_week
	`
	return scanLessons(db, conflictIDs, q, roomID)
}

func GetAssignedLessonsByInterval(db *sql.DB, intervalID int, conflictIDs map[int]models.ConflictTypes) ([]models.LessonData, error) {
	q := `
		SELECT l.id, IFNULL(t.name,''), s.name, IFNULL(sg.name,''), IFNULL(r.name,''),
		       ts.day_of_week, ts.name_of_day, ts.start_time, ts.end_time, IFNULL(s.color, '#8AE234'), IFNULL(s.pattern, 'bg-solid'), l.is_pinned, l.parent_id
		FROM lesson l
		LEFT JOIN teacher t        ON l.teacher_id       = t.id
		JOIN  subject s            ON l.subject_id       = s.id
		JOIN  student_group sg     ON l.student_group_id = sg.id
		LEFT JOIN room r           ON l.room_id          = r.id
		JOIN  timeslot ts          ON l.timeslot_id      = ts.id
		WHERE l.timeslot_id = ?
		ORDER BY r.name
	`
	return scanLessons(db, conflictIDs, q, intervalID)
}

func GetAssignedLessonsByTeacher(db *sql.DB, teacherID int, conflictIDs map[int]models.ConflictTypes) ([]models.LessonData, error) {
	q := `
		SELECT l.id, IFNULL(t.name,''), s.name, IFNULL(sg.name,''), IFNULL(r.name,''),
		       ts.day_of_week, ts.name_of_day, ts.start_time, ts.end_time, IFNULL(s.color, '#8AE234'), IFNULL(s.pattern, 'bg-solid'), l.is_pinned, l.parent_id
		FROM lesson l
		LEFT JOIN teacher t        ON l.teacher_id       = t.id
		JOIN  subject s            ON l.subject_id       = s.id
		JOIN  student_group sg     ON l.student_group_id = sg.id
		LEFT JOIN room r           ON l.room_id          = r.id
		JOIN  timeslot ts          ON l.timeslot_id      = ts.id
		WHERE l.teacher_id = ?
		ORDER BY ts.start_time, ts.day_of_week
	`
	return scanLessons(db, conflictIDs, q, teacherID)
}

func GetAssignedLessonsByGroup(db *sql.DB, groupID int, conflictIDs map[int]models.ConflictTypes) ([]models.LessonData, error) {
	ids, err := getAllGroupIDs(db, groupID)
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, nil
	}

	placeholders := "?"
	args := []interface{}{ids[0]}
	for _, id := range ids[1:] {
		placeholders += ",?"
		args = append(args, id)
	}

	q := `
		SELECT l.id, IFNULL(t.name,''), s.name, IFNULL(sg.name,''), IFNULL(r.name,''),
		       ts.day_of_week, ts.name_of_day, ts.start_time, ts.end_time, IFNULL(s.color, '#8AE234'), IFNULL(s.pattern, 'bg-solid'), l.is_pinned, l.parent_id
		FROM lesson l
		LEFT JOIN teacher t        ON l.teacher_id       = t.id
		JOIN  subject s            ON l.subject_id       = s.id
		JOIN  student_group sg     ON l.student_group_id = sg.id
		LEFT JOIN room r           ON l.room_id          = r.id
		JOIN  timeslot ts          ON l.timeslot_id      = ts.id
		WHERE l.student_group_id IN (` + placeholders + `)
		ORDER BY ts.start_time, ts.day_of_week
	`
	return scanLessons(db, conflictIDs, q, args...)
}

// getAllGroupIDs returns the given groupID plus all descendant group IDs.
func getAllGroupIDs(db *sql.DB, rootID int) ([]int, error) {
	rows, err := db.Query("SELECT id, parent_id FROM student_group")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	children := make(map[int][]int)
	for rows.Next() {
		var id int
		var pid sql.NullInt64
		rows.Scan(&id, &pid)
		if pid.Valid {
			children[int(pid.Int64)] = append(children[int(pid.Int64)], id)
		}
	}

	var ids []int
	var walk func(int)
	walk = func(id int) {
		ids = append(ids, id)
		for _, child := range children[id] {
			walk(child)
		}
	}
	walk(rootID)
	return ids, nil
}

// scanLessons is a generic helper that executes a query and scans LessonData rows.
// conflictIDs is the set of lesson IDs involved in any conflict (may be nil).
func scanLessons(db *sql.DB, conflictIDs map[int]models.ConflictTypes, query string, args ...interface{}) ([]models.LessonData, error) {
	slog.Debug("Executing query", "query", query, "args", fmt.Sprint(args...))
	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var res []models.LessonData
	for rows.Next() {
		var lessonID int
		var teacher, subject, group, room, dayName, startTime, endTime, color, pattern sql.NullString
		var dayOfWeek, parentId sql.NullInt64
		var isPinned bool
		rows.Scan(&lessonID, &teacher, &subject, &group, &room, &dayOfWeek, &dayName, &startTime, &endTime, &color, &pattern, &isPinned, &parentId)

		bgColor := color.String
		if bgColor == "" {
			bgColor = "#8AE234"
		}

		hexStr := bgColor
		if len(hexStr) > 0 && hexStr[0] == '#' {
			hexStr = hexStr[1:]
		}
		var textColor = "#000000"
		if len(hexStr) == 6 {
			var r, g, b int
			fmt.Sscanf(hexStr, "%02x%02x%02x", &r, &g, &b)
			luminance := (float64(r)*299 + float64(g)*587 + float64(b)*114) / 1000
			if luminance <= 128 {
				textColor = "#FFFFFF"
			}
		}

		c := conflictIDs[lessonID]
		hasConflict := c.Group || c.Room || c.Teacher || c.TeacherConsistency

		blockId := lessonID
		var pIdPtr *int
		if parentId.Valid {
			id := int(parentId.Int64)
			pIdPtr = &id
			blockId = id
		}

		res = append(res, models.LessonData{
			LessonID:    lessonID,
			ParentID:    pIdPtr,
			BlockID:     blockId,
			Teacher:     teacher.String,
			Subject:     subject.String,
			Group:       group.String,
			Room:        room.String,
			DayOfWeek:   int(dayOfWeek.Int64),
			StartTime:   startTime.String,
			EndTime:     endTime.String,
			BgColor:     bgColor,
			TextColor:   textColor,
			Pattern:     pattern.String,
			HasConflict: hasConflict,
			Conflicts:   c,
			IsPinned:    isPinned,
		})
	}
	return res, nil
}

// ── lesson actions ───────────────────────────────────────────────────────────

func SetLessonPinned(db *sql.DB, lessonID int, pinned bool) error {
	val := 0
	if pinned {
		val = 1
	}
	_, err := db.Exec("UPDATE lesson SET is_pinned = ? WHERE id = ? OR parent_id = ?", val, lessonID, lessonID)
	return err
}

func UnassignLesson(db *sql.DB, lessonID int) error {
	_, err := db.Exec("UPDATE lesson SET room_id = NULL, teacher_id = NULL, timeslot_id = NULL, is_pinned = 0 WHERE id = ? OR parent_id = ?", lessonID, lessonID)
	return err
}

// GetDays returns distinct days present in the timeslot table.
func GetDays(db *sql.DB) ([]models.DayData, error) {
	rows, err := db.Query("SELECT DISTINCT day_of_week, name_of_day FROM timeslot ORDER BY day_of_week")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var days []models.DayData
	for rows.Next() {
		var d models.DayData
		rows.Scan(&d.DayOfWeek, &d.Name)
		days = append(days, d)
	}
	return days, nil
}

// UpdateGroupParent updates the parent of a group.
// If parentCod is empty, parent_id is set to NULL.
func UpdateGroupParent(db *sql.DB, groupID int, parentCod string) error {
	var parentID sql.NullInt64

	if parentCod != "" {
		err := db.QueryRow("SELECT id FROM student_group WHERE cod = ?", parentCod).Scan(&parentID)
		if err != nil {
			return err
		}
	}

	var pCod sql.NullString
	if parentCod != "" {
		pCod.String = parentCod
		pCod.Valid = true
	}

	query := "UPDATE student_group SET parent_cod = ?, parent_id = ? WHERE id = ?"
	slog.Debug("Executing query", "query", query, "args", fmt.Sprint([]interface{}{pCod, parentID, groupID}))
	_, err := db.Exec(query, pCod, parentID, groupID)
	return err
}

// UpdateGroup updates a group's code, name, and parent.
func UpdateGroup(db *sql.DB, groupID int, cod, name, parentCod string) error {
	var parentID sql.NullInt64
	if parentCod != "" {
		err := db.QueryRow("SELECT id FROM student_group WHERE cod = ?", parentCod).Scan(&parentID)
		if err != nil && err != sql.ErrNoRows {
			return err
		}
	}

	var pCod sql.NullString
	if parentCod != "" {
		pCod.String = parentCod
		pCod.Valid = true
	}

	query := "UPDATE student_group SET cod = ?, name = ?, parent_cod = ?, parent_id = ? WHERE id = ?"

	slog.Debug("Executing query", "query", query, "args", fmt.Sprint([]interface{}{cod, name, pCod, parentID, groupID}))
	_, err := db.Exec(query, cod, name, pCod, parentID, groupID)
	return err
}

// CreateGroup creates a new group.
func CreateGroup(db *sql.DB, cod, name, parentCod string) error {
	var parentID sql.NullInt64
	if parentCod != "" {
		err := db.QueryRow("SELECT id FROM student_group WHERE cod = ?", parentCod).Scan(&parentID)
		if err != nil && err != sql.ErrNoRows {
			return err
		}
	}

	var pCod sql.NullString
	if parentCod != "" {
		pCod.String = parentCod
		pCod.Valid = true
	}

	query := "INSERT INTO student_group (cod, name, parent_cod, parent_id) VALUES (?, ?, ?, ?)"
	slog.Debug("Executing query", "query", query, "args", fmt.Sprint([]interface{}{cod, name, pCod, parentID}))
	_, err := db.Exec(query, cod, name, pCod, parentID)
	return err
}

// DeleteGroup deletes a group by ID.
func DeleteGroup(db *sql.DB, groupID int) error {
	query := "DELETE FROM student_group WHERE id = ?"
	slog.Debug("Executing query", "query", query, "args", fmt.Sprint([]interface{}{groupID}))
	_, err := db.Exec(query, groupID)
	return err
}

// ── lesson manual edit ────────────────────────────────────────────────────────

// GetLessonForEdit returns the full data needed to populate the edit modal.
func GetLessonForEdit(db *sql.DB, lessonID int) (models.LessonEditData, error) {
	var data models.LessonEditData
	data.LessonID = lessonID

	// Load the lesson with its current assignments and subject.
	err := db.QueryRow(`
		SELECT l.subject_id, s.name, sg.name,
		       IFNULL(l.timeslot_id, 0),
		       IFNULL(l.room_id, 0),
		       IFNULL(l.teacher_id, 0)
		FROM lesson l
		JOIN subject s ON s.id = l.subject_id
		JOIN student_group sg ON sg.id = l.student_group_id
		WHERE l.id = ?
	`, lessonID).Scan(
		&data.SubjectID, &data.SubjectName, &data.GroupName,
		&data.CurrentTimeslotID, &data.CurrentRoomID, &data.CurrentTeacherID,
	)
	if err != nil {
		return data, err
	}

	// Load all timeslots, marking the current one.
	tsRows, err := db.Query(
		`SELECT id, name_of_day, start_time, end_time FROM timeslot ORDER BY day_of_week, start_time`,
	)
	if err != nil {
		return data, err
	}
	defer tsRows.Close()
	for tsRows.Next() {
		var opt models.TimeslotOption
		tsRows.Scan(&opt.ID, &opt.NameOfDay, &opt.StartTime, &opt.EndTime)
		opt.IsCurrent = opt.ID == data.CurrentTimeslotID
		data.Timeslots = append(data.Timeslots, opt)
	}

	// Load all rooms, marking the current one.
	rRows, err := db.Query(`SELECT id, name FROM room ORDER BY name`)
	if err != nil {
		return data, err
	}
	defer rRows.Close()
	for rRows.Next() {
		var opt models.RoomOption
		rRows.Scan(&opt.ID, &opt.Name)
		opt.IsCurrent = opt.ID == data.CurrentRoomID
		data.Rooms = append(data.Rooms, opt)
	}

	// Load only teachers qualified for this subject.
	tRows, err := db.Query(`
		SELECT t.id, t.name FROM teacher t
		JOIN teacher_subject ts ON ts.teacher_id = t.id
		WHERE ts.subject_id = ?
		ORDER BY t.name
	`, data.SubjectID)
	if err != nil {
		return data, err
	}
	defer tRows.Close()
	for tRows.Next() {
		var opt models.TeacherOption
		tRows.Scan(&opt.ID, &opt.Name)
		opt.IsCurrent = opt.ID == data.CurrentTeacherID
		data.Teachers = append(data.Teachers, opt)
	}

	// Mark occupancy on options relative to the currently selected timeslot.
	// This gives the user an immediate preview of conflicts before saving.
	if data.CurrentTimeslotID > 0 {
		markOccupancy(db, &data, data.CurrentTimeslotID, lessonID)
	}

	return data, nil
}

// MarkOccupancyForTimeslot annotates RoomOption and TeacherOption with what else is
// scheduled at a given timeslot, so the modal can warn the user.
// Exported so the lesson handler can call it when the timeslot selector changes.
func MarkOccupancyForTimeslot(db *sql.DB, data *models.LessonEditData, timeslotID, excludeLessonID int) {
	markOccupancy(db, data, timeslotID, excludeLessonID)
}

// markOccupancy annotates RoomOption and TeacherOption with what else is
// scheduled at a given timeslot, so the modal can warn the user.
func markOccupancy(db *sql.DB, data *models.LessonEditData, timeslotID, excludeLessonID int) {
	// Rooms occupied at this timeslot (by lessons other than the one being edited).
	roomRows, err := db.Query(`
		SELECT l.room_id, s.name || ' (' || sg.name || ')'
		FROM lesson l
		JOIN subject s ON s.id = l.subject_id
		JOIN student_group sg ON sg.id = l.student_group_id
		WHERE l.timeslot_id = ? AND l.room_id IS NOT NULL AND l.id <> ?
	`, timeslotID, excludeLessonID)
	if err == nil {
		defer roomRows.Close()
		occupiedRooms := make(map[int]string)
		for roomRows.Next() {
			var rID int
			var desc string
			roomRows.Scan(&rID, &desc)
			occupiedRooms[rID] = desc
		}
		for i := range data.Rooms {
			data.Rooms[i].OccupiedBy = occupiedRooms[data.Rooms[i].ID]
		}
	}

	// Teachers busy at this timeslot.
	teacherRows, err := db.Query(`
		SELECT l.teacher_id, s.name || ' (' || sg.name || ')'
		FROM lesson l
		JOIN subject s ON s.id = l.subject_id
		JOIN student_group sg ON sg.id = l.student_group_id
		WHERE l.timeslot_id = ? AND l.teacher_id IS NOT NULL AND l.id <> ?
	`, timeslotID, excludeLessonID)
	if err == nil {
		defer teacherRows.Close()
		busyTeachers := make(map[int]string)
		for teacherRows.Next() {
			var tID int
			var desc string
			teacherRows.Scan(&tID, &desc)
			busyTeachers[tID] = desc
		}
		for i := range data.Teachers {
			data.Teachers[i].OccupiedBy = busyTeachers[data.Teachers[i].ID]
		}
	}
}

// CheckHardConflicts verifies if assigning a lesson to a specific timeslot, room, and teacher
// creates a hard conflict. Returns an error message if a conflict is found, or an empty string if it's safe.
func CheckHardConflicts(db *sql.DB, lessonID, timeslotID, roomID, teacherID int) (string, error) {
	if timeslotID <= 0 {
		return "", nil // No timeslot, no time-based conflicts possible
	}

	var count int

	// Check Teacher Conflict
	if teacherID > 0 {
		err := db.QueryRow(`SELECT COUNT(*) FROM lesson WHERE timeslot_id = ? AND teacher_id = ? AND id != ?`, timeslotID, teacherID, lessonID).Scan(&count)
		if err != nil {
			return "", err
		}
		if count > 0 {
			return "El docente ya está asignado a otra clase en este horario.", nil
		}
	}

	// Check Room Conflict
	if roomID > 0 {
		err := db.QueryRow(`SELECT COUNT(*) FROM lesson WHERE timeslot_id = ? AND room_id = ? AND id != ?`, timeslotID, roomID, lessonID).Scan(&count)
		if err != nil {
			return "", err
		}
		if count > 0 {
			return "El aula ya está asignada a otra clase en este horario.", nil
		}
	}

	// Check Group Conflict
	var groupID int
	err := db.QueryRow(`SELECT student_group_id FROM lesson WHERE id = ?`, lessonID).Scan(&groupID)
	if err != nil {
		return "", err
	}
	if groupID > 0 {
		err = db.QueryRow(`SELECT COUNT(*) FROM lesson WHERE timeslot_id = ? AND student_group_id = ? AND id != ?`, timeslotID, groupID, lessonID).Scan(&count)
		if err != nil {
			return "", err
		}
		if count > 0 {
			return "La sección ya tiene otra clase en este horario.", nil
		}
	}

	// Check Teacher Consistency
	if teacherID > 0 && groupID > 0 {
		var subjectID int
		err := db.QueryRow(`SELECT subject_id FROM lesson WHERE id = ?`, lessonID).Scan(&subjectID)
		if err == nil && subjectID > 0 {
			err = db.QueryRow(`SELECT COUNT(*) FROM lesson WHERE student_group_id = ? AND subject_id = ? AND teacher_id IS NOT NULL AND teacher_id != ? AND id != ?`, groupID, subjectID, teacherID, lessonID).Scan(&count)
			if err == nil && count > 0 {
				return "Inconsistencia: El grupo ya tiene otro docente para esta asignatura.", nil
			}
		}
	}

	return "", nil
}

// UpdateLesson saves a manual lesson assignment to the database.
// Pass 0 for any field to set it to NULL (unassign).
func UpdateLesson(db *sql.DB, lessonID, timeslotID, roomID, teacherID int) error {
	toNull := func(v int) interface{} {
		if v == 0 {
			return nil
		}
		return v
	}
	_, err := db.Exec(
		`UPDATE lesson SET timeslot_id = ?, room_id = ?, teacher_id = ? WHERE id = ?`,
		toNull(timeslotID), toNull(roomID), toNull(teacherID), lessonID,
	)
	return err
}

func PinEntityIfNoConflicts(db *sql.DB, entityType string, entityID int) (bool, error) {
	conflictIDs, err := GetConflictLessonIDs(db)
	if err != nil {
		return false, err
	}

	var lessons []models.LessonData
	switch entityType {
	case "teacher":
		lessons, err = GetAssignedLessonsByTeacher(db, entityID, conflictIDs)
	case "room":
		lessons, err = GetAssignedLessonsByRoom(db, entityID, conflictIDs)
	case "group":
		lessons, err = GetAssignedLessonsByGroup(db, entityID, conflictIDs)
	case "interval":
		lessons, err = GetAssignedLessonsByInterval(db, entityID, conflictIDs)
	default:
		return false, fmt.Errorf("invalid entity type: %s", entityType)
	}

	if err != nil {
		return false, err
	}

	if len(lessons) == 0 {
		// Nothing to pin
		return true, nil
	}

	// Check if any lesson has a conflict
	for _, l := range lessons {
		if l.HasConflict {
			return false, nil
		}
	}

	// If no conflicts, pin all lessons
	tx, err := db.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()

	for _, l := range lessons {
		// val = 1 (true) for is_pinned
		_, err = tx.Exec("UPDATE lesson SET is_pinned = 1 WHERE id = ? OR parent_id = ?", l.BlockID, l.BlockID)
		if err != nil {
			return false, err
		}
	}

	return true, tx.Commit()
}
