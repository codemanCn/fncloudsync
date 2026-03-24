package api

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/xiaoxuesen/fn-cloudsync/internal/api/handlers"
	"github.com/xiaoxuesen/fn-cloudsync/internal/domain"
)

type connectionCreator interface {
	Create(context.Context, domain.Connection, string) (domain.Connection, error)
	List(context.Context) ([]domain.Connection, error)
	GetByID(context.Context, string) (domain.Connection, error)
	Update(context.Context, domain.Connection, string) (domain.Connection, error)
	Delete(context.Context, string) error
}

type taskCreator interface {
	Create(context.Context, domain.Task) (domain.Task, error)
	List(context.Context) ([]domain.Task, error)
	GetByID(context.Context, string) (domain.Task, error)
	Update(context.Context, domain.Task) (domain.Task, error)
	Delete(context.Context, string) error
}

func NewRouter(connectionService connectionCreator, taskService taskCreator) http.Handler {
	router := chi.NewRouter()

	router.Get("/healthz", handlers.Health)

	router.Route("/api/v1", func(r chi.Router) {
		if connectionService != nil {
			r.Post("/connections", handlers.CreateConnection(connectionService))
			r.Get("/connections", handlers.ListConnections(connectionService))
			r.Get("/connections/{connectionID}", handlers.GetConnection(connectionService))
			r.Put("/connections/{connectionID}", handlers.UpdateConnection(connectionService))
			r.Delete("/connections/{connectionID}", handlers.DeleteConnection(connectionService))
		}
		if taskService != nil {
			r.Post("/tasks", handlers.CreateTask(taskService))
			r.Get("/tasks", handlers.ListTasks(taskService))
			r.Get("/tasks/{taskID}", handlers.GetTask(taskService))
			r.Put("/tasks/{taskID}", handlers.UpdateTask(taskService))
			r.Delete("/tasks/{taskID}", handlers.DeleteTask(taskService))
		}
	})

	return router
}
