import json
import threading
import uuid
from datetime import datetime
from typing import Any, Dict, Optional
from contextlib import contextmanager

from opentelemetry import trace
from opentelemetry.sdk.trace import TracerProvider
from opentelemetry.sdk.trace.export import BatchSpanProcessor
from opentelemetry.exporter.otlp.proto.grpc.trace_exporter import OTLPSpanExporter
from opentelemetry.sdk.resources import Resource

from .config import Config
from .events import Event, EventType
from .s2client import S2Client


class Session:
    def __init__(
        self,
        config: Config,
        agent_id: Optional[str] = None,
        agent_name: Optional[str] = None,
    ):
        config.validate()
        self.config = config
        self.id = str(uuid.uuid4())
        self.agent_id = agent_id or ""
        self.agent_name = agent_name or ""
        self.start_time = datetime.now()
        
        self._lock = threading.Lock()
        self._event_seq = 0
        self._s2_client: Optional[S2Client] = None
        self._tracer: Optional[trace.Tracer] = None
        
        self._setup_tracing()
        self._setup_s2()
        
        self._emit_event(Event(
            type=EventType.SESSION_START,
            timestamp=self.start_time,
            data={
                "agent_id": self.agent_id,
                "agent_name": self.agent_name,
            },
        ))
    
    @property
    def stream_name(self) -> str:
        return f"{self.config.s2_stream_prefix}{self.id}"
    
    def _setup_tracing(self) -> None:
        if not self.config.otlp_endpoint:
            return
        
        resource = Resource.create({
            "service.name": self.config.service_name,
            "service.version": self.config.service_version,
        })
        
        provider = TracerProvider(resource=resource)
        exporter = OTLPSpanExporter(
            endpoint=self.config.otlp_endpoint,
            insecure=self.config.otlp_insecure,
        )
        provider.add_span_processor(BatchSpanProcessor(exporter))
        trace.set_tracer_provider(provider)
        
        self._tracer = trace.get_tracer("agentsdk")
    
    def _setup_s2(self) -> None:
        if not self.config.s2_endpoint or not self.config.s2_api_key:
            return
        
        self._s2_client = S2Client(self.config.s2_endpoint, self.config.s2_api_key)
        self._s2_client.create_stream(self.stream_name)
    
    def _next_seq(self) -> int:
        with self._lock:
            self._event_seq += 1
            return self._event_seq
    
    def _emit_event(self, event: Event) -> None:
        event.sequence = self._next_seq()
        event.session_id = self.id
        
        if self._s2_client:
            threading.Thread(
                target=self._s2_client.append_event,
                args=(self.stream_name, event),
                daemon=True,
            ).start()
    
    def close(self) -> None:
        self._emit_event(Event(
            type=EventType.SESSION_END,
            timestamp=datetime.now(),
            data={
                "duration_ms": int((datetime.now() - self.start_time).total_seconds() * 1000),
            },
        ))
        
        if self._s2_client:
            self._s2_client.close()
    
    def __enter__(self) -> "Session":
        return self
    
    def __exit__(self, exc_type, exc_val, exc_tb) -> None:
        self.close()
    
    @contextmanager
    def invoke(self, input_text: str):
        inv = AgentInvocation(self, input_text)
        try:
            yield inv
        finally:
            pass


class AgentInvocation:
    def __init__(self, session: Session, input_text: str):
        self.id = str(uuid.uuid4())
        self.session = session
        self.input = input_text
        self.start_time = datetime.now()
        self._span = None
        
        if session._tracer:
            self._span = session._tracer.start_span("agent.invoke")
            self._span.set_attribute("gen_ai.conversation.id", session.id)
            self._span.set_attribute("gen_ai.agent.id", session.agent_id)
            self._span.set_attribute("gen_ai.agent.name", session.agent_name)
            self._span.set_attribute("gen_ai.operation.name", "invoke_agent")
            self._span.set_attribute("gen_ai.input.messages", json.dumps([
                {"role": "user", "content": input_text}
            ]))
        
        session._emit_event(Event(
            type=EventType.AGENT_START,
            timestamp=self.start_time,
            data={
                "invocation_id": self.id,
                "input": input_text,
            },
        ))
    
    def end(self, output: str) -> None:
        if self._span:
            self._span.set_attribute("gen_ai.output.messages", json.dumps([
                {"role": "assistant", "content": output}
            ]))
            self._span.end()
        
        self.session._emit_event(Event(
            type=EventType.AGENT_END,
            timestamp=datetime.now(),
            data={
                "invocation_id": self.id,
                "output": output,
                "duration_ms": int((datetime.now() - self.start_time).total_seconds() * 1000),
            },
        ))
    
    @contextmanager
    def tool_call(self, tool_name: str, arguments: Dict[str, Any]):
        tc = ToolCall(self, tool_name, arguments)
        try:
            yield tc
        finally:
            pass
    
    @contextmanager
    def llm_call(self, provider: str, model: str):
        llm = LLMCall(self, provider, model)
        try:
            yield llm
        finally:
            pass


