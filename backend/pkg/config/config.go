package config

import (
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	Port           string
	DBPath         string
	SeedUsers      string
	JWTSecret      string
	CookieSecure   bool
	MCPToken       string // required bearer token for /mcp; if empty, /mcp is disabled
	BackupAPIToken string // private project-owned backup API; if empty, job creation is disabled

	// Project-owned off-host backup configuration. All fields are optional at
	// startup; the backup runner remains disabled unless every required field is
	// present and the S3 endpoint is a valid HTTPS origin.
	BackupS3Endpoint      string
	BackupS3Bucket        string
	BackupS3AccessKey     string
	BackupS3SecretKey     string
	BackupS3Region        string
	BackupAgeRecipient    string
	BackupEncryptionKeyID string

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
		Port:                  viper.GetString("PORT"),
		DBPath:                viper.GetString("DBPATH"),
		SeedUsers:             viper.GetString("SEED_USERS"),
		JWTSecret:             viper.GetString("JWT_SECRET"),
		CookieSecure:          viper.GetBool("COOKIE_SECURE"),
		MCPToken:              viper.GetString("MCP_TOKEN"),
		BackupAPIToken:        viper.GetString("BACKUP_API_TOKEN"),
		BackupS3Endpoint:      viper.GetString("BACKUP_S3_ENDPOINT"),
		BackupS3Bucket:        viper.GetString("BACKUP_S3_BUCKET"),
		BackupS3AccessKey:     viper.GetString("BACKUP_S3_ACCESS_KEY"),
		BackupS3SecretKey:     viper.GetString("BACKUP_S3_SECRET_KEY"),
		BackupS3Region:        viper.GetString("BACKUP_S3_REGION"),
		BackupAgeRecipient:    viper.GetString("BACKUP_AGE_RECIPIENT"),
		BackupEncryptionKeyID: viper.GetString("BACKUP_ENCRYPTION_KEY_ID"),

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
