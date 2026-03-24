package domain

import (
	"errors"
	"strings"
	"time"
)

type TaskDirection string
type TaskStatus string

const (
	TaskDirectionUpload        TaskDirection = "upload"
	TaskDirectionDownload      TaskDirection = "download"
	TaskDirectionBidirectional TaskDirection = "bidirectional"
)

const (
	TaskStatusCreated TaskStatus = "created"
	TaskStatusPaused  TaskStatus = "paused"
	TaskStatusRunning TaskStatus = "running"
	TaskStatusFailed  TaskStatus = "failed"
	TaskStatusStopped TaskStatus = "stopped"
)

type Task struct {
	ID                 string
	Name               string
	ConnectionID       string
	LocalPath          string
	RemotePath         string
	Direction          TaskDirection
	PollIntervalSec    int
	ConflictPolicy     string
	DeletePolicy       string
	EmptyDirPolicy     string
	BandwidthLimitKbps int
	MaxWorkers         int
	EncryptionEnabled  bool
	HashMode           string
	Status             TaskStatus
	DesiredState       string
	LastError          string
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

func (t *Task) ApplyDefaults() {
	if t.Status == "" {
		t.Status = TaskStatusCreated
	}
}

func (t Task) Validate() error {
	if strings.TrimSpace(t.Name) == "" {
		return errors.Join(ErrInvalidArgument, errors.New("name is required"))
	}
	if strings.TrimSpace(t.ConnectionID) == "" {
		return errors.Join(ErrInvalidArgument, errors.New("connection_id is required"))
	}
	if strings.TrimSpace(t.LocalPath) == "" {
		return errors.Join(ErrInvalidArgument, errors.New("local_path is required"))
	}
	if strings.TrimSpace(t.RemotePath) == "" {
		return errors.Join(ErrInvalidArgument, errors.New("remote_path is required"))
	}

	switch t.Direction {
	case TaskDirectionUpload, TaskDirectionDownload, TaskDirectionBidirectional:
		return nil
	default:
		return errors.Join(ErrInvalidArgument, errors.New("direction is invalid"))
	}
}
