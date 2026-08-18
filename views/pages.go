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

// ListColumn is a single column definition in the JSON list config (_tabulator.config).
type ListColumn struct {
	Field      string `json:"field"`
	Title      string `json:"title"`
	Sortable   bool   `json:"sortable"`
	Searchable bool   `json:"searchable"`
}

// ListConfig is the JSON config (_tabulator.config) for a list/tabulator view.
type ListConfig struct {
	Title            string       `json:"title"`
	Description      string       `json:"description"`
	DisplaySystemCol bool         `json:"displaySystemCol"`
	SearchBox        bool         `json:"searchBox"`
	Pagination       bool         `json:"pagination"`
	Filter           string       `json:"filter"`
	Columns          []ListColumn `json:"columns"`
}

// MssqlMapping maps a PocketBase field to a database table column.
type MssqlMapping struct {
	PBField string `json:"pbField"`
	DBField string `json:"dbField"`
}

// MssqlConfig is the JSON _mssql field of a list config record.
type MssqlConfig struct {
	DSN     string         `json:"dsn"`
	Table   string         `json:"table"`
	Mode    string         `json:"mode"`
	Mapping []MssqlMapping `json:"mapping"`
}

type TabulatorPageData struct {
	ConfigName     string
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
	FormDescr        string
	DisplaySystemCol bool
	FormLayout       string
	FormLabels       string
	ColumnOrder      string
}

// FormCollectionRef references a base collection edited through a view-only form.
type FormCollectionRef struct {
	Name      string `json:"name"`
	JoinField string `json:"joinField"`
}

// FormConfigJSON is the JSON config (_form.config) for a form view.
type FormConfigJSON struct {
	Title            string              `json:"title"`
	Description      string              `json:"description"`
	DisplaySystemCol bool                `json:"displaySystemCol"`
	Layout           [][][]int           `json:"layout"`
	Labels           map[string]string   `json:"labels"`
	Collections      []FormCollectionRef `json:"collections"`
}

type FormPageData struct {
	ConfigName     string
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
	URL        string
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

// ConfigEntry is a row in the /pbx-config overview listing a list or form configuration.
type ConfigEntry struct {
	Type      string
	Name      string
	CollName  string
	Title     string
	HasConfig bool
}

type PbxConfigPageData struct {
	ListConfigs []ConfigEntry
	FormConfigs []ConfigEntry
}

// ConfigEditorPageData backs the /pbx-config/{type}/new|{name} editor form.
type ConfigEditorPageData struct {
	Type        string
	TypeLabel   string
	Name        string
	CollName    string
	ConfigJSON  string
	Collections []string
	IsNew       bool
}
