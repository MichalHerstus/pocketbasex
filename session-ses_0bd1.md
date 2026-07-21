# Tabular view ImportToExcel button with modal

**Session ID:** ses_0bd141c24ffe3dBFdsk18jXqpR
**Created:** 7/8/2026, 8:09:43 PM
**Updated:** 7/8/2026, 8:16:52 PM

---

## User

add Import button to /tabular view. The button will dispay modal dialog to specify ImportToExcel parameters plus buttons Import and Cancel.

---

## Assistant

Done. Here's what was added:

**`main.go:96-98`** — new `POST /import/{collectionName}` route that calls `pbexcel.ImportFromExcel` with `excelFileName`, `sheetName`, and `mode` form params, then redirects back to the tabulator view.

**`views/tabulator.html`**:
- Purple "Import" button in the toolbar (line 66-68), before the Export button
- Import modal (lines 111-155) with:
  - **File name** input (defaults to collection name)
  - **Sheet name** input (defaults to "Sheet1")
  - **Mode** radio buttons: Insert, Update, Replace
  - **Import** (purple, submits the form) and **Cancel** buttons
- Client-side validation requires both file name and sheet name before submitting

