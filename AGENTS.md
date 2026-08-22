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

No CI, no test runner. `pbai/agent_test.go` has unit tests (run with `go test ./pbai/`).

## Routes (all in `main.go`)

| Route | Handler | Purpose |
|-------|---------|---------|
| `GET /login` | login form | Renders login.html |
| `POST /login` | authenticate | Email/password auth via `users` collection, sets `pb_auth` cookie |
| `GET /app` | dashboard | Menu of links grouped by `_app` collection records |
| `GET /pbx-setup` | setup | Shows `_app`, `_views` tables + Theme setup |
| `GET /pbx-setup/record/{coll}/new` | setup record editor | New record form for `_app`/`_views`/`_agent` (superadmin) |
| `GET /pbx-setup/record/{coll}/{id}` | setup record editor | Edit record for `_app`/`_views`/`_agent` (superadmin) |
| `POST /pbx-setup/record/{coll}` | setup record create | Create system record (multipart; file fields via `filesystem.NewFileFromMultipart`) |
| `POST /pbx-setup/record/{coll}/{id}` | setup record update | Update system record (`{field}_remove=on` clears a file) |
| `POST /pbx-setup/record/{coll}/{id}/delete` | setup record delete | Delete system record, returns JSON |
| `POST /pbx-setup/rules` | setup rules save | Persist collection API rules for data collections (superadmin) |
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
| `POST /api/lang/{code}` | set UI language | Persist global default UI language (`en`/`cs`) to `pb_data/lang.json` |
| `GET /api/lang/{code}/catalog.js` | i18n catalog | JavaScript exposing `window._t(key)` + merged translations for client-side strings |
| `GET /api/tabulator-data/{collectionName}` | JSON API | Raw JSON for relation modal |
| `GET /export/{collectionName}` | export | Export to Excel via `pbexcel` |
| `POST /import/{collectionName}` | import | Import from Excel via `pbexcel` |
| `POST /mssql-export/{collectionName}` | MSSQL export | Push PocketBase records to a MSSQL table (JSON response) |
| `POST /mssql-import/{collectionName}` | MSSQL import | Pull rows from a MSSQL table into PocketBase (JSON response) |
| `GET /mssql-introspect` | MSSQL introspect | List MSSQL table columns from `INFORMATION_SCHEMA` |
| `GET /api/filters/{configName}` | saved filters | List named advanced filters for a `/tabular` view (own filters, or all for superusers) |
| `POST /api/filters/{configName}` | save filter | Upsert a named filter (`{name, conditions, chains}`) owned by the caller |
| `DELETE /api/filters/{id}` | delete filter | Delete a saved filter (own, or any for superusers) |
| `GET /api/ai-config` | agent config (super-admin) | Read `_agent` config (JSON) |
| `POST /api/ai-config` | agent config (super-admin) | Save `_agent` config (JSON) |
| `GET /api/ai/status` | agent status | `{configured, provider, model, enabled}` |
| `GET /ai` | AI agent chat | Chat UI (`agent.html`), linked from `/app` |
| `POST /ai/chat` | AI agent chat | Run agent loop; returns `{transcript, finalText, pendingAction?, records?, render?}` — `records` is the last `query_records` dataset; `render` is the server-rendered HTML fragment the chat UI injects for the turn. Tabular fast path: when `query_records` is the first and only executed tool call of the turn and returns ≥1 record, the response returns immediately without a follow-up LLM call (`finalText` empty) |
| `POST /ai/chat/stream` | AI agent chat (SSE) | Streaming variant of `/ai/chat`: emits `text` deltas live, `status` notices per tool execution, then one final `done` event whose `result` payload equals the non-streaming response (`error` event on failure). Internally the agent always talks to the LLM via `Client.Stream` (`pbai/llm.go`) — even `/ai/chat` |
| `POST /ai/confirm` | AI agent confirm | Approve/reject a pending write action (`{actionID, approved}`) |
| `GET /api/actions/{collectionName}` | list actions | Actions for a collection (all for superusers, `_public` only otherwise) |
| `POST /actions/execute` | run action | Execute a custom action (`{actionID, recordIds}`) → `{ok, output[], affected}` |
| `GET /assets/{path...}` | static | Serves `views/assets/` files |

