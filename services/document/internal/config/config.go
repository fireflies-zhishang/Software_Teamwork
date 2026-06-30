package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	DefaultHTTPAddr        = ":8085"
	DefaultShutdownTimeout = 10 * time.Second
	DefaultPandocPath      = "pandoc"
	DefaultLibreOfficePath = "soffice"
)

type Config struct {
	HTTPAddr              string
	DatabaseURL           string
	RedisAddr             string
	FileServiceURL        string
	FileServiceToken      string
	AIGatewayURL          string
	AIGatewayProfileID    string
	AIGatewayServiceToken string
	KnowledgeServiceURL   string
	KnowledgeServiceToken string
	PandocPath            string
	LibreOfficePath       string
	ShutdownTimeout       time.Duration
}

func Load() (Config, error) {
	cfg := Config{
		HTTPAddr:              envOr("DOCUMENT_HTTP_ADDR", DefaultHTTPAddr),
		DatabaseURL:           strings.TrimSpace(os.Getenv("DOCUMENT_DATABASE_URL")),
		RedisAddr:             strings.TrimSpace(os.Getenv("DOCUMENT_REDIS_ADDR")),
		FileServiceURL:        strings.TrimSpace(os.Getenv("DOCUMENT_FILE_SERVICE_URL")),
		FileServiceToken:      firstEnv("DOCUMENT_FILE_SERVICE_TOKEN", "INTERNAL_SERVICE_TOKEN"),
		AIGatewayURL:          strings.TrimSpace(os.Getenv("DOCUMENT_AI_GATEWAY_URL")),
		AIGatewayProfileID:    strings.TrimSpace(os.Getenv("DOCUMENT_AI_GATEWAY_PROFILE_ID")),
		AIGatewayServiceToken: firstEnv("DOCUMENT_AI_GATEWAY_SERVICE_TOKEN", "INTERNAL_SERVICE_TOKEN"),
		KnowledgeServiceURL:   strings.TrimSpace(os.Getenv("DOCUMENT_KNOWLEDGE_SERVICE_URL")),
		KnowledgeServiceToken: firstEnv("DOCUMENT_KNOWLEDGE_SERVICE_TOKEN", "INTERNAL_SERVICE_TOKEN"),
		PandocPath:            envOr("DOCUMENT_PANDOC_PATH", DefaultPandocPath),
		LibreOfficePath:       envOr("DOCUMENT_LIBREOFFICE_PATH", DefaultLibreOfficePath),
		ShutdownTimeout:       DefaultShutdownTimeout,
	}

	if raw := strings.TrimSpace(os.Getenv("DOCUMENT_SHUTDOWN_TIMEOUT")); raw != "" {
		value, err := time.ParseDuration(raw)
		if err != nil || value <= 0 {
			return Config{}, errors.New("DOCUMENT_SHUTDOWN_TIMEOUT must be a positive duration")
		}
		cfg.ShutdownTimeout = value
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c Config) Validate() error {
	if strings.TrimSpace(c.HTTPAddr) == "" {
		return errors.New("DOCUMENT_HTTP_ADDR is required")
	}
	if strings.TrimSpace(c.DatabaseURL) == "" {
		return errors.New("DOCUMENT_DATABASE_URL is required")
	}
	if strings.TrimSpace(c.RedisAddr) == "" {
		return errors.New("DOCUMENT_REDIS_ADDR is required")
	}
	if err := validateHTTPURL("DOCUMENT_FILE_SERVICE_URL", c.FileServiceURL); err != nil {
		return err
	}
	if err := validateHTTPURL("DOCUMENT_AI_GATEWAY_URL", c.AIGatewayURL); err != nil {
		return err
	}
	if strings.TrimSpace(c.AIGatewayProfileID) == "" {
		return errors.New("DOCUMENT_AI_GATEWAY_PROFILE_ID is required")
	}
	if strings.TrimSpace(c.KnowledgeServiceURL) != "" {
		if err := validateHTTPURL("DOCUMENT_KNOWLEDGE_SERVICE_URL", c.KnowledgeServiceURL); err != nil {
			return err
		}
	}
	if strings.TrimSpace(c.PandocPath) == "" {
		return errors.New("DOCUMENT_PANDOC_PATH is required")
	}
	if strings.TrimSpace(c.LibreOfficePath) == "" {
		return errors.New("DOCUMENT_LIBREOFFICE_PATH is required")
	}
	if c.ShutdownTimeout <= 0 {
		return errors.New("DOCUMENT_SHUTDOWN_TIMEOUT must be a positive duration")
	}
	return nil
}

func validateHTTPURL(name, value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s is required", name)
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return fmt.Errorf("%s must be an absolute http(s) URL", name)
	}
	if parsed.User != nil {
		return fmt.Errorf("%s must not contain credentials", name)
	}
	return nil
}

func envOr(key string, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func firstEnv(keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value
		}
	}
	return ""
}
