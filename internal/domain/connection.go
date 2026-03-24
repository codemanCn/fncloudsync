package domain

import (
	"errors"
	"strings"
	"time"
)

type TLSMode string

const (
	TLSModeStrict   TLSMode = "strict"
	TLSModeInsecure TLSMode = "insecure"
)

type Connection struct {
	ID                 string
	Name               string
	Endpoint           string
	Username           string
	PasswordCiphertext string
	RootPath           string
	TLSMode            TLSMode
	TimeoutSec         int
	CapabilitiesJSON   string
	Status             string
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

type ConnectionCapabilities struct {
	SupportsETag              bool     `json:"supports_etag"`
	SupportsLastModified      bool     `json:"supports_last_modified"`
	SupportsContentLength     bool     `json:"supports_content_length"`
	SupportsRecursivePropfind bool     `json:"supports_recursive_propfind"`
	SupportsMove              bool     `json:"supports_move"`
	PathEncodingMode          string   `json:"path_encoding_mode"`
	MTimePrecision            string   `json:"mtime_precision"`
	ServerFingerprint         string   `json:"server_fingerprint"`
	ProbeWarnings             []string `json:"probe_warnings"`
}

type ConnectionTestResult struct {
	Success      bool                   `json:"success"`
	Capabilities ConnectionCapabilities `json:"capabilities"`
}

func (c Connection) Validate() error {
	if strings.TrimSpace(c.Name) == "" {
		return errors.Join(ErrInvalidArgument, errors.New("name is required"))
	}
	if strings.TrimSpace(c.Endpoint) == "" {
		return errors.Join(ErrInvalidArgument, errors.New("endpoint is required"))
	}
	if strings.TrimSpace(c.Username) == "" {
		return errors.Join(ErrInvalidArgument, errors.New("username is required"))
	}

	return nil
}
