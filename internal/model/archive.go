package model

import "time"

// Archive 为数据归档对象（幂等、仅软删除、不可物理删除）。
type Archive struct {
	ID             int64      `json:"id"`
	BatchID        int64      `json:"batch_id"`
	ObjectURI      string     `json:"object_uri"`
	ChecksumSHA256 string     `json:"checksum_sha256"`
	SizeBytes      int64      `json:"size_bytes"`
	Status         string     `json:"status"`
	IdempotencyKey string     `json:"idempotency_key"`
	RequestedBy    string     `json:"requested_by"`
	VerifiedBy     string     `json:"verified_by,omitempty"`
	DeletedAt      *time.Time `json:"deleted_at,omitempty"`
	Version        int64      `json:"version"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}
