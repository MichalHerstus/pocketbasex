/// <reference path="../pb_data/types.d.ts" />
// Create the _actions collection storing user-defined Goja scripts that run
// from the tabular and form views (Phase 10).
// Each record holds:
//   _name        - display name (e.g. "Export overdue tasks")
//   _description - help text shown in the UI
//   _script      - JavaScript (Goja) source executed by the pbactions runner
//   _collection  - target collection name
//   _onList      - show in tabular view dropdown (default true)
//   _onForm      - show in form view dropdown (default false)
//   _public      - visible to non-superusers (default false)
// Rules are null (superuser only) - only admins create/edit actions.
migrate((app) => {
  const collection = new Collection({
    "createRule": null,
    "deleteRule": null,
    "fields": [
      {
        "autogeneratePattern": "[a-z0-9]{15}",
        "help": "",
        "hidden": false,
        "id": "text4520000001",
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
        "id": "autodate4520000002",
        "name": "created",
        "onCreate": true,
        "onUpdate": false,
        "presentable": false,
        "system": false,
        "type": "autodate"
      },
      {
        "hidden": false,
        "id": "autodate4520000003",
        "name": "updated",
        "onCreate": true,
        "onUpdate": true,
        "presentable": false,
        "system": false,
        "type": "autodate"
      },
      {
        "autogeneratePattern": "",
        "help": "Display name of the action (e.g. 'Export overdue tasks')",
        "hidden": false,
        "id": "text4520001001",
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
        "help": "Help text shown in the UI",
        "hidden": false,
        "id": "text4520001002",
        "max": 0,
        "min": 0,
        "name": "_description",
        "pattern": "",
        "presentable": false,
        "primaryKey": false,
        "required": false,
        "system": false,
        "type": "text"
      },
      {
        "autogeneratePattern": "",
        "help": "JavaScript (Goja) source executed by the action runner",
        "hidden": false,
        "id": "text4520001003",
        "max": 0,
        "min": 0,
        "name": "_script",
        "pattern": "",
        "presentable": false,
        "primaryKey": false,
        "required": true,
        "system": false,
        "type": "editor"
      },
      {
        "autogeneratePattern": "",
        "help": "Target collection name the action operates on",
        "hidden": false,
        "id": "text4520001004",
        "max": 0,
        "min": 0,
        "name": "_collection",
        "pattern": "",
        "presentable": false,
        "primaryKey": false,
        "required": true,
        "system": false,
        "type": "text"
      },
      {
        "hidden": false,
        "id": "bool4520001005",
        "name": "_onList",
        "presentable": false,
        "required": false,
        "system": false,
        "type": "bool"
      },
      {
        "hidden": false,
        "id": "bool4520001006",
        "name": "_onForm",
        "presentable": false,
        "required": false,
        "system": false,
        "type": "bool"
      },
      {
        "hidden": false,
        "id": "bool4520001007",
        "name": "_public",
        "presentable": false,
        "required": false,
        "system": false,
        "type": "bool"
      }
    ],
    "id": "pbc_4520000001",
    "indexes": [],
    "listRule": null,
    "name": "_actions",
    "system": false,
    "type": "base",
    "updateRule": null,
    "viewRule": null
  });

  app.save(collection);
}, (app) => {
  const coll = app.findCollectionByNameOrId("_actions")

  const all = app.findRecordsByFilter("_actions", "", "", 0, 0)
  for (const r of all) {
    app.delete(r)
  }

  app.delete(coll)
})