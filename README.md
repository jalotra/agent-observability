# AI Agent Observability

Real-time and historical observability for AI agents using S2.dev streams and OpenTelemetry.

## Architecture
![HLD](./docs/AgentHLD.png)

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
 agentsdk/
    go/              # Go SDK
    python/          # Python SDK
 s2exporter/          # OTel S2 exporter (optional)
 otelcol-agent/       # Custom collector
 examples/
    go_agent/
    python_agent/
    stream_reader/   # Real-time viewer
 config.yaml          # Collector config
```

## License

MIT
