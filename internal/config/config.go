package config

import (
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	// Database
	DB DBConfig

	// NATS
	NATS NATSConfig

	// Server
	Server ServerConfig

	// Profiles
	Profile string

	// Minio
	Minio MinioConfig
	 Eureka EurekaConfig
}

type DBConfig struct {
	Host            string
	Port            int
	User            string
	Password        string
	Database        string
	SSLMode         string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
	ConnMaxIdleTime time.Duration
	MaxRetries      int
	RetryInterval   time.Duration
}

type NATSConfig struct {
	URL           string
	User          string
	Password      string
	MaxRetries    int
	RetryInterval time.Duration
	DialTimeout   time.Duration
	SubjectPrefix string
}
	// subject := fmt.Sprintf("%s.network.v1.vpc.default.get", p.profile)
// 
type MinioConfig struct {
	Endpoint  string
	AccessKey string
	SecretKey string
	UseSSL    bool
	Bucket    string
}

type ServerConfig struct {
	Port            string
	ServiceName     string
	HTTPPort        int
	PublicInterface string
	ShutdownTimeout time.Duration
}
type EurekaConfig struct {
    ServerURL         string
    AppName           string
    HostName          string
    IPAddr            string
    Port              int
    VipAddress        string
    InstanceID        string
    HeartbeatInterval time.Duration
}

func Load() (*Config, error) {
	_ = godotenv.Load()
	profile := getEnv("APP_PROFILE", "dev")
	_ = godotenv.Load(".env-" + strings.ToLower(profile))

	cfg := &Config{
		DB: DBConfig{
			Host:            getEnv("DB_HOST", "localhost"),
			Port:            getEnvInt("DB_PORT", 5432),
			User:            getEnv("DB_USER", "root"),
			Password:        getEnv("DB_PASSWORD", "root"),
			Database:        getEnv("DB_NAME", "network_db"),
			SSLMode:         getEnv("DB_SSLMODE", "disable"),
			MaxOpenConns:    getEnvInt("DB_MAX_OPEN_CONNS", 25),
			MaxIdleConns:    getEnvInt("DB_MAX_IDLE_CONNS", 10),
			ConnMaxLifetime: getEnvDuration("DB_CONN_MAX_LIFETIME", 5*time.Minute),
			ConnMaxIdleTime: getEnvDuration("DB_CONN_MAX_IDLE_TIME", 10*time.Minute),
			MaxRetries:      getEnvInt("DB_MAX_RETRIES", 4),
			RetryInterval:   getEnvDuration("DB_RETRY_INTERVAL", 2*time.Second),
		},
		Eureka: EurekaConfig{
    ServerURL:         getEnv("EUREKA_SERVER_URL", "http://localhost:8761/eureka"),
    AppName:           getEnv("EUREKA_APP_NAME", "NETWORK-SERVICE"),
    HostName:          getEnv("EUREKA_HOSTNAME", "localhost"),
    IPAddr:            getEnv("EUREKA_IP_ADDR", "127.0.0.1"),
    Port:              getEnvInt("SERVER_PORT", 8084),
    VipAddress:        getEnv("EUREKA_VIP_ADDRESS", "network-service"),
    InstanceID:        getEnv("EUREKA_INSTANCE_ID", "network-service:8084"),
    HeartbeatInterval: getEnvDuration("EUREKA_HEARTBEAT_INTERVAL", 30*time.Second),
},
		NATS: NATSConfig{
			URL:           getEnv("NATS_URL", "nats://localhost:4222"),
			User:          getEnv("NATS_USER", ""),
			Password:      getEnv("NATS_PASSWORD", ""),
			MaxRetries:    getEnvInt("NATS_MAX_RETRIES", 5),
			RetryInterval: getEnvDuration("NATS_RETRY_INTERVAL", 2*time.Second),
			DialTimeout:   getEnvDuration("NATS_DIAL_TIMEOUT", 2*time.Second),
			SubjectPrefix: getEnv("NATS_SUBJECT_PREFIX", "dev.v1"),
			
		},
		Minio: MinioConfig{
			Endpoint:  getEnv("MINIO_ENDPOINT", "localhost:9000"),
			AccessKey: getEnv("MINIO_ACCESS_KEY", "minioadmin"),
			SecretKey: getEnv("MINIO_SECRET_KEY", "minioadmin"),
			UseSSL:    getEnvBool("MINIO_USE_SSL", false),
			Bucket:    getEnv("MINIO_BUCKET", "network-service"),
		},
		Server: ServerConfig{
			HTTPPort:        getEnvInt("HTTP_PORT", 8084),
			PublicInterface: getEnv("PUBLIC_INTERFACE", "eth0"),
			ShutdownTimeout: getEnvDuration("SHUTDOWN_TIMEOUT", 10*time.Second),
		},
		Profile: profile,

	}

	return cfg, nil
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultVal int) int {
	if val := os.Getenv(key); val != "" {
		if intVal, err := strconv.Atoi(val); err == nil {
			return intVal
		}
	}
	return defaultVal
}

func getEnvDuration(key string, defaultVal time.Duration) time.Duration {
	if val := os.Getenv(key); val != "" {
		if d, err := time.ParseDuration(val); err == nil {
			return d
		}
	}
	return defaultVal
}

func getEnvBool(key string, defaultVal bool) bool {
	if val := os.Getenv(key); val != "" {
		if b, err := strconv.ParseBool(val); err == nil {
			return b
		}
	}
	return defaultVal
}
