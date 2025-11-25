package s2exporter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"go.uber.org/zap"
)

type S2Client struct {
	endpoint   string
	apiKey     string
	httpClient *http.Client
	logger     *zap.Logger

	streamsMu      sync.RWMutex
	knownStreams   map[string]bool
}

func NewS2Client(endpoint, apiKey string, logger *zap.Logger) *S2Client {
	return &S2Client{
		endpoint: endpoint,
		apiKey:   apiKey,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		logger:       logger,
		knownStreams: make(map[string]bool),
	}
}

func (c *S2Client) EnsureStream(ctx context.Context, streamID string) error {
	c.streamsMu.RLock()
	if c.knownStreams[streamID] {
		c.streamsMu.RUnlock()
		return nil
	}
	c.streamsMu.RUnlock()

	c.streamsMu.Lock()
	defer c.streamsMu.Unlock()

	if c.knownStreams[streamID] {
		return nil
	}

	if err := c.createStream(ctx, streamID); err != nil {
		return err
	}

	c.knownStreams[streamID] = true
	return nil
}

func (c *S2Client) createStream(ctx context.Context, streamID string) error {
	payload := map[string]interface{}{
		"stream": streamID,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal stream creation request: %w", err)
	}

	url := fmt.Sprintf("%s/streams", c.endpoint)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	c.setHeaders(req)

	resp, err := c.doWithRetry(ctx, req, 3)
	if err != nil {
		return fmt.Errorf("failed to create stream: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusConflict {
		return nil
	}

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("stream creation failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	c.logger.Info("Created stream", zap.String("stream", streamID))
	return nil
}

func (c *S2Client) AppendEvents(ctx context.Context, streamID string, events []*S2Event) error {
	if len(events) == 0 {
		return nil
	}

	records := make([]map[string]interface{}, len(events))
	for i, event := range events {
		eventJSON, err := event.ToJSON()
		if err != nil {
			return fmt.Errorf("failed to marshal event: %w", err)
		}
		records[i] = map[string]interface{}{
			"body": string(eventJSON),
		}
	}

	payload := map[string]interface{}{
		"records": records,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal append request: %w", err)
	}

	url := fmt.Sprintf("%s/streams/%s/records", c.endpoint, streamID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	c.setHeaders(req)

	resp, err := c.doWithRetry(ctx, req, 3)
	if err != nil {
		return fmt.Errorf("failed to append events: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("append failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

func (c *S2Client) setHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.apiKey))
}

func (c *S2Client) doWithRetry(ctx context.Context, req *http.Request, maxRetries int) (*http.Response, error) {
	var lastErr error
	backoff := 100 * time.Millisecond

	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff):
				backoff *= 2
			}

			body, _ := io.ReadAll(req.Body)
			req.Body = io.NopCloser(bytes.NewReader(body))
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = err
			c.logger.Warn("Request failed, retrying",
				zap.Int("attempt", attempt+1),
				zap.Error(err))
			continue
		}

		if resp.StatusCode >= 500 {
			resp.Body.Close()
			lastErr = fmt.Errorf("server error: %d", resp.StatusCode)
			c.logger.Warn("Server error, retrying",
				zap.Int("attempt", attempt+1),
				zap.Int("status", resp.StatusCode))
			continue
		}

		return resp, nil
	}

	return nil, fmt.Errorf("max retries exceeded: %w", lastErr)
}


