package pbmssql

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	_ "github.com/microsoft/go-mssqldb"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/dbutils"
)

var (
	dbPool   = map[string]*sql.DB{}
	dbMutex  sync.RWMutex
	poolSize = 5
)

func getPool(dsn string) (*sql.DB, error) {
	dbMutex.RLock()
	if db, ok := dbPool[dsn]; ok {
		dbMutex.RUnlock()
		return db, nil
	}
	dbMutex.RUnlock()

	dbMutex.Lock()
	defer dbMutex.Unlock()

	if db, ok := dbPool[dsn]; ok {
		return db, nil
	}

	db, err := sql.Open("sqlserver", dsn)
	if err != nil {
		return nil, fmt.Errorf("pbmssql: failed to open connection: %w", err)
	}

	db.SetMaxOpenConns(poolSize)
	db.SetMaxIdleConns(poolSize)
	db.SetConnMaxLifetime(5 * time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("pbmssql: failed to ping: %w", err)
	}

	dbPool[dsn] = db
	return db, nil
}

type ColumnInfo struct {
	Name         string
	DataType     string
	IsNull       bool
	OrdinalPos   int
	DefaultValue string
}

func IntrospectTable(dsn, table string) ([]ColumnInfo, error) {
	db, err := getPool(dsn)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	query := `SELECT 
		c.COLUMN_NAME, 
		c.DATA_TYPE, 
		c.IS_NULLABLE, 
		c.ORDINAL_POSITION,
		ISNULL(c.COLUMN_DEFAULT, '')
	FROM INFORMATION_SCHEMA.COLUMNS c
	WHERE c.TABLE_NAME = @p1
	ORDER BY c.ORDINAL_POSITION`

	rows, err := db.QueryContext(ctx, query, table)
	if err != nil {
		return nil, fmt.Errorf("pbmssql: introspect query failed: %w", err)
	}
	defer rows.Close()

	var columns []ColumnInfo
	for rows.Next() {
		var col ColumnInfo
		var isNull string
		if err := rows.Scan(&col.Name, &col.DataType, &isNull, &col.OrdinalPos, &col.DefaultValue); err != nil {
			return nil, fmt.Errorf("pbmssql: failed to scan column: %w", err)
		}
		col.IsNull = strings.EqualFold(isNull, "YES")
		columns = append(columns, col)
	}

	return columns, nil
}

var (
	systemFieldNames = map[string]bool{
		"id": true, "created": true, "updated": true,
		"collectionId": true, "collectionName": true, "expand": true,
	}

	skipFieldTypes = map[string]bool{
		"file": true, "password": true,
	}
)

