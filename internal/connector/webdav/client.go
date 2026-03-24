package webdav

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/xiaoxuesen/fn-cloudsync/internal/domain"
)

type Client struct {
	httpClient *http.Client
}

type propfindResponse struct {
	Responses []struct {
		Href     string `xml:"href"`
		Propstat struct {
			Prop struct {
				ResourceType struct {
					Collection *struct{} `xml:"collection"`
				} `xml:"resourcetype"`
				ETag          string `xml:"getetag"`
				LastModified  string `xml:"getlastmodified"`
				ContentLength string `xml:"getcontentlength"`
				ContentType   string `xml:"getcontenttype"`
			} `xml:"prop"`
		} `xml:"propstat"`
	} `xml:"response"`
}

func NewClient() *Client {
	return &Client{httpClient: &http.Client{}}
}

func (c *Client) Probe(ctx context.Context, connection domain.Connection, password string) (domain.ConnectionCapabilities, error) {
	optionsReq, err := http.NewRequestWithContext(ctx, http.MethodOptions, strings.TrimRight(connection.Endpoint, "/")+connection.RootPath, nil)
	if err != nil {
		return domain.ConnectionCapabilities{}, err
	}
	if connection.Username != "" {
		optionsReq.SetBasicAuth(connection.Username, password)
	}

	optionsResp, err := c.httpClient.Do(optionsReq)
	if err != nil {
		return domain.ConnectionCapabilities{}, err
	}
	defer optionsResp.Body.Close()

	propfindReq, err := http.NewRequestWithContext(ctx, "PROPFIND", strings.TrimRight(connection.Endpoint, "/")+connection.RootPath, bytes.NewBufferString(`<?xml version="1.0" encoding="utf-8"?><d:propfind xmlns:d="DAV:"><d:prop><d:getetag/><d:getlastmodified/><d:getcontentlength/></d:prop></d:propfind>`))
	if err != nil {
		return domain.ConnectionCapabilities{}, err
	}
	propfindReq.Header.Set("Depth", "0")
	propfindReq.Header.Set("Content-Type", "application/xml")
	if connection.Username != "" {
		propfindReq.SetBasicAuth(connection.Username, password)
	}

	propfindResp, err := c.httpClient.Do(propfindReq)
	if err != nil {
		return domain.ConnectionCapabilities{}, err
	}
	defer propfindResp.Body.Close()

	body, err := io.ReadAll(propfindResp.Body)
	if err != nil {
		return domain.ConnectionCapabilities{}, err
	}

	var ms propfindResponse
	if err := xml.Unmarshal(body, &ms); err != nil {
		return domain.ConnectionCapabilities{}, err
	}

	var p struct {
		ETag          string
		LastModified  string
		ContentLength string
	}
	if len(ms.Responses) > 0 {
		prop := ms.Responses[0].Propstat.Prop
		p.ETag = prop.ETag
		p.LastModified = prop.LastModified
		p.ContentLength = prop.ContentLength
	}

	allow := optionsResp.Header.Get("Allow")
	return domain.ConnectionCapabilities{
		SupportsETag:              p.ETag != "",
		SupportsLastModified:      p.LastModified != "",
		SupportsContentLength:     p.ContentLength != "",
		SupportsRecursivePropfind: true,
		SupportsMove:              strings.Contains(strings.ToUpper(allow), "MOVE"),
		PathEncodingMode:          "plain",
		MTimePrecision:            "second",
		ServerFingerprint:         fmt.Sprintf("%s|%s", connection.Endpoint, optionsResp.Header.Get("Server")),
		ProbeWarnings:             nil,
	}, nil
}

func (c *Client) Stat(ctx context.Context, connection domain.Connection, password string, targetPath string) (domain.RemoteEntry, error) {
	ms, err := c.propfind(ctx, connection, password, targetPath, "0")
	if err != nil {
		return domain.RemoteEntry{}, err
	}
	if len(ms.Responses) == 0 {
		return domain.RemoteEntry{}, fmt.Errorf("empty propfind response")
	}

	return toRemoteEntry(connection.RootPath, ms.Responses[0])
}

func (c *Client) List(ctx context.Context, connection domain.Connection, password string, targetPath string) ([]domain.RemoteEntry, error) {
	ms, err := c.propfind(ctx, connection, password, targetPath, "1")
	if err != nil {
		return nil, err
	}

	entries := make([]domain.RemoteEntry, 0, len(ms.Responses))
	for index, response := range ms.Responses {
		if index == 0 {
			continue
		}
		entry, err := toRemoteEntry(connection.RootPath, response)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}

	return entries, nil
}

func (c *Client) propfind(ctx context.Context, connection domain.Connection, password string, targetPath string, depth string) (propfindResponse, error) {
	requestURL := strings.TrimRight(connection.Endpoint, "/") + path.Join(connection.RootPath, targetPath)
	propfindReq, err := http.NewRequestWithContext(ctx, "PROPFIND", requestURL, bytes.NewBufferString(`<?xml version="1.0" encoding="utf-8"?><d:propfind xmlns:d="DAV:"><d:prop><d:resourcetype/><d:getetag/><d:getlastmodified/><d:getcontentlength/><d:getcontenttype/></d:prop></d:propfind>`))
	if err != nil {
		return propfindResponse{}, err
	}
	propfindReq.Header.Set("Depth", depth)
	propfindReq.Header.Set("Content-Type", "application/xml")
	if connection.Username != "" {
		propfindReq.SetBasicAuth(connection.Username, password)
	}

	propfindResp, err := c.httpClient.Do(propfindReq)
	if err != nil {
		return propfindResponse{}, err
	}
	defer propfindResp.Body.Close()

	body, err := io.ReadAll(propfindResp.Body)
	if err != nil {
		return propfindResponse{}, err
	}

	var ms propfindResponse
	if err := xml.Unmarshal(body, &ms); err != nil {
		return propfindResponse{}, err
	}

	return ms, nil
}

