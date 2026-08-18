# Assets

Web app to provide UI to view and edit pocketbase collections data. PocketBase custom Go build.

## Stack

- **Runtime**: PocketBase v0.39 custom Go build (`./pbx`)
- **Frontend**: server-rendered Go templates (`views/*.html`)
- **Backend**: Go on port 8090
- **DB**: SQLite via `pb_data/data.db` (gitignored; only `pb_data/types.d.ts` is tracked)
- **CSS**: `assets/w3.css` (W3.CSS v5.01). `assets/myapp.css` does not exist — do not reference it.
- **Module**: `pbx`; Go 1.26.3

## Commands

| Action | Command |
|--------|---------|
| Build & vet | `go build ./... && go vet ./...` |
| Run server | `./pbx serve` (listens on :8090) |

No CI, no tests.

## Routes (all in `main.go`)

| Route | Handler | Purpose |
|-------|---------|---------|
| `GET /login` | login form | Renders login.html |
| `POST /login` | authenticate | Email/password auth via `users` collection, sets `pb_auth` cookie |
| `GET /app` | dashboard | Menu of links grouped by `_app` collection records |
| `GET /pbx-setup` | setup | Shows `_app`, `_views` tables + Theme setup |
| `GET /pbx-config` | config editor | List `_views` configs |
| `GET /pbx-config/view/new|{name}` | config editor | Edit `_views` `_tabulator`/`_form`/`_mssql` JSON |
| `POST /pbx-config/save` | config save | Persist `_views` record |
| `POST /pbx-config/delete` | config delete | Delete `_views` record |
| `GET/POST /pbx-config/import-excel` | import wizard | 3-step wizard: introspect Excel → preview schema → create collection (+ optional data import) |
| `GET/POST /pbx-config/import-mssql` | import wizard | 3-step wizard: introspect MSSQL table → preview schema → create collection (+ optional data import) |
| `GET /tabular/{configName}` | table view | All records as client-side JSON; 20/page, sort, search |
| `GET /form/{configName}` | form view | New record form, layout configured via `_views` `_form` |
| `GET /form/{configName}/{id}` | form view | Edit existing record |
| `POST /form/{configName}` | form submit | Create record |
| `POST /form/{configName}/{id}` | form submit | Update record |
| `POST /form/{configName}/{id}/delete` | delete record | Delete record, returns JSON |
| `POST /api/theme/{mode}` | set theme | Persist global default theme (`light`/`dark`) to `pb_data/theme.json` |
| `POST /api/mssql-dsn` | set MSSQL DSN | Persist global default MSSQL DSN to `pb_data/mssql.json` |
| `GET /api/tabulator-data/{collectionName}` | JSON API | Raw JSON for relation modal |
| `GET /export/{collectionName}` | export | Export to Excel via `pbexcel` |
| `POST /import/{collectionName}` | import | Import from Excel via `pbexcel` |
| `POST /mssql-export/{collectionName}` | MSSQL export | Push PocketBase records to a MSSQL table (JSON response) |
| `POST /mssql-import/{collectionName}` | MSSQL import | Pull rows from a MSSQL table into PocketBase (JSON response) |
| `GET /mssql-introspect` | MSSQL introspect | List MSSQL table columns from `INFORMATION_SCHEMA` |
| `GET /assets/{path...}` | static | Serves `views/assets/` files |

**Auth**: Cookie-based `pb_auth` (JWT via PocketBase). Login uses `FindAuthRecordByEmail("users", ...)` — name field is the email.

## Template functions

Defined in `main.go` (used in `views/*.html`): `add`, `sub`, `seq`, `safeJS`, `safeHTML`.

## Theme

- Global default stored in `pb_data/theme.json` (`{"mode":"light"|"dark"}`); set via `POST /api/theme/{mode}` (also from the `/pbx-setup` Theme section or the topbar switch on any page).
- Every page data struct has a `Theme` field; templates render `<body data-theme="{{.Theme}}">` and link `/assets/theme.css` (light + `[data-theme="dark"]` variable sets).
- The topbar switch also stores a per-browser override in `localStorage` key `pbx-theme`; on load the override wins, otherwise the server default applies.
- `login.html` uses its own `:root` variables plus the shared `theme.css`.

## `_views` collection (unified config)

A record with `_name={configName}` configures BOTH `/tabular/{configName}` and `/form/{configName}` for the collection named in `_collName`. Holds all settings that used to live in the separate `_tabulator` and `_form` collections:

