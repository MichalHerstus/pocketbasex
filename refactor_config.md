Let unify the _tabulator and _form configuration collections into one, the _views collection.
Fields in _views:
_name of the configuration, for example “products” (text field)
_collName stored name of collection the view is configured for
_form is field JSON storing all form configurating which is now in _form collection fields like formTitle, formDescr, formLabels, formLayout etc. 
_tabulator is JSON field storing all form configurating which is now in _tabulator collection fields like columnTitles, columnSorting etc.
_mssql stores DSN of MSSQL db for exp/imp
Example of _tabulator JSON:
{
“pageTitle”:”Produkty”,
“collectionDesr”:"databáze produktů naší firmy”,
“columnTitles”:"Kód, Název, Cena, Aktivní”,
“columnSorting”:”2,3",
“columnOrder”:"2,3,6,7,8,9"
}
Then the endpoint for views will /tabulator/{_name}, PBX will read configuration from _tabulator field of _views collection and display the tabulator view of collection according field _collname.
Create the _views collection in PBX database and add record _name = “produkty”, _collName = “produkty”. The JSONs of _tabulator, _form fields are constructed from the values in existing _form, _tabular collections (with field _name = “produkty").
Let implement above.