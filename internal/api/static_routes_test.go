package api_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/xiaoxuesen/fn-cloudsync/internal/api"
)

func TestStaticDocsAndAdminRoutesAreServed(t *testing.T) {
	t.Parallel()

	router := api.NewRouter(nil, nil)

	openAPIRecorder := httptest.NewRecorder()
	router.ServeHTTP(openAPIRecorder, httptest.NewRequest(http.MethodGet, "/openapi.yaml", nil))
	if got, want := openAPIRecorder.Code, http.StatusOK; got != want {
		t.Fatalf("openapi status = %d, want %d", got, want)
	}
	if contentType := openAPIRecorder.Header().Get("Content-Type"); contentType == "" {
		t.Fatal("openapi content type empty")
	}
	if !bytes.Contains(openAPIRecorder.Body.Bytes(), []byte("/api/v1/tasks/{taskID}/conflicts")) {
		t.Fatalf("openapi body missing conflicts path: %s", openAPIRecorder.Body.String())
	}

	adminRecorder := httptest.NewRecorder()
	router.ServeHTTP(adminRecorder, httptest.NewRequest(http.MethodGet, "/admin/", nil))
	if got, want := adminRecorder.Code, http.StatusOK; got != want {
		t.Fatalf("admin status = %d, want %d", got, want)
	}
	body := adminRecorder.Body.Bytes()
	for _, needle := range [][]byte{
		[]byte("<title>CloudSync 控制台</title>"),
		[]byte("class=\"sidebar\""),
		[]byte("id=\"open-connection-modal\""),
		[]byte(">总览<"),
		[]byte(">任务列表<"),
		[]byte(">计划<"),
		[]byte(">设置<"),
		[]byte(">日志<"),
		[]byte("连接信息"),
		[]byte("id=\"task-table-body\""),
		[]byte("id=\"connection-wizard-modal\""),
		[]byte("Cloud Sync 设置"),
		[]byte("id=\"manage-menu\""),
	} {
		if !bytes.Contains(body, needle) {
			t.Fatalf("admin body missing %q: %s", needle, adminRecorder.Body.String())
		}
	}
}
