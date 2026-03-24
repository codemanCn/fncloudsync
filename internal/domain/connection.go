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
	Status             string
	CreatedAt          time.Time
	UpdatedAt          time.Time
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
