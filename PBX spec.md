# PBX spec
## Theme Colors
- default
- primary
- susćess
- warning
- danger
Create light and dark pallete variant!

## Pages/endpoints
Aplikace bude mít tyto stránky, resp. endpointy
### Login
GET /login

### Dashboard
GET /app

### Tabulator
GET /tabulator/{collectionName}
Let enhance how the field types are displayed in table (PB field type: how to display on tabulator page) ->
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
Please implement.    

### Form
GET /tabulator/{collectionName}/{recordId}

Enhance /form view mode in the way, how the PB field types are displayed:
	text: text
    number: decimal 
    bool: icon for True/False or empty, if not set
    email: text, put “email icon" in front of field label.
    URL: link, last part of URL as link text. Put "url icon" in front of field label, to indicate field type.
    editor: as is now (field content as formatted html)
    file: file name, if file uploaded, otherwise empty. Put “file icon" in front of field label.
    select: selected option as text. Put “selection" icon" in front of field label.
    relation: button, its label show numer of records in relation. When user click on button, modal tabular view appears, displaying related 					  collection. Put “relation icon" in front of field label.
    date: formatted date as text
    autodate: same as date
    JSON: as formatted html, the json keys display bold
    GeoPoint: GPS coordinates as text. Put “position icon" in front of field label.

Implement filter button functionality in /tabular page.
Filter definition: {condition1} {chain} {condition2}..etc. Exapmle “(Price > 10) && (Price < 100)”. Filter definition is stored in field “filter” of _tabulator collection.
Fileter definition syntax: 
	condition operators: < (less then),> (bigger then),<= (less or equal then), => (equal or bigger then), != (not equal), ~ (contain)
    can be applied to string type fields: =, !=, ~
    number, datetime fields: only <,>,<=, =>, !=
    selection type of field is treated as string, rich editor as well. Other types (relation, GPS, JSON) cannot be used in filters.
    if “?” is used asi value in condition, modal dialog is diplayed to allow user enter the values for filter. Dialog contain also 2 buttons - “Set 	filter” (= switch filter ON) and “Cancel filter” (switch filter off).
    If there are not inputs (means no “?” in definition), filter is applied automatically when Filter button is pressed. 
    
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

## package external_db
Connect external SQL database, allow to access data online or synchronize with Pockbase collection. Package will support Microsoft SQL, MySQL/MariaDB and Postgres.
1) online mode
View and edit data in SQL database table. As UI the /tabular and /form pages are used. When commit in /form, the record is immediatelly saved to SQL database. Use contex for handling timeout, lost connection etc.
2) synchronization
Synchronize PB collection and SQL database table. The collection must have same 