func ExportToMSSQL(app core.App, collName string, dsn, table, mode string, mapping []struct {
	PBField string `json:"pbField"`
	DBField string `json:"dbField"`
}) error {
	db, err := getPool(dsn)
	if err != nil {
		return err
	}

	collection, err := app.FindCachedCollectionByNameOrId(collName)
	if err != nil {
		return fmt.Errorf("pbmssql: collection %q not found: %w", collName, err)
	}

	records, err := app.FindAllRecords(collName)
	if err != nil {
		return fmt.Errorf("pbmssql: failed to fetch records from %q: %w", collName, err)
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

	if len(mapping) == 0 {
		for _, f := range exportFields {
			mapping = append(mapping, struct {
				PBField string `json:"pbField"`
				DBField string `json:"dbField"`
			}{PBField: f.GetName(), DBField: f.GetName()})
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if mode == "replace" {
		deleteQuery := fmt.Sprintf("DELETE FROM [%s]", table)
		if _, err := db.ExecContext(ctx, deleteQuery); err != nil {
			return fmt.Errorf("pbmssql: failed to delete from %s: %w", table, err)
		}
	}

	for _, rec := range records {
		var colNames []string
		var placeholders []string
		var values []any

		for _, m := range mapping {
			colNames = append(colNames, fmt.Sprintf("[%s]", m.DBField))
			placeholders = append(placeholders, "@p"+fmt.Sprintf("%d", len(values)+1))
			values = append(values, rec.Get(m.PBField))
		}

		var query string
		if mode == "update" {
			var setClauses []string
			for i, m := range mapping {
				setClauses = append(setClauses, fmt.Sprintf("[%s] = @p%d", m.DBField, i+1))
			}
			query = fmt.Sprintf("UPDATE [%s] SET %s WHERE [id] = @p%d", table, strings.Join(setClauses, ", "), len(values))
			values = append(values, rec.Id)
		} else {
			query = fmt.Sprintf("INSERT INTO [%s] (%s) VALUES (%s)", table, strings.Join(colNames, ", "), strings.Join(placeholders, ", "))
		}

		if _, err := db.ExecContext(ctx, query, values...); err != nil {
			log.Printf("pbmssql: failed to insert/update record %s: %v", rec.Id, err)
			continue
		}
	}

	return nil
}

func ImportFromMSSQL(app core.App, collName string, dsn, table, mode string, mapping []struct {
	PBField string `json:"pbField"`
	DBField string `json:"dbField"`
}) error {
	db, err := getPool(dsn)
	if err != nil {
		return err
	}

	collection, err := app.FindCachedCollectionByNameOrId(collName)
	if err != nil {
		return fmt.Errorf("pbmssql: collection %q not found: %w", collName, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	query := fmt.Sprintf("SELECT * FROM [%s]", table)
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return fmt.Errorf("pbmssql: failed to query %s: %w", table, err)
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return fmt.Errorf("pbmssql: failed to get columns: %w", err)
	}

	colIndexMap := make(map[string]int)
	for i, col := range columns {
		colIndexMap[strings.ToLower(col)] = i
	}

	if len(mapping) == 0 {
		for _, f := range collection.Fields {
			if systemFieldNames[f.GetName()] || skipFieldTypes[f.Type()] {
				continue
			}
			mapping = append(mapping, struct {
				PBField string `json:"pbField"`
				DBField string `json:"dbField"`
			}{PBField: f.GetName(), DBField: f.GetName()})
		}
	}

	fieldMap := make(map[string]core.Field)
	for _, f := range collection.Fields {
		fieldMap[f.GetName()] = f
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
		allRecs, err := app.FindAllRecords(collName)
		if err != nil {
			return fmt.Errorf("pbmssql: failed to fetch existing records: %w", err)
		}
		for _, rec := range allRecs {
			if err := app.Delete(rec); err != nil {
				return fmt.Errorf("pbmssql: failed to delete record %s: %w", rec.Id, err)
			}
		}
	}

	imported := 0
	skipped := 0
	updated := 0

	for rows.Next() {
		values := make([]any, len(columns))
		valuePtrs := make([]any, len(columns))
		for i := range values {
			valuePtrs[i] = &values[i]
		}

		if err := rows.Scan(valuePtrs...); err != nil {
			log.Printf("pbmssql: failed to scan row: %v", err)
			continue
		}

		record := core.NewRecord(collection)

		var matchField core.Field
		var matchedVal any

		for _, m := range mapping {
			idx, ok := colIndexMap[strings.ToLower(m.DBField)]
			if !ok {
				continue
			}

			val := values[idx]
			if val == nil {
				continue
			}

			field, ok := fieldMap[m.PBField]
			if !ok {
				continue
			}

			converted := convertValue(field.Type(), val)
			record.Set(m.PBField, converted)

			if uniqueFieldNames[m.PBField] && matchField == nil {
				matchField = field
				matchedVal = val
			}
		}

		if matchField != nil {
			matchParam := convertValue(matchField.Type(), matchedVal)
			existing, findErr := app.FindRecordsByFilter(
				collName,
				fmt.Sprintf("%s = {:val}", matchField.GetName()),
				"", 1, 0, nil,
				map[string]any{"val": matchParam},
			)
			if findErr == nil && len(existing) > 0 {
				switch mode {
				case "insert":
					skipped++
					continue
				default:
					for _, m := range mapping {
						idx, ok := colIndexMap[strings.ToLower(m.DBField)]
						if !ok {
							continue
						}
						val := values[idx]
						if val == nil {
							continue
						}
						field, ok := fieldMap[m.PBField]
						if !ok {
							continue
						}
						converted := convertValue(field.Type(), val)
						existing[0].Set(m.PBField, converted)
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

	log.Printf("pbmssql: imported %d records from %s (skipped: %d, updated: %d)", imported, table, skipped, updated)
	return nil
}

func convertValue(fieldType string, val any) any {
	if val == nil {
		return nil
	}

	switch fieldType {
	case "number":
		switch v := val.(type) {
		case int64:
			return float64(v)
		case float64:
			return v
		case []byte:
			var f float64
			if _, err := fmt.Sscanf(string(v), "%f", &f); err == nil {
				return f
			}
		}
	case "bool":
		switch v := val.(type) {
		case bool:
			return v
		case int64:
			return v != 0
		case []byte:
			return string(v) == "1" || strings.EqualFold(string(v), "true")
		}
	case "date", "autodate":
		switch v := val.(type) {
		case time.Time:
			return v.Format("2006-01-02 15:04:05")
		case []byte:
			return string(v)
		}
	}

	return fmt.Sprintf("%v", val)
}
