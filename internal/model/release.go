package model

import "time"

// ReleasePermit 为观测成果发布许可（双人复核）。
type ReleasePermit struct {
	ID          int64      `json:"id"`
	ArchiveID   int64      `json:"archive_id"`
	Title       string     `json:"title"`
	Status      string     `json:"status"`
	SubmittedBy string     `json:"submitted_by"`
	ReviewedBy  string     `json:"reviewed_by,omitempty"`
	SubmittedAt time.Time  `json:"submitted_at"`
	ReviewedAt  *time.Time `json:"reviewed_at,omitempty"`
	PublishedAt *time.Time `json:"published_at,omitempty"`
	ExpiresAt   time.Time  `json:"expires_at"`
	Version     int64      `json:"version"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}
