package pbexcel

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/dbutils"
	"github.com/xuri/excelize/v2"
)

var systemFieldNames = map[string]bool{
	"id": true, "created": true, "updated": true,
	"collectionId": true, "collectionName": true, "expand": true,
}

var skipFieldTypes = map[string]bool{
	"file": true, "password": true,
}

func resolveExcelPath(fileName string) string {
	// Sanitize path to prevent directory traversal
	fileName = filepath.Clean(fileName)
	if strings.HasPrefix(fileName, "..") || strings.Contains(fileName, ".."+string(filepath.Separator)) {
		fileName = filepath.Base(fileName)
	}
	if !strings.Contains(fileName, "/") && !strings.Contains(fileName, "\\") {
		fileName = filepath.Join("pb_data", fileName)
	}
	if !strings.HasSuffix(fileName, ".xlsx") {
		fileName += ".xlsx"
	}
	// Ensure the resolved path is still under pb_data
	absPath, err := filepath.Abs(fileName)
	if err == nil {
		pbDataAbs, _ := filepath.Abs("pb_data")
		if !strings.HasPrefix(absPath, pbDataAbs+string(filepath.Separator)) && absPath != pbDataAbs {
			fileName = filepath.Join("pb_data", filepath.Base(fileName))
		}
	}
	return fileName
}

func defaultSheetName(sheetName string) string {
	if sheetName == "" {
		return "Sheet1"
	}
	return sheetName
}

const sampleRowLimit = 50

// DetectedColumn describes a detected column from an Excel sheet: its header
// name and the PocketBase field type inferred from the sample values.
type DetectedColumn struct {
	Name   string
	Type   string
	Values []string
}

// IntrospectSheet reads an Excel sheet and returns the detected columns with
// inferred PocketBase field types (text/number/bool/date) based on the header
// row and a limited number of sample data rows.
func IntrospectSheet(fileName, sheetName string) ([]DetectedColumn, error) {
	path := resolveExcelPath(fileName)

	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, fmt.Errorf("pbexcel: file %q not found", path)
	}

	sheetName = defaultSheetName(sheetName)

	f, err := excelize.OpenFile(path)
	if err != nil {
		return nil, fmt.Errorf("pbexcel: failed to open file %s: %w", path, err)
	}
	defer f.Close()

	if idx, _ := f.GetSheetIndex(sheetName); idx < 0 {
		return nil, fmt.Errorf("pbexcel: sheet %q not found in file %s", sheetName, path)
	}

	rows, err := f.GetRows(sheetName)
	if err != nil {
		return nil, fmt.Errorf("pbexcel: failed to read rows from sheet %q: %w", sheetName, err)
	}
	if len(rows) < 1 {
		return nil, fmt.Errorf("pbexcel: sheet %q has no header row", sheetName)
	}

	headers := rows[0]
	dataRows := rows[1:]
	if len(dataRows) > sampleRowLimit {
		dataRows = dataRows[:sampleRowLimit]
	}

	result := make([]DetectedColumn, 0, len(headers))
	seen := map[string]bool{}
	for ci, h := range headers {
		h = strings.TrimSpace(h)
		// skip empty or duplicate header names
		if h == "" || seen[h] {
			continue
		}
		seen[h] = true

		col := DetectedColumn{Name: h}
		for _, row := range dataRows {
			if ci < len(row) {
				v := strings.TrimSpace(row[ci])
				if v != "" {
					col.Values = append(col.Values, v)
				}
			}
		}
		col.Type = inferColumnType(col.Values)
		result = append(result, col)
	}

	return result, nil
}

// inferColumnType guesses the PocketBase field type from sample string values.
func inferColumnType(values []string) string {
	if len(values) == 0 {
		return "text"
	}

	allNums := true
	allBools := true
	allDates := true

	for _, v := range values {
		if _, err := strconv.ParseFloat(v, 64); err != nil {
			allNums = false
		}
		switch strings.ToLower(v) {
		case "true", "false", "1", "0", "yes", "no", "y", "n", "on", "off":
		default:
			allBools = false
		}
		if !isDateValue(v) {
			allDates = false
		}
	}

	switch {
	case allNums:
		return "number"
	case allBools:
		return "bool"
	case allDates:
		return "date"
	default:
		return "text"
	}
}

// isDateValue reports whether v parses as a date in a supported layout.
func isDateValue(v string) bool {
	for _, layout := range []string{
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05Z",
		"2006-01-02T15:04:05",
		"2006-01-02",
		"02.01.2006",
		"1/2/2006",
		"2006/01/02",
		"02.01.2006 15:04",
		"1/2/2006 15:04",
		time.RFC3339,
	} {
		if _, err := time.Parse(layout, v); err == nil {
			return true
		}
	}
	return false
}

