# PBX spec
## Pages/endpoints
Aplikace bude mít tyto stránky, resp. endpointy
### Login
GET /login

### Dashboard
GET /dashboard

### Tabulator
GET /tabulator/{collectionName}
how the field types are displayed in table (PB field type: how to display on tabulator page) ->
	text: text
    number: decimal 
    bool: icon for True/False or empty, if not set
    email: text
    URL: link, last part of URL as link text
    editor: first 20 chars as plain text (html elements excluded) + “..."
    file: icon, if file uploaded, otherwise empty
    select: selected option as text
    relation: button, its label show numer of records in relation. When user click on button, modal tabular view appears, displaying related 					  collection.
    date: formatted date as text
    autodate: same as date
    JSON: icon, if not empty
    GeoPoint: icon, if not empty
    

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

## package Excel
Create package pbexcel with functions for export collection data to Excel sheet and import from Excel sheet to collection. Use "github.com/360EntSecGroup-Skylar/excelize/v2" for working with Excel file.
Functions:
    func ExportToExcel(excelFileName, sheetName, collectionName)
    func ImportFromExcel(excelFileName, sheetName, collectionName, mode)

ExpoetToExcel:
    - if excelFileName not exist, create. If exist, owerwrite.
    - if sheetName not exist in excel file, create. If exist, owerwrite.
   

ImportFromExcel
    - if excelFileName not found, cancel import, return err
    - if excelFileName not found in excel file, cancel import, return err
    - if collectionName does not exist in Pbx database, cancel import, raise err
    - first row of sheet, must contain titles, data start from row 2
    - sheet columns are asigned to collection fields by name (sheet column ProdCode do collection field ProdCode). If sheet column  is not found in collection, then is not imported. If value from sheet cannot be coverted to collection field value (for example value in sheet is "abc" and collection field type is numeric), put there NULL value. If rule "unique" is set for the field and it exist in the collection data, owerwrite the record.

For both functions:
     - if exelFileName does not contain path, assume /exp-imp in web root folder. If extension omitted, asume ".xlsx"
     - if sheetName is empty, assume "Sheet 1"
     - always create "log" sheet and print export result the ("10 lines exported from collection <example>)

