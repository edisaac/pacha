package service

import (
	"database/sql"
	"fmt"

	"github.com/xuri/excelize/v2"
)

// ExportToExcel genera un archivo Excel (.xlsx) con el contenido procesado de la base de datos SQLite,
// manteniendo exactamente las mismas columnas que en la subida sin IDs generados.
func ExportToExcel(db *sql.DB) (*excelize.File, error) {
	f := excelize.NewFile()
	defer func() {
		// No cerramos f aquí para permitir que el caller lo escriba al HTTP ResponseWriter.
		// El caller o recolector de basura se encargará de liberar recursos tras Write.
	}()

	// Renombrar la hoja por defecto "Sheet1" a "Aulas"
	f.SetSheetName("Sheet1", "Aulas")

	// Crear estilo en negrita para encabezados
	styleID, err := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true},
	})
	if err != nil {
		styleID = 0
	}

	sheets := []struct {
		Name    string
		Headers []interface{}
		Query   string
	}{
		{
			Name:    "Aulas",
			Headers: []interface{}{"Código", "Nombre"},
			Query:   "SELECT cod, name FROM room ORDER BY id",
		},
		{
			Name:    "Docentes",
			Headers: []interface{}{"Código", "Nombre"},
			Query:   "SELECT cod, name FROM teacher ORDER BY id",
		},
		{
			Name:    "Cursos",
			Headers: []interface{}{"Código", "Nombre"},
			Query:   "SELECT cod, name FROM subject ORDER BY id",
		},
		{
			Name:    "Secciones",
			Headers: []interface{}{"Código", "Nombre", "Código Padre"},
			Query:   "SELECT cod, name, COALESCE(parent_cod, '') FROM student_group ORDER BY id",
		},
		{
			Name:    "Bloques Horarios",
			Headers: []interface{}{"Código", "Día de la semana", "Nombre del día", "Hora Inicio", "Hora Fin"},
			Query:   "SELECT cod, day_of_week, name_of_day, start_time, end_time FROM timeslot ORDER BY id",
		},
		{
			Name:    "Curso-Docente",
			Headers: []interface{}{"Código Docente", "Nombre Docente", "Código Curso", "Nombre Curso"},
			Query:   "SELECT t.cod, t.name, s.cod, s.name FROM teacher_subject ts JOIN teacher t ON ts.teacher_id = t.id JOIN subject s ON ts.subject_id = s.id ORDER BY t.id, s.id",
		},
		{
			Name:    "Sesiones",
			Headers: []interface{}{"Código Curso", "Nombre Curso", "Código Sección", "Nombre Sección", "Código Docente", "Nombre Docente", "Código Aula", "Nombre Aula", "Código Bloque", "Descripción Bloque", "Duración", "Fija"},
			Query: `SELECT 
				s.cod, s.name,
				g.cod, g.name,
				COALESCE(t.cod, ''), COALESCE(t.name, ''),
				COALESCE(r.cod, ''), COALESCE(r.name, ''),
				COALESCE(ts.cod, ''),
				CASE WHEN ts.id IS NOT NULL THEN ts.name_of_day || ' ' || ts.start_time || '-' || ts.end_time ELSE '' END,
				1 + (SELECT COUNT(*) FROM lesson h WHERE h.parent_id = l.id),
				CASE WHEN l.is_pinned THEN 'Sí' ELSE 'No' END
			FROM lesson l
			JOIN subject s ON l.subject_id = s.id
			JOIN student_group g ON l.student_group_id = g.id
			LEFT JOIN teacher t ON l.teacher_id = t.id
			LEFT JOIN room r ON l.room_id = r.id
			LEFT JOIN timeslot ts ON l.timeslot_id = ts.id
			WHERE l.parent_id IS NULL
			ORDER BY l.id`,
		},
	}

	for i, sheet := range sheets {
		if i > 0 {
			if _, err := f.NewSheet(sheet.Name); err != nil {
				return nil, fmt.Errorf("error creando hoja %s: %w", sheet.Name, err)
			}
		}

		if err := writeQueryToSheet(f, db, sheet.Name, sheet.Query, sheet.Headers, styleID); err != nil {
			return nil, fmt.Errorf("error poblando hoja %s: %w", sheet.Name, err)
		}
	}

	// Seleccionar la primera hoja como activa
	f.SetActiveSheet(0)
	return f, nil
}

func writeQueryToSheet(f *excelize.File, db *sql.DB, sheetName string, query string, headers []interface{}, styleID int) error {
	rows, err := db.Query(query)
	if err != nil {
		return err
	}
	defer rows.Close()

	if err := f.SetSheetRow(sheetName, "A1", &headers); err != nil {
		return err
	}
	if styleID > 0 {
		_ = f.SetRowStyle(sheetName, 1, 1, styleID)
	}

	cols, err := rows.Columns()
	if err != nil {
		return err
	}

	rowIdx := 2
	for rows.Next() {
		values := make([]interface{}, len(cols))
		valuePtrs := make([]interface{}, len(cols))
		for i := range values {
			valuePtrs[i] = &values[i]
		}

		if err := rows.Scan(valuePtrs...); err != nil {
			return err
		}

		rowVals := make([]interface{}, len(cols))
		for i, val := range values {
			if val == nil {
				rowVals[i] = ""
			} else {
				switch v := val.(type) {
				case []byte:
					rowVals[i] = string(v)
				default:
					rowVals[i] = fmt.Sprintf("%v", v)
				}
			}
		}

		cell, err := excelize.CoordinatesToCellName(1, rowIdx)
		if err != nil {
			return err
		}
		if err := f.SetSheetRow(sheetName, cell, &rowVals); err != nil {
			return err
		}
		rowIdx++
	}
	return rows.Err()
}
