package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	agentsdk "github.com/agent-observability/agentsdk/go"
)

func getWeather(location string) map[string]interface{} {
	return map[string]interface{}{
		"temperature": 72,
		"condition":   "sunny",
		"location":    location,
	}
}

func webSearch(query string) map[string]interface{} {
	return map[string]interface{}{
		"results": []string{"Result 1", "Result 2", "Result 3"},
	}
}

func simulateLLMResponse(prompt string) (string, int64, int64) {
	time.Sleep(100 * time.Millisecond)
	truncated := prompt
	if len(prompt) > 50 {
		truncated = prompt[:50] + "..."
	}
	return fmt.Sprintf("Response to: %s", truncated), 150, 50
}

type WeatherAgent struct {
	session *agentsdk.Session
}

func (a *WeatherAgent) Run(ctx context.Context, userInput string) string {
	inv, ctx := a.session.StartAgentInvocation(ctx, userInput)

	var prompt string
	lower := strings.ToLower(userInput)

	if strings.Contains(lower, "weather") {
		tc, tcCtx := inv.StartToolCall(ctx, "get_weather", map[string]interface{}{"location": "San Francisco"})
		result := getWeather("San Francisco")
		tc.End(tcCtx, result, nil)
		prompt = fmt.Sprintf("Tool result: %v", result)
	} else if strings.Contains(lower, "search") {
		tc, tcCtx := inv.StartToolCall(ctx, "web_search", map[string]interface{}{"query": userInput})
		result := webSearch(userInput)
		tc.End(tcCtx, result, nil)
		prompt = fmt.Sprintf("Search results: %v", result)
	} else {
		prompt = userInput
	}

	llm, llmCtx := inv.StartLLMCall(ctx, "openai", "gpt-4")
	response, inputTokens, outputTokens := simulateLLMResponse(prompt)
	llm.End(llmCtx, response, inputTokens, outputTokens)

	inv.End(ctx, response)
	return response
}

func main() {
	ctx := context.Background()

	cfg := &agentsdk.Config{
		S2Endpoint:     "https://api.s2.dev",
		S2APIKey:       "your-s2-api-key",
		S2StreamPrefix: "agent-session-",
		OTLPEndpoint:   "localhost:4317",
		OTLPInsecure:   true,
		ServiceName:    "weather-agent",
		ServiceVersion: "1.0.0",
	}

	tp, err := agentsdk.SetupTracing(ctx, cfg)
	if err != nil {
		fmt.Printf("Warning: Failed to setup tracing: %v\n", err)
	}
	if tp != nil {
		defer func() {
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			tp.Shutdown(shutdownCtx)
		}()
	}

	session, err := agentsdk.NewSession(ctx, cfg,
		agentsdk.WithAgentID("agent-001"),
		agentsdk.WithAgentName("WeatherBot"),
	)
	if err != nil {
		fmt.Printf("Failed to create session: %v\n", err)
		return
	}
	defer session.Close(ctx)

	fmt.Printf("Session started: %s\n", session.ID)
	fmt.Printf("S2 Stream: %s\n", session.StreamName())
	fmt.Println()

	agent := &WeatherAgent{session: session}

	messages := []string{
		"Hello, how are you?",
		"What's the weather like?",
		"Search for Go tutorials",
	}

	for _, msg := range messages {
		fmt.Printf("User: %s\n", msg)
		response := agent.Run(ctx, msg)
		fmt.Printf("Agent: %s\n\n", response)
	}

	time.Sleep(1 * time.Second)
	fmt.Println("Session closed.")
}
