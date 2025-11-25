package agentsdk

import (
	"errors"
)

type Config struct {
	S2Endpoint     string
	S2APIKey       string
	S2StreamPrefix string

	OTLPEndpoint string
	OTLPInsecure bool

	ServiceName    string
	ServiceVersion string
}

func (c *Config) Validate() error {
	if c.S2Endpoint == "" && c.OTLPEndpoint == "" {
		return errors.New("at least one of S2Endpoint or OTLPEndpoint must be set")
	}
	if c.S2StreamPrefix == "" {
		c.S2StreamPrefix = "agent-session-"
	}
	if c.ServiceName == "" {
		c.ServiceName = "agent"
	}
	return nil
}

func DefaultConfig() *Config {
	return &Config{
		S2Endpoint:     "https://api.s2.dev",
		S2StreamPrefix: "agent-session-",
		OTLPEndpoint:   "localhost:4317",
		OTLPInsecure:   true,
		ServiceName:    "agent",
		ServiceVersion: "1.0.0",
	}
}

