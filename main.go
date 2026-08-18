package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"math"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/plugins/jsvm"
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

	// load jsvm so pb_migrations/*.js migrations are auto-applied on serve
	jsvm.MustRegister(app, jsvm.Config{
		MigrationsDir: "pb_migrations",
		HooksDir:      "pb_hooks",
		HooksWatch:    false,
	})

	// all endpoints
	app.OnServe().Bind(&hook.Handler[*core.ServeEvent]{
		Func: func(se *core.ServeEvent) error {
			// login dialog
			se.Router.GET("/login", func(e *core.RequestEvent) error {
				e.Response.Header().Set("Content-Type", "text/html; charset=utf-8")
				return templates.ExecuteTemplate(e.Response, "login.html", map[string]string{
					"Theme": getThemeMode(e.App),
				})
			})
			// login form submission
			se.Router.POST("/login", func(e *core.RequestEvent) error {
				return handleLoginPost(e)
			})
			// logout: clear the auth cookie and redirect to the login page
			se.Router.GET("/logout", func(e *core.RequestEvent) error {
				e.SetCookie(&http.Cookie{
					Name:     "pb_auth",
					Value:    "",
					Path:     "/",
					HttpOnly: true,
					MaxAge:   -1,
				})
				return e.Redirect(http.StatusSeeOther, "/login")
			})
			// main app page: list of collections, export/import, etc.
			se.Router.GET("/app", func(e *core.RequestEvent) error {
				return handleApp(e)
			})
			// pbx setup: tables for _app, _tabulator, _form
			se.Router.GET("/pbx-setup", func(e *core.RequestEvent) error {
				return handlePbxSetup(e)
			})
			// pbx config editor (super admin)
			se.Router.GET("/pbx-config", func(e *core.RequestEvent) error {
				return handlePbxConfig(e)
			})
			se.Router.GET("/pbx-config/list/new", func(e *core.RequestEvent) error {
				e.Request.SetPathValue("configType", "list")
				e.Request.SetPathValue("name", "new")
				return handlePbxConfigEditor(e)
			})
			se.Router.GET("/pbx-config/list/{name}", func(e *core.RequestEvent) error {
				e.Request.SetPathValue("configType", "list")
				return handlePbxConfigEditor(e)
			})
			se.Router.GET("/pbx-config/form/new", func(e *core.RequestEvent) error {
				e.Request.SetPathValue("configType", "form")
				e.Request.SetPathValue("name", "new")
				return handlePbxConfigEditor(e)
			})
			se.Router.GET("/pbx-config/form/{name}", func(e *core.RequestEvent) error {
				e.Request.SetPathValue("configType", "form")
				return handlePbxConfigEditor(e)
			})
			se.Router.POST("/pbx-config/save", func(e *core.RequestEvent) error {
				return handlePbxConfigSave(e)
			})
			se.Router.POST("/pbx-config/delete", func(e *core.RequestEvent) error {
				return handlePbxConfigDelete(e)
			})
			// collection tableform view by configuration name
			se.Router.GET("/tabular/{configName}", func(e *core.RequestEvent) error {
				return handleTabulator(e)
			})
			// collection form view (new or edit) by configuration name
			se.Router.GET("/form/{configName}", func(e *core.RequestEvent) error {
				return handleForm(e)
			})

			se.Router.GET("/form/{configName}/{id}", func(e *core.RequestEvent) error {
				return handleForm(e)
			})

			se.Router.POST("/form/{configName}", func(e *core.RequestEvent) error {
				return handleFormPost(e)
			})

			se.Router.POST("/form/{configName}/{id}", func(e *core.RequestEvent) error {
				return handleFormPost(e)
			})
			// delete record
			se.Router.POST("/form/{configName}/{id}/delete", func(e *core.RequestEvent) error {
				return handleDeleteRecord(e)
			})
			// JSON data for relation modal
			se.Router.GET("/api/tabulator-data/{collectionName}", func(e *core.RequestEvent) error {
				return handleTabulatorDataJSON(e)
			})
			// export collection to Excel
			se.Router.GET("/export/{collectionName}", func(e *core.RequestEvent) error {
				return handleExport(e)
			})
			// import collection from Excel
			se.Router.POST("/import/{collectionName}", func(e *core.RequestEvent) error {
				return handleImport(e)
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
			// set the global default theme (light/dark), persisted in pb_data/theme.json
			se.Router.POST("/api/theme/{mode}", func(e *core.RequestEvent) error {
				mode := e.Request.PathValue("mode")
				if mode != "light" && mode != "dark" {
					return e.BadRequestError("Invalid theme mode", nil)
				}
				if err := setThemeMode(e.App, mode); err != nil {
					return e.InternalServerError("Failed to save theme", err)
				}
				return e.JSON(http.StatusOK, map[string]any{"ok": true, "mode": mode})
			})

			err := se.Next()
			printPbxEndpoints(se)
			return err
		},
	})

	if err := app.Start(); err != nil {
		log.Fatal(err)
	}
}

// printPbxEndpoints prints the PBX endpoints to stdout once the server is up,
// right after the standard PocketBase startup banner.
func printPbxEndpoints(se *core.ServeEvent) {
	host := se.Server.Addr
	if host == "" || strings.HasSuffix(host, ":http") || strings.HasSuffix(host, ":https") {
		host = "127.0.0.1"
	}
	baseURL := "http://" + host

	go func() {
		// wait until the server starts accepting connections so the PBX
		// endpoints appear after the standard startup banner
		for {
			conn, err := net.DialTimeout("tcp", host, 200*time.Millisecond)
			if err == nil {
				conn.Close()
				break
			}
			time.Sleep(200 * time.Millisecond)
		}

		cyan := func(s string) string {
			return "\x1b[36m" + s + "\x1b[0m"
		}

		println("┌─ PBX app:      " + cyan(baseURL+"/app"))
		println("├─ PBX list:     " + cyan(baseURL+"/tabular/{configName}"))
		println("├─ PBX form:     " + cyan(baseURL+"/form/{configName}"))
		println("├─ PBX setup:    " + cyan(baseURL+"/pbx-setup"))
		println("└─ PBX config:   " + cyan(baseURL+"/pbx-config"))
	}()
}

// --- App ---

func handleApp(e *core.RequestEvent) error {
	var userName string
	signedIn := false

	cookie, cookieErr := e.Request.Cookie("pb_auth")
	if cookieErr == nil {
		record, findErr := e.App.FindAuthRecordByToken(cookie.Value, core.TokenTypeAuth)
		if findErr == nil && record != nil {
			signedIn = true
			userName = record.GetString("name")
			if userName == "" {
				userName = record.GetString("email")
			}
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
		configName string
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
			configName: rec.GetString("configName"),
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
		linkURL := ent.configName
		if linkURL == "" {
			if def := defaultListConfig(e, ent.collection); def != nil {
				linkURL = def.GetString("_name")
			}
		}
		if linkURL == "" {
			linkURL = ent.collection
		}
		g.Links = append(g.Links, views.AppLink{
			Collection: ent.collection,
			Label:      ent.label,
			URL:        "/tabular/" + linkURL,
		})
	}

	grouped := make([]views.AppGroup, 0, len(groupOrder))
	for _, g := range groupOrder {
		grouped = append(grouped, *groups[g])
	}

	data := views.AppPageData{
		Theme:  getThemeMode(e.App),
		Name:   userName,
		Groups: grouped,
	}
	if !signedIn {
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
			"Theme": getThemeMode(e.App),
		})
	}

	record, err := e.App.FindAuthRecordByEmail("users", name)
	if err == nil {
		if record.ValidatePassword(password) {
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
	}

	record, err = e.App.FindAuthRecordByEmail("_superusers", name)
	if err == nil {
		if record.ValidatePassword(password) {
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
			return e.Redirect(http.StatusSeeOther, "/pbx-setup")
		}
	}

	e.Response.Header().Set("Content-Type", "text/html; charset=utf-8")
	return templates.ExecuteTemplate(e.Response, "login.html", map[string]string{
		"Error": "Invalid name or password",
		"Theme": getThemeMode(e.App),
	})
}

// --- Theme ---

// themeFilePath returns the JSON file that stores the global default theme.
func themeFilePath(app core.App) string {
	return filepath.Join(app.DataDir(), "theme.json")
}

// getThemeMode returns the global default theme ("light" or "dark").
func getThemeMode(app core.App) string {
	data, err := os.ReadFile(themeFilePath(app))
	if err != nil {
		return "light"
	}
	var cfg struct {
		Mode string `json:"mode"`
	}
	if json.Unmarshal(data, &cfg) != nil || (cfg.Mode != "light" && cfg.Mode != "dark") {
		return "light"
	}
	return cfg.Mode
}

// setThemeMode persists the global default theme to pb_data/theme.json.
func setThemeMode(app core.App, mode string) error {
	if mode != "light" && mode != "dark" {
		return fmt.Errorf("invalid theme mode %q", mode)
	}
	data, err := json.Marshal(map[string]string{"mode": mode})
	if err != nil {
		return err
	}
	return os.WriteFile(themeFilePath(app), data, 0o644)
}

// --- Config resolution ---

// findConfigByAttr returns the first record of a setup collection matching an attribute value.
func findConfigByAttr(e *core.RequestEvent, setupColl, attr, value string) *core.Record {
	if value == "" {
		return nil
	}
	recs, err := e.App.FindRecordsByFilter(setupColl, attr+" = {:v}", "", 1, 0, nil, map[string]any{"v": value})
	if err != nil || len(recs) == 0 {
		return nil
	}
	return recs[0]
}

// resolveListConfig resolves a list configuration by its _name and returns the target
// collection name plus the configuration record.
func resolveListConfig(e *core.RequestEvent, configName string) (string, *core.Record, error) {
	rec := findConfigByAttr(e, "_tabulator", "_name", configName)
	if rec == nil {
		return "", nil, fmt.Errorf("list configuration %q not found", configName)
	}
	return rec.GetString("collName"), rec, nil
}

// resolveFormConfig resolves a form configuration by its _name.
func resolveFormConfig(e *core.RequestEvent, configName string) (string, *core.Record, error) {
	rec := findConfigByAttr(e, "_form", "_name", configName)
	if rec == nil {
		return "", nil, fmt.Errorf("form configuration %q not found", configName)
	}
	return rec.GetString("collName"), rec, nil
}

// defaultListConfig returns the default (first) list configuration record for a collection.
func defaultListConfig(e *core.RequestEvent, collName string) *core.Record {
	recs, err := e.App.FindRecordsByFilter("_tabulator", "collName = {:c}", "", 1, 0, nil, map[string]any{"c": collName})
	if err != nil || len(recs) == 0 {
		return nil
	}
	return recs[0]
}

func parseListConfig(rec *core.Record) views.ListConfig {
	var lc views.ListConfig
	if rec == nil {
		return lc
	}
	if raw := configRaw(rec, "config"); raw != "" {
		_ = json.Unmarshal([]byte(raw), &lc)
	}
	return lc
}

func parseMssqlConfig(rec *core.Record) views.MssqlConfig {
	var mc views.MssqlConfig
	if rec == nil {
		return mc
	}
	if raw := rec.GetString("_mssql"); raw != "" {
		_ = json.Unmarshal([]byte(raw), &mc)
	}
	return mc
}

func parseFormConfigJSON(rec *core.Record) views.FormConfigJSON {
	var fc views.FormConfigJSON
	if rec == nil {
		return fc
	}
	if raw := configRaw(rec, "config"); raw != "" {
		_ = json.Unmarshal([]byte(raw), &fc)
	}
	return fc
}

// configRaw returns the normalized raw JSON stored in a record field, or "" when the
// field holds an empty/absent value ("", "null", "\"\"" are all treated as empty).
func configRaw(rec *core.Record, field string) string {
	if rec == nil {
		return ""
	}
	raw := strings.TrimSpace(rec.GetString(field))
	switch raw {
	case "", "null", `""`, "{}", "[]":
		return ""
	}
	return raw
}

// --- Tabulator ---

func buildTabulatorData(e *core.RequestEvent, collName string, configRec *core.Record) (*views.TabulatorPageData, error) {
	collection, err := e.App.FindCachedCollectionByNameOrId(collName)
	if err != nil {
		return nil, err
	}

	records, err := e.App.FindAllRecords(collName)
	if err != nil {
		return nil, err
	}

	lc := parseListConfig(configRec)

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
		cfg.Filter = configRec.GetString("filter")
	}
	// JSON config overrides scalar values where set
	if lc.Title != "" {
		cfg.PageTitle = lc.Title
	}
	if lc.Description != "" {
		cfg.CollectionDescr = lc.Description
	}
	if lc.SearchBox {
		cfg.SearchBox = true
	}
	if lc.Pagination {
		cfg.Pagination = true
	}
	if lc.DisplaySystemCol {
		cfg.DisplaySystemCol = true
	}
	if lc.Filter != "" {
		cfg.Filter = lc.Filter
	}

	fields := collection.Fields

	systemCols := map[string]bool{"id": true, "created": true, "updated": true}

	var fieldIndices []int
	var headers []string
	if len(lc.Columns) > 0 {
		nameToIdx := map[string]int{}
		for i, f := range fields {
			nameToIdx[f.GetName()] = i
		}
		for _, col := range lc.Columns {
			if idx, ok := nameToIdx[col.Field]; ok {
				fieldIndices = append(fieldIndices, idx)
				headers = append(headers, col.Title)
			}
		}
	} else if cfg.ColumnOrder != "" {
		parts := strings.Split(cfg.ColumnOrder, ",")
		for _, p := range parts {
			idx, err := strconv.Atoi(strings.TrimSpace(p))
			if err == nil && idx >= 1 && idx <= len(fields) {
				fieldIndices = append(fieldIndices, idx-1)
				headers = append(headers, "")
			}
		}
	}

	if len(fieldIndices) == 0 {
		for i := range fields {
			fieldIndices = append(fieldIndices, i)
		}
		headers = make([]string, len(fieldIndices))
	}

	var visibleFields []core.Field
	var visibleHeaders []string
	for j, i := range fieldIndices {
		f := fields[i]
		fName := f.GetName()
		if !cfg.DisplaySystemCol && systemCols[fName] {
			continue
		}
		visibleFields = append(visibleFields, f)
		h := ""
		if j < len(headers) {
			h = headers[j]
		}
		if h == "" {
			h = fName
		}
		visibleHeaders = append(visibleHeaders, h)
	}

	fieldNames := make([]string, len(visibleFields))
	fieldTypes := make([]string, len(visibleFields))

	relCollNames := map[string]string{}
	for i, f := range visibleFields {
		fn := f.GetName()
		fieldNames[i] = fn
		fieldTypes[i] = f.Type()
		if f.Type() == "relation" {
			if rf, ok := f.(*core.RelationField); ok {
				relColl, rerr := e.App.FindCachedCollectionByNameOrId(rf.CollectionId)
				if rerr == nil {
					relCollNames[fn] = relColl.Name
				}
			}
		}
	}

	if len(lc.Columns) == 0 && cfg.ColumnTitles != "" {
		parts := strings.Split(cfg.ColumnTitles, ",")
		for i, p := range parts {
			if i < len(visibleHeaders) {
				visibleHeaders[i] = strings.TrimSpace(p)
			}
		}
	}

	allData := make([]map[string]any, 0, len(records))
	for _, rec := range records {
		rm := map[string]any{}
		for i, fn := range fieldNames {
			f := visibleFields[i]
			switch f.Type() {
			case "bool":
				rm[fn] = rec.GetBool(fn)
			case "number":
				if rec.Get(fn) != nil {
					rm[fn] = rec.GetFloat(fn)
				} else {
					rm[fn] = nil
				}
			case "date", "autodate":
				if t := rec.GetDateTime(fn).Time(); !t.IsZero() {
					rm[fn] = t.Format("2006-01-02 15:04")
				} else {
					rm[fn] = nil
				}
			case "relation":
				raw := rec.GetString(fn)
				var ids []string
				if raw != "" {
					ids = strings.Split(raw, ",")
				}
				rm[fn] = map[string]any{
					"ids":            ids,
					"count":          len(ids),
					"collectionName": relCollNames[fn],
				}
			case "file":
				filename := rec.GetString(fn)
				if filename != "" {
					rm[fn] = map[string]any{
						"filename": filename,
						"url":      "/api/files/" + collection.Id + "/" + rec.GetString("id") + "/" + filename,
						"has":      true,
					}
				} else {
					rm[fn] = map[string]any{"has": false}
				}
			case "geo":
				geoVal := rec.GetString(fn)
				if geoVal != "" {
					parts := strings.Split(geoVal, ",")
					if len(parts) == 2 {
						rm[fn] = map[string]any{"lat": strings.TrimSpace(parts[0]), "lng": strings.TrimSpace(parts[1])}
					} else {
						rm[fn] = nil
					}
				} else {
					rm[fn] = nil
				}
			case "json":
				jv := rec.GetString(fn)
				if jv != "" && jv != "{}" && jv != "[]" && jv != "null" {
					rm[fn] = jv
				} else {
					rm[fn] = nil
				}
			default:
				rm[fn] = rec.GetString(fn)
			}
		}
		rm["id"] = rec.GetString("id")
		allData = append(allData, rm)
	}

	fieldsJSON, _ := json.Marshal(fieldNames)
	fieldTypesJSON, _ := json.Marshal(fieldTypes)
	headersJSON, _ := json.Marshal(visibleHeaders)
	recordsJSON, _ := json.Marshal(allData)

	totalPages := int(math.Ceil(float64(len(records)) / 20))
	if totalPages < 1 {
		totalPages = 1
	}

	return &views.TabulatorPageData{
		Theme:          getThemeMode(e.App),
		CollectionName: collName,
		TotalRecords:   len(records),
		Fields:         fieldNames,
		FieldTypes:     fieldTypes,
		ColumnHeaders:  visibleHeaders,
		FieldsJSON:     string(fieldsJSON),
		FieldTypesJSON: string(fieldTypesJSON),
		HeadersJSON:    string(headersJSON),
		RecordsJSON:    string(recordsJSON),
		PerPage:        20,
		Page:           1,
		TotalPages:     totalPages,
		Config:         cfg,
	}, nil
}

