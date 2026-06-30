package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	DefaultHTTPAddr          = ":8080"
	DefaultMetricsAddr       = ":9091"
	DefaultServiceVersion    = "0.1.0"
	DefaultEnvironment       = "local"
	DefaultMaxBodyBytes      = int64(10 << 20)
	DefaultRequestTimeout    = 30 * time.Second
	DefaultShutdownTimeout   = 10 * time.Second
	DefaultDownstreamTimeout = 10 * time.Second
	DefaultRedisAddr         = "localhost:6379"
	DefaultTokenHashSecret   = "local-dev-token-hash-secret"
	DefaultTokenKeyVersion   = "v1"
)

type Config struct {
	HTTPAddr             string
	MetricsAddr          string
	ServiceVersion       string
	Environment          string
	MaxBodyBytes         int64
	RequestTimeout       time.Duration
	ShutdownTimeout      time.Duration
	DownstreamTimeout    time.Duration
	CORSAllowedOrigins   []string
	CORSAllowedMethods   []string
	CORSAllowedHeaders   []string
	CORSAllowCredentials bool
	RedisAddr            string
	RedisPassword        string
	RedisDB              int
	TokenHashSecret      string
	TokenHashKeyVersion  string
	InternalServiceToken string
	AuthBaseURL          string
	KnowledgeBaseURL     string
	QABaseURL            string
	DocumentBaseURL      string
	AIGatewayBaseURL     string
}

func Load() (Config, error) {
	cfg := Config{
		HTTPAddr:             stringValue("GATEWAY_HTTP_ADDR", DefaultHTTPAddr),
		MetricsAddr:          stringValue("GATEWAY_METRICS_ADDR", DefaultMetricsAddr),
		ServiceVersion:       stringValue("GATEWAY_SERVICE_VERSION", DefaultServiceVersion),
		Environment:          stringValue("GATEWAY_ENV", DefaultEnvironment),
		MaxBodyBytes:         DefaultMaxBodyBytes,
		RequestTimeout:       DefaultRequestTimeout,
		ShutdownTimeout:      DefaultShutdownTimeout,
		DownstreamTimeout:    DefaultDownstreamTimeout,
		CORSAllowedOrigins:   csvValue("GATEWAY_CORS_ALLOWED_ORIGINS", []string{"*"}),
		CORSAllowedMethods:   csvValue("GATEWAY_CORS_ALLOWED_METHODS", []string{"GET", "POST", "PATCH", "DELETE", "OPTIONS"}),
		CORSAllowedHeaders:   csvValue("GATEWAY_CORS_ALLOWED_HEADERS", []string{"Authorization", "Content-Type", "X-Request-Id"}),
		RedisAddr:            stringValue("GATEWAY_REDIS_ADDR", DefaultRedisAddr),
		RedisPassword:        os.Getenv("GATEWAY_REDIS_PASSWORD"),
		TokenHashSecret:      stringValue("GATEWAY_TOKEN_HASH_SECRET", DefaultTokenHashSecret),
		TokenHashKeyVersion:  stringValue("GATEWAY_TOKEN_HASH_KEY_VERSION", DefaultTokenKeyVersion),
		InternalServiceToken: strings.TrimSpace(os.Getenv("GATEWAY_INTERNAL_SERVICE_TOKEN")),
		AuthBaseURL:          stringValue("GATEWAY_AUTH_BASE_URL", "http://localhost:8001"),
		KnowledgeBaseURL:     strings.TrimSpace(os.Getenv("GATEWAY_KNOWLEDGE_BASE_URL")),
		QABaseURL:            strings.TrimSpace(os.Getenv("GATEWAY_QA_BASE_URL")),
		DocumentBaseURL:      strings.TrimSpace(os.Getenv("GATEWAY_DOCUMENT_BASE_URL")),
		AIGatewayBaseURL:     strings.TrimSpace(os.Getenv("GATEWAY_AI_GATEWAY_BASE_URL")),
	}

	if raw := os.Getenv("GATEWAY_MAX_BODY_BYTES"); raw != "" {
		value, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
		if err != nil || value <= 0 {
			return Config{}, fmt.Errorf("GATEWAY_MAX_BODY_BYTES must be a positive integer")
		}
		cfg.MaxBodyBytes = value
	}

	if raw := os.Getenv("GATEWAY_REQUEST_TIMEOUT"); raw != "" {
		value, err := time.ParseDuration(strings.TrimSpace(raw))
		if err != nil || value <= 0 {
			return Config{}, fmt.Errorf("GATEWAY_REQUEST_TIMEOUT must be a positive duration")
		}
		cfg.RequestTimeout = value
	}

	if raw := os.Getenv("GATEWAY_SHUTDOWN_TIMEOUT"); raw != "" {
		value, err := time.ParseDuration(strings.TrimSpace(raw))
		if err != nil || value <= 0 {
			return Config{}, fmt.Errorf("GATEWAY_SHUTDOWN_TIMEOUT must be a positive duration")
		}
		cfg.ShutdownTimeout = value
	}

	if raw := os.Getenv("GATEWAY_DOWNSTREAM_TIMEOUT"); raw != "" {
		value, err := time.ParseDuration(strings.TrimSpace(raw))
		if err != nil || value <= 0 {
			return Config{}, fmt.Errorf("GATEWAY_DOWNSTREAM_TIMEOUT must be a positive duration")
		}
		cfg.DownstreamTimeout = value
	}

	if raw := os.Getenv("GATEWAY_REDIS_DB"); raw != "" {
		value, err := strconv.Atoi(strings.TrimSpace(raw))
		if err != nil || value < 0 {
			return Config{}, fmt.Errorf("GATEWAY_REDIS_DB must be a non-negative integer")
		}
		cfg.RedisDB = value
	}

	if raw := os.Getenv("GATEWAY_CORS_ALLOW_CREDENTIALS"); raw != "" {
		value, err := strconv.ParseBool(strings.TrimSpace(raw))
		if err != nil {
			return Config{}, fmt.Errorf("GATEWAY_CORS_ALLOW_CREDENTIALS must be a boolean")
		}
		cfg.CORSAllowCredentials = value
	}

	if strings.TrimSpace(cfg.HTTPAddr) == "" {
		return Config{}, fmt.Errorf("GATEWAY_HTTP_ADDR must not be empty")
	}
	if len(cfg.CORSAllowedOrigins) == 0 {
		return Config{}, fmt.Errorf("GATEWAY_CORS_ALLOWED_ORIGINS must not be empty")
	}
	if strings.TrimSpace(cfg.RedisAddr) == "" {
		return Config{}, fmt.Errorf("GATEWAY_REDIS_ADDR must not be empty")
	}
	if strings.TrimSpace(cfg.TokenHashSecret) == "" {
		return Config{}, fmt.Errorf("GATEWAY_TOKEN_HASH_SECRET must not be empty")
	}
	if strings.TrimSpace(cfg.TokenHashKeyVersion) == "" {
		return Config{}, fmt.Errorf("GATEWAY_TOKEN_HASH_KEY_VERSION must not be empty")
	}

	return cfg, nil
}

func stringValue(key string, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func csvValue(key string, fallback []string) []string {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return append([]string(nil), fallback...)
	}
	parts := strings.Split(raw, ",")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			values = append(values, trimmed)
		}
	}
	return values
}
