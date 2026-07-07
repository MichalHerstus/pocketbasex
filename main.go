package main

import (
	"embed"
	"encoding/json"
	"html/template"
	"log"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/hook"

	"pbx/pbexcel"
	"pbx/views"
)

//go:embed views/*.html
//go:embed views/assets
var viewsFS embed.FS

var templates *template.Template

func init() {
	templates = template.Must(template.New("").
		Funcs(template.FuncMap{
			"add": func(a, b int) int { return a + b },
			"sub": func(a, b int) int { return a - b },
			"seq": func(n int) []int {
				r := make([]int, n)
				for i := range r {
					r[i] = i
				}
				return r
			},
			"safeJS":   func(s string) template.JS { return template.JS(s) },
			"safeHTML": func(s string) template.HTML { return template.HTML(s) },
		}).
		ParseFS(viewsFS, "views/*.html"))
}

func main() {
	app := pocketbase.New()

	// all endpoints
	app.OnServe().Bind(&hook.Handler[*core.ServeEvent]{
		Func: func(se *core.ServeEvent) error {
			// login dialog
			se.Router.GET("/login", func(e *core.RequestEvent) error {
				e.Response.Header().Set("Content-Type", "text/html; charset=utf-8")
				return templates.ExecuteTemplate(e.Response, "login.html", map[string]string{})
			})
			// login form submission
			se.Router.POST("/login", func(e *core.RequestEvent) error {
				return handleLoginPost(e)
			})
			// main app page: list of collections, export/import, etc.
			se.Router.GET("/app", func(e *core.RequestEvent) error {
				return handleApp(e)
			})
			// collection tableform view
			se.Router.GET("/tabulator/{collectionName}", func(e *core.RequestEvent) error {
				return handleTabulator(e)
			})
			// collection form view (new or edit)
			se.Router.GET("/form/{collectionName}", func(e *core.RequestEvent) error {
				return handleForm(e)
			})

			se.Router.GET("/form/{collectionName}/{id}", func(e *core.RequestEvent) error {
				return handleForm(e)
			})

			se.Router.POST("/form/{collectionName}", func(e *core.RequestEvent) error {
				return handleFormPost(e)
			})

			se.Router.POST("/form/{collectionName}/{id}", func(e *core.RequestEvent) error {
				return handleFormPost(e)
			})
			// delete record
			se.Router.POST("/form/{collectionName}/{id}/delete", func(e *core.RequestEvent) error {
				return handleDeleteRecord(e)
			})
			// export collection to Excel
			se.Router.GET("/export/{collectionName}", func(e *core.RequestEvent) error {
				return handleExport(e)
			})
			// serve static assets
			se.Router.GET("/assets/{path...}", func(e *core.RequestEvent) error {
				path := e.Request.PathValue("path")
				if path == "" {
					return e.NotFoundError("Missing path", nil)
				}
				data, err := viewsFS.ReadFile("views/assets/" + path)
				if err != nil {
					return e.NotFoundError("Asset not found", err)
				}
				ct := "application/octet-stream"
				if strings.HasSuffix(path, ".png") {
					ct = "image/png"
				} else if strings.HasSuffix(path, ".css") {
					ct = "text/css; charset=utf-8"
				}
				e.Response.Header().Set("Content-Type", ct)
				e.Response.Write(data)
				return nil
			})

			return se.Next()
		},
	})

	if err := app.Start(); err != nil {
		log.Fatal(err)
	}
}

// --- App ---

