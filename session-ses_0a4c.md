# Implement filter button in tabular page

**Session ID:** ses_0a4cc9bebffelPibZ3v8HGqHTp
**Created:** 7/13/2026, 1:18:42 PM
**Updated:** 7/13/2026, 1:49:21 PM

---

## User

Implement filter button functionality in /tabular page.
Filter definition: {condition1} {chain} {condition2}..etc. Exapmle “(Price > 10) && (Price < 100)”. Filter definition is stored in field “filter” of _tabulator collection.
Fileter definition syntax: 
	condition operators: < (less then),> (bigger then),<= (less or equal then), => (equal or bigger then), != (not equal), ~ (contain)
    can be applied to string type fields: =, !=, ~
    number, datetime fields: only <,>,<=, =>, !=
    selection type of field is treated as string, rich editor as well. Other types (relation, GPS, JSON) cannot be used in filters.
    if “?” is used asi value in condition, modal dialog is diplayed to allow user enter the values for filter. Dialog contain also 2 buttons - “Set 	filter” (= switch filter ON) and “Cancel filter” (switch filter off).
    If there are not inputs (means no “?” in definition), filter is applied automatically when Filter button is pressed. 

---

## Assistant

I'll start by exploring the codebase to understand the current tabulator implementation.

**Tool: task**

---

## Assistant

Let me read the key files in detail.

**Tool: read**

**Tool: read**

**Tool: read**

---

## Assistant

Now I have a complete understanding. Let me create the plan.

**Tool: todowrite**

---

## Assistant

Here's my implementation plan for the filter button functionality:

## Plan: Filter Button on `/tabulator` Page

### Overview

The filter button will parse a filter definition string from the `_tabulator` collection's `filter` field, parse conditions like `(Price > 10) && (Price < 100)`, and filter records client-side. If the definition contains `?` placeholder values, a modal dialog prompts the user to enter values before applying.

### Files to modify

| File | Change |
|------|--------|
| `views/pages.go:8-17` | Add `Filter string` field to `TabulatorConfig` struct |
| `main.go:270-280` | Read `filter` field from config record into `cfg.Filter` |
| `views/tabulator.html` | All JS + modal HTML + CSS for filter functionality |

### JavaScript Implementation (in `tabulator.html`)

**1. `parseFilterDef(def)` — Tokenizer/Parser**
- Splits the filter definition into tokens respecting parentheses and quoted strings
- Extracts conditions: `{field} {operator} {value}` and chain operators (`&&`, `||`)
- Returns structured array of conditions + chains

**2. `evaluateFilter(record, conditions)` — Evaluator**
- Evaluates all conditions against a record, joined by chain operators
- Operators: `<`, `>`, `<=`, `=>`, `!=`, `=`, `~`
- Type-aware: string fields only allow `=`, `!=`, `~`; number/datetime fields allow `<`, `>`, `<=`, `=>`, `!=`
- String comparison is case-insensitive; `~` is substring containment
- Number values are parsed with `parseFloat()`

**3. Filter Modal HTML**
- New `filterModal` overlay with dynamically generated input fields (one per `?` placeholder)
- Two buttons: "Set filter" (apply + close) and "Cancel filter" (clear filter + close)

**4. Filter Button Logic**
- On click: if filter def has `?` placeholders → open modal; otherwise apply filter directly
- Filter state stored in `filterActive` (boolean) and `filterValues` (array of user-entered values)
- "Set filter" sets `filterActive = true`, applies filter, closes modal
- "Cancel filter" sets `filterActive = false`, clears filter, closes modal

**5. Integration with `getFiltered()`**
- After search filtering and before sorting, apply the active filter via `evaluateFilter()`

### Filter Definition Syntax (matching your spec)

| Operator | Allowed field types | Meaning |
|----------|-------------------|---------|
| `<` | number, datetime | Less than |
| `>` | number, datetime | Greater than |
| `<=` | number, datetime | Less or equal |
| `=>` | number, datetime | Greater or equal |
| `!=` | all (string, number, datetime) | Not equal |
| `=` | string only | Equal |
| `~` | string only | Contains |

Chain operators: `&&` (AND), `||` (OR)

**Example**: `(Price > 10) && (Price < 100)` — filters records where Price is between 10 and 100.

**Example with `?`**: `(Price > ?) && (Price < ?)` — modal shows two input fields for min/max price.

**Tool: question**

---

## Assistant

I'll proceed with `=>` as written in your spec, and support both `&&` and `||` chain operators. Let me implement everything now.

**Tool: todowrite**

---

## Assistant

**Tool: edit**

---

## Assistant

