package conf

import (
	"github.com/joho/godotenv"
	"github.com/kelseyhightower/envconfig"
)

type PostgresSettings struct {
	DiskSize       int    `json:"disk_size" split_words:"true" default:"10"`
	DefaultVersion string `json:"default_version" split_words:"true" default:"14.2"`
	DockerImage    string `json:"docker_image" split_words:"true" default:"supabase/postgres"`
}

type DomainSettings struct {
	StudioUrl  string  `json:"studio_url" split_words:"true" required:"true"`
	Base       string  `json:"base_url" required:"true"`
	DnsHookUrl *string `json:"dns_hook_url" split_words:"true"`
	DnsHookKey *string `json:"dns_hook_key" split_words:"true"`
}

type ProvisioningSettings struct {
	Enabled          bool   `json:"enabled" default:"true"`
	DockerHost       string `json:"docker_host" split_words:"true" default:"unix:///var/run/docker.sock"`
	ProjectsDir      string `json:"projects_dir" split_words:"true" default:"./projects"`
	BasePostgresPort int    `json:"base_postgres_port" split_words:"true" default:"5433"`
	BaseKongHTTPPort int    `json:"base_kong_http_port" split_words:"true" default:"54321"`
	// ProjectHost is the hostname the API dials to reach a provisioned project's
	// published Kong/Postgres ports. Projects run as separate compose stacks that
	// publish their ports on the Docker HOST, so from inside the API container
	// "localhost" is wrong — it must resolve to the host. On Docker this is
	// host.docker.internal (requires `extra_hosts: host.docker.internal:host-gateway`
	// on the api service). Set to "localhost" only when the API runs directly on
	// the host (e.g. local dev without containerizing the API).
	ProjectHost string `json:"project_host" split_words:"true" default:"host.docker.internal"`
}

type Config struct {
	DatabaseUrl       string               `json:"database_url" split_words:"true" required:"true"`
	Port              int                  `json:"port" default:"8080"`
	EncryptionSecret  string               `json:"encryption_secret" split_words:"true" required:"true"`
	JwtSecret         string               `json:"jwt_secret" split_words:"true" required:"true"`
	AllowSignup       bool                 `json:"allow_signup" split_words:"true" default:"false"`
	ServiceVersionUrl string               `json:"service_version_url" split_words:"true" required:"true" default:"https://supamanager.io/updates"`
	Domain            DomainSettings       `json:"domain" required:"true"`
	Postgres          PostgresSettings     `json:"postgres" required:"true"`
	Provisioning      ProvisioningSettings `json:"provisioning"`
	AllowedOrigins    []string             `json:"allowed_origins" split_words:"true" default:"*"`
	RateLimitRequests int                  `json:"rate_limit_requests" split_words:"true" default:"100"`
}

func LoadConfig(filename string) (*Config, error) {
	// Try to load .env file if it exists, but don't fail if it doesn't.
	// Production environments (like Coolify/Docker) pass env vars directly.
	if filename != "" {
		_ = godotenv.Load(filename)
	} else {
		_ = godotenv.Load()
	}

	config := new(Config)
	if err := envconfig.Process("", config); err != nil {
		return nil, err
	}
	return config, nil
}


