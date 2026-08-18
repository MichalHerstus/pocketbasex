/// <reference path="../pb_data/types.d.ts" />
// Create the unified _views collection that consolidates _tabulator and _form
// configuration into a single record per view, then backfill from the legacy
// collections. Each record has:
//   _name      - configuration name (endpoint /tabular/{_name}, /form/{_name})
//   _collName  - collection the view is configured for
//   _tabulator - JSON with all tabulator settings (pageTitle, columnTitles, ...)
//   _form      - JSON with all form settings (formTitle, formLabels, layout, ...)
//   _mssql     - JSON with MSSQL exp/imp config
migrate((app) => {
  const collection = new Collection({
    "createRule": null,
    "deleteRule": null,
    "fields": [
      {
        "autogeneratePattern": "[a-z0-9]{15}",
        "help": "",
        "hidden": false,
        "id": "text3208210256",
        "max": 15,
        "min": 15,
        "name": "id",
        "pattern": "^[a-z0-9]+$",
        "presentable": false,
        "primaryKey": true,
        "required": true,
        "system": true,
        "type": "text"
      },
      {
        "hidden": false,
        "id": "autodate2990389176",
        "name": "created",
        "onCreate": true,
        "onUpdate": false,
        "presentable": false,
        "system": false,
        "type": "autodate"
      },
      {
        "hidden": false,
        "id": "autodate3332085495",
        "name": "updated",
        "onCreate": true,
        "onUpdate": true,
        "presentable": false,
        "system": false,
        "type": "autodate"
      },
      {
        "autogeneratePattern": "",
        "help": "Unique configuration name used in the endpoint URL (/tabular/{_name}, /form/{_name})",
        "hidden": false,
        "id": "text4511001001",
        "max": 0,
        "min": 0,
        "name": "_name",
        "pattern": "",
        "presentable": false,
        "primaryKey": false,
        "required": true,
        "system": false,
        "type": "text"
      },
      {
        "autogeneratePattern": "",
        "help": "Name of the collection this view is configured for",
        "hidden": false,
        "id": "text4511001002",
        "max": 0,
        "min": 0,
        "name": "_collName",
        "pattern": "",
        "presentable": false,
        "primaryKey": false,
        "required": false,
        "system": false,
        "type": "text"
      },
      {
        "hidden": false,
        "id": "json4511001003",
        "maxSize": 0,
        "name": "_tabulator",
        "presentable": false,
        "required": false,
        "system": false,
        "type": "json"
      },
      {
        "hidden": false,
        "id": "json4511001004",
        "maxSize": 0,
        "name": "_form",
        "presentable": false,
        "required": false,
        "system": false,
        "type": "json"
      },
      {
        "hidden": false,
        "id": "json4511001005",
        "maxSize": 0,
        "name": "_mssql",
        "presentable": false,
        "required": false,
        "system": false,
        "type": "json"
      }
    ],
    "id": "pbc_4511000001",
    "indexes": [],
    "listRule": null,
    "name": "_views",
    "system": false,
    "type": "base",
    "updateRule": null,
    "viewRule": null
  });

  app.save(collection);

  const viewsColl = app.findCollectionByNameOrId("_views")

  // backfill _tabulator -> _views._tabulator
  const tabRecs = app.findRecordsByFilter("_tabulator", "", "", 0, 0)
  for (const t of tabRecs) {
    let existing = app.findRecordsByFilter("_views", "_name = {:n}", "", 1, 0, { n: t.get("_name") })
    let rec = existing.length > 0 ? existing[0] : null
    const isNew = !rec
    if (!rec) {
      rec = new Record(viewsColl)
      rec.set("_name", t.get("_name"))
    }
    rec.set("_collName", t.get("collName"))

    const tab = {}
    for (const f of ["pageTitle", "collectionDescr", "columnTitles", "columnOrder", "filter"]) {
      const v = t.get(f)
      if (v) tab[f] = v
    }
    for (const f of ["columnSorting", "searchBox", "pagination", "displaySystemCol"]) {
      tab[f] = !!t.get(f)
    }
    const cfg = t.get("config")
    if (cfg && cfg.columns) tab.columns = cfg.columns

    rec.set("_tabulator", tab)

    if (t.get("_mssql")) rec.set("_mssql", t.get("_mssql"))
    app.save(rec)
  }

  // backfill _form -> _views._form
  const formRecs = app.findRecordsByFilter("_form", "", "", 0, 0)
  for (const f of formRecs) {
    let existing = app.findRecordsByFilter("_views", "_name = {:n}", "", 1, 0, { n: f.get("_name") })
    let rec = existing.length > 0 ? existing[0] : null
    const isNew = !rec
    if (!rec) {
      rec = new Record(viewsColl)
      rec.set("_name", f.get("_name"))
    }
    rec.set("_collName", f.get("collName"))

    const fv = {}
    for (const k of ["formTitle", "formDescr", "formLabels", "columnOrder", "formLayout"]) {
      const v = f.get(k)
      if (v) fv[k] = v
    }
    fv.displaySystemCol = !!f.get("displaySystemCol")

    const cfg = f.get("config")
    if (cfg) {
      for (const k of ["layout", "labels", "collections"]) {
        if (cfg[k]) fv[k] = cfg[k]
      }
    }

    rec.set("_form", fv)
    app.save(rec)
  }
}, (app) => {
  const viewsColl = app.findCollectionByNameOrId("_views")

  // remove all _views records created by this migration (only ones with _tabulator/_form set)
  const allViews = app.findRecordsByFilter("_views", "", "", 0, 0)
  for (const v of allViews) {
    if (v.get("_tabulator") || v.get("_form")) {
      app.delete(v)
    }
  }

  app.delete(viewsColl)
})
