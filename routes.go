package main

// Route registration helpers. Kept in package main because all handlers and
// helpers (templates, viewsFS, authRequestInfo, ...) live here.
//
// Each register* function adds one logical group of routes to the router,
// keeping the OnServe hook in main.go short.

import (
	"encoding/json"
	"log"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/pocketbase/pocketbase/core"

	"pbx/i18n"
	"pbx/views"
)

// statusWriter captures the HTTP status for request logging.
type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

// requestLogger wraps the PB mux (assigned to Server.Handler after se.Next()
// builds it) and emits one structured log line per request.
func requestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)
		log.Printf("method=%s path=%q status=%d dur=%s remote=%s",
			r.Method, r.URL.Path, sw.status, time.Since(start).Round(time.Microsecond), r.RemoteAddr)
	})
}

// registerAuthRoutes registers the login/logout flow.
func registerAuthRoutes(se *core.ServeEvent) {
	// login dialog
	se.Router.GET("/login", func(e *core.RequestEvent) error {
		e.Response.Header().Set("Content-Type", "text/html; charset=utf-8")
		return templates.ExecuteTemplate(e.Response, "login.html", map[string]string{
			"Theme":     getThemeMode(e.App),
			"Lang":      getLangCode(e.App, e.Request),
			"CSRFToken": generateCSRFToken(e.Request),
		})
	})
	// login form submission
	se.Router.POST("/login", csrfMiddleware(rateLimitMiddleware(loginRateLimiter)(handleLoginPost)))
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
}

// registerAppRoutes registers the desktop + mobile dashboard.
func registerAppRoutes(se *core.ServeEvent) {
	se.Router.GET("/app", func(e *core.RequestEvent) error { return handleApp(e) })
	se.Router.GET("/mobile/app", func(e *core.RequestEvent) error { return handleMobileApp(e) })
}

// registerSetupRoutes registers the super-admin /pbx-setup hub and its record editors.
func registerSetupRoutes(se *core.ServeEvent) {
	se.Router.GET("/pbx-setup", func(e *core.RequestEvent) error { return handlePbxSetup(e) })
	se.Router.GET("/pbx-setup/record/{coll}/new", func(e *core.RequestEvent) error { return handleSetupRecord(e) })
	se.Router.GET("/pbx-setup/record/{coll}/{id}", func(e *core.RequestEvent) error { return handleSetupRecord(e) })
	se.Router.POST("/pbx-setup/record/{coll}", func(e *core.RequestEvent) error { return handleSetupRecordPost(e) })
	se.Router.POST("/pbx-setup/record/{coll}/{id}", func(e *core.RequestEvent) error { return handleSetupRecordPost(e) })
	se.Router.POST("/pbx-setup/record/{coll}/{id}/delete", func(e *core.RequestEvent) error { return handleSetupRecordDelete(e) })
	se.Router.POST("/pbx-setup/rules", func(e *core.RequestEvent) error { return handleSetupRulesPost(e) })
}

// registerConfigRoutes registers the super-admin config editor + import wizard.
func registerConfigRoutes(se *core.ServeEvent) {
	se.Router.GET("/pbx-config", func(e *core.RequestEvent) error { return handlePbxConfig(e) })
	se.Router.GET("/pbx-config/view/new", func(e *core.RequestEvent) error {
		e.Request.SetPathValue("name", "new")
		return handlePbxConfigEditor(e)
	})
	se.Router.GET("/pbx-config/view/{name}", func(e *core.RequestEvent) error { return handlePbxConfigEditor(e) })
	se.Router.POST("/pbx-config/save", func(e *core.RequestEvent) error { return handlePbxConfigSave(e) })
	se.Router.POST("/pbx-config/delete", func(e *core.RequestEvent) error { return handlePbxConfigDelete(e) })

	se.Router.GET("/pbx-config/import-excel", func(e *core.RequestEvent) error { return handleImportWizard(e, "excel") })
	se.Router.POST("/pbx-config/import-excel", func(e *core.RequestEvent) error { return handleImportWizard(e, "excel") })
	se.Router.GET("/pbx-config/import-mssql", func(e *core.RequestEvent) error { return handleImportWizard(e, "mssql") })
	se.Router.POST("/pbx-config/import-mssql", func(e *core.RequestEvent) error { return handleImportWizard(e, "mssql") })
}

