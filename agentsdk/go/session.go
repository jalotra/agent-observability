package agentsdk

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

type Session struct {
	ID        string
	AgentID   string
	AgentName string
	StartTime time.Time

	tracer   trace.Tracer
	s2Client *S2Client
	config   *Config

	mu       sync.Mutex
	eventSeq int64
}

type SessionOption func(*Session)

func WithAgentID(id string) SessionOption {
	return func(s *Session) {
		s.AgentID = id
	}
}

func WithAgentName(name string) SessionOption {
	return func(s *Session) {
		s.AgentName = name
	}
}

func NewSession(ctx context.Context, cfg *Config, opts ...SessionOption) (*Session, error) {
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	session := &Session{
		ID:        uuid.New().String(),
		StartTime: time.Now(),
		tracer:    otel.Tracer("agentsdk"),
		config:    cfg,
	}

	for _, opt := range opts {
		opt(session)
	}

	if cfg.S2Endpoint != "" && cfg.S2APIKey != "" {
		session.s2Client = NewS2Client(cfg.S2Endpoint, cfg.S2APIKey)
		streamName := cfg.S2StreamPrefix + session.ID
		if err := session.s2Client.CreateStream(ctx, streamName); err != nil {
			return nil, fmt.Errorf("failed to create S2 stream: %w", err)
		}
	}

	session.emitEvent(ctx, &Event{
		Type:      EventSessionStart,
		Timestamp: session.StartTime,
		Data: map[string]interface{}{
			"agent_id":   session.AgentID,
			"agent_name": session.AgentName,
		},
	})

	return session, nil
}

func (s *Session) StreamName() string {
	return s.config.S2StreamPrefix + s.ID
}

func (s *Session) Close(ctx context.Context) error {
	s.emitEvent(ctx, &Event{
		Type:      EventSessionEnd,
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"duration_ms": time.Since(s.StartTime).Milliseconds(),
		},
	})
	return nil
}

func (s *Session) emitEvent(ctx context.Context, event *Event) {
	s.mu.Lock()
	s.eventSeq++
	event.Sequence = s.eventSeq
	event.SessionID = s.ID
	s.mu.Unlock()

	if s.s2Client != nil {
		go func() {
			streamName := s.config.S2StreamPrefix + s.ID
			_ = s.s2Client.AppendEvent(context.Background(), streamName, event)
		}()
	}
}

func (s *Session) StartAgentInvocation(ctx context.Context, input string) (*AgentInvocation, context.Context) {
	ctx, span := s.tracer.Start(ctx, "agent.invoke")
	span.SetAttributes(
		attribute.String("gen_ai.conversation.id", s.ID),
		attribute.String("gen_ai.agent.id", s.AgentID),
		attribute.String("gen_ai.agent.name", s.AgentName),
		attribute.String("gen_ai.operation.name", "invoke_agent"),
	)

	inputMsgs, _ := json.Marshal([]map[string]string{{"role": "user", "content": input}})
	span.SetAttributes(attribute.String("gen_ai.input.messages", string(inputMsgs)))

	inv := &AgentInvocation{
		ID:        uuid.New().String(),
		session:   s,
		span:      span,
		input:     input,
		startTime: time.Now(),
	}

	s.emitEvent(ctx, &Event{
		Type:      EventAgentStart,
		Timestamp: inv.startTime,
		Data: map[string]interface{}{
			"invocation_id": inv.ID,
			"input":         input,
		},
	})

	return inv, ctx
}

type AgentInvocation struct {
	ID        string
	session   *Session
	span      trace.Span
	input     string
	startTime time.Time
}

func (inv *AgentInvocation) End(ctx context.Context, output string) {
	outputMsgs, _ := json.Marshal([]map[string]string{{"role": "assistant", "content": output}})
	inv.span.SetAttributes(attribute.String("gen_ai.output.messages", string(outputMsgs)))
	inv.span.End()

	inv.session.emitEvent(ctx, &Event{
		Type:      EventAgentEnd,
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"invocation_id": inv.ID,
			"output":        output,
			"duration_ms":   time.Since(inv.startTime).Milliseconds(),
		},
	})
}

