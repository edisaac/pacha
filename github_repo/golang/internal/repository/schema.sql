CREATE TABLE IF NOT EXISTS timeslot (id INTEGER PRIMARY KEY, cod TEXT, day_of_week INTEGER, name_of_day TEXT, start_time TEXT, end_time TEXT);
CREATE TABLE IF NOT EXISTS room (id INTEGER PRIMARY KEY, cod TEXT UNIQUE, name TEXT);
CREATE TABLE IF NOT EXISTS teacher (id INTEGER PRIMARY KEY, cod TEXT UNIQUE, name TEXT);
CREATE TABLE IF NOT EXISTS teacher_subject (
    teacher_id INTEGER, 
    subject_id INTEGER,
    PRIMARY KEY (teacher_id, subject_id),
    FOREIGN KEY (teacher_id) REFERENCES teacher(id) ON DELETE CASCADE,
    FOREIGN KEY (subject_id) REFERENCES subject(id) ON DELETE CASCADE
);
CREATE TABLE IF NOT EXISTS subject (id INTEGER PRIMARY KEY, cod TEXT UNIQUE, name TEXT, color TEXT DEFAULT '#8AE234', pattern TEXT DEFAULT 'bg-solid');
CREATE TABLE IF NOT EXISTS student_group (
    id INTEGER PRIMARY KEY, 
    cod TEXT UNIQUE, 
    name TEXT, 
    parent_id INTEGER, 
    parent_cod TEXT,
    FOREIGN KEY (parent_id) REFERENCES student_group(id) ON DELETE SET NULL
);
CREATE TABLE IF NOT EXISTS student_group_conflict (
    group_id INTEGER NOT NULL,
    conflicting_group_id INTEGER NOT NULL,
    PRIMARY KEY (group_id, conflicting_group_id),
    FOREIGN KEY (group_id) REFERENCES student_group(id) ON DELETE CASCADE,
    FOREIGN KEY (conflicting_group_id) REFERENCES student_group(id) ON DELETE CASCADE
);
CREATE TABLE IF NOT EXISTS student_group_leaf (
    group_id INTEGER NOT NULL,
    leaf_group_id INTEGER NOT NULL,
    PRIMARY KEY (group_id, leaf_group_id),
    FOREIGN KEY (group_id) REFERENCES student_group(id) ON DELETE CASCADE,
    FOREIGN KEY (leaf_group_id) REFERENCES student_group(id) ON DELETE CASCADE
);
CREATE TABLE IF NOT EXISTS lesson (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    subject_id INTEGER,
    teacher_id INTEGER,
    student_group_id INTEGER,
    room_id INTEGER,
    timeslot_id INTEGER,
    is_pinned BOOLEAN DEFAULT 0,
    parent_id INTEGER,
    FOREIGN KEY (subject_id) REFERENCES subject(id) ON DELETE CASCADE,
    FOREIGN KEY (student_group_id) REFERENCES student_group(id) ON DELETE CASCADE,
    FOREIGN KEY (teacher_id) REFERENCES teacher(id) ON DELETE SET NULL,
    FOREIGN KEY (room_id) REFERENCES room(id) ON DELETE SET NULL,
    FOREIGN KEY (timeslot_id) REFERENCES timeslot(id) ON DELETE SET NULL,
    FOREIGN KEY (parent_id) REFERENCES lesson(id) ON DELETE CASCADE
);
CREATE TABLE IF NOT EXISTS task_meta (
    solver_status  TEXT    DEFAULT 'NEW',
    score          TEXT    DEFAULT '',
    score_detail   TEXT    DEFAULT '',
    started_at     TEXT    DEFAULT '',
    finished_at    TEXT    DEFAULT '',
    error_msg      TEXT    DEFAULT ''
);

INSERT OR IGNORE INTO task_meta (solver_status) SELECT 'NEW' WHERE NOT EXISTS (SELECT 1 FROM task_meta);

