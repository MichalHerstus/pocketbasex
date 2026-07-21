# Add /pbx-setup for setup collections tables

**Session ID:** ses_0ba216343ffeVlgjVF0AfqZZb3
**Created:** 7/9/2026, 9:54:04 AM
**Updated:** 7/9/2026, 9:58:03 AM

---

## User

let add new endpoint /pbx-setup. It displays all three "setup" collections (_app,_tabular, _form) as tables on 1 page. Same capabilitis like in /tabular, just omitt search and export,import.

---

## Assistant

**Tool: read**

**Tool: read**

---

## Assistant

**Tool: read**

**Tool: read**

**Tool: read**

---

## Assistant

**Tool: read**

---

## Assistant

**Tool: todowrite**

**Tool: edit**

---

## Assistant

**Tool: todowrite**

---

## Assistant

**Tool: write**

---

## Assistant

**Tool: todowrite**

**Tool: edit**

---

## Assistant

Now add the `handlePbxSetup` handler and the route.

**Tool: edit**

---

## Assistant

**Tool: edit**

---

## Assistant

**Tool: todowrite**

**Tool: bash**

---

## Assistant

Build and vet pass cleanly.

**Tool: todowrite**

---

## Assistant

Done. Here's what was changed:

- **`main.go`**: Extracted `buildTabulatorData()` helper from `handleTabulator` to avoid duplication. Added `handlePbxSetup()` handler and `GET /pbx-setup` route. Loads `_app`, `_tabulator`, `_form` collections.

- **`views/pages.go`**: Added `PbxSetupPageData` struct with `Sections []TabulatorPageData`.

- **`views/pbxsetup.html`**: New template rendering each collection as a table with sorting, pagination, view/edit/delete actions, and the relation modal. No search, filter, import, or export.

---

## User

I got error, when run /pbx-setup: {"data":{},"message":"The requested resource wasn't found.","status":404}. Please fix.

---

## Assistant

**Tool: read**

---

## Assistant

---