func colLetter(n int) string {
	s := ""
	for n >= 0 {
		s = string(rune('A'+n%26)) + s
		n = n/26 - 1
	}
	return s
}

func cellRef(col, row int) string {
	return fmt.Sprintf("%s%d", colLetter(col), row)
}

func writeLogSheet(f *excelize.File, lines []string) {
	const logSheetName = "Log"
	if idx, _ := f.GetSheetIndex(logSheetName); idx > 0 {
		f.DeleteSheet(logSheetName)
	}
	f.NewSheet(logSheetName)
	for i, line := range lines {
		f.SetCellValue(logSheetName, cellRef(0, i+1), line)
	}
}

func ExportToExcel(app core.App, excelFileName, sheetName, collectionName string) error {
	path := resolveExcelPath(excelFileName)

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, os.ModePerm); err != nil {
		return fmt.Errorf("pbexcel: failed to create directory %s: %w", dir, err)
	}

	sheetName = defaultSheetName(sheetName)

	collection, err := app.FindCachedCollectionByNameOrId(collectionName)
	if err != nil {
		return fmt.Errorf("pbexcel: collection %q not found: %w", collectionName, err)
	}

	records, err := app.FindAllRecords(collectionName)
	if err != nil {
		return fmt.Errorf("pbexcel: failed to fetch records from %q: %w", collectionName, err)
	}

	var exportFields []core.Field
	for _, f := range collection.Fields {
		if systemFieldNames[f.GetName()] {
			continue
		}
		if skipFieldTypes[f.Type()] {
			continue
		}
		exportFields = append(exportFields, f)
	}

	f := excelize.NewFile()

	if idx, _ := f.GetSheetIndex(sheetName); idx > 0 {
		f.DeleteSheet(sheetName)
	}
	f.NewSheet(sheetName)
	if idx, _ := f.GetSheetIndex("Sheet1"); idx > 0 {
		f.DeleteSheet("Sheet1")
	}

	for ci, field := range exportFields {
		f.SetCellValue(sheetName, cellRef(ci, 1), field.GetName())
	}

	for ri, rec := range records {
		row := ri + 2
		for ci, field := range exportFields {
			f.SetCellValue(sheetName, cellRef(ci, row), rec.GetString(field.GetName()))
		}
	}

	writeLogSheet(f, []string{
		fmt.Sprintf("Export date: %s", time.Now().Format("2006-01-02 15:04:05")),
		fmt.Sprintf("%d lines exported from collection %s", len(records), collectionName),
	})

	if err := f.SaveAs(path); err != nil {
		return fmt.Errorf("pbexcel: failed to save file %s: %w", path, err)
	}

	return nil
}