func toRemoteEntry(rootPath string, response struct {
	Href     string `xml:"href"`
	Propstat struct {
		Prop struct {
			ResourceType struct {
				Collection *struct{} `xml:"collection"`
			} `xml:"resourcetype"`
			ETag          string `xml:"getetag"`
			LastModified  string `xml:"getlastmodified"`
			ContentLength string `xml:"getcontentlength"`
			ContentType   string `xml:"getcontenttype"`
		} `xml:"prop"`
	} `xml:"propstat"`
}) (domain.RemoteEntry, error) {
	decodedHref, err := url.PathUnescape(response.Href)
	if err != nil {
		return domain.RemoteEntry{}, err
	}

	relativePath := strings.TrimPrefix(decodedHref, rootPath)
	if relativePath == "" {
		relativePath = "/"
	}

	size := int64(0)
	if response.Propstat.Prop.ContentLength != "" {
		size, err = strconv.ParseInt(response.Propstat.Prop.ContentLength, 10, 64)
		if err != nil {
			return domain.RemoteEntry{}, err
		}
	}

	var mtime time.Time
	if response.Propstat.Prop.LastModified != "" {
		mtime, err = time.Parse(time.RFC1123, response.Propstat.Prop.LastModified)
		if err != nil {
			return domain.RemoteEntry{}, err
		}
	}

	return domain.RemoteEntry{
		Path:        ensureLeadingSlash(relativePath),
		IsDir:       response.Propstat.Prop.ResourceType.Collection != nil,
		Size:        size,
		MTime:       mtime.UTC(),
		ETag:        response.Propstat.Prop.ETag,
		ContentType: response.Propstat.Prop.ContentType,
		Exists:      true,
	}, nil
}

func ensureLeadingSlash(value string) string {
	if value == "" {
		return "/"
	}
	if strings.HasPrefix(value, "/") {
		return value
	}
	return "/" + value
}

func (c *Client) MkdirAll(ctx context.Context, connection domain.Connection, password string, targetPath string) error {
	current := ""
	for _, segment := range strings.Split(strings.Trim(targetPath, "/"), "/") {
		if segment == "" {
			continue
		}
		current += "/" + segment
		request, err := http.NewRequestWithContext(ctx, "MKCOL", strings.TrimRight(connection.Endpoint, "/")+path.Join(connection.RootPath, current), nil)
		if err != nil {
			return err
		}
		if connection.Username != "" {
			request.SetBasicAuth(connection.Username, password)
		}

		response, err := c.httpClient.Do(request)
		if err != nil {
			return err
		}
		response.Body.Close()
	}

	return nil
}

func (c *Client) Delete(ctx context.Context, connection domain.Connection, password string, targetPath string, _ bool) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodDelete, strings.TrimRight(connection.Endpoint, "/")+path.Join(connection.RootPath, targetPath), nil)
	if err != nil {
		return err
	}
	if connection.Username != "" {
		request.SetBasicAuth(connection.Username, password)
	}

	response, err := c.httpClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	return nil
}

func (c *Client) Move(ctx context.Context, connection domain.Connection, password string, srcPath string, dstPath string) error {
	request, err := http.NewRequestWithContext(ctx, "MOVE", strings.TrimRight(connection.Endpoint, "/")+path.Join(connection.RootPath, srcPath), nil)
	if err != nil {
		return err
	}
	request.Header.Set("Destination", strings.TrimRight(connection.Endpoint, "/")+path.Join(connection.RootPath, dstPath))
	if connection.Username != "" {
		request.SetBasicAuth(connection.Username, password)
	}

	response, err := c.httpClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	return nil
}

func (c *Client) Upload(ctx context.Context, connection domain.Connection, password string, targetPath string, reader io.Reader, contentType string) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodPut, strings.TrimRight(connection.Endpoint, "/")+path.Join(connection.RootPath, targetPath), reader)
	if err != nil {
		return err
	}
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	if connection.Username != "" {
		request.SetBasicAuth(connection.Username, password)
	}

	response, err := c.httpClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	return nil
}

func (c *Client) Download(ctx context.Context, connection domain.Connection, password string, targetPath string) (io.ReadCloser, domain.RemoteEntry, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(connection.Endpoint, "/")+path.Join(connection.RootPath, targetPath), nil)
	if err != nil {
		return nil, domain.RemoteEntry{}, err
	}
	if connection.Username != "" {
		request.SetBasicAuth(connection.Username, password)
	}

	response, err := c.httpClient.Do(request)
	if err != nil {
		return nil, domain.RemoteEntry{}, err
	}

	var mtime time.Time
	if header := response.Header.Get("Last-Modified"); header != "" {
		mtime, _ = time.Parse(time.RFC1123, header)
	}

	return response.Body, domain.RemoteEntry{
		Path:   ensureLeadingSlash(targetPath),
		ETag:   response.Header.Get("ETag"),
		MTime:  mtime.UTC(),
		Exists: true,
	}, nil
}

func (c *Client) HealthCheck(ctx context.Context, connection domain.Connection, password string) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodOptions, strings.TrimRight(connection.Endpoint, "/")+connection.RootPath, nil)
	if err != nil {
		return err
	}
	if connection.Username != "" {
		request.SetBasicAuth(connection.Username, password)
	}

	response, err := c.httpClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	return nil
}
