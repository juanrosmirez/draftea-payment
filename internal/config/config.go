package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config configuración de la aplicación
type Config struct {
	Server   ServerConfig   `json:"server"`
	Database DatabaseConfig `json:"database"`
	Redis    RedisConfig    `json:"redis"`
	Kafka    KafkaConfig    `json:"kafka"`
	Gateway  GatewayConfig  `json:"gateway"`
	Metrics  MetricsConfig  `json:"metrics"`
	Logging  LoggingConfig  `json:"logging"`
}

// ServerConfig configuración del servidor HTTP
type ServerConfig struct {
	Port         int           `json:"port"`
	Host         string        `json:"host"`
	ReadTimeout  time.Duration `json:"read_timeout"`
	WriteTimeout time.Duration `json:"write_timeout"`
	IdleTimeout  time.Duration `json:"idle_timeout"`
}

// DatabaseConfig configuración de base de datos
type DatabaseConfig struct {
	Host            string        `json:"host"`
	Port            int           `json:"port"`
	Database        string        `json:"database"`
	Username        string        `json:"username"`
	Password        string        `json:"password"`
	SSLMode         string        `json:"ssl_mode"`
	MaxOpenConns    int           `json:"max_open_conns"`
	MaxIdleConns    int           `json:"max_idle_conns"`
	ConnMaxLifetime time.Duration `json:"conn_max_lifetime"`
}

// RedisConfig configuración de Redis
type RedisConfig struct {
	Host         string        `json:"host"`
	Port         int           `json:"port"`
	Password     string        `json:"password"`
	Database     int           `json:"database"`
	PoolSize     int           `json:"pool_size"`
	DialTimeout  time.Duration `json:"dial_timeout"`
	ReadTimeout  time.Duration `json:"read_timeout"`
	WriteTimeout time.Duration `json:"write_timeout"`
}

// KafkaConfig configuración de Kafka
type KafkaConfig struct {
	Brokers       []string      `json:"brokers"`
	Topic         string        `json:"topic"`
	ConsumerGroup string        `json:"consumer_group"`
	BatchSize     int           `json:"batch_size"`
	BatchTimeout  time.Duration `json:"batch_timeout"`
}

// GatewayConfig configuración de pasarela de pagos
type GatewayConfig struct {
	Provider           string        `json:"provider"`
	BaseURL            string        `json:"base_url"`
	APIKey             string        `json:"api_key"`
	Timeout            time.Duration `json:"timeout"`
	MaxRetries         int           `json:"max_retries"`
	CircuitBreakerMaxFailures int    `json:"circuit_breaker_max_failures"`
	CircuitBreakerResetTimeout time.Duration `json:"circuit_breaker_reset_timeout"`
}

// MetricsConfig configuración de métricas
type MetricsConfig struct {
	Enabled         bool          `json:"enabled"`
	Port            int           `json:"port"`
	Path            string        `json:"path"`
	AlertCheckInterval time.Duration `json:"alert_check_interval"`
}

// LoggingConfig configuración de logging
type LoggingConfig struct {
	Level      string `json:"level"`
	Format     string `json:"format"`
	OutputPath string `json:"output_path"`
}

