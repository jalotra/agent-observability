import json
from dataclasses import dataclass, field
from datetime import datetime
from enum import Enum
from typing import Any, Dict, Optional


class EventType(str, Enum):
    SESSION_START = "session.start"
    SESSION_END = "session.end"
    AGENT_START = "agent.start"
    AGENT_END = "agent.end"
    TOOL_START = "tool.start"
    TOOL_END = "tool.end"
    LLM_START = "llm.start"
    LLM_END = "llm.end"
    THINKING = "agent.thinking"
    CUSTOM = "custom"


@dataclass
class Event:
    type: EventType
    timestamp: datetime
    session_id: str = ""
    sequence: int = 0
    data: Dict[str, Any] = field(default_factory=dict)
    
    def to_json(self) -> str:
        return json.dumps({
            "type": self.type.value,
            "timestamp": self.timestamp.isoformat(),
            "session_id": self.session_id,
            "sequence": self.sequence,
            "data": self.data,
        })
    
    @classmethod
    def from_json(cls, data: str) -> "Event":
        obj = json.loads(data)
        return cls(
            type=EventType(obj["type"]),
            timestamp=datetime.fromisoformat(obj["timestamp"]),
            session_id=obj.get("session_id", ""),
            sequence=obj.get("sequence", 0),
            data=obj.get("data", {}),
        )


