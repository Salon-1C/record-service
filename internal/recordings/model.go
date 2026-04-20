package recordings

import "time"

type RecordingStatus string

const (
	StatusReady   RecordingStatus = "ready"
	StatusFailed  RecordingStatus = "failed"
	StatusPending RecordingStatus = "pending"
)

type Recording struct {
	ID             string          `gorm:"primaryKey;size:64" json:"id"`
	StreamKey      string          `gorm:"index;size:128;not null" json:"streamKey"`
	Title          string          `gorm:"size:255;not null" json:"title"`
	Description    string          `gorm:"type:text" json:"description"`
	InstructorName string          `gorm:"size:255;not null" json:"instructorName"`
	StartedAt      time.Time       `gorm:"not null" json:"startedAt"`
	EndedAt        time.Time       `gorm:"not null" json:"endedAt"`
	DurationSec    int64           `gorm:"not null" json:"durationSec"`
	ObjectKey      string          `gorm:"size:512;not null;uniqueIndex" json:"objectKey"`
	PlaybackURL    string          `gorm:"size:1024;not null" json:"playbackUrl"`
	Status         RecordingStatus `gorm:"size:32;not null;index" json:"status"`
	CreatedAt      time.Time       `gorm:"autoCreateTime" json:"createdAt"`
}

type StreamMetadata struct {
	StreamKey      string    `gorm:"primaryKey;size:128" json:"streamKey"`
	Title          string    `gorm:"size:255;not null" json:"title"`
	Description    string    `gorm:"type:text" json:"description"`
	InstructorName string    `gorm:"size:255;not null" json:"instructorName"`
	CreatedAt      time.Time `gorm:"autoCreateTime" json:"createdAt"`
	UpdatedAt      time.Time `gorm:"autoUpdateTime" json:"updatedAt"`
}

type ListResponse struct {
	Recordings []Recording `json:"recordings"`
	Count      int         `json:"count"`
}
