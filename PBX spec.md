# PBX spec
## Pages/endpoints
Aplikace bude mít tyto stránky, resp. endpointy
### Login
GET /login

### Dashboard
GET /dashboard

### Tabulator
GET /tabulator/{collectionName}

### Form
GET /tabulator/{collectionName}/{recordId}

### Actions
GET /pbxAction/{actionName}?{parameters} … built in Go, inside Pbx
GET /jsAction/{actionName}?{parameters} … in js, soubory *.pb.js v ./pb_hooks

Akce mohou být přiřazeny buttons, později možná i dalším prvkům a událostem.
Např. actions: validateForm, save, delete, export, import, print… Inspirace v kalipso.

## Commands

### Export
./pbx export {collectionName} {outputDir} —format CSV/JSON/XML/XLSX —cp 1252
{collectionName} = “all” exportuje všechny collections v db

### Import
./pbx import {collectionName} {inputDir} append/update

### Sync
./pbx sync {collectionName} {tableName} —db <connectionString> 