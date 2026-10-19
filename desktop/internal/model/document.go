package model

type Picture struct {
	Data     []byte            `json:"-"`
	Variants map[string][]byte `json:"-"`
	Row      int               `json:"row"`
	Column   int               `json:"column"`
	Width    float64           `json:"width"`
	Height   float64           `json:"height"`
	File     string            `json:"file"`
}

type TaskRow struct {
	RecordID       string      `json:"record_id,omitempty"`
	StatusEdit     *StatusEdit `json:"status_edit,omitempty"`
	StatusSyncedAt string      `json:"status_synced_at,omitempty"`
	Number         int         `json:"number"`
	Values         []any       `json:"values"`
	Pictures       []Picture   `json:"pictures"`
}

type TaskSheet struct {
	StatusOptions []string  `json:"status_options,omitempty"`
	Name          string    `json:"name"`
	HeaderRow     int       `json:"header_row"`
	Headers       []any     `json:"headers"`
	Scanned       int       `json:"scanned"`
	Rows          []TaskRow `json:"rows"`
}

type Document struct {
	Key                   string                  `json:"key"`
	Version               int                     `json:"version"`
	SourceURL             string                  `json:"source_url"`
	SourcePath            string                  `json:"source_path"`
	Owner                 string                  `json:"owner"`
	Title                 string                  `json:"title"`
	UpdatedAt             string                  `json:"updated_at"`
	Warnings              []string                `json:"warnings"`
	Sheets                []TaskSheet             `json:"sheets"`
	Cached                bool                    `json:"cached"`
	IncludeCompleted      bool                    `json:"include_completed"`
	PullConditions        *DocumentPullConditions `json:"pull_conditions,omitempty"`
	AppliedPullConditions *DocumentPullConditions `json:"applied_pull_conditions,omitempty"`
	SetupPending          bool                    `json:"setup_pending,omitempty"`
}

type DocumentFilter struct {
	Sheet    string `json:"sheet"`
	Column   int    `json:"column"`
	Header   string `json:"header"`
	Operator string `json:"operator"`
	Value    string `json:"value"`
}

type DocumentPullConditions struct {
	Owner            string           `json:"owner"`
	IncludeCompleted bool             `json:"include_completed"`
	Filters          []DocumentFilter `json:"filters,omitempty"`
}
