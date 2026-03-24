package handlers_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/xiaoxuesen/fn-cloudsync/internal/api"
)

func TestHealthHandlerReturnsOK(t *testing.T) {
	t.Parallel()

	router := api.NewRouter(nil, nil)
	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, request)

	if got, want := recorder.Code, http.StatusOK; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
}
