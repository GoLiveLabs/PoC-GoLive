// Package mediaserver provides a client for the MediaMTX HTTP API.
package mediaserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

const requestTimeout = 5 * time.Second

// StreamInfo represents a single active stream path reported by MediaMTX.
type StreamInfo struct {
	Name  string
	Ready bool
}

// Client talks to the MediaMTX HTTP API.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// NewClient creates a Client targeting the given MediaMTX API base URL
// (e.g. "http://localhost:9997").
func NewClient(baseURL string) *Client {
	return &Client{
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: requestTimeout},
	}
}

type pathsListResponse struct {
	ItemCount int    `json:"itemCount"`
	PageCount int    `json:"pageCount"`
	Items     []item `json:"items"`
}

type item struct {
	Name  string `json:"name"`
	Ready bool   `json:"ready"`
}

// ListActiveStreams fetches all paths from MediaMTX and returns only the
// ones that currently have a ready (active) source.
func (c *Client) ListActiveStreams(ctx context.Context) ([]StreamInfo, error) {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()

	url := c.baseURL + "/v3/paths/list"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("building mediamtx request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("calling mediamtx: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("mediamtx returned status %d", resp.StatusCode)
	}

	var parsed pathsListResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("decoding mediamtx response: %w", err)
	}

	streams := make([]StreamInfo, 0, len(parsed.Items))
	for _, it := range parsed.Items {
		if !it.Ready {
			continue
		}
		streams = append(streams, StreamInfo{Name: it.Name, Ready: it.Ready})
	}
	return streams, nil
}
