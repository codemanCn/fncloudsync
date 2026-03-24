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

type TaskRuntimeState struct {
	TaskID           string
	Phase            string
	LastLocalScanAt  time.Time
	LastRemoteScanAt time.Time
	LastReconcileAt  time.Time
	LastSuccessAt    time.Time
	BackoffUntil     time.Time
	RetryStreak      int
	LastError        string
	UpdatedAt        time.Time
}

type OperationQueueItem struct {
	ID            string
	TaskID        string
	OpType        string
	TargetPath    string
	SrcSide       string
	Reason        string
	PayloadJSON   string
	Priority      int
	Status        string
	AttemptCount  int
	NextAttemptAt time.Time
	LastError     string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type FailureRecord struct {
	ID            string
	TaskID        string
	Path          string
	OpType        string
	ErrorCode     string
	ErrorMessage  string
	Retryable     bool
	FirstFailedAt time.Time
	LastFailedAt  time.Time
	AttemptCount  int
	ResolvedAt    time.Time
}

type FileIndexEntry struct {
	ID                string
	TaskID            string
	RelativePath      string
	EntryType         string
	LocalExists       bool
	RemoteExists      bool
	LocalSize         int64
	RemoteSize        int64
	LocalMTime        time.Time
	RemoteMTime       time.Time
	LocalFileID       string
	RemoteETag        string
	ContentHash       string
	LastSyncDirection string
	LastSyncAt        time.Time
	Version           int
	SyncState         string
	ConflictFlag      bool
	DeletedTombstone  bool
}

type SyncActionType string

const (
	SyncActionCreateDirLocal   SyncActionType = "CreateDirLocal"
	SyncActionCreateDirRemote  SyncActionType = "CreateDirRemote"
	SyncActionUploadFile       SyncActionType = "UploadFile"
	SyncActionDownloadFile     SyncActionType = "DownloadFile"
	SyncActionDeleteLocal      SyncActionType = "DeleteLocal"
	SyncActionDeleteRemote     SyncActionType = "DeleteRemote"
	SyncActionMoveConflictLocal  SyncActionType = "MoveConflictLocal"
	SyncActionMoveConflictRemote SyncActionType = "MoveConflictRemote"
	SyncActionRefreshMetadata  SyncActionType = "RefreshMetadata"
)

type SyncAction struct {
	Type         SyncActionType
	RelativePath string
	LocalPath    string
	RemotePath   string
	ConflictPath string
	IsDir        bool
}

type TaskRuntimeView struct {
	Task          Task
	Runtime       TaskRuntimeState
	Queue         []OperationQueueItem
	Failures      []FailureRecord
	QueueSummary  TaskQueueSummary
	FailureSummary TaskFailureSummary
}

type TaskQueueSummary struct {
	Total      int
	Pending    int
	Executing  int
	RetryWait  int
}

type TaskFailureSummary struct {
	Total     int
	Resolved  int
	Open      int
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
