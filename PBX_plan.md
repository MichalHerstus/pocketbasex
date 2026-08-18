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

## Future development 
- LUA scripting
- reporting
- mobile access (optimized views for mobile phone/tablet)
- AI built-in agent for operations (insert record from paln text/PDF/md/image, create collection and configuration, summary of data etc.). Use LLMs through OpenRouter.
  
**Not in this iteration (later phases):** Lua actions, card/kanban/detail/report views, print-to-PDF, MySQL/Postgres support.
