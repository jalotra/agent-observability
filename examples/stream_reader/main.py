"""
Real-time S2 Stream Reader - Watch agent activity as it happens.
This is what users run to see what the agent is doing in real-time.
"""

import sys
import time
import argparse
from datetime import datetime

sys.path.insert(0, "../../agentsdk/python")

from agentsdk import S2Client, StreamReader, EventType


def format_event(event) -> str:
    timestamp = event.timestamp.strftime("%H:%M:%S.%f")[:-3]
    
    formatters = {
        EventType.SESSION_START: lambda e: f" Session started - Agent: {e.data.get('agent_name', 'unknown')}",
        EventType.SESSION_END: lambda e: f" Session ended - Duration: {e.data.get('duration_ms', 0)}ms",
        EventType.AGENT_START: lambda e: f" Agent invoked: \"{e.data.get('input', '')[:50]}...\"",
        EventType.AGENT_END: lambda e: f" Agent responded: \"{e.data.get('output', '')[:50]}...\" ({e.data.get('duration_ms', 0)}ms)",
        EventType.TOOL_START: lambda e: f" Tool call: {e.data.get('tool_name', 'unknown')} - Args: {e.data.get('arguments', {})}",
        EventType.TOOL_END: lambda e: f"    Result: {str(e.data.get('result', ''))[:60]}... ({e.data.get('duration_ms', 0)}ms)",
        EventType.LLM_START: lambda e: f" LLM call: {e.data.get('provider', '')} / {e.data.get('model', '')}",
        EventType.LLM_END: lambda e: f"    Tokens: {e.data.get('input_tokens', 0)} in / {e.data.get('output_tokens', 0)} out ({e.data.get('duration_ms', 0)}ms)",
        EventType.THINKING: lambda e: f" Thinking: {e.data.get('thought', '')[:60]}...",
    }
    
    formatter = formatters.get(event.type, lambda e: f" {e.type.value}: {e.data}")
    return f"[{timestamp}] {formatter(event)}"


def watch_stream(stream_name: str, s2_endpoint: str, s2_api_key: str):
    client = S2Client(s2_endpoint, s2_api_key)
    reader = StreamReader(client, stream_name)
    
    print(f" Watching stream: {stream_name}")
    print(f"   Press Ctrl+C to stop\n")
    print("-" * 60)
    
    try:
        while True:
            events = reader.read_events()
            
            for event in events:
                print(format_event(event))
            
            if not events:
                time.sleep(0.5)
            
    except KeyboardInterrupt:
        print("\n" + "-" * 60)
        print(" Stopped watching")
    finally:
        client.close()


def main():
    parser = argparse.ArgumentParser(description="Watch an agent session in real-time")
    parser.add_argument("stream_name", help="Name of the S2 stream to watch (e.g., agent-session-abc123)")
    parser.add_argument("--endpoint", default="https://api.s2.dev", help="S2 API endpoint")
    parser.add_argument("--api-key", required=True, help="S2 API key")
    
    args = parser.parse_args()
    
    watch_stream(args.stream_name, args.endpoint, args.api_key)


if __name__ == "__main__":
    main()


