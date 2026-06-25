package config

import "github.com/spf13/viper"

type Config struct {
	Port         string
	DBPath       string
	SeedUsers    string
	JWTSecret    string
	CookieSecure bool
	MCPToken     string // required bearer token for /mcp; if empty, /mcp is disabled
}

func Load() (*Config, error) {
	viper.SetEnvPrefix("HCW")
	viper.AutomaticEnv()
	viper.SetDefault("PORT", "8080")
	viper.SetDefault("DBPATH", "hcw.db")
	viper.SetDefault("COOKIE_SECURE", true)
	return &Config{
		Port:         viper.GetString("PORT"),
		DBPath:       viper.GetString("DBPATH"),
		SeedUsers:    viper.GetString("SEED_USERS"),
		JWTSecret:    viper.GetString("JWT_SECRET"),
		CookieSecure: viper.GetBool("COOKIE_SECURE"),
		MCPToken:     viper.GetString("MCP_TOKEN"),
	}, nil
}
