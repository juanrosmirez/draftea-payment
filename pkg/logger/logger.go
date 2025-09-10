package logger

import (
	"os"

	"payment-system/internal/config"

	"go.uber.org/zap"
)

// New crea un nuevo logger configurado
func New(level, format string) *zap.Logger {
	cfg := config.LoggingConfig{
		Level:  level,
		Format: format,
	}
	var zapConfig zap.Config

	// Configurar según el nivel
	switch cfg.Level {
	case "debug":
		zapConfig = zap.NewDevelopmentConfig()
	case "info", "warn", "error":
		zapConfig = zap.NewProductionConfig()
	default:
		zapConfig = zap.NewProductionConfig()
		zapConfig.Level = zap.NewAtomicLevelAt(zap.InfoLevel)
	}

	// Configurar nivel específico
	switch cfg.Level {
	case "debug":
		zapConfig.Level = zap.NewAtomicLevelAt(zap.DebugLevel)
	case "info":
		zapConfig.Level = zap.NewAtomicLevelAt(zap.InfoLevel)
	case "warn":
		zapConfig.Level = zap.NewAtomicLevelAt(zap.WarnLevel)
	case "error":
		zapConfig.Level = zap.NewAtomicLevelAt(zap.ErrorLevel)
	}

	// Configurar formato
	if cfg.Format == "console" {
		zapConfig.Encoding = "console"
		zapConfig.EncoderConfig = zap.NewDevelopmentEncoderConfig()
	} else {
		zapConfig.Encoding = "json"
		zapConfig.EncoderConfig = zap.NewProductionEncoderConfig()
	}

	// Configurar salida
	if cfg.OutputPath != "" && cfg.OutputPath != "stdout" {
		zapConfig.OutputPaths = []string{cfg.OutputPath}
		zapConfig.ErrorOutputPaths = []string{cfg.OutputPath}
	}

	// Agregar campos comunes
	zapConfig.InitialFields = map[string]interface{}{
		"service": "payment-system",
		"version": "1.0.0",
		"pid":     os.Getpid(),
	}

	logger, err := zapConfig.Build()
	if err != nil {
		// Fallback to a basic logger if configuration fails
		logger, _ = zap.NewProduction()
	}
	return logger
}
