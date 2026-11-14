package transcoder

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/denuser/fuchsia-photos/backend/internal/vaapi"
)

// ErrUnavailable means the service could not be asked: it does not answer,
// refuses the token, or its graphics do not work. The video is not to blame,
// so its copy is tried again once the service is back.
var ErrUnavailable = errors.New("the transcoder is unavailable")

// Client calls the service over HTTP.
type Client struct {
	url   string
	token string
	http  *http.Client
}

// NewClient returns a client of the service at url, such as
// http://transcoder:8091, sending token as a bearer token.
func NewClient(url, token string) *Client {
	// No timeout of its own: a copy of a long 4K video takes minutes, and
	// the job's context bounds it.
	return &Client{url: strings.TrimRight(url, "/"), token: token, http: &http.Client{}}
}

// Hardware asks the service which graphics make the copies.
func (c *Client) Hardware(ctx context.Context) (vaapi.Hardware, error) {
	var hw vaapi.Hardware
	err := c.do(ctx, http.MethodGet, "/hardware", nil, &hw)
	return hw, err
}

// Transcode makes the copy of the video at in and writes it to out. Both
// paths must be the same in the service.
func (c *Client) Transcode(ctx context.Context, in, out string, video vaapi.Source) error {
	body, err := json.Marshal(Request{In: in, Out: out, Source: video})
	if err != nil {
		return err
	}
	return c.do(ctx, http.MethodPost, "/transcode", body, &struct{}{})
}

func (c *Client) do(ctx context.Context, method, path string, body []byte, into any) error {
	req, err := http.NewRequestWithContext(ctx, method, c.url+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	resp, err := c.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		var out struct {
			Error string `json:"error"`
		}
		detail, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		if json.Unmarshal(detail, &out) != nil || out.Error == "" {
			out.Error = strings.TrimSpace(string(detail))
		}
		err := fmt.Errorf("transcoder: %s %s: %s: %s", method, path, resp.Status, out.Error)
		switch resp.StatusCode {
		case http.StatusUnauthorized, http.StatusServiceUnavailable, http.StatusBadGateway, http.StatusGatewayTimeout:
			return fmt.Errorf("%w: %s", ErrUnavailable, err.Error())
		}
		return err
	}
	if err := json.NewDecoder(resp.Body).Decode(into); err != nil {
		return fmt.Errorf("%w: transcoder: %s %s: decode response: %v", ErrUnavailable, method, path, err)
	}
	return nil
}
