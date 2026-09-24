// Package config loads platform configuration from the environment. Nothing
// in the platform is hardcoded (§47): every IP, credential, repository,
// version default, and provider selection is configuration-driven.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
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

	// TalosConfigFile is a path to a talosconfig YAML (as produced by
	// `talosctl config`) used to construct the real Talos client when
	// TalosAdapterMode is "real". This is a Phase 2 bootstrapping
	// simplification for a single-cluster deployment — see the limitation
	// noted in internal/integrations/talos/client.go and docs/roadmap.md
	// for per-cluster credential resolution via the SecretStore.
	TalosConfigFile string

	// GitHubToken is a personal access token or GitHub App installation
	// token used to construct the real GitHub client when GitHubAdapterMode
	// is "real". Same Phase-2-style bootstrapping simplification as
	// TalosConfigFile — see docs/roadmap.md for moving this to the
	// SecretStore-backed path (github.LoadClientFromSecretStore already
	// exists for that).
	GitHubToken string

	// GitHubWebhookSecret verifies the X-Hub-Signature-256 header on inbound
	// GitHub webhook deliveries (§4 human-in-the-loop approval gate). Empty
	// (the default) disables the webhook endpoint entirely — an unverifiable
	// webhook is never accepted.
	GitHubWebhookSecret string

	// ArgoCDServerURL/ArgoCDToken configure the real Argo CD REST client
	// when ArgoCDAdapterMode is "real" — same bootstrapping simplification
	// as TalosConfigFile/GitHubToken.
	ArgoCDServerURL string
	ArgoCDToken     string

	// ClusterAPIKubeconfigFile points at a kubeconfig for the Cluster API
	// management cluster, used when ClusterAPIAdapterMode is "real" — same
	// bootstrapping simplification as TalosConfigFile.
	ClusterAPIKubeconfigFile string

	// Proxmox* configure the real Proxmox client when ProxmoxAdapterMode is
	// "real" (§12). ProxmoxAPIToken is pre-formatted the way Proxmox itself
	// prints it ("user@realm!tokenid=secret").
	ProxmoxAPIURL       string
	ProxmoxNode         string
	ProxmoxAPIToken     string
	ProxmoxTemplateVMID string

	// BareMetal*Enabled register real IPMI/Redfish power controllers (§11).
	// Both may be enabled at once — a site's inventory can mix protocols,
	// dispatched per-machine. Neither implies the other; a protocol left
	// disabled reports NOT_CONFIGURED rather than silently no-op-ing.
	BareMetalIPMIEnabled    bool
	BareMetalRedfishEnabled bool

	SecretStoreBackend string // "local" (dev, AES-GCM at rest) or "vault"
	SecretStoreKeyHex  string // 32-byte hex key for the local backend

	// Vault* configure the real SecretStore backend when SecretStoreBackend
	// is "vault" (ADR-0006, Phase 7). VaultToken is a static token here as
	// the simplest bootstrap; production deployments should prefer an
	// AppRole or Kubernetes auth login instead of a long-lived static token.
	VaultAddr  string
	VaultToken string
	VaultMount string

	// CORSOrigins are the web origins the API accepts cross-origin requests
	// from (§48); empty means same-origin only. RateLimitRPS/RateLimitBurst
	// configure the per-client-IP rate limiter (§25); RateLimitRPS <= 0
	// disables rate limiting.
	CORSOrigins    []string
	RateLimitRPS   float64
	RateLimitBurst int
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
		TalosAdapterMode:         getenv("PLATFORM_TALOS_ADAPTER", "mock"),
		GitHubAdapterMode:        getenv("PLATFORM_GITHUB_ADAPTER", "mock"),
		ArgoCDAdapterMode:        getenv("PLATFORM_ARGOCD_ADAPTER", "mock"),
		ClusterAPIAdapterMode:    getenv("PLATFORM_CLUSTERAPI_ADAPTER", "mock"),
		ProxmoxAdapterMode:       getenv("PLATFORM_PROXMOX_ADAPTER", "mock"),
		TalosConfigFile:          getenv("PLATFORM_TALOS_CONFIG_FILE", ""),
		GitHubToken:              getenv("PLATFORM_GITHUB_TOKEN", ""),
		GitHubWebhookSecret:      getenv("PLATFORM_GITHUB_WEBHOOK_SECRET", ""),
		ArgoCDServerURL:          getenv("PLATFORM_ARGOCD_SERVER_URL", ""),
		ArgoCDToken:              getenv("PLATFORM_ARGOCD_TOKEN", ""),
		ClusterAPIKubeconfigFile: getenv("PLATFORM_CLUSTERAPI_KUBECONFIG_FILE", ""),
		ProxmoxAPIURL:            getenv("PLATFORM_PROXMOX_API_URL", ""),
		ProxmoxNode:              getenv("PLATFORM_PROXMOX_NODE", ""),
		ProxmoxAPIToken:          getenv("PLATFORM_PROXMOX_API_TOKEN", ""),
		ProxmoxTemplateVMID:      getenv("PLATFORM_PROXMOX_TEMPLATE_VMID", ""),
		BareMetalIPMIEnabled:     getenvBool("PLATFORM_BAREMETAL_IPMI_ENABLED", false),
		BareMetalRedfishEnabled:  getenvBool("PLATFORM_BAREMETAL_REDFISH_ENABLED", false),
		SecretStoreBackend:       getenv("PLATFORM_SECRETSTORE_BACKEND", "local"),
		SecretStoreKeyHex:        getenv("PLATFORM_SECRETSTORE_KEY_HEX", ""),
		VaultAddr:                getenv("PLATFORM_VAULT_ADDR", ""),
		VaultToken:               getenv("PLATFORM_VAULT_TOKEN", ""),
		VaultMount:               getenv("PLATFORM_VAULT_MOUNT_PATH", ""),
		CORSOrigins:              getenvList("PLATFORM_CORS_ORIGINS"),
		RateLimitRPS:             getenvFloat("PLATFORM_RATE_LIMIT_RPS", 20),
		RateLimitBurst:           getenvInt("PLATFORM_RATE_LIMIT_BURST", 40),
	}

	if cfg.Auth.Mode != "dev" && cfg.Auth.Mode != "oidc" {
		return Config{}, fmt.Errorf("invalid PLATFORM_AUTH_MODE %q: must be \"dev\" or \"oidc\"", cfg.Auth.Mode)
	}
	if cfg.Auth.Mode == "oidc" && (cfg.Auth.OIDCIssuer == "" || cfg.Auth.OIDCClientID == "") {
		return Config{}, fmt.Errorf("PLATFORM_AUTH_MODE=oidc requires PLATFORM_OIDC_ISSUER and PLATFORM_OIDC_CLIENT_ID")
	}
	if cfg.TalosAdapterMode != "mock" && cfg.TalosAdapterMode != "real" {
		return Config{}, fmt.Errorf("invalid PLATFORM_TALOS_ADAPTER %q: must be \"mock\" or \"real\"", cfg.TalosAdapterMode)
	}
	if cfg.TalosAdapterMode == "real" && cfg.TalosConfigFile == "" {
		return Config{}, fmt.Errorf("PLATFORM_TALOS_ADAPTER=real requires PLATFORM_TALOS_CONFIG_FILE to point at a talosconfig")
	}
	if cfg.GitHubAdapterMode != "mock" && cfg.GitHubAdapterMode != "real" {
		return Config{}, fmt.Errorf("invalid PLATFORM_GITHUB_ADAPTER %q: must be \"mock\" or \"real\"", cfg.GitHubAdapterMode)
	}
	if cfg.GitHubAdapterMode == "real" && cfg.GitHubToken == "" {
		return Config{}, fmt.Errorf("PLATFORM_GITHUB_ADAPTER=real requires PLATFORM_GITHUB_TOKEN")
	}
	if cfg.ArgoCDAdapterMode != "mock" && cfg.ArgoCDAdapterMode != "real" {
		return Config{}, fmt.Errorf("invalid PLATFORM_ARGOCD_ADAPTER %q: must be \"mock\" or \"real\"", cfg.ArgoCDAdapterMode)
	}
	if cfg.ArgoCDAdapterMode == "real" && (cfg.ArgoCDServerURL == "" || cfg.ArgoCDToken == "") {
		return Config{}, fmt.Errorf("PLATFORM_ARGOCD_ADAPTER=real requires PLATFORM_ARGOCD_SERVER_URL and PLATFORM_ARGOCD_TOKEN")
	}
	if cfg.ClusterAPIAdapterMode != "mock" && cfg.ClusterAPIAdapterMode != "real" {
		return Config{}, fmt.Errorf("invalid PLATFORM_CLUSTERAPI_ADAPTER %q: must be \"mock\" or \"real\"", cfg.ClusterAPIAdapterMode)
	}
	if cfg.ClusterAPIAdapterMode == "real" && cfg.ClusterAPIKubeconfigFile == "" {
		return Config{}, fmt.Errorf("PLATFORM_CLUSTERAPI_ADAPTER=real requires PLATFORM_CLUSTERAPI_KUBECONFIG_FILE")
	}
	if cfg.SecretStoreBackend != "local" && cfg.SecretStoreBackend != "vault" {
		return Config{}, fmt.Errorf("invalid PLATFORM_SECRETSTORE_BACKEND %q: must be \"local\" or \"vault\"", cfg.SecretStoreBackend)
	}
	if cfg.SecretStoreBackend == "vault" && (cfg.VaultAddr == "" || cfg.VaultToken == "" || cfg.VaultMount == "") {
		return Config{}, fmt.Errorf("PLATFORM_SECRETSTORE_BACKEND=vault requires PLATFORM_VAULT_ADDR, PLATFORM_VAULT_TOKEN, and PLATFORM_VAULT_MOUNT_PATH")
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

func getenvFloat(key string, def float64) float64 {
	if v, ok := os.LookupEnv(key); ok {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return def
}

// getenvList splits a comma-separated environment variable, trimming
// whitespace around each entry and dropping empty ones. Returns nil (not an
// empty non-nil slice) when the variable is unset, so callers can
// distinguish "not configured" from "configured as empty".
func getenvList(key string) []string {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return nil
	}
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if trimmed := strings.TrimSpace(p); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}
