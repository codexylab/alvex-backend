package handlers

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type stubHealthChecker struct {
	err error
}

func (s stubHealthChecker) PingContext(context.Context) error {
	return s.err
}

func TestHealthHandlerLive(t *testing.T) {
	handler := NewHealthHandler(stubHealthChecker{}, time.Now())
	recorder := httptest.NewRecorder()

	handler.Live(recorder, httptest.NewRequest(http.MethodGet, "/health/live", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}
}

func TestHealthHandlerReady(t *testing.T) {
	tests := []struct {
		name        string
		databaseErr error
		wantStatus  int
	}{
		{name: "database available", wantStatus: http.StatusOK},
		{name: "database unavailable", databaseErr: errors.New("unavailable"), wantStatus: http.StatusServiceUnavailable},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			handler := NewHealthHandler(stubHealthChecker{err: test.databaseErr}, time.Now())
			recorder := httptest.NewRecorder()

			handler.Ready(recorder, httptest.NewRequest(http.MethodGet, "/health/ready", nil))

			if recorder.Code != test.wantStatus {
				t.Fatalf("expected status %d, got %d", test.wantStatus, recorder.Code)
			}
		})
	}
}
