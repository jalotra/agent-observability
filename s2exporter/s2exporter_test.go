package s2exporter

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/zap"
)

func TestConfigValidation(t *testing.T) {
	tests := []struct {
		name    string
		config  Config
		wantErr bool
	}{
		{
			name: "valid config",
			config: Config{
				Endpoint: "https://api.s2.dev",
				APIKey:   "test-key",
			},
			wantErr: false,
		},
		{
			name: "missing endpoint",
			config: Config{
				APIKey: "test-key",
			},
			wantErr: true,
		},
		{
			name: "missing api key",
			config: Config{
				Endpoint: "https://api.s2.dev",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestConfigDefaults(t *testing.T) {
	cfg := Config{
		Endpoint: "https://api.s2.dev",
		APIKey:   "test-key",
	}
	_ = cfg.Validate()

	if cfg.BatchSize != 100 {
		t.Errorf("BatchSize = %d, want 100", cfg.BatchSize)
	}
	if cfg.FlushInterval != 5*time.Second {
		t.Errorf("FlushInterval = %v, want 5s", cfg.FlushInterval)
	}
	if cfg.StreamPrefix != "agent-session-" {
		t.Errorf("StreamPrefix = %s, want agent-session-", cfg.StreamPrefix)
	}
}

func TestEventConverter(t *testing.T) {
	converter := NewEventConverter()

	td := ptrace.NewTraces()
	rs := td.ResourceSpans().AppendEmpty()
	rs.Resource().Attributes().PutStr("service.name", "test-service")

	ss := rs.ScopeSpans().AppendEmpty()
	span := ss.Spans().AppendEmpty()
	span.SetName("test-span")
	span.SetTraceID(pcommon.TraceID([16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}))
	span.SetSpanID(pcommon.SpanID([8]byte{1, 2, 3, 4, 5, 6, 7, 8}))
	span.SetStartTimestamp(pcommon.NewTimestampFromTime(time.Now()))
	span.SetEndTimestamp(pcommon.NewTimestampFromTime(time.Now().Add(100 * time.Millisecond)))
	span.Attributes().PutStr(AttrConversationID, "conv-123")
	span.Attributes().PutStr(AttrOperationName, "invoke_agent")
	span.Attributes().PutStr(AttrAgentName, "TestAgent")

	events := converter.ConvertTraces(td)

	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}

	event := events[0]
	if event.ConversationID != "conv-123" {
		t.Errorf("ConversationID = %s, want conv-123", event.ConversationID)
	}
	if event.OperationType != "invoke_agent" {
		t.Errorf("OperationType = %s, want invoke_agent", event.OperationType)
	}
	if event.SpanName != "test-span" {
		t.Errorf("SpanName = %s, want test-span", event.SpanName)
	}
}

func TestEventConverterFallbackToTraceID(t *testing.T) {
	converter := NewEventConverter()

	td := ptrace.NewTraces()
	rs := td.ResourceSpans().AppendEmpty()
	ss := rs.ScopeSpans().AppendEmpty()
	span := ss.Spans().AppendEmpty()
	span.SetName("test-span")
	traceID := pcommon.TraceID([16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16})
	span.SetTraceID(traceID)
	span.SetSpanID(pcommon.SpanID([8]byte{1, 2, 3, 4, 5, 6, 7, 8}))
	span.Attributes().PutStr(AttrOperationName, "chat")

	events := converter.ConvertTraces(td)

	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}

	if events[0].ConversationID != traceID.String() {
		t.Errorf("ConversationID = %s, want %s", events[0].ConversationID, traceID.String())
	}
}

func TestS2EventJSON(t *testing.T) {
	event := &S2Event{
		Timestamp:      time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		TraceID:        "trace-123",
		SpanID:         "span-456",
		ConversationID: "conv-789",
		OperationType:  "invoke_agent",
		SpanName:       "test",
		Duration:       100 * time.Millisecond,
		Status:         "ok",
		Attributes: map[string]interface{}{
			"gen_ai.agent.name": "TestBot",
		},
	}

	jsonBytes, err := event.ToJSON()
	if err != nil {
		t.Fatalf("ToJSON() error = %v", err)
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal(jsonBytes, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	if decoded["conversation_id"] != "conv-789" {
		t.Errorf("conversation_id = %v, want conv-789", decoded["conversation_id"])
	}
	if decoded["operation_type"] != "invoke_agent" {
		t.Errorf("operation_type = %v, want invoke_agent", decoded["operation_type"])
	}
}

func TestS2ClientCreateStream(t *testing.T) {
	streamCreated := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/streams" && r.Method == http.MethodPost {
			streamCreated = true
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(`{"stream": "test-stream"}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	logger := zap.NewNop()
	client := NewS2Client(server.URL, "test-key", logger)

	err := client.EnsureStream(context.Background(), "test-stream")
	if err != nil {
		t.Errorf("EnsureStream() error = %v", err)
	}

	if !streamCreated {
		t.Error("stream was not created")
	}

	streamCreated = false
	err = client.EnsureStream(context.Background(), "test-stream")
	if err != nil {
		t.Errorf("EnsureStream() second call error = %v", err)
	}
	if streamCreated {
		t.Error("stream should not be created twice")
	}
}

func TestS2ClientAppendEvents(t *testing.T) {
	var receivedRecords []interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/streams/test-stream/records" && r.Method == http.MethodPost {
			var body map[string]interface{}
			json.NewDecoder(r.Body).Decode(&body)
			receivedRecords = body["records"].([]interface{})
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	logger := zap.NewNop()
	client := NewS2Client(server.URL, "test-key", logger)

	events := []*S2Event{
		{
			Timestamp:      time.Now(),
			TraceID:        "trace-1",
			SpanID:         "span-1",
			ConversationID: "conv-1",
			OperationType:  "chat",
			SpanName:       "test",
			Status:         "ok",
			Attributes:     map[string]interface{}{},
		},
		{
			Timestamp:      time.Now(),
			TraceID:        "trace-1",
			SpanID:         "span-2",
			ConversationID: "conv-1",
			OperationType:  "execute_tool",
			SpanName:       "tool.search",
			Status:         "ok",
			Attributes:     map[string]interface{}{},
		},
	}

	err := client.AppendEvents(context.Background(), "test-stream", events)
	if err != nil {
		t.Errorf("AppendEvents() error = %v", err)
	}

	if len(receivedRecords) != 2 {
		t.Errorf("received %d records, want 2", len(receivedRecords))
	}
}

func TestGetStreamID(t *testing.T) {
	cfg := &Config{
		Endpoint:     "https://api.s2.dev",
		APIKey:       "test-key",
		StreamPrefix: "agent-session-",
	}
	_ = cfg.Validate()

	logger := zap.NewNop()
	exp, _ := newS2Exporter(cfg, logger)

	tests := []struct {
		conversationID string
		want           string
	}{
		{"conv-123", "agent-session-conv-123"},
		{"", "agent-session-default"},
		{"session-abc", "agent-session-session-abc"},
	}

	for _, tt := range tests {
		got := exp.getStreamID(tt.conversationID)
		if got != tt.want {
			t.Errorf("getStreamID(%q) = %q, want %q", tt.conversationID, got, tt.want)
		}
	}
}


