# PBX Implementation Plan

## Context

Current state: single config per collection (`_tabulator`/`_form` keyed by `collName` via scalar fields), routes `/tabulator/{collectionName}` + `/form/{collectionName}[/{id}]`, Excel exp/imp in `pbexcel`, `_app` dashboard, `/pbx-setup` (superuser landing). View collections exist (`pokus`) but are read-only.

Decisions confirmed with user:
- Extend existing collections (`_tabulator`/`_form` get `_name` + JSON `config`)
- Switch fully to config-name routing (`/tabular/{_name}`, `/form/{_name}`)
- Lua actions and Reports/PDF implemented in separate later phases
- MSSQL via `github.com/microsoft/go-mssqldb`, SQL-auth DSN
- Existing pages/templates keep their current look and structure (only additive changes)

## Phase 1 — Configuration model & config-name routing

**Migrations** (`pb_migrations/`, via JS SDK):
- `_tabulator`: add `_name` (text, required), `config` (json), `_mssql` (json). Backfill existing records: `_name = collName`.
- `_form`: add `_name` (text, required), `config` (json). Backfill `_name = collName`.
- `_app`: add `configName` (text, optional) — explicit config `_name` each dashboard link targets.

**Config JSON schemas:**
- `_tabulator.config` (list): `{ title, description, displaySystemCol, columns:[{field,title,sortable,searchable}], searchBox, pagination, filter, columnOrder }`
- `_form.config` (form): `{ title, description, displaySystemCol, layout:[[cols…]], labels:{field:Label}, collections:[{name,joinField}] }` — `collections` is the override for view-join editing (Phase 3).
- `_tabulator._mssql`: `{ dsn, table, mode, mapping:[{pbField,dbField}] }`

**Routing change** (`main.go`):
- `GET /tabulator/{collectionName}` → `GET /tabular/{configName}`. New resolver `resolveConfig(e, "_tabulator", configName)` → returns collection name + config record. 404 if `_name` unknown.
- `GET /form/{configName}` + `/form/{configName}/{id}` + POST variants → resolve via `_form._name`. Form action URLs and redirect targets use the config name.
- `/app` dashboard: links become `/tabular/{configName}` (from `_app.configName`, else resolve default config for that collection).
- `/api/tabulator-data/{collectionName}`, `/export/{collectionName}`, `/import/{collectionName}` stay collection-based (internal/utility APIs).
- Keep `/pbx-setup` working (renders `_app`/`_tabulator`/`_form` via raw collections).

**`views/pages.go`:** add typed `ListConfig`, `FormConfig`, `MssqlConfig` structs; add `ConfigName` to `TabulatorPageData`/`FormPageData`; `_name` to config structs.

**`views/tabulator.html` / `form.html`:** row action links (`/form/{configName}/{id}`), delete, redirects, Import/Export modals — all use config name. No visual redesign.

## Phase 2 — Super-admin config editor

- **Auth helper** `requireSuperAdmin(e)`: reads `pb_auth` cookie, `FindAuthRecordByToken(token, core.TokenTypeAuth)`, checks `record.Collection().Name == "_superusers"`. Applies to all `/pbx-config*` routes.
- **Routes:**
  - `GET /pbx-config` — landing: list all `_tabulator` + `_form` configs with edit/delete/new links.
  - `GET /pbx-config/list/new` · `GET /pbx-config/list/{name}` — friendly list-config editor.
  - `GET /pbx-config/form/new` · `GET /pbx-config/form/{name}` — friendly form-config editor.
  - `POST /pbx-config/save` — persists `_name`, `collName`, `config` JSON (keeps scalar fields in sync).
  - `POST /pbx-config/delete` — delete a config record.
- **`views/config.html`** — new page (same W3.CSS style): collapsible sections (Header, Columns with checkboxes/title/sort/search, Search/Pagination toggles, Filter text, MSSQL block for `_mssql`). For forms: layout builder (rows/columns of fields) + label overrides.
- `/pbx-setup` stays as-is; gets a link to `/pbx-config`.

## Phase 3 — Editing view-only collections

- **View query parsing** (new `viewsql.go` in main package): lightweight tokenizer/regex parse of `collection.ViewQuery` → extract FROM tables, JOINs + ON conditions, and selected columns (with aliases). Produces `viewField → (baseCollection, baseField)` map and join keys.
- **`handleForm`:** if `collection.Type == core.CollectionTypeView`:
  - Fetch view record; for each base collection, fetch the underlying base record via join key (falls back to matching by id when single-table).
  - Render one form **section per base collection** (its editable fields, excluding the join key/system cols). Reuses `_form.config.collections` overrides when provided.
- **`handleFormPost`:** save each base record in a transaction (`e.App.RunInTransaction`); view then reflects the joined result. Create works for single-table views; for multi-table joins new records are created in each base collection (join key auto-set if configured).
- **`views/form.html`** — additive: per-collection section header when editing a view; no restructuring.

## Phase 4 — MSSQL import/export

- **Dependency:** add `github.com/microsoft/go-mssqldb` to `go.mod` (blank-import driver for `database/sql`).
- **New package `pbmssql/`** (`pb-mssql.go`; delete orphan `pb-mssql.g`):
  - `ExportToMSSQL(app, collName, cfg MssqlConfig) (n int, err error)` — reads PB records, maps fields per `cfg.mapping`, upserts into `cfg.table`.
  - `ImportFromMSSQL(app, collName, cfg MssqlConfig) (n int, err error)` — reads table, type-converts (SQL ↔ PB: number/bool/date/text), upserts with same `insert/update/replace` semantics as `pbexcel`.
  - `IntrospectTable(dsn, table)` — INFORMATION_SCHEMA columns → field/type info (shared with Phase 5).
  - Context timeouts on all DB calls; `sql.DB` reuse via DSN-keyed pool.
- **UI:** MSSQL Import/Export buttons + modal on `tabulator.html` (matches existing Import/Export modal pattern); `_mssql` editor section in `config.html`.

## Phase 5 — Create collection from Excel / MSSQL

- **Routes (super-admin):**
  - `GET/POST /pbx-config/import-excel` — file/sheet picker → introspect header + sample rows → preview schema (names + inferred types: text/number/bool/date) → create collection → optionally import data immediately.
  - `GET/POST /pbx-config/import-mssql` — DSN + table → `IntrospectTable` → preview → create collection → optionally import.
- **Collection creation in Go:** `core.NewBaseCollection(name)`, build fields from detected schema, `app.Save(collection)` (schema auto-created).
- **`pbexcel`:** add `IntrospectSheet(path, sheet)` returning detected columns/types.
- **`views/import-wizard.html`** — new page, shared 3-step wizard (Source → Preview/Edit → Create).

## Phase 6 — Cleanup & docs

- Update `AGENTS.md` (routes, `_name` config semantics, new packages, `pbmssql`) and `PBX spec.md`.
- Remove `pb-mssql.g` orphan file.
- `go build ./... && go vet ./...` after every phase.

## Build order & verification

1. Phase 1 (schema + routing) → build/vet; smoke-test `/tabular/{_name}`, `/form/{_name}`.
2. Phase 2 (config editor) → build/vet; manual editor round-trip.
3. Phase 3 (view editing) → build/vet; test with `pokus` (single-table) then a JOINed view.
4. Phase 4 (MSSQL) → `go mod tidy`, build/vet (needs a reachable MSSQL to test).
5. Phase 5 (collection creation) → build/vet.
6. Phase 6 docs/cleanup.

  ## Phase 7 — Superadmin `/pbx-setup` management hub
### done!

Context: `/pbx-setup` (main.go:947) is currently NOT superadmin-gated and renders `_app`/`_views` as read-only tables whose edit links target `/form/{collName}` — those 404 because `resolveFormConfig` needs a `_views` config named after the collection (none exists for `_app`/`_views`/`_agent`). All data collections have `NULL` rules (superuser-only); `users` uses `id = @request.auth.id`. App routes ignore PB rules; `handleFormPost` has no file-upload support.

Decisions confirmed with user:
- Rules UI = mode select + user checkboxes → generate `@request.auth.id = "id1" || @request.auth.id = "id2"`.
- Rules editor applies to data collections only (exclude `users`, `roles`, `_`-prefixed, `collection.IsView()`).
- Enforce rules also in app routes (`/tabular`, `/form` GET/POST, delete, `/api/tabulator-data`); view collections skipped.
- `_app.group_icon` gets file-upload support (`filesystem.NewFileFromMultipart`).
- Create-rule dummy-record evaluation: duplicate `checkCreateRule` helper in main.go (mirrors `pbai.checkCreateRule`), keep packages decoupled.

### 7.1 Superadmin gate
- `handlePbxSetup` starts with `if err := requireSuperAdmin(e); err != nil { return err }` (mirrors `handlePbxConfig`).

### 7.2 System record editors (`_app`, `_views`, `_agent`)
- New superadmin-gated routes in `main.go`:
  - `GET /pbx-setup/record/{coll}/new` · `GET /pbx-setup/record/{coll}/{id}` → `handleSetupRecord` (renders new template `views/setup-record.html`).
  - `POST /pbx-setup/record/{coll}` (create) · `POST /pbx-setup/record/{coll}/{id}` (update) → `handleSetupRecordPost` — multipart; `_app.group_icon` via `filesystem.NewFileFromMultipart` (keep existing unless replaced; "remove" option).
  - `POST /pbx-setup/record/{coll}/{id}/delete` → `handleSetupRecordDelete`.
- Generic renderer (`SetupRecordPageData` in `views/pages.go`): iterate `collection.Fields`, skip `id/created/updated`; `text` (with `_collName` → `<select>` of all collections), `number`, `bool`, `file`, `json` → structured JSON form (7.3).
- `/pbx-setup` `_app`/`_views` table links switch from `/form/{coll}` to these routes.