**Auth**: Cookie-based `pb_auth` (JWT via PocketBase). Login uses `FindAuthRecordByEmail("users", ...)` — name field is the email. NB: PocketBase's `loadAuthToken` middleware only reads the `Authorization` header, NOT the `pb_auth` cookie, so `e.RequestInfo().Auth` is nil on custom routes. The AI handlers resolve the cookie manually via `agentRequestInfo()` (main.go).

## Collection API rules enforcement

- Rules are stored as nullable TEXT on each collection (`listRule`/`viewRule`/`createRule`/`updateRule`/`deleteRule`): `nil` = superusers only, `""` = public, else a PB filter expression.
- The `/pbx-setup` **Collection API rules** editor (superadmin, `POST /pbx-setup/rules`) offers 5 modes per rule per data collection (excludes `users`, `roles`, `_`-prefixed, and view collections): Public (`""`), Signed-in (`@request.auth.id != ''`), Selected users (`@request.auth.id = "id1" || ...`), Superusers only (`nil`), Custom (raw expression).
- Enforcement in app routes uses `authRequestInfo(e)` (main.go, aliased `agentRequestInfo`) to inject the cookie auth into the `RequestInfo`, then `e.App.CanAccessRecord` / a duplicated `checkCreateRule` dummy-record evaluator:
  - `/tabular/{configName}` + `/api/tabulator-data/{coll}`: records filtered by `listRule` (empty list for non-superusers when `nil`).
  - `/form/{configName}` edit: `viewRule` per record; new: denied when `createRule == nil` for non-superusers.
  - `/form/{configName}[/{id}]` POST: `updateRule` per record on update; `checkCreateRule` on create.
  - `/form/{configName}/{id}/delete`: `deleteRule` per record.
  - View collections (`IsView()`) are skipped.
- Superusers always bypass; the setup-side record editors (`/pbx-setup/record/...`) are superadmin-only via `requireSuperAdmin`.

## Template functions

Defined in `main.go` (used in `views/*.html`): `add`, `sub`, `seq`, `safeJS`, `safeHTML`.

## Theme

- Global default stored in `pb_data/theme.json` (`{"mode":"light"|"dark"}`); set via `POST /api/theme/{mode}` (also from the `/pbx-setup` Theme section or the topbar switch on any page).
- Every page data struct has a `Theme` field; templates render `<body data-theme="{{.Theme}}">` and link `/assets/theme.css`.
- `theme.css` uses a 4-base-color system per theme (`--bg`, `--surface`, `--text`, `--accent` in `:root` for light; `[data-theme="dark"]` overrides only those 4) plus shared semantic tokens (`--success`/`--warning`/`--danger` + soft/strong variants). All other tokens (`--btn-primary`, `--muted`, `--border`, …) are derived via `color-mix()` and keep their names, so re-skinning = editing ~8 numbers. Requires `color-mix` (2023+ browsers).
- The topbar switch also stores a per-browser override in `localStorage` key `pbx-theme`; on load the override wins, otherwise the server default applies.
- `login.html` uses its own `:root` variables plus the shared `theme.css`.

## Language (i18n)