- `_name` — configuration name (endpoint `/tabular/{_name}`, `/form/{_name}`)
- `_collName` — collection the view is configured for
- `_tabulator` — JSON with tabulator (list) settings:
  - `pageTitle` — custom `<h1>` heading (falls back to collection name)
  - `collectionDescr` — italic text below record count
  - `columnTitles` — comma-delimited override for column headers
  - `columnOrder` — comma-delimited 1-based absolute field indices
  - `displaySystemCol` — if false, hides `id`, `created`, `updated`
  - `columnSorting` — if true, clickable sort (↕→▲→▼)
  - `searchBox` — if true, search input filters across all columns
  - `pagination` — if true, « ‹ [input] › » controls
  - `filter` — filter expression for records
  - `columns` — optional `[{field, title}]` column list override
- `_form` — JSON with form settings:
  - `formTitle` — custom heading (falls back to collection name)
  - `formDescr` — description paragraph below heading
  - `displaySystemCol` — if true, shows `id`, `created`, `updated` as read-only
  - `columnOrder` — comma-delimited 1-based field indices (when no `formLayout`)
  - `formLayout` — slash-delimited rows, parentheses for column groups: `"row:(1,2) (3,4) / row:(5,6)"` (0-based internally, 1-based in config)
  - `formLabels` — comma-delimited `field=Label` pairs (e.g. `"name=Jméno,email=E-mail"`)
  - `layout` / `labels` / `collections` — optional structured form JSON
- `_mssql` — JSON config `{dsn, table, mode, mapping:[{pbField,dbField}]}` enabling the MSSQL Sync modal (DSN falls back to the global DSN from `/pbx-setup`)

The `/pbx-config` editor at `/pbx-config/view/{name}` edits the `_tabulator`, `_form` and `_mssql` JSON fields of a `_views` record.

## MSSQL sync

- Package `pbmssql/pb-mssql.go` provides `ExportToMSSQL`, `ImportFromMSSQL`, `IntrospectTable`, `TableExists`, `CreateTable` with a DSN-keyed connection pool (driver `sqlserver`).
- Global default DSN persisted in `pb_data/mssql.json`, editable from `/pbx-setup` via `POST /api/mssql-dsn`.
- `mode` semantics (insert/update/replace) mirror the Excel importer; unique single-column indexes are used to match records on import.
- Export requires user confirmation before creating a missing table: `ExportToMSSQL` returns `ErrTableMissing`, the handler replies with HTTP 409 + `{tableMissing:true}`, the UI prompts the user, and only after confirm does the request re-send with `createTable=1` (which calls `pbmssql.CreateTable`, seeding columns from PB field types).

## `_app` collection

Configures the `/app` dashboard. Fields:

- `group` / `group_label` — links are grouped under a heading
- `group_icon` — uploaded icon file for the group
- `collection` / `collectionLabel` — link target (`/tabular/{configName}`, falls back to default config then collection name) and display text

## Collections (from `_app`, `_tabulator`, `_form` records in DB)

`zamestnanci`, `produkty`, `karta_majetku`, `inventury`, `inv_radky`, `cinnosti`, `mapa_umisteni`, `umisteni`, `organizacni_struktura`, `kat_produktu`, `definice_stitku`, `poznamky`. System: `users` (auth), `roles`, `_views`, `_app`, `_metadata`.

## Schema changes

Add/modify collection fields via **JS SDK** in `pb_migrations/`. See `.opencode/skills/pocketbase-api-add-field/SKILL.md`.

`pb_data/types.d.ts` is auto-generated — do not edit manually.

## Conventions

- `pb_migrations/` — JS migrations, auto-applied on `serve`
- `pb_hooks/` — does not exist; do not reference it
- `pb_public/` — does not exist; do not reference it
- `views/assets/` — icons (PNG) and `theme.css`; served via embedded FS
- `pbexcel/` — Excel import/export logic (`pb-excel.go`); also `IntrospectSheet` (header/type detection) for the collection wizard
- `pbmssql/` — MSSQL import/export/introspection logic (`pb-mssql.go`)
- `views/import-wizard.html` — shared 3-step wizard: Source → Preview/Edit → Create (used by both Excel and MSSQL collection import)
- `views/pages.go` — Go structs for template data (`TabulatorPageData`, `FormPageData`, `AppPageData`, `ImportWizardPageData`, etc.)
- No `README.md` exists

## Collection creation wizard

`/pbx-config/import-excel` and `/pbx-config/import-mssql` (super-admin) create a new base collection from a source:

1. **Source** — collection name + Excel file/sheet or MSSQL DSN/table.
2. **Preview/Edit** — introspected columns (`pbexcel.IntrospectSheet` / `pbmssql.IntrospectTable`) with inferred PB types (text/number/bool/date); names/types editable, columns can be skipped.
3. **Create** — `core.NewBaseCollection` + fields from detected schema → `app.Save`; optionally imports the source data immediately (with `ImportFromExcel` mapping or `pbmssql.ImportFromMSSQL`).

Field/collection names are normalized to lowercase `[a-z0-9_]` (reserved system names `id`/`created`/`updated`/`collectionid`/`collectionname`/`expand` are skipped).