func handleApp(e *core.RequestEvent) error {
	var userName string

	cookie, cookieErr := e.Request.Cookie("pb_auth")
	if cookieErr == nil {
		record, findErr := e.App.FindAuthRecordByToken(cookie.Value, core.TokenTypeAuth)
		if findErr == nil && record != nil {
			userName = record.GetString("name")
		}
	}

	appRecs, recErr := e.App.FindAllRecords("_app")
	if recErr != nil {
		return e.InternalServerError("Failed to fetch app records", recErr)
	}

	appColl, _ := e.App.FindCachedCollectionByNameOrId("_app")
	appCollID := ""
	if appColl != nil {
		appCollID = appColl.Id
	}

	type linkEntry struct {
		group      string
		groupLabel string
		groupIcon  string
		collection string
		label      string
	}

	entries := make([]linkEntry, 0, len(appRecs))
	for _, rec := range appRecs {
		iconURL := ""
		if gi := rec.GetString("group_icon"); gi != "" && appCollID != "" {
			iconURL = "/api/files/" + appCollID + "/" + rec.Id + "/" + gi
		}
		entries = append(entries, linkEntry{
			group:      rec.GetString("group"),
			groupLabel: rec.GetString("group_label"),
			groupIcon:  iconURL,
			collection: rec.GetString("collection"),
			label:      rec.GetString("collectionLabel"),
		})
	}

	groupOrder := make([]string, 0)
	groups := map[string]*views.AppGroup{}
	for _, ent := range entries {
		g, ok := groups[ent.group]
		if !ok {
			g = &views.AppGroup{GroupLabel: ent.groupLabel, GroupIcon: ent.groupIcon}
			groups[ent.group] = g
			groupOrder = append(groupOrder, ent.group)
		}
		g.Links = append(g.Links, views.AppLink{
			Collection: ent.collection,
			Label:      ent.label,
		})
	}

	grouped := make([]views.AppGroup, 0, len(groupOrder))
	for _, g := range groupOrder {
		grouped = append(grouped, *groups[g])
	}

	data := views.AppPageData{
		Name:   userName,
		Groups: grouped,
	}
	if userName == "" {
		data.Error = "Please sign in"
	}

	e.Response.Header().Set("Content-Type", "text/html; charset=utf-8")
	return templates.ExecuteTemplate(e.Response, "app.html", data)
}

// --- Login ---

func handleLoginPost(e *core.RequestEvent) error {
	name := e.Request.FormValue("name")
	password := e.Request.FormValue("password")

	if name == "" || password == "" {
		e.Response.Header().Set("Content-Type", "text/html; charset=utf-8")
		return templates.ExecuteTemplate(e.Response, "login.html", map[string]string{
			"Error": "Name and password are required",
		})
	}

	record, err := e.App.FindAuthRecordByEmail("users", name)
	if err != nil || !record.ValidatePassword(password) {
		e.Response.Header().Set("Content-Type", "text/html; charset=utf-8")
		return templates.ExecuteTemplate(e.Response, "login.html", map[string]string{
			"Error": "Invalid name or password",
		})
	}

	token, tokenErr := record.NewAuthToken()
	if tokenErr != nil {
		return e.InternalServerError("Failed to create auth token", tokenErr)
	}

	e.SetCookie(&http.Cookie{
		Name:     "pb_auth",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   false,
	})

	return e.Redirect(http.StatusSeeOther, "/app")
}

// --- Tabulator ---

