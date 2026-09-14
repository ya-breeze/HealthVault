package config

import (
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	Port         string
	DBPath       string
	SeedUsers    string
	JWTSecret    string
	CookieSecure bool
	MCPToken     string // required bearer token for /mcp; if empty, /mcp is disabled

	// Cloudflare Access sign-in exchange (POST /api/auth/cf-access). All three
	// default to empty; the endpoint answers 404 unless all three are set.
	CFAccessTeamDomain string // e.g. "myteam.cloudflareaccess.com"
	CFAccessAUD        string // the Access application's AUD tag
	CFAccessEmailMap   string // "email:username,..." — see database.SeedUsers's spec shape

	// Food logging
	OpenAIAPIKey   string
	OpenAIModel    string
	UploadsDir     string
	USDADBPath     string
	OFFDBPath      string
	MaxUploadBytes int64
	VisionTimeout  time.Duration
}

func Load() (*Config, error) {
	viper.SetEnvPrefix("HCW")
	viper.AutomaticEnv()
	viper.SetDefault("PORT", "8080")
	viper.SetDefault("DBPATH", "hcw.db")
	viper.SetDefault("COOKIE_SECURE", true)
	viper.SetDefault("UPLOADS_DIR", "./data/uploads")
	viper.SetDefault("USDA_DB_PATH", "./data/usda.db")
	viper.SetDefault("OFF_DB_PATH", "./data/off.db")
	viper.SetDefault("MAX_UPLOAD_BYTES", 10*1024*1024)
	viper.SetDefault("VISION_TIMEOUT", "60s")
	viper.SetDefault("OPENAI_MODEL", "gpt-4o-mini")
	return &Config{
		Port:         viper.GetString("PORT"),
		DBPath:       viper.GetString("DBPATH"),
		SeedUsers:    viper.GetString("SEED_USERS"),
		JWTSecret:    viper.GetString("JWT_SECRET"),
		CookieSecure: viper.GetBool("COOKIE_SECURE"),
		MCPToken:     viper.GetString("MCP_TOKEN"),

		CFAccessTeamDomain: viper.GetString("CF_ACCESS_TEAM_DOMAIN"),
		CFAccessAUD:        viper.GetString("CF_ACCESS_AUD"),
		CFAccessEmailMap:   viper.GetString("CF_ACCESS_EMAIL_MAP"),

		OpenAIAPIKey:   viper.GetString("OPENAI_API_KEY"),
		OpenAIModel:    viper.GetString("OPENAI_MODEL"),
		UploadsDir:     viper.GetString("UPLOADS_DIR"),
		USDADBPath:     viper.GetString("USDA_DB_PATH"),
		OFFDBPath:      viper.GetString("OFF_DB_PATH"),
		MaxUploadBytes: viper.GetInt64("MAX_UPLOAD_BYTES"),
		VisionTimeout:  viper.GetDuration("VISION_TIMEOUT"),
	}, nil
}
