package model

import "time"

const (
	TargetHost      = "host"
	TargetContainer = "container"
)

type Target struct {
	Kind   string `json:"kind"`
	ID     string `json:"id,omitempty"`
	Name   string `json:"name"`
	Image  string `json:"image,omitempty"`
	Status string `json:"status,omitempty"`
}

type CreateSessionRequest struct {
	Target Target `json:"target"`
}

type SessionSummary struct {
	ID             string     `json:"id"`
	Title          string     `json:"title"`
	Target         Target     `json:"target"`
	State          string     `json:"state"`
	CreatedAt      time.Time  `json:"createdAt"`
	ExpiresAt      time.Time  `json:"expiresAt"`
	ReconnectUntil *time.Time `json:"reconnectUntil,omitempty"`
}

type ControlMessage struct {
	Type           string `json:"type"`
	State          string `json:"state,omitempty"`
	Cols           uint16 `json:"cols,omitempty"`
	Rows           uint16 `json:"rows,omitempty"`
	Code           int    `json:"code,omitempty"`
	Signal         string `json:"signal,omitempty"`
	Message        string `json:"message,omitempty"`
	ReconnectUntil string `json:"reconnectUntil,omitempty"`
}

type APIError struct {
	Error ErrorBody `json:"error"`
}

type ErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}