**Tool: todowrite**

---

## Assistant

**Tool: edit**

---

## Assistant

**Tool: todowrite**

---

## Assistant

Now let me implement the JavaScript. I'll start with adding CSS for filter modal inputs.

**Tool: edit**

---

## Assistant

Now add the filter modal HTML after the export modal.

**Tool: edit**

---

## Assistant

Now replace the filter toggle stub with real filter button logic, and add the parser/evaluator. I'll add the filter functions right after the state variables and replace the filter toggle code.

**Tool: edit**

---

## Assistant

Now I need to inject the filter definition from Go template and add the filter button click handler. Let me add this right after the filterModal click handler, and also wire the filter into `getFiltered()`.

**Tool: edit**

---

## Assistant

Now wire the filter into `getFiltered()`.

**Tool: edit**

---

## Assistant

**Tool: todowrite**

---

## Assistant

**Tool: bash**

---

## Assistant

Build and vet passed. Let me verify the final state of the modified files.

**Tool: read**

**Tool: read**

---

## Assistant

**Tool: read**

**Tool: read**

---

## Assistant

**Tool: read**

**Tool: read**

---

## Assistant

**Tool: read**

---

## Assistant

**Tool: todowrite**

---

## Assistant

All done. Build and vet passed. Here's a summary of what was implemented:

### Changes made

**`views/pages.go:17`** — Added `Filter string` field to `TabulatorConfig`

**`main.go:280`** — Reads `filter` field from `_tabulator` config record

**`views/tabulator.html`** — All JavaScript filter logic:
- **`parseFilterDef(def)`** — Tokenizes filter string like `(Price > 10) && (Price < 100)` into structured conditions + chains
- **`evaluateFilter(record, parsed)`** — Evaluates a record against parsed conditions with type-aware comparisons
- **`compareValues()`** — Handles all 6 operators with proper type coercion (string case-insensitive, number parseFloat, datetime string comparison)
- **Filter modal** — Dynamic form with input fields per `?` placeholder, type-appropriate inputs (number/text/datetime-local)
- **Button behavior**: No `?` → toggle filter on/off directly; with `?` → open modal with "Set filter" / "Cancel filter" buttons
- **`getFiltered()`** — Now applies the active filter between search and sort

### How to use

In the `_tabulator` collection, set the `filter` field on a record:

| Filter definition | Behavior |
|---|---|
| `(Price > 10) && (Price < 100)` | Click Filter → toggles on/off directly |
| `(Price > ?) && (Price < ?)` | Click Filter → modal asks for min/max values |
| `(Name ~ "test")` | String contains "test" |
| `(Status = "active") && (Created > "2024-01-01")` | Combines string equality with date comparison |

---

## User

instead just  "Filter is set" label display "Filter is: " plus the filter definition.

---

## Assistant

**Tool: read**

---

## Assistant

**Tool: grep**

---

## Assistant

I need to inject the filter definition string into JS and update the label text. Two edits:

**Tool: edit**

---

## Assistant

**Tool: read**

---

## Assistant

**Tool: read**

---

## Assistant

**Tool: edit**

---

## Assistant

Now update `applyFilterDef` to display the filter definition in the label.

**Tool: edit**

---

## Assistant

**Tool: bash**

---

## Assistant

Done. The filter label now shows `Filter is: (Price > 10) && (Price < 100)` (the actual filter definition) instead of just "Filter is set".

---

## User

instead of ? show in the filter label the actual applied value.

---

## Assistant

**Tool: read**

---

## Assistant

**Tool: edit**

---

## Assistant

Done. Now when the user enters values like `10` and `100`, the label shows `Filter is: (Price > 10) && (Price < 100)` instead of `Filter is: (Price > ?) && (Price < ?)`.

---

## User

in both filter label and also in dialog use columnTitles instead of field names.

---

## Assistant

**Tool: read**

---

## Assistant

**Tool: read**

---

## Assistant

Two changes: add a column title lookup helper, then use it in the label and modal.

**Tool: edit**

---

## Assistant

Now update the label to use column titles instead of field names:

**Tool: edit**

---

## Assistant

**Tool: read**

---

## Assistant

Let me fix the display string construction to properly interleave conditions with chains:

**Tool: edit**

---

## Assistant

Now update the modal to use column titles:

**Tool: edit**

---

## Assistant

**Tool: bash**

---

## Assistant

Done. Both the filter label and modal dialog now show column titles (from `columnTitles`) instead of raw field names. For example, if field `price` has column title `Cena`, the label shows `Filter is: (Cena > 10) && (Cena < 100)` and the modal shows `Cena >` above the input.

---

