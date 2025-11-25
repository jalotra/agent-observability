package s2exporter

import (
	"context"
	"sync"
	"time"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/zap"
)

type s2Exporter struct {
	config    *Config
	logger    *zap.Logger
	client    *S2Client
	converter *EventConverter

	bufferMu sync.Mutex
	buffers  map[string][]*S2Event

	stopCh chan struct{}
	wg     sync.WaitGroup
}

func newS2Exporter(cfg *Config, logger *zap.Logger) (*s2Exporter, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	client := NewS2Client(cfg.Endpoint, cfg.APIKey, logger)
	converter := NewEventConverter()

	return &s2Exporter{
		config:    cfg,
		logger:    logger,
		client:    client,
		converter: converter,
		buffers:   make(map[string][]*S2Event),
		stopCh:    make(chan struct{}),
	}, nil
}

func (e *s2Exporter) start(ctx context.Context, _ component.Host) error {
	e.wg.Add(1)
	go e.flushLoop()
	e.logger.Info("S2 exporter started",
		zap.String("endpoint", e.config.Endpoint),
		zap.String("stream_prefix", e.config.StreamPrefix))
	return nil
}

func (e *s2Exporter) shutdown(ctx context.Context) error {
	close(e.stopCh)
	e.wg.Wait()
	e.flushAllBuffers(ctx)
	e.logger.Info("S2 exporter shutdown complete")
	return nil
}

func (e *s2Exporter) pushTraces(ctx context.Context, td ptrace.Traces) error {
	events := e.converter.ConvertTraces(td)

	e.bufferMu.Lock()
	for _, event := range events {
		streamID := e.getStreamID(event.ConversationID)
		e.buffers[streamID] = append(e.buffers[streamID], event)

		if len(e.buffers[streamID]) >= e.config.BatchSize {
			batch := e.buffers[streamID]
			e.buffers[streamID] = nil
			e.bufferMu.Unlock()
			if err := e.flushBatch(ctx, streamID, batch); err != nil {
				e.logger.Error("Failed to flush batch", zap.Error(err), zap.String("stream", streamID))
			}
			e.bufferMu.Lock()
		}
	}
	e.bufferMu.Unlock()

	return nil
}

func (e *s2Exporter) flushLoop() {
	defer e.wg.Done()
	ticker := time.NewTicker(e.config.FlushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			e.flushAllBuffers(context.Background())
		case <-e.stopCh:
			return
		}
	}
}

func (e *s2Exporter) flushAllBuffers(ctx context.Context) {
	e.bufferMu.Lock()
	buffersToFlush := make(map[string][]*S2Event)
	for streamID, events := range e.buffers {
		if len(events) > 0 {
			buffersToFlush[streamID] = events
			e.buffers[streamID] = nil
		}
	}
	e.bufferMu.Unlock()

	for streamID, events := range buffersToFlush {
		if err := e.flushBatch(ctx, streamID, events); err != nil {
			e.logger.Error("Failed to flush buffer",
				zap.Error(err),
				zap.String("stream", streamID),
				zap.Int("event_count", len(events)))
		}
	}
}

func (e *s2Exporter) flushBatch(ctx context.Context, streamID string, events []*S2Event) error {
	if len(events) == 0 {
		return nil
	}

	if err := e.client.EnsureStream(ctx, streamID); err != nil {
		return err
	}

	if err := e.client.AppendEvents(ctx, streamID, events); err != nil {
		return err
	}

	e.logger.Debug("Flushed events to S2",
		zap.String("stream", streamID),
		zap.Int("event_count", len(events)))

	return nil
}

func (e *s2Exporter) getStreamID(conversationID string) string {
	if conversationID == "" {
		conversationID = "default"
	}
	return e.config.StreamPrefix + conversationID
}

