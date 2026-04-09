package model

type Notification struct {
	ID     string `json:"id"`
	Type   string `json:"type"`
	Icon   string `json:"icon"`
	Text   string `json:"text"`
	Time   string `json:"time"`
	Unread bool   `json:"unread"`
	Action string `json:"action"`
}
