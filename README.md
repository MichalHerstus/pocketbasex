# PBX — PocketBase Extended

A custom **PocketBase v0.39** build that adds a configurable, server‑rendered web UI on top
of PocketBase collections. You keep all the standard PocketBase power (storage, the stock
admin console at `/_/`, migrations, CLI) and gain an app‑style front‑end: dashboards,
tables, forms, Excel/MSSQL import·export, an AI agent, mobile views, and a superadmin
setup area — all in English **and** Czech.

> **Documentation**: the end‑user manual lives in [`PBX_USER_GUIDE.md`](PBX_USER_GUIDE.md).
> This README is a quick orientation for developers/operators.

---

## What PBX adds to PocketBase

* **App-style UI** instead of the schema-oriented admin — dashboard, tables, forms, setup
* **Configurable list/form views** — one `_views` JSON config drives both the tabular view
  and the form (columns, titles, labels, layout, sorting, filters, pagination)
* **Excel import/export** and **MSSQL sync** with a "create collection from source" wizard
* **AI agent** — natural-language chat over your data with streaming replies, rule‑enforced
  read/write tools, conversation persistence, welcome suggestions, slash commands, and
  structured form‑fill in the form view
* **Custom actions** — Goja‑scripted business logic (bulk updates, reports, cross‑collection
  ops) runnable from list/form views with a 10‑s timeout
* **Mobile views** — automatic phone/tablet detection routes `/mobile/*` variants
* **Multilingual** — English and Czech UI with a per‑user switcher
* **Superadmin setup** — theme, default language, global MSSQL DSN, AI config, per‑collection
  API rules, and record editors for the system collections

## Architecture & stack

- **Runtime**: Go binary (`pbx`); the server listens on `127.0.0.1:8090` by default
- **Database:** SQLite at `pb_data/data.db` (git-ignored)
- **Frontend:** Go server-rendered HTML templates (`views/*.html`), W3.CSS, light/dark theme,
  English/Czech catalogs (`i18n/*.json`, embedded in the binary)
- **Migrations:** `pb_migrations/*.js`, auto-applied on every `serve`
- **AI agent:** OpenAI-compatible client (OpenRouter / LM Studio), tools that read and write
  through PocketBase's own rule engine, Goja script runner (`pbactions`)

Auth is cookie-based (`pb_auth` JWT). Regular users live in the `users` collection;
administrators live in `_superusers`. Access to every collection is governed by PocketBase
rules (`nil` = superusers only, `""` = public, otherwise a filter expression).

### Main routes

| Path | Purpose |
|------|---------|
| `/app` | Dashboard with group cards + links |
| `/tabular/{configName}` | Configurable table view |
| `/form/{configName}[/{id}]` | New / edit / view record form |
| `/ai` | AI agent chat |
| `/pbx-setup` | Superadmin hub (rules, config, AI, theme, DSN) |
| `/pbx-config` | `_views`/config editor + collection-from-source wizards |
| `/mobile/*` | Mobile variants |
| `/_/` | Stock PocketBase admin console (always available) |

### Developers

```bash
# Build, vet and test
go build ./... && go vet ./... && go test ./...

# Run
./pbx serve            # then open http://127.0.0.1:8090
./pbx serve --dev      # verbose logs + SQL
```

Create the first superuser with `./pbx superuser create admin@example.com 'password'`.

### Repository map

- `main.go`, `routes.go` — HTTP handlers, template funcs, CSRF/rate-limit helpers,
  tabular pagination, auth resolution, theme/lang helpers
- `pbai/` — AI agent (LLM client, agent loop, tools, server-rendered chat answers, exports,
  suggestions, form-fill extraction)
- `pbactions/` — Goja custom-action engine
- `pbexcel/`, `pbmssql/` — Excel and MSSQL import/export/introspection
- `pbrules/` — shared create-rule checker used by the app, AI agent, and custom actions
- `views/`, `views/pages.go`, `i18n/` — templates, page-data structs, localized catalogs
- `pb_migrations/` — schema changes (JS SDK)