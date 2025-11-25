package s2exporter

import (
	"encoding/json"
	"time"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

const (
	AttrConversationID   = "gen_ai.conversation.id"
	AttrAgentID          = "gen_ai.agent.id"
	AttrAgentName        = "gen_ai.agent.name"
	AttrOperationName    = "gen_ai.operation.name"
	AttrToolName         = "gen_ai.tool.name"
	AttrToolCallID       = "gen_ai.tool.call.id"
	AttrToolCallArgs     = "gen_ai.tool.call.arguments"
	AttrToolCallResult   = "gen_ai.tool.call.result"
	AttrInputMessages    = "gen_ai.input.messages"
	AttrOutputMessages   = "gen_ai.output.messages"
	AttrSystemPrompt     = "gen_ai.system_instructions"
	AttrProviderName     = "gen_ai.provider.name"
	AttrRequestModel     = "gen_ai.request.model"
	AttrResponseModel    = "gen_ai.response.model"
	AttrInputTokens      = "gen_ai.usage.input_tokens"
	AttrOutputTokens     = "gen_ai.usage.output_tokens"
)

type S2Event struct {
	Timestamp      time.Time              `json:"timestamp"`
	TraceID        string                 `json:"trace_id"`
	SpanID         string                 `json:"span_id"`
	ParentSpanID   string                 `json:"parent_span_id,omitempty"`
	ConversationID string                 `json:"conversation_id"`
	OperationType  string                 `json:"operation_type"`
	SpanName       string                 `json:"span_name"`
	Duration       time.Duration          `json:"duration_ns"`
	Status         string                 `json:"status"`
	Attributes     map[string]interface{} `json:"attributes"`
}

type EventConverter struct{}

func NewEventConverter() *EventConverter {
	return &EventConverter{}
}

func (c *EventConverter) ConvertTraces(td ptrace.Traces) []*S2Event {
	var events []*S2Event

	resourceSpans := td.ResourceSpans()
	for i := 0; i < resourceSpans.Len(); i++ {
		rs := resourceSpans.At(i)
		resourceAttrs := extractAttributes(rs.Resource().Attributes())

		scopeSpans := rs.ScopeSpans()
		for j := 0; j < scopeSpans.Len(); j++ {
			ss := scopeSpans.At(j)
			spans := ss.Spans()

			for k := 0; k < spans.Len(); k++ {
				span := spans.At(k)
				event := c.convertSpan(span, resourceAttrs)
				events = append(events, event)
			}
		}
	}

	return events
}

func (c *EventConverter) convertSpan(span ptrace.Span, resourceAttrs map[string]interface{}) *S2Event {
	attrs := extractAttributes(span.Attributes())

	for k, v := range resourceAttrs {
		if _, exists := attrs[k]; !exists {
			attrs[k] = v
		}
	}

	conversationID := getStringAttr(attrs, AttrConversationID)
	if conversationID == "" {
		conversationID = span.TraceID().String()
	}

	operationType := getStringAttr(attrs, AttrOperationName)
	if operationType == "" {
		operationType = "unknown"
	}

	status := "ok"
	if span.Status().Code() == ptrace.StatusCodeError {
		status = "error"
	}

	var parentSpanID string
	if !span.ParentSpanID().IsEmpty() {
		parentSpanID = span.ParentSpanID().String()
	}

	event := &S2Event{
		Timestamp:      span.StartTimestamp().AsTime(),
		TraceID:        span.TraceID().String(),
		SpanID:         span.SpanID().String(),
		ParentSpanID:   parentSpanID,
		ConversationID: conversationID,
		OperationType:  operationType,
		SpanName:       span.Name(),
		Duration:       time.Duration(span.EndTimestamp() - span.StartTimestamp()),
		Status:         status,
		Attributes:     filterGenAIAttributes(attrs),
	}

	return event
}

func extractAttributes(attrs pcommon.Map) map[string]interface{} {
	result := make(map[string]interface{})
	attrs.Range(func(k string, v pcommon.Value) bool {
		result[k] = convertValue(v)
		return true
	})
	return result
}

func convertValue(v pcommon.Value) interface{} {
	switch v.Type() {
	case pcommon.ValueTypeStr:
		return v.Str()
	case pcommon.ValueTypeInt:
		return v.Int()
	case pcommon.ValueTypeDouble:
		return v.Double()
	case pcommon.ValueTypeBool:
		return v.Bool()
	case pcommon.ValueTypeSlice:
		slice := v.Slice()
		result := make([]interface{}, slice.Len())
		for i := 0; i < slice.Len(); i++ {
			result[i] = convertValue(slice.At(i))
		}
		return result
	case pcommon.ValueTypeMap:
		return extractAttributes(v.Map())
	default:
		return v.AsString()
	}
}

func getStringAttr(attrs map[string]interface{}, key string) string {
	if v, ok := attrs[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func filterGenAIAttributes(attrs map[string]interface{}) map[string]interface{} {
	filtered := make(map[string]interface{})
	genAIKeys := []string{
		AttrConversationID, AttrAgentID, AttrAgentName, AttrOperationName,
		AttrToolName, AttrToolCallID, AttrToolCallArgs, AttrToolCallResult,
		AttrInputMessages, AttrOutputMessages, AttrSystemPrompt,
		AttrProviderName, AttrRequestModel, AttrResponseModel,
		AttrInputTokens, AttrOutputTokens,
	}

	for _, key := range genAIKeys {
		if v, ok := attrs[key]; ok {
			filtered[key] = v
		}
	}

	return filtered
}

func (e *S2Event) ToJSON() ([]byte, error) {
	return json.Marshal(e)
}


