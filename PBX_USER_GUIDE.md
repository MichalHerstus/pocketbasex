# PBX User Guide

PBX ("Pocketbase Extended") is a custom PocketBase build that adds a configurable,
server-rendered web UI on top of PocketBase collections. It keeps all the standard
PocketBase capabilities (data storage, the stock admin UI at `/_/`, CLI) and extends
them with ready-made list (tabular) and form pages, a dashboard, Excel and MSSQL
import/export, an AI agent, mobile views, and a superadmin setup area.

This guide covers everything implemented up to **Phase 13** of `PBX_plan.md`
(configuration model, config-name routing, superadmin config editor, view-collection
editing, Excel/MSSQL import+sync, collection-creation wizard, mobile views, the
per-user landing-page `_view` field, custom actions, AI-agent action-management tools,
multilingual UI (English/Czech), and the AI agent enhancements: conversation memory,
streaming responses, self-correcting collection lookup, and server-rendered chat replies).

---

## 1. Overview

### 1.1 What PBX adds to PocketBase

| Area | Stock PocketBase | PBX |
|------|------------------|-----|
| Web UI | Generic admin at `/_/` (technical, schema-focused) | An app-style UI: dashboard, tables, forms, setup |
| List/Form views | Manual per-collection | Configurable via `_views` JSON (one config drives both) |
| Data import/export | — | Excel import/export, MSSQL sync, "create collection from source" wizard |
| Built-in agent | None | Chat AI agent with read + confirmed write tools, streaming replies, action-management tools |
| Language | English only | English **and Czech**, per-user switcher |

### 1.2 Technologies

- **Runtime**: PocketBase v0.39 (custom Go build, the `pbx` binary).
- **Server**: Go HTTP server, listens on `127.0.0.1:8090` by default (configurable).
- **Database**: SQLite at `pb_data/data.db` (the SQLite file is not tracked in git).
- **Frontend**: Go server-rendered HTML templates (`views/*.html`) with light/dark theme
  and English/Czech localization (`i18n/` catalogs embedded in the binary).
- **Migrations**: applied automatically on every `serve` from `pb_migrations/*.js`.

### 1.3 Who uses it (two account tiers)

- **Regular users** — records live in the `users` collection; they sign in with email +
  password, land on the dashboard (`/app`) and use tables, forms, and the AI agent.
- **Superusers** — records live in the `_superusers` collection; after login they land
  on `/pbx-setup` and get full system administration (themes, default language, DSN, AI
  config, collection rules, config editor, record editors) **plus** the standard PocketBase
  admin UI.

All access is cookie-based (`pb_auth`). Logout is at `/logout`.

---

## 2. Roles, authentication & first-time setup

PBX manages two different "people" tables:

1. **`users`** — end users of the app. Their dashboard login is:
   - Username = the `name` field (in this installation that is an email address).
   - Password = stored with PocketBase password hashing.
2. **`_superusers`** — administrators of the whole PocketBase instance. The first one is
   created from the command line (section 3). Superusers automatically inherit all
   collection permissions and can open `/pbx-setup` and `/_/`.

### 2.1 Creating the first superuser (CLI)

Before anyone can sign in as an administrator you must create a superuser from the command
line:

```bash
./pbx superuser create admin@example.com Admin_Pass_2025
```

> Use superusers only for administration. Regular people belong in the `users` collection
> so they get the dashboard and properly scoped permissions.

### 2.2 Signing in as a regular user

Open `http://127.0.0.1:8090` (or your domain) in a browser. PBX shows a login dialog.
Enter the user's email (Username) + password. After successful login you land on:

- the **dashboard** `/app` — the default landing page, or
- the **landing config** if the user record has a `_view` value (Phase 9): a specific
  tabular configuration, e.g. `/tabular/zamestnanci`, or `/mobile/tabular/zamestnanci`
  on phones.
- If the `_view` name is empty or no longer valid (config deleted), you fall back to `/app`.

Superusers always land on `/pbx-setup` — the `_view` override never applies to them.

To sign out, use the **Logout** button in the top bar (goes to `/logout`).

---

## 3. PBX command-line interface (CLI)

The `pbx` executable is a normal Go binary. Run it with **no arguments** to see the full
help.

### 3.1 Global flags

