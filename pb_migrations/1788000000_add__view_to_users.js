/// <reference path="../pb_data/types.d.ts" />
migrate((app) => {
  const users = app.findCollectionByNameOrId("users")

  users.fields.addAt(1, new Field({
    "autogeneratePattern": "",
    "help": "Optional list configuration _name to land on after login (/tabular/{_view}); empty falls back to /app. Superusers always go to /pbx-setup.",
    "hidden": false,
    "id": "text9120000001",
    "max": 0,
    "min": 0,
    "name": "_view",
    "pattern": "",
    "presentable": false,
    "primaryKey": false,
    "required": false,
    "system": false,
    "type": "text"
  }))

  app.save(users)
}, (app) => {
  const users = app.findCollectionByNameOrId("users")
  users.fields.removeById("text9120000001")
  app.save(users)
})