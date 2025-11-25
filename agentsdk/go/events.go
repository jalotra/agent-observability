package agentsdk

import (
	"encoding/json"
	"time"
)

type EventType string

const (
	EventSessionStart EventType = "session.start"
	EventSessionEnd   EventType = "session.end"
	EventAgentStart   EventType = "agent.start"
	EventAgentEnd     EventType = "agent.end"
	EventToolStart    EventType = "tool.start"
	EventToolEnd      EventType = "tool.end"
	EventLLMStart     EventType = "llm.start"
	EventLLMEnd       EventType = "llm.end"
	EventThinking     EventType = "agent.thinking"
	EventCustom       EventType = "custom"
)

type Event struct {
	Type      EventType              `json:"type"`
	Timestamp time.Time              `json:"timestamp"`
	SessionID string                 `json:"session_id"`
	Sequence  int64                  `json:"sequence"`
	Data      map[string]interface{} `json:"data,omitempty"`
}

func (e *Event) ToJSON() ([]byte, error) {
	return json.Marshal(e)
}

func EventFromJSON(data []byte) (*Event, error) {
	var e Event
	if err := json.Unmarshal(data, &e); err != nil {
		return nil, err
	}
	return &e, nil
}

