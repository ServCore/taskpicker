package model

type NoteEdit struct {
	Column   int    `json:"column"`
	Header   string `json:"header"`
	Original string `json:"original"`
}

type StatusEdit struct {
	Original string     `json:"original"`
	Notes    []NoteEdit `json:"notes,omitempty"`
	SavedAt  string     `json:"saved_at"`
	Error    string     `json:"error,omitempty"`
}
