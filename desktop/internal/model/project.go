package model

type Project struct {
	ID                 string   `json:"id"`
	Name               string   `json:"name"`
	Enabled            *bool    `json:"enabled"`
	LocalPath          string   `json:"local_path"` // Legacy first directory, kept for older configurations.
	LocalPaths         []string `json:"local_paths"`
	DailyReportEnabled *bool    `json:"daily_report_enabled"`
	DocumentKeys       []string `json:"document_keys"`
}