func handleTabulator(e *core.RequestEvent) error {
	collName := e.Request.PathValue("collectionName")

	collection, err := e.App.FindCachedCollectionByNameOrId(collName)
	if err != nil {
		return e.NotFoundError("Collection not found", err)
	}

	records, err := e.App.FindAllRecords(collName)
	if err != nil {
		return e.InternalServerError("Failed to fetch records", err)
	}

	var configRec *core.Record
	configRecs, _ := e.App.FindRecordsByFilter("_tabulator", "collName = {:name}", "", 1, 0, nil, map[string]any{"name": collName})
	if len(configRecs) > 0 {
		configRec = configRecs[0]
	}

	cfg := views.TabulatorConfig{}
	if configRec != nil {
		cfg.PageTitle = configRec.GetString("pageTitle")
		cfg.CollectionDescr = configRec.GetString("collectionDescr")
		cfg.ColumnTitles = configRec.GetString("columnTitles")
		cfg.ColumnOrder = configRec.GetString("columnOrder")
		cfg.ColumnSorting = configRec.GetBool("columnSorting")
		cfg.SearchBox = configRec.GetBool("searchBox")
		cfg.Pagination = configRec.GetBool("pagination")
		cfg.DisplaySystemCol = configRec.GetBool("displaySystemCol")
	}

	fields := collection.Fields

	systemCols := map[string]bool{"id": true, "created": true, "updated": true}

	var fieldIndices []int
	if cfg.ColumnOrder != "" {
		parts := strings.Split(cfg.ColumnOrder, ",")
		for _, p := range parts {
			idx, err := strconv.Atoi(strings.TrimSpace(p))
			if err == nil && idx >= 1 && idx <= len(fields) {
				fieldIndices = append(fieldIndices, idx-1)
			}
		}
	}

	if len(fieldIndices) == 0 {
		for i := range fields {
			fieldIndices = append(fieldIndices, i)
		}
	}

	var visibleFields []core.Field
	var visibleHeaders []string
	for _, i := range fieldIndices {
		f := fields[i]
		fName := f.GetName()
		if !cfg.DisplaySystemCol && systemCols[fName] {
			continue
		}
		visibleFields = append(visibleFields, f)
		visibleHeaders = append(visibleHeaders, fName)
	}

	fieldNames := make([]string, len(visibleFields))
	headers := make([]string, len(visibleFields))
	for i, f := range visibleFields {
		fieldNames[i] = f.GetName()
		headers[i] = f.GetName()
	}

	if cfg.ColumnTitles != "" {
		parts := strings.Split(cfg.ColumnTitles, ",")
		for i, p := range parts {
			if i < len(headers) {
				headers[i] = strings.TrimSpace(p)
			}
		}
	}

	allData := make([]map[string]string, 0, len(records))
	for _, rec := range records {
		rm := map[string]string{}
		for _, fn := range fieldNames {
			rm[fn] = rec.GetString(fn)
		}
		rm["id"] = rec.GetString("id")
		allData = append(allData, rm)
	}

	fieldsJSON, _ := json.Marshal(fieldNames)
	headersJSON, _ := json.Marshal(headers)
	recordsJSON, _ := json.Marshal(allData)

	totalPages := int(math.Ceil(float64(len(records)) / 20))
	if totalPages < 1 {
		totalPages = 1
	}

	data := views.TabulatorPageData{
		CollectionName: collName,
		TotalRecords:   len(records),
		Fields:         fieldNames,
		ColumnHeaders:  headers,
		FieldsJSON:     string(fieldsJSON),
		HeadersJSON:    string(headersJSON),
		RecordsJSON:    string(recordsJSON),
		PerPage:        20,
		Page:           1,
		TotalPages:     totalPages,
		Config:         cfg,
	}

	e.Response.Header().Set("Content-Type", "text/html; charset=utf-8")
	return templates.ExecuteTemplate(e.Response, "tabulator.html", data)
}

// --- Form ---

