# Plan: Search & Filter Layout Redesign

## Current State

The tabular view (`views/tabulator.html`) has:
- A plain search `<input>` (no clear button)
- A "Filter" `<button>` that opens the full advanced filter builder modal (`#advFilterModal`)
- A `#filterModal` for parameterized filters (prompts for `?` placeholder values)
- Saved filters loaded via `GET /api/filters/{configName}` into the builder's dropdown
- Static filter from `_views._tabulator.filter` auto-applied on page load
- All filtering is client-side in `getFiltered()` (lines 356-393)

## Target Layout

```
[—search input——[x]]      [—filter selector——[v]] [Edit filters]
```

- **Search input** with `[x]` clear button (clears search text only)
- **Filter dropdown** shows: `--All records--` (default) + all saved filters
- **Edit filters** button opens the existing `#advFilterModal` builder
- When a parameterized filter is selected from the dropdown, a parameter dialog appears with type-appropriate inputs
- `relation` fields are excluded from filtering

---

## Changes

### 1. `views/pages.go` — Add select field options to template data

**What**: Add `FieldOptionsJSON string` to `TabulatorPageData` (line 109). This passes a JSON map of `{fieldName: [option1, option2, ...]}` for `select` fields, so the parameter dialog can render dropdowns.

**Where**: `views/pages.go` line ~109 (TabulatorPageData struct)

**Add field**:
```go
FieldOptionsJSON string  // JSON map: {fieldName: [option1, option2, ...]} for select fields
```

### 2. `main.go` — Build select field options in `buildTabulatorData`

**What**: In `buildTabulatorData` (line 746), after building `fieldTypes`, iterate `visibleFields` and for each `*core.SelectField`, collect `sf.Values` into a `map[string][]string`. Marshal to JSON and pass as `FieldOptionsJSON`.

**Where**: `main.go` ~line 925 (after `fieldTypesJSON`)

**Logic**:
```go
selectOpts := map[string][]string{}
for i, f := range visibleFields {
    if sf, ok := f.(*core.SelectField); ok {
        selectOpts[fieldNames[i]] = sf.Values
    }
}
fieldOptionsJSON, _ := json.Marshal(selectOpts)
```

Then set `FieldOptionsJSON: string(fieldOptionsJSON)` in the returned struct (~line 935).

### 3. `views/tabulator.html` — HTML changes (toolbar, ~lines 89-104)

**Replace** the current toolbar section with:

```html
<div style="display:flex;align-items:center;gap:12px;margin-bottom:12px;flex-wrap:wrap;">
    {{if .Config.SearchBox}}
    <div class="search-box" style="margin-bottom:0;display:flex;align-items:center;gap:4px;">
        <input type="text" id="searchInput" placeholder="Search across all columns…" style="flex:1;">
        <button id="searchClearBtn" style="padding:4px 8px;background:none;border:none;cursor:pointer;font-size:1rem;color:#888;" title="Clear search">&times;</button>
    </div>
    {{end}}
    <select id="filterSelect" style="padding:8px 12px;border:1px solid #ccc;border-radius:6px;font-size:.9rem;min-width:180px;">
        <option value="">-- All records --</option>
    </select>
    <button id="editFiltersBtn" style="padding:8px 16px;background:var(--btn-neutral);color:#fff;border:none;border-radius:6px;font-size:.9rem;cursor:pointer;">Edit filters</button>
    <!-- Import/Export/Add unchanged -->
    <button id="importBtn" ...>Import</button>
    <button id="exportBtn" ...>Export</button>
    <a href="/form/{{.ConfigName}}" ...>+ Add new</a>
</div>
```

**Remove**: `#filterLabel` span and `#filterBtn` button (replaced by `#filterSelect` and `#editFiltersBtn`).

### 4. `views/tabulator.html` — Parameter dialog HTML (replace `#filterModal`)

**Replace** the existing `#filterModal` (lines 240-249) with a more structured version:

```html
<!-- Filter parameter prompt -->
<div class="modal-overlay" id="filterModal">
    <div class="modal-content" style="max-width:400px;">
        <div class="modal-header">
            <h3 id="filterModalTitle">Fill filter parameters</h3>
            <button class="modal-close" onclick="closeFilterModal()">&times;</button>
        </div>
        <div class="modal-body" id="filterModalBody"></div>
        <div style="display:flex;gap:8px;padding:0 20px 20px;">
            <button onclick="submitFilterParams()" style="flex:1;padding:10px;background:var(--btn-primary);color:#fff;border:none;border-radius:6px;font-size:1rem;cursor:pointer;">Use filter</button>
            <button onclick="cancelFilterParams()" style="flex:1;padding:10px;background:var(--btn-neutral);color:#fff;border:none;border-radius:6px;font-size:1rem;cursor:pointer;">Cancel</button>
        </div>
    </div>
</div>
```

### 5. `views/tabulator.html` — JavaScript changes

#### 5a. Add `fieldOptions` variable (after line 290)

```javascript
const fieldOptions = {{.FieldOptionsJSON | safeJS}};
```

#### 5b. Add search clear button handler (after line 441)

