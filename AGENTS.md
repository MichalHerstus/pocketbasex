# PBX

Web UI over PocketBase collections. Custom PocketBase v0.39 Go build.

## Stack & commands

- **Module** `pbx`, Go on :8090 (`./pbx serve`). Frontend = server-rendered Go templates (`views/*.html`), CSS = W3.CSS (`assets/w3.css`; `assets/myapp.css` does not exist).
- **DB**: `pb_data/data.db` (gitignored). Default data dir is `<executable_dir>/pb_data` — running the binary from another CWD silently bootstraps an empty DB.
- Build/vet/test: `go build ./... && go vet ./... && go test ./...`
- Tests exist in root package (`tabulator_test.go` route-level regression, `templates_test.go`, `filters_test.go`), `pbai/`, `pbactions/`. No CI.

## Layout

| Location | Contents |
|---|---|
| `main.go` | Handlers, template funcs, CSRF/rate-limit helpers, tabular pagination, theme/lang/mssql persistence |
| `routes.go` | All route registration (`registerAuth/App/Setup/Config/Data/AssetsAndAPI/AI/ActionRoutes`); desktop + `/mobile/*` variants |
| `pbrules/` | Shared `CheckCreateRule` (+ `isValidCollectionName`) used by main.go, `pbai/tools.go`, `pbactions/builtins.go` |
| `pbexcel/`, `pbmssql/` | Excel/MSSQL import-export-introspection; DSN-keyed pool in pbmssql (`CloseAll()` on terminate) |
| `pbai/` | AI agent: `llm.go`, `agent.go`, `tools.go`, `ingest.go`, `render.go` |
| `pbactions/` | Goja custom-action engine (`types.go`, `runner.go`, `builtins.go`) |
| `views/pages.go` | Template data structs; `i18n/` embedded catalogs |

## Auth & rules (critical)

- Cookie auth `pb_auth` (JWT). **PocketBase's `loadAuthToken` reads only the `Authorization` header**, so `e.RequestInfo().Auth` is nil on custom routes — always resolve via `authRequestInfo(e)`/`agentRequestInfo(e)` (main.go) before rule checks.
- Collection rules are nullable TEXT: `nil` = superusers only, `""` = public, else PB filter expression. Enforcement: `CanAccessRecord` per record; create checks via `pbrules.CheckCreateRule` (dummy-record evaluator). Superusers bypass; view collections skipped.
- Setup routes (`/pbx-setup/*`, incl. record editors and `POST /pbx-setup/rules`) are superadmin-only via `requireSuperAdmin`.

## Security posture (Phase 14)

- CSRF middleware (HMAC over UA+IP, secret `csrfSecret`) currently applied to `POST /login` only (rate-limited 5/15min); AI chat/stream rate-limited too. Token renders as hidden input `csrf_token` — login error re-renders must include `CSRFToken`.
- Path traversal sanitized in assets handler + `pbexcel.resolveExcelPath`; cookies set `Secure` on TLS; config names sanitized before redirects.

## `_views` config (drives `/tabular/{_name}` + `/form/{_name}`)

JSON fields on a `_views` record (`_name`, `_collName`):
- `_tabulator`: `pageTitle`, `collectionDescr`, `columnTitles`, `columnOrder` (1-based indices), `displaySystemCol`, `columnSorting`, `searchBox`, `pagination`, `filter`, `columns [{field,title}]`.
- `_form`: `formTitle`, `formDescr`, `displaySystemCol`, `columnOrder`, `formLayout` (`"row:(1,2) (3,4)/row:(5)"`, 1-based in config), `formLabels` (`field=Label,...`).
- `_mssql`: `{dsn, table, mode, mapping:[{pbField,dbField}]}` for the Sync modal.

**GOTCHA**: `_tabulator.filter` is a CLIENT-side expression that may use `?` placeholders filled by JS. Never pass it into server-side queries (`FindRecordsByFilter` etc.) — it breaks with "invalid filter expression" → 404. Tabular pagination (`buildTabulatorDataWithPagination`, `getTotalRecords`, `fetchPaginatedRecords` in main.go) queries the DB without it; listRule filtering stays in-memory post-fetch.

## Other systems

- **Theme**: global default in `pb_data/theme.json` via `POST /api/theme/{mode}`; per-browser override `localStorage.pbx-theme` wins. `theme.css` = 4 base colors per theme + derived `color-mix()` tokens.
- **i18n**: `en`/`cs`, catalogs `i18n/*.json`. Resolution: `--lang` flag > `pb_lang` cookie > `pb_data/lang.json` > en. Server strings: `{{t .Lang "key"}}`; client strings: `/api/lang/{code}/catalog.js` → `window._t(key)`.
- **MSSQL**: export/import/introspect against INFORMATION_SCHEMA; missing-table export returns 409 `{tableMissing:true}` until re-sent with `createTable=1`. Global DSN in `pb_data/mssql.json`.
- **Collection wizard** (`/pbx-config/import-excel|mssql`): introspect source → edit schema → create collection (+ optional data import); names normalized to `[a-z0-9_]`.
- **Custom actions** (`_actions` collection): Goja scripts, 10s timeout, builtins enforce caller's PB rules; non-superusers see only `_public`.
- **AI agent** (`/ai`, `_agent` collection holds provider config; API key only in `_config` JSON): OpenAI-compatible client (OpenRouter/LM Studio — LM Studio baseURL must be the `/v1` endpoint; empty-stream retries built into `Client.Stream`). Write tools return `PendingAction` → approve/reject via `POST /ai/confirm` (permissions re-checked at confirm). Responses render server-side sanitized HTML (`goldmark`+`bluemonday`, `<img>` stripped); conversation memory lives client-side (last 16 turns sent).

## Conventions

- Schema changes via JS SDK in `pb_migrations/` (see `.opencode/skills/pocketbase-api-add-field/SKILL.md`). `pb_data/types.d.ts` is generated — don't edit.
- `pb_hooks/` and `pb_public/` do NOT exist — do not reference them.
- `views/assets/` (icons PNG + theme.css) served via embedded FS at `/assets/{path...}`.
- No README.md exists.