func handleForm(e *core.RequestEvent) error {
	collName := e.Request.PathValue("collectionName")
	recordID := e.Request.PathValue("id")

	collection, err := e.App.FindCachedCollectionByNameOrId(collName)
	if err != nil {
		return e.NotFoundError("Collection not found", err)
	}

	fields := collection.Fields

	var record *core.Record
	if recordID != "" {
		record, err = e.App.FindRecordById(collName, recordID)
		if err != nil {
			return e.NotFoundError("Record not found", err)
		}
	}

	var configRec *core.Record
	configRecs, _ := e.App.FindRecordsByFilter("_form", "collName = {:name}", "", 1, 0, nil, map[string]any{"name": collName})
	if len(configRecs) > 0 {
		configRec = configRecs[0]
	}

	title := collName
	if configRec != nil {
		if t := configRec.GetString("formTitle"); t != "" {
			title = t
		}
	}

	description := ""
	if configRec != nil {
		description = configRec.GetString("formDescr")
	}

	displaySystemCol := false
	if configRec != nil {
		displaySystemCol = configRec.GetBool("displaySystemCol")
	}

	formLayout := ""
	if configRec != nil {
		formLayout = configRec.GetString("formLayout")
	}

	columnOrder := ""
	if configRec != nil {
		columnOrder = configRec.GetString("columnOrder")
	}

	formLabels := ""
	if configRec != nil {
		formLabels = configRec.GetString("formLabels")
	}

	labelsOverride := map[string]string{}
	if formLabels != "" {
		parts := strings.Split(formLabels, ",")
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if kv := strings.SplitN(p, "=", 2); len(kv) == 2 {
				labelsOverride[strings.TrimSpace(kv[0])] = strings.TrimSpace(kv[1])
			}
		}
	}

	orderMap := map[int]int{}
	if columnOrder != "" {
		parts := strings.Split(columnOrder, ",")
		for i, p := range parts {
			idx, err := strconv.Atoi(strings.TrimSpace(p))
			if err == nil && idx >= 1 && idx <= len(fields) {
				orderMap[idx-1] = i
			}
		}
	}

	layout := [][]int{}
	if formLayout != "" {
		rows := strings.Split(formLayout, ";")
		for _, row := range rows {
			row = strings.TrimSpace(row)
			if row == "" {
				continue
			}
			cols := strings.Split(row, ",")
			colIndices := make([]int, 0, len(cols))
			for _, c := range cols {
				c = strings.TrimSpace(c)
				if idx, err := strconv.Atoi(c); err == nil && idx >= 0 && idx < len(fields) {
					colIndices = append(colIndices, idx)
				}
			}
			if len(colIndices) > 0 {
				layout = append(layout, colIndices)
			}
		}
	}

	systemCols := map[string]bool{"id": true, "created": true, "updated": true}

	sysFields := make([]views.FormFieldItem, 0)
	if displaySystemCol {
		for _, f := range fields {
			fName := f.GetName()
			if systemCols[fName] {
				val := ""
				if record != nil {
					if fName == "id" {
						val = record.GetString("id")
					} else {
						val = record.GetString(fName)
					}
				}
				sysFields = append(sysFields, views.FormFieldItem{
					Name:  fName,
					Label: fName,
					Type:  "text",
					Value: val,
				})
			}
		}
	}

	fieldType := func(f core.Field) string {
		switch f.Type() {
		case "bool":
			return "bool"
		case "number":
			return "number"
		case "editor":
			return "editor"
		default:
			return "text"
		}
	}

	fieldLabel := func(fName string) string {
		if l, ok := labelsOverride[fName]; ok {
			return l
		}
		return fName
	}

	rows := make([]views.FormRow, 0)

	if len(layout) > 0 {
		for _, rowCols := range layout {
			columns := make([]views.FormColumn, 0, len(rowCols))
			for _, ci := range rowCols {
				if ci < 0 || ci >= len(fields) {
					continue
				}
				f := fields[ci]
				fName := f.GetName()
				if systemCols[fName] && !displaySystemCol {
					continue
				}
				val := ""
				if record != nil {
					val = record.GetString(fName)
				}
				columns = append(columns, views.FormColumn{
					Fields: []views.FormFieldItem{
						{
							Name:  fName,
							Label: fieldLabel(fName),
							Type:  fieldType(f),
							Value: val,
						},
					},
				})
			}
			if len(columns) > 0 {
				rows = append(rows, views.FormRow{Columns: columns})
			}
		}
	} else {
		type sf struct {
			order int
			idx   int
			f     core.Field
		}
		sorted := make([]sf, 0, len(fields))
		for i, f := range fields {
			o := i
			if pos, ok := orderMap[i]; ok {
				o = pos
			}
			sorted = append(sorted, sf{order: o, idx: i, f: f})
		}
		for i := 0; i < len(sorted); i++ {
			for j := i + 1; j < len(sorted); j++ {
				if sorted[i].order > sorted[j].order {
					sorted[i], sorted[j] = sorted[j], sorted[i]
				}
			}
		}

		rowFields := make([]views.FormFieldItem, 0)
		for _, s := range sorted {
			fName := s.f.GetName()
			if systemCols[fName] && !displaySystemCol {
				continue
			}
			val := ""
			if record != nil {
				val = record.GetString(fName)
			}
			rowFields = append(rowFields, views.FormFieldItem{
				Name:  fName,
				Label: fieldLabel(fName),
				Type:  fieldType(s.f),
				Value: val,
			})
		}
		if len(rowFields) > 0 {
			rows = append(rows, views.FormRow{
				Columns: []views.FormColumn{{Fields: rowFields}},
			})
		}
	}

	data := views.FormPageData{
		CollectionName: collName,
		ID:             recordID,
		Title:          title,
		Description:    description,
		SystemFields:   sysFields,
		Rows:           rows,
		HasConfig:      configRec != nil,
		ViewOnly:       e.Request.URL.Query().Get("view") == "1",
	}

	e.Response.Header().Set("Content-Type", "text/html; charset=utf-8")
	return templates.ExecuteTemplate(e.Response, "form.html", data)
}

