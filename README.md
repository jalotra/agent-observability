# AI Agent Observability

Real-time and historical observability for AI agents using S2.dev streams and OpenTelemetry.

## Architecture

```
                              ┌──────────────────┐
                         ┌───▶│   S2 Stream      │◀──── User watches real-time
                         │    │  (per session)   │
                         │    └──────────────────┘
┌──────────┐      ┌──────┴────┐           
│   User   │─────▶│   Agent   │           
└──────────┘      │   (SDK)   │           
                  └──────┬────┘           
                         │ OTLP           
                         ▼                
                  ┌──────────────┐         ┌───────────────┐
                  │  Collector   │────────▶│     OLAP      │
                  └──────────────┘         └───────────────┘
```

**Real-time**: Agent writes events directly to S2 stream → User tails stream to watch agent live  
**Analytics**: Agent emits OTLP spans → Collector → ClickHouse for historical queries

## Quick Start

### 1. Install the SDK

**Python:**
```bash
cd agentsdk/python
pip install -e .
```

**Go:**
```bash
go get github.com/agent-observability/agentsdk
```

### 2. Instrument Your Agent

**Python:**
```python
from agentsdk import Session, Config

config = Config(
    s2_endpoint="https://api.s2.dev",
    s2_api_key="your-key",
    otlp_endpoint="localhost:4317",
)

with Session(config, agent_name="MyBot") as session:
    print(f"Stream: {session.stream_name}")
    
    with session.invoke(user_input) as inv:
        # Tool call
        with inv.tool_call("search", {"query": q}) as tc:
            result = do_search(q)
            tc.end(result)
        
        # LLM call
        with inv.llm_call("openai", "gpt-4") as llm:
            response = call_llm(prompt)
            llm.end(response, input_tokens=100, output_tokens=50)
        
        inv.end(response)
```

**Go:**
```go
cfg := &agentsdk.Config{
    S2Endpoint:   "https://api.s2.dev",
    S2APIKey:     "your-key",
    OTLPEndpoint: "localhost:4317",
}

session, _ := agentsdk.NewSession(ctx, cfg, agentsdk.WithAgentName("MyBot"))
defer session.Close(ctx)

inv, ctx := session.StartAgentInvocation(ctx, userInput)

tc, tcCtx := inv.StartToolCall(ctx, "search", args)
result := doSearch(args)
tc.End(tcCtx, result, nil)

llm, llmCtx := inv.StartLLMCall(ctx, "openai", "gpt-4")
response := callLLM(prompt)
llm.End(llmCtx, response, 100, 50)

inv.End(ctx, response)
```

### 3. Watch Real-time

```bash
python examples/stream_reader/main.py agent-session-$ID --api-key $S2_API_KEY
```

Output:
```
[14:23:01.234] 🚀 Session started - Agent: MyBot
[14:23:01.456] 🤖 Agent invoked: "What's the weather?"
[14:23:01.567] 🔧 Tool call: get_weather - Args: {"location": "SF"}
[14:23:01.678]    └─ Result: {"temp": 72} (111ms)
[14:23:01.890] 🧠 LLM call: openai / gpt-4
[14:23:02.234]    └─ Tokens: 150 in / 50 out (344ms)
[14:23:02.345] ✅ Agent responded: "It's 72°F in SF" (889ms)
```

### 4. Run Collector (for OLAP)

```bash
# Build
cd otelcol-agent && go build -o otelcol-agent .

# Run
export CLICKHOUSE_USER=default
./otelcol-agent --config ../config.yaml
```

### 5. Query Historical Data

```sql
SELECT 
  JSONExtractString(body, 'gen_ai.agent.name') as agent,
  count() as calls,
  avg(duration_ns / 1e6) as avg_ms
FROM otel_traces
WHERE timestamp > now() - interval 1 day
GROUP BY agent
```

## SDK Features

| Feature | Description |
|---------|-------------|
| **Session** | Creates S2 stream, manages OTLP tracer |
| **AgentInvocation** | Tracks user→agent→response cycle |
| **ToolCall** | Records tool name, args, result, duration |
| **LLMCall** | Records provider, model, tokens, duration |
| **Dual-write** | S2 for real-time, OTLP for analytics |

## Configuration

**Python:**
```python
Config(
    s2_endpoint="https://api.s2.dev",     # S2 API endpoint
    s2_api_key="...",                      # S2 API key
    s2_stream_prefix="agent-session-",    # Stream name prefix
    otlp_endpoint="localhost:4317",       # Collector endpoint
    otlp_insecure=True,                   # Use insecure gRPC
    service_name="my-agent",              # Service name for traces
)
```

**Go:**
```go
&agentsdk.Config{
    S2Endpoint:     "https://api.s2.dev",
    S2APIKey:       "...",
    S2StreamPrefix: "agent-session-",
    OTLPEndpoint:   "localhost:4317",
    OTLPInsecure:   true,
    ServiceName:    "my-agent",
}
```

## GenAI Semantic Conventions

The SDK uses standard OTel GenAI attributes:

- `gen_ai.conversation.id` - Session/conversation ID
- `gen_ai.agent.id`, `gen_ai.agent.name` - Agent identification
- `gen_ai.operation.name` - `invoke_agent`, `execute_tool`, `chat`
- `gen_ai.tool.name`, `gen_ai.tool.call.id` - Tool identification
- `gen_ai.tool.call.arguments`, `gen_ai.tool.call.result` - Tool I/O
- `gen_ai.provider.name`, `gen_ai.request.model` - LLM details
- `gen_ai.usage.input_tokens`, `gen_ai.usage.output_tokens` - Token usage

## Project Structure

```
├── agentsdk/
│   ├── go/              # Go SDK
│   └── python/          # Python SDK
├── s2exporter/          # OTel S2 exporter (optional)
├── otelcol-agent/       # Custom collector
├── examples/
│   ├── go_agent/
│   ├── python_agent/
│   └── stream_reader/   # Real-time viewer
└── config.yaml          # Collector config
```

## License

MIT