```javascript
document.getElementById("searchClearBtn").addEventListener("click", function () {
    document.getElementById("searchInput").value = "";
    searchTerm = "";
    currentPage = 1;
    render();
});
```

#### 5c. Add filter dropdown population function

```javascript
function populateFilterDropdown() {
    var sel = document.getElementById("filterSelect");
    while (sel.options.length > 1) sel.remove(1);
    savedFilters.forEach(function(f) {
        var opt = document.createElement("option");
        opt.value = f.id;
        opt.textContent = f.name + (f.user ? " (" + f.user + ")" : "");
        sel.appendChild(opt);
    });
}
```

#### 5d. Add filter dropdown change handler

```javascript
document.getElementById("filterSelect").addEventListener("change", function () {
    var selectedId = this.value;
    if (!selectedId) {
        clearFilter();
        return;
    }
    var filter = savedFilters.find(function(f) { return f.id === selectedId; });
    if (!filter || !filter.def) return;
    var def = filter.def;
    var hasParams = def.conditions && def.conditions.some(function(c) { return c.value === '?'; });
    if (hasParams) {
        paramOnDone = function(values) { activateFilterWithValues(def, values); };
        currentParamFilterId = selectedId;
        promptFilterParams(def);
    } else {
        activateFilterWithValues(def, []);
    }
});
```

#### 5e. Add edit filters button handler

```javascript
document.getElementById("editFiltersBtn").addEventListener("click", function () {
    openAdvFilterModal();
});
```

#### 5f. Modify `loadSavedFilters()` (~line 853)

After fetching filters, call `populateFilterDropdown()`:

```javascript
function loadSavedFilters() {
    fetch('/api/filters/{{.ConfigName}}')
        .then(function(r) { return r.json(); })
        .then(function(data) {
            savedFilters = data.filters || [];
            populateFilterDropdown();
            // Also populate the builder's saved filter dropdown (existing logic)
            var sel = document.getElementById("savedFilterSelect");
            if (sel) {
                sel.innerHTML = '';
                savedFilters.forEach(function(f) {
                    var opt = document.createElement("option");
                    opt.value = f.id;
                    opt.textContent = f.name + (f.user ? " (" + f.user + ")" : "");
                    sel.appendChild(opt);
                });
            }
        });
}
```

#### 5g. Modify `clearFilter()` (~line 618)

Reset the dropdown to "All records":

```javascript
function clearFilter() {
    filterActive = false;
    activeFilter = null;
    filterValues = [];
    var sel = document.getElementById("filterSelect");
    if (sel) sel.value = "";
    currentPage = 1;
    render();
}
```

#### 5h. Modify `activateFilter()` (~line 591)

Update dropdown selection to match active filter:

```javascript
function activateFilter(def) {
    activeFilter = def;
    filterActive = true;
    var sel = document.getElementById("filterSelect");
    if (sel && def && def.name) {
        for (var i = 0; i < sel.options.length; i++) {
            if (sel.options[i].textContent === def.name || sel.options[i].textContent.startsWith(def.name + " (")) {
                sel.value = sel.options[i].value;
                break;
            }
        }
    }
    currentPage = 1;
    render();
    closeAdvFilterModal();
}
```

#### 5i. Modify `promptFilterParams()` (~line 630)

Update to use new dialog structure with type-appropriate inputs:
- `select` fields: `<select>` dropdown with options from `fieldOptions[fieldName]`
- `bool` fields: `<select>` with `true`/`false`
- `number` fields: `<input type="number">`
- `date`/`autodate` fields: `<input type="datetime-local">`
- `text`/other fields: `<input type="text">`

Set dialog title: `document.getElementById("filterModalTitle").textContent = "Fill filter parameters — " + def.name;`

#### 5j. Add `cancelFilterParams()` function

```javascript
function cancelFilterParams() {
    closeFilterModal();
    paramOnDone = null;
    currentParamFilterId = null;
    var sel = document.getElementById("filterSelect");
    if (sel && !filterActive) sel.value = "";
}
```

#### 5k. Add `currentParamFilterId` variable

```javascript
let currentParamFilterId = null;
```

#### 5l. Remove `#filterLabel` and `#filterBtn` references

Remove all references to `document.getElementById("filterLabel")` and `document.getElementById("filterBtn")`.

---

## Files Modified

| File | Action | Lines (est.) |
|------|--------|-------------|
| `views/pages.go` | Edit — add `FieldOptionsJSON` to `TabulatorPageData` | +1 |
| `main.go` | Edit — build select options in `buildTabulatorData` | +10 |
| `views/tabulator.html` | Edit — new toolbar HTML, dropdown JS, parameter dialog, remove old filter button | +60 / -30 |

**Total**: ~70 new lines, ~30 removed lines.

## Verification

- `go build ./... && go vet ./...`
- Load a tabular view → search input has `[x]` clear button, filter dropdown shows `-- All records --` + saved filters
- Clear button clears search text, does not affect filter
- Select a parameterized filter → parameter dialog appears with correct input types
- Select `-- All records --` → filter clears, all records shown
- "Edit filters" button opens the existing advanced filter builder
- After saving a filter in the builder, the dropdown updates to include it
- `select` field parameters render as dropdowns with correct options
- `relation` fields do not appear in filter parameters
