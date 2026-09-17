// Package config loads platform configuration from the environment. Nothing
// in the platform is hardcoded (§47): every IP, credential, repository,
// version default, and provider selection is configuration-driven.
package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config is the fully-resolved runtime configuration for every platform
// component (platform-api, platform-worker, platform-scheduler, CLI).
type Config struct {
	Env string // "development", "staging", "production"

	HTTPAddr string

	Postgres PostgresConfig
	Redis    RedisConfig
	RabbitMQ RabbitMQConfig

	Auth AuthConfig

	Observability ObservabilityConfig

	// AdapterMode selects "mock" or "real" per integration; "mock" is the
	// default so the platform runs without physical infrastructure (§44).
	TalosAdapterMode      string
	GitHubAdapterMode     string
	ArgoCDAdapterMode     string
	ClusterAPIAdapterMode string
	ProxmoxAdapterMode    string

	SecretStoreBackend string // "local" (dev, AES-GCM at rest) or "vault"
	SecretStoreKeyHex  string // 32-byte hex key for the local backend
}

type PostgresConfig struct {
	DSN             string
	MaxConns        int32
	ConnMaxLifetime time.Duration
}

type RedisConfig struct {
	Addr string
}

type RabbitMQConfig struct {
	URL string
}

type AuthConfig struct {
	// Mode is "dev" (locally-signed HS256 tokens) or "oidc".
	Mode          string
	DevSigningKey string
	OIDCIssuer    string
	OIDCClientID  string
}

type ObservabilityConfig struct {
	ServiceName    string
	OTLPEndpoint   string
	PrometheusAddr string
	TracingEnabled bool
}

// Load reads configuration from the environment, applying documented
// defaults suitable for local development only.
func Load() (Config, error) {
	cfg := Config{
		Env:      getenv("PLATFORM_ENV", "development"),
		HTTPAddr: getenv("PLATFORM_HTTP_ADDR", ":8080"),
		Postgres: PostgresConfig{
			DSN:             getenv("PLATFORM_POSTGRES_DSN", "postgres://platform:platform@localhost:5432/platform?sslmode=disable"),
			MaxConns:        int32(getenvInt("PLATFORM_POSTGRES_MAX_CONNS", 10)),
			ConnMaxLifetime: time.Hour,
		},
		Redis: RedisConfig{
			Addr: getenv("PLATFORM_REDIS_ADDR", "localhost:6379"),
		},
		RabbitMQ: RabbitMQConfig{
			URL: getenv("PLATFORM_RABBITMQ_URL", "amqp://platform:platform@localhost:5672/"),
		},
		Auth: AuthConfig{
			Mode:          getenv("PLATFORM_AUTH_MODE", "dev"),
			DevSigningKey: getenv("PLATFORM_AUTH_DEV_SIGNING_KEY", "development-only-signing-key-change-me"),
			OIDCIssuer:    getenv("PLATFORM_OIDC_ISSUER", ""),
			OIDCClientID:  getenv("PLATFORM_OIDC_CLIENT_ID", ""),
		},
		Observability: ObservabilityConfig{
			ServiceName:    getenv("PLATFORM_SERVICE_NAME", "platform-api"),
			OTLPEndpoint:   getenv("PLATFORM_OTLP_ENDPOINT", "localhost:4317"),
			PrometheusAddr: getenv("PLATFORM_PROMETHEUS_ADDR", ":9090"),
			TracingEnabled: getenvBool("PLATFORM_TRACING_ENABLED", true),
		},
		TalosAdapterMode:      getenv("PLATFORM_TALOS_ADAPTER", "mock"),
		GitHubAdapterMode:     getenv("PLATFORM_GITHUB_ADAPTER", "mock"),
		ArgoCDAdapterMode:     getenv("PLATFORM_ARGOCD_ADAPTER", "mock"),
		ClusterAPIAdapterMode: getenv("PLATFORM_CLUSTERAPI_ADAPTER", "mock"),
		ProxmoxAdapterMode:    getenv("PLATFORM_PROXMOX_ADAPTER", "mock"),
		SecretStoreBackend:    getenv("PLATFORM_SECRETSTORE_BACKEND", "local"),
		SecretStoreKeyHex:     getenv("PLATFORM_SECRETSTORE_KEY_HEX", ""),
	}

	if cfg.Auth.Mode != "dev" && cfg.Auth.Mode != "oidc" {
		return Config{}, fmt.Errorf("invalid PLATFORM_AUTH_MODE %q: must be \"dev\" or \"oidc\"", cfg.Auth.Mode)
	}
	return cfg, nil
}

func getenv(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return def
}

func getenvInt(key string, def int) int {
	if v, ok := os.LookupEnv(key); ok {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func getenvBool(key string, def bool) bool {
	if v, ok := os.LookupEnv(key); ok {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return def
}
