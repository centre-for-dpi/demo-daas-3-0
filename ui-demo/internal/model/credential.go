package model

type Schema struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Version   string `json:"version"`
	Standard  string `json:"standard"`
	Status    string `json:"status"`
	FieldCount int   `json:"field_count"`
	CreatedAt string `json:"created_at"`
}

type Credential struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Schema   string `json:"schema"`
	Holder   string `json:"holder"`
	Status   string `json:"status"`
	IssuedAt string `json:"issued_at"`
	Issuer   string `json:"issuer"`
}

type Template struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Schema string `json:"schema"`
	Status string `json:"status"`
}