// ImportFromExcel imports records from an Excel sheet into a PocketBase
// collection. When headerFieldMap is non-nil it maps each sheet header to the
// target PocketBase field name (headers not present in the map are skipped);
// otherwise the header is used as the field name directly.
func ImportFromExcel(app core.App, excelFileName, sheetName, collectionName, mode string, headerFieldMap ...map[string]string) error {
	path := resolveExcelPath(excelFileName)

	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("pbexcel: file %q not found", path)
	}

	sheetName = defaultSheetName(sheetName)

	f, err := excelize.OpenFile(path)
	if err != nil {
		return fmt.Errorf("pbexcel: failed to open file %s: %w", path, err)
	}
	defer f.Close()

	if idx, _ := f.GetSheetIndex(sheetName); idx < 0 {
		return fmt.Errorf("pbexcel: sheet %q not found in file %s", sheetName, path)
	}

	collection, err := app.FindCachedCollectionByNameOrId(collectionName)
	if err != nil {
		return fmt.Errorf("pbexcel: collection %q not found: %w", collectionName, err)
	}

	rows, err := f.GetRows(sheetName)
	if err != nil {
		return fmt.Errorf("pbexcel: failed to read rows from sheet %q: %w", sheetName, err)
	}
	if len(rows) < 1 {
		return fmt.Errorf("pbexcel: sheet %q has no header row", sheetName)
	}

	headers := rows[0]
	dataRows := rows[1:]

	fieldMap := map[string]core.Field{}
	for _, field := range collection.Fields {
		if skipFieldTypes[field.Type()] {
			continue
		}
		fieldMap[field.GetName()] = field
	}

	var colFields []core.Field
	var colNames []string
	var mapping map[string]string
	if len(headerFieldMap) > 0 {
		mapping = headerFieldMap[0]
	}
	for _, h := range headers {
		h = strings.TrimSpace(h)
		fieldName := h
		if mapping != nil {
			if mapped, ok := mapping[h]; ok {
				fieldName = mapped
			} else {
				continue
			}
		}
		if f, ok := fieldMap[fieldName]; ok {
			colFields = append(colFields, f)
			colNames = append(colNames, fieldName)
		}
	}

	uniqueFieldNames := map[string]bool{}
	for _, idxExpr := range collection.Indexes {
		parsed := dbutils.ParseIndex(idxExpr)
		if parsed.Unique && len(parsed.Columns) == 1 {
			name := parsed.Columns[0].Name
			if _, ok := fieldMap[name]; ok && !systemFieldNames[name] {
				uniqueFieldNames[name] = true
			}
		}
	}

	if mode == "replace" {
		allRecs, err := app.FindAllRecords(collectionName)
		if err != nil {
			return fmt.Errorf("pbexcel: failed to fetch existing records: %w", err)
		}
		for _, rec := range allRecs {
			if err := app.Delete(rec); err != nil {
				return fmt.Errorf("pbexcel: failed to delete record %s: %w", rec.Id, err)
			}
		}
	}

	systemFieldNames["id"] = true

	imported := 0
	skipped := 0
	updated := 0

	for _, row := range dataRows {
		record := core.NewRecord(collection)

		var matchFieldName string
		var matchRawValue string

		for ci, val := range row {
			if ci >= len(colFields) {
				continue
			}
			fName := colNames[ci]
			field := colFields[ci]
			val = strings.TrimSpace(val)

			if systemFieldNames[fName] {
				continue
			}

			converted := convertValue(field.Type(), val)

			record.Set(fName, converted)

			if uniqueFieldNames[fName] && val != "" {
				matchFieldName = fName
				matchRawValue = val
			}
		}

		if matchFieldName != "" {
			filterParam := convertValue(colFields[indexOf(colNames, matchFieldName)].Type(), matchRawValue)
			existing, findErr := app.FindRecordsByFilter(
				collectionName,
				fmt.Sprintf("%s = {:val}", matchFieldName),
				"", 1, 0, nil,
				map[string]any{"val": filterParam},
			)
			if findErr == nil && len(existing) > 0 {
				switch mode {
				case "insert":
					skipped++
					continue
				default:
					for ci, val := range row {
						if ci >= len(colFields) {
							continue
						}
						fName := colNames[ci]
						if systemFieldNames[fName] {
							continue
						}
						field := colFields[ci]
						val = strings.TrimSpace(val)
						converted := convertValue(field.Type(), val)
						existing[0].Set(fName, converted)
					}
					if err := app.Save(existing[0]); err != nil {
						skipped++
						continue
					}
					updated++
					imported++
					continue
				}
			}
		}

		if mode == "update" {
			skipped++
			continue
		}

		if err := app.Save(record); err != nil {
			skipped++
			continue
		}
		imported++
	}

	logLines := []string{
		fmt.Sprintf("Import date: %s", time.Now().Format("2006-01-02 15:04:05")),
		fmt.Sprintf("File: %s", path),
		fmt.Sprintf("Sheet: %s", sheetName),
		fmt.Sprintf("Collection: %s", collectionName),
		fmt.Sprintf("Total rows in sheet: %d", len(dataRows)),
		fmt.Sprintf("Imported: %d", imported),
	}
	if updated > 0 {
		logLines = append(logLines, fmt.Sprintf("Updated: %d", updated))
	}
	if skipped > 0 {
		logLines = append(logLines, fmt.Sprintf("Skipped: %d", skipped))
	}

	writeLogSheet(f, logLines)

	if err := f.Save(); err != nil {
		return fmt.Errorf("pbexcel: failed to save file %s: %w", path, err)
	}

	return nil
}

func convertValue(fieldType, val string) any {
	if val == "" {
		return nil
	}
	switch fieldType {
	case "number":
		if v, err := strconv.ParseFloat(val, 64); err == nil {
			return v
		}
		return nil
	case "bool":
		switch strings.ToLower(val) {
		case "true", "1", "yes", "y", "on":
			return true
		default:
			return false
		}
	case "date":
		for _, layout := range []string{
			"2006-01-02 15:04:05",
			"2006-01-02T15:04:05Z",
			"2006-01-02T15:04:05",
			"2006-01-02",
			"02.01.2006",
			"1/2/2006",
			"2006/01/02",
		} {
			if t, err := time.Parse(layout, val); err == nil {
				return t.Format("2006-01-02 15:04:05")
			}
		}
		return val
	default:
		return val
	}
}

func indexOf(slice []string, val string) int {
	for i, s := range slice {
		if s == val {
			return i
		}
	}
	return -1
}
