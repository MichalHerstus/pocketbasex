# Assets

Web app to manage company assets (notebooks, phones, furniture, etc.). PocketBase custom Go build.

## Stack

- **Runtime**: PocketBase v0.39 custom Go build (`./pbx`)
- **Frontend**: server-rendered Go templates (`views/*.html`) + two React SPAs at root (`home.html`, `rlogin.html`)
- **Backend**: Go on port 8090
- **DB**: SQLite via `pb_data/data.db`
- **CSS**: `assets/w3.css` (W3.CSS v5.01). `assets/myapp.css` does not exist — do not reference it.

## Commands

| Action | Command |
|--------|---------|
| Build | `go build ./... && go vet ./...` |
| Run server | `./pbx serve` (listens on :8090) |

## Key files

| File | Purpose |
|------|---------|
| `main.go` | Server entrypoint, route handlers, custom CLI commands |
| `views/pages.go` | Go structs for template data |
| `views/tabulator.html` | Table view template (client-side JS for pagination/sort/search) |
| `pb_migrations/*.js` | Schema migrations (auto-applied on `serve`) |

## Architecture

- **`/tabulator/{collectionName}`** — renders any collection as an HTML table. All records loaded client-side as JSON; JS handles pagination (20/page), sorting, and search. Configurable via `_tabulator` collection records.
- **`/`** — static file serving from `pb_public/` (falls back to `index.html` for SPA routes)
- **Custom CLI commands**: `./pbx export <collection> [file]` (CSV export), `./pbx cdef <collection> [file]` (SQL CREATE TABLE)

## `_tabulator` collection

Base collection. A record with `collName={collectionName}` configures the table view at `/tabulator/{collectionName}`:
- `pageTitle` — custom `<h1>` heading (falls back to collection name)
- `collectionDescr` — italic text below record count
- `columnTitles` — comma-delimited override for column headers
- `columnOrder` — comma-delimited 1-based absolute field indices (applied before `displaySystemCol`)
- `displaySystemCol` — if 0, hides `id`, `created`, `updated` columns
- `columnSorting` — if 1, clickable sort icons (↕→▲→▼ cycle, single column at a time)
- `searchBox` — if 1, shows search input (filters across all columns)
- `pagination` — if 1, shows « ‹ [input] › » controls with direct page entry
- `multiSelect` — reserved, not used

## Collections (in `pb_data/data.db`)

- **`users`** — auth collection (email, password, name, avatar)
- **`roles`** — base collection (roleName, description, active)
- **`_tabulator`** — table view config per collection
- **`_metadata`** — internal: colName, colllectionID, description, icon, config, active
- Custom app collections: `zamestnanci`, `produkty`, `karta_majetku`, `inventury`, `inv_radky`, `cinnosti`, `mapa_umisteni`, `umisteni`, `organizacni_struktura`, `kat_produktu`, `definice_stitku`, `poznamky`

## Schema changes

Add/modify collection fields via **JS SDK** in `pb_migrations/`. Use the `.opencode/skills/pocketbase-api-add-field/SKILL.md` skill for help with programmatic field additions.

## Conventions

- `pb_data/` — SQLite DB + file storage, committed
- `pb_migrations/` — JS migrations, auto-applied
- `pb_hooks/` — does not exist; do not reference it
- `assets/` — CSS and static assets
- Go templates use `add`, `sub`, `seq`, `safeJS` template functions (defined in `main.go`)
- No `README.md` exists — AGENTS.md is the sole documentation
