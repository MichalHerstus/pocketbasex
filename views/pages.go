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

// MssqlConfig is the JSON _mssql field of a view config record.
type MssqlConfig struct {
	DSN     string         `json:"dsn"`
	Table   string         `json:"table"`
	Mode    string         `json:"mode"`
	Mapping []MssqlMapping `json:"mapping"`
}

// ViewTabulatorConfig is the JSON stored in a _views record's _tabulator field.
// It holds all settings previously spread across _tabulator collection scalar
// fields plus the list JSON config.
type ViewTabulatorConfig struct {
	PageTitle        string       `json:"pageTitle"`
	CollectionDescr  string       `json:"collectionDescr"`
	ColumnTitles     string       `json:"columnTitles"`
	ColumnOrder      string       `json:"columnOrder"`
	ColumnSorting    bool         `json:"columnSorting"`
	SearchBox        bool         `json:"searchBox"`
	Pagination       bool         `json:"pagination"`
	DisplaySystemCol bool         `json:"displaySystemCol"`
	Filter           string       `json:"filter"`
	Columns          []ListColumn `json:"columns,omitempty"`
}

// ViewFormConfig is the JSON stored in a _views record's _form field. It holds
// all settings previously in the _form collection scalar fields plus the form
// JSON config (layout/labels/collections).
type ViewFormConfig struct {
	FormTitle        string              `json:"formTitle"`
	FormDescr        string              `json:"formDescr"`
	FormLabels       string              `json:"formLabels"`
	FormLayout       string              `json:"formLayout"`
	ColumnOrder      string              `json:"columnOrder"`
	DisplaySystemCol bool                `json:"displaySystemCol"`
	Layout           [][][]int           `json:"layout,omitempty"`
	Labels           map[string]string   `json:"labels,omitempty"`
	Collections      []FormCollectionRef `json:"collections,omitempty"`
}

type TabulatorPageData struct {
	Theme          string
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
	Mssql          *MssqlConfig
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

// FormSection groups rows under a base collection name for view editing.
type FormSection struct {
	CollectionName string
	Rows           []FormRow
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
	Theme          string
	ConfigName     string
	CollectionName string
	ID             string
	Title          string
	Description    string
	SystemFields   []FormFieldItem
	Rows           []FormRow
	Sections       []FormSection
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
	Theme  string
	Name   string
	Error  string
	Groups []AppGroup
}

type PbxSetupPageData struct {
	Theme    string
	MssqlDSN string
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
	Theme       string
	ListConfigs []ConfigEntry
	FormConfigs []ConfigEntry
}

// ConfigEditorPageData backs the /pbx-config/view/new|{name} editor form for a
// unified _views record (holds list + form config in one record).
type ConfigEditorPageData struct {
	Theme          string
	Name           string
	CollName       string
	TabulatorJSON  string
	FormJSON       string
	MssqlJSON      string
	Collections    []string
	IsNew          bool
}

// WizardColumn is one editable column in the import wizard preview step.
type WizardColumn struct {
	Header   string
	Field    string
	Type     string
	Include  bool
	Values   string
}

// ImportWizardPageData backs the /pbx-config/import-* wizard.
type ImportWizardPageData struct {
	Theme    string
	Source   string // "excel" | "mssql"
	Step     int    // 1 = source, 2 = preview, 3 = done
	Name     string
	FileName string
	Sheet    string
	DSN      string
	Table    string
	Import   bool
	Columns  []WizardColumn
	Message  string
	Created  string // created collection name (step 3)
	CreatedID string
}
