package main

import (
	"os"

	"gopkg.in/yaml.v3"
)

type Route struct {
	Prefix string `yaml:"prefix"`
	Target string `yaml:"target"`
}

type Config struct {
	Port   int     `yaml:"port"`
	Routes []Route `yaml:"routes"`
}

func loadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	if cfg.Port == 0 {
		cfg.Port = 8080
	}
	return &cfg, nil
}