class ToolCall:
    def __init__(self, invocation: AgentInvocation, tool_name: str, arguments: Dict[str, Any]):
        self.id = str(uuid.uuid4())
        self.invocation = invocation
        self.name = tool_name
        self.arguments = arguments
        self.start_time = datetime.now()
        self._span = None
        
        if invocation.session._tracer:
            self._span = invocation.session._tracer.start_span(f"tool.{tool_name}")
            self._span.set_attribute("gen_ai.conversation.id", invocation.session.id)
            self._span.set_attribute("gen_ai.operation.name", "execute_tool")
            self._span.set_attribute("gen_ai.tool.name", tool_name)
            self._span.set_attribute("gen_ai.tool.call.id", self.id)
            self._span.set_attribute("gen_ai.tool.call.arguments", json.dumps(arguments))
        
        invocation.session._emit_event(Event(
            type=EventType.TOOL_START,
            timestamp=self.start_time,
            data={
                "invocation_id": invocation.id,
                "tool_call_id": self.id,
                "tool_name": tool_name,
                "arguments": arguments,
            },
        ))
    
    def end(self, result: Any, error: Optional[Exception] = None) -> None:
        if self._span:
            self._span.set_attribute("gen_ai.tool.call.result", json.dumps(result))
            if error:
                self._span.record_exception(error)
            self._span.end()
        
        status = "error" if error else "success"
        self.invocation.session._emit_event(Event(
            type=EventType.TOOL_END,
            timestamp=datetime.now(),
            data={
                "invocation_id": self.invocation.id,
                "tool_call_id": self.id,
                "tool_name": self.name,
                "result": result,
                "status": status,
                "duration_ms": int((datetime.now() - self.start_time).total_seconds() * 1000),
            },
        ))


class LLMCall:
    def __init__(self, invocation: AgentInvocation, provider: str, model: str):
        self.id = str(uuid.uuid4())
        self.invocation = invocation
        self.provider = provider
        self.model = model
        self.start_time = datetime.now()
        self._span = None
        
        if invocation.session._tracer:
            self._span = invocation.session._tracer.start_span("llm.generate")
            self._span.set_attribute("gen_ai.conversation.id", invocation.session.id)
            self._span.set_attribute("gen_ai.operation.name", "chat")
            self._span.set_attribute("gen_ai.provider.name", provider)
            self._span.set_attribute("gen_ai.request.model", model)
        
        invocation.session._emit_event(Event(
            type=EventType.LLM_START,
            timestamp=self.start_time,
            data={
                "invocation_id": invocation.id,
                "llm_call_id": self.id,
                "provider": provider,
                "model": model,
            },
        ))
    
    def end(self, response: str, input_tokens: int = 0, output_tokens: int = 0) -> None:
        if self._span:
            self._span.set_attribute("gen_ai.usage.input_tokens", input_tokens)
            self._span.set_attribute("gen_ai.usage.output_tokens", output_tokens)
            self._span.end()
        
        self.invocation.session._emit_event(Event(
            type=EventType.LLM_END,
            timestamp=datetime.now(),
            data={
                "invocation_id": self.invocation.id,
                "llm_call_id": self.id,
                "input_tokens": input_tokens,
                "output_tokens": output_tokens,
                "duration_ms": int((datetime.now() - self.start_time).total_seconds() * 1000),
            },
        ))


