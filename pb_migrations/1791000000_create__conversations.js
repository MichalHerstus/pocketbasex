/// <reference path="../pb_data/types.d.ts" />
// Create the _conversations collection storing persisted AI chat histories.
// Each record holds:
//   _title    - human readable conversation title (auto-derived from 1st user msg)
//   _messages - JSON array of [{id, role, content}] turns
//   _user     - owner (pb user record id); superusers manage all
//   _model    - model id used for the conversation
// Rules are null (superuser only via direct REST); the app routes enforce
// ownership server-side (same pattern as _views/_agent/_app/_filters).
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
        "help": "Conversation title (auto-derived from the first user message)",
        "hidden": false,
        "id": "text4511001001",
        "max": 0,
        "min": 0,
        "name": "_title",
        "pattern": "",
        "presentable": false,
        "primaryKey": false,
        "required": false,
        "system": false,
        "type": "text"
      },
      {
        "help": "Array of {id, role, content} message turns",
        "hidden": false,
        "id": "json4511001002",
        "maxSize": 0,
        "name": "_messages",
        "presentable": false,
        "required": true,
        "system": false,
        "type": "json"
      },
      {
        "autogeneratePattern": "",
        "help": "Owner user id; superusers manage all conversations",
        "hidden": false,
        "id": "text4511001003",
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
        "autogeneratePattern": "",
        "help": "Model id used for the conversation",
        "hidden": false,
        "id": "text4511001004",
        "max": 0,
        "min": 0,
        "name": "_model",
        "pattern": "",
        "presentable": false,
        "primaryKey": false,
        "required": false,
        "system": false,
        "type": "text"
      }
    ],
    "id": "pbc_4520000001",
    "indexes": [],
    "listRule": null,
    "name": "_conversations",
    "system": false,
    "type": "base",
    "updateRule": null,
    "viewRule": null
  });

  app.save(collection);
}, (app) => {
  const convColl = app.findCollectionByNameOrId("_conversations")

  const all = app.findRecordsByFilter("_conversations", "", "", 0, 0)
  for (const r of all) {
    app.delete(r)
  }

  app.delete(convColl)
})