// Load carga la configuración desde variables de entorno
func Load() (*Config, error) {
	config := &Config{
		Server: ServerConfig{
			Port:         getEnvAsInt("SERVER_PORT", 8080),
			Host:         getEnv("SERVER_HOST", "0.0.0.0"),
			ReadTimeout:  getEnvAsDuration("SERVER_READ_TIMEOUT", "30s"),
			WriteTimeout: getEnvAsDuration("SERVER_WRITE_TIMEOUT", "30s"),
			IdleTimeout:  getEnvAsDuration("SERVER_IDLE_TIMEOUT", "120s"),
		},
		Database: DatabaseConfig{
			Host:            getEnv("DB_HOST", "localhost"),
			Port:            getEnvAsInt("DB_PORT", 5432),
			Database:        getEnv("DB_NAME", "payment_system"),
			Username:        getEnv("DB_USER", "postgres"),
			Password:        getEnv("DB_PASSWORD", ""),
			SSLMode:         getEnv("DB_SSL_MODE", "disable"),
			MaxOpenConns:    getEnvAsInt("DB_MAX_OPEN_CONNS", 25),
			MaxIdleConns:    getEnvAsInt("DB_MAX_IDLE_CONNS", 25),
			ConnMaxLifetime: getEnvAsDuration("DB_CONN_MAX_LIFETIME", "5m"),
		},
		Redis: RedisConfig{
			Host:         getEnv("REDIS_HOST", "localhost"),
			Port:         getEnvAsInt("REDIS_PORT", 6379),
			Password:     getEnv("REDIS_PASSWORD", ""),
			Database:     getEnvAsInt("REDIS_DB", 0),
			PoolSize:     getEnvAsInt("REDIS_POOL_SIZE", 10),
			DialTimeout:  getEnvAsDuration("REDIS_DIAL_TIMEOUT", "5s"),
			ReadTimeout:  getEnvAsDuration("REDIS_READ_TIMEOUT", "3s"),
			WriteTimeout: getEnvAsDuration("REDIS_WRITE_TIMEOUT", "3s"),
		},
		Kafka: KafkaConfig{
			Brokers:       getEnvAsSlice("KAFKA_BROKERS", []string{"localhost:9092"}),
			Topic:         getEnv("KAFKA_TOPIC", "payment-events"),
			ConsumerGroup: getEnv("KAFKA_CONSUMER_GROUP", "payment-service"),
			BatchSize:     getEnvAsInt("KAFKA_BATCH_SIZE", 100),
			BatchTimeout:  getEnvAsDuration("KAFKA_BATCH_TIMEOUT", "10ms"),
		},
		Gateway: GatewayConfig{
			Provider:                   getEnv("GATEWAY_PROVIDER", "stripe"),
			BaseURL:                    getEnv("GATEWAY_BASE_URL", "https://api.stripe.com"),
			APIKey:                     getEnv("GATEWAY_API_KEY", ""),
			Timeout:                    getEnvAsDuration("GATEWAY_TIMEOUT", "30s"),
			MaxRetries:                 getEnvAsInt("GATEWAY_MAX_RETRIES", 3),
			CircuitBreakerMaxFailures:  getEnvAsInt("GATEWAY_CB_MAX_FAILURES", 5),
			CircuitBreakerResetTimeout: getEnvAsDuration("GATEWAY_CB_RESET_TIMEOUT", "60s"),
		},
		Metrics: MetricsConfig{
			Enabled:            getEnvAsBool("METRICS_ENABLED", true),
			Port:               getEnvAsInt("METRICS_PORT", 9090),
			Path:               getEnv("METRICS_PATH", "/metrics"),
			AlertCheckInterval: getEnvAsDuration("METRICS_ALERT_CHECK_INTERVAL", "30s"),
		},
		Logging: LoggingConfig{
			Level:      getEnv("LOG_LEVEL", "info"),
			Format:     getEnv("LOG_FORMAT", "json"),
			OutputPath: getEnv("LOG_OUTPUT_PATH", "stdout"),
		},
	}

	return config, nil
}

// GetDSN retorna el DSN de conexión a la base de datos
func (c *DatabaseConfig) GetDSN() string {
	return fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		c.Host, c.Port, c.Username, c.Password, c.Database, c.SSLMode)
}

// GetRedisAddr retorna la dirección de Redis
func (c *RedisConfig) GetRedisAddr() string {
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
}

// Funciones auxiliares para leer variables de entorno

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvAsInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}

func getEnvAsBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if boolValue, err := strconv.ParseBool(value); err == nil {
			return boolValue
		}
	}
	return defaultValue
}

func getEnvAsDuration(key string, defaultValue string) time.Duration {
	if value := os.Getenv(key); value != "" {
		if duration, err := time.ParseDuration(value); err == nil {
			return duration
		}
	}
	if duration, err := time.ParseDuration(defaultValue); err == nil {
		return duration
	}
	return 0
}

func getEnvAsSlice(key string, defaultValue []string) []string {
	if value := os.Getenv(key); value != "" {
		// Implementación simple - en producción usar un parser más robusto
		return []string{value}
	}
	return defaultValue
}
