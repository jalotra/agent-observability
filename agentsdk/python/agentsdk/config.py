from dataclasses import dataclass, field
from typing import Optional


@dataclass
class Config:
    s2_endpoint: str = "https://api.s2.dev"
    s2_api_key: Optional[str] = None
    s2_stream_prefix: str = "agent-session-"
    
    otlp_endpoint: str = "localhost:4317"
    otlp_insecure: bool = True
    
    service_name: str = "agent"
    service_version: str = "1.0.0"
    
    def validate(self) -> None:
        if not self.s2_endpoint and not self.otlp_endpoint:
            raise ValueError("At least one of s2_endpoint or otlp_endpoint must be set")