// --- Form POST (create/update) ---

func handleFormPost(e *core.RequestEvent) error {
	collName := e.Request.PathValue("collectionName")
	recordID := e.Request.PathValue("id")

	collection, err := e.App.FindCachedCollectionByNameOrId(collName)
	if err != nil {
		return e.NotFoundError("Collection not found", err)
	}

	var record *core.Record
	if recordID != "" {
		record, err = e.App.FindRecordById(collName, recordID)
		if err != nil {
			return e.NotFoundError("Record not found", err)
		}
	} else {
		record = core.NewRecord(collection)
	}

	systemCols := map[string]bool{"id": true, "created": true, "updated": true}

	for _, f := range collection.Fields {
		fName := f.GetName()
		if systemCols[fName] {
			continue
		}
		switch f.Type() {
		case "bool":
			record.Set(fName, e.Request.FormValue(fName) == "on")
		case "number":
			val := e.Request.FormValue(fName)
			if val == "" {
				record.Set(fName, nil)
			} else {
				record.Set(fName, val)
			}
		default:
			record.Set(fName, e.Request.FormValue(fName))
		}
	}

	if err := e.App.Save(record); err != nil {
		return e.InternalServerError("Failed to save record", err)
	}

	msg := "Record successfully added."
	if recordID != "" {
		msg = "Record successfully updated."
	}

	return e.Redirect(http.StatusSeeOther, "/tabulator/"+collName+"?msg="+url.QueryEscape(msg))
}

// --- Export ---

func handleExport(e *core.RequestEvent) error {
	collName := e.Request.PathValue("collectionName")
	excelFileName := e.Request.URL.Query().Get("excelFileName")
	sheetName := e.Request.URL.Query().Get("sheetName")

	if err := pbexcel.ExportToExcel(e.App, excelFileName, sheetName, collName); err != nil {
		return e.InternalServerError("Export failed", err)
	}

	msg := url.QueryEscape("Export successful")
	return e.Redirect(http.StatusSeeOther, "/tabulator/"+collName+"?msg="+msg)
}

// --- Delete record ---

func handleDeleteRecord(e *core.RequestEvent) error {
	collName := e.Request.PathValue("collectionName")
	recordID := e.Request.PathValue("id")

	record, err := e.App.FindRecordById(collName, recordID)
	if err != nil {
		return e.NotFoundError("Record not found", err)
	}

	if err := e.App.Delete(record); err != nil {
		return e.InternalServerError("Failed to delete record", err)
	}

	return e.JSON(http.StatusOK, map[string]any{"ok": true})
}