// registerDataRoutes registers the tabular/form views (desktop + mobile),
// record deletion, JSON data, saved filters, and Excel/MSSQL sync endpoints.
func registerDataRoutes(se *core.ServeEvent) {
	se.Router.GET("/tabular/{configName}", func(e *core.RequestEvent) error { return handleTabulator(e) })

	se.Router.GET("/form/{configName}", func(e *core.RequestEvent) error { return handleForm(e) })
	se.Router.GET("/form/{configName}/{id}", func(e *core.RequestEvent) error { return handleForm(e) })
	se.Router.POST("/form/{configName}", func(e *core.RequestEvent) error { return handleFormPost(e) })
	se.Router.POST("/form/{configName}/{id}", func(e *core.RequestEvent) error { return handleFormPost(e) })
	se.Router.POST("/form/{configName}/{id}/delete", func(e *core.RequestEvent) error { return handleDeleteRecord(e) })

	se.Router.GET("/mobile/tabular/{configName}", func(e *core.RequestEvent) error { return handleMobileTabulator(e) })
	se.Router.GET("/mobile/form/{configName}", func(e *core.RequestEvent) error { return handleMobileForm(e) })
	se.Router.GET("/mobile/form/{configName}/{id}", func(e *core.RequestEvent) error { return handleMobileForm(e) })
	se.Router.POST("/mobile/form/{configName}", func(e *core.RequestEvent) error { return handleMobileFormPost(e) })
	se.Router.POST("/mobile/form/{configName}/{id}", func(e *core.RequestEvent) error { return handleMobileFormPost(e) })
	se.Router.POST("/mobile/form/{configName}/{id}/delete", func(e *core.RequestEvent) error { return handleDeleteRecord(e) })

	se.Router.GET("/api/tabulator-data/{collectionName}", func(e *core.RequestEvent) error { return handleTabulatorDataJSON(e) })

	se.Router.GET("/api/filters/{configName}", func(e *core.RequestEvent) error { return handleFiltersList(e) })
	se.Router.POST("/api/filters/{configName}", func(e *core.RequestEvent) error { return handleFilterSave(e) })
	se.Router.DELETE("/api/filters/{id}", func(e *core.RequestEvent) error { return handleFilterDelete(e) })

	se.Router.GET("/export/{collectionName}", func(e *core.RequestEvent) error { return handleExport(e) })
	se.Router.POST("/import/{collectionName}", func(e *core.RequestEvent) error { return handleImport(e) })

	se.Router.POST("/mssql-export/{collectionName}", func(e *core.RequestEvent) error { return handleMssqlExport(e) })
	se.Router.POST("/mssql-import/{collectionName}", func(e *core.RequestEvent) error { return handleMssqlImport(e) })
	se.Router.GET("/mssql-introspect", func(e *core.RequestEvent) error { return handleMssqlIntrospect(e) })
}

// registerAssetsAndAPI routes static assets and the small pref/settings APIs.
func registerAssetsAndAPIRoutes(se *core.ServeEvent) {
	se.Router.GET("/assets/{path...}", func(e *core.RequestEvent) error {
		path := e.Request.PathValue("path")
		if path == "" {
			return e.NotFoundError("Missing path", nil)
		}
		path = filepath.Clean(path)
		if strings.HasPrefix(path, "..") || strings.Contains(path, ".."+string(filepath.Separator)) {
			return e.NotFoundError("Invalid path", nil)
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
			// CSS is embedded at build time and iterates on frequently — forbid
			// heuristic browser caching so theme changes always reach the client.
			e.Response.Header().Set("Cache-Control", "no-cache")
		}
		e.Response.Header().Set("Content-Type", ct)
		e.Response.Write(data)
		return nil
	})

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

	se.Router.POST("/api/lang/{code}", func(e *core.RequestEvent) error {
		code := e.Request.PathValue("code")
		code = i18n.Normalize(code)
		if !i18n.IsValid(code) {
			return e.BadRequestError("Unsupported language", nil)
		}
		if err := setLangCode(e.App, code); err != nil {
			return e.InternalServerError("Failed to save language", err)
		}
		return e.JSON(http.StatusOK, map[string]any{"ok": true, "lang": code})
	})

	se.Router.GET("/api/lang/{code}/catalog.js", func(e *core.RequestEvent) error {
		code := e.Request.PathValue("code")
		code = i18n.Normalize(code)
		if !i18n.IsValid(code) {
			code = "en"
		}
		js := "window._tCatalog=" + i18n.CatalogJSON(code) + ";" +
			"window._t=function(key){var c=window._tCatalog;return c&&c[key]?c[key]:key;};" +
			"window._tLang=function(){return document.documentElement.lang||'en';};"
		e.Response.Header().Set("Content-Type", "application/javascript; charset=utf-8")
		e.Response.Write([]byte(js))
		return nil
	})

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
}

// registerAIRoutes registers the AI agent pages, chat endpoints and config.
func registerAIRoutes(se *core.ServeEvent) {
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

	se.Router.GET("/api/ai-config", func(e *core.RequestEvent) error {
		if err := requireSuperAdmin(e); err != nil {
			return err
		}
		return e.JSON(http.StatusOK, getAgentConfig(e))
	})
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

	se.Router.GET("/ai", func(e *core.RequestEvent) error { return handleAgent(e) })
	se.Router.GET("/mobile/ai", func(e *core.RequestEvent) error { return handleMobileAi(e) })
	se.Router.POST("/ai/chat", rateLimitMiddleware(aiChatRateLimiter)(handleAgentChat))
	se.Router.POST("/ai/chat/stream", rateLimitMiddleware(aiStreamRateLimiter)(handleAgentChatStream))
	se.Router.POST("/ai/confirm", func(e *core.RequestEvent) error { return handleAgentConfirm(e) })
}

// registerActionRoutes registers the custom action list/execute endpoints.
func registerActionRoutes(se *core.ServeEvent) {
	se.Router.GET("/api/actions/{collectionName}", func(e *core.RequestEvent) error { return handleActionsList(e) })
	se.Router.POST("/actions/execute", func(e *core.RequestEvent) error { return handleActionExecute(e) })
}
