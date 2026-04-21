package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	HTTPAddr string

	DBHost     string
	DBPort     int
	DBUser     string
	DBPassword string
	DBName     string

	S3Region          string
	S3Bucket          string
	S3Endpoint        string
	S3AccessKeyID     string
	S3SecretAccessKey string
	S3UsePathStyle    bool
	S3PublicBaseURL   string

	RecordingsDir      string
	ScanInterval       time.Duration
	StableWindow       time.Duration
	MaxUploadFileBytes int64
	RabbitMQURL        string
	RabbitMQQueue      string
}

func Load() (Config, error) {
	cfg := Config{
		HTTPAddr:           getEnv("HTTP_LISTEN_ADDR", ":8081"),
		DBHost:             getEnv("DB_HOST", "localhost"),
		DBPort:             getEnvAsInt("DB_PORT", 3306),
		DBUser:             getEnv("DB_USER", "root"),
		DBPassword:         getEnv("DB_PASSWORD", ""),
		DBName:             getEnv("DB_NAME", "recordings"),
		S3Region:           getEnv("S3_REGION", "us-east-1"),
		S3Bucket:           getEnv("S3_BUCKET", "blume-recordings"),
		S3Endpoint:         os.Getenv("S3_ENDPOINT"),
		S3AccessKeyID:      os.Getenv("S3_ACCESS_KEY_ID"),
		S3SecretAccessKey:  os.Getenv("S3_SECRET_ACCESS_KEY"),
		S3UsePathStyle:     getEnvAsBool("S3_USE_PATH_STYLE", true),
		S3PublicBaseURL:    os.Getenv("S3_PUBLIC_BASE_URL"),
		RecordingsDir:      getEnv("RECORDINGS_DIR", "/recordings"),
		ScanInterval:       getEnvAsDuration("SCAN_INTERVAL", 10*time.Second),
		StableWindow:       getEnvAsDuration("STABLE_WINDOW", 20*time.Second),
		MaxUploadFileBytes: getEnvAsInt64("MAX_UPLOAD_FILE_BYTES", 5*1024*1024*1024),
		RabbitMQURL:        os.Getenv("RABBITMQ_URL"),
		RabbitMQQueue:      getEnv("RABBITMQ_QUEUE", "recordings.ready"),
	}

	if cfg.DBPassword == "" {
		return Config{}, fmt.Errorf("DB_PASSWORD is required")
	}
	return cfg, nil
}

func getEnv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}

func getEnvAsInt(key string, fallback int) int {
	val := os.Getenv(key)
	if val == "" {
		return fallback
	}
	n, err := strconv.Atoi(val)
	if err != nil {
		return fallback
	}
	return n
}

func getEnvAsInt64(key string, fallback int64) int64 {
	val := os.Getenv(key)
	if val == "" {
		return fallback
	}
	n, err := strconv.ParseInt(val, 10, 64)
	if err != nil {
		return fallback
	}
	return n
}

func getEnvAsBool(key string, fallback bool) bool {
	val := os.Getenv(key)
	if val == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(val)
	if err != nil {
		return fallback
	}
	return parsed
}

func getEnvAsDuration(key string, fallback time.Duration) time.Duration {
	val := os.Getenv(key)
	if val == "" {
		return fallback
	}
	d, err := time.ParseDuration(val)
	if err != nil {
		return fallback
	}
	return d
}
