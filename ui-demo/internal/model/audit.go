package model

type AuditEntry struct {
	Timestamp string `json:"timestamp"`
	Module    string `json:"module"`
	Actor     string `json:"actor"`
	Action    string `json:"action"`
	Target    string `json:"target"`
	Outcome   string `json:"outcome"`
}
