package s2exporter

import (
	"errors"
	"time"
)

type Config struct {
	Endpoint      string        `mapstructure:"endpoint"`
	APIKey        string        `mapstructure:"api_key"`
	StreamPrefix  string        `mapstructure:"stream_prefix"`
	BatchSize     int           `mapstructure:"batch_size"`
	FlushInterval time.Duration `mapstructure:"flush_interval"`
}

func (c *Config) Validate() error {
	if c.Endpoint == "" {
		return errors.New("endpoint is required")
	}
	if c.APIKey == "" {
		return errors.New("api_key is required")
	}
	if c.BatchSize <= 0 {
		c.BatchSize = 100
	}
	if c.FlushInterval <= 0 {
		c.FlushInterval = 5 * time.Second
	}
	if c.StreamPrefix == "" {
		c.StreamPrefix = "agent-session-"
	}
	return nil
}

