# AI Agent Observability with S2.dev Streams

## Architecture Overview

```
                              
                            S2 Stream       User reads real-time
                               (per session)          (stream_reader)
                             
                 
   User      Agent              
         (SDK)              
                             
                                         
                          OTLP spans     
                                         
                           
                    Collector        OLAP      
                     (OTel)               (ClickHouse)  
                           
```

### Data Flow

1. **Real-time (S2)**: Agent SDK writes events directly to S2 stream for immediate visibility
2. **Analytics (OLAP)**: Agent SDK emits OTLP spans → Collector → ClickHouse for historical analysis

### Components

1. **Agent SDK** (Go/Python) - Dual-write library for agent instrumentation
   - Creates S2 stream per session
   - Writes events to S2 in real-time (sub-second latency)
   - Emits OTLP spans for collector

2. **S2.dev Streams** - Real-time event log per agent session
   - User can tail the stream to watch agent activity live
   - Events: session start/end, agent invocations, tool calls, LLM calls

3. **OTel Collector** - Receives OTLP spans, exports to OLAP
   - Standard OTLP receiver
   - Batching, memory limiting
   - ClickHouse exporter for analytics

4. **OLAP Store (ClickHouse)** - Historical trace storage
   - Query past agent sessions
   - Build dashboards, analytics
   - Long-term retention

## Implementation

### 1. Agent SDK

**Go SDK** (`agentsdk/go/`)
- `config.go` - S2 and OTLP configuration
- `session.go` - Session management, agent invocations, tool/LLM calls
- `events.go` - Event types for S2 streaming
- `s2client.go` - S2.dev API client with stream reader
- `tracing.go` - OTLP tracer setup

**Python SDK** (`agentsdk/python/`)
- `config.py` - Configuration dataclass
- `session.py` - Session, AgentInvocation, ToolCall, LLMCall classes
- `events.py` - Event types and serialization
- `s2client.py` - S2.dev client and StreamReader

### 2. S2.dev Exporter (Optional)

If you want the collector to also write to S2 (instead of agent direct-write):

**Files:** `s2exporter/`
- `config.go` - Endpoint, API key, stream prefix, batching
- `factory.go` - OTel exporter factory
- `s2exporter.go` - Buffering per conversation, batch flush
- `event_converter.go` - OTel span → S2 event conversion
- `s2client.go` - S2.dev API client

### 3. Collector Configuration

```yaml
receivers:
  otlp:
    protocols:
      grpc:
        endpoint: 0.0.0.0:4317
      http:
        endpoint: 0.0.0.0:4318

exporters:
  clickhouse:
    endpoint: tcp://localhost:9000
    database: agent_traces

processors:
  batch:
    timeout: 5s
    send_batch_size: 1000

service:
  pipelines:
    traces:
      receivers: [otlp]
      processors: [batch]
      exporters: [clickhouse]
```

### 4. Stream Reader

Real-time CLI to watch agent sessions:

```bash
python stream_reader/main.py agent-session-abc123 --api-key $S2_API_KEY
```

Output:
```
[14:23:01.234]  Session started - Agent: WeatherBot
[14:23:01.456]  Agent invoked: "What's the weather like?"
[14:23:01.567]  Tool call: get_weather - Args: {"location": "SF"}
[14:23:01.678]     Result: {"temperature": 72, "condition": "sunny"} (111ms)
[14:23:01.890]  LLM call: openai / gpt-4
[14:23:02.234]     Tokens: 150 in / 50 out (344ms)
[14:23:02.345]  Agent responded: "It's 72°F and sunny in SF" (889ms)
```

## GenAI Semantic Conventions

Uses OTel GenAI semconv for interoperability:

| Attribute | Description |
|-----------|-------------|
| `gen_ai.conversation.id` | Session identifier |
| `gen_ai.agent.id`, `gen_ai.agent.name` | Agent identification |
| `gen_ai.operation.name` | `invoke_agent`, `execute_tool`, `chat` |
| `gen_ai.tool.name`, `gen_ai.tool.call.id` | Tool identification |
| `gen_ai.tool.call.arguments`, `gen_ai.tool.call.result` | Tool I/O |
| `gen_ai.provider.name`, `gen_ai.request.model` | LLM details |
| `gen_ai.usage.input_tokens`, `gen_ai.usage.output_tokens` | Token usage |

## Usage

### Agent Code (Python)

```python
from agentsdk import Session, Config

config = Config(
    s2_endpoint="https://api.s2.dev",
    s2_api_key="...",
    otlp_endpoint="localhost:4317",
)

with Session(config, agent_name="MyBot") as session:
    print(f"Watch live: {session.stream_name}")
    
    with session.invoke(user_input) as inv:
        with inv.tool_call("search", {"query": "..."}) as tc:
            result = do_search(...)
            tc.end(result)
        
        with inv.llm_call("openai", "gpt-4") as llm:
            response = call_llm(...)
            llm.end(response, input_tokens=100, output_tokens=50)
        
        inv.end(response)
```

### Watch Real-time

```bash
python examples/stream_reader/main.py agent-session-$SESSION_ID --api-key $S2_API_KEY
```

### Query Historical (ClickHouse)

```sql
SELECT 
  JSONExtractString(body, 'gen_ai.agent.name') as agent,
  count() as invocations,
  avg(duration_ns / 1e6) as avg_duration_ms
FROM otel_traces
WHERE timestamp > now() - interval 1 day
GROUP BY agent
```

## Project Structure

```
.
 agentsdk/
    go/                    # Go SDK
       config.go
       session.go
       events.go
       s2client.go
       tracing.go
    python/                # Python SDK
        agentsdk/
            config.py
            session.py
            events.py
            s2client.py
 s2exporter/                # OTel S2 exporter (optional)
 otelcol-agent/             # Custom collector
 examples/
    go_agent/              # Go agent example
    python_agent/          # Python agent example
    stream_reader/         # Real-time viewer
 config.yaml                # Collector config
 README.md
```

## Key Design Decisions

1. **Dual-write from SDK**: Agent writes to S2 directly for lowest latency real-time viewing
2. **OTLP for analytics**: Standard protocol allows any OLAP backend
3. **S2 as event log**: Ordered, append-only stream per session - perfect for "watch agent" UX
4. **GenAI semconv**: Standard attributes for tool interop
5. **SDK handles complexity**: Simple API for agent developers, dual-write hidden internally
