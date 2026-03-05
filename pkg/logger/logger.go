package logger

import (
	"context"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var Log *zap.Logger

type contextKey string

const CorrelationIDKey contextKey = "correlation_id"

func Init(level string) {
	config := zap.NewProductionConfig()

	// Adjust log level
	var zapLevel zapcore.Level
	if err := zapLevel.UnmarshalText([]byte(level)); err != nil {
		zapLevel = zap.InfoLevel
	}
	config.Level = zap.NewAtomicLevelAt(zapLevel)

	var err error
	Log, err = config.Build()
	if err != nil {
		panic(err)
	}

	zap.ReplaceGlobals(Log)
}

func WithContext(ctx context.Context) *zap.Logger {
	if ctx == nil {
		return zap.L()
	}

	if cid, ok := ctx.Value(CorrelationIDKey).(string); ok && cid != "" {
		return zap.L().With(zap.String("correlation_id", cid))
	}

	return zap.L()
}
