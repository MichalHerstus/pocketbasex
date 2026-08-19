package main

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"log"
	"math"
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/plugins/jsvm"
	"github.com/pocketbase/pocketbase/tools/filesystem"
	"github.com/pocketbase/pocketbase/tools/hook"
	"github.com/pocketbase/pocketbase/tools/inflector"
	"github.com/pocketbase/pocketbase/tools/security"
	"github.com/pocketbase/pocketbase/tools/search"

	"pbx/pbai"
	"pbx/pbexcel"
	"pbx/pbmssql"
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
			"dict": func(vals ...any) map[string]any {
				m := map[string]any{}
				for i := 0; i+1 < len(vals); i += 2 {
					if k, ok := vals[i].(string); ok {
						m[k] = vals[i+1]
					}
				}
				return m
			},
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
			// pbx setup: system record editors (_app, _views, _agent) — super admin
			se.Router.GET("/pbx-setup/record/{coll}/new", func(e *core.RequestEvent) error {
				return handleSetupRecord(e)
			})
			se.Router.GET("/pbx-setup/record/{coll}/{id}", func(e *core.RequestEvent) error {
				return handleSetupRecord(e)
			})
			se.Router.POST("/pbx-setup/record/{coll}", func(e *core.RequestEvent) error {
				return handleSetupRecordPost(e)
			})
			se.Router.POST("/pbx-setup/record/{coll}/{id}", func(e *core.RequestEvent) error {
				return handleSetupRecordPost(e)
			})
			se.Router.POST("/pbx-setup/record/{coll}/{id}/delete", func(e *core.RequestEvent) error {
				return handleSetupRecordDelete(e)
			})
			// pbx setup: collection API rules — super admin
			se.Router.POST("/pbx-setup/rules", func(e *core.RequestEvent) error {
				return handleSetupRulesPost(e)
			})
			// pbx config editor (super admin)
			se.Router.GET("/pbx-config", func(e *core.RequestEvent) error {
				return handlePbxConfig(e)
			})
			se.Router.GET("/pbx-config/view/new", func(e *core.RequestEvent) error {
				e.Request.SetPathValue("name", "new")
				return handlePbxConfigEditor(e)
			})
			se.Router.GET("/pbx-config/view/{name}", func(e *core.RequestEvent) error {
				return handlePbxConfigEditor(e)
			})
			se.Router.POST("/pbx-config/save", func(e *core.RequestEvent) error {
				return handlePbxConfigSave(e)
			})
			se.Router.POST("/pbx-config/delete", func(e *core.RequestEvent) error {
				return handlePbxConfigDelete(e)
			})
			// collection creation wizard from Excel / MSSQL
			se.Router.GET("/pbx-config/import-excel", func(e *core.RequestEvent) error {
				return handleImportWizard(e, "excel")
			})
			se.Router.POST("/pbx-config/import-excel", func(e *core.RequestEvent) error {
				return handleImportWizard(e, "excel")
			})
			se.Router.GET("/pbx-config/import-mssql", func(e *core.RequestEvent) error {
				return handleImportWizard(e, "mssql")
			})
			se.Router.POST("/pbx-config/import-mssql", func(e *core.RequestEvent) error {
				return handleImportWizard(e, "mssql")
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
			// named (saved) advanced filters for a /tabular view
			se.Router.GET("/api/filters/{configName}", func(e *core.RequestEvent) error {
				return handleFiltersList(e)
			})
			se.Router.POST("/api/filters/{configName}", func(e *core.RequestEvent) error {
				return handleFilterSave(e)
			})
			se.Router.DELETE("/api/filters/{id}", func(e *core.RequestEvent) error {
				return handleFilterDelete(e)
			})
			// export collection to Excel
			se.Router.GET("/export/{collectionName}", func(e *core.RequestEvent) error {
				return handleExport(e)
			})
			// import collection from Excel
			se.Router.POST("/import/{collectionName}", func(e *core.RequestEvent) error {
				return handleImport(e)
			})
			// MSSQL export
			se.Router.POST("/mssql-export/{collectionName}", func(e *core.RequestEvent) error {
				return handleMssqlExport(e)
			})
			// MSSQL import
			se.Router.POST("/mssql-import/{collectionName}", func(e *core.RequestEvent) error {
				return handleMssqlImport(e)
			})
			// MSSQL introspect table
			se.Router.GET("/mssql-introspect", func(e *core.RequestEvent) error {
				return handleMssqlIntrospect(e)
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
			// save the global default MSSQL DSN, persisted in pb_data/mssql.json
			se.Router.POST("/api/mssql-dsn", func(e *core.RequestEvent) error {
				dsn := e.Request.FormValue("dsn")
				if strings.TrimSpace(dsn) == "" {
					return e.BadRequestError("DSN cannot be empty", nil)
				}
				if err := setMssqlDSN(e.App, dsn); err != nil {
					return e.InternalServerError("Failed to save MSSQL DSN", err)
				}
				return e.JSON(http.StatusOK, map[string]any{"ok": true, "dsn": dsn})
			})
			// read the active AI agent configuration (super admin)
			se.Router.GET("/api/ai-config", func(e *core.RequestEvent) error {
				if err := requireSuperAdmin(e); err != nil {
					return err
				}
				return e.JSON(http.StatusOK, getAgentConfig(e))
			})
			// save the active AI agent configuration (super admin)
			se.Router.POST("/api/ai-config", func(e *core.RequestEvent) error {
				if err := requireSuperAdmin(e); err != nil {
					return err
				}
				var cfg views.AgentConfig
				if err := json.NewDecoder(e.Request.Body).Decode(&cfg); err != nil {
					return e.BadRequestError("Invalid agent config JSON", err)
				}
				if err := setAgentConfig(e, cfg); err != nil {
					return e.InternalServerError("Failed to save agent config", err)
				}
				return e.JSON(http.StatusOK, map[string]any{"ok": true})
			})
			// AI agent status used by the /ai page and setup section
			se.Router.GET("/api/ai/status", func(e *core.RequestEvent) error {
				cfg := getAgentConfig(e)
				status := "not_configured"
				if cfg.Enabled && cfg.BaseURL != "" && cfg.Model != "" {
					status = "configured"
				}
				return e.JSON(http.StatusOK, map[string]any{
					"configured": status == "configured",
					"enabled":    cfg.Enabled,
					"provider":   cfg.Provider,
					"model":      cfg.Model,
					"status":     status,
				})
			})
			// AI agent chat page
			se.Router.GET("/ai", func(e *core.RequestEvent) error {
				return handleAgent(e)
			})
			// AI agent chat request
			se.Router.POST("/ai/chat", func(e *core.RequestEvent) error {
				return handleAgentChat(e)
			})
			// AI agent write-op confirmation
			se.Router.POST("/ai/confirm", func(e *core.RequestEvent) error {
				return handleAgentConfirm(e)
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

// --- MSSQL global DSN ---

// mssqlDSNFilePath returns the JSON file that stores the global default MSSQL DSN.
func mssqlDSNFilePath(app core.App) string {
	return filepath.Join(app.DataDir(), "mssql.json")
}

// getMssqlDSN returns the global default MSSQL DSN ("" when unset).
func getMssqlDSN(app core.App) string {
	data, err := os.ReadFile(mssqlDSNFilePath(app))
	if err != nil {
		return ""
	}
	var cfg struct {
		DSN string `json:"dsn"`
	}
	if json.Unmarshal(data, &cfg) != nil {
		return ""
	}
	return cfg.DSN
}

// setMssqlDSN persists the global default MSSQL DSN to pb_data/mssql.json.
func setMssqlDSN(app core.App, dsn string) error {
	if strings.TrimSpace(dsn) == "" {
		return fmt.Errorf("dsn cannot be empty")
	}
	data, err := json.Marshal(map[string]string{"dsn": dsn})
	if err != nil {
		return err
	}
	return os.WriteFile(mssqlDSNFilePath(app), data, 0o644)
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
	rec := findConfigByAttr(e, "_views", "_name", configName)
	if rec == nil {
		return "", nil, fmt.Errorf("list configuration %q not found", configName)
	}
	return rec.GetString("_collName"), rec, nil
}

// resolveFormConfig resolves a form configuration by its _name.
func resolveFormConfig(e *core.RequestEvent, configName string) (string, *core.Record, error) {
	rec := findConfigByAttr(e, "_views", "_name", configName)
	if rec == nil {
		return "", nil, fmt.Errorf("form configuration %q not found", configName)
	}
	return rec.GetString("_collName"), rec, nil
}

// defaultListConfig returns the default (first) list configuration record for a collection.
func defaultListConfig(e *core.RequestEvent, collName string) *core.Record {
	recs, err := e.App.FindRecordsByFilter("_views", "_collName = {:c}", "", 1, 0, nil, map[string]any{"c": collName})
	if err != nil || len(recs) == 0 {
		return nil
	}
	return recs[0]
}

// parseListConfig is retained for legacy reading of a raw list config JSON blob.
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

// parseViewTabulator parses the _views record _tabulator JSON field.
func parseViewTabulator(rec *core.Record) views.ViewTabulatorConfig {
	var vc views.ViewTabulatorConfig
	if rec == nil {
		return vc
	}
	if raw := configRaw(rec, "_tabulator"); raw != "" {
		_ = json.Unmarshal([]byte(raw), &vc)
	}
	return vc
}

// parseViewForm parses the _views record _form JSON field.
func parseViewForm(rec *core.Record) views.ViewFormConfig {
	var fc views.ViewFormConfig
	if rec == nil {
		return fc
	}
	if raw := configRaw(rec, "_form"); raw != "" {
		_ = json.Unmarshal([]byte(raw), &fc)
	}
	return fc
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

// --- AI agent config ---

const agentConfigName = "default"

// getAgentConfig loads the active _agent configuration (record named "default").
func getAgentConfig(e *core.RequestEvent) views.AgentConfig {
	var cfg views.AgentConfig
	rec := findConfigByAttr(e, "_agent", "_name", agentConfigName)
	if rec == nil {
		return cfg
	}
	if raw := rec.GetString("_config"); raw != "" {
		_ = json.Unmarshal([]byte(raw), &cfg)
	}
	return cfg
}

// setAgentConfig upserts the active _agent configuration (record named "default").
func setAgentConfig(e *core.RequestEvent, cfg views.AgentConfig) error {
	agentColl, err := e.App.FindCachedCollectionByNameOrId("_agent")
	if err != nil {
		return fmt.Errorf("agent collection not found: %w", err)
	}

	rec := findConfigByAttr(e, "_agent", "_name", agentConfigName)
	if rec == nil {
		rec = core.NewRecord(agentColl)
		rec.Set("_name", agentConfigName)
	}

	data, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	rec.Set("_config", string(data))
	return e.App.Save(rec)
}

// formConfigFromView converts the unified _views._form config to the legacy
// FormConfigJSON shape used by the view-editing model.
func formConfigFromView(fc views.ViewFormConfig) views.FormConfigJSON {
	return views.FormConfigJSON{
		Title:            fc.FormTitle,
		Description:      fc.FormDescr,
		DisplaySystemCol: fc.DisplaySystemCol,
		Layout:           fc.Layout,
		Labels:           fc.Labels,
		Collections:      fc.Collections,
	}
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

	if !collection.IsView() {
		records = filterListedRecords(e, collection, records)
	}

	vc := parseViewTabulator(configRec)

	cfg := views.TabulatorConfig{
		PageTitle:        vc.PageTitle,
		CollectionDescr:  vc.CollectionDescr,
		ColumnTitles:     vc.ColumnTitles,
		ColumnOrder:      vc.ColumnOrder,
		ColumnSorting:    vc.ColumnSorting,
		SearchBox:        vc.SearchBox,
		Pagination:       vc.Pagination,
		DisplaySystemCol: vc.DisplaySystemCol,
		Filter:           vc.Filter,
	}

	fields := collection.Fields

	systemCols := map[string]bool{"id": true, "created": true, "updated": true}

	var fieldIndices []int
	var headers []string
	if len(vc.Columns) > 0 {
		nameToIdx := map[string]int{}
		for i, f := range fields {
			nameToIdx[f.GetName()] = i
		}
		for _, col := range vc.Columns {
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

	if len(vc.Columns) == 0 && cfg.ColumnTitles != "" {
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
		Mssql:          effectiveMssqlConfig(parseMssqlConfig(configRec), getMssqlDSN(e.App)),
	}, nil
}

// effectiveMssqlConfig returns the MSSQL config for a tabular view, falling back
// to the global default DSN when the record config does not provide one. The
// pointer is nil when nothing MSSQL-related has been configured.
func effectiveMssqlConfig(mc views.MssqlConfig, globalDSN string) *views.MssqlConfig {
	if mc.DSN == "" {
		mc.DSN = globalDSN
	}
	if mc.DSN == "" {
		return nil
	}
	return &mc
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
	if err := requireSuperAdmin(e); err != nil {
		return err
	}

	collections := []string{"_app", "_views"}
	sections := make([]views.TabulatorPageData, 0, len(collections))

	for _, name := range collections {
		data, err := buildTabulatorData(e, name, nil)
		if err != nil {
			continue
		}
		data.SetupLinks = true
		sections = append(sections, *data)
	}

	pageData := views.PbxSetupPageData{
		Theme:    getThemeMode(e.App),
		MssqlDSN: getMssqlDSN(e.App),
		Agent:    getAgentConfig(e),
		Sections: sections,
		Rules:    buildSetupRules(e),
		Users:    buildSetupUsers(e),
	}

	e.Response.Header().Set("Content-Type", "text/html; charset=utf-8")
	return templates.ExecuteTemplate(e.Response, "pbxsetup.html", pageData)
}

// --- Collection API rules ---

// ruleCollectionNames returns the data collections the rules editor applies to
// (excludes users, roles, _-prefixed system collections and view collections).
func ruleCollectionNames(e *core.RequestEvent) []*core.Collection {
	colls, err := e.App.FindAllCollections()
	if err != nil {
		return nil
	}
	out := make([]*core.Collection, 0, len(colls))
	for _, c := range colls {
		if c.IsView() {
			continue
		}
		n := c.Name
		if strings.HasPrefix(n, "_") || n == "users" || n == "roles" {
			continue
		}
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// buildSetupRules parses the current collection rules into the rules editor UI state.
func buildSetupRules(e *core.RequestEvent) []views.SetupCollectionRules {
	colls := ruleCollectionNames(e)
	rules := make([]views.SetupCollectionRules, 0, len(colls))
	for _, c := range colls {
		rules = append(rules, views.SetupCollectionRules{
			Collection: c.Name,
			Items: []views.SetupRuleItem{
				{Type: "list", Rule: parseRuleToSetup(c.ListRule)},
				{Type: "view", Rule: parseRuleToSetup(c.ViewRule)},
				{Type: "create", Rule: parseRuleToSetup(c.CreateRule)},
				{Type: "update", Rule: parseRuleToSetup(c.UpdateRule)},
				{Type: "delete", Rule: parseRuleToSetup(c.DeleteRule)},
			},
		})
	}
	return rules
}

// parseRuleToSetup converts a stored collection rule pointer to the editor UI state.
func parseRuleToSetup(rule *string) views.SetupRule {
	if rule == nil {
		return views.SetupRule{Mode: views.RuleModeSuper}
	}
	s := *rule
	if s == "" {
		return views.SetupRule{Mode: views.RuleModePublic}
	}
	if s == "@request.auth.id != ''" {
		return views.SetupRule{Mode: views.RuleModeSignedIn}
	}
	// OR-chain of @request.auth.id = "id1" || @request.auth.id = "id2"
	parts := strings.Split(s, "||")
	allIDs := true
	ids := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		prefix := `@request.auth.id = "`
		if !strings.HasPrefix(p, prefix) || !strings.HasSuffix(p, `"`) {
			allIDs = false
			break
		}
		ids = append(ids, strings.TrimSuffix(strings.TrimPrefix(p, prefix), `"`))
	}
	if allIDs && len(ids) > 0 {
		return views.SetupRule{Mode: views.RuleModeSelected, Users: ids}
	}
	return views.SetupRule{Mode: views.RuleModeCustom, Custom: s}
}

// ruleFromSetup converts the editor UI state back to a stored rule pointer.
func ruleFromSetup(r views.SetupRule) *string {
	switch r.Mode {
	case views.RuleModePublic:
		s := ""
		return &s
	case views.RuleModeSignedIn:
		s := "@request.auth.id != ''"
		return &s
	case views.RuleModeSelected:
		parts := make([]string, 0, len(r.Users))
		for _, id := range r.Users {
			if id == "" {
				continue
			}
			parts = append(parts, `@request.auth.id = "`+id+`"`)
		}
		if len(parts) == 0 {
			s := "id = ''" // deny all
			return &s
		}
		s := strings.Join(parts, " || ")
		return &s
	case views.RuleModeCustom:
		if strings.TrimSpace(r.Custom) == "" {
			return nil
		}
		s := r.Custom
		return &s
	default: // super
		return nil
	}
}

// buildSetupUsers returns all users records for the rules editor checkbox list.
func buildSetupUsers(e *core.RequestEvent) []views.SetupUser {
	recs, err := e.App.FindAllRecords("users")
	if err != nil {
		return nil
	}
	out := make([]views.SetupUser, 0, len(recs))
	for _, rec := range recs {
		label := rec.GetString("name")
		if label == "" {
			label = rec.GetString("email")
		}
		if label == "" {
			label = rec.GetString("id")
		}
		out = append(out, views.SetupUser{ID: rec.GetString("id"), Label: label})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Label < out[j].Label })
	return out
}

// handleSetupRulesPost persists the collection API rules from the /pbx-setup rules form.
func handleSetupRulesPost(e *core.RequestEvent) error {
	if err := requireSuperAdmin(e); err != nil {
		return err
	}

	if err := e.Request.ParseForm(); err != nil {
		return e.BadRequestError("Invalid form data", err)
	}

	for _, c := range ruleCollectionNames(e) {
		rule, err := e.App.FindCollectionByNameOrId(c.Name)
		if err != nil {
			continue
		}
		rule.ListRule = ruleFromSetup(setupRuleFromForm(e, c.Name, "list"))
		rule.ViewRule = ruleFromSetup(setupRuleFromForm(e, c.Name, "view"))
		rule.CreateRule = ruleFromSetup(setupRuleFromForm(e, c.Name, "create"))
		rule.UpdateRule = ruleFromSetup(setupRuleFromForm(e, c.Name, "update"))
		rule.DeleteRule = ruleFromSetup(setupRuleFromForm(e, c.Name, "delete"))
		if err := e.App.Save(rule); err != nil {
			return e.InternalServerError("Failed to save collection rules for "+c.Name, err)
		}
	}

	return e.Redirect(http.StatusSeeOther, "/pbx-setup?msg="+url.QueryEscape("Rules saved"))
}

// setupRuleFromForm reads one rule row from the /pbx-setup rules form.
func setupRuleFromForm(e *core.RequestEvent, collName, ruleType string) views.SetupRule {
	base := "rules_" + collName + "_" + ruleType + "_"
	mode := views.RuleMode(e.Request.FormValue(base + "mode"))
	r := views.SetupRule{Mode: mode}
	if mode == views.RuleModeSelected {
		r.Users = e.Request.Form[base+"user"]
	}
	if mode == views.RuleModeCustom {
		r.Custom = e.Request.FormValue(base + "custom")
	}
	return r
}

// --- Setup record editors (_app, _views, _agent) ---

// handleSetupRecord renders the system record editor (new or edit).
func handleSetupRecord(e *core.RequestEvent) error {
	if err := requireSuperAdmin(e); err != nil {
		return err
	}

	collName := e.Request.PathValue("coll")
	recordID := e.Request.PathValue("id")
	isNew := recordID == "new"
	if isNew {
		recordID = ""
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
	}

	targetColl := ""
	if record != nil {
		targetColl = record.GetString("_collName")
	}

	pageData := views.SetupRecordPageData{
		Theme:       getThemeMode(e.App),
		CollName:    collName,
		RecordID:    recordID,
		IsNew:       isNew,
		Title:       collName,
		Collections: listCollections(e),
	}

	for _, f := range collection.Fields {
		fName := f.GetName()
		if isSystemField(fName) {
			continue
		}
		if f.Type() == "json" {
			pageData.JsonSections = append(pageData.JsonSections, buildJsonSection(e, collection, record, f, targetColl))
			continue
		}
		item := buildFieldItemFor(e, collection, record, f, fName, nil)
		if fName == "_collName" {
			item.Type = "select"
			item.Data["options"] = listCollections(e)
		}
		pageData.Fields = append(pageData.Fields, item)
	}

	e.Response.Header().Set("Content-Type", "text/html; charset=utf-8")
	return templates.ExecuteTemplate(e.Response, "setup-record.html", pageData)
}

// handleSetupRecordPost creates or updates a system record from the editor form.
func handleSetupRecordPost(e *core.RequestEvent) error {
	if err := requireSuperAdmin(e); err != nil {
		return err
	}

	if err := e.Request.ParseMultipartForm(32 << 20); err != nil && !errors.Is(err, http.ErrNotMultipart) {
		return e.BadRequestError("Invalid form data", err)
	}
	if e.Request.Form == nil {
		if err := e.Request.ParseForm(); err != nil {
			return e.BadRequestError("Invalid form data", err)
		}
	}

	collName := e.Request.PathValue("coll")
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

	for _, f := range collection.Fields {
		fName := f.GetName()
		if isSystemField(fName) {
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
		case "json":
			jsonVal, jerr := jsonValueFromSetupForm(e, collection, f, record)
			if jerr != nil {
				return e.BadRequestError("Invalid JSON for field "+fName+": "+jerr.Error(), jerr)
			}
			if jsonVal == "" {
				record.Set(fName, nil)
			} else {
				record.Set(fName, jsonVal)
			}
		case "file":
			if err := applySetupFileField(e, record, f); err != nil {
				return e.BadRequestError("Failed to process file field "+fName+": "+err.Error(), err)
			}
		default:
			record.Set(fName, e.Request.FormValue(fName))
		}
	}

	if err := e.App.Save(record); err != nil {
		return e.InternalServerError("Failed to save record", err)
	}

	return e.Redirect(http.StatusSeeOther, "/pbx-setup")
}

// handleSetupRecordDelete deletes a system record.
func handleSetupRecordDelete(e *core.RequestEvent) error {
	if err := requireSuperAdmin(e); err != nil {
		return err
	}

	collName := e.Request.PathValue("coll")
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

// applySetupFileField handles an uploaded file (or removal) for a record file field.
func applySetupFileField(e *core.RequestEvent, record *core.Record, f core.Field) error {
	fName := f.GetName()

	// explicit remove request
	if e.Request.FormValue(fName+"_remove") == "on" {
		record.Set(fName, "")
		return nil
	}

	mh, err := formFileHeader(e, fName)
	if err != nil {
		return nil // no new file provided, keep existing
	}

	file, err := filesystem.NewFileFromMultipart(mh)
	if err != nil {
		return err
	}

	record.Set(fName, file)
	return nil
}

// formFileHeader returns the multipart FileHeader for the given form field.
func formFileHeader(e *core.RequestEvent, fName string) (*multipart.FileHeader, error) {
	if e.Request.MultipartForm == nil {
		return nil, http.ErrMissingFile
	}
	files := e.Request.MultipartForm.File[fName]
	if len(files) == 0 {
		return nil, http.ErrMissingFile
	}
	return files[0], nil
}

// --- Schema-driven structured JSON editor ---

// buildJsonSection renders the structured editor for one JSON record field.
func buildJsonSection(e *core.RequestEvent, collection *core.Collection, record *core.Record, f core.Field, targetColl string) views.JsonFormSection {
	fName := f.GetName()
	raw := configRaw(record, fName)

	sec := views.JsonFormSection{
		Key:              fName,
		Title:            fName,
		TargetColl:       targetColl,
		TargetCollFields: setupTargetFields(e, targetColl),
		Raw:              raw,
	}

	targetFields := setupTargetFields(e, targetColl)

	switch fName {
	case "_tabulator":
		vc := parseViewTabulator(record)
		sec.Fields = []views.JsonFormField{
			{Key: "pageTitle", Label: "Page title", Type: "text", Value: vc.PageTitle},
			{Key: "collectionDescr", Label: "Collection description", Type: "text", Value: vc.CollectionDescr},
			{Key: "columnTitles", Label: "Column titles (comma-separated)", Type: "text", Value: vc.ColumnTitles},
			{Key: "columnOrder", Label: "Column order", Type: "fieldMulti", FieldOptions: fieldMultiFromCSV(targetFields, vc.ColumnOrder)},
			{Key: "filter", Label: "Filter expression", Type: "text", Value: vc.Filter},
			{Key: "columnSorting", Label: "Column sorting", Type: "bool", Checked: vc.ColumnSorting},
			{Key: "searchBox", Label: "Search box", Type: "bool", Checked: vc.SearchBox},
			{Key: "pagination", Label: "Pagination", Type: "bool", Checked: vc.Pagination},
			{Key: "displaySystemCol", Label: "Display system columns", Type: "bool", Checked: vc.DisplaySystemCol},
			{Key: "columns", Label: "Columns", Type: "columns", FieldOptions: columnsFromConfig(vc.Columns), Options: targetFields},
		}
	case "_form":
		fv := parseViewForm(record)
		sec.Fields = []views.JsonFormField{
			{Key: "formTitle", Label: "Form title", Type: "text", Value: fv.FormTitle},
			{Key: "formDescr", Label: "Form description", Type: "text", Value: fv.FormDescr},
			{Key: "formLabels", Label: "Field labels", Type: "fieldLabels", FieldOptions: fieldLabelsFromConfig(targetFields, fv.FormLabels)},
			{Key: "formLayout", Label: "Form layout", Type: "text", Value: fv.FormLayout},
			{Key: "columnOrder", Label: "Column order", Type: "fieldMulti", FieldOptions: fieldMultiFromCSV(targetFields, fv.ColumnOrder)},
			{Key: "displaySystemCol", Label: "Display system columns", Type: "bool", Checked: fv.DisplaySystemCol},
		}
	case "_mssql":
		mc := parseMssqlConfig(record)
		sec.Fields = []views.JsonFormField{
			{Key: "dsn", Label: "DSN", Type: "text", Value: mc.DSN},
			{Key: "table", Label: "Table", Type: "text", Value: mc.Table},
			{Key: "mode", Label: "Mode", Type: "select", Value: mc.Mode, Options: []string{"insert", "update", "replace"}},
			{Key: "mapping", Label: "Field mapping", Type: "mapping", FieldOptions: mappingFromConfig(mc.Mapping), Options: targetFields},
		}
	case "_config":
		var cfg views.AgentConfig
		if raw != "" {
			_ = json.Unmarshal([]byte(raw), &cfg)
		}
		timeout := strconv.Itoa(cfg.TimeoutSeconds)
		sec.Fields = []views.JsonFormField{
			{Key: "provider", Label: "Provider", Type: "select", Value: cfg.Provider, Options: []string{"openrouter", "lmstudio"}},
			{Key: "baseURL", Label: "Base URL", Type: "text", Value: cfg.BaseURL},
			{Key: "apiKey", Label: "API key", Type: "text", Value: cfg.APIKey},
			{Key: "model", Label: "Model", Type: "text", Value: cfg.Model},
			{Key: "timeoutSeconds", Label: "Timeout (s)", Type: "number", Value: timeout},
			{Key: "enabled", Label: "Enabled", Type: "bool", Checked: cfg.Enabled},
		}
	}

	return sec
}

// setupTargetFields returns the non-system field names of a collection (for
// fieldMulti / columns / mapping option lists). Returns nil when the collection
// cannot be resolved (e.g. new record without _collName yet).
func setupTargetFields(e *core.RequestEvent, collName string) []string {
	if collName == "" {
		return nil
	}
	coll, err := e.App.FindCachedCollectionByNameOrId(collName)
	if err != nil {
		return nil
	}
	out := make([]string, 0, len(coll.Fields))
	for _, f := range coll.Fields {
		if isSystemField(f.GetName()) {
			continue
		}
		out = append(out, f.GetName())
	}
	return out
}

// fieldMultiFromCSV converts a comma-delimited 1-based index list into checkbox options.
func fieldMultiFromCSV(fields []string, csv string) []views.FieldOpt {
	sel := map[int]bool{}
	for _, p := range strings.Split(csv, ",") {
		idx, err := strconv.Atoi(strings.TrimSpace(p))
		if err == nil {
			sel[idx] = true
		}
	}
	opts := make([]views.FieldOpt, 0, len(fields))
	for i, name := range fields {
		opts = append(opts, views.FieldOpt{Index: i + 1, Name: name, Checked: sel[i+1]})
	}
	return opts
}

// fieldLabelsFromConfig converts formLabels (field=Label pairs) into per-field label inputs.
func fieldLabelsFromConfig(fields []string, formLabels string) []views.FieldOpt {
	labels := buildFormLabels(formLabels, nil)
	opts := make([]views.FieldOpt, 0, len(fields))
	for i, name := range fields {
		opts = append(opts, views.FieldOpt{Index: i + 1, Name: name, Value: labels[name]})
	}
	return opts
}

// columnsFromConfig converts the columns JSON list into dynamic column rows.
func columnsFromConfig(cols []views.ListColumn) []views.FieldOpt {
	opts := make([]views.FieldOpt, 0, len(cols))
	for i, c := range cols {
		opts = append(opts, views.FieldOpt{Index: i + 1, Name: c.Field, Label: c.Title})
	}
	return opts
}

// mappingFromConfig converts the _mssql mapping into dynamic mapping rows.
func mappingFromConfig(mapping []views.MssqlMapping) []views.FieldOpt {
	opts := make([]views.FieldOpt, 0, len(mapping))
	for i, m := range mapping {
		opts = append(opts, views.FieldOpt{Index: i + 1, Name: m.PBField, Label: m.DBField})
	}
	return opts
}

// jsonValueFromSetupForm reconstructs the JSON string for a record JSON field
// from the structured form inputs, preserving keys not covered by the schema.
func jsonValueFromSetupForm(e *core.RequestEvent, collection *core.Collection, f core.Field, record *core.Record) (string, error) {
	fName := f.GetName()
	prefix := "json_" + fName + "_"

	targetColl := e.Request.FormValue("_collName")

	existing := map[string]any{}
	if raw := configRaw(record, fName); raw != "" {
		_ = json.Unmarshal([]byte(raw), &existing)
	}

	sec := buildJsonSection(e, collection, record, f, targetColl)

	if len(sec.Fields) == 0 {
		raw := e.Request.FormValue(prefix + "raw")
		if strings.TrimSpace(raw) == "" {
			return "", nil
		}
		var probe any
		if err := json.Unmarshal([]byte(raw), &probe); err != nil {
			return "", err
		}
		return raw, nil
	}

for _, ff := range sec.Fields {
			key := prefix + ff.Key
			switch ff.Type {
			case "text", "select":
				v := e.Request.FormValue(key)
				if v == "" {
					delete(existing, ff.Key)
				} else {
					existing[ff.Key] = v
				}
		case "number":
			v := e.Request.FormValue(key)
			if v == "" {
				delete(existing, ff.Key)
			} else if n, err := strconv.Atoi(v); err == nil {
				existing[ff.Key] = n
			} else {
				return "", fmt.Errorf("%s: invalid number %q", ff.Key, v)
			}
		case "bool":
			existing[ff.Key] = e.Request.FormValue(key) == "on"
		case "fieldMulti":
			indices := e.Request.Form[key]
			csv := ""
			if len(indices) > 0 {
				ints := make([]int, 0, len(indices))
				for _, ix := range indices {
					if n, err := strconv.Atoi(ix); err == nil {
						ints = append(ints, n)
					}
				}
				sort.Ints(ints)
				parts := make([]string, 0, len(ints))
				for _, n := range ints {
					parts = append(parts, strconv.Itoa(n))
				}
				csv = strings.Join(parts, ",")
			}
			if csv == "" {
				delete(existing, ff.Key)
			} else {
				existing[ff.Key] = csv
			}
		case "fieldLabels":
			labels := map[string]string{}
			for _, opt := range ff.FieldOptions {
				v := e.Request.FormValue(key + "_" + strconv.Itoa(opt.Index))
				if v != "" {
					labels[opt.Name] = v
				}
			}
			if len(labels) == 0 {
				delete(existing, ff.Key)
			} else {
				pairs := make([]string, 0, len(labels))
				// stable order by field name
				names := make([]string, 0, len(labels))
				for name := range labels {
					names = append(names, name)
				}
				sort.Strings(names)
				for _, name := range names {
					pairs = append(pairs, name+"="+labels[name])
				}
				existing[ff.Key] = strings.Join(pairs, ",")
			}
		case "columns":
			cols := make([]views.ListColumn, 0)
			names := e.Request.Form[key+"_field"]
			titles := e.Request.Form[key+"_title"]
			for i, name := range names {
				title := ""
				if i < len(titles) {
					title = titles[i]
				}
				if name != "" {
					cols = append(cols, views.ListColumn{Field: name, Title: title})
				}
			}
			if len(cols) == 0 {
				delete(existing, ff.Key)
			} else {
				existing[ff.Key] = cols
			}
		case "mapping":
			mapping := make([]views.MssqlMapping, 0)
			pbs := e.Request.Form[key+"_pb"]
			dbs := e.Request.Form[key+"_db"]
			for i, pb := range pbs {
				db := ""
				if i < len(dbs) {
					db = dbs[i]
				}
				if pb != "" && db != "" {
					mapping = append(mapping, views.MssqlMapping{PBField: pb, DBField: db})
				}
			}
			if len(mapping) == 0 {
				delete(existing, ff.Key)
			} else {
				existing[ff.Key] = mapping
			}
		}
	}

	if len(existing) == 0 {
		return "", nil
	}
	data, err := json.Marshal(existing)
	if err != nil {
		return "", err
	}
	return string(data), nil
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

	if !collection.IsView() {
		records = filterListedRecords(e, collection, records)
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

// --- Named (saved) advanced filters ---

// filterUserLabel resolves a _filters owner id to a human readable label
// (email, else name, else the raw id).
func filterUserLabel(e *core.RequestEvent, userID string) string {
	if userID == "" {
		return userID
	}
	rec, err := e.App.FindRecordById("users", userID)
	if err != nil {
		return userID
	}
	for _, k := range []string{"email", "name"} {
		if v := strings.TrimSpace(rec.GetString(k)); v != "" {
			return v
		}
	}
	return userID
}

// requestedAuthUserID returns the cookie-authenticated user id, or "" when the
// caller is not signed in.
func requestedAuthUserID(e *core.RequestEvent) string {
	info, err := authRequestInfo(e)
	if err != nil || info.Auth == nil {
		return ""
	}
	return info.Auth.GetString("id")
}

// isSuperUserFromID reports whether the current request carries a superuser
// auth (used for filter permission checks).
func isSuperUserFromID(e *core.RequestEvent, _ string) bool {
	info, err := authRequestInfo(e)
	if err != nil {
		return false
	}
	return info.HasSuperuserAuth()
}

func handleFiltersList(e *core.RequestEvent) error {
	configName := e.Request.PathValue("configName")

	if _, _, err := resolveFormConfig(e, configName); err != nil {
		return e.NotFoundError("Configuration not found", err)
	}

	userID := requestedAuthUserID(e)
	if userID == "" {
		return e.JSON(http.StatusOK, map[string]any{"filters": []views.SavedFilter{}})
	}

	filter := "_config = {:c}"
	params := dbx.Params{"c": configName}
	if !isSuperUserFromID(e, userID) {
		filter += " && _user = {:u}"
		params["u"] = userID
	}

	records, err := e.App.FindRecordsByFilter("_filters", filter, "-created", 200, 0, nil, params)
	if err != nil {
		return e.InternalServerError("Failed to list filters", err)
	}

	out := make([]views.SavedFilter, 0, len(records))
	for _, rec := range records {
		var def views.FilterDef
		def.Name = rec.GetString("_name")
		if raw := rec.GetString("_def"); raw != "" {
			_ = json.Unmarshal([]byte(raw), &def)
		}
		out = append(out, views.SavedFilter{
			ID:   rec.GetString("id"),
			Name: def.Name,
			User: filterUserLabel(e, rec.GetString("_user")),
			Def:  def,
		})
	}

	return e.JSON(http.StatusOK, map[string]any{"filters": out})
}

func validateFilterDef(def views.FilterDef) error {
	if strings.TrimSpace(def.Name) == "" {
		return fmt.Errorf("filter name is required")
	}
	if len(def.Conditions) == 0 {
		return fmt.Errorf("filter must have at least one condition")
	}
	for _, c := range def.Conditions {
		if strings.TrimSpace(c.Field) == "" {
			return fmt.Errorf("filter condition field is required")
		}
		if strings.TrimSpace(c.Op) == "" {
			return fmt.Errorf("filter condition operator is required")
		}
	}
	if len(def.Chains) > 0 && len(def.Chains) != len(def.Conditions)-1 {
		return fmt.Errorf("filter connectors do not match the number of conditions")
	}
	return nil
}

func handleFilterSave(e *core.RequestEvent) error {
	configName := e.Request.PathValue("configName")

	collName, _, err := resolveFormConfig(e, configName)
	if err != nil {
		return e.NotFoundError("Configuration not found", err)
	}

	userID := requestedAuthUserID(e)
	if userID == "" {
		return e.UnauthorizedError("Sign in required", nil)
	}

	var def views.FilterDef
	if err := json.NewDecoder(e.Request.Body).Decode(&def); err != nil {
		return e.BadRequestError("Invalid filter JSON", err)
	}
	if err := validateFilterDef(def); err != nil {
		return e.BadRequestError("Invalid filter: "+err.Error(), err)
	}

	// upsert by (config, name); non-owners may not overwrite
	exists, ferr := e.App.FindRecordsByFilter("_filters", "_config = {:c} && _name = {:n}", "", 1, 0, nil, dbx.Params{"c": configName, "n": def.Name})
	if ferr != nil {
		return e.InternalServerError("Failed to look up filter", ferr)
	}

	defJSON, jerr := json.Marshal(def)
	if jerr != nil {
		return e.InternalServerError("Failed to encode filter", jerr)
	}

	var rec *core.Record
	if len(exists) > 0 {
		rec = exists[0]
		owner := rec.GetString("_user")
		if owner != userID && !isSuperUserFromID(e, userID) {
			return e.ForbiddenError("You cannot modify another user's filter", nil)
		}
		if owner == "" {
			rec.Set("_user", userID)
		}
	} else {
		coll, cerr := e.App.FindCachedCollectionByNameOrId("_filters")
		if cerr != nil {
			return e.InternalServerError("Filters collection not found", cerr)
		}
		rec = core.NewRecord(coll)
		rec.Set("_name", def.Name)
		rec.Set("_coll", collName)
		rec.Set("_config", configName)
		rec.Set("_user", userID)
	}
	if rec.GetString("_name") == "" {
		rec.Set("_name", def.Name)
	}
	if rec.GetString("_coll") == "" {
		rec.Set("_coll", collName)
	}
	if rec.GetString("_config") == "" {
		rec.Set("_config", configName)
	}
	rec.Set("_def", string(defJSON))

	if serr := e.App.Save(rec); serr != nil {
		return e.InternalServerError("Failed to save filter", serr)
	}
	return e.JSON(http.StatusOK, map[string]any{"ok": true, "id": rec.GetString("id")})
}

func handleFilterDelete(e *core.RequestEvent) error {
	id := e.Request.PathValue("id")

	userID := requestedAuthUserID(e)
	if userID == "" {
		return e.UnauthorizedError("Sign in required", nil)
	}

	rec, err := e.App.FindRecordById("_filters", id)
	if err != nil {
		return e.NotFoundError("Filter not found", nil)
	}
	if rec.GetString("_user") != userID && !isSuperUserFromID(e, userID) {
		return e.NotFoundError("Filter not found", nil)
	}

	if derr := e.App.Delete(rec); derr != nil {
		return e.InternalServerError("Failed to delete filter", derr)
	}
	return e.JSON(http.StatusOK, map[string]any{"ok": true})
}

// --- Form helpers ---

// buildFormLabels parses formLabels string and JSON labels into a unified map.
func buildFormLabels(formLabels string, fcLabels map[string]string) map[string]string {
	labels := map[string]string{}
	if formLabels != "" {
		for _, p := range strings.Split(formLabels, ",") {
			p = strings.TrimSpace(p)
			if kv := strings.SplitN(p, "=", 2); len(kv) == 2 {
				labels[strings.TrimSpace(kv[0])] = strings.TrimSpace(kv[1])
			}
		}
	}
	for k, v := range fcLabels {
		labels[k] = v
	}
	return labels
}

// relationDisplayFields returns the fields of relColl whose values are shown
// as a relation label/option. System fields (id/created/updated) as well as
// nested-relation and file fields are excluded.
func relationDisplayFields(relColl *core.Collection) []core.Field {
	var out []core.Field
	for _, f := range relColl.Fields {
		fName := f.GetName()
		if f.GetSystem() {
			continue
		}
		if fName == "id" || fName == "created" || fName == "updated" {
			continue
		}
		t := f.Type()
		if t == "relation" || t == "file" {
			continue
		}
		out = append(out, f)
	}
	return out
}

// relationRecordLabel joins the non-empty display-field values of a related
// record with " | ".
func relationRecordLabel(rec *core.Record, fields []core.Field) string {
	vals := make([]string, 0, len(fields))
	for _, f := range fields {
		v := strings.TrimSpace(rec.GetString(f.GetName()))
		if v != "" {
			vals = append(vals, v)
		}
	}
	return strings.Join(vals, " | ")
}

// relatedRecordsDisplay builds a "|"-delimited summary of the related records
// referenced by the given ids in relColl. The returned count is the number of
// related records visible to the caller (listRule enforced).
func relatedRecordsDisplay(e *core.RequestEvent, relColl *core.Collection, ids []string) (string, int) {
	records := make([]*core.Record, 0, len(ids))
	for _, id := range ids {
		if id == "" {
			continue
		}
		rec, err := e.App.FindRecordById(relColl.Id, id)
		if err != nil {
			continue
		}
		records = append(records, rec)
	}
	records = filterListedRecords(e, relColl, records)

	fields := relationDisplayFields(relColl)
	if len(records) == 0 {
		return "", len(records)
	}

	parts := make([]string, 0, len(records))
	for _, rec := range records {
		if label := relationRecordLabel(rec, fields); label != "" {
			parts = append(parts, label)
		}
	}
	return strings.Join(parts, " | "), len(records)
}

// buildRelationOptions returns the selectable records of the related
// collection as id/label options (sorted by label). The "sel" flag marks the
// ids selected in the current record.
func buildRelationOptions(e *core.RequestEvent, relColl *core.Collection, selected []string) []map[string]any {
	records, err := e.App.FindAllRecords(relColl.Name)
	if err != nil {
		return nil
	}
	records = filterListedRecords(e, relColl, records)

	sel := make(map[string]bool, len(selected))
	for _, id := range selected {
		sel[id] = true
	}

	fields := relationDisplayFields(relColl)
	opts := make([]map[string]any, 0, len(records))
	for _, rec := range records {
		id := rec.GetString("id")
		label := relationRecordLabel(rec, fields)
		if label == "" {
			label = id
		}
		opts = append(opts, map[string]any{"id": id, "label": label, "sel": sel[id]})
	}
	sort.SliceStable(opts, func(i, j int) bool {
		return opts[i]["label"].(string) < opts[j]["label"].(string)
	})
	return opts
}

// formatJSONValue decodes a JSON field value and returns a pretty-printed
// representation. It also unwraps JSON-string-wrapped JSON (double-encoded
// payloads); if even the inner content is malformed it returns the unwrapped
// text so newlines render instead of raw escape sequences.
func formatJSONValue(raw string) string {
	var v any
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		return raw
	}
	if s, ok := v.(string); ok {
		trim := strings.TrimSpace(s)
		if strings.HasPrefix(trim, "{") || strings.HasPrefix(trim, "[") {
			if json.Unmarshal([]byte(trim), &v) != nil {
				return s
			}
		}
	}
	pretty, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return raw
	}
	return string(pretty)
}

// buildFieldItemFor creates a FormFieldItem for a field on a given record.
func buildFieldItemFor(e *core.RequestEvent, collection *core.Collection, record *core.Record, f core.Field, fName string, labelsOverride map[string]string) views.FormFieldItem {
	label := fName
	if l, ok := labelsOverride[fName]; ok {
		label = l
	}
	item := views.FormFieldItem{
		Name:  fName,
		Label: label,
		Type:  f.Type(),
		Data:  map[string]any{},
	}
	if f.Type() == "relation" {
		item.Data["collectionName"] = ""
		var ids []string
		if record != nil {
			if raw := record.GetString(fName); raw != "" {
				ids = strings.Split(raw, ",")
			}
		}
		item.Data["ids"] = ids
		item.Data["count"] = len(ids)
		if rf, ok := f.(*core.RelationField); ok {
			if relColl, rerr := e.App.FindCachedCollectionByNameOrId(rf.CollectionId); rerr == nil {
				item.Data["collectionName"] = relColl.Name
				if record != nil {
					display, n := relatedRecordsDisplay(e, relColl, ids)
					item.Data["display"] = display
					item.Data["count"] = n
					item.Value = display
				}
				item.Data["options"] = buildRelationOptions(e, relColl, ids)
			}
		}
		return item
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
	case "json":
		jv := record.GetString(fName)
		if jv != "" && jv != "{}" && jv != "[]" && jv != "null" {
			item.Value = formatJSONValue(jv)
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

// --- View editing ---

// viewEditSection holds editable field info for one base collection within a view.
type viewEditSection struct {
	BaseCollName string   // name of the base collection
	EditableCols []string // non-system, non-join fields present in the view
	JoinField    string   // field on this base collection that links to the main (parent→child)
	MainRefField string   // field on the main base collection that links here (child→parent)
}

// viewEditModel describes how to edit a view collection.
type viewEditModel struct {
	MainCollName string
	Sections     []viewEditSection
}

// buildViewEditModel resolves which base collections to edit for a view.
// Uses config override from _form.config.collections, or falls back to query parsing.
func buildViewEditModel(app core.App, viewColl *core.Collection, fc views.FormConfigJSON, labelsOverride map[string]string) (*viewEditModel, error) {
	if !viewColl.IsView() {
		return nil, fmt.Errorf("collection %s is not a view", viewColl.Name)
	}

	viewQI := parseViewQuery(viewColl.ViewQuery)
	if len(viewQI.Columns) == 0 {
		return nil, fmt.Errorf("failed to parse view query for %s", viewColl.Name)
	}

	// resolve main (FROM) collection
	mainFrom := viewQI.From.Name
	mainColl, err := app.FindCachedCollectionByNameOrId(mainFrom)
	if err != nil {
		return nil, fmt.Errorf("base collection %q not found", mainFrom)
	}
	if mainColl.IsView() {
		return nil, fmt.Errorf("base collection %q is itself a view", mainFrom)
	}

	model := &viewEditModel{MainCollName: mainColl.Name}

	if len(fc.Collections) > 0 {
		// config override: each collection entry defines a section
		for _, cr := range fc.Collections {
			coll, cerr := app.FindCachedCollectionByNameOrId(cr.Name)
			if cerr != nil {
				continue
			}
			if coll.IsView() {
				continue
			}
			sec := viewEditSection{
				BaseCollName: coll.Name,
				JoinField:    cr.JoinField,
			}
			// collect editable fields: fields present in view columns that belong to this base
			for _, col := range viewQI.Columns {
				if strings.EqualFold(col.Table, cr.Name) || col.Table == "" && coll.Name == mainColl.Name {
					fName := col.Field
					if fName == "*" {
						continue
					}
					if !isSystemField(fName) && !strings.EqualFold(fName, cr.JoinField) {
						sec.EditableCols = append(sec.EditableCols, fName)
					}
				}
			}
			if len(sec.EditableCols) > 0 {
				model.Sections = append(model.Sections, sec)
			}
		}
		// infer MainRefField for each section by checking which section links back to main
		for i := range model.Sections {
			sec := &model.Sections[i]
			if sec.BaseCollName == mainColl.Name {
				continue // main section, no parent ref needed
			}
			if sec.JoinField != "" {
				// the join field is on this child section; the main's field is inferred from ON condition
				for _, j := range viewQI.Joins {
					if j.On.RightTable == sec.BaseCollName && j.On.RightField == sec.JoinField {
						sec.MainRefField = j.On.LeftField
						break
					}
				}
			}
		}
	} else {
		// auto-infer from parsed query
		mainSec := viewEditSection{BaseCollName: mainColl.Name}
		for _, col := range viewQI.Columns {
			if col.Table == "" || strings.EqualFold(col.Table, mainColl.Name) || strings.EqualFold(col.Table, viewQI.From.Alias) {
				fName := col.Field
				if fName != "*" && !isSystemField(fName) {
					mainSec.EditableCols = append(mainSec.EditableCols, fName)
				}
			}
		}
		model.Sections = append(model.Sections, mainSec)

		for _, j := range viewQI.Joins {
			joinColl, jerr := app.FindCachedCollectionByNameOrId(j.Table.Name)
			if jerr != nil || joinColl.IsView() {
				continue
			}
			sec := viewEditSection{
				BaseCollName: joinColl.Name,
				MainRefField: j.On.LeftField,
				JoinField:    j.On.RightField,
			}
			for _, col := range viewQI.Columns {
				if strings.EqualFold(col.Table, joinColl.Name) || strings.EqualFold(col.Table, j.Table.Alias) {
					fName := col.Field
					if fName != "*" && !isSystemField(fName) && !strings.EqualFold(fName, j.On.RightField) {
						sec.EditableCols = append(sec.EditableCols, fName)
					}
				}
			}
			if len(sec.EditableCols) > 0 {
				model.Sections = append(model.Sections, sec)
			}
		}
	}

	return model, nil
}

func isSystemField(name string) bool {
	return name == "id" || name == "created" || name == "updated"
}

// handleViewForm renders a form for editing a view collection record.
func handleViewForm(e *core.RequestEvent, configName, recordID string, configRec *core.Record) error {
	viewColl, err := e.App.FindCachedCollectionByNameOrId(configRec.GetString("_collName"))
	if err != nil {
		return e.NotFoundError("View collection not found", err)
	}

	fc := parseViewForm(configRec)
	title := viewColl.Name
	if fc.FormTitle != "" {
		title = fc.FormTitle
	}
	description := fc.FormDescr
	displaySystemCol := fc.DisplaySystemCol

	labelsOverride := buildFormLabels(fc.FormLabels, nil)

	model, err := buildViewEditModel(e.App, viewColl, formConfigFromView(fc), labelsOverride)
	if err != nil {
		return e.InternalServerError("Failed to build view model", err)
	}

	// fetch the view record
	var viewRecord *core.Record
	if recordID != "" {
		viewRecord, err = e.App.FindRecordById(viewColl.Name, recordID)
		if err != nil {
			return e.NotFoundError("Record not found", err)
		}
	}

	sections := make([]views.FormSection, 0, len(model.Sections))

	for _, sec := range model.Sections {
		coll, cerr := e.App.FindCachedCollectionByNameOrId(sec.BaseCollName)
		if cerr != nil {
			continue
		}

		var baseRecord *core.Record
		if viewRecord != nil {
			if sec.BaseCollName == model.MainCollName {
				// main record: for single-table views the view ID is the base ID
				if len(model.Sections) == 1 {
					baseRecord, _ = e.App.FindRecordById(sec.BaseCollName, recordID)
				}
			} else if sec.MainRefField != "" && sec.JoinField != "" {
				// find child via parent→child (MainRefField on main = JoinField on child)
				mainID := ""
				if sec.MainRefField == "id" {
					mainID = recordID
				} else if viewRecord.Get(sec.MainRefField) != nil {
					mainID = viewRecord.GetString(sec.MainRefField)
				}
				if mainID != "" {
					recs, rerr := e.App.FindRecordsByFilter(sec.BaseCollName, sec.JoinField+" = {:v}", "", 1, 0, nil, map[string]any{"v": mainID})
					if rerr == nil && len(recs) > 0 {
						baseRecord = recs[0]
					}
				}
			}
		}

		// build rows: one row per editable column
		var rows []views.FormRow
		if displaySystemCol {
			var sysFields []views.FormFieldItem
			for _, f := range coll.Fields {
				if isSystemField(f.GetName()) {
					item := views.FormFieldItem{Name: f.GetName(), Label: f.GetName(), Type: "text", Data: map[string]any{}}
					if baseRecord != nil {
						if f.GetName() == "id" {
							item.Value = baseRecord.GetString("id")
						} else if f.GetName() == "created" || f.GetName() == "updated" {
							item.Type = "autodate"
							if t := baseRecord.GetDateTime(f.GetName()).Time(); !t.IsZero() {
								item.Value = t.Format("2006-01-02 15:04")
							}
						} else {
							item.Value = baseRecord.GetString(f.GetName())
						}
					}
					sysFields = append(sysFields, item)
				}
			}
			if len(sysFields) > 0 {
				sections = append(sections, views.FormSection{
					CollectionName: sec.BaseCollName + " (system)",
					Rows:           []views.FormRow{{Columns: []views.FormColumn{{Fields: sysFields}}}},
				})
			}
		}

		for _, eName := range sec.EditableCols {
			var field core.Field
			for _, f := range coll.Fields {
				if strings.EqualFold(f.GetName(), eName) {
					field = f
					break
				}
			}
			if field == nil {
				continue
			}
			// namespace the field name as collName.fieldName to avoid collisions
			nsName := sec.BaseCollName + "." + eName
			nsLabels := map[string]string{nsName: labelsOverride[eName]}
			if labelsOverride[eName] == "" {
				nsLabels[nsName] = eName
			}
			item := buildFieldItemFor(e, coll, baseRecord, field, nsName, nsLabels)
			item.Name = nsName
			rows = append(rows, views.FormRow{
				Columns: []views.FormColumn{{Fields: []views.FormFieldItem{item}}},
			})
		}
		if len(rows) > 0 {
			sections = append(sections, views.FormSection{
				CollectionName: sec.BaseCollName,
				Rows:           rows,
			})
		}
	}

	data := views.FormPageData{
		Theme:          getThemeMode(e.App),
		ConfigName:     configName,
		CollectionName: viewColl.Name,
		ID:             recordID,
		Title:          title,
		Description:    description,
		Sections:       sections,
		HasConfig:      true,
		ViewOnly:       e.Request.URL.Query().Get("view") == "1",
	}

	e.Response.Header().Set("Content-Type", "text/html; charset=utf-8")
	return templates.ExecuteTemplate(e.Response, "form.html", data)
}

// handleViewFormPost saves a view form submission across multiple base collections.
func handleViewFormPost(e *core.RequestEvent, configName, recordID string, configRec *core.Record) error {
	viewColl, err := e.App.FindCachedCollectionByNameOrId(configRec.GetString("_collName"))
	if err != nil {
		return e.NotFoundError("View collection not found", err)
	}

	fc := parseViewForm(configRec)
	labelsOverride := buildFormLabels(fc.FormLabels, nil)

	model, err := buildViewEditModel(e.App, viewColl, formConfigFromView(fc), labelsOverride)
	if err != nil {
		return e.InternalServerError("Failed to build view model", err)
	}

	// collect per-base-collection form values
	type baseUpdate struct {
		collName string
		record   *core.Record
		isNew    bool
	}
	var updates []baseUpdate
	mainUpdate := -1

	for _, sec := range model.Sections {
		coll, cerr := e.App.FindCachedCollectionByNameOrId(sec.BaseCollName)
		if cerr != nil {
			continue
		}

		var baseRecord *core.Record
		isNew := false

		if sec.BaseCollName == model.MainCollName && recordID != "" {
			baseRecord, _ = e.App.FindRecordById(sec.BaseCollName, recordID)
		}
		if baseRecord == nil {
			baseRecord = core.NewRecord(coll)
			isNew = true
		}

		// set field values from namespaced form input
		for _, eName := range sec.EditableCols {
			nsName := sec.BaseCollName + "." + eName
			val := e.Request.FormValue(nsName)
			var field core.Field
			for _, f := range coll.Fields {
				if strings.EqualFold(f.GetName(), eName) {
					field = f
					break
				}
			}
			if field == nil {
				continue
			}
			switch field.Type() {
			case "bool":
				baseRecord.Set(eName, val == "on")
			case "number":
				if val == "" {
					baseRecord.Set(eName, nil)
				} else {
					baseRecord.Set(eName, val)
				}
			default:
				baseRecord.Set(eName, val)
			}
		}
		updates = append(updates, baseUpdate{collName: sec.BaseCollName, record: baseRecord, isNew: isNew})
		if sec.BaseCollName == model.MainCollName {
			mainUpdate = len(updates) - 1
		}
	}

	// for new child sections, set the join field to point to the main record's ID
	for i, u := range updates {
		if u.isNew && u.collName != model.MainCollName {
			for _, sec := range model.Sections {
				if sec.BaseCollName == u.collName && sec.JoinField != "" && mainUpdate >= 0 {
					updates[i].record.Set(sec.JoinField, updates[mainUpdate].record.GetString("id"))
				}
			}
		}
	}

	// save all in a transaction
	if err := e.App.RunInTransaction(func(txApp core.App) error {
		for i := range updates {
			if err := txApp.Save(updates[i].record); err != nil {
				return fmt.Errorf("failed to save %s: %w", updates[i].collName, err)
			}
		}
		return nil
	}); err != nil {
		return e.InternalServerError("Failed to save record", err)
	}

	msg := "Record successfully added."
	if recordID != "" {
		msg = "Record successfully updated."
	}
	return e.Redirect(http.StatusSeeOther, "/form/"+configName+"?msg="+url.QueryEscape(msg))
}

// handleViewDelete deletes the main base record for a view collection.
func handleViewDelete(e *core.RequestEvent, recordID string, configRec *core.Record) error {
	viewColl, err := e.App.FindCachedCollectionByNameOrId(configRec.GetString("_collName"))
	if err != nil {
		return e.NotFoundError("View collection not found", err)
	}
	if !viewColl.IsView() {
		return e.BadRequestError("Not a view collection", nil)
	}

	viewQI := parseViewQuery(viewColl.ViewQuery)
	mainFrom := viewQI.From.Name

	record, err := e.App.FindRecordById(mainFrom, recordID)
	if err != nil {
		return e.NotFoundError("Record not found", err)
	}
	if err := e.App.Delete(record); err != nil {
		return e.InternalServerError("Failed to delete record", err)
	}
	return e.JSON(http.StatusOK, map[string]any{"ok": true})
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

	if collection.IsView() {
		return handleViewForm(e, configName, recordID, configRec)
	}

	fields := collection.Fields

	var record *core.Record
	if recordID != "" {
		record, err = e.App.FindRecordById(collName, recordID)
		if err != nil {
			return e.NotFoundError("Record not found", err)
		}
		// enforce the collection view rule per record
		if !canAccessRecord(e, record, collection.ViewRule) {
			return e.NotFoundError("Record not found", nil)
		}
	} else if !canViewCollection(e, collection) {
		return e.NotFoundError("Collection not found", nil)
	}

	fv := parseViewForm(configRec)

	title := collName
	if fv.FormTitle != "" {
		title = fv.FormTitle
	}

	description := fv.FormDescr

	displaySystemCol := fv.DisplaySystemCol

	formLayout := fv.FormLayout

	columnOrder := fv.ColumnOrder

	formLabels := fv.FormLabels

	labelsOverride := buildFormLabels(formLabels, fv.Labels)

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
	if len(fv.Layout) > 0 {
		layout = fv.Layout
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
					fieldItems = append(fieldItems, buildFieldItemFor(e, collection, record, f, fName, labelsOverride))
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
			rowFields = append(rowFields, buildFieldItemFor(e, collection, record, s.f, fName, labelsOverride))
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

	collName, configRec, err := resolveFormConfig(e, configName)
	if err != nil {
		return e.NotFoundError("Configuration not found", err)
	}

	collection, err := e.App.FindCachedCollectionByNameOrId(collName)
	if err != nil {
		return e.NotFoundError("Collection not found", err)
	}

	if collection.IsView() {
		return handleViewFormPost(e, configName, recordID, configRec)
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

	// enforce collection rules for create/update
	if recordID != "" {
		if !canAccessRecord(e, record, collection.UpdateRule) {
			return e.ForbiddenError("You are not allowed to update this record", nil)
		}
	} else {
		data := map[string]any{}
		for _, f := range collection.Fields {
			fName := f.GetName()
			if systemCols[fName] {
				continue
			}
			switch f.Type() {
			case "bool":
				data[fName] = e.Request.FormValue(fName) == "on"
			case "number":
				val := e.Request.FormValue(fName)
				if val == "" {
					data[fName] = nil
				} else {
					data[fName] = val
				}
			default:
				data[fName] = e.Request.FormValue(fName)
			}
		}
		if err := checkCreateRule(e, collection, data); err != nil {
			return e.ForbiddenError("You are not allowed to create records: "+err.Error(), err)
		}
	}

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

// --- MSSQL Export ---

func handleMssqlExport(e *core.RequestEvent) error {
	collName := e.Request.PathValue("collectionName")
	dsn := e.Request.FormValue("dsn")
	table := e.Request.FormValue("table")
	mode := e.Request.FormValue("mode")

	if dsn == "" || table == "" {
		return e.BadRequestError("DSN and table are required", nil)
	}

	if mode == "" {
		mode = "insert"
	}

	var mapping []struct {
		PBField string `json:"pbField"`
		DBField string `json:"dbField"`
	}
	if raw := e.Request.FormValue("mapping"); raw != "" {
		_ = json.Unmarshal([]byte(raw), &mapping)
	}

	if err := pbmssql.ExportToMSSQL(e.App, collName, dsn, table, mode, mapping); err != nil {
		var missing *pbmssql.ErrTableMissing
		if errors.As(err, &missing) {
			// The target table does not exist yet. Ask the user for
			// confirmation before creating it; abort unless confirmed.
			if e.Request.FormValue("createTable") == "1" {
				if cerr := pbmssql.CreateTable(e.App, collName, dsn, table, mapping); cerr != nil {
					return e.InternalServerError("MSSQL table creation failed", cerr)
				}
				if err := pbmssql.ExportToMSSQL(e.App, collName, dsn, table, mode, mapping); err != nil {
					return e.InternalServerError("MSSQL export failed", err)
				}
			} else {
				return e.JSON(http.StatusConflict, map[string]any{
					"ok":          false,
					"tableMissing": true,
					"table":       missing.Table,
					"message":     "Table " + missing.Table + " does not exist on the MSSQL server.",
				})
			}
		} else {
			return e.InternalServerError("MSSQL export failed", err)
		}
	}

	return e.JSON(http.StatusOK, map[string]any{"ok": true, "message": "Export successful"})
}

// --- MSSQL Import ---

func handleMssqlImport(e *core.RequestEvent) error {
	collName := e.Request.PathValue("collectionName")
	dsn := e.Request.FormValue("dsn")
	table := e.Request.FormValue("table")
	mode := e.Request.FormValue("mode")

	if dsn == "" || table == "" {
		return e.BadRequestError("DSN and table are required", nil)
	}

	if mode == "" {
		mode = "insert"
	}

	var mapping []struct {
		PBField string `json:"pbField"`
		DBField string `json:"dbField"`
	}
	if raw := e.Request.FormValue("mapping"); raw != "" {
		_ = json.Unmarshal([]byte(raw), &mapping)
	}

	if err := pbmssql.ImportFromMSSQL(e.App, collName, dsn, table, mode, mapping); err != nil {
		return e.InternalServerError("MSSQL import failed", err)
	}

	return e.JSON(http.StatusOK, map[string]any{"ok": true, "message": "Import successful"})
}

// --- MSSQL Introspect ---

func handleMssqlIntrospect(e *core.RequestEvent) error {
	dsn := e.Request.URL.Query().Get("dsn")
	table := e.Request.URL.Query().Get("table")

	if dsn == "" || table == "" {
		return e.BadRequestError("DSN and table are required", nil)
	}

	columns, err := pbmssql.IntrospectTable(dsn, table)
	if err != nil {
		return e.InternalServerError("Introspect failed", err)
	}

	return e.JSON(http.StatusOK, map[string]any{"columns": columns})
}

// --- AI agent ---

// handleAgent renders the /ai chat page for a signed-in user.
func handleAgent(e *core.RequestEvent) error {
	cookie, err := e.Request.Cookie("pb_auth")
	if err != nil {
		return e.Redirect(http.StatusSeeOther, "/login")
	}
	record, err := e.App.FindAuthRecordByToken(cookie.Value, core.TokenTypeAuth)
	if err != nil || record == nil {
		return e.Redirect(http.StatusSeeOther, "/login")
	}

	cfg := getAgentConfig(e)
	status := "Agent is not configured (set it up in /pbx-setup → AI agent)."
	if cfg.Enabled && cfg.BaseURL != "" && cfg.Model != "" {
		status = "Ready — " + cfg.Provider + " / " + cfg.Model
	}

	data := views.AgentPageData{
		Theme:   getThemeMode(e.App),
		Name:    record.GetString("name"),
		Config:  cfg,
		IsSuper: record.Collection().Name == "_superusers",
		Status:  status,
	}
	e.Response.Header().Set("Content-Type", "text/html; charset=utf-8")
	return templates.ExecuteTemplate(e.Response, "agent.html", data)
}

// handleAgentChat processes a chat message and runs the agent loop.
func handleAgentChat(e *core.RequestEvent) error {
	var req struct {
		Messages []pbai.ChatMessage `json:"messages"`
		File     *pbai.FileInput    `json:"file,omitempty"`
	}
	if err := json.NewDecoder(e.Request.Body).Decode(&req); err != nil {
		return e.BadRequestError("Invalid request body", err)
	}

	cfg := getAgentConfig(e)
	if !cfg.Enabled || cfg.BaseURL == "" || cfg.Model == "" {
		return e.BadRequestError("The AI agent is not configured. Ask a superuser to set it up in /pbx-setup → AI agent.", nil)
	}

	info, err := agentRequestInfo(e)
	if err != nil {
		return e.InternalServerError("Failed to resolve request context", err)
	}

	agent := pbai.NewAgent(e.App, info, cfg)
	result, err := agent.Run(e.Request.Context(), req.Messages, req.File)
	if err != nil {
		return e.InternalServerError("Agent failed: "+err.Error(), err)
	}

	return e.JSON(http.StatusOK, result)
}

// handleAgentConfirm executes or rejects a pending agent write action.
func handleAgentConfirm(e *core.RequestEvent) error {
	var req struct {
		ActionID string `json:"actionID"`
		Approved bool   `json:"approved"`
	}
	if err := json.NewDecoder(e.Request.Body).Decode(&req); err != nil {
		return e.BadRequestError("Invalid request body", err)
	}
	if req.ActionID == "" {
		return e.BadRequestError("Missing actionID", nil)
	}

	cfg := getAgentConfig(e)
	info, err := agentRequestInfo(e)
	if err != nil {
		return e.InternalServerError("Failed to resolve request context", err)
	}

	agent := pbai.NewAgent(e.App, info, cfg)
	result, err := agent.Confirm(e.Request.Context(), req.ActionID, req.Approved)
	if err != nil {
		return e.InternalServerError("Confirm failed: "+err.Error(), err)
	}
	return e.JSON(http.StatusOK, result)
}

// --- Delete record ---

func handleDeleteRecord(e *core.RequestEvent) error {
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

	if collection.IsView() {
		return handleViewDelete(e, recordID, configRec)
	}

	record, err := e.App.FindRecordById(collName, recordID)
	if err != nil {
		return e.NotFoundError("Record not found", err)
	}

	if !canAccessRecord(e, record, collection.DeleteRule) {
		return e.ForbiddenError("You are not allowed to delete this record", nil)
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

// authRequestInfo builds the RequestInfo for rule enforcement, injecting the
// caller's auth record resolved from the pb_auth cookie (PocketBase's default
// auth middleware only reads the Authorization header, not this app's cookie).
func authRequestInfo(e *core.RequestEvent) (*core.RequestInfo, error) {
	info, err := e.RequestInfo()
	if err != nil {
		return nil, err
	}
	if cookie, cerr := e.Request.Cookie("pb_auth"); cerr == nil {
		if record, rerr := e.App.FindAuthRecordByToken(cookie.Value, core.TokenTypeAuth); rerr == nil && record != nil {
			info.Auth = record
		}
	}
	return info, nil
}

// agentRequestInfo is kept as an alias for the AI agent handlers.
func agentRequestInfo(e *core.RequestEvent) (*core.RequestInfo, error) {
	return authRequestInfo(e)
}

// filterListedRecords applies the collection listRule per record for a
// non-superuser caller. A nil listRule means superusers only, so non-superusers
// get an empty list. Superusers bypass the check.
func filterListedRecords(e *core.RequestEvent, coll *core.Collection, records []*core.Record) []*core.Record {
	info, err := authRequestInfo(e)
	if err != nil {
		return nil
	}
	if info.HasSuperuserAuth() {
		return records
	}
	if coll.ListRule == nil {
		return nil
	}
	out := make([]*core.Record, 0, len(records))
	for _, rec := range records {
		ok, aerr := e.App.CanAccessRecord(rec, info, coll.ListRule)
		if aerr == nil && ok {
			out = append(out, rec)
		}
	}
	return out
}

// canAccessRecord reports whether the caller may access a record under the
// given rule pointer. Superusers always pass; nil rules deny everyone else.
func canAccessRecord(e *core.RequestEvent, rec *core.Record, rule *string) bool {
	info, err := authRequestInfo(e)
	if err != nil {
		return false
	}
	if info.HasSuperuserAuth() {
		return true
	}
	if rule == nil {
		return false
	}
	ok, aerr := e.App.CanAccessRecord(rec, info, rule)
	return aerr == nil && ok
}

// canViewCollection reports whether the caller may access the collection's
// "new record" form (create access). Superusers always pass; a nil createRule
// means superusers only.
func canViewCollection(e *core.RequestEvent, coll *core.Collection) bool {
	info, err := authRequestInfo(e)
	if err != nil {
		return false
	}
	if info.HasSuperuserAuth() {
		return true
	}
	return coll.CreateRule != nil
}

// checkCreateRule enforces the collection createRule for a non-superuser.
// It mirrors pbai.checkCreateRule (kept decoupled from the pbai package).
func checkCreateRule(e *core.RequestEvent, coll *core.Collection, data map[string]any) error {
	info, err := authRequestInfo(e)
	if err != nil {
		return err
	}
	if info.HasSuperuserAuth() {
		return nil
	}
	if coll.CreateRule == nil {
		return fmt.Errorf("only superusers can create records in %q", coll.Name)
	}
	rule := *coll.CreateRule
	if rule == "" {
		return nil
	}

	record := core.NewRecord(coll)
	for k, v := range data {
		record.Set(k, v)
	}
	if record.Id == "" {
		record.Id = "__pb_create__" + security.PseudorandomString(6)
	}
	record.SetVerified(false)

	dummyExport, err := record.DBExport(e.App)
	if err != nil {
		return fmt.Errorf("failed to evaluate create rule: %w", err)
	}
	dummyParams := make(dbx.Params, len(dummyExport))
	selects := make([]string, 0, len(dummyExport))
	for k, v := range dummyExport {
		k = inflector.Columnify(k)
		param := "__pb_create__" + k
		dummyParams[param] = v
		selects = append(selects, "{:"+param+"} AS [["+k+"]]")
	}

	dummyCollection := *coll
	dummyCollection.Id += "__pb_create__" + security.PseudorandomString(6)
	dummyCollection.Name += inflector.Columnify("__pb_create__" + security.PseudorandomString(6))

	withFrom := fmt.Sprintf("WITH {{%s}} as (SELECT %s)", dummyCollection.Name, strings.Join(selects, ","))

	ruleQuery := e.App.ConcurrentDB().Select("(1)").PreFragment(withFrom).From(dummyCollection.Name).AndBind(dummyParams)
	resolver := core.NewRecordFieldResolver(e.App, &dummyCollection, info, true)
	expr, err := search.FilterData(rule).BuildExpr(resolver)
	if err != nil {
		return fmt.Errorf("invalid create rule: %w", err)
	}
	ruleQuery.AndWhere(expr)
	if err := resolver.UpdateQuery(ruleQuery); err != nil {
		return fmt.Errorf("failed to evaluate create rule: %w", err)
	}

	var exists int
	if err := ruleQuery.Limit(1).Row(&exists); err != nil || exists == 0 {
		return fmt.Errorf("the create rule for %q forbids this record", coll.Name)
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

	if recs, err := e.App.FindAllRecords("_views"); err == nil {
		for _, rec := range recs {
			vc := parseViewTabulator(rec)
			fv := parseViewForm(rec)
			title := vc.PageTitle
			if title == "" {
				title = fv.FormTitle
			}
			pageData.ListConfigs = append(pageData.ListConfigs, views.ConfigEntry{
				Type:      "view",
				Name:      rec.GetString("_name"),
				CollName:  rec.GetString("_collName"),
				Title:     title,
				HasConfig: true,
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

	name := e.Request.PathValue("name")
	isNew := name == "new"

	pageData := views.ConfigEditorPageData{
		Theme:       getThemeMode(e.App),
		Collections: listCollections(e),
		IsNew:       isNew,
	}

	if !isNew {
		rec := findConfigByAttr(e, "_views", "_name", name)
		if rec == nil {
			return e.NotFoundError("Configuration not found", nil)
		}
		pageData.Name = rec.GetString("_name")
		pageData.CollName = rec.GetString("_collName")
		pageData.TabulatorJSON = configRaw(rec, "_tabulator")
		pageData.FormJSON = configRaw(rec, "_form")
		pageData.MssqlJSON = rec.GetString("_mssql")
	}

	e.Response.Header().Set("Content-Type", "text/html; charset=utf-8")
	return templates.ExecuteTemplate(e.Response, "config.html", pageData)
}

func handlePbxConfigSave(e *core.RequestEvent) error {
	if err := requireSuperAdmin(e); err != nil {
		return err
	}

	name := e.Request.FormValue("name")
	collName := e.Request.FormValue("collName")
	tabulatorJSON := e.Request.FormValue("tabulator")
	formJSON := e.Request.FormValue("form")
	mssqlJSON := e.Request.FormValue("mssql")

	name = strings.TrimSpace(name)
	collName = strings.TrimSpace(collName)
	if name == "" || collName == "" {
		e.Response.Header().Set("Content-Type", "text/html; charset=utf-8")
		return templates.ExecuteTemplate(e.Response, "config.html", views.ConfigEditorPageData{
			Name:          name,
			CollName:      collName,
			TabulatorJSON: tabulatorJSON,
			FormJSON:      formJSON,
			MssqlJSON:     mssqlJSON,
			Collections:   listCollections(e),
			IsNew:         true,
		})
	}

	rec := findConfigByAttr(e, "_views", "_name", name)
	if rec == nil {
		setupCollection, err := e.App.FindCachedCollectionByNameOrId("_views")
		if err != nil {
			return e.InternalServerError("Views collection not found", err)
		}
		rec = core.NewRecord(setupCollection)
	}

	rec.Set("_name", name)
	rec.Set("_collName", collName)
	rec.Set("_tabulator", tabulatorJSON)
	rec.Set("_form", formJSON)
	rec.Set("_mssql", mssqlJSON)

	if err := e.App.Save(rec); err != nil {
		return e.InternalServerError("Failed to save configuration", err)
	}

	return e.Redirect(http.StatusSeeOther, "/pbx-config")
}

func handlePbxConfigDelete(e *core.RequestEvent) error {
	if err := requireSuperAdmin(e); err != nil {
		return err
	}

	name := e.Request.FormValue("name")

	rec := findConfigByAttr(e, "_views", "_name", name)
	if rec != nil {
		if err := e.App.Delete(rec); err != nil {
			return e.InternalServerError("Failed to delete configuration", err)
		}
	}

	return e.Redirect(http.StatusSeeOther, "/pbx-config")
}

// --- Import wizard (create collection from Excel / MSSQL) ---

func renderImportWizard(e *core.RequestEvent, data views.ImportWizardPageData) error {
	e.Response.Header().Set("Content-Type", "text/html; charset=utf-8")
	return templates.ExecuteTemplate(e.Response, "import-wizard.html", data)
}

func handleImportWizard(e *core.RequestEvent, source string) error {
	if err := requireSuperAdmin(e); err != nil {
		return err
	}

	action := e.Request.FormValue("action")
	step := 1
	page := views.ImportWizardPageData{
		Theme:  getThemeMode(e.App),
		Source: source,
	}

	switch action {
	case "preview":
		if source == "excel" {
			fileName := strings.TrimSpace(e.Request.FormValue("fileName"))
			sheet := strings.TrimSpace(e.Request.FormValue("sheet"))
			page.FileName = fileName
			page.Sheet = sheet
			page.Name = strings.TrimSpace(e.Request.FormValue("name"))
			page.Import = e.Request.FormValue("import") == "1"

			if page.Name == "" {
				page.Message = "Collection name is required."
				return renderImportWizard(e, page)
			}
			cols, err := pbexcel.IntrospectSheet(fileName, sheet)
			if err != nil {
				page.Message = "Failed to read Excel: " + err.Error()
				return renderImportWizard(e, page)
			}
			page.Columns = wizardColumnsToDetected(cols)
			if len(page.Columns) == 0 {
				page.Message = "No usable columns detected in sheet " + sheet
				return renderImportWizard(e, page)
			}
			step = 2
		} else {
			dsn := strings.TrimSpace(e.Request.FormValue("dsn"))
			table := strings.TrimSpace(e.Request.FormValue("table"))
			page.DSN = dsn
			page.Table = table
			page.Name = strings.TrimSpace(e.Request.FormValue("name"))
			page.Import = e.Request.FormValue("import") == "1"

			if page.Name == "" {
				page.Message = "Collection name is required."
				return renderImportWizard(e, page)
			}
			cols, err := pbmssql.IntrospectTable(dsn, table)
			if err != nil {
				page.Message = "Failed to introspect MSSQL table: " + err.Error()
				return renderImportWizard(e, page)
			}
			if len(cols) == 0 {
				page.Message = "No columns found in table " + table
				return renderImportWizard(e, page)
			}
			for _, c := range cols {
				page.Columns = append(page.Columns, views.WizardColumn{
					Header:  c.Name,
					Field:   c.Name,
					Type:    mssqlTypeToPB(c.DataType),
					Include: true,
				})
			}
			step = 2
		}

	case "create":
		if source == "excel" {
			page.FileName = strings.TrimSpace(e.Request.FormValue("fileName"))
			page.Sheet = strings.TrimSpace(e.Request.FormValue("sheet"))
		} else {
			page.DSN = strings.TrimSpace(e.Request.FormValue("dsn"))
			page.Table = strings.TrimSpace(e.Request.FormValue("table"))
		}
		page.Name = strings.TrimSpace(e.Request.FormValue("name"))
		page.Import = e.Request.FormValue("import") == "1"

		page.Columns = parseWizardColumns(e)
		if page.Name == "" {
			page.Message = "Collection name is required."
			return renderImportWizard(e, page)
		}

		// normalize field/collection names so creation and import stay consistent
		page.Name = sanitizeFieldName(page.Name)
		used := map[string]bool{}
		for _, reserved := range []string{"id", "created", "updated", "collectionid", "collectionname", "expand"} {
			used[reserved] = true
		}
		anyField := false
		for i := range page.Columns {
			if !page.Columns[i].Include {
				continue
			}
			page.Columns[i].Field = sanitizeFieldName(page.Columns[i].Field)
			if page.Columns[i].Field == "" || used[page.Columns[i].Field] {
				page.Columns[i].Include = false
				continue
			}
			used[page.Columns[i].Field] = true
			anyField = true
		}
		if !anyField {
			page.Message = "No usable columns selected."
			return renderImportWizard(e, page)
		}

		created, err := createCollectionFromWizard(e, page)
		if err != nil {
			page.Message = "Failed to create collection: " + err.Error()
			return renderImportWizard(e, page)
		}

		if page.Import {
			if source == "excel" {
				headerMap := map[string]string{}
				for _, c := range page.Columns {
					if c.Include {
						headerMap[c.Header] = c.Field
					}
				}
				if ierr := pbexcel.ImportFromExcel(e.App, page.FileName, page.Sheet, page.Name, "insert", headerMap); ierr != nil {
					page.Message = "Collection created, but data import failed: " + ierr.Error()
					page.Created = created
					return renderImportWizard(e, page)
				}
			} else {
				var mapping []struct {
					PBField string `json:"pbField"`
					DBField string `json:"dbField"`
				}
				for _, c := range page.Columns {
					if c.Include {
						mapping = append(mapping, struct {
							PBField string `json:"pbField"`
							DBField string `json:"dbField"`
						}{PBField: c.Field, DBField: c.Header})
					}
				}
				if ierr := pbmssql.ImportFromMSSQL(e.App, page.Name, page.DSN, page.Table, "insert", mapping); ierr != nil {
					page.Message = "Collection created, but data import failed: " + ierr.Error()
					page.Created = created
					return renderImportWizard(e, page)
				}
			}
		}

		page.Created = created
		step = 3
	}

	page.Step = step
	return renderImportWizard(e, page)
}

func wizardColumnsToDetected(cols []pbexcel.DetectedColumn) []views.WizardColumn {
	result := make([]views.WizardColumn, 0, len(cols))
	for _, c := range cols {
		result = append(result, views.WizardColumn{
			Header:  c.Name,
			Field:   c.Name,
			Type:    c.Type,
			Include: true,
			Values:  strings.Join(c.Values, ", "),
		})
	}
	return result
}

func parseWizardColumns(e *core.RequestEvent) []views.WizardColumn {
	countStr := e.Request.FormValue("colCount")
	count, err := strconv.Atoi(countStr)
	if err != nil || count > 200 {
		return nil
	}

	var cols []views.WizardColumn
	for i := 0; i < count; i++ {
		header := strings.TrimSpace(e.Request.FormValue(fmt.Sprintf("col_%d_header", i)))
		field := strings.TrimSpace(e.Request.FormValue(fmt.Sprintf("col_%d_field", i)))
		typ := strings.TrimSpace(e.Request.FormValue(fmt.Sprintf("col_%d_type", i)))
		include := e.Request.FormValue(fmt.Sprintf("col_%d_include", i)) == "1"
		values := strings.TrimSpace(e.Request.FormValue(fmt.Sprintf("col_%d_values", i)))
		if header == "" && field == "" {
			continue
		}
		if field == "" {
			field = header
		}
		if typ == "" {
			typ = "text"
		}
		cols = append(cols, views.WizardColumn{
			Header:  header,
			Field:   field,
			Type:    typ,
			Include: include,
			Values:  values,
		})
	}
	return cols
}

// sanitizeFieldName converts a header into a valid PocketBase field name.
func sanitizeFieldName(name string) string {
	name = strings.TrimSpace(strings.ToLower(name))
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	s := b.String()
	if s == "" || (s[0] >= '0' && s[0] <= '9') {
		s = "f_" + s
	}
	return s
}

// mssqlTypeToPB maps an MSSQL column data type to a PocketBase field type.
func mssqlTypeToPB(dt string) string {
	t := strings.ToLower(dt)
	switch {
	case strings.Contains(t, "int"), strings.Contains(t, "decimal"), strings.Contains(t, "numeric"),
		strings.Contains(t, "float"), strings.Contains(t, "real"), strings.Contains(t, "money"):
		return "number"
	case t == "bit":
		return "bool"
	case strings.Contains(t, "date"), strings.Contains(t, "time"):
		return "date"
	default:
		return "text"
	}
}

// createCollectionFromWizard builds and saves a new base collection from the
// wizard columns and returns its name.
func createCollectionFromWizard(e *core.RequestEvent, page views.ImportWizardPageData) (string, error) {
	name := sanitizeFieldName(page.Name)

	if existing, _ := e.App.FindCachedCollectionByNameOrId(name); existing != nil {
		return "", fmt.Errorf("collection %q already exists", name)
	}

	// build fields from included columns
	var newFields []core.Field
	used := map[string]bool{}
	for _, reserved := range []string{"id", "created", "updated", "collectionid", "collectionname", "expand"} {
		used[reserved] = true
	}
	for _, c := range page.Columns {
		if !c.Include {
			continue
		}
		fieldName := sanitizeFieldName(c.Field)
		if fieldName == "" || used[fieldName] {
			continue
		}
		used[fieldName] = true

		typ := core.FieldTypeText
		switch c.Type {
		case "number":
			typ = core.FieldTypeNumber
		case "bool":
			typ = core.FieldTypeBool
		case "date", "autodate":
			typ = core.FieldTypeDate
		}

		f := core.Fields[typ]()
		f.SetName(fieldName)
		newFields = append(newFields, f)
	}

	if len(newFields) == 0 {
		return "", fmt.Errorf("no usable columns selected")
	}

	// base collection with system fields already present
	coll := core.NewBaseCollection(name)
	coll.Fields.Add(newFields...)

	if err := e.App.Save(coll); err != nil {
		return "", err
	}
	return name, nil
}
