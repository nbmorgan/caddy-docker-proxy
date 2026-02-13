package caddydockerproxy

import (
	"strings"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var appLogger *zap.Logger = zap.NewNop()

func setLogger(logger *zap.Logger) {
	if logger == nil {
		appLogger = zap.NewNop()
		return
	}
	appLogger = logger
}

func logger() *zap.Logger {
	return appLogger.Named("docker-proxy")
}

func buildLogger(format string) *zap.Logger {
	encoding := "json"
	if strings.EqualFold(format, "console") {
		encoding = "console"
	}

	cfg := zap.NewProductionConfig()
	cfg.Encoding = encoding
	cfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	cfg.EncoderConfig.EncodeDuration = zapcore.StringDurationEncoder
	cfg.EncoderConfig.EncodeLevel = zapcore.LowercaseLevelEncoder
	if encoding == "console" {
		cfg.EncoderConfig.EncodeLevel = zapcore.CapitalLevelEncoder
	}

	logger, err := cfg.Build()
	if err != nil {
		return zap.NewNop()
	}
	return logger
}
