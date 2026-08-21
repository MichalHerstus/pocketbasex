/// <reference path="../pb_data/types.d.ts" />
// Seed two example custom actions for the "produkty" collection (Phase 10).
//   1. "Report active products"  — list view (_onList) read-only report
//   2. "Product detail"          — form view (_onForm) logs the current record
// Both are _public so non-superusers can run them. Edit/delete via /pbx-setup.
migrate((app) => {
  // Existing actions are left untouched; we only add if not already present.
  const ensure = (name, data) => {
    const existing = app.findRecordsByFilter("_actions", "_name = {:n}", "", 1, 0, { n: name })
    if (existing.length > 0) {
      return
    }
    const rec = new Record(app.findCollectionByNameOrId("_actions"))
    rec.set("_name", name)
    for (const [k, v] of Object.entries(data)) {
      rec.set(k, v)
    }
    app.save(rec)
  }

  ensure("Report active products", {
    "_description": "List view: logs the product code and short name of every active product.",
    "_script": `var rows = select("produkty", "active = true", "", 100);
log("Active products: " + rows.length);
rows.forEach(function (r) {
    log(r.productCode + " — " + r.prodShortName);
});`,
    "_collection": "produkty",
    "_onList": true,
    "_onForm": false,
    "_public": true,
  })

  ensure("Product detail", {
    "_description": "Form view: logs the code, short name and price of the opened product.",
    "_script": `var p = currentRecord();
if (!p) {
    log("No product opened.");
} else {
    log("Code: " + p.productCode);
    log("Short name: " + p.prodShortName);
    log("Price: " + (p.prodPrice != null ? p.prodPrice : "n/a"));
}`,
    "_collection": "produkty",
    "_onList": false,
    "_onForm": true,
    "_public": true,
  })
}, (app) => {
  // Down: remove the two seeded actions (only if they are the seeded ones).
  const names = ["Report active products", "Product detail"]
  for (const n of names) {
    const existing = app.findRecordsByFilter("_actions", "_name = {:n}", "", 1, 0, { n: n })
    for (const r of existing) {
      app.delete(r)
    }
  }
})