/// <reference path="../pb_data/types.d.ts" />
migrate((app) => {
  // --- _tabulator: add _name, config, _mssql ---
  const tab = app.findCollectionByNameOrId("_tabulator")

  // add field
  tab.fields.addAt(1, new Field({
    "autogeneratePattern": "",
    "help": "Unique configuration name used in the endpoint URL (/tabular/{_name})",
    "hidden": false,
    "id": "text9115051001",
    "max": 0,
    "min": 0,
    "name": "_name",
    "pattern": "",
    "presentable": false,
    "primaryKey": false,
    "required": true,
    "system": false,
    "type": "text"
  }))

  // add field
  tab.fields.addAt(2, new Field({
    "hidden": false,
    "id": "json9115051002",
    "maxSize": 0,
    "name": "config",
    "presentable": false,
    "required": false,
    "system": false,
    "type": "json"
  }))

  // add field
  tab.fields.addAt(3, new Field({
    "hidden": false,
    "id": "json9115051003",
    "maxSize": 0,
    "name": "_mssql",
    "presentable": false,
    "required": false,
    "system": false,
    "type": "json"
  }))

  app.save(tab)

  // backfill _name from collName
  const tabRecs = app.findRecordsByFilter("_tabulator", "", "", 0, 0)
  for (const r of tabRecs) {
    if (!r.get("_name")) {
      r.set("_name", r.get("collName"))
      app.save(r)
    }
  }

  // --- _form: add _name, config ---
  const form = app.findCollectionByNameOrId("_form")

  // add field
  form.fields.addAt(1, new Field({
    "autogeneratePattern": "",
    "help": "Unique configuration name used in the endpoint URL (/form/{_name})",
    "hidden": false,
    "id": "text9115052001",
    "max": 0,
    "min": 0,
    "name": "_name",
    "pattern": "",
    "presentable": false,
    "primaryKey": false,
    "required": true,
    "system": false,
    "type": "text"
  }))

  // add field
  form.fields.addAt(2, new Field({
    "hidden": false,
    "id": "json9115052002",
    "maxSize": 0,
    "name": "config",
    "presentable": false,
    "required": false,
    "system": false,
    "type": "json"
  }))

  app.save(form)

  // backfill _name from collName
  const formRecs = app.findRecordsByFilter("_form", "", "", 0, 0)
  for (const r of formRecs) {
    if (!r.get("_name")) {
      r.set("_name", r.get("collName"))
      app.save(r)
    }
  }

  // --- _app: add configName ---
  const appColl = app.findCollectionByNameOrId("_app")

  // add field
  appColl.fields.addAt(1, new Field({
    "autogeneratePattern": "",
    "help": "Optional list configuration _name to link to (/tabular/{configName}); falls back to the default config of the collection",
    "hidden": false,
    "id": "text9115053001",
    "max": 0,
    "min": 0,
    "name": "configName",
    "pattern": "",
    "presentable": false,
    "primaryKey": false,
    "required": false,
    "system": false,
    "type": "text"
  }))

  app.save(appColl)
}, (app) => {
  const appColl = app.findCollectionByNameOrId("_app")
  appColl.fields.removeById("text9115053001")
  app.save(appColl)

  const form = app.findCollectionByNameOrId("_form")
  form.fields.removeById("text9115052001")
  form.fields.removeById("json9115052002")
  app.save(form)

  const tab = app.findCollectionByNameOrId("_tabulator")
  tab.fields.removeById("text9115051001")
  tab.fields.removeById("json9115051002")
  tab.fields.removeById("json9115051003")
  app.save(tab)
})