| Flag | Description |
|------|-------------|
| `--dev` | Enable dev mode — prints logs and SQL statements to the console. |
| `--dir <path>` | PocketBase data directory (default `pb_data`). |
| `--encryptionEnv <var>` | Name of an env variable whose 32-char value is the encryption key. |
| `--queryTimeout <sec>` | Default SELECT query timeout in seconds (default 30). |
| `--version` | Print the version. |

### 3.2 Commands

| Command | Purpose |
|---------|---------|
| `serve [domain(s)]` | Start the web server (default `127.0.0.1:8090`). |
| `superuser` | Manage PocketBase superusers. |

### 3.3 `serve` flags

| Flag | Description |
|------|-------------|
| `--http <addr>` | HTTP TCP address. If you give domain arguments, defaults to `0.0.0.0:80`; otherwise `127.0.0.1:8090`. |
| `--https <addr>` | HTTPS TCP address. If domains given, default `0.0.0.0:443`; otherwise empty (= no TLS). HTTP is auto-redirected to HTTPS. |
| `--origins` | CORS allowed origins (default `*`). |
| `--lang <en\|cs>` | Force the UI language for the whole server, overriding the per-browser and persistent defaults. |

**Examples**

```bash
# Local server on port 8090 (default)
./pbx serve

# Bind to a fixed address
./pbx serve --http 0.0.0.0:8090

# Server behind a domain with TLS
./pbx serve example.com:8090 --https example.com:443

# Dev mode (logs + SQL on the console)
./pbx serve --dev
```

When the server is up, PBX prints a colored summary of its endpoints:

```
┌─ PBX app:      http://127.0.0.1:8090/app
├─ PBX list:     http://127.0.0.1:8090/tabular/{configName}
├─ PBX form:     http://127.0.0.1:8090/form/{configName}
├─ PBX setup:    http://127.0.0.1:8090/pbx-setup
└─ PBX config:   http://127.0.0.1:8090/pbx-config
```

Migrations in `pb_migrations/` are auto-applied on every `serve`.

### 3.4 `superuser` subcommands

| Command | Signature | Description |
|---------|-----------|-------------|
| `create <email> <password>` | | Create a new superuser. |
| `update <email> <password>` | | Change the password of one superuser. |
| `upsert <email> <password>` | | Create, or update if the email exists, a superuser. |
| `delete <email>` | | Remove a superuser. |
| `ips` | address... (space-separated) | Restrict a superuser to an IP whitelist; empty clears. |
| `otp <email>` | | Generate a fresh OTP for a superuser. |

**Example**

```bash
./pbx superuser create admin@example.com SomePassword
```

---

## 4. Using the app (regular users)

### 4.1 Dashboard `/app`

The app's home page, linked from the top bar on every page. It:

- Shows "Signed in as **{user}**".
- Groups your collections into cards. Each card has an optional **group icon** and a
  **group label** (e.g. *Administration*), then a set of links, one per collection.
- Each link opens the collection's configured tabular view (`/tabular/{configName}`). If
  a collection has no view configuration, the link falls back to the default config, then
  to the raw collection name.
- Includes an always-present **AI agent** link at the bottom of the menu.

The links (which config they target) are driven by the system `_app` collection, edited
by a superuser from `/pbx-setup`.

### 4.2 Table (tabular) view — `/tabular/{configName}`

Opens a live table of the collection's records:

- **Search** — if enabled in the config, a search box filters across all columns as you
  type.
- **Sort** — if enabled, click a column header to toggle sort (↕→▲→▼).
- **Pagination** — if enabled, first/previous/page-input/next/last controls under the table.
- **Saved (advanced) filters** — a dropdown lists any named filters you have saved (or all
  filters for superusers), plus an **Edit filters** button opens an "Edit filters" panel:
  build multi-condition filters, give them a name, and save or delete them. Filters may use
  parameter placeholders (`?`) that are prompted at apply time.
- **Related records modal** — for relation fields, a button opens a small tabular modal of
  the related collection.
- **Import / Export (Excel)** — the two buttons open modals for importing from or exporting
  to an `.xlsx` workbook (choose a file name and sheet name).
- **MSSQL Sync** — if the config has an `_MSSQL` block, a **MSSQL** button opens a modal to
  import from or export to the configured SQL Server table (DSN falls back to the global
  DSN in `/pbx-setup`). Export to a not-yet-existing table asks for confirmation before the table
  is created.