CREATE TABLE IF NOT EXISTS score_history (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    task_id TEXT NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    time_millis INTEGER NOT NULL,
    hard_score INTEGER NOT NULL,
    soft_score INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS solver_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    event_type TEXT NOT NULL,
    time_millis INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS conflict_history (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    task_id TEXT NOT NULL,
    time_millis INTEGER NOT NULL,
    rule_name TEXT NOT NULL,
    count INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS rule_configuration (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    rule_name TEXT UNIQUE,
    weight INTEGER,
    label TEXT,
    description TEXT,
    score_type TEXT DEFAULT 'soft'
);

INSERT OR IGNORE INTO rule_configuration (rule_name, weight, label, description, score_type) VALUES
-- Hard Constraints
('roomConflict', NULL, 'Conflicto de Aula', 'Un aula no puede tener asignadas dos clases simultáneas.', 'hard'),
('teacherConflict', NULL, 'Conflicto de Docente', 'Un docente no puede dar dos clases al mismo tiempo.', 'hard'),
('studentGroupConflict', NULL, 'Conflicto de Sección/Grupo', 'Una sección o grupo no puede tener dos clases al mismo tiempo.', 'hard'),
('teacherConsistency', NULL, 'Inconsistencia Docente', 'Todas las sesiones de un mismo curso para un grupo deben ser dictadas por el mismo docente (evita múltiples profesores para la misma materia).', 'hard'),

-- Soft Constraints
('studentGroupSubjectVariety', 10, 'Variedad de Cursos por Sección', 'Evita tener la misma materia seguida en el día para una sección.', 'soft'),
('teacherRoomStability', 6, 'Estabilidad de Aulas para el Docente', 'Intenta que un docente dicte sus clases en la misma aula el mismo día.', 'soft'),
('penalizeWeekends', 4, 'Penalizar fines de semana', 'Evita asignar sesiones los sábados y domingos.', 'soft'),
('penalizeSunday', 0, 'Penalizar solo domingo', 'Evita asignar sesiones únicamente los domingos.', 'soft'),
('teacherTimeEfficiency', 8, 'Eficiencia de Tiempo del Docente', 'Reduce los huecos libres (ventanas) en el horario de los docentes.', 'soft'),
('studentTimeEfficiency', 0, 'Eficiencia de Tiempo del Estudiante', 'Reduce los huecos libres (ventanas) en el horario de los estudiantes.', 'soft');

DROP TABLE IF EXISTS lesson_conflict;
DROP VIEW IF EXISTS v_lesson_conflict;

CREATE VIEW IF NOT EXISTS v_lesson_conflict AS
SELECT 'group' AS type, la.id AS lesson_id, la.student_group_id AS entity_id, count(1) AS conflict_count
FROM lesson la
INNER JOIN student_group_conflict sgc ON sgc.group_id = la.student_group_id
INNER JOIN lesson lb ON lb.student_group_id = sgc.conflicting_group_id
WHERE la.timeslot_id = lb.timeslot_id AND la.id <> lb.id
GROUP BY la.id, la.student_group_id

UNION ALL

SELECT 'room' AS type, a.id AS lesson_id, a.room_id AS entity_id, count(1) AS conflict_count
FROM lesson a
JOIN lesson b ON a.timeslot_id = b.timeslot_id AND a.room_id = b.room_id AND a.id <> b.id
GROUP BY a.id, a.room_id

UNION ALL

SELECT 'teacher' AS type, a.id AS lesson_id, a.teacher_id AS entity_id, count(1) AS conflict_count
FROM lesson a
JOIN lesson b ON a.timeslot_id = b.timeslot_id AND a.teacher_id = b.teacher_id AND a.id <> b.id
GROUP BY a.id, a.teacher_id

UNION ALL

SELECT 'teacherConsistency' AS type, l1.id AS lesson_id, l1.student_group_id AS entity_id, count(1) AS conflict_count
FROM lesson l1
JOIN lesson l2 ON l1.student_group_id = l2.student_group_id AND l1.subject_id = l2.subject_id AND l1.teacher_id != l2.teacher_id AND l1.id <> l2.id
WHERE l1.teacher_id IS NOT NULL AND l2.teacher_id IS NOT NULL
GROUP BY l1.id, l1.student_group_id;

DROP VIEW IF EXISTS v_constraint_violations;

CREATE VIEW IF NOT EXISTS v_constraint_violations AS
-- 1. Room conflict (Hard)
SELECT 'roomConflict' as rule_name, 'hard' as score_type, -1 as weight, 'Room ' || r.name || ' is used for multiple lessons' as description
FROM lesson l1
JOIN lesson l2 ON l1.timeslot_id = l2.timeslot_id AND l1.room_id = l2.room_id AND l1.id < l2.id
JOIN room r ON l1.room_id = r.id
WHERE l1.room_id IS NOT NULL

UNION ALL

-- 2. Teacher conflict (Hard)
SELECT 'teacherConflict' as rule_name, 'hard' as score_type, -1 as weight, 'Teacher ' || t.name || ' has multiple lessons' as description
FROM lesson l1
JOIN lesson l2 ON l1.timeslot_id = l2.timeslot_id AND l1.teacher_id = l2.teacher_id AND l1.id < l2.id
JOIN teacher t ON l1.teacher_id = t.id
WHERE l1.teacher_id IS NOT NULL

UNION ALL

-- 3. Student group hierarchy conflict (Hard)
SELECT 'groupHierarchyConflict' as rule_name, 'hard' as score_type, -1 as weight, 'Groups ' || sg1.name || ' and ' || sg2.name || ' conflict' as description
FROM lesson l1
JOIN lesson l2 ON l1.timeslot_id = l2.timeslot_id AND l1.id < l2.id
JOIN student_group_conflict sgc ON 
    (l1.student_group_id = sgc.group_id AND l2.student_group_id = sgc.conflicting_group_id)
JOIN student_group sg1 ON l1.student_group_id = sg1.id
JOIN student_group sg2 ON l2.student_group_id = sg2.id
WHERE l1.timeslot_id IS NOT NULL

UNION ALL

-- 4. Teacher consistency per subject (Hard)
SELECT 'teacherConsistency' as rule_name, 'hard' as score_type, -1 as weight, 'Group ' || sg.name || ' has multiple teachers for ' || sub.name as description
FROM lesson l1
JOIN lesson l2 ON l1.student_group_id = l2.student_group_id AND l1.subject_id = l2.subject_id AND l1.teacher_id != l2.teacher_id AND l1.id < l2.id
JOIN student_group sg ON l1.student_group_id = sg.id
JOIN subject sub ON l1.subject_id = sub.id
WHERE l1.teacher_id IS NOT NULL AND l2.teacher_id IS NOT NULL

UNION ALL

-- 5. Student group subject variety (Soft)
SELECT 'studentGroupSubjectVariety' as rule_name, 'soft' as score_type, -10 as weight, 'Subject ' || sub.name || ' taught multiple times on ' || ts1.name_of_day || ' for ' || sg.name as description
FROM lesson l1
JOIN lesson l2 ON l1.student_group_id = l2.student_group_id AND l1.subject_id = l2.subject_id AND l1.id < l2.id
JOIN timeslot ts1 ON l1.timeslot_id = ts1.id
JOIN timeslot ts2 ON l2.timeslot_id = ts2.id
JOIN student_group sg ON l1.student_group_id = sg.id
JOIN subject sub ON l1.subject_id = sub.id
WHERE ts1.day_of_week = ts2.day_of_week AND l1.timeslot_id IS NOT NULL AND l2.timeslot_id IS NOT NULL

UNION ALL

-- 6. Student time efficiency - Gaps (Soft)
SELECT 'studentTimeEfficiency' as rule_name, 'soft' as score_type, -1 as weight, 'Gap for leaf group ' || sg.name || ' on ' || gap_query.name_of_day as description
FROM (
    SELECT 
        l.id as curr_lesson_id,
        LAG(l.id) OVER (PARTITION BY sgl.leaf_group_id, ts.day_of_week ORDER BY ts.start_time) as prev_lesson_id,
        sgl.leaf_group_id,
        ts.day_of_week,
        ts.name_of_day,
        ts.id as curr_ts_id,
        LAG(ts.id) OVER (PARTITION BY sgl.leaf_group_id, ts.day_of_week ORDER BY ts.start_time) as prev_ts_id
    FROM lesson l
    JOIN student_group_leaf sgl ON l.student_group_id = sgl.group_id
    JOIN timeslot ts ON l.timeslot_id = ts.id
) as gap_query
JOIN student_group sg ON gap_query.leaf_group_id = sg.id
WHERE gap_query.prev_ts_id IS NOT NULL 
  AND gap_query.curr_ts_id - gap_query.prev_ts_id > 1

UNION ALL

-- 7. Teacher room stability (Soft)
SELECT 'teacherRoomStability' as rule_name, 'soft' as score_type, -1 as weight, 'Teacher ' || t.name || ' uses ' || COUNT(DISTINCT l.room_id) || ' different rooms' as description
FROM lesson l
JOIN teacher t ON l.teacher_id = t.id
WHERE l.room_id IS NOT NULL
GROUP BY t.id, t.name
HAVING COUNT(DISTINCT l.room_id) > 1

UNION ALL

-- 8. Teacher time efficiency (Soft)
SELECT 'teacherTimeEfficiency' as rule_name, 'soft' as score_type, -1 as weight, 'Gap for teacher ' || t.name || ' on ' || gap_query.name_of_day as description
FROM (
    SELECT 
        l.id as curr_lesson_id,
        l.teacher_id,
        ts.day_of_week,
        ts.name_of_day,
        ts.id as curr_ts_id,
        LAG(ts.id) OVER (PARTITION BY l.teacher_id, ts.day_of_week ORDER BY ts.start_time) as prev_ts_id
    FROM lesson l
    JOIN timeslot ts ON l.timeslot_id = ts.id
    WHERE l.teacher_id IS NOT NULL
) as gap_query
JOIN teacher t ON gap_query.teacher_id = t.id
WHERE gap_query.prev_ts_id IS NOT NULL 
  AND gap_query.curr_ts_id - gap_query.prev_ts_id > 1

UNION ALL

-- 9. Penalize weekends (Soft)
SELECT 'penalizeWeekends' as rule_name, 'soft' as score_type, -1 as weight, 'Lesson scheduled on weekend (' || ts.name_of_day || ')' as description
FROM lesson l
JOIN timeslot ts ON l.timeslot_id = ts.id
WHERE ts.day_of_week >= 5

UNION ALL

-- 10. Penalize Sunday (Soft)
SELECT 'penalizeSunday' as rule_name, 'soft' as score_type, -1 as weight, 'Lesson scheduled on Sunday' as description
FROM lesson l
JOIN timeslot ts ON l.timeslot_id = ts.id
WHERE ts.day_of_week = 6;
