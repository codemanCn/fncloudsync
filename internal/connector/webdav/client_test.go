package webdav_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/xiaoxuesen/fn-cloudsync/internal/connector/webdav"
	"github.com/xiaoxuesen/fn-cloudsync/internal/domain"
)

func TestProbeDetectsBasicCapabilities(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodOptions:
			w.Header().Set("Allow", "OPTIONS, PROPFIND, GET, PUT, DELETE, MOVE")
			w.WriteHeader(http.StatusOK)
		case "PROPFIND":
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusMultiStatus)
			_, _ = w.Write([]byte(`<?xml version="1.0" encoding="utf-8"?>
<d:multistatus xmlns:d="DAV:">
  <d:response>
    <d:href>/dav/</d:href>
    <d:propstat>
      <d:prop>
        <d:getetag>"abc"</d:getetag>
        <d:getlastmodified>Mon, 24 Mar 2026 03:00:00 GMT</d:getlastmodified>
        <d:getcontentlength>0</d:getcontentlength>
      </d:prop>
    </d:propstat>
  </d:response>
</d:multistatus>`))
		default:
			http.Error(w, "unexpected method", http.StatusMethodNotAllowed)
		}
	}))
	defer server.Close()

	client := webdav.NewClient()
	caps, err := client.Probe(context.Background(), domain.Connection{
		Endpoint: server.URL,
		Username: "alice",
		RootPath: "/dav",
	}, "")
	if err != nil {
		t.Fatalf("Probe() error = %v", err)
	}

	if !caps.SupportsMove {
		t.Fatal("SupportsMove = false, want true")
	}
	if !caps.SupportsETag {
		t.Fatal("SupportsETag = false, want true")
	}
	if !caps.SupportsLastModified {
		t.Fatal("SupportsLastModified = false, want true")
	}
	if !caps.SupportsContentLength {
		t.Fatal("SupportsContentLength = false, want true")
	}
	if got, want := caps.PathEncodingMode, "plain"; got != want {
		t.Fatalf("PathEncodingMode = %q, want %q", got, want)
	}
	if !strings.Contains(caps.ServerFingerprint, server.URL) {
		t.Fatalf("ServerFingerprint = %q, want contains %q", caps.ServerFingerprint, server.URL)
	}
}

func TestStatReturnsRemoteEntry(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "PROPFIND" {
			http.Error(w, "unexpected method", http.StatusMethodNotAllowed)
			return
		}

		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusMultiStatus)
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="utf-8"?>
<d:multistatus xmlns:d="DAV:">
  <d:response>
    <d:href>/dav/report.txt</d:href>
    <d:propstat>
      <d:prop>
        <d:resourcetype></d:resourcetype>
        <d:getcontentlength>42</d:getcontentlength>
        <d:getlastmodified>Mon, 24 Mar 2026 03:00:00 GMT</d:getlastmodified>
        <d:getetag>"etag-1"</d:getetag>
      </d:prop>
    </d:propstat>
  </d:response>
</d:multistatus>`))
	}))
	defer server.Close()

	client := webdav.NewClient()
	entry, err := client.Stat(context.Background(), domain.Connection{
		Endpoint: server.URL,
		RootPath: "/dav",
	}, "", "/report.txt")
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}

	if entry.Path != "/report.txt" {
		t.Fatalf("Path = %q, want %q", entry.Path, "/report.txt")
	}
	if entry.IsDir {
		t.Fatal("IsDir = true, want false")
	}
	if entry.Size != 42 {
		t.Fatalf("Size = %d, want 42", entry.Size)
	}
	if entry.ETag != `"etag-1"` {
		t.Fatalf("ETag = %q, want %q", entry.ETag, `"etag-1"`)
	}
	if entry.MTime.IsZero() {
		t.Fatal("MTime is zero, want parsed time")
	}
}

func TestListReturnsChildrenOnly(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "PROPFIND" {
			http.Error(w, "unexpected method", http.StatusMethodNotAllowed)
			return
		}

		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusMultiStatus)
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="utf-8"?>
<d:multistatus xmlns:d="DAV:">
  <d:response>
    <d:href>/dav/</d:href>
    <d:propstat>
      <d:prop>
        <d:resourcetype><d:collection/></d:resourcetype>
      </d:prop>
    </d:propstat>
  </d:response>
  <d:response>
    <d:href>/dav/docs/</d:href>
    <d:propstat>
      <d:prop>
        <d:resourcetype><d:collection/></d:resourcetype>
        <d:getlastmodified>Mon, 24 Mar 2026 03:00:00 GMT</d:getlastmodified>
      </d:prop>
    </d:propstat>
  </d:response>
  <d:response>
    <d:href>/dav/readme.md</d:href>
    <d:propstat>
      <d:prop>
        <d:resourcetype></d:resourcetype>
        <d:getcontentlength>7</d:getcontentlength>
        <d:getlastmodified>Mon, 24 Mar 2026 03:01:00 GMT</d:getlastmodified>
        <d:getetag>"etag-2"</d:getetag>
      </d:prop>
    </d:propstat>
  </d:response>
</d:multistatus>`))
	}))
	defer server.Close()

	client := webdav.NewClient()
	entries, err := client.List(context.Background(), domain.Connection{
		Endpoint: server.URL,
		RootPath: "/dav",
	}, "", "/")
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}

	if len(entries) != 2 {
		t.Fatalf("len(entries) = %d, want 2", len(entries))
	}
	if entries[0].Path == "/" || entries[1].Path == "/" {
		t.Fatal("List() returned root entry, want children only")
	}
	if !entries[0].MTime.Equal(time.Date(2026, 3, 24, 3, 0, 0, 0, time.UTC)) && !entries[1].MTime.Equal(time.Date(2026, 3, 24, 3, 0, 0, 0, time.UTC)) {
		t.Fatal("List() did not parse child mtime")
	}
}

