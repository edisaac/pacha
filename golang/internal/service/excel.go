package service

import (
	"database/sql"
	"fmt"
	"log"
	"strconv"
	"strings"

	"github.com/xuri/excelize/v2"
)

type GroupNode struct {
	ID        int
	Cod       string
	Name      string
	ParentCod string
	ParentID  sql.NullInt64
}

func GenerateData(db *sql.DB, f *excelize.File) error {
	rooms, _ := f.GetRows("Aulas")
	for i, row := range rooms {
		if i == 0 || len(row) < 2 {
			continue
		}
		db.Exec("INSERT INTO room (id, cod, name) VALUES (?, ?, ?)", i, row[0], row[1])
	}

	teachers, _ := f.GetRows("Docentes")
	for i, row := range teachers {
		if i == 0 || len(row) < 2 {
			continue
		}
		db.Exec("INSERT INTO teacher (id, cod, name) VALUES (?, ?, ?)", i, row[0], row[1])
	}

	courses, _ := f.GetRows("Cursos")
	cp := NewColorPicker()
	for i, row := range courses {
		if i == 0 || len(row) < 2 {
			continue
		}
		color, pattern := cp.NextColor()
		db.Exec("INSERT INTO subject (id, cod, name, color, pattern) VALUES (?, ?, ?, ?, ?)", i, row[0], row[1], color, pattern)
	}

	groups, _ := f.GetRows("Secciones")
	CargarGrupos(db, groups)

	intervals, _ := f.GetRows("Bloques Horarios")
	for i, row := range intervals {
		if i == 0 || len(row) < 5 {
			continue
		}
		db.Exec("INSERT INTO timeslot (id, cod, day_of_week, name_of_day, start_time, end_time) VALUES (?, ?, ?, ?, ?, ?)",
			i, row[0], row[1], row[2], row[3], row[4])
	}

	teacherSubjects, _ := f.GetRows("Curso-Docente")
	for i, row := range teacherSubjects {
		if i == 0 || len(row) < 3 {
			continue
		}
		var tId, sId int
		db.QueryRow("SELECT id FROM teacher WHERE cod = ?", row[0]).Scan(&tId)
		db.QueryRow("SELECT id FROM subject WHERE cod = ?", row[2]).Scan(&sId)
		db.Exec("INSERT INTO teacher_subject (teacher_id, subject_id) VALUES (?, ?)", tId, sId)
	}

	lessons, _ := f.GetRows("Sesiones")
	for i, row := range lessons {
		if i == 0 || len(row) < 4 {
			continue
		}
		courseCode := strings.TrimSpace(row[0])
		groupCode := strings.TrimSpace(row[2])
		teacherCode := ""
		if len(row) > 4 {
			teacherCode = strings.TrimSpace(row[4])
		}
		roomCode := ""
		if len(row) > 6 {
			roomCode = strings.TrimSpace(row[6])
		}
		timeslotCode := ""
		if len(row) > 8 {
			timeslotCode = strings.TrimSpace(row[8])
		}

		var subjectId, groupId int
		var teacherId, roomId, timeslotId sql.NullInt64
		db.QueryRow("SELECT id FROM subject WHERE cod = ?", courseCode).Scan(&subjectId)
		db.QueryRow("SELECT id FROM student_group WHERE cod = ?", groupCode).Scan(&groupId)

		if teacherCode != "" {
			err := db.QueryRow("SELECT id FROM teacher WHERE cod = ?", teacherCode).Scan(&teacherId)
			if err != nil {
				log.Printf("Error encontrando teacher con cod %s: %v", teacherCode, err)
			}
		}
		if roomCode != "" {
			err := db.QueryRow("SELECT id FROM room WHERE cod = ?", roomCode).Scan(&roomId)
			if err != nil {
				log.Printf("Error encontrando room con cod %s: %v", roomCode, err)
			}
		}
		if timeslotCode != "" {
			err := db.QueryRow("SELECT id FROM timeslot WHERE cod = ?", timeslotCode).Scan(&timeslotId)
			if err != nil {
				log.Printf("Error encontrando timeslot con cod %s: %v", timeslotCode, err)
			}
		}

		duration := 1
		if len(row) > 10 {
			durationStr := strings.TrimSpace(row[10])
			if d, err := strconv.Atoi(durationStr); err == nil && d > 0 {
				duration = d
			}
		}

		isPinned := false
		if len(row) > 11 {
			isPinnedStr := strings.TrimSpace(strings.ToLower(row[11]))
			if isPinnedStr == "sí" || isPinnedStr == "si" || isPinnedStr == "true" || isPinnedStr == "1" {
				isPinned = true
			}
		}

		res, err := db.Exec("INSERT INTO lesson (subject_id, teacher_id, student_group_id, room_id, timeslot_id, is_pinned) VALUES (?, ?, ?, ?, ?, ?)",
			subjectId, teacherId, groupId, roomId, timeslotId, isPinned)
		
		if err == nil && duration > 1 {
			parentId, _ := res.LastInsertId()
			
			var nextTimeslotIds []int64
			if timeslotId.Valid {
				rowsTS, err := db.Query(`
					SELECT id FROM timeslot 
					WHERE day_of_week = (SELECT day_of_week FROM timeslot WHERE id = ?) 
					  AND start_time > (SELECT start_time FROM timeslot WHERE id = ?)
					ORDER BY start_time ASC LIMIT ?`, timeslotId.Int64, timeslotId.Int64, duration-1)
				if err == nil {
					for rowsTS.Next() {
						var ntid int64
						if err := rowsTS.Scan(&ntid); err == nil {
							nextTimeslotIds = append(nextTimeslotIds, ntid)
						}
					}
					rowsTS.Close()
				}
			}

			for j := 1; j < duration; j++ {
				var childTimeslotId sql.NullInt64
				if j-1 < len(nextTimeslotIds) {
					childTimeslotId = sql.NullInt64{Int64: nextTimeslotIds[j-1], Valid: true}
				}
				db.Exec("INSERT INTO lesson (subject_id, teacher_id, student_group_id, room_id, timeslot_id, is_pinned, parent_id) VALUES (?, ?, ?, ?, ?, ?, ?)",
					subjectId, teacherId, groupId, roomId, childTimeslotId, isPinned, parentId)
			}
		}
	}



	return nil
}