### 7.3 Schema-driven structured JSON editor
- New Go types: `JsonFormField{Key,Label,Type,Value,Checked,Options,FieldOptions}` (`Type ∈ text|number|bool|select|fieldMulti|fieldLabels|mapping|columns`, `FieldOpt{Index(1-based),Name,Checked,Value}`), `JsonFormSection{Key,Title,TargetColl,Fields,Raw}`.
- Schema mapping (target collection = record's `_collName`):
  - `columnOrder` → `fieldMulti` — checkbox list of target collection fields → comma-delimited 1-based indices.
  - `columnTitles`, `filter`, `pageTitle`, `formTitle`, `formDescr`, `collectionDescr`, `formLayout`, `dsn`, `table` → text.
  - `columnSorting`, `searchBox`, `pagination`, `displaySystemCol`, `enabled` → bool.
  - `mode` → select (insert/update/replace).
  - `formLabels` → `fieldLabels` per-field label inputs → `field=Label` pairs.
  - `columns` → dynamic rows (field select + title); `_mssql.mapping` → dynamic rows (pbField select + dbField text).
  - `_agent._config` → provider select (openrouter/lmstudio), baseURL/apiKey/model text, timeoutSeconds number, enabled bool.
  - Unknown/advanced keys preserved from existing JSON on save; collapsible raw-JSON textarea fallback.

### 7.4 Collection API rules editor
- New `/pbx-setup` section "Collection API rules" for data collections (exclude `users`, `roles`, `_`-prefixed, `IsView()`).
- Per collection: 5 rule rows (list/view/create/update/delete), each a mode dropdown (Public / Signed-in / Selected users / Superusers only / Custom) + checkbox list of all `users` records (id + name/email label) + custom filter text input.
- Generation: Public→`""`; Signed-in→`@request.auth.id != ''`; Selected→`@request.auth.id = "id1" || @request.auth.id = "id2"`; Superusers only→`nil`; Custom→raw text.
- Parse existing rule back to UI state on load (nil→superusers, `""`→public, auth-id `||` chain→selected ids, else custom).
- New `POST /pbx-setup/rules` (superadmin) → parse form, `e.App.Save(collection)` per collection, redirect back.
- Page-data additions: `SetupCollectionRules[]`, `SetupUser{ID,Label}`.

### 7.5 Enforce rules in app routes
- Rename `agentRequestInfo` → `authRequestInfo` (keep alias).
- Non-superuser checks:
  - list: `handleTabulator`/`buildTabulatorData` + `handleTabulatorDataJSON` filter via `app.CanAccessRecord(rec, info, coll.ListRule)`; nil listRule → empty for non-superusers.
  - form GET edit: `CanAccessRecord(rec, info, coll.ViewRule)`; GET new: deny if `!info.HasSuperuserAuth() && coll.CreateRule == nil`.
  - POST update: `CanAccessRecord(rec, info, coll.UpdateRule)`; POST create: duplicated `checkCreateRule` helper (dummy-record eval, mirrors pbai).
  - delete: `CanAccessRecord(rec, info, coll.DeleteRule)`.
  - View collections (`IsView()`): skipped.

### 7.6 Verification
- `go build ./... && go vet ./...`, `go test ./pbai/`.
- Manual E2E as superuser (`mherstus@pointx.cz` / `Michal_1962`): gate `/pbx-setup`; edit `_app` (incl. icon upload), `_views` (structured JSON + columnOrder checkboxes), `_agent`; save rules via user selection; confirm non-superuser sees filtered `/tabular` and 403 on create when rules deny.
- Update `AGENTS.md` (new routes, rules editor, enforcement, setup-record page).

**Not in this iteration (later phases):** Lua actions, card/kanban/detail/report views, print-to-PDF, MySQL/Postgres support.
## Phase 8 — Mobile views (phone/tablet)

Context: PBX currently serves desktop-optimized HTML templates for all pages. No mobile detection, no responsive CSS (`@media` queries), no separate mobile templates. The `User-Agent` header is already captured in `RequestInfo.Headers` but unused. Templates are self-contained (one `.html` per page type, inline `<style>`/`<script>`). Each page data struct has a `Theme` field.

Decisions confirmed with user:
- Server-side User-Agent detection with automatic redirect (302) for mobile browsers.
- Explicit `/mobile/` route prefix for manual navigation.
- Separate mobile templates (not shared templates with `{{if .IsMobile}}`).
- Four mobile views: app (dashboard), tabular (card-based list), form (stacked), and /ai (agent chat).
- Mobile templates share `theme.css` (light/dark variables) + new `mobile.css`.
- Purely HTML/CSS (no swipe/gesture JS).
- `/pbx-setup`, `/pbx-config/*`, `/pbx-config/import-*` remain desktop-only.

### 8.1 Mobile detection

New helper in `main.go` (near `authRequestInfo` ~line 3150):

```go
func isMobile(r *http.Request) bool {
    ua := strings.ToLower(r.UserAgent())
    mobiles := []string{"iphone", "ipod", "android", "mobile", "windows phone",
        "blackberry", "opera mini", "opera mobi"}
    tablets := []string{"ipad", "tablet", "kindle", "silk"}
    for _, m := range mobiles {
        if strings.Contains(ua, m) { return true }
    }
    for _, t := range tablets {
        if strings.Contains(ua, t) { return true }
    }
    return false
}
```

Covers iOS, Android phones & tablets. iPadOS sends desktop UA by default, so iPads get desktop view (acceptable for large screens).

### 8.2 Page data struct changes

**File**: `views/pages.go`

Add `BasePath string` to `TabulatorPageData` (line 109), `FormPageData` (line 177), `AppPageData` (line 203), and `AgentPageData` (line 266). Desktop handlers set `BasePath: ""`. Mobile handlers set `BasePath: "/mobile"`. Templates use `{{.BasePath}}` in URL constructions.

### 8.3 Desktop route redirect

In existing desktop GET handlers, add early `isMobile` check → `302` redirect to `/mobile/...` equivalent:

| Desktop handler | Redirect target |
|----------------|-----------------|
| `handleApp` (line 359) | `/mobile/app` |
| `handleTabulator` (line 967) | `/mobile/tabular/{configName}` |
| `handleForm` (line 2538) | `/mobile/form/{configName}[/{id}][?view=1]` |

POST routes (`handleFormPost`) are NOT redirected — mobile form templates post to `/mobile/form/...` which goes through separate mobile POST handlers.

### 8.4 New mobile handlers

**File**: `main.go`

| Handler | Shares logic with | Diff |
|---------|-------------------|------|
| `handleMobileApp` | `handleApp` (lines 359-452) | Links use `/mobile/tabular/` prefix; renders `mobile-app.html` |
| `handleMobileTabulator` | `handleTabulator` (lines 967-983) via `buildTabulatorData` | Sets `BasePath: "/mobile"`; renders `mobile-tabulator.html` |
| `handleMobileForm` | `handleForm` (lines 2538-2760) | Sets `BasePath: "/mobile"`; renders `mobile-form.html` |
| `handleMobileFormPost` | `handleFormPost` (lines 2764-2855) | Redirect to `/mobile/tabular/{configName}` instead of `/tabular/{configName}` |
| `handleMobileDeleteRecord` | `handleDeleteRecord` (lines 3081-3113) | No change (returns JSON; mobile JS calls correct URL) |
| `handleMobileAi` | `handleAgent` (lines 2995-3021) | Renders `agent.html` directly (no separate mobile template); close button links to `/mobile/app` |

### 8.5 Route registration

**File**: `main.go` — inside `OnServe()` hook (after existing routes, ~line 301):

```
GET  /mobile/app                              → handleMobileApp
GET  /mobile/tabular/{configName}             → handleMobileTabulator
GET  /mobile/form/{configName}                → handleMobileForm
GET  /mobile/form/{configName}/{id}           → handleMobileForm
POST /mobile/form/{configName}                → handleMobileFormPost
POST /mobile/form/{configName}/{id}           → handleMobileFormPost
POST /mobile/form/{configName}/{id}/delete    → handleMobileDeleteRecord
GET  /mobile/ai                               → handleMobileAi
```

### 8.6 Mobile CSS

**New file**: `views/assets/mobile.css` (~100 lines)

Shared styles for all mobile templates:
- Touch-friendly targets (min 44px tap, 48px for actions)
- 16px font on inputs (prevents iOS auto-zoom)
- Card components (`.m-card`, `.m-card-row`, `.m-card-title`)
- FAB button (`.m-fab`) for "add new record"
- Full-width search input (`.m-search`)
- Stacked form fields (`.m-field`)
- Sticky bottom action bar (`.m-actions`)
- Pagination controls (`.m-pagination`)

Consumes theme CSS custom properties via `var(--body-bg)`, `var(--card-bg, #fff)`, `var(--btn-primary)`, etc.

### 8.7 Mobile templates

#### `views/mobile-app.html` (~90 lines)

Dashboard — single-column card layout:
- Top bar: "PBX" title + theme toggle + logout
- Each `_app` group = one `.m-card` with group header + link rows
- Links point to `/mobile/tabular/{name}` (built by handler)
- AI agent link row pointing to `/mobile/ai` (always shown, not gated by `_app` groups)
- Large touch-friendly link targets

#### `views/mobile-tabulator.html` (~300 lines)

Card-based record list (NOT a table):
- Search bar at top
- Each record = one `.m-card` showing first 2-3 visible fields
- Card tap → view/edit record (`/mobile/form/{cfg}/{id}`)
- Action icons at card bottom: view, edit, delete
- Pagination at bottom (same JS logic as desktop, different render output)
- FAB "+" button linking to `/mobile/form/{cfg}` (new record)
- JS reuses desktop's `getFiltered()`, sort, search, filter logic — only `render()` output changes from `<tr>` to card HTML
- All internal URLs prefixed with `{{.BasePath}}`

#### `views/mobile-form.html` (~200 lines)

Stacked form:
- All fields single-column (vertical stack)
- Full-width inputs with 12px padding
- Form `action="{{.BasePath}}/form/{{.ConfigName}}{{if .ID}}/{{.ID}}{{end}}"`
- Sticky bottom bar: Submit + Cancel buttons
- Cancel → `{{.BasePath}}/tabular/{{.ConfigName}}` (or history back)
- Reuses the same `{{define "formRowLoop"}}` template block as desktop — only CSS layout changes (columns stack vertically)

### 8.8 Link map

| Location | Desktop URL | Mobile URL | Mechanism |
|----------|------------|------------|-----------|
| `app.html` group links | `/tabular/{name}` | `/mobile/tabular/{name}` | Handler builds different URL |
| `app.html` AI link | `/ai` | `/mobile/ai` | Handler builds different URL |
| `tabulator.html` JS → view | `/form/{cfg}/{id}?view=1` | `/mobile/form/{cfg}/{id}?view=1` | `{{.BasePath}}` prefix |
| `tabulator.html` JS → edit | `/form/{cfg}/{id}` | `/mobile/form/{cfg}/{id}` | Same |
| `tabulator.html` JS → delete | `/form/{cfg}/{id}/delete` | `/mobile/form/{cfg}/{id}/delete` | Same |
| `tabulator.html` JS → new | `/form/{cfg}` | `/mobile/form/{cfg}` | Same |
| `tabulator.html` filter API | `/api/filters/{cfg}` | `/api/filters/{cfg}` | **No change** — shared API |
| `form.html` action | `/form/{cfg}[/{id}]` | `/mobile/form/{cfg}[/{id}]` | `{{.BasePath}}` prefix |
| `form.html` cancel | history.back or `/app` | history.back or `/mobile/app` | `{{.BasePath}}` |
| `handleFormPost` redirect | `/tabular/{cfg}` | `/mobile/tabular/{cfg}` | Different handler |
| `agent.html` close button | `/app` | `/mobile/app` | `{{.BasePath}}` |
| Theme toggle API | `/api/theme/{mode}` | `/api/theme/{mode}` | **No change** — shared API |

### 8.9 Desktop-only pages

`/pbx-setup`, `/pbx-config/*`, `/pbx-config/import-*` — no mobile versions. Since these are superadmin-only, acceptable to show desktop view on mobile. No redirect or "desktop only" message needed.

### 8.10 File change summary

| File | Action | Lines (est.) |
|------|--------|-------------|
| `main.go` | Edit: add `isMobile()`, redirect in 3 desktop handlers, add 6 mobile handler functions, register 8 routes | +215 |
| `views/pages.go` | Edit: add `BasePath` to 4 structs | +4 |
| `views/assets/mobile.css` | **New** | ~100 |
| `views/mobile-app.html` | **New** — includes AI agent link | ~90 |
| `views/mobile-tabulator.html` | **New** | ~300 |
| `views/mobile-form.html` | **New** | ~200 |

Total: ~895 new lines, ~20 modified lines.

### 8.11 Verification

- `go build ./... && go vet ./...`
- Desktop browser: all existing routes unchanged
- Phone/emulator (Chrome DevTools device mode): auto-redirect to `/mobile/...`
- Navigate directly to `/mobile/tabular/{name}` from desktop: renders mobile template
- Mobile form submit → redirects to `/mobile/tabular/{name}` with success message
- Theme toggle works on all mobile pages
- `/pbx-setup` on mobile shows desktop view (no redirect)
- `/mobile/ai` renders the agent chat page correctly on mobile

## phases 1 - 8 are implemented!
-------------------------------------------------------------------------
## Phase 9 — User landing page (`_view` field)
### done!

Context: After login, regular users always go to `/app` (line 482). Superusers always go to `/pbx-setup` (line 500). Some users need a specific view as their landing page (e.g., a tabular view of data they work with daily).

Decisions confirmed with user:
- `_view` field holds a config name (e.g., `zamestnanci`), not a full path.
- Superusers always go to `/pbx-setup` — no `_view` override.
- If `_view` is empty or the config doesn't exist, fall back to `/app`.
- If Phase 8 (mobile) is also implemented, check `isMobile()` and redirect to `/mobile/tabular/{viewName}` for mobile users.

### 9.1 Migration

`pb_migrations/` JS SDK — add `_view` (text, optional) field to the `users` collection.

### 9.2 Handler change

In `handleLoginPost` (main.go:482), replace the hardcoded `return e.Redirect(http.StatusSeeOther, "/app")` with:

```go
viewName := record.GetString("_view")
if viewName != "" {
    if _, _, err := resolveListConfig(e, viewName); err == nil {
        if isMobile(e.Request) {
            return e.Redirect(http.StatusSeeOther, "/mobile/tabular/"+viewName)
        }
        return e.Redirect(http.StatusSeeOther, "/tabular/"+viewName)
    }
}
return e.Redirect(http.StatusSeeOther, "/app")
```

Superusers (line 500) remain unchanged — always redirect to `/pbx-setup`.

### 9.3 Verification

- `go build ./... && go vet ./...`
- Login as user with `_view` set → redirected to `/tabular/{viewName}`
- Login as user with `_view` empty → redirected to `/app`
- Login as user with `_view` set to nonexistent config → redirected to `/app`
- Login as superuser → always `/pbx-setup`
- With Phase 8: login as mobile user with `_view` → redirected to `/mobile/tabular/{viewName}`

## Phase 10 — Custom actions in tabular and form views
### done!

Context: Users need to run custom business logic (report generation, bulk updates, cross-collection operations) from the tabular and form views. Actions are scripts stored in an `_actions` collection, executed via Goja (already in go.mod as indirect dep via PocketBase jsvm). Scripts run as the current user with collection rules enforced at the Go level.

Decisions confirmed with user:
- Goja (not Lua) — already in dependency graph, zero new deps.
- Scripts execute immediately (no confirmation step).
- Synchronous execution with 10-second timeout.
- Desktop and mobile views (depends on Phase 8 for mobile templates).
- Actions edited via existing `/pbx-setup` record editor (direct `_actions` record editing).

### 10.1 `_actions` collection schema

Migration via JS SDK in `pb_migrations/`:

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `_name` | text | yes | Display name (e.g., "Export overdue tasks") |
| `_description` | text | no | Help text shown in UI |
| `_script` | text | yes | JavaScript (Goja) source |
| `_collection` | text | yes | Target collection name (e.g., `zamestnanci`) |
| `_onList` | bool | no | Show in tabular view dropdown (default true) |
| `_onForm` | bool | no | Show in form view dropdown (default false) |
| `_public` | bool | no | Visible to non-superusers (default false — superuser-only) |

API rules: `nil` (superusers only) — only admins create/edit actions. Non-superusers can only see actions where `_public = true`.

### 10.2 Script execution engine

**New package**: `pbactions/`

#### `types.go`

```go
type ActionDef struct {
    ID          string
    Name        string
    Description string
    Script      string
    Collection  string
    OnList      bool
    OnForm      bool
    Public      bool
}

type ActionResult struct {
    OK      bool     `json:"ok"`
    Output  []string `json:"output"`   // log() calls
    Affected int     `json:"affected"` // total inserts+updates+deletes
    Error   string   `json:"error,omitempty"`
}
```

#### `runner.go`

Core execution:

```go
func Run(app core.App, authRecord *core.Record, action *ActionDef, recordIDs []string) (*ActionResult, error)
```

Steps:
1. Create Goja VM
2. Register built-in functions (see 10.3)
3. Set timeout (10 seconds — Goja's `Interrupt()` method)
4. If `recordIDs` provided: fetch records via `FindRecordById`, inject as `record` (single) or `records` (array)
5. If empty: inject empty record/records
6. Execute script
7. Return `ActionResult`

Sandbox restrictions:
- No `require()`, `import()`, `fs`, `net`, `http` — only the injected builtins
- Iteration limit (max 10000 loop iterations via Goja's `interrupt` mechanism)
- Timeout via Goja's `Interrupt()` method tied to `context.WithTimeout`

#### `builtins.go`

Built-in functions injected into the Goja VM:

| Function | Signature | Description |
|----------|-----------|-------------|
| `select(coll, filter, sort?, limit?)` | Returns `[]map` | Query records. `filter` is PB filter syntax. Enforces `listRule`. |
| `insert(coll, data)` | Returns `string` (new record ID) | Create record. Enforces `createRule`. |
| `update(coll, id, data)` | Returns `bool` | Update record by ID. Enforces `updateRule`. |
| `delete(coll, id)` | Returns `bool` | Delete record by ID. Enforces `deleteRule`. |
| `get(coll, id)` | Returns `map` | Fetch single record. Enforces `viewRule`. |
| `log(...args)` | Returns `void` | Append to output buffer (displayed in result modal). |
| `currentUser()` | Returns `{id, name, email}` | Current authenticated user. |
| `currentRecord()` | Returns `map` or `null` | The current record (form view) or first selected record (tabular). |
| `selectedRecords()` | Returns `[]map` | All selected records (tabular), or `[currentRecord]` (form). |
| `count(coll, filter?)` | Returns `number` | Count records matching filter. |

Each built-in function calls the corresponding `core.App` method, enforcing PocketBase collection rules.

### 10.3 Backend routes

**File**: `main.go`

New routes registered in `OnServe()`:

```
POST /actions/execute              → handleActionExecute
GET  /api/actions/{collectionName} → handleActionsList
```

#### `handleActionExecute`

```
POST /actions/execute
Body: { "actionId": "...", "recordIds": ["id1", "id2", ...] }
Response: { "ok": true, "output": [...], "affected": 3 }
```

1. Parse JSON body
2. Fetch `_actions` record by ID → `ActionDef`
3. Verify user can access the target collection
4. Call `pbactions.Run(app, authRecord, action, recordIDs)`
5. Return `ActionResult` as JSON

#### `handleActionsList`

```
GET /api/actions/{collectionName}
Response: { "actions": [{ "id", "name", "description" }, ...] }
```

Returns actions where `_collection == collectionName` AND (`_public == true` OR user is superuser).

### 10.4 Desktop tabular view changes

**File**: `views/tabulator.html`

Add an "Actions" dropdown next to existing action bar (Import/Export/Add new):

```html
<select id="actionSelect" style="...">
    <option value="">— Actions —</option>
    <!-- populated via JS from /api/actions/{collectionName} -->
</select>
<button id="runActionBtn" style="...">Run</button>
```

JS logic:
1. On page load: fetch `/api/actions/{{.CollectionName}}` → populate `<select>`
2. On "Run" click: collect selected record IDs from checkbox column (add checkboxes to each row)
3. If no records selected: run action on all visible records
4. POST to `/actions/execute` with `{actionId, recordIds}`
5. Show result modal with output/affected count/error

Add a checkbox column to the table (first `<th>`, checkboxes in each `<td>`). "Select all" checkbox in header.

### 10.5 Desktop form view changes

**File**: `views/form.html`

Add actions dropdown in the header bar (next to theme toggle / close button):

```html
<select id="actionSelect" style="...">
    <option value="">— Actions —</option>
</select>
<button id="runActionBtn" style="...">Run</button>
```

JS: on "Run" click, POST to `/actions/execute` with `{actionId, recordIds: ["{{.ID}}"]}`.
If ID is empty (new record, not yet saved): disable the dropdown.

### 10.6 Mobile views

**Files**: `views/mobile-tabulator.html`, `views/mobile-form.html`

Same pattern — action dropdown in the action bar area. For mobile tabular: actions apply to all records (no per-row checkboxes — card layout). For mobile form: same as desktop form.

### 10.7 Result modal

Shared across desktop and mobile templates (inline in each, since templates are self-contained):

```html
<div class="modal-overlay" id="actionResultModal">
    <div class="modal-content">
        <div class="modal-header">
            <h3>Action Result</h3>
            <button class="modal-close" onclick="closeActionResultModal()">&times;</button>
        </div>
        <div class="modal-body">
            <div id="actionResultOutput"></div>
            <div id="actionResultStatus"></div>
        </div>
    </div>
</div>
```

Display: output lines as `<pre>` text, status as badge (green "Success" / red "Error: ...").

### 10.8 Example scripts

**Generate report** (tabular, read-only):
```javascript
var rows = select("zamestnanci", "active = true", "", 100);
log("Active employees: " + rows.length);
rows.forEach(function(r) {
    log(r.jmeno + " " + r.prijmeni + " — " + r.odeleni);
});
```

**Bulk update** (tabular, write):
```javascript
var rows = select("produkty", "cena < 100", "", 0);
rows.forEach(function(r) {
    update("produkty", r.id, { cena: r.cena * 1.1 });
});
log("Updated " + rows.length + " products (price +10%)");
```

**Cross-collection operation** (form):
```javascript
var emp = currentRecord();
var tasks = select("ukoly", "zamestnanec = '" + emp.id + "'", "", 0);
tasks.forEach(function(t) {
    update("ukoly", t.id, { stav: "dokonceno" });
});
log("Completed " + tasks.length + " tasks for " + emp.jmeno);
```

**Delete with confirmation** (tabular):
```javascript
var rows = selectedRecords();
rows.forEach(function(r) {
    delete("poznamky", r.id);
});
log("Deleted " + rows.length + " notes");
```

### 10.9 File change summary

| File | Action | Lines (est.) |
|------|--------|-------------|
| `pbactions/types.go` | **New** | ~30 |
| `pbactions/runner.go` | **New** — Goja VM setup, timeout, iteration limit | ~120 |
| `pbactions/builtins.go` | **New** — select/insert/update/delete/get/log/currentUser/selectedRecords/count | ~250 |
| `main.go` | Edit — add 2 routes + 2 handlers | +80 |
| `views/tabulator.html` | Edit — add action dropdown + checkbox column + JS fetch/execute | +60 |
| `views/form.html` | Edit — add action dropdown + JS | +30 |
| `views/mobile-tabulator.html` | Edit — add action dropdown (Phase 8) | +20 |
| `views/mobile-form.html` | Edit — add action dropdown (Phase 8) | +15 |
| `pb_migrations/` | **New** — JS migration to create `_actions` collection | ~40 |

Total: ~400 new lines, ~130 modified lines.

### 10.10 Circular import avoidance

`pbactions` needs `core.App` and the user's auth record. Instead of importing the main package, `Run()` accepts `core.App` + `*core.Record` (auth record) as parameters. The caller in `main.go` resolves the auth record via `authRequestInfo(e)` and passes both to `Run()`.

### 10.11 Verification

- `go build ./... && go vet ./...`
- Create `_actions` record via `/pbx-setup` → record editor
- Navigate to `/tabular/{collection}` → actions dropdown populated
- Run read-only action → result modal shows output
- Run write action → records modified, result modal shows affected count
- Run action on selected records → only those records processed
- Navigate to `/form/{collection}/{id}` → form action runs on current record
- Mobile views: action dropdown works on `/mobile/tabular/` and `/mobile/form/`
- Non-superuser: only sees `_public = true` actions
- Script error → result modal shows error, no records modified
- Timeout (infinite loop) → script killed after 10s, error returned

### 10.12 Dependencies on other phases

- **Phase 8 (mobile)**: mobile action dropdowns go into mobile templates. If Phase 8 not yet implemented, skip mobile changes.
- **Phase 9 (landing page)**: no dependency.
- Independent of Phases 1-7.

## Phase 11 — AI agent action management tools
### done!

Context: Phase 10 adds custom actions (Goja scripts in `_actions` collection). Phase 11 extends the AI agent (`pbai/`) with tools to create, update, and list actions via natural language. Superuser-only.

### 11.1 New tools

| Tool | Type | Purpose |
|------|------|---------|
| `create_action` | write (pending) | Create or update an `_actions` record (upsert by `_name`) |
| `list_actions` | read | List existing actions for a given collection |

### 11.2 `create_action` tool

Superuser-only (mirrors `set_view_config` pattern at tools.go:625). Upserts `_actions` by `_name` — if an action with that name exists, update it; otherwise create new.

**Parameters** (JSON Schema):
- `name` (string, required) — action display name
- `collection` (string, required) — target collection name
- `script` (string, required) — JavaScript source
- `description` (string, optional) — help text
- `onList` (boolean, optional, default true) — show in tabular view
- `onForm` (boolean, optional, default false) — show in form view
- `public` (boolean, optional, default false) — visible to non-superusers

**`pending` function** — validates collection exists, returns `PendingAction` with summary "Create/update action '{name}' on collection '{collection}'".

**`exec` function** — upserts the `_actions` record:
```go
recs, _ := a.App.FindRecordsByFilter("_actions", "_name = {:name}", "", 1, 0, dbx.Params{"name": in.Name})
if len(recs) > 0 {
    rec = recs[0]
} else {
    rec = core.NewRecord(actionsColl)
}
rec.Set("_name", in.Name)
rec.Set("_collection", in.Collection)
rec.Set("_script", in.Script)
rec.Set("_description", in.Description)
rec.Set("_onList", in.OnList)
rec.Set("_onForm", in.OnForm)
rec.Set("_public", in.Public)
a.App.Save(rec)
```

### 11.3 `list_actions` tool

Read-only. Lists actions for a given collection.

**Parameters**: `collection` (string, required)

**`exec` function**:
```go
recs, _ := a.App.FindRecordsByFilter("_actions", "_collection = {:coll}", "_name asc", 100, 0, dbx.Params{"coll": in.Collection})
```

Returns formatted list of action names, descriptions, and onList/onForm/public flags.

### 11.4 Registration

In `allTools()` (tools.go:47), add both tools:
```go
func allTools() []tool {
    return []tool{
        listCollectionsTool(),
        getSchemaTool(),
        queryRecordsTool(),
        insertRecordsTool(),
        createCollectionTool(),
        setViewConfigTool(),
        createActionTool(),   // new
        listActionsTool(),    // new
    }
}
```

### 11.5 File changes

| File | Change | Lines (est.) |
|------|--------|-------------|
| `pbai/tools.go` | Add `createActionTool()` + `listActionsTool()`, register in `allTools()` | +80 |
| `AGENTS.md` | Document new tools | +10 |

### 11.6 Dependency on Phase 10

Tools operate on `_actions` collection directly (same pattern as `set_view_config` on `_views`). If Phase 10 hasn't been implemented, the agent can still create/edit action records — they just won't be executable until Phase 10 is in place.

### 11.7 Verification

- `go build ./... && go vet ./...`, `go test ./pbai/`
- Ask agent: "Create an action called 'test' on collection 'zamestnanci' that logs the record count" → confirm modal → `_actions` record created
- Ask agent: "List actions for zamestnanci" → returns the action
- Non-superuser → both tools return permission error

## Phase 12 — Multilanguage UI (i18n)
### done!

Implemented (see git history for the Phase 12 commit).

**Deviation from spec**: the per-browser language override uses a `pb_lang` cookie instead of `localStorage`. Unlike themes (pure CSS that applies post-render), language must be server-rendered — a cookie lets `getLangCode(app, r)` resolve the caller's language before emitting the HTML. Resolution order: `--lang` CLI flag > `pb_lang` cookie > `pb_data/lang.json` > `en`.

- New `i18n/` package: embedded `en.json`/`cs.json` (231 keys each), `i18n.T(lang, key)`, `i18n.CatalogJSON(lang)`.
- Template func `{{t .Lang "key"}}`; every page data struct embeds `views.LangData` (`Lang`).
- Routes: `POST /api/lang/{code}` (persists global default), `GET /api/lang/{code}/catalog.js` (serves `window._t` + catalog).
- All 10 in-scope templates translated; mobile templates untouched.
- Also fixed a latent nil-pointer in `tabulator.html` (`getMssqlParams` referenced `.Mssql.DSN` even when `.Mssql` was nil).
- Added `templates_test.go` (root package) asserting every template renders under `cs`.

Context: All UI strings are hardcoded in English across 10 HTML templates and ~15 locations in `main.go`. Users need Czech + English support with a per-user language switcher. The theme toggle pattern (localStorage + server JSON + topbar button) provides the exact template to mirror.

Decisions confirmed with user:
- Initial languages: Czech (`cs`) + English (`en`)
- Language resolution: CLI flag `--lang` (server default) > `localStorage` (per-browser) > `pb_data/lang.json` (persistent default)
- Translation storage: JSON files in `i18n/` directory, loaded at startup
- Template function: `{{t .Lang "key"}}` for Go templates; `window._t("key")` for JS strings
- Language switcher: topbar button mirroring the theme toggle

### 12.1 Translation infrastructure

**New directory**: `i18n/`

**Files**:
- `i18n/en.json` — English translations (~180 keys)
- `i18n/cs.json` — Czech translations (~180 keys)

**Key naming convention**: dot-notation grouped by page/section:
```
common.close, common.cancel, common.save, common.delete, common.loading,
common.yes, common.no, tabulator.search_placeholder, tabulator.filter,
tabulator.import, tabulator.export, tabulator.add_new, tabulator.no_records,
tabulator.page_info, tabulator.really_delete, tabulator.record_deleted,
form.submit, form.cancel, form.none, login.sign_in, login.credentials,
login.name, login.password, login.login, login.invalid, login.required,
app.title, app.logout, app.please_sign_in, app.signed_in_as, app.ai_agent,
app.description, setup.title, setup.applications, setup.config_editor,
setup.administration, setup.theme, setup.theme_description, setup.light,
setup.dark, setup.mssql, setup.ai_agent, setup.save_config, setup.save_dsn,
setup.collection_rules, setup.public, setup.signed_in, setup.selected_users,
setup.superusers_only, setup.custom, setup.save_rules, config.title,
config.manage, config.from_excel, config.from_mssql, config.view_configs,
config.new_config, config.name, config.collection, config.configured,
config.list, config.form, config_editor.title, config_editor.back,
config_editor.choose_collection, config_editor.tabulator_json,
config_editor.form_json, config_editor.mssql_json, import.source,
import.preview, import.create, import.collection_name, import.detect_schema,
import.create_collection, import.import_data, import.create_view_config,
agent.title, agent.thinking, agent.confirm, agent.reject, agent.approve,
agent.ask, agent.network_error, agent.please_confirm, setup_record.new_edit,
setup_record.back, setup_record.create_in, setup_record.editing_in,
setup_record.remove_file, setup_record.current_file, setup_record.toggle_json,
setup_record.add_column, setup_record.add_mapping, setup_record.mssql_column,
setup_record.pb_field, wizard.import_title, wizard.create_from,
wizard.excel_file, wizard.sheet_name, wizard.mssql_dsn, wizard.table,
wizard.back, wizard.use, wizard.header, wizard.pb_field_name, wizard.type,
wizard.sample_values, result.success, result.export_success,
result.import_success, result.record_added, result.record_updated
```

**New package**: `i18n.go` in project root (or `pbx/i18n/` subpackage)

```go
package i18n

import (
    "encoding/json"
    "os"
    "path/filepath"
    "fmt"
    "sync"
)

var (
    mu       sync.RWMutex
    catalogs map[string]map[string]string // lang → key → value
)

func Load(dir string) error {
    mu.Lock()
    defer mu.Unlock()
    catalogs = make(map[string]map[string]string)
    entries, err := os.ReadDir(dir)
    if err != nil { return err }
    for _, e := range entries {
        if e.IsDir() || filepath.Ext(e.Name()) != ".json" { continue }
        lang := e.Name()[:len(e.Name())-5] // strip .json
        data, err := os.ReadFile(filepath.Join(dir, e.Name()))
        if err != nil { continue }
        var m map[string]string
        if json.Unmarshal(data, &m) != nil { continue }
        catalogs[lang] = m
    }
    return nil
}

func T(lang, key string, args ...any) string {
    mu.RLock()
    defer mu.RUnlock()
    if m, ok := catalogs[lang]; ok {
        if v, ok := m[key]; ok {
            if len(args) > 0 {
                return fmt.Sprintf(v, args...)
            }
            return v
        }
    }
    if m, ok := catalogs["en"]; ok {
        if v, ok := m[key]; ok { return v }
    }
    return key
}

func langs() []string {
    mu.RLock()
    defer mu.RUnlock()
    out := make([]string, 0, len(catalogs))
    for k := range catalogs { out = append(out, k) }
    return out
}
```

### 12.2 Language resolution

**Server-side**: `getLangCode(app)` in `main.go` (mirrors `getThemeMode`):
1. CLI flag `--lang` overrides (highest priority)
2. Read `pb_data/lang.json` → `{"code":"en"}`
3. Fallback: `"en"`

**Per-request**: Template handlers pass `Lang: getLangCode(e.App)` + `CatalogJSON` (full catalog as JSON string for JS `window._t()`).

**Persistence**: `POST /api/lang/{code}` route:
- Validates code is in loaded catalog (`i18n.langs()`)
- Writes `pb_data/lang.json` → `{"code":"cs"}`
- Returns JSON `{lang: code}`

### 12.3 CLI flag

In `main.go`, before `app.Start()`:
```go
app.RootCmd.PersistentFlags().String("lang", "", "default UI language (e.g. en, cs)")
```

In the `OnServe` hook, read the flag and override `pb_data/lang.json` if set:
```go
langFlag, _ := app.RootCmd.Flags().GetString("lang")
if langFlag != "" {
    setDefaultLang(langFlag)
}
```

### 12.4 Page data structs

**File**: `views/pages.go`

Add `Lang string` + `CatalogJSON string` to all 9 structs that have `Theme`:
- `TabulatorPageData`, `FormPageData`, `AppPageData`, `PbxSetupPageData`, `SetupRecordPageData`, `AgentPageData`, `PbxConfigPageData`, `ConfigEditorPageData`, `ImportWizardPageData`

Each handler sets:
```go
Lang: getLangCode(e.App),
CatalogJSON: getCatalogJSON(getLangCode(e.App)),
```

Where `CatalogJSON` is the full catalog for the current language serialized as JSON.

### 12.5 Template function `{{t}}`

Register in `main.go` template function map:
```go
"t": func(lang, key string, args ...any) string {
    return i18n.T(lang, key, args...)
}
```

Usage in templates: `{{t .Lang "key"}}` — explicit language parameter, no closure needed.

### 12.6 Template changes

**All 10 HTML files** — replace hardcoded strings with `{{t .Lang "key"}}`:

| File | Approx. replacements |
|------|---------------------|
| `tabulator.html` | ~40 |
| `form.html` | ~12 |
| `app.html` | ~10 |
| `login.html` | ~8 |
| `pbxsetup.html` | ~30 |
| `pbxconfig.html` | ~12 |
| `config.html` | ~15 |
| `agent.html` | ~10 |
| `setup-record.html` | ~15 |
| `import-wizard.html` | ~25 |

**Topbar language switcher** — add button next to theme toggle in all templates:
```html
<button type="button" id="langBtn" onclick="toggleLang()"
    style="padding:8px 12px;background:var(--btn-neutral);color:#fff;border:none;border-radius:6px;font-size:1.1rem;cursor:pointer;">CS</button>
```

### 12.7 JavaScript translations

Inject full catalog into each template:
```html
<script>
window._translations = {{.CatalogJSON}};
window._t = function(key) { return window._translations[key] || key; };
</script>
```

Replace JS hardcoded strings:
```javascript
// Before:
confirm("Really delete selected record?")
// After:
confirm(window._t("tabulator.really_delete"))
```

### 12.8 Go handler strings

Strings in `main.go` handlers use `i18n.T(lang, key)` directly:
```go
lang := getLangCode(e.App)
e.String(http.StatusBadRequest, i18n.T(lang, "login.invalid"))
```

Affected: `handleLoginPost`, `handleApp`, `handleFormPost`, `handleDeleteRecord`, `handleExport`, `handleImport`, `handleMssqlExport`, `handleMssqlImport`, wizard handlers.

### 12.9 Language switcher JS

Mirrors theme toggle pattern:
```javascript
function currentLang() {
    var l = localStorage.getItem('pbx-lang');
    if (l === 'cs' || l === 'en') return l;
    return document.documentElement.getAttribute('lang') || 'en';
}
function applyLang(code) {
    document.documentElement.setAttribute('lang', code);
    var btn = document.getElementById('langBtn');
    if (btn) btn.textContent = code.toUpperCase();
    window.location.reload(); // full reload to re-execute Go templates
}
function toggleLang() {
    var next = currentLang() === 'en' ? 'cs' : 'en';
    localStorage.setItem('pbx-lang', next);
    fetch('/api/lang/' + next, { method: 'POST' }).catch(function(){});
    applyLang(next);
}
```

Full page reload required — unlike theme (CSS-only), language needs Go templates to re-render with new `{{t}}` values.

### 12.10 Login page

`login.html` lacks the topbar — add a small language toggle in the top-right corner of the login card:
```html
<button id="langBtn" onclick="toggleLang()" style="position:absolute;top:12px;right:12px;...">CS</button>
```

### 12.11 File change summary

| File | Action | Lines (est.) |
|------|--------|-------------|
| `i18n/en.json` | **New** — English translations | ~180 |
| `i18n/cs.json` | **New** — Czech translations | ~180 |
| `i18n.go` | **New** — loader + T() function | ~60 |
| `main.go` | Edit — register `t` func, getLangCode, setDefaultLang, CLI flag, POST /api/lang, pass Lang + CatalogJSON | +80 |
| `views/pages.go` | Edit — add Lang + CatalogJSON to 9 structs | +18 |
| `views/tabulator.html` | Edit — ~40 string replacements + lang switcher + JS translations | +15 |
| `views/form.html` | Edit — ~12 replacements + lang switcher | +10 |
| `views/app.html` | Edit — ~10 replacements | +5 |
| `views/login.html` | Edit — ~8 replacements + lang toggle | +8 |
| `views/pbxsetup.html` | Edit — ~30 replacements + lang switcher | +12 |
| `views/pbxconfig.html` | Edit — ~12 replacements + lang switcher | +8 |
| `views/config.html` | Edit — ~15 replacements + lang switcher | +8 |
| `views/agent.html` | Edit — ~10 replacements + lang switcher | +8 |
| `views/setup-record.html` | Edit — ~15 replacements + lang switcher | +8 |
| `views/import-wizard.html` | Edit — ~25 replacements + lang switcher | +10 |

Total: ~420 new lines (translations + loader), ~100 modified lines (handlers + templates).

### 12.12 Verification

- `go build ./... && go vet ./...`
- `./pbx serve` → default language from `pb_data/lang.json`
- `./pbx serve --lang cs` → default language Czech
- Navigate to any page → all strings in English (default)
- Click language button → page reloads in Czech
- Refresh → stays Czech (localStorage)
- `./pbx serve --lang en` → overrides back to English
- Login page toggle works, error messages translate
- All 10 templates render correctly in both languages
- JS strings (alerts, confirms) translate correctly
- `<html lang="cs">` attribute set correctly

### 12.13 Dependencies

- Independent of Phases 1-11
- No new Go dependencies (plain `encoding/json` + `fmt.Sprintf`)


## Phase 13 — Agent AI enhancements
### done!

Context: two issues surfaced while using `/ai` with a local LM Studio model (`google/gemma-4-e4b`):
1. Prompt "list all records in collection produkty" got an answer about a nonexistent "produkti" collection — the LLM mistyped the name inside its tool-call arguments and relayed the terse `collection "produkti" not found` error instead of retrying.
2. The same request took ~20s. Measured breakdown: call 1 (770 prompt tok → tool call) 5.7s incl. 90 hidden reasoning tokens; call 2 (1190 prompt tok → final answer) 10.9s generating a markdown table that the UI discards (it renders records as its own table). The second LLM call is pure waste for tabular questions.

Decisions confirmed with user:
- Fix #1 via self-correcting errors: fuzzy-match suggestions (Levenshtein) + list of available collections in the not-found error, plus a system-prompt rule to retry with the suggested name.
- Fix #2 via short-circuit: skip the final LLM call when `query_records` is the first and only executed tool call of the turn and it returned ≥1 record — the UI shows just the table anyway.
- Deferred: SSE token streaming (separate follow-up), slimming LLM-facing tool results, prompt compression, LM Studio tuning checklist (disable Reasoning toggle ≈3–8s/call, GPU offload, flash attention, minimal context).

### 13.1 Self-correcting collection lookup (`pbai/tools.go`) — done

- New `(a *Agent) findCollection(name string) (*core.Collection, error)`: exact `FindCachedCollectionByNameOrId` first; on failure builds accessible collection names (skip `_`-prefixed, view collections, non-listable), computes case-insensitive Levenshtein distance (threshold `min(2, max(1, len/3))`), returns e.g. `collection "produkti" not found (did you mean "produkty"?). Available collections: ...`.
- Small `levenshtein(a, b string) int` DP helper.
- All 8 call sites switched (get_schema, query_records, insert_records pending+exec, set_view_config pending+exec, create_action pending+exec); canned `fmt.Errorf("collection %q not found")` messages removed so the detailed error propagates. `create_collection` existence check untouched.
- System prompt rule added: copy collection names exactly; on not-found retry with the suggested name or call `list_collections` first.

### 13.2 Tabular fast path — skip final LLM call (`pbai/agent.go`)

- In `Run()` add `toolCallsSoFar := 0`; increment per executed tool call.
- After a successful `query_records` exec (records parsed into `lastRecords`), if `toolCallsSoFar == 1 && len(lastRecords) > 0` return immediately:
  `&ChatResult{Transcript: transcript, Records: lastRecords}` — `FinalText` stays empty; UI already renders table-only when `records` is present.
- Guardrails: "No accessible records found." leaves `lastRecords` nil → normal flow; multi-step flows (schema/list first, or query_records as 2nd exec) keep the final LLM pass; pending-action path untouched.

### 13.3 Tests (`pbai/agent_test.go`)

- `TestFindCollectionSuggestsTypo`: exact match works; `"produkti"` error contains suggestion `"products"` + `Available collections:`; same through the `query_records` tool path.
- `TestRunCapturesQueryRecords` updated: mock server counts requests — expect exactly **1** LLM call, `res.Records` populated, `res.FinalText == ""`.
- New `TestQueryRecordsFastPathNotForMultiStep`: schema → query_records → final text = 3 LLM calls, final text passes through (guardrail holds).

### 13.4 Verification

- `go build ./... && go vet ./... && go test ./pbai/`, rebuild `./pbx`.
- Manual: "list all records in collection produkty" drops from ~20s to ~5–6s (single LLM call), table-only answer; misspelled collection names self-correct within the same turn.

### 13.5 Follow-ups implemented in the same phase

- **Conversation memory** (was: agent forgot everything between prompts — the UI sent only the current message and `Run` is stateless): `views/agent.html` keeps the visible turns in a JS array, sends the last 16 with each request (server clamps at 40), records assistant answers / fast-path table notes / pending summaries / confirm outcomes into it; "New chat" button resets. Test `TestRunSendsHistoryToLLM`.
- **SSE streaming**: `Client.Stream` (`pbai/llm.go`) assembles streamed tool-call deltas by index; `RunStream` (`pbai/agent.go`) mirrors the loop emitting `text`/`status`/`done` events (`Run` delegates to it); `POST /ai/chat/stream` (`main.go`) serves `text/event-stream`; UI consumes via fetch ReadableStream — live text bubbles, tool status lines, table/pending rendering on done. All LLM traffic now uses the streaming API; test mocks emit SSE chunks; `TestStreamAssemblyMultiChunk`.

### 13.6 Still deferred

- Truncate long field values in the LLM-facing `query_records` result (UI table keeps full data via side-channel); compress system prompt/tool descriptions; document LM Studio performance checklist in `AGENTS.md`.

## Phase 14 — Security audit, code quality & performance optimizations
### done!

Context: Full codebase audit performed after Phases 1–13. Found critical vulnerabilities, duplicated logic, dead code, and architectural debt.

**Implemented (all sub-phases):**
- **14a — Security hotfixes**: path traversal blocked in `/assets` (`filepath.Clean` + `..` rejection) and `pbexcel.resolveExcelPath`; HMAC-SHA256 CSRF middleware + token in all form POSTs (login + setup + config + data); `Secure: r.TLS != nil` on auth cookies; `_view` redirect sanitized via `sanitizeConfigName`; `isValidCollectionName` defense-in-depth in all three `checkCreateRule` copies; per-IP rate limiters on `/login` (5/15min), `/ai/chat` (30/min), `/ai/chat/stream` (20/min).
- **14b — Shared rule evaluation**: new `pbrules` package (`CheckCreateRuleContext` + `CheckCreateRule`); `main.go`, `pbai/tools.go`, `pbactions/builtins.go` all delegate to it (3× duplication eliminated).
- **14c — DB-level pagination**: `/tabular` and `/api/tabulator-data` use `FindRecordsByFilter` + limit/offset (`?page=`, `?perPage=`); `TotalRecords`/`TotalPages` computed from a count query, no more `FindAllRecords` on the hot path.
- **14d — Dead code removed**: `parseListConfig`, `parseFormConfigJSON`, `formConfigFromView` (main.go) and legacy types `ListConfig`, `ListColumn`, `FormConfig`, `FormConfigJSON` (views/pages.go, `ListColumn` → `ViewColumn`).
- **14e — Route registration split**: new `routes.go` (package main) with `registerAuth/App/Setup/Config/Data/AssetsAndAPI/AI/ActionRoutes`; the 300-line `OnServe` closure shrinks to ~28 lines. Handler functions stay in `main.go` (they depend on package globals like `templates`).
- **14e — Bubble sort** in form field ordering replaced with `sort.SliceStable`.
- **14f — Resource & logging hygiene**: `pbmssql.CloseAll()` registered on `app.OnTerminate()`; `pbai` pending-action store gets a `sync.Once` background TTL cleanup goroutine; `requestLogger` middleware wraps the PB mux (installed after `se.Next()`) and emits one structured `log.Printf` line per request (method/path/status/duration/remote).

**Deviations / deferred:**
- `_filters` compound index `(_config, _name, _user)` — not yet added via migration; needs a JS migration in `pb_migrations/`.
- In-memory pending action store — kept (not DB-backed); now bounded by the TTL cleanup goroutine.
- **14g — Template consolidation**: evaluated and intentionally NOT done. Phase 8 explicitly chose separate mobile templates; the shared `formRowLoop` block already covers the core field rendering. Full consolidation contradicts the established design and adds regression risk.
- Client-side AI history clamp left to `agent.html` (already sends 16; server clamps at 40).

### 14.1 Critical vulnerabilities (fix immediately)

| Issue | Location | Fix |
|-------|----------|-----|
| Path traversal in static assets | `main.go:267-285` | Sanitize `path` with `filepath.Clean`, reject `..` prefix |
| Missing CSRF protection | All POST endpoints | Add CSRF middleware (gorilla/csrf or custom); inject token in all forms |
| Insecure cookie flags | `main.go:597,624` | Set `Secure: r.TLS != nil` on auth cookie |
| Unvalidated redirect via user data | `main.go:599-605` | Validate `_view` config exists before redirect; whitelist routes |
| SQL injection surface in `checkCreateRule` | `main.go:3649-3710`, `pbai/tools.go:167-223` | Parameterized CTE; validate table/column names from collection metadata |
| Missing rate limiting | `/ai/chat`, `/ai/chat/stream`, `/login` | Add per-IP + per-user rate limiter (token bucket) |
| Path traversal in Excel import | `pbexcel/pb-excel.go:26` | Sanitize `fileName`; reject `..`; resolve absolute path under `pb_data/` |

### 14.2 Ineffective / duplicated code (refactor)

| Issue | Location | Action |
|-------|----------|--------|
| N+1 / full-load in tabulator | `main.go:929-1100` | Replace `FindAllRecords` with `FindRecordsByFilter` + limit/offset; DB-level pagination & filtering |
| `checkCreateRule` duplicated 3× | `main.go:3649`, `pbactions/builtins.go:44`, `pbai/tools.go:167` | Extract to shared `pbrules` package; import in all three |
| Duplicate rule parsing | `main.go:1273-1335` | Unify `parseRuleToSetup` / `ruleFromSetup` via shared core |
| Repeated `configRaw` + unmarshal | `main.go:807,811,823,839,845` | Add generic `parseJSONField(rec, field, target)` helper |
| O(n²) bubble sort | `main.go:3068-3074` | Replace with `sort.Slice` |
| Inefficient Levenshtein alloc | `pbai/tools.go:144-163` | Reuse slice buffers; memoize or use stdlib |
| Missing DB index on `_filters` | `main.go:1971` | Add compound index `(_config, _name, _user)` |

### 14.3 Dead / legacy code (remove)

| Item | Location | Notes |
|------|----------|-------|
| `parseListConfig` | `main.go:798-808` | Legacy `_tabulator.config` reader — superseded by `_views._tabulator` |
| `parseFormConfigJSON` | `main.go:896-905` | Legacy `_form.config` reader |
| `formConfigFromView` | `main.go:883-894` | Legacy converter — only used in view editing fallback |
| `views.ListConfig`, `ListColumn` | `views/pages.go:35-44` | Legacy types — replaced by `ViewTabulatorConfig` |
| `views.FormConfig`, `FormConfigJSON` | `views/pages.go:162-185` | Legacy types — replaced by `ViewFormConfig` |
| `handleTabulatorDataJSON` string-only | `main.go:1873-1910` | Returns all fields as strings; loses type info — re-fetches anyway |
| `mobile-*` templates | `views/mobile-*.html` | 80% duplication with desktop — consolidate using `BasePath` |

### 14.4 Architectural improvements

| # | Task | Effort |
|---|------|--------|
| 1 | Split `main.go` (4160 lines) into route groups: `routes_app.go`, `routes_ai.go`, `routes_setup.go`, `routes_form.go`, `routes_actions.go`, `routes_mssql.go` | Medium |
| 2 | Add middleware layer for auth, mobile detection, superadmin, CSRF, rate limiting | Medium |
| 3 | Replace in-memory pending action store with DB-backed `_pending_actions` collection | Medium |
| 4 | Add structured logging (replace `log.Printf`); error context propagation | Low |
| 5 | MSSQL pool cleanup on app shutdown (`app.OnTerminate`) | Low |
| 6 | Client-side AI history clamp (server: 40, client: 16) — enforce on client | Low |
| 7 | Consolidate mobile/desktop templates using `BasePath` | Medium |

### 14.5 Implementation order (Phase 14)

1. **Phase 14a** — Security hotfixes (14.1): path traversal, CSRF, cookie flags, redirects, rate limiting
2. **Phase 14b** — Extract `checkCreateRule` to shared `pbrules` package (eliminates 3× duplication)
3. **Phase 14c** — DB-level pagination/filtering for `/tabular` + `/api/tabulator-data`
4. **Phase 14d** — Remove dead code (14.3); add `_filters` compound index
5. **Phase 14e** — Refactor `main.go` into route files + middleware layer
6. **Phase 14f** — DB-backed pending actions; MSSQL pool cleanup; logging
7. **Phase 14g** — Template consolidation (mobile/desktop)

### 14.6 Verification checklist

- `go build ./... && go vet ./...` after each sub-phase
- `go test ./pbai/` — all tests pass
- Manual pen-test: path traversal attempts blocked, CSRF tokens required, rate limits trigger
- Load test: `/tabular` with 10k+ records renders in <200ms (paginated)
- Login flow works; redirects validated; cookies secure in HTTPS
- AI agent streaming works; history persisted across prompts

## Phase 15 — Setup consolidation: move config files to DB
### planned

**Context:** Currently three JSON files live in `pb_data/`:
- `mssql.json` — global MSSQL DSN (read: `getMssqlDSN` via `mssqlConfigPath`; write: `POST /api/mssql-dsn` → `setMssqlDSN`)
- `theme.json` — global theme mode (read: `getThemeMode`; write: `POST /api/theme/{mode}` → `setThemeMode`; used by 13 page handlers)
- `lang.json` — global default language (read: `getLangCode`; write: `POST /api/lang/{code}` → `setLangCode`; used by 16 page handlers + 21 i18n lookups)

Goal: unify into a single `_global_setup` collection record (`_name="default"`) so all PBX configuration lives in the database and obeys PocketBase rules.

**Decisions confirmed with user:**
- Single record (`_name = "default"`) — no multi-environment support needed
- Superadmin-only write; read access for page rendering (non-superusers read via handlers)
- One-time migration on first run: if `pb_data/*.json` exist, migrate values into the new record
- Concurrency: `app.Dao().RunInTransaction()` for writes
- Fallback to defaults if DB unavailable during read

---

### 15.1 Schema (`_global_setup` collection)

| Field | Type | Description |
|-------|------|-------------|
| `name` | text (required, unique) | Fixed value `"default"` |
| `description` | text | Human-readable description |
| `_dsn` | json | `{dsn: "..."}` |
| `_theme` | json | `{mode: "light"|"dark"}` |
| `_lang` | json | `{lang: "en"|"cs"}` |

**API Rules:**
- `listRule: null`, `viewRule: null`
- `createRule`: superuser only
- `updateRule`: superuser only
- `deleteRule`: superuser only

---

### 15.2 Bootstrap & migration (`main.go`)

```go
func ensureGlobalSetup(app core.App) error {
    coll, err := app.Dao().FindCollectionByNameOrId("_global_setup")
    if err != nil {
        coll = core.NewBaseCollection("_global_setup")
        // add fields, set rules per above
        if err := app.Dao().SaveCollection(coll); err != nil {
            return err
        }
    }
    recs, _ := app.Dao().FindRecordsByFilter("_global_setup", "name = 'default'", "", 1, 0)
    if len(recs) == 0 {
        rec := core.NewRecord(coll)
        rec.Set("name", "default")
        rec.Set("description", "Global PBX configuration")
        migrateFromFiles(app, rec)
        return app.Dao().SaveRecord(rec)
    }
    return nil
}

func migrateFromFiles(app core.App, rec *core.Record) {
    // mssql.json
    if data, _ := os.ReadFile(mssqlConfigPath(app)); len(data) > 0 {
        var m struct{ DSN string `json:"dsn"` }
        if json.Unmarshal(data, &m) == nil && m.DSN != "" {
            rec.Set("_dsn", map[string]string{"dsn": m.DSN})
        }
    }
    // theme.json
    if data, _ := os.ReadFile(themeFilePath(app)); len(data) > 0 {
        var m struct{ Mode string `json:"mode"` }
        if json.Unmarshal(data, &m) == nil && (m.Mode == "light" || m.Mode == "dark") {
            rec.Set("_theme", map[string]string{"mode": m.Mode})
        }
    }
    // lang.json
    if data, _ := os.ReadFile(langFilePath(app)); len(data) > 0 {
        var m struct{ Lang string `json:"lang"` }
        if json.Unmarshal(data, &m) == nil && i18n.IsValid(m.Lang) {
            rec.Set("_lang", map[string]string{"lang": m.Lang})
        }
    }
}
```

Call `ensureGlobalSetup(app)` in `OnServe` hook after `app.Bootstrap()`.

---

### 15.3 New helpers (`main.go`) — replace file I/O

```go
func getGlobalSetup(app core.App) (*core.Record, error) {
    return app.Dao().FindFirstRecordByFilter("_global_setup", "name = 'default'")
}

func getThemeMode(app core.App) string {
    rec, _ := getGlobalSetup(app)
    if rec == nil { return "light" }
    if v, ok := rec.Get("_theme").(map[string]any); ok {
        if m, ok := v["mode"].(string); ok && (m == "light" || m == "dark") {
            return m
        }
    }
    return "light"
}

func setThemeMode(app core.App, mode string) error {
    return app.Dao().RunInTransaction(func(txDao *core.Dao) error {
        rec, _ := getGlobalSetup(app)
        rec.Set("_theme", map[string]string{"mode": mode})
        return txDao.SaveRecord(rec)
    })
}

func getLangCode(app core.App, r *http.Request) string {
    if r != nil {
        if c, err := r.Cookie("pb_lang"); err == nil && i18n.IsValid(c.Value) {
            return c.Value
        }
    }
    if cliLangOverride && i18n.IsValid(cliLang) {
        return cliLang
    }
    rec, _ := getGlobalSetup(app)
    if rec == nil { return "en" }
    if v, ok := rec.Get("_lang").(map[string]any); ok {
        if l, ok := v["lang"].(string); ok && i18n.IsValid(l) {
            return l
        }
    }
    return "en"
}

func setLangCode(app core.App, code string) error {
    code = i18n.Normalize(code)
    if !i18n.IsValid(code) { return fmt.Errorf("invalid language %q", code) }
    return app.Dao().RunInTransaction(func(txDao *core.Dao) error {
        rec, _ := getGlobalSetup(app)
        rec.Set("_lang", map[string]string{"lang": code})
        return txDao.SaveRecord(rec)
    })
}

func getMssqlDSN(app core.App) string {
    rec, _ := getGlobalSetup(app)
    if rec == nil { return "" }
    if v, ok := rec.Get("_dsn").(map[string]any); ok {
        if d, ok := v["dsn"].(string); ok {
            return d
        }
    }
    return ""
}

func setMssqlDSN(app core.App, dsn string) error {
    return app.Dao().RunInTransaction(func(txDao *core.Dao) error {
        rec, _ := getGlobalSetup(app)
        rec.Set("_dsn", map[string]string{"dsn": dsn})
        return txDao.SaveRecord(rec)
    })
}
```

---

### 15.4 Update API endpoints (`routes.go`)

| Endpoint | Add superadmin check |
|----------|---------------------|
| `POST /api/theme/{mode}` | `if err := requireSuperAdmin(e); err != nil { return err }` |
| `POST /api/lang/{code}` | `if err := requireSuperAdmin(e); err != nil { return err }` |
| `POST /api/mssql-dsn` | `if err := requireSuperAdmin(e); err != nil { return err }` |

(Already have `requireSuperAdmin` from Phase 14 — reuse it.)

---

### 15.5 Page handlers (`main.go`)

**No changes needed** — all 13 theme handlers and 16+ lang handlers already call `getThemeMode(e.App)` / `getLangCode(e.App, e.Request)` / `langData(e)` which now read from DB.

Templates continue to receive `.Theme` and `.Lang` fields unchanged.

---

### 15.6 Cleanup (after verification)

- Delete `themeFilePath()`, `langFilePath()`, `mssqlConfigPath()`
- Delete old `setThemeMode()`, `setLangCode()`, `setMssqlDSN()` file-based implementations
- Remove `theme.json`, `lang.json`, `mssql.json` from `.gitignore`
- Optionally delete physical files from `pb_data/`

---

### 15.7 File change summary

| File | Action | Lines (est.) |
|------|--------|-------------|
| `main.go` | Add `ensureGlobalSetup`, new helpers, remove old file I/O | +80 |
| `routes.go` | Add superadmin checks to 3 POST endpoints | +6 |
| `pb_data/` | (runtime) files removed after migration | — |

Total: ~86 new lines, ~50 removed lines.

---

### 15.8 Verification

- `go build ./... && go vet ./... && go test ./...`
- Theme toggle persists across reloads (DB read/write)
- Language toggle persists across reloads (DB read/write)
- MSSQL DSN save/load works in export/import modals
- Non-superadmin POST to config endpoints → 403
- Fresh DB with existing `pb_data/*.json` → values migrated on first run
- Delete `_global_setup` record → app falls back to defaults (light/en)

---

# PBX Development Plan - Phase 17
### done!
## Extend AI Agent Toolset for Collections, Views, and Records

### Objective
Extend the built-in AI agent (`pbai`) with 7 new tools to enable full CRUD operations on Collections, Views, and Records while respecting PocketBase collection rules.

---

### Current Toolset (8 tools)

| Tool | Purpose | Write? | Auth |
|------|---------|--------|------|
| `list_collections` | List accessible collections | No | User |
| `get_collection_schema` | Show collection fields | No | User |
| `query_records` | Query records (filter, limit) | No | User |
| `insert_records` | Insert new records | Yes | User |
| `create_collection` | Create new collection | Yes | Superuser |
| `set_view_config` | Create/update view config | Yes | Superuser |
| `create_action` | Create/update custom action | Yes | Superuser |
| `list_actions` | List custom actions | No | Superuser |

---

### New Tools (7 tools) - Phase 17

#### Collections (3 tools)

| Tool | Parameters | Behavior |
|------|------------|----------|
| `update_collection` | `{collection, addFields: [{name, type, options?, required?}], removeFields: [string]}` | Add/remove fields only; **no type changes**; validates no duplicates; reconstructs schema |
| `delete_collection` | `{collection, force?: false}` | Deletes base collection; **warns if `_views` configs reference it**; requires `force=true` to override |
| `set_collection_rules` | `{collection, listRule?, viewRule?, createRule?, updateRule?, deleteRule?}` | Full replace of rule strings (nil/empty/raw filter); validates filter syntax |

#### Views (2 tools) - `_views` collection

| Tool | Parameters | Behavior |
|------|------------|----------|
| `update_view_config` | `{configName, collection?, pageTitle?, columnTitles?, columnSorting?, searchBox?, pagination?, displaySystemCol?, filter?, formTitle?, formDescr?, formLabels?, formLayout?, columnOrder?, displaySystemCol?, mssql?}` | **Full replace** of JSON fields (`_tabulator`, `_form`, `_mssql`); finds by `_name` |
| `delete_view_config` | `{configName}` | Deletes `_views` record by `_name` |

#### Records (2 tools) - extends existing

| Tool | Parameters | Behavior |
|------|------------|----------|
| `update_records` | `{collection, records: [{id, field: value, ...}]}` | Max **50 records**; per-record `updateRule` check via `CanAccessRecord`; value coercion |
| `delete_records` | `{collection, ids: [string]}` | Max **50 IDs**; per-record `deleteRule` check |
| `query_records` (enhanced) | Add: `sort: "label1,-label2"`, `offset: 0`, `fields: "col1,col2"` | Sort by **column labels** (from view config); offset for pagination; field projection |

---

### Rule Enforcement Model

| Operation | Rule Checked | Superuser Bypass |
|-----------|--------------|------------------|
| List collections | listRule | Yes |
| View schema/records | viewRule | Yes |
| Create records | createRule | Yes |
| Update records | updateRule (per record) | Yes |
| Delete records | deleteRule (per record) | Yes |
| Manage collections/views/actions | N/A (superuser only) | N/A |

**Key Pattern**: For record-level rules (update/delete):
```go
ok, err := a.App.CanAccessRecord(rec, a.Info, rec.Collection().UpdateRule)
// or DeleteRule
```

---

### Implementation Details

#### 1. Access Helpers (tools.go)
```go
func (a *Agent) canUpdate(coll *core.Collection) bool
func (a *Agent) canDelete(coll *core.Collection) bool
```

#### 2. Collection Update Logic
- Get collection via `FindCachedCollectionByNameOrId`
- For `addFields`: create new `core.Field` instances, append to `coll.Fields`
- For `removeFields`: filter out from `coll.Fields` (skip system fields)
- **Reject** if attempting to change type of existing field
- Save with `a.App.Save(coll)`

#### 3. View Config Full Replace
- Find `_views` record by `_name` (configName)
- Rebuild entire `_tabulator` / `_form` / `_mssql` JSON from params
- Similar to `set_view_config` but all params optional

#### 4. Record Sort by Labels
- Fetch view config for collection to get column label mapping
- Translate label sort string (`"Name,-Date"`) → field sort string (`"name,-created"`)
- Pass to `FindRecordsByFilter`

#### 5. Cascade Warning for Collection Delete
```go
recs, _ := a.App.FindRecordsByFilter("_views", "_collName = {:c}", "", 1, 0, dbx.Params{"c": collName})
if len(recs) > 0 && !force {
    return fmt.Errorf("collection has %d view config(s); use force=true to delete", len(recs))
}
```

---

### Files to Modify

| File | Changes |
|------|---------|
| `pbai/tools.go` | 7 new tool functions + register in `allTools()` + access helpers |
| `pbai/agent.go` | Update `systemMessages()` with new tool docs |
| `pbrules/rules.go` | (Optional) `CheckUpdateRule`/`CheckDeleteRule` helpers |

---

### Testing Checklist

- [ ] `update_collection`: add field, remove field, reject type change, reject duplicate name
- [ ] `delete_collection`: succeeds when no views, warns with views, force overrides
- [ ] `set_collection_rules`: all 5 rules, nil/empty/string validation
- [ ] `update_view_config`: partial params merge correctly
- [ ] `delete_view_config`: removes record
- [ ] `update_records`: 1-50 records, updateRule enforced, coercion works
- [ ] `delete_records`: 1-50 IDs, deleteRule enforced
- [ ] `query_records`: sort by labels, offset, fields projection
- [ ] Non-superuser blocked appropriately on all write tools

---

### Estimated Effort

| Task | Complexity |
|------|------------|
| Access helpers + rule checks | Low |
| Collection CRUD tools | Medium |
| View config tools | Low |
| Record update/delete tools | Medium |
| Sort by labels translation | Medium |
| System prompt updates | Low |
| Tests | Medium |

**Total**: ~300-400 lines of new code across 2-3 files.

---

### Notes
- Phase 17 follows Phase 14 (Security posture - CSRF/rate limiting) and precedes future phases
- Tools use existing pending action pattern (write=true, confirmation required)
- All write operations respect collection rules via existing `pbrules` package patterns

---

## Phase 18 — AI Agent Integration in Tabular & Form Views
### done!

Context: The existing AI agent lives at `/ai` in a dedicated chat interface. Users want to interact with AI directly from the data views (`/tabular/{name}` and `/form/{name}`) to search, filter, add, edit, and delete records without leaving the view. The AI manipulates the view directly — search results filter the existing table, writes show inline confirmations. The existing `/ai` chat agent remains completely untouched.

Decisions confirmed with user:
- UI placement: input bar at the top of the view (below toolbar/title)
- Write safety: inline confirmation (approve/reject) before executing writes
- Result display (tabular): filter existing table in-place — replace `allRecords` and re-render
- Result display (form): search results shown in a modal overlay within the form (not a redirect)
- Restricted tool set: query_records, insert_records, update_records, delete_records only (no collection/schema operations)

### 18.1 Backend — Agent tool filtering (`pbai/agent.go`)

Add to `Agent` struct:
```go
allowedTools []string  // nil = all tools (existing behavior); set = view agent mode
viewCollection string  // collection context for view agent
viewConfigName string  // config name for view agent
```

New constructor:
```go
func NewViewAgent(app core.App, info *core.Record, cfg AgentConfig, collection, configName string) *Agent
```
Sets `allowedTools` to `[]string{"query_records", "insert_records", "update_records", "delete_records"}`.

New method `viewSystemMessages()` — generates a focused system prompt:
- "You are embedded in a data view of collection `{collection}` (config: `{configName}`)."
- Tools: query_records (read), insert/update/delete_records (write, confirmation required).
- For **search/filter** requests: call `query_records` — the UI displays returned records directly as table rows. Keep text answers minimal.
- For **write** requests: call the appropriate tool — the system will ask for user confirmation before executing.
- Enforce collection rules; respect the caller's permissions.
- Answer in the user's language.
- Do not invent record contents.

Modify `RunStream` to:
- Use `viewSystemMessages()` when `allowedTools` is set (instead of `systemMessages()`)
- Filter `toolDefs()` to only include tools named in `allowedTools`
- Otherwise identical loop logic

**Existing agent untouched**: `NewAgent()` sets `allowedTools = nil`, all existing behavior preserved.

### 18.2 Backend — View chat endpoint (`main.go`)

New handler `handleViewChatStream` (~70 lines):

```
POST /ai/view-chat/stream
Request:  { messages: [{role,content}], collection: "collName", configName: "viewName" }
Response: SSE stream — text | status | done { records?, pendingAction?, finalText? }
```

- Same auth pattern as existing `handleAgentChatStream` (cookie auth via `agentRequestInfo`)
- Creates `pbai.NewViewAgent(app, info, cfg, collection, configName)`
- Streams SSE identically to existing endpoint
- The `done` event carries `records` (from `query_records` results) and/or `pendingAction` (from write tools)

Key difference from chat agent: Write tools **do not halt the loop**. Instead, when a write tool is called:
1. The pending action is created and returned in the `done` event
2. The loop stops (same as current behavior — the `done` event is emitted)
3. The user confirms via the existing `POST /ai/confirm` endpoint
4. On confirm, the write executes and returns a result message

This means **at most one write per request**, same as the chat agent. The LLM is instructed in the system prompt to do one operation at a time.

### 18.3 Backend — Route registration (`routes.go`)

Add to `registerAIRoutes()`:
```go
se.Router.POST("/ai/view-chat/stream", rateLimitMiddleware(aiChatRateLimiter)(handleViewChatStream))
```

The existing `POST /ai/confirm` endpoint works unchanged — it already resolves the pending action and executes it.

### 18.4 Frontend — Tabular view (`views/tabulator.html`)

#### HTML additions

**AI input bar** — inserted between toolbar row (line 121) and table (line 123):
```html
<div class="ai-bar" id="aiBar">
    <span class="ai-icon">✨</span>
    <input type="text" id="aiInput" placeholder="..." autocomplete="off">
    <button id="aiSendBtn" class="btn-sm btn-primary">AI</button>
    <button id="aiClearBtn" class="btn-sm btn-neutral" style="display:none;">✕</button>
</div>
<div id="aiStatus" class="ai-status" style="display:none;"></div>
<div id="aiConfirm" class="ai-confirm" style="display:none;"></div>
```

#### CSS additions (to `<style>` block)

```css
.ai-bar { display:flex; align-items:center; gap:8px; margin-bottom:12px; padding:8px 12px;
          background:var(--card-bg); border:2px solid var(--accent); border-radius:8px; }
.ai-bar input { flex:1; padding:8px 12px; border:1px solid var(--input-border); border-radius:6px;
                font-size:.9rem; outline:none; background:var(--body-bg); color:var(--text); }
.ai-bar input:focus { border-color:var(--accent); }
.ai-icon { font-size:1.1rem; }
.ai-status { padding:6px 12px; margin-bottom:8px; font-size:.85rem; color:var(--muted);
             background:var(--card-bg); border-radius:6px; border-left:3px solid var(--accent); }
.ai-confirm { padding:10px 14px; margin-bottom:12px; background:var(--accent-soft);
              border:1px solid var(--accent); border-radius:8px; display:flex; align-items:center; gap:12px; }
.ai-confirm .ai-confirm-text { flex:1; font-size:.9rem; }
.ai-confirm button { padding:6px 16px; border:none; border-radius:6px; font-size:.85rem; cursor:pointer; }
```

#### JS additions

State variables:
```js
var aiBusy = false;
var aiPendingAction = null;
var aiOriginalRecords = null;  // saved before AI filter
```

Functions:
- **`sendAI()`** — Reads `#aiInput` value, builds `[{role:"user", content:text}]`, POSTs to `/ai/view-chat/stream` with `{messages, collection:"{{.CollectionName}}", configName:"{{.ConfigName}}"}`, parses SSE stream via `ReadableStream` (same pattern as `agent.html`). Shows spinner in `#aiStatus`.
- **`handleAIEvent(ev)`** — Dispatches: `text` → updates `#aiStatus` with streaming text; `status` → shows tool execution notice; `done` → calls `applyAIResult(ev.result)`; `error` → shows error.
- **`applyAIResult(result)`** — If `result.records`: saves original `allRecords` on first filter, replaces with AI records, calls `render()`, shows "AI filtered • N records • Clear" in `#aiStatus`, shows clear button. If `result.pendingAction`: shows inline confirm bar.
- **`showAIConfirm(action)`** — Populates `#aiConfirm` with summary text + Approve/Reject buttons.
- **`confirmAI(approved)`** — POSTs to `/ai/confirm`, on success shows result in `#aiStatus`, clears `aiPendingAction`, if write succeeded refreshes table via `fetchTabulatorPage()`.
- **`clearAIFilter()`** — Restores `aiOriginalRecords` to `allRecords`, calls `render()`, hides status and clear button.
- **`fetchTabulatorPage()`** — Refetches page data from `/api/tabulator-data/{configName}` and refreshes `allRecords` + `render()`. Needed after AI writes to show the updated data.

Enter key on `#aiInput` triggers `sendAI()`.

### 18.5 Frontend — Form view (`views/form.html`)

#### HTML additions

Same AI bar as tabular, inserted after the title row (line 60), before the description.

#### CSS additions

Same as tabular (duplicated inline — matches existing template pattern).

#### JS additions

Similar to tabular but with form-specific behavior:

- **`sendAI()`** — Same SSE flow, sends `collection:"{{.CollectionName}}"`, `configName:"{{.ConfigName}}"`.
- **`handleAIEvent(ev)`** — Same text/status/done dispatch.
- **`applyAIResult(result)`**:
  - **Search results (records)**: Opens a modal overlay displaying the records as a scrollable table. The modal uses the existing `.modal-overlay` pattern. Each row has a "Select" button that navigates to `/form/{configName}/{recordId}` for editing. Modal has a Close button.
  - **Write results**: Shows success/error in `#aiStatus`. For insert: shows "Created. View in table" link. For update: shows "Updated" with the record data. For delete: shows "Deleted" with a link to table.
  - **Pending action**: Shows inline confirm bar (same as tabular).

#### Search results modal

```html
<div class="modal-overlay" id="aiSearchModal">
    <div class="modal-content" style="max-width:90vw;">
        <div class="modal-header">
            <h3 id="aiSearchTitle">AI Search Results</h3>
            <button class="modal-close" onclick="closeAISearchModal()">&times;</button>
        </div>
        <div class="modal-body" id="aiSearchBody" style="max-height:70vh;overflow-y:auto;"></div>
    </div>
</div>
```

The `openAISearchModal(records)` function builds a table from the records (using the same `renderCell`-like logic) and displays it. Each row has a link `<a href="/form/{configName}/{id}">Edit</a>`.

### 18.6 Mobile variants (`views/mobile-tabulator.html`, `views/mobile-form.html`)

Same AI bar HTML + CSS + JS logic adapted for mobile layout. The mobile tabulator uses cards instead of tables, so AI search results render as cards. The mobile form modal is full-width.

### 18.7 i18n keys

**`i18n/en.json`** additions:
```json
"tabulator.aiPlaceholder": "Ask AI to search, add, edit or delete records\u2026",
"tabulator.aiThinking": "AI is thinking\u2026",
"tabulator.aiFiltered": "AI filtered \u2022 %d records",
"tabulator.aiClear": "Clear AI filter",
"tabulator.aiConfirmTitle": "AI wants to modify data",
"tabulator.aiApprove": "Approve",
"tabulator.aiReject": "Reject",
"tabulator.aiDone": "Done: %s",
"tabulator.aiFailed": "AI action failed: %s",
"tabulator.aiSearchTitle": "AI Search Results",
"form.aiPlaceholder": "Ask AI to fill fields, search or save\u2026",
"form.aiThinking": "AI is thinking\u2026",
"form.aiSearchTitle": "AI Search Results",
"form.aiSelect": "Select",
"form.aiCreated": "Record created.",
"form.aiUpdated": "Record updated.",
"form.aiDeleted": "Record deleted.",
"form.aiToTable": "View in table"
```

**`i18n/cs.json`** — corresponding Czech translations.

### 18.8 File change summary

| File | Action | Lines (est.) |
|------|--------|-------------|
| `pbai/agent.go` | Edit — `allowedTools` field, `NewViewAgent()`, `viewSystemMessages()`, tool filtering in `RunStream` | +80 |
| `main.go` | Edit — `handleViewChatStream` handler | +70 |
| `routes.go` | Edit — register `POST /ai/view-chat/stream` | +1 |
| `views/tabulator.html` | Edit — AI bar HTML + CSS + JS | +120 |
| `views/form.html` | Edit — AI bar HTML + CSS + JS + search results modal | +150 |
| `views/mobile-tabulator.html` | Edit — mobile AI bar variant | +100 |
| `views/mobile-form.html` | Edit — mobile AI bar variant | +100 |
| `i18n/en.json` | Edit — ~18 new keys | +18 |
| `i18n/cs.json` | Edit — ~18 new keys | +18 |

Total: ~657 new lines, ~20 modified lines.

### 18.9 What stays completely untouched

- `views/agent.html` — existing chat UI
- `main.go` handlers: `handleAgentChatStream`, `handleAgentConfirm`, `handleAgentChat`, `handleAgent` — all unchanged
- `pbai/tools.go` — all 15 tools unchanged
- `pbai/render.go` — server-side HTML rendering unchanged
- `pbai/llm.go`, `pbai/ingest.go` — unchanged

### 18.10 Implementation order

1. `pbai/agent.go` (backend foundation — tool filtering + view system prompt)
2. `main.go` + `routes.go` (new endpoint)
3. `i18n/en.json` + `i18n/cs.json` (translations)
4. `views/tabulator.html` (tabular UI)
5. `views/form.html` (form UI + search modal)
6. `views/mobile-tabulator.html` + `views/mobile-form.html` (mobile)
7. Build & test: `go build ./... && go vet ./...`

### 18.11 Verification

- `go build ./... && go vet ./...`
- Navigate to `/tabular/{name}` → AI input bar visible below toolbar
- Type "show me all records" → table filtered to AI results, status bar shows count, clear button visible
- Click "Clear AI filter" → original records restored
- Type "add a record with name=Test" → inline confirm bar appears, approve → record created, table refreshes
- Type "delete record with id=xxx" → inline confirm bar, approve → record deleted, table refreshes
- Navigate to `/form/{name}` → AI input bar visible
- Type "show me all customers" → modal opens with records table, click "Select" → navigates to that record's form
- Type "save this record" (on a new form with data filled) → inline confirm, approve → record created
- Mobile: `/mobile/tabular/{name}` and `/mobile/form/{name}` → AI bar renders correctly
- Non-superuser: AI respects collection rules (cannot query collections they can't list, cannot write without create/update/delete rules)
- Existing `/ai` chat agent: completely unchanged and functional

### 18.12 Dependencies

- Depends on Phase 17 (update_records, delete_records tools must exist)
- Depends on Phase 12 (i18n — for translation keys)
- Depends on Phase 8 (mobile templates — for mobile variants)