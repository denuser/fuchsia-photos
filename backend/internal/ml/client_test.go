package ml

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

type faceRoundTripper func(*http.Request) (*http.Response, error)

func (f faceRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func TestDetectFaces(t *testing.T) {
	vector := make([]float32, 128)
	vector[0] = 1
	valid := DetectedFace{Box: []float64{0.1, 0.2, 0.3, 0.4}, Embedding: vector}
	tests := []struct {
		name  string
		model string
		faces []DetectedFace
		err   bool
	}{
		{name: "one face", model: "face@1", faces: []DetectedFace{valid}},
		{name: "no faces", model: "face@1", faces: []DetectedFace{}},
		{name: "model changed", model: "face@2", faces: []DetectedFace{valid}, err: true},
		{name: "missing face list", model: "face@1", err: true},
		{name: "invalid box", model: "face@1", faces: []DetectedFace{{Box: []float64{0.9, 0.2, 0.3, 0.4}, Embedding: vector}}, err: true},
		{name: "invalid vector", model: "face@1", faces: []DetectedFace{{Box: valid.Box, Embedding: make([]float32, 128)}}, err: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := NewClient("http://ml", "secret")
			client.http.Transport = faceRoundTripper(func(r *http.Request) (*http.Response, error) {
				body, err := io.ReadAll(r.Body)
				if err != nil {
					return nil, err
				}
				if r.Method != http.MethodPost || r.URL.Path != "/faces" ||
					r.Header.Get("Authorization") != "Bearer secret" ||
					r.Header.Get("Content-Type") != "application/octet-stream" || string(body) != "preview" {
					t.Fatal("unexpected face inference request")
				}
				response, err := json.Marshal(map[string]any{"model": tt.model, "faces": tt.faces})
				if err != nil {
					return nil, err
				}
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(string(response)))}, nil
			})
			faces, err := client.DetectFaces(t.Context(), []byte("preview"), FaceModel{Model: "face@1", Dimensions: 128, Scores: true})
			if (err != nil) != tt.err {
				t.Fatalf("DetectFaces returned %v, want error %v", err, tt.err)
			}
			if tt.model != "face@1" && !errors.Is(err, ErrModelMismatch) {
				t.Fatalf("got %v, want model mismatch", err)
			}
			if err == nil && len(faces) != len(tt.faces) {
				t.Fatalf("got %d faces, want %d", len(faces), len(tt.faces))
			}
		})
	}
}
