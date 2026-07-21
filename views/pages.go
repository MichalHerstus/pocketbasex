package views

type TopbarData struct {
	BaseURL       string
	ShowSearchBar bool
}

type TabulatorConfig struct {
	CollectionDescr  string
	ColumnTitles     string
	ColumnSorting    bool
	ColumnOrder      string
	Pagination       bool
	SearchBox        bool
	DisplaySystemCol bool
	PageTitle        string
	Filter           string
}

type TabulatorPageData struct {
	CollectionName string
	TotalRecords   int
	Fields         []string
	FieldTypes     []string
	ColumnHeaders  []string
	FieldsJSON     string
	FieldTypesJSON string
	HeadersJSON    string
	RecordsJSON    string
	PerPage        int
	Page           int
	TotalPages     int
	Config         TabulatorConfig
}

type FormFieldItem struct {
	Name     string
	Label    string
	Type     string
	Value    string
	IsSystem bool
	Data     map[string]any
}

type FormColumn struct {
	Fields []FormFieldItem
}

type FormRow struct {
	Columns []FormColumn
}

type FormConfig struct {
	FormTitle        string
	DisplaySystemCol bool
	FormLayout       string
	FormLabels       string
}

type FormPageData struct {
	CollectionName string
	ID             string
	Title          string
	Description    string
	SystemFields   []FormFieldItem
	Rows           []FormRow
	HasConfig      bool
	ViewOnly       bool
}

type AppLink struct {
	Collection string
	Label      string
}

type AppGroup struct {
	GroupLabel string
	GroupIcon  string
	Links      []AppLink
}

type AppPageData struct {
	Name   string
	Error  string
	Groups []AppGroup
}

type PbxSetupPageData struct {
	Sections []TabulatorPageData
}
