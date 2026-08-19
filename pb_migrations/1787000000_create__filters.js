/// <reference path="../pb_data/types.d.ts" />
// Create the _filters collection storing named (saved) advanced filters for
// tabular/views. Each record holds:
//   _name   - filter name
//   _coll   - collection name the filter applies to
//   _config - _views config name (the /tabular/{config} URL key)
//   _user   - owner (pb user record id); superusers manage all
//   _def    - JSON filter definition {conditions:[{field,op,value}], chains:[...]}
// Rules are null (superuser only via direct REST); the app routes enforce
// ownership server-side (same pattern as _views/_agent/_app).
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
        "help": "Filter name",
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
        "help": "Collection name the filter applies to",
        "hidden": false,
        "id": "text4511001002",
        "max": 0,
        "min": 0,
        "name": "_coll",
        "pattern": "",
        "presentable": false,
        "primaryKey": false,
        "required": true,
        "system": false,
        "type": "text"
      },
      {
        "autogeneratePattern": "",
        "help": "_views config name (the /tabular/{config} URL key)",
        "hidden": false,
        "id": "text4511001003",
        "max": 0,
        "min": 0,
        "name": "_config",
        "pattern": "",
        "presentable": false,
        "primaryKey": false,
        "required": true,
        "system": false,
        "type": "text"
      },
      {
        "autogeneratePattern": "",
        "help": "Owner user id; superusers manage all filters",
        "hidden": false,
        "id": "text4511001004",
        "max": 0,
        "min": 0,
        "name": "_user",
        "pattern": "",
        "presentable": false,
        "primaryKey": false,
        "required": true,
        "system": false,
        "type": "text"
      },
      {
        "hidden": false,
        "id": "json4511001005",
        "maxSize": 0,
        "name": "_def",
        "presentable": false,
        "required": false,
        "system": false,
        "type": "json"
      }
    ],
    "id": "pbc_4510000002",
    "indexes": [],
    "listRule": null,
    "name": "_filters",
    "system": false,
    "type": "base",
    "updateRule": null,
    "viewRule": null
  });

  app.save(collection);
}, (app) => {
  const filtersColl = app.findCollectionByNameOrId("_filters")

  const all = app.findRecordsByFilter("_filters", "", "", 0, 0)
  for (const r of all) {
    app.delete(r)
  }

  app.delete(filtersColl)
})