- Two supported UI languages: English (`en`) and Czech (`cs`). Catalogs live in `i18n/en.json` / `i18n/cs.json` (embedded into the binary, ~230 keys each, dot-notation grouped by section).
- **Resolution order** (server-side, `getLangCode` in main.go): `--lang` CLI flag > caller's `pb_lang` cookie (per-browser override) > global default in `pb_data/lang.json` > `en`.
- Global default is persisted via `POST /api/lang/{code}` (mirrors `POST /api/theme/{mode}`); the topbar switcher button shows the target language (`CS`/`EN`), sets the `pb_lang` cookie, POSTs the global default, and reloads.
- **Server-side strings**: templates call `{{t .Lang "key"}}` (template func registered in main.go's `init()`); every page data struct embeds `views.LangData` (`Lang` field), and `login.html` gets a `Lang` map key.
- **Client-side strings**: each page loads `<script src="/api/lang/{{.Lang}}/catalog.js">`, which defines `window._tCatalog`, `window._t(key)` and `window._tLang()`. JS alerts/confirms/status text use `_t('key')`; strings with placeholders use `.replace('%s', ...)`.
- Server-rendered flash messages (`?msg=` after form/export/import/delete) are translated in the Go handlers via `i18n.T(getLangCode(e.App, e.Request), "key")`.

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

`zamestnanci`, `produkty`, `karta_majetku`, `inventury`, `inv_radky`, `cinnosti`, `mapa_umisteni`, `umisteni`, `organizacni_struktura`, `kat_produktu`, `definice_stitku`, `poznamky`. System: `users` (auth), `roles`, `_views`, `_app`, `_metadata`, `_agent`, `_filters`, `_actions`.

## Custom actions (`pbactions` package)

User-defined Goja (JavaScript) scripts that run from the tabular and form views (Phase 10). Package `pbactions/` (`types.go`, `runner.go`, `builtins.go`).

- **`_actions` collection**: `_name` (text req), `_description` (text), `_script` (editor req), `_collection` (text req), `_onList`/`_onForm`/`_public` (bool). Rules are `nil` (superuser only) — actions are created/edited via the `/pbx-setup` record editor (the Setup page lists `_actions`). Non-superusers only see/run `_public` actions.
- **Execution** (`runner.go`): fresh Goja VM per run, 10-second timeout via `vm.Interrupt()`, injects `record`/`records` (selected ids, filtered by `viewRule`). Builtins (`builtins.go`): `select`, `get`, `count`, `insert`, `update`, `delete`, `log`, `currentUser`, `currentRecord`, `selectedRecords`. Each enforces the PB rules for the caller; `insert` uses a create-rule check, `update`/`delete` use `updateRule`/`deleteRule` per record. Affected count accumulates writes.
- **Routes**: `GET /api/actions/{collectionName}` (list; `_public` only for non-superusers), `POST /actions/execute` (`{actionId, recordIds}` → `{ok, output[], affected}`).

## AI agent (`pbai` package)

Built-in chat agent at `/ai` (linked from `/app`). Package `pbai/` (`llm.go`, `agent.go`, `tools.go`, `ingest.go`, `render.go`).

- **LLM**: single OpenAI-compatible client via `github.com/sashabaranov/go-openai` (covers both OpenRouter and LM Studio). Config lives in the `_agent` collection. LM Studio `baseURL` should be the `/v1` endpoint (e.g. `http://127.0.0.1:1234/v1`); when `provider=lmstudio` a missing `/v1` suffix is appended automatically (`resolveBaseURL` in `llm.go`) — a bare host URL makes LM Studio answer HTTP 200 with an error body and zero choices ("empty response from model"). LM Studio intermittently ends a stream immediately with an empty delta + `finish_reason:"stop"`; `Client.Stream` retries such empty completions twice before surfacing the error (`maxEmptyStreamRetries`).
- **`_agent` collection**: system collection with `_name` (text, required), `_description` (text), `_config` (json). One record named `default` holds `{"provider":"lmstudio"|"openrouter","baseURL","apiKey","model","timeoutSeconds","enabled"}`. Edited from the `/pbx-setup` "AI agent" section (super-admin). The API key is stored in the record's `_config` JSON — never in env vars or git.
- **Tools** (`tools.go`): `list_collections`, `get_collection_schema`, `query_records`, `insert_records` (write), `create_collection` (write), `set_view_config` (write), `create_action` (write, upserts an `_actions` record by name; validates the target collection exists and the script compiles), `list_actions` (read). Write tools return a `PendingAction` instead of executing; the loop stops and the UI shows an approve/reject modal, then calls `POST /ai/confirm`.
- **Permissions**: create_collection/set_view_config/create_action/list_actions are super-admin only; record ops enforce each collection's PB rules (`listRule`/`viewRule`/`createRule`/`updateRule`/`deleteRule`) via `app.CanAccessRecord` with the caller's `RequestInfo`. `nil` rule = super-user only; `""` = everyone. Permission is re-checked at confirm time.
- **Files** (`ingest.go`): text/md/csv read inline, PDF via `github.com/ledongthuc/pdf` (max 20 pages, 300 KB extracted text), images sent as base64 multimodal (8 MB cap). Uploaded files are marked as untrusted data.
- **System prompt** (`agent.go`): reminds the model to answer in the user's language, use tools rather than invent facts, copy collection names exactly, keep prose brief after `query_records` (the UI renders the data), and — for "show/find a specific record" requests — to call `query_records` using the exact field name from the schema (check the schema first if unsure about spelling/casing).
- **Response rendering** (`pbai/render.go`, Go-side; `views/agent.html`, client-side): every terminal `ChatResult` carries `render`, a server-rendered, sanitized HTML fragment the chat page shows in the final bubble — no `<iframe>`:
  - *many records* → markdown lead-in (if `finalText`) + tabulator-style table (`ai-table`)
  - *one record* → markdown lead-in (if `finalText`) + read-only detail card (`ai-detail`, `<dl>`; labels/title pulled from the matching `_views` record `_form.formLabels`/`labels` and `_tabulator.pageTitle`, else the collection name); explicit "show/find a record" prompts make the model typically land here via the tabular fast path
  - *no records* → markdown only; *pending action* → markdown summary (the confirm modal handles approve/reject)
  - Markdown is rendered with `github.com/yuin/goldmark` (GFM) and sanitized with `github.com/microcosm-cc/bluemonday` (`UGCPolicy` + `AllowStandardURLs`, nofollow/no-referrer); `<img>` elements are stripped so a prompt-injected record value can never trigger an external fetch. Tables/cards use `html/template` auto-escaping.
  - Client contract: the streamed text bubble is replaced by the `render` fragment on `done`. **Fallback**: if a response carries `records` but no/empty `render` (e.g. stale server binary), `agent.html` renders the records table client-side (`addRecordsTable`) so an answer is never silently dropped.
- **Auth note**: handlers use `agentRequestInfo(e)` (main.go) to inject the `pb_auth` cookie auth into the RequestInfo; without it `isSuper()` is always false.
- **Conversation memory**: stateless server-side — the UI (`views/agent.html`) accumulates the visible turns in a JS array and sends the last 16 with each request; `Run` clamps incoming history to 40 messages. Assistant turns (incl. "Awaiting confirmation" summaries, fast-path table notes and confirm outcomes) are appended to that history client-side; the topbar "New chat" button resets it.

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
- `pbai/` — AI agent (`llm.go`, `agent.go`, `tools.go`, `ingest.go`, `render.go`)
- `pbactions/` — custom action engine (`types.go`, `runner.go`, `builtins.go`)
- `views/import-wizard.html` — shared 3-step wizard: Source → Preview/Edit → Create (used by both Excel and MSSQL collection import)
- `views/pages.go` — Go structs for template data (`TabulatorPageData`, `FormPageData`, `AppPageData`, `ImportWizardPageData`, etc.)
- No `README.md` exists

## Collection creation wizard

`/pbx-config/import-excel` and `/pbx-config/import-mssql` (super-admin) create a new base collection from a source:

1. **Source** — collection name + Excel file/sheet or MSSQL DSN/table.
2. **Preview/Edit** — introspected columns (`pbexcel.IntrospectSheet` / `pbmssql.IntrospectTable`) with inferred PB types (text/number/bool/date); names/types editable, columns can be skipped.
3. **Create** — `core.NewBaseCollection` + fields from detected schema → `app.Save`; optionally imports the source data immediately (with `ImportFromExcel` mapping or `pbmssql.ImportFromMSSQL`).

Field/collection names are normalized to lowercase `[a-z0-9_]` (reserved system names `id`/`created`/`updated`/`collectionid`/`collectionname`/`expand` are skipped).
