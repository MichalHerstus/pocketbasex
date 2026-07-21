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
| `GET /pbx-setup` | setup | Shows `_app`, `_tabulator`, `_form` tables |
| `GET /tabulator/{collectionName}` | table view | All records as client-side JSON; 20/page, sort, search |
| `GET /form/{collectionName}` | form view | New record form, layout configured via `_form` collection |
| `GET /form/{collectionName}/{id}` | form view | Edit existing record |
| `POST /form/{collectionName}` | form submit | Create record |
| `POST /form/{collectionName}/{id}` | form submit | Update record |
| `POST /form/{collectionName}/{id}/delete` | delete record | Delete record, returns JSON |
| `GET /api/tabulator-data/{collectionName}` | JSON API | Raw JSON for relation modal |
| `GET /export/{collectionName}` | export | Export to Excel via `pbexcel` |
| `POST /import/{collectionName}` | import | Import from Excel via `pbexcel` |
| `GET /assets/{path...}` | static | Serves `views/assets/` files |

**Auth**: Cookie-based `pb_auth` (JWT via PocketBase). Login uses `FindAuthRecordByEmail("users", ...)` — name field is the email.

## Template functions

Defined in `main.go` (used in `views/*.html`): `add`, `sub`, `seq`, `safeJS`, `safeHTML`.

## `_tabulator` collection

A record with `collName={collectionName}` configures `/tabulator/{collectionName}`:

- `pageTitle` — custom `<h1>` heading (falls back to collection name)
- `collectionDescr` — italic text below record count
- `columnTitles` — comma-delimited override for column headers
- `columnOrder` — comma-delimited 1-based absolute field indices (applied before `displaySystemCol`)
- `displaySystemCol` — if false, hides `id`, `created`, `updated`
- `columnSorting` — if true, clickable sort (↕→▲→▼)
- `searchBox` — if true, search input filters across all columns
- `pagination` — if true, « ‹ [input] › » controls
- `filter` — filter expression for records

## `_form` collection

A record with `collName={collectionName}` configures `/form/{collectionName}`:

- `formTitle` — custom heading (falls back to collection name)
- `formDescr` — description paragraph below heading
- `displaySystemCol` — if true, shows `id`, `created`, `updated` as read-only
- `columnOrder` — comma-delimited 1-based field indices (applied when no `formLayout`)
- `formLayout` — slash-delimited rows, parentheses for column groups: `"row:(1,2) (3,4) / row:(5,6)"` (0-based internally, 1-based in config)
- `formLabels` — comma-delimited `field=Label` pairs (e.g. `"name=Jméno,email=E-mail"`)

## `_app` collection

Configures the `/app` dashboard. Fields:

- `group` / `group_label` — links are grouped under a heading
- `group_icon` — uploaded icon file for the group
- `collection` / `collectionLabel` — link target (`/tabulator/{collection}`) and display text

## Collections (from `_app`, `_tabulator`, `_form` records in DB)

`zamestnanci`, `produkty`, `karta_majetku`, `inventury`, `inv_radky`, `cinnosti`, `mapa_umisteni`, `umisteni`, `organizacni_struktura`, `kat_produktu`, `definice_stitku`, `poznamky`. System: `users` (auth), `roles`, `_tabulator`, `_form`, `_app`, `_metadata`.

## Schema changes

Add/modify collection fields via **JS SDK** in `pb_migrations/`. See `.opencode/skills/pocketbase-api-add-field/SKILL.md`.

`pb_data/types.d.ts` is auto-generated — do not edit manually.

## Conventions

- `pb_migrations/` — JS migrations, auto-applied on `serve`
- `pb_hooks/` — does not exist; do not reference it
- `pb_public/` — does not exist; do not reference it
- `views/assets/` — icons (PNG) and `theme.css`; served via embedded FS
- `pbexcel/` — Excel import/export logic (`pb-excel.go`)
- `views/pages.go` — Go structs for template data (`TabulatorPageData`, `FormPageData`, `AppPageData`, etc.)
- No `README.md` exists
