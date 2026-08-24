package model

import "time"

// AuditEntry 为一条审计记录（不可变，只追加）。
type AuditEntry struct {
	ID        int64     `json:"id"`
	Entity    string    `json:"entity"`
	EntityID  int64     `json:"entity_id"`
	Action    string    `json:"action"`
	Actor     string    `json:"actor"`
	Detail    string    `json:"detail"`
	CreatedAt time.Time `json:"created_at"`
}