func TestMkdirAllCreatesMissingSegments(t *testing.T) {
	t.Parallel()

	var mkcolTargets []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "MKCOL" {
			http.Error(w, "unexpected method", http.StatusMethodNotAllowed)
			return
		}
		mkcolTargets = append(mkcolTargets, r.URL.Path)
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	client := webdav.NewClient()
	if err := client.MkdirAll(context.Background(), domain.Connection{Endpoint: server.URL, RootPath: "/dav"}, "", "/nested/dir"); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	if len(mkcolTargets) != 2 {
		t.Fatalf("len(mkcolTargets) = %d, want 2", len(mkcolTargets))
	}
}

func TestDeleteRemovesRemoteEntry(t *testing.T) {
	t.Parallel()

	var calledPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			http.Error(w, "unexpected method", http.StatusMethodNotAllowed)
			return
		}
		calledPath = r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := webdav.NewClient()
	if err := client.Delete(context.Background(), domain.Connection{Endpoint: server.URL, RootPath: "/dav"}, "", "/report.txt", false); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if calledPath != "/dav/report.txt" {
		t.Fatalf("calledPath = %q, want %q", calledPath, "/dav/report.txt")
	}
}

func TestMoveRenamesRemoteEntry(t *testing.T) {
	t.Parallel()

	var destination string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "MOVE" {
			http.Error(w, "unexpected method", http.StatusMethodNotAllowed)
			return
		}
		destination = r.Header.Get("Destination")
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	client := webdav.NewClient()
	if err := client.Move(context.Background(), domain.Connection{Endpoint: server.URL, RootPath: "/dav"}, "", "/old.txt", "/new.txt"); err != nil {
		t.Fatalf("Move() error = %v", err)
	}
	if !strings.HasSuffix(destination, "/dav/new.txt") {
		t.Fatalf("Destination = %q, want suffix %q", destination, "/dav/new.txt")
	}
}

func TestUploadAndDownloadRoundTrip(t *testing.T) {
	t.Parallel()

	stored := []byte("hello")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPut:
			body, _ := io.ReadAll(r.Body)
			stored = body
			w.WriteHeader(http.StatusCreated)
		case http.MethodGet:
			w.Header().Set("ETag", `"etag-upload"`)
			w.Header().Set("Last-Modified", "Mon, 24 Mar 2026 03:02:00 GMT")
			_, _ = w.Write(stored)
		default:
			http.Error(w, "unexpected method", http.StatusMethodNotAllowed)
		}
	}))
	defer server.Close()

	client := webdav.NewClient()
	if err := client.Upload(context.Background(), domain.Connection{Endpoint: server.URL, RootPath: "/dav"}, "", "/report.txt", strings.NewReader("payload"), "text/plain"); err != nil {
		t.Fatalf("Upload() error = %v", err)
	}

	reader, entry, err := client.Download(context.Background(), domain.Connection{Endpoint: server.URL, RootPath: "/dav"}, "", "/report.txt")
	if err != nil {
		t.Fatalf("Download() error = %v", err)
	}
	defer reader.Close()

	body, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("io.ReadAll() error = %v", err)
	}
	if string(body) != "payload" {
		t.Fatalf("body = %q, want %q", string(body), "payload")
	}
	if entry.ETag != `"etag-upload"` {
		t.Fatalf("ETag = %q, want %q", entry.ETag, `"etag-upload"`)
	}
}

func TestHealthCheckUsesOptions(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodOptions {
			http.Error(w, "unexpected method", http.StatusMethodNotAllowed)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := webdav.NewClient()
	if err := client.HealthCheck(context.Background(), domain.Connection{Endpoint: server.URL, RootPath: "/dav"}, ""); err != nil {
		t.Fatalf("HealthCheck() error = %v", err)
	}
}