func handleTabulator(e *core.RequestEvent) error {
	configName := e.Request.PathValue("configName")

	collName, configRec, err := resolveListConfig(e, configName)
	if err != nil {
		return e.NotFoundError("Configuration not found", err)
	}

	data, err := buildTabulatorData(e, collName, configRec)
	if err != nil {
		return e.NotFoundError("Collection not found", err)
	}
	data.ConfigName = configName

	e.Response.Header().Set("Content-Type", "text/html; charset=utf-8")
	return templates.ExecuteTemplate(e.Response, "tabulator.html", data)
}

// --- PBX Setup ---

func handlePbxSetup(e *core.RequestEvent) error {
	collections := []string{"_app", "_tabulator", "_form"}
	sections := make([]views.TabulatorPageData, 0, len(collections))

	for _, name := range collections {
		data, err := buildTabulatorData(e, name, nil)
		if err != nil {
			continue
		}
		sections = append(sections, *data)
	}

	pageData := views.PbxSetupPageData{
		Theme:    getThemeMode(e.App),
		Sections: sections,
	}

	e.Response.Header().Set("Content-Type", "text/html; charset=utf-8")
	return templates.ExecuteTemplate(e.Response, "pbxsetup.html", pageData)
}

// --- Tabulator JSON data (for relation modal) ---

