package domain

import "time"

type RemoteEntry struct {
	Path        string
	IsDir       bool
	Size        int64
	MTime       time.Time
	ETag        string
	ContentType string
	Exists      bool
}
