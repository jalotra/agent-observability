package agentsdk

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type S2Client struct {
	endpoint   string
	apiKey     string
	httpClient *http.Client
}

func NewS2Client(endpoint, apiKey string) *S2Client {
	return &S2Client{
		endpoint: endpoint,
		apiKey:   apiKey,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (c *S2Client) CreateStream(ctx context.Context, streamName string) error {
	payload := map[string]interface{}{
		"stream": streamName,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/streams", c.endpoint)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	c.setHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusConflict {
		return nil
	}

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("stream creation failed (%d): %s", resp.StatusCode, string(respBody))
	}

	return nil
}

func (c *S2Client) AppendEvent(ctx context.Context, streamName string, event *Event) error {
	eventJSON, err := event.ToJSON()
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	payload := map[string]interface{}{
		"records": []map[string]interface{}{
			{"body": string(eventJSON)},
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/streams/%s/records", c.endpoint, streamName)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	c.setHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("append failed (%d): %s", resp.StatusCode, string(respBody))
	}

	return nil
}

func (c *S2Client) setHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.apiKey))
}

type StreamReader struct {
	client     *S2Client
	streamName string
	lastSeq    int64
}

func (c *S2Client) NewStreamReader(streamName string) *StreamReader {
	return &StreamReader{
		client:     c,
		streamName: streamName,
		lastSeq:    0,
	}
}

func (r *StreamReader) ReadEvents(ctx context.Context) ([]*Event, error) {
	url := fmt.Sprintf("%s/streams/%s/records?after=%d", r.client.endpoint, r.streamName, r.lastSeq)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	r.client.setHeaders(req)

	resp, err := r.client.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("read failed (%d): %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Records []struct {
			Sequence int64  `json:"sequence"`
			Body     string `json:"body"`
		} `json:"records"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	events := make([]*Event, 0, len(result.Records))
	for _, rec := range result.Records {
		event, err := EventFromJSON([]byte(rec.Body))
		if err != nil {
			continue
		}
		events = append(events, event)
		if rec.Sequence > r.lastSeq {
			r.lastSeq = rec.Sequence
		}
	}

	return events, nil
}

