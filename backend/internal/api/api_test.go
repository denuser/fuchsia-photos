package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type fakeDB struct{ err error }

func (f fakeDB) Ping(context.Context) error { return f.err }

func TestRoutes(t *testing.T) {
	static := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("static"))
	})

	tests := []struct {
		name       string
		db         fakeDB
		method     string
		path       string
		wantStatus int
		wantBody   string
	}{
		{"healthy", fakeDB{}, http.MethodGet, "/api/v1/health", http.StatusOK, `"ok"`},
		{"database down", fakeDB{errors.New("down")}, http.MethodGet, "/api/v1/health", http.StatusServiceUnavailable, `"unavailable"`},
		{"unknown api path is not the frontend", fakeDB{}, http.MethodGet, "/api/v1/nope", http.StatusNotFound, `"not found"`},
		{"other paths are the frontend", fakeDB{}, http.MethodGet, "/albums", http.StatusOK, "static"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			NewHandler(tt.db, static).ServeHTTP(rec, httptest.NewRequest(tt.method, tt.path, nil))

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if !strings.Contains(rec.Body.String(), tt.wantBody) {
				t.Errorf("body = %q, want it to contain %q", rec.Body.String(), tt.wantBody)
			}
		})
	}
}
