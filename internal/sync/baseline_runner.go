package sync

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/xiaoxuesen/fn-cloudsync/internal/domain"
)

type remoteFS interface {
	MkdirAll(context.Context, domain.Connection, string, string) error
	Upload(context.Context, domain.Connection, string, string, io.Reader, string) error
	List(context.Context, domain.Connection, string, string) ([]domain.RemoteEntry, error)
	Download(context.Context, domain.Connection, string, string) (io.ReadCloser, domain.RemoteEntry, error)
}

type BaselineRunner struct {
	remote remoteFS
}

func NewBaselineRunner(remote remoteFS) *BaselineRunner {
	return &BaselineRunner{remote: remote}
}

func (r *BaselineRunner) RunOnce(ctx context.Context, task domain.Task, connection domain.Connection, password string) error {
	switch task.Direction {
	case domain.TaskDirectionUpload:
		return r.UploadOnce(ctx, task, connection, password)
	case domain.TaskDirectionDownload:
		return r.DownloadOnce(ctx, task, connection, password)
	case domain.TaskDirectionBidirectional:
		if err := r.UploadOnce(ctx, task, connection, password); err != nil {
			return err
		}
		return r.DownloadOnce(ctx, task, connection, password)
	default:
		return domain.ErrInvalidArgument
	}
}

func (r *BaselineRunner) UploadOnce(ctx context.Context, task domain.Task, connection domain.Connection, password string) error {
	return filepath.WalkDir(task.LocalPath, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if path == task.LocalPath {
			return nil
		}

		relPath, err := filepath.Rel(task.LocalPath, path)
		if err != nil {
			return err
		}
		remotePath := joinRemotePath(task.RemotePath, filepath.ToSlash(relPath))

		if entry.IsDir() {
			return r.remote.MkdirAll(ctx, connection, password, remotePath)
		}

		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()

		return r.remote.Upload(ctx, connection, password, remotePath, file, detectContentType(path))
	})
}

func (r *BaselineRunner) DownloadOnce(ctx context.Context, task domain.Task, connection domain.Connection, password string) error {
	return r.downloadDir(ctx, task, connection, password, task.RemotePath)
}

func (r *BaselineRunner) downloadDir(ctx context.Context, task domain.Task, connection domain.Connection, password string, remotePath string) error {
	entries, err := r.remote.List(ctx, connection, password, remotePath)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		localPath := filepath.Join(task.LocalPath, strings.TrimPrefix(strings.TrimPrefix(entry.Path, task.RemotePath), "/"))
		if entry.IsDir {
			if err := os.MkdirAll(localPath, 0o755); err != nil {
				return err
			}
			if err := r.downloadDir(ctx, task, connection, password, entry.Path); err != nil {
				return err
			}
			continue
		}

		reader, _, err := r.remote.Download(ctx, connection, password, entry.Path)
		if err != nil {
			return err
		}

		if err := os.MkdirAll(filepath.Dir(localPath), 0o755); err != nil {
			reader.Close()
			return err
		}
		file, err := os.Create(localPath)
		if err != nil {
			reader.Close()
			return err
		}
		if _, err := io.Copy(file, reader); err != nil {
			file.Close()
			reader.Close()
			return err
		}
		file.Close()
		reader.Close()
	}

	return nil
}

func joinRemotePath(base, rel string) string {
	base = strings.TrimRight(base, "/")
	rel = strings.TrimLeft(rel, "/")
	if base == "" {
		return "/" + rel
	}
	return base + "/" + rel
}

func detectContentType(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".txt", ".md":
		return "text/plain"
	default:
		return "application/octet-stream"
	}
}