func handleTabulatorDataJSON(e *core.RequestEvent) error {
	collName := e.Request.PathValue("collectionName")

	collection, err := e.App.FindCachedCollectionByNameOrId(collName)
	if err != nil {
		return e.NotFoundError("Collection not found", err)
	}

	records, err := e.App.FindAllRecords(collName)
	if err != nil {
		return e.InternalServerError("Failed to fetch records", err)
	}

	fields := collection.Fields
	fieldNames := make([]string, 0, len(fields))
	for _, f := range fields {
		fieldNames = append(fieldNames, f.GetName())
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

	return e.JSON(http.StatusOK, map[string]any{
		"fields": fieldNames,
		"records": allData,
	})
}

// --- Form ---

func handleForm(e *core.RequestEvent) error {
	configName := e.Request.PathValue("configName")
	recordID := e.Request.PathValue("id")

	collName, configRec, err := resolveFormConfig(e, configName)
	if err != nil {
		return e.NotFoundError("Configuration not found", err)
	}

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

	fc := parseFormConfigJSON(configRec)
	if fc.Title != "" {
		title = fc.Title
	}
	if fc.Description != "" {
		description = fc.Description
	}
	if fc.DisplaySystemCol {
		displaySystemCol = true
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
	for k, v := range fc.Labels {
		labelsOverride[k] = v
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

	layout := [][][]int{}
	if len(fc.Layout) > 0 {
		layout = fc.Layout
	} else if formLayout != "" {
		rowStrs := strings.Split(formLayout, "/")
		for _, rowStr := range rowStrs {
			rowStr = strings.TrimSpace(rowStr)
			if rowStr == "" {
				continue
			}
			rowStr = strings.TrimPrefix(rowStr, "row:")
			rowStr = strings.TrimSpace(rowStr)
			if rowStr == "" {
				continue
			}
			cols := make([][]int, 0)
			for i := 0; i < len(rowStr); i++ {
				if rowStr[i] == '(' {
					j := i + 1
					depth := 1
					for j < len(rowStr) && depth > 0 {
						if rowStr[j] == '(' {
							depth++
						} else if rowStr[j] == ')' {
							depth--
						}
						j++
					}
					group := rowStr[i+1 : j-1]
					i = j - 1
					parts := strings.Split(group, ",")
					colFields := make([]int, 0, len(parts))
					for _, p := range parts {
						p = strings.TrimSpace(p)
						if idx, err := strconv.Atoi(p); err == nil {
							colFields = append(colFields, idx-1)
						}
					}
					if len(colFields) > 0 {
						cols = append(cols, colFields)
					}
				}
			}
			if len(cols) > 0 {
				layout = append(layout, cols)
			}
		}
	}

	systemCols := map[string]bool{"id": true, "created": true, "updated": true}

	sysFields := make([]views.FormFieldItem, 0)
	if displaySystemCol {
		for _, f := range fields {
			fName := f.GetName()
			if !systemCols[fName] {
				continue
			}
			item := views.FormFieldItem{
				Name: fName,
				Label: fName,
				Type: "text",
				Data: map[string]any{},
			}
			if record != nil {
				if fName == "id" {
					item.Value = record.GetString("id")
				} else if fName == "created" || fName == "updated" {
					item.Type = "autodate"
					if t := record.GetDateTime(fName).Time(); !t.IsZero() {
						item.Value = t.Format("2006-01-02 15:04")
					}
				} else {
					item.Value = record.GetString(fName)
				}
			}
			sysFields = append(sysFields, item)
		}
	}

	fieldType := func(f core.Field) string {
		return f.Type()
	}

	fieldLabel := func(fName string) string {
		if l, ok := labelsOverride[fName]; ok {
			return l
		}
		return fName
	}

	buildFieldItem := func(f core.Field, fName string) views.FormFieldItem {
		item := views.FormFieldItem{
			Name:  fName,
			Label: fieldLabel(fName),
			Type:  fieldType(f),
			Data:  map[string]any{},
		}
		if record == nil {
			return item
		}
		switch f.Type() {
		case "bool":
			item.Value = strconv.FormatBool(record.GetBool(fName))
		case "number":
			if record.Get(fName) != nil {
				item.Value = strconv.FormatFloat(record.GetFloat(fName), 'f', -1, 64)
			}
		case "date", "autodate":
			if t := record.GetDateTime(fName).Time(); !t.IsZero() {
				item.Value = t.Format("2006-01-02 15:04")
			}
		case "email":
			item.Value = record.GetString(fName)
		case "url":
			raw := record.GetString(fName)
			item.Value = raw
			if raw != "" {
				if u, err := url.Parse(raw); err == nil {
					parts := strings.Split(strings.TrimRight(u.Path, "/"), "/")
					display := raw
					for i := len(parts) - 1; i >= 0; i-- {
						if parts[i] != "" {
							display = parts[i]
							break
						}
					}
					item.Data["display"] = display
				}
			}
		case "editor":
			item.Value = record.GetString(fName)
		case "file":
			filename := record.GetString(fName)
			if filename != "" {
				item.Value = filename
				item.Data["url"] = "/api/files/" + collection.Id + "/" + record.GetString("id") + "/" + filename
				item.Data["has"] = true
			} else {
				item.Data["has"] = false
			}
		case "select":
			item.Value = record.GetString(fName)
			if sf, ok := f.(*core.SelectField); ok {
				item.Data["options"] = sf.Values
			}
		case "relation":
			raw := record.GetString(fName)
			var ids []string
			if raw != "" {
				ids = strings.Split(raw, ",")
			}
			item.Data["ids"] = ids
			item.Data["count"] = len(ids)
			item.Value = strconv.Itoa(len(ids))
			if rf, ok := f.(*core.RelationField); ok {
				if relColl, rerr := e.App.FindCachedCollectionByNameOrId(rf.CollectionId); rerr == nil {
					item.Data["collectionName"] = relColl.Name
				}
			}
		case "json":
			jv := record.GetString(fName)
			if jv != "" && jv != "{}" && jv != "[]" && jv != "null" {
				var parsed any
				if err := json.Unmarshal([]byte(jv), &parsed); err == nil {
					pretty, _ := json.MarshalIndent(parsed, "", "  ")
					item.Value = string(pretty)
				} else {
					item.Value = jv
				}
			}
		case "geo":
			geoVal := record.GetString(fName)
			if geoVal != "" {
				parts := strings.Split(geoVal, ",")
				if len(parts) == 2 {
					item.Data["lat"] = strings.TrimSpace(parts[0])
					item.Data["lng"] = strings.TrimSpace(parts[1])
				}
				item.Value = geoVal
			}
		default:
			item.Value = record.GetString(fName)
		}
		return item
	}

	rows := make([]views.FormRow, 0)

	if len(layout) > 0 {
		for _, rowCols := range layout {
			columns := make([]views.FormColumn, 0, len(rowCols))
			for _, colFieldIndices := range rowCols {
				fieldItems := make([]views.FormFieldItem, 0, len(colFieldIndices))
				for _, ci := range colFieldIndices {
					if ci < 0 || ci >= len(fields) {
						continue
					}
					f := fields[ci]
					fName := f.GetName()
					if systemCols[fName] && !displaySystemCol {
						continue
					}
					fieldItems = append(fieldItems, buildFieldItem(f, fName))
				}
				if len(fieldItems) > 0 {
					columns = append(columns, views.FormColumn{Fields: fieldItems})
				}
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
			rowFields = append(rowFields, buildFieldItem(s.f, fName))
		}
		if len(rowFields) > 0 {
			rows = append(rows, views.FormRow{
				Columns: []views.FormColumn{{Fields: rowFields}},
			})
		}
	}

	data := views.FormPageData{
		Theme:          getThemeMode(e.App),
		ConfigName:     configName,
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
	configName := e.Request.PathValue("configName")
	recordID := e.Request.PathValue("id")

	collName, _, err := resolveFormConfig(e, configName)
	if err != nil {
		return e.NotFoundError("Configuration not found", err)
	}

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

	return e.Redirect(http.StatusSeeOther, "/tabular/"+configName+"?msg="+url.QueryEscape(msg))
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
	return e.Redirect(http.StatusSeeOther, "/tabular/"+collName+"?msg="+msg)
}

// --- Import ---

func handleImport(e *core.RequestEvent) error {
	collName := e.Request.PathValue("collectionName")
	excelFileName := e.Request.FormValue("excelFileName")
	sheetName := e.Request.FormValue("sheetName")
	mode := e.Request.FormValue("mode")

	if mode == "" {
		mode = "insert"
	}

	if err := pbexcel.ImportFromExcel(e.App, excelFileName, sheetName, collName, mode); err != nil {
		return e.InternalServerError("Import failed", err)
	}

	msg := url.QueryEscape("Import successful")
	return e.Redirect(http.StatusSeeOther, "/tabular/"+collName+"?msg="+msg)
}

// --- Delete record ---

func handleDeleteRecord(e *core.RequestEvent) error {
	configName := e.Request.PathValue("configName")
	recordID := e.Request.PathValue("id")

	collName, _, err := resolveFormConfig(e, configName)
	if err != nil {
		return e.NotFoundError("Configuration not found", err)
	}

	record, err := e.App.FindRecordById(collName, recordID)
	if err != nil {
		return e.NotFoundError("Record not found", err)
	}

	if err := e.App.Delete(record); err != nil {
		return e.InternalServerError("Failed to delete record", err)
	}

	return e.JSON(http.StatusOK, map[string]any{"ok": true})
}

// --- PBX Config editor (super admin) ---

// requireSuperAdmin returns nil only when the request carries a valid _superusers auth token.
func requireSuperAdmin(e *core.RequestEvent) error {
	cookie, err := e.Request.Cookie("pb_auth")
	if err != nil {
		return e.Redirect(http.StatusSeeOther, "/login")
	}
	record, err := e.App.FindAuthRecordByToken(cookie.Value, core.TokenTypeAuth)
	if err != nil || record == nil {
		return e.Redirect(http.StatusSeeOther, "/login")
	}
	if record.Collection().Name != "_superusers" {
		return e.Redirect(http.StatusSeeOther, "/app")
	}
	return nil
}

// listCollections returns all non-system collection names, ordered by name.
func listCollections(e *core.RequestEvent) []string {
	colls, err := e.App.FindAllCollections()
	if err != nil {
		return nil
	}
	names := make([]string, 0, len(colls))
	for _, c := range colls {
		n := c.Name
		if strings.HasPrefix(n, "_") {
			continue
		}
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

func handlePbxConfig(e *core.RequestEvent) error {
	if err := requireSuperAdmin(e); err != nil {
		return err
	}

	pageData := views.PbxConfigPageData{
		Theme: getThemeMode(e.App),
	}

	if recs, err := e.App.FindAllRecords("_tabulator"); err == nil {
		for _, rec := range recs {
			pageData.ListConfigs = append(pageData.ListConfigs, views.ConfigEntry{
				Type:      "list",
				Name:      rec.GetString("_name"),
				CollName:  rec.GetString("collName"),
				Title:     rec.GetString("pageTitle"),
				HasConfig: configRaw(rec, "config") != "",
			})
		}
	}
	if recs, err := e.App.FindAllRecords("_form"); err == nil {
		for _, rec := range recs {
			pageData.FormConfigs = append(pageData.FormConfigs, views.ConfigEntry{
				Type:      "form",
				Name:      rec.GetString("_name"),
				CollName:  rec.GetString("collName"),
				Title:     rec.GetString("formTitle"),
				HasConfig: configRaw(rec, "config") != "",
			})
		}
	}

	e.Response.Header().Set("Content-Type", "text/html; charset=utf-8")
	return templates.ExecuteTemplate(e.Response, "pbxconfig.html", pageData)
}

func handlePbxConfigEditor(e *core.RequestEvent) error {
	if err := requireSuperAdmin(e); err != nil {
		return err
	}

	cfgType := e.Request.PathValue("configType")
	name := e.Request.PathValue("name")
	isNew := name == "new"

	if cfgType != "list" && cfgType != "form" {
		return e.NotFoundError("Unknown config type", nil)
	}

	pageData := views.ConfigEditorPageData{
		Theme:       getThemeMode(e.App),
		Type:        cfgType,
		TypeLabel:   "List",
		Collections: listCollections(e),
		IsNew:       isNew,
	}
	if cfgType == "form" {
		pageData.TypeLabel = "Form"
	}

	if !isNew {
		rec := findConfigByAttr(e, "_tabulator", "_name", name)
		if cfgType == "form" {
			rec = findConfigByAttr(e, "_form", "_name", name)
		}
		if rec == nil {
			return e.NotFoundError("Configuration not found", nil)
		}
		pageData.Name = rec.GetString("_name")
		pageData.CollName = rec.GetString("collName")
		pageData.ConfigJSON = configRaw(rec, "config")
	}

	e.Response.Header().Set("Content-Type", "text/html; charset=utf-8")
	return templates.ExecuteTemplate(e.Response, "config.html", pageData)
}

func handlePbxConfigSave(e *core.RequestEvent) error {
	if err := requireSuperAdmin(e); err != nil {
		return err
	}

	cfgType := e.Request.FormValue("type")
	name := e.Request.FormValue("name")
	collName := e.Request.FormValue("collName")
	configJSON := e.Request.FormValue("config")

	if cfgType != "list" && cfgType != "form" {
		return e.NotFoundError("Unknown config type", nil)
	}
	name = strings.TrimSpace(name)
	collName = strings.TrimSpace(collName)
	if name == "" || collName == "" {
		e.Response.Header().Set("Content-Type", "text/html; charset=utf-8")
		return templates.ExecuteTemplate(e.Response, "config.html", views.ConfigEditorPageData{
			Type:        cfgType,
			TypeLabel:   "List",
			Name:        name,
			CollName:    collName,
			ConfigJSON:  configJSON,
			Collections: listCollections(e),
			IsNew:       true,
		})
	}

	setupColl := "_tabulator"
	if cfgType == "form" {
		setupColl = "_form"
	}

	rec := findConfigByAttr(e, setupColl, "_name", name)
	if rec == nil {
		setupCollection, err := e.App.FindCachedCollectionByNameOrId(setupColl)
		if err != nil {
			return e.InternalServerError("Setup collection not found", err)
		}
		rec = core.NewRecord(setupCollection)
	}

	rec.Set("_name", name)
	rec.Set("collName", collName)
	rec.Set("config", configJSON)
	if cfgType == "list" {
		rec.Set("pageTitle", parseListConfig(rec).Title)
	} else {
		rec.Set("formTitle", parseFormConfigJSON(rec).Title)
	}

	if err := e.App.Save(rec); err != nil {
		return e.InternalServerError("Failed to save configuration", err)
	}

	return e.Redirect(http.StatusSeeOther, "/pbx-config")
}

func handlePbxConfigDelete(e *core.RequestEvent) error {
	if err := requireSuperAdmin(e); err != nil {
		return err
	}

	cfgType := e.Request.FormValue("type")
	name := e.Request.FormValue("name")

	setupColl := "_tabulator"
	if cfgType == "form" {
		setupColl = "_form"
	}

	rec := findConfigByAttr(e, setupColl, "_name", name)
	if rec != nil {
		if err := e.App.Delete(rec); err != nil {
			return e.InternalServerError("Failed to delete configuration", err)
		}
	}

	return e.Redirect(http.StatusSeeOther, "/pbx-config")
}
