"""
Example AI Agent using the agentsdk for dual-write observability.
Writes events directly to S2 (real-time) and sends OTLP spans to collector (OLAP).
"""

import sys
import time
sys.path.insert(0, "../../agentsdk/python")

from agentsdk import Session, Config


def get_weather(location: str) -> dict:
    return {"temperature": 72, "condition": "sunny", "location": location}


def web_search(query: str) -> dict:
    return {"results": ["Result 1", "Result 2", "Result 3"]}


def simulate_llm_response(prompt: str) -> tuple[str, int, int]:
    time.sleep(0.1)
    truncated = prompt[:50] + "..." if len(prompt) > 50 else prompt
    return f"Response to: {truncated}", 150, 50


class WeatherAgent:
    def __init__(self, session: Session):
        self.session = session
    
    def run(self, user_input: str) -> str:
        with self.session.invoke(user_input) as inv:
            if "weather" in user_input.lower():
                with inv.tool_call("get_weather", {"location": "San Francisco"}) as tc:
                    result = get_weather("San Francisco")
                    tc.end(result)
                    prompt = f"Tool result: {result}"
            elif "search" in user_input.lower():
                with inv.tool_call("web_search", {"query": user_input}) as tc:
                    result = web_search(user_input)
                    tc.end(result)
                    prompt = f"Search results: {result}"
            else:
                prompt = user_input
            
            with inv.llm_call("openai", "gpt-4") as llm:
                response, input_tokens, output_tokens = simulate_llm_response(prompt)
                llm.end(response, input_tokens, output_tokens)
            
            inv.end(response)
            return response


def main():
    config = Config(
        s2_endpoint="https://api.s2.dev",
        s2_api_key="your-s2-api-key",
        s2_stream_prefix="agent-session-",
        otlp_endpoint="localhost:4317",
        otlp_insecure=True,
        service_name="weather-agent",
        service_version="1.0.0",
    )
    
    with Session(config, agent_id="agent-001", agent_name="WeatherBot") as session:
        print(f"Session started: {session.id}")
        print(f"S2 Stream: {session.stream_name}")
        print()
        
        agent = WeatherAgent(session)
        
        messages = [
            "Hello, how are you?",
            "What's the weather like?",
            "Search for Python tutorials",
        ]
        
        for msg in messages:
            print(f"User: {msg}")
            response = agent.run(msg)
            print(f"Agent: {response}")
            print()
        
        time.sleep(1)
    
    print("Session closed.")


if __name__ == "__main__":
    main()
