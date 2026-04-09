package model

// Filter types for list operations.

type SchemaFilter struct {
	Standard string
	Status   string
	Search   string
}

type CredentialFilter struct {
	Status string
	Issuer string
	Holder string
	Schema string
	Search string
}

type VerificationFilter struct {
	Result string // pass, fail, pending
	Search string
}

type AuditFilter struct {
	Module string
	Search string
}

type NotifFilter struct {
	Type string
}

type IssuerFilter struct {
	Status string
	Search string
}