func (inv *AgentInvocation) StartToolCall(ctx context.Context, toolName string, args map[string]interface{}) (*ToolCall, context.Context) {
	ctx, span := inv.session.tracer.Start(ctx, fmt.Sprintf("tool.%s", toolName))

	argsJSON, _ := json.Marshal(args)
	toolCallID := uuid.New().String()

	span.SetAttributes(
		attribute.String("gen_ai.conversation.id", inv.session.ID),
		attribute.String("gen_ai.operation.name", "execute_tool"),
		attribute.String("gen_ai.tool.name", toolName),
		attribute.String("gen_ai.tool.call.id", toolCallID),
		attribute.String("gen_ai.tool.call.arguments", string(argsJSON)),
	)

	tc := &ToolCall{
		ID:         toolCallID,
		Name:       toolName,
		Args:       args,
		invocation: inv,
		span:       span,
		startTime:  time.Now(),
	}

	inv.session.emitEvent(ctx, &Event{
		Type:      EventToolStart,
		Timestamp: tc.startTime,
		Data: map[string]interface{}{
			"invocation_id": inv.ID,
			"tool_call_id":  tc.ID,
			"tool_name":     toolName,
			"arguments":     args,
		},
	})

	return tc, ctx
}

type ToolCall struct {
	ID         string
	Name       string
	Args       map[string]interface{}
	invocation *AgentInvocation
	span       trace.Span
	startTime  time.Time
}

func (tc *ToolCall) End(ctx context.Context, result interface{}, err error) {
	resultJSON, _ := json.Marshal(result)
	tc.span.SetAttributes(attribute.String("gen_ai.tool.call.result", string(resultJSON)))

	status := "success"
	if err != nil {
		status = "error"
		tc.span.RecordError(err)
	}
	tc.span.End()

	tc.invocation.session.emitEvent(ctx, &Event{
		Type:      EventToolEnd,
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"invocation_id": tc.invocation.ID,
			"tool_call_id":  tc.ID,
			"tool_name":     tc.Name,
			"result":        result,
			"status":        status,
			"duration_ms":   time.Since(tc.startTime).Milliseconds(),
		},
	})
}

func (inv *AgentInvocation) StartLLMCall(ctx context.Context, provider, model string) (*LLMCall, context.Context) {
	ctx, span := inv.session.tracer.Start(ctx, "llm.generate")

	span.SetAttributes(
		attribute.String("gen_ai.conversation.id", inv.session.ID),
		attribute.String("gen_ai.operation.name", "chat"),
		attribute.String("gen_ai.provider.name", provider),
		attribute.String("gen_ai.request.model", model),
	)

	llm := &LLMCall{
		ID:         uuid.New().String(),
		Provider:   provider,
		Model:      model,
		invocation: inv,
		span:       span,
		startTime:  time.Now(),
	}

	inv.session.emitEvent(ctx, &Event{
		Type:      EventLLMStart,
		Timestamp: llm.startTime,
		Data: map[string]interface{}{
			"invocation_id": inv.ID,
			"llm_call_id":   llm.ID,
			"provider":      provider,
			"model":         model,
		},
	})

	return llm, ctx
}

type LLMCall struct {
	ID         string
	Provider   string
	Model      string
	invocation *AgentInvocation
	span       trace.Span
	startTime  time.Time
}

func (llm *LLMCall) End(ctx context.Context, response string, inputTokens, outputTokens int64) {
	llm.span.SetAttributes(
		attribute.Int64("gen_ai.usage.input_tokens", inputTokens),
		attribute.Int64("gen_ai.usage.output_tokens", outputTokens),
	)
	llm.span.End()

	llm.invocation.session.emitEvent(ctx, &Event{
		Type:      EventLLMEnd,
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"invocation_id": llm.invocation.ID,
			"llm_call_id":   llm.ID,
			"input_tokens":  inputTokens,
			"output_tokens": outputTokens,
			"duration_ms":   time.Since(llm.startTime).Milliseconds(),
		},
	})
}