- **+ Add new** — jumps to a blank form for a new record.
- **Actions** — if the collection has custom actions configured, an **Actions** dropdown + **Run** button appears. Check the boxes next to the records you want to process (or select none to run on all visible records), pick an action, and press Run. The result (output lines, affected count, or error) is shown in a modal. See [§5 Custom actions](#5-custom-actions-scripts).

Row actions (view / edit / delete) appear in the last column. Deleting asks for
confirmation.

### 4.3 Form view — `/form/{configName}` and `/form/{configName}/{id}`

The form page shows one record at a time:

- **New record** (`/form/{name}`) — a blank collection form. Fill fields (text, number,
  boolean, select, date, email, relation, files…) and Submit.
- **Edit / view** (`/form/{name}/{id}`) — the record's values. Edit and re-submit to save.
  Optional `?view=1` opens in read-only view mode.
- **Delete** — button removes the record (with confirmation) and returns JSON.
- **Files** — the record form handles file fields (uploaded files, and removal where the
  field value is blanked). The current file shows as a link; a "remove" option is offered.
- **Relations** — relation fields display a button that opens a small modal listing related
  records.
- **View-only collections** — if the collection is a PB **view** (SQL view), editing shows
  **one section per base collection** the view joins. Saving a multi-table view updates each
  base table in a single transaction, so the joined result reflects all new values.
- **Actions** — when editing an existing record (`{id}` present), an **Actions** dropdown +
  **Run** button is shown. Picking an action runs it against the current record and displays
  the result in a modal. See [§5 Custom actions](#5-custom-actions-scripts).

### 4.4 AI agent — `/ai`

A chat page. You can:

- Ask natural-language questions about your data ("how many products cost less than 100").
- The agent has tools: `list_collections`, `get_collection_schema`, `query_records`,
  `insert_records`, `create_collection`, `set_view_config`, `create_action`, and
  `list_actions`.
- **Read-only tools** (list/schema/query, list actions) run immediately.
- **Write tools** (insert/create/set config/create action) never run straight away — they
  produce a **Pending Action**, shown in a confirm modal with an *Approve & execute* /
  *Reject* button. Only after your explicit confirmation does the action execute.
- **Streaming replies** — the assistant's answer streams in live, with a status line
  beneath each tool the agent calls; the final answer (a table, a detail card, or plain
  text) replaces the streamed text when it is ready.
- **Rich, server-rendered answers** — record data is rendered as a proper table (or a
  detail card for a single record), not raw markdown, and is sanitized server-side.
- **Tabular fast path** — "list records in …" style questions answer from a **single**
  model call: you get the table without a second, slower generation step.
- **Self-correcting collection names** — if the model types a collection name wrong, the
  agent detects it, suggests the closest real name, and retries in the same turn.
- **Conversation memory** — the chat keeps your last 16 turns in context, so follow-up
  questions ("and now give me the most expensive one") work without repeating yourself.
- **Attach files** — you can attach a file to a message. Text / Markdown / CSV are read
  inline, PDFs (max 20 pages / 300 KB) are extracted, and images are sent to the model.

If the agent is unconfigured, it says so — a superuser must fill in `/pbx-setup` → **AI agent** first.

### 4.5 Language (Czech / English)

Every page — including the login screen — has a **language button** in the top bar (shows
`CS` when the page is in English, `EN` when it is in Czech). Clicking it switches the whole
site. The language is resolved (highest priority first):

1. the `--lang` CLI flag when the server was started with one,
2. your **per-browser** choice (remembered in a `pb_lang` cookie),
3. the **global default** stored by a superuser (persisted to `pb_data/lang.json`),
4. English.

A superuser can change the **global default** the same way: on any page the topbar switch
both sets your per-browser choice and persists the new global default
(`pb_data/lang.json`). The checkbox-free approach keeps every page consistent because the
language is chosen server-side before the page is rendered.

---

## 5. Custom actions (scripts)

Custom actions let you run business logic (reports, bulk updates, cross-collection
operations, clean-ups) directly from the tabular and form views. An action is a small
**JavaScript** script stored in the system `_actions` collection and executed on the server.

### 5.1 How actions are shown

- **List view** (`/tabular/...`) — an **Actions** dropdown next to "Run". Check the records you
  want to process, or select none to run on **all visible** (filtered) records.
- **Form view** (`/form/.../id`) — the **Actions** dropdown runs against the single open record.
- **Mobile** — the same Actions dropdown is available in both views.
- A result modal shows the script's `log()` output, the number of records affected, or an
  error if the script failed.

### 5.2 Who can see and run actions

- Non-superusers only see actions that are marked **Public** (`_public = true`).
- Superusers see and can run all actions.
- Each action visibly runs as the **current user**, so the script's reads and writes are
  checked against the target collections' API rules (list/view/create/update/delete).
- Actions execute with a **10-second timeout** — an infinite loop is killed automatically.

### 5.3 Creating an action (superuser)

Actions live in the `_actions` collection, edited from **`/pbx-setup`** (the Setup page lists
`_actions`; use **+ Add new** / **Edit** to open the record editor). Fill in:

| Field | Meaning |
|-------|---------|
| `_name` | Display name shown in the Actions dropdown (e.g. `Bulk +10% price`). |
| `_description` | Help text shown as a tooltip. |
| `_script` | The JavaScript source (Goja) to run. |
| `_collection` | The collection the action is configured for (shown only on that collection's views). |
| `_onList` | Show the action in the tabular (list) view dropdown. |
| `_onForm` | Show the action in the form view dropdown. |
| `_public` | Let non-superusers see and run the action. |

You can also ask the **AI agent** to create or update an action: e.g. *"create an action
'Bulk +10% price' on collection produkty that raises produkty.price by 10%"*. The result
appears in `/pbx-setup` as a normal `_actions` record and works exactly like a hand-written
one. The write is always shown for confirmation before it is saved.

### 5.4 Available built-in functions

| Function | Description |
|----------|-------------|
| `select(coll, filter, sort?, limit?)` | Return `[]` of records matching a PB filter. Enforces `listRule`. |
| `get(coll, id)` | Return a single record as a map, or `null`. Enforces `viewRule`. |
| `count(coll, filter?)` | Return the number of matching records. Enforces `listRule`. |
| `insert(coll, data)` | Create a record; returns the new record id. Enforces `createRule`. |
| `update(coll, id, data)` | Update a record; returns `true`/`false`. Enforces `updateRule`. |
| `delete(coll, id)` | Delete a record; returns `true`/`false`. Enforces `deleteRule`. |
| `log(...)` | Append text to the result output (shown in the result modal). |
| `currentUser()` | The current user `{id, name, email}`. |
| `currentRecord()` | The current record (form) or first selected record (list), or `null`. |
| `selectedRecords()` | All selected records (list), or `[currentRecord]` (form). |

### 5.5 Example 1 — report (list view, read-only)

Prints a report of products priced under 100 to the result modal:

```javascript
var rows = select("produkty", "cena < 100", "", 100);
log("Cheap products: " + rows.length);
rows.forEach(function (r) {
    log(r.nazev + " — " + r.cena + " Kč");
});
```

### 5.6 Example 2 — bulk update (list view, write)

Increases the price of all products under 100 by 10 %:

```javascript
var rows = select("produkty", "cena < 100", "", 0);
var updated = 0;
rows.forEach(function (r) {
    var ok = update("produkty", r.id, { cena: Math.round(r.cena * 1.1) });
    if (ok) updated++;
});
log("Updated " + updated + " products (price +10%).");
```

### 5.7 Example 3 — cross-collection (form view)

Completes all tasks assigned to the current employee:

```javascript
var emp = currentRecord();
var tasks = select("ukoly", "zamestnanec = '" + emp.id + "'", "", 0);
var done = 0;
tasks.forEach(function (t) {
    if (update("ukoly", t.id, { stav: "dokonceno" })) done++;
});
log("Completed " + done + " tasks for " + emp.jmeno);
```

### 5.8 Example 4 — delete with confirmation (list view)

Deletes the selected records from the `poznamky` collection:

```javascript
var rows = selectedRecords();
var deleted = 0;
rows.forEach(function (r) {
    if (delete("poznamky", r.id)) deleted++;
});
log("Deleted " + deleted + " note(s).");
```

> When a list action runs with **no records selected**, it runs against all visible records.
> You can use `selectedRecords()` or `currentRecord()` in combination with safe selections —
> double-check the filter before a destructive bulk action.

---

## 6. Superadmin area: `/pbx-setup`

Superusers land here after login. The page has separate focused panels:

### 6.1 Theme

- A Light/Dark toggle that sets the **global default** theme, stored server-side. Users can
  still override per-browser with the topbar switch.

### 6.2 MSSQL (global DSN)

- A single text box for the **global default DSN** used by the MSSQL Sync feature /
  import wizards. Example: `sqlserver://user:pass@host:1433?database=db&encrypt=disable`.

### 6.3 AI agent

- Provider (`openrouter` or `lmstudio`), Base URL, API key (typed and stored only in the
  `_agent` config record, never in env or git), Model name, Timeout in seconds, and an
  **Enabled** checkbox.

### 6.4 System record browsers ( `_app`, `_views`)

The two sections show `_app` and `_views` as tables. Each row has **+ Add new** and per-record
**Edit / Delete** links. Editing a system record goes to a **record editor** form
(`/pbx-setup/record/{coll}/new` or `/pbx-setup/record/{coll}/{id}`):

- A generic editor that renders each field by its PocketBase type.
- **`_views` record** — the editor understands the JSON `config` field of the coupled
  list/form configuration and offers a *structured* form for the most common keys:
  - Header (title, description, collection description).
  - **Columns** — one checkbox per target-collection field, each with a display **title**, and
    toggles for sort (sortable) and search.
  - Search / Pagination / column-sorting toggles; Filter text.
  - `_form` layout builder (rows/columns of fields) + per-field `label` overrides.
  - For MSSQL: DSN, table, mode, and a **mapping** table (PocketBase field → DB column),
    added with an "Add row" button.
  - Unknown/advanced JSON keys are kept intact and exposed in a raw JSON textarea that can be
    collapsed.
- **`_app` records** — the `group`, `group_label`, `collection`/`collectionLabel`,
  `configName` fields, plus **group icon** file upload (with a remove option).

### 6.5 Collection API rules (per collection)

This is a security matrix for **every data collection** (excluding `users`, `roles`, `_`-
prefixed system collections, and view collections). For each operation — **List / View /
Create / Update / Delete** — you choose one of five modes:

| Mode | Meaning |
|------|---------|
| **Public** | Anyone (even unauthenticated) — rule `""`. |
| **Signed-in** | Any authenticated user — rule `@request.auth.id != ''`. |
| **Selected users** | Only checked `users` — either of the OR-chain `@request.auth.id = "id1" …`. |
| **Superusers only** | Only superusers — rule `nil`. |
| **Custom** | Raw PB filter expression. |

The UI offers the user list as checkboxes; the editor writes/reads the PocketBase rule
field. The rules are **enforced in all app routes**: list view, form GET (view), form
EDIT/POST (update + create), and delete. Superusers always bypass the rules.

---

## 7. Configuration & collection import (`/pbx-config`, superuser)

### 7.1 Config editor `/pbx-config`

The one section lists every `_views` (list + form) configuration. For each row you can:

- **List** → open `/tabular/{name}`,
- **Form** → open `/form/{name}`,
- **Edit** → open the friendly JSON config editor,
- **Delete** → remove the config.

**Create collection from source** — two wizard links: **From Excel** and **From MSSQL**.

### 7.2 View config editor `/pbx-config/view/...`

A form that edits the `_tabulator`, `_form` and `_MSSQL` JSON of a `_views` record. You
pick the target collection, name the config (once saved, the endpoint uses this name), and
edit each JSON block in a collapsible textarea with hints:

- `_tabulator` keys: `pageTitle`, `collectionDescr`, `columnTitles`, `columnOrder`,
  `columnSorting`, `searchBox`, `pagination`, `displaySystemCol`, `filter`,
  `columns` (`[{field,title}]`).
- `_form` keys: `formTitle`, `formDescr`, `formLabels` (`field=Label`), `formLayout`
  (`row:(1,2) (3,4) / row:(5,6)`), `displaySystemCol`, `layout`, `labels`.
- `_MSSQL` keys: `dsn`, `table`, `mode` (`insert`/`update`/`replace`), `mapping`
  (`[{pbField,dbField}]`).

### 7.3 Collection import from Excel / MSSQL

`/pbx-config/import-excel` and `/pbx-config/import-mssql` are a **3-step wizard**
(Source → Preview/Edit → Create):

1. **Source** — a collection name + an Excel file/sheet **or** an MSSQL DSN/table.
2. **Preview/Edit** — introspects the source (sheet header / MSSQL `INFORMATION_SCHEMA`) and
   infers PocketBase types (`text`/`number`/`bool`/`date`). You may rename fields, change
   their types, or skip columns.
3. **Create** — the collection is created (plus its fields), and an optional **"import data
   immediately"** box fills it from the source.

Field/collection names are normalized to lowercase `[a-z0-9_]`; reserved system names
(`id`, `created`, `updated`, …) are skipped.

---

## 8. Managing via the standard PocketBase UI (`/_/`)

The classic PocketBase admin console is **always available** on top of PBX (at `/_/`); it is
not disabled. It is useful for things the PBX UI does not cover.

### When to use `/_/` vs `/pbx-setup`

| Task | Recommended place |
|------|-------------------|
| Configure list/form views, layout, labels, MSSQL mapping | **`/pbx-setup` + `/pbx-config`** (user-friendly) |
| Change app dashboard links + icons | **`/pbx-setup`** (record editor) |
| Set data-collection API rules | **`/pbx-setup`** → Collection API rules |
| Create a collection from Excel / MSSQL | **`/pbx-config`** → From Excel / From MSSQL |
| Inspect/polish raw schemas, rename fields, adjust field options | **`/_/`** (Schema editor) |
| Manage `users`, `roles`, and superuser records | **`/_/`** |
| Browse raw records with a grid/table | **`/_/`** → Collection → filter |
| Drop/delete a collection | **`/_/`** |

### Accessing `/_/`

1. Sign in at `/_/` with a **superuser** email/password. (A fresh PBX instance has none —
   create one via the CLI, see §2.1.) All superuser accounts work.
2. The console shows your **collections** and lets you open the **schema** and the
   **record explorer**.

### Core concepts (in `/_/`)

- **Collection** — a table of records (e.g. `zamestnanci`, `produkty`). A collection has
  fields and relations.
- **Field** — one column: `text`, `number`, `bool`, `date`, `select`, `email`,
  `url`, `json`, `file`, etc., with properties such as *required*, *unique*, *options*
  (select choices), and *relationship*.
- **Records** — individual rows you create/edit/delete in the collection explorer.
- **Buckets / files** — uploaded files are stored in the files collection.
- **Users & roles** — standard PocketBase `users`, `roles`, and the `_superusers`
  collection. PBX puts regular people in `users` and administrators in `_superusers`; the
  PBX collection rules (see `/pbx-setup` setup) decide what each record can do.
- **Backups** — in the status sidebar, you can download the SQLite DB backup.

To change a schema (add/modify fields), open a collection in `/_/`, switch to the Schema /
Field editor, and use the field editor. On save, PocketBase creates/updates/may generate a
**migrations** script in `pb_migrations`.

---

## 9. Mobile

PBX auto-detects phone/tablet browsers (server-side **User-Agent**) and redirects the desktop
route to a `/mobile/...` equivalent:

| Intended | Desktop URL | Mobile URL |
|----------|-------------|------------|
| Dashboard | `/app` | `/mobile/app` |
| Table | `/tabular/{configName}` | `/mobile/tabular/{configName}` |
| Form | `/form/{configName}[/{id}]` | `/mobile/form/{configName}[/{id}]` |
| AI agent | `/ai` | `/mobile/ai` |

- **Mobile table** — records render as **cards** (first few visible fields), with a search
  bar, a **FAB +** button for a new record, and pagination. Tap a card to open/edit.
- **Mobile form** — all fields stack in a single column with large touch targets; the
  buttons are at the bottom.
- **Desktop routes are unchanged** — you can still open `/mobile/...` directly on the desktop.

The following stay desktop-only (no redirect): `/pbx-setup`, `/pbx-config`, and the import
wizards — they are superuser tools.

---

## 10. Reference: routes & data

### 10.1 Web routes

| Method+Path | Purpose |
|-------------|---------|
| `GET /login` · `POST /login` | Login dialog / authenticate. |
| `GET /logout` | Clear the auth cookie, go to `/login`. |
| `GET /app` | Dashboard: menu of groups/links (`_app`). |
| `GET /tabular/{configName}` | Table view for a named config. |
| `GET /form/{name}` · `/form/{name}/{id}` | New / edit / (view-only → `?view=1`) form. |
| `POST /form/{name}` · `/form/{name}/{id}` | Create / update. |
| `POST /form/{name}/{id}/delete` | Delete (returns JSON). |
| `GET /api/tabulator-data/{coll}` | Raw JSON (for relation modals). |
| `GET/POST /api/filters/{configName}` · `DELETE /api/filters/{id}` | Saved filters. |
| `GET /export/{coll}` · `POST /import/{coll}` | Excel export / import. |
| `POST /mssql-export/{coll}` · `/mssql-import/{coll}` | SQL Server sync. |
| `GET /mssql-introspect` | List columns of a SQL Server table. |
| `GET /mobile/app` · `/mobile/tabular/...` · `/mobile/form/...` · `/mobile/ai` | Mobile views. |
| `POST /mobile/form/...` · `/mobile/form/.../{id}/delete` | Mobile form create / update / delete. |
| `GET /ai` · `POST /ai/chat` · `POST /ai/chat/stream` · `POST /ai/confirm` | AI agent chat (plain + streaming) and confirm modal. |
| `GET /api/actions/{collectionName}` | List custom actions for a collection. |
| `POST /actions/execute` | Run a custom action (`{actionId, recordIds}`). |
| `GET /api/ai/status` · `/api/ai-config` | AI status / config. |
| `POST /api/theme/{mode}` | Persist global default theme. |
| `POST /api/mssql-dsn` | Persist global DSN. |
| `POST /api/lang/{code}` | Persist global default UI language. |
| `GET /api/lang/{code}/catalog.js` | Client-side translation catalog (`window._t`). |
| `GET /pbx-setup` (superuser landing) + record editors + collection rules editor | Superuser setup hub. |
| `GET /pbx-setup/record/{coll}/{id}` | Record editor for `_app` / `_views` / `_agent` / `_actions`. |
| `POST /pbx-setup/record/{coll}` · `/{id}` · `/{id}/delete` | Save/delete system records. |
| `POST /pbx-setup/rules` | Save collection API rules. |
| `GET /pbx-config` | Config editor overview (list/form configs, edit/delete links). |
| `GET /pbx-config/view/new` · `/view/{name}` · `POST /pbx-config/save` · `/delete` | JSON config editor. |
| `GET/POST /pbx-config/import-excel` · `/pbx-config/import-mssql` | Collection-from-source wizard. |

### 10.2 Template files

| Template | Backs |
|----------|-------|
| `login.html`, `app.html` | Login + dashboard |
| `tabulator.html` | Table (desktop) |
| `mobile-app.html`, `mobile-tabulator.html`, `mobile-form.html` | Phone/tablet |
| `form.html` | Form + view-collection editing |
| `pbxsetup.html` | Superadmin hub + rules |
| `setup-record.html` | Generic system-record editor |
| `config.html`, `pbxconfig.html` | Config editor |
| `import-wizard.html` | Excel / MSSQL collection wizard |
| `agent.html` | AI agent chat |

### 10.3 System (underscore) collections

| Collection | Purpose | Edited from |
|------------|----------|-------------|
| `users` | People | `/_/` (or PBX rules) |
| `_superusers` | Admins | CLI + `/_/` |
| `roles` | Roles | `/_/` |
| `_app` | Dashboard links/groups/icons | `/pbx-setup` record editor |
| `_views` | List+form config (JSON) | `/pbx-config` |
| `_agent` | AI agent config | `/pbx-setup` → AI agent |
| `_actions` | Custom action scripts | `/pbx-setup` record editor |
| `_filters` | Saved advanced filters | saved per-user from `/tabular` |
| `_metadata` | PocketBase internals | `/_/` (system) |

### 10.4 Migration lifecycle

When you start `serve`, PocketBase runs any new `pb_migrations/*.js` scripts. Every schema
change (fields, new columns, etc.) is captured this way. PBX ships migration scripts that
create/extend the underscore collections and the `users._view` landing field.

---

*This document describes PBX as of **Phase 13** — the named feature set above is complete
up to the config model & routing, superuser setup/config editing, view-collection editing,
Excel/MSSQL sync, the collection-from-wizard, mobile views, the per-user `_view` landing,
custom actions, AI-agent action-management tools, multilingual UI (English/Czech), and the
AI agent enhancements (conversation memory, streaming, self-correcting collection lookup,
server-rendered chat answers).*
