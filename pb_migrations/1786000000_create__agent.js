/// <reference path="../pb_data/types.d.ts" />
// Create the _agent collection storing the built-in AI agent configuration.
// Each record holds:
//   _name        - config name (the agent uses the record named "default")
//   _description - human readable description
//   _config      - JSON with the LLM provider settings
// Rules are null (superuser only), same as _views/_app.
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
        "help": "Unique configuration name (the agent uses the record named \"default\")",
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
        "help": "Human readable description of this agent configuration",
        "hidden": false,
        "id": "text4511001002",
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
        "hidden": false,
        "id": "json4511001003",
        "maxSize": 0,
        "name": "_config",
        "presentable": false,
        "required": false,
        "system": false,
        "type": "json"
      }
    ],
    "id": "pbc_4510000001",
    "indexes": [],
    "listRule": null,
    "name": "_agent",
    "system": false,
    "type": "base",
    "updateRule": null,
    "viewRule": null
  });

  app.save(collection);
}, (app) => {
  const agentColl = app.findCollectionByNameOrId("_agent")

  const all = app.findRecordsByFilter("_agent", "", "", 0, 0)
  for (const r of all) {
    app.delete(r)
  }

  app.delete(agentColl)
})