package views

// LangData carries the active UI language for a rendered page. It is embedded
// in every page data struct so templates can call {{t .Lang "key"}} and the
// client-side catalog URL keeps working across pages.
type LangData struct {
	Lang string
}

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

// FilterCondition is a single field/operator/value clause of a saved filter.
// Value may be literally "?" to denote a user-supplied parameter placeholder.
type FilterCondition struct {
	Field string `json:"field"`
	Op    string `json:"op"`
	Value string `json:"value"`
}

// FilterDef is the JSON filter definition stored in a _filters record. Chains
// holds one AND/OR connector per gap between consecutive conditions
// (len(chains) == len(conditions)-1), evaluated left-to-right.
type FilterDef struct {
	Name       string            `json:"name"`
	Conditions []FilterCondition `json:"conditions"`
	Chains     []string          `json:"chains"`
}

// SavedFilter is a _filters record as returned by the list API.
type SavedFilter struct {
	ID   string    `json:"id"`
	Name string    `json:"name"`
	User string    `json:"user"`
	Def  FilterDef `json:"def"`
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
	LangData
	Theme            string
	BasePath         string
	ConfigName       string
	CollectionName   string
	TotalRecords     int
	Fields           []string
	FieldTypes       []string
	ColumnHeaders    []string
	FieldsJSON       string
	FieldTypesJSON   string
	HeadersJSON      string
	RecordsJSON      string
	FieldOptionsJSON string
	PerPage          int
	Page             int
	TotalPages       int
	Config           TabulatorConfig
	Mssql            *MssqlConfig
	SetupLinks       bool // render actions to /pbx-setup/record/... editors
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
	LangData
	Theme          string
	BasePath       string
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
	LangData
	Theme    string
	BasePath string
	Name     string
	Error    string
	Groups   []AppGroup
}

type PbxSetupPageData struct {
	LangData
	Theme    string
	MssqlDSN string
	Agent    AgentConfig
	Sections []TabulatorPageData
	Rules    []SetupCollectionRules
	Users    []SetupUser
}

// SetupUser is a user record shown in the rules editor's user checkbox list.
type SetupUser struct {
	ID    string
	Label string
}

// RuleMode is the friendly mode of a collection API rule (5 values).
type RuleMode string

const (
	RuleModePublic   RuleMode = "public"   // "" (everyone)
	RuleModeSignedIn RuleMode = "signedin" // @request.auth.id != ''
	RuleModeSelected RuleMode = "selected" // OR-chain of selected user ids
	RuleModeSuper    RuleMode = "super"    // nil (superusers only)
	RuleModeCustom   RuleMode = "custom"   // raw filter expression
)

// SetupRule holds the UI state for one of the 5 API rules of a collection.
type SetupRule struct {
	Mode   RuleMode
	Users  []string // selected user ids (for RuleModeSelected)
	Custom string   // raw filter (for RuleModeCustom)
}

// SetupRuleItem pairs a rule type with its editor state.
type SetupRuleItem struct {
	Type string
	Rule SetupRule
}

// SetupCollectionRules is the rules-editor row for one data collection.
type SetupCollectionRules struct {
	Collection string
	Items      []SetupRuleItem // list, view, create, update, delete
}

// AgentConfig is the JSON _config field of an _agent record. It holds the LLM
// provider settings used by the built-in AI agent.
type AgentConfig struct {
	Provider       string `json:"provider"`       // "openrouter" | "lmstudio"
	BaseURL        string `json:"baseURL"`        // OpenAI-compatible API base URL
	APIKey         string `json:"apiKey"`         // API key (optional for LM Studio)
	Model          string `json:"model"`          // model identifier
	TimeoutSeconds int    `json:"timeoutSeconds"` // per-request timeout
	Enabled        bool   `json:"enabled"`        // whether the agent is active
}

// AgentPageData backs the /ai agent chat page.
type AgentPageData struct {
	LangData
	Theme    string
	BasePath string
	Name     string
	Config   AgentConfig
	IsSuper  bool
	Status   string
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
	LangData
	Theme       string
	ListConfigs []ConfigEntry
	FormConfigs []ConfigEntry
}

// ConfigEditorPageData backs the /pbx-config/view/new|{name} editor form for a
// unified _views record (holds list + form config in one record).
type ConfigEditorPageData struct {
	LangData
	Theme         string
	Name          string
	CollName      string
	TabulatorJSON string
	FormJSON      string
	MssqlJSON     string
	Collections   []string
	IsNew         bool
}

// FieldOpt is a single option of a structured JSON field (checkbox option or
// dynamic row) in the schema-driven setup record editor.
type FieldOpt struct {
	Index   int    // 1-based index (absolute field index for fieldMulti/fieldLabels)
	Name    string // field name (or DB column for mapping rows)
	Label   string // display label / title
	Checked bool
	Value   string // value when present (e.g. per-field label for fieldLabels)
}

// JsonFormField is a single structured input in a schema-driven JSON editor.
// Type is one of: text, number, bool, select, fieldMulti, fieldLabels, mapping, columns.
type JsonFormField struct {
	Key          string
	Label        string
	Type         string
	Value        string
	Checked      bool
	Options      []string
	FieldOptions []FieldOpt
}

// JsonFormSection groups the structured inputs for one JSON record field
// (e.g. _tabulator, _form, _mssql, _config). TargetColl is the collection the
// field-index options refer to (the record's _collName for _views records);
// TargetCollFields holds that collection's non-system field names.
type JsonFormSection struct {
	Key              string
	Title            string
	TargetColl       string
	TargetCollFields []string
	Fields           []JsonFormField
	Raw              string // raw JSON fallback textarea content
}

// SetupRecordPageData backs the /pbx-setup/record/{coll}/... editors.
type SetupRecordPageData struct {
	LangData
	Theme        string
	CollName     string
	RecordID     string
	IsNew        bool
	Title        string
	Collections  []string
	Fields       []FormFieldItem
	JsonSections []JsonFormSection
	Error        string
}

// WizardColumn is one editable column in the import wizard preview step.
type WizardColumn struct {
	Header  string
	Field   string
	Type    string
	Include bool
	Values  string
}

// ImportWizardPageData backs the /pbx-config/import-* wizard.
type ImportWizardPageData struct {
	LangData
	Theme     string
	Source    string // "excel" | "mssql"
	Step      int    // 1 = source, 2 = preview, 3 = done
	Name      string
	FileName  string
	Sheet     string
	DSN       string
	Table     string
	Import    bool
	Columns   []WizardColumn
	Message   string
	Created   string // created collection name (step 3)
	CreatedID string
}
