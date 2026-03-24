package watcher

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/xiaoxuesen/fn-cloudsync/internal/domain"
)

type taskRunner interface {
	List(context.Context) ([]domain.Task, error)
	ExecuteRunningTask(context.Context, string) error
}

type Backend interface {
	Add(string) error
	Remove(string) error
	Close() error
	Events() <-chan string
	Errors() <-chan error
}

type Watcher struct {
	tasks          taskRunner
	refreshInterval time.Duration
	debounce        time.Duration
	newBackend      func() (Backend, error)
	watched         map[string]string
	roots           map[string]domain.Task
	lastTriggered   map[string]time.Time
}

func New(tasks taskRunner, refreshInterval, debounce time.Duration) *Watcher {
	return &Watcher{
		tasks:           tasks,
		refreshInterval: refreshInterval,
		debounce:        debounce,
		newBackend:      newFSNotifyBackend,
		watched:         make(map[string]string),
		roots:           make(map[string]domain.Task),
		lastTriggered:   make(map[string]time.Time),
	}
}

func (w *Watcher) SetBackendFactory(factory func() (Backend, error)) {
	if factory != nil {
		w.newBackend = factory
	}
}

func (w *Watcher) Run(ctx context.Context) {
	backend, err := w.newBackend()
	if err != nil {
		return
	}
	defer backend.Close()

	w.reconcile(ctx, backend)

	ticker := time.NewTicker(w.normalizedRefreshInterval())
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case path := <-backend.Events():
			w.handleEvent(ctx, path)
		case <-backend.Errors():
		case <-ticker.C:
			w.reconcile(ctx, backend)
		}
	}
}

func (w *Watcher) reconcile(ctx context.Context, backend Backend) {
	tasks, err := w.tasks.List(ctx)
	if err != nil {
		return
	}

	active := make(map[string]domain.Task)
	for _, task := range tasks {
		if !shouldWatch(task) {
			continue
		}
		root := filepath.Clean(task.LocalPath)
		active[root] = task
		for _, dir := range listWatchDirs(root) {
			if _, ok := w.watched[dir]; ok {
				continue
			}
			if err := backend.Add(dir); err != nil {
				continue
			}
			w.watched[dir] = task.ID
		}
	}

	for watchedPath := range w.watched {
		if task, ok := taskForWatchedPath(active, watchedPath); ok && pathWithinRoot(watchedPath, filepath.Clean(task.LocalPath)) {
			continue
		}
		_ = backend.Remove(watchedPath)
		delete(w.watched, watchedPath)
	}

	w.roots = active
}

func (w *Watcher) handleEvent(ctx context.Context, path string) {
	cleanPath := filepath.Clean(path)
	now := time.Now().UTC()

	for root, task := range w.roots {
		if !pathWithinRoot(cleanPath, root) {
			continue
		}
		if !w.shouldTrigger(task.ID, now) {
			continue
		}
		w.lastTriggered[task.ID] = now
		_ = w.tasks.ExecuteRunningTask(ctx, task.ID)
	}
}

func (w *Watcher) shouldTrigger(taskID string, now time.Time) bool {
	if w.debounce <= 0 {
		return true
	}
	last, ok := w.lastTriggered[taskID]
	if !ok {
		return true
	}
	return now.Sub(last) >= w.debounce
}

func (w *Watcher) normalizedRefreshInterval() time.Duration {
	if w.refreshInterval <= 0 {
		return time.Second
	}
	return w.refreshInterval
}

func shouldWatch(task domain.Task) bool {
	if task.Status != domain.TaskStatusRunning {
		return false
	}
	return task.Direction == domain.TaskDirectionUpload || task.Direction == domain.TaskDirectionBidirectional
}

func pathWithinRoot(path, root string) bool {
	if path == root {
		return true
	}
	root = strings.TrimRight(root, string(filepath.Separator)) + string(filepath.Separator)
	return strings.HasPrefix(path, root)
}

func listWatchDirs(root string) []string {
	var dirs []string
	_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if entry.IsDir() {
			dirs = append(dirs, filepath.Clean(path))
		}
		return nil
	})
	if len(dirs) == 0 {
		dirs = append(dirs, filepath.Clean(root))
	}
	return dirs
}

func taskForWatchedPath(tasks map[string]domain.Task, watchedPath string) (domain.Task, bool) {
	for root, task := range tasks {
		if pathWithinRoot(watchedPath, root) {
			return task, true
		}
	}
	return domain.Task{}, false
}

type fsnotifyBackend struct {
	watcher *fsnotify.Watcher
	events  chan string
	errors  chan error
}

func newFSNotifyBackend() (Backend, error) {
	base, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	backend := &fsnotifyBackend{
		watcher: base,
		events:  make(chan string, 64),
		errors:  make(chan error, 8),
	}

	go func() {
		defer close(backend.events)
		defer close(backend.errors)
		for {
			select {
			case event, ok := <-base.Events:
				if !ok {
					return
				}
				backend.events <- event.Name
			case err, ok := <-base.Errors:
				if !ok {
					return
				}
				backend.errors <- err
			}
		}
	}()

	return backend, nil
}

func (b *fsnotifyBackend) Add(path string) error {
	return b.watcher.Add(path)
}

func (b *fsnotifyBackend) Remove(path string) error {
	return b.watcher.Remove(path)
}

func (b *fsnotifyBackend) Close() error {
	return b.watcher.Close()
}

func (b *fsnotifyBackend) Events() <-chan string {
	return b.events
}

func (b *fsnotifyBackend) Errors() <-chan error {
	return b.errors
}
