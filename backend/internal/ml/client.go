// Package ml connects the application to the ML service, which turns photos
// and search texts into vectors. The service keeps nothing: the application
// stores the vectors and checks permissions. See the "Machine learning"
// section of docs/architecture.md.
package ml

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"time"
)

// ErrModelMismatch means the service runs a model other than the configured one.
var ErrModelMismatch = errors.New("the ML service runs another model")

// Client calls the ML service over HTTP.
type Client struct {
	url   string
	token string
	http  *http.Client
}

// NewClient returns a client of the service at url, such as
// http://mac.local:8090. The token is sent as a bearer token if set.
func NewClient(url, token string) *Client {
	return &Client{
		url:   strings.TrimRight(url, "/"),
		token: token,
		// A batch of previews on a busy machine can take a while.
		http: &http.Client{Timeout: 2 * time.Minute},
	}
}

// Health is what the service reports about itself.
type Health struct {
	// Model names the model and the version of the service's preprocessing.
	Model      string `json:"model"`
	Dimensions int    `json:"dimensions"`
	Device     string `json:"device"`
	// MinSimilarity is the least cosine similarity of a search match, if the
	// service knows one for its model.
	MinSimilarity *float64 `json:"minSimilarity"`
}

// Health asks the service which model it runs.
func (c *Client) Health(ctx context.Context) (Health, error) {
	var health Health
	err := c.do(ctx, http.MethodGet, "/health", "", nil, &health)
	return health, err
}

// TextEmbedding is the vector of a search text.
type TextEmbedding struct {
	Model     string    `json:"model"`
	Embedding []float32 `json:"embedding"`
	// MinSimilarity is the least cosine similarity of a photo that matches
	// the text, if the service knows one for its model.
	MinSimilarity *float64 `json:"minSimilarity"`
}

// EmbedText returns the vector of a search text.
func (c *Client) EmbedText(ctx context.Context, text string) (TextEmbedding, error) {
	body, err := json.Marshal(map[string]string{"text": text})
	if err != nil {
		return TextEmbedding{}, err
	}
	var out TextEmbedding
	err = c.do(ctx, http.MethodPost, "/embed/text", "application/json", bytes.NewReader(body), &out)
	return out, err
}

// Image is one preview to embed.
type Image struct {
	Name string
	Data []byte
}

// ImageResult is the vector of one image, or the reason the service could not
// make it.
type ImageResult struct {
	Embedding []float32 `json:"embedding"`
	Error     string    `json:"error"`
}

// EmbedImages returns one result per image, in order, and the model that made
// them.
func (c *Client) EmbedImages(ctx context.Context, images []Image) (string, []ImageResult, error) {
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	for _, image := range images {
		part, err := form.CreateFormFile("images", image.Name)
		if err != nil {
			return "", nil, err
		}
		if _, err := part.Write(image.Data); err != nil {
			return "", nil, err
		}
	}
	if err := form.Close(); err != nil {
		return "", nil, err
	}
	var out struct {
		Model   string        `json:"model"`
		Results []ImageResult `json:"results"`
	}
	if err := c.do(ctx, http.MethodPost, "/embed/image", form.FormDataContentType(), &body, &out); err != nil {
		return "", nil, err
	}
	if len(out.Results) != len(images) {
		return "", nil, fmt.Errorf("the ML service returned %d results for %d images", len(out.Results), len(images))
	}
	return out.Model, out.Results, nil
}

func (c *Client) do(ctx context.Context, method, path, contentType string, body io.Reader, into any) error {
	req, err := http.NewRequestWithContext(ctx, method, c.url+path, body)
	if err != nil {
		return err
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		detail, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("ML service: %s %s: %s: %s", method, path, resp.Status, strings.TrimSpace(string(detail)))
	}
	if err := json.NewDecoder(resp.Body).Decode(into); err != nil {
		return fmt.Errorf("ML service: %s %s: decode response: %w", method, path, err)
	}
	return nil
}