func CargarGrupos(db *sql.DB, rows [][]string) error {
	// -------------------------------------------------------------
	// FASE 1: Leer el Excel completo a memoria
	// -------------------------------------------------------------
	nodesByCod := make(map[string]*GroupNode)
	var allNodes []*GroupNode

	for i, row := range rows {
		if i == 0 || len(row) < 2 {
			continue // Omitir cabecera o filas incompletas
		}

		cod := strings.TrimSpace(row[0])
		name := strings.TrimSpace(row[1])
		pCode := ""
		if len(row) > 2 {
			pCode = strings.TrimSpace(row[2])
		}

		node := &GroupNode{
			ID:        i, // ID basado en el índice
			Cod:       cod,
			Name:      name,
			ParentCod: pCode,
		}

		nodesByCod[cod] = node
		allNodes = append(allNodes, node)
	}

	// -------------------------------------------------------------
	// FASE 2: Enlazar los ParentID y crear el árbol de dependencias
	// -------------------------------------------------------------
	childrenByParentID := make(map[int][]int)

	for _, node := range allNodes {
		if node.ParentCod != "" {
			if parentNode, exists := nodesByCod[node.ParentCod]; exists {
				node.ParentID = sql.NullInt64{Int64: int64(parentNode.ID), Valid: true}
				childrenByParentID[parentNode.ID] = append(childrenByParentID[parentNode.ID], node.ID)
			}
		}
	}

	// -------------------------------------------------------------
	// FASE 3: Ordenar los nodos (Padres PRIMERO -> Hijos DESPUÉS)
	// -------------------------------------------------------------
	var orderedNodes []*GroupNode
	visited := make(map[int]bool)

	// Función recursiva para agregar un nodo y luego a sus descendientes
	var addNodeAndChildren func(node *GroupNode)
	addNodeAndChildren = func(node *GroupNode) {
		if visited[node.ID] {
			return
		}
		visited[node.ID] = true
		orderedNodes = append(orderedNodes, node)

		// Agregar a los hijos inmediatamente después del padre
		for _, childID := range childrenByParentID[node.ID] {
			for _, n := range allNodes {
				if n.ID == childID {
					addNodeAndChildren(n)
					break
				}
			}
		}
	}

	// Primero procesamos los nodos Raíz (los que no tienen padre)
	for _, node := range allNodes {
		if !node.ParentID.Valid {
			addNodeAndChildren(node)
		}
	}

	// -------------------------------------------------------------
	// FASE 4: Calcular Matriz de Conflictos (Ancestros + Descendientes)
	// -------------------------------------------------------------
	nodesByID := make(map[int]*GroupNode)
	for _, n := range orderedNodes {
		nodesByID[n.ID] = n
	}

	var getDescendants func(id int, set map[int]bool)
	getDescendants = func(id int, set map[int]bool) {
		for _, childID := range childrenByParentID[id] {
			set[childID] = true
			getDescendants(childID, set)
		}
	}

	conflicts := make(map[int]map[int]bool)
	for _, node := range orderedNodes {
		set := make(map[int]bool)
		set[node.ID] = true // Mismo grupo

		// Ancestros
		currParent := node.ParentID
		for currParent.Valid {
			parentID := int(currParent.Int64)
			set[parentID] = true
			if parentNode, exists := nodesByID[parentID]; exists {
				currParent = parentNode.ParentID
			} else {
				break
			}
		}

		// Descendientes
		getDescendants(node.ID, set)
		conflicts[node.ID] = set
	}

	// -------------------------------------------------------------
	// FASE 4.5: Calcular Hojas (Leafs) para cada grupo
	// -------------------------------------------------------------
	var getLeafs func(id int) []int
	getLeafs = func(id int) []int {
		children := childrenByParentID[id]
		if len(children) == 0 {
			return []int{id}
		}
		var leafs []int
		for _, childID := range children {
			leafs = append(leafs, getLeafs(childID)...)
		}
		return leafs
	}

	leafsByNode := make(map[int][]int)
	for _, node := range orderedNodes {
		leafsByNode[node.ID] = getLeafs(node.ID)
	}

	// -------------------------------------------------------------
	// FASE 5: Insertar en SQLite dentro de una sola Transacción
	// -------------------------------------------------------------
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// 1. Limpiar tablas en orden inverso respetando Foreign Keys
	if _, err := tx.Exec("DELETE FROM student_group_leaf"); err != nil {
		return err
	}
	if _, err := tx.Exec("DELETE FROM student_group_conflict"); err != nil {
		return err
	}
	if _, err := tx.Exec("DELETE FROM student_group"); err != nil {
		return err
	}

	// 2. Insertar 'student_group' en el ORDEN CORRECTO (Padres primero)
	stmtGroup, err := tx.Prepare("INSERT INTO student_group (id, cod, name, parent_id, parent_cod) VALUES (?, ?, ?, ?, ?)")
	if err != nil {
		return err
	}
	defer stmtGroup.Close()

	for _, node := range orderedNodes {
		_, err := stmtGroup.Exec(node.ID, node.Cod, node.Name, node.ParentID, node.ParentCod)
		if err != nil {
			return fmt.Errorf("error insertando grupo %s: %w", node.Cod, err)
		}
	}

	// 3. Insertar 'student_group_conflict'
	stmtConflict, err := tx.Prepare("INSERT INTO student_group_conflict (group_id, conflicting_group_id) VALUES (?, ?)")
	if err != nil {
		return err
	}
	defer stmtConflict.Close()

	for groupID, set := range conflicts {
		for confID := range set {
			_, err := stmtConflict.Exec(groupID, confID)
			if err != nil {
				return fmt.Errorf("error en conflicto %d -> %d: %w", groupID, confID, err)
			}
		}
	}

	// 4. Insertar 'student_group_leaf'
	stmtLeaf, err := tx.Prepare("INSERT INTO student_group_leaf (group_id, leaf_group_id) VALUES (?, ?)")
	if err != nil {
		return err
	}
	defer stmtLeaf.Close()

	for groupID, leafs := range leafsByNode {
		for _, leafID := range leafs {
			_, err := stmtLeaf.Exec(groupID, leafID)
			if err != nil {
				return fmt.Errorf("error en leaf %d -> %d: %w", groupID, leafID, err)
			}
		}
	}

	return tx.Commit()
}
