package main

import (
	"log"

	"go.opentelemetry.io/collector/otelcol"
)

func main() {
	factories, err := components()
	if err != nil {
		log.Fatalf("Failed to build components: %v", err)
	}

	info := otelcol.CollectorSettings{
		Factories: func() (otelcol.Factories, error) {
			return factories, nil
		},
		BuildInfo: otelcol.BuildInfo{
			Command:     "otelcol-agent",
			Description: "AI Agent Observability Collector with S2.dev Exporter",
			Version:     "0.1.0",
		},
	}

	if err := otelcol.NewCommand(info).Execute(); err != nil {
		log.Fatal(err)
	}
}

