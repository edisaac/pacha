package models

// ── task meta ────────────────────────────────────────────────────────────────

type TaskMeta struct {
	SolverStatus string
	Score        string
	ScoreDetail  string
	StartedAt    string
	FinishedAt   string
	ErrorMsg     string
}

type ConstraintResult struct {
	Name       string   `json:"name"`
	Label      string   `json:"label"`
	Type       string   `json:"type"`       // "hard" | "medium" | "soft"
	Matches    int      `json:"matches"`
	Violations []string `json:"violations"` // human-readable descriptions of each violation (up to 50)
}

// LessonConflict represents a detected hard collision between two lessons.
// Owned and maintained by Go — independent of the Kotlin solver.
// type: "room" | "teacher" | "group"
type LessonConflict struct {
	ID         int
	Type       string // "room" | "teacher" | "group"
	LessonIDA  int
	LessonIDB  int
	TimeslotID int
	EntityID   int    // room_id, teacher_id, or student_group_id depending on Type
	EntityName string // denormalised for zero-join rendering
}

// ── task stats ───────────────────────────────────────────────────────────────

type TaskStats struct {
	TotalRooms           int
	TotalTeachers        int
	TotalIntervals       int
	TotalSubjects        int
	TotalGroups          int
	TotalTeacherSubjects int
	TotalLessons         int
	AssignedLessons      int
	UnassignedLessons    int
	ConflictLessons      int
	CleanLessons         int
}

// ── diagnostics ──────────────────────────────────────────────────────────────

type DiagnosticWarning struct {
	EntityName string
	Required   int
	Available  int
}

type Diagnostics struct {
	GlobalRoomShortage *DiagnosticWarning
	TeacherShortages   []DiagnosticWarning
	GroupOverloads     []DiagnosticWarning
	HasWarnings        bool
}

// ── lesson data (used in schedule grids) ─────────────────────────────────────

type ConflictTypes struct {
	Group              bool
	Room               bool
	Teacher            bool
	TeacherConsistency bool
}

type LessonData struct {
	LessonID      int
	ParentID      *int
	BlockID       int
	Teacher       string
	Subject       string
	Group         string
	Room          string
	DayOfWeek     int
	StartTime     string
	EndTime       string
	BgColor       string
	TextColor     string
	Pattern       string
	HasConflict   bool // true if this lesson is involved in a room/teacher/group hard collision
	Conflicts     ConflictTypes
	IsPinned      bool
}

type TimeGroup struct {
	TimeRange string
	StartTime string
	EndTime   string
	Days        map[int][]LessonData
	TimeslotIDs map[int]int
}

type DayData struct {
	DayOfWeek int
	Name      string
}

// ── page data structs ────────────────────────────────────────────────────────

type SchedulePageData struct {
	TaskID     string
	Status     string
	Score      string
	ErrorMsg   string
	Percent    int // assigned/total * 100
	Days       []DayData
	TimeGroups []TimeGroup
	Stats      TaskStats
	Diagnostics Diagnostics
}

// ScheduleByEntityData is used by the by-room, by-teacher and by-group views.
type ScheduleByEntityData struct {
	TaskID     string
	Status     string
	ViewType   string // "room" | "teacher" | "group"
	EntityID   int
	EntityName string
	Days       []DayData
	TimeGroups []TimeGroup
	// Selector lists — only one will be populated per view type
	Rooms    []Room
	Teachers []Teacher
	Groups   []StudentGroup
}

type AnalyzePageData struct {
	TaskID      string
	Score       string
	Status      string
	Constraints []ConstraintResult
}

// ── entity structs ────────────────────────────────────────────────────────────

type Room struct {
	ID            int
	Cod           string
	Name          string
	ConflictCount int // number of timeslots where this room has double-booking
}

type Teacher struct {
	ID            int
	Cod           string
	Name          string
	ConflictCount int // number of timeslots where this teacher has double-booking
}

type Subject struct {
	ID      int
	Cod     string
	Name    string
	Color   string
	Pattern string
}

type StudentGroup struct {
	ID            int
	Cod           string
	Name          string
	ParentID      *int
	ParentCod     string
	ConflictCount int // number of timeslots where this group has a hierarchy conflict
	Level         int // Depth level for recursive trees
}

type Timeslot struct {
	ID        int
	Cod       string
	DayOfWeek int
	NameOfDay string
	StartTime string
	EndTime   string
}

type LessonDetail struct {
	ID          int
	SubjectID   int
	Subject     string
	TeacherID   int
	Teacher     string
	GroupID     int
	Group       string
	RoomID      int
	Room        string
	TimeslotID  int
	Timeslot    string
	Pattern     string
	BgColor     string
	TextColor   string
	HasConflict bool
	Conflicts   ConflictTypes
}

type LessonFilter struct {
	Search     string
	Status     string
	SubjectID    string
	SubjectName  string
	GroupID      string
	GroupName    string
	TeacherID    string
	TeacherName  string
	RoomID       string
	RoomName     string
	TimeslotID   string
	TimeslotName string
}

// ── lesson edit ───────────────────────────────────────────────────────────────

// LessonEditData is passed to the lesson edit modal template.
type LessonEditData struct {
	TaskID     string
	LessonID   int
	SubjectID  int
	SubjectName string
	GroupName  string
	// Current assignments
	CurrentTimeslotID int
	CurrentRoomID     int
	CurrentTeacherID  int
	// Available options with conflict preview
	Timeslots []TimeslotOption
	Rooms     []RoomOption
	Teachers  []TeacherOption
}

// TimeslotOption is a timeslot enriched with occupancy info for the edit modal.
type TimeslotOption struct {
	ID          int
	NameOfDay   string
	StartTime   string
	EndTime     string
	IsCurrent   bool
	OccupiedBy  string // non-empty if another lesson occupies same room at this slot
}

// RoomOption is a room enriched with occupancy info for the edit modal.
type RoomOption struct {
	ID         int
	Name       string
	IsCurrent  bool
	OccupiedBy string // non-empty if another lesson is in this room at the target slot
}

// TeacherOption is a teacher enriched with occupancy info for the edit modal.
type TeacherOption struct {
	ID         int
	Name       string
	IsCurrent  bool
	OccupiedBy string // non-empty if this teacher has another lesson at the target slot
}
