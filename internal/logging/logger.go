package logging

import (
	"context"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type Logger struct {
	*zap.Logger
}

func NewLogger(level string) (*Logger, error) {
	config := zap.NewProductionConfig()

	// Parse log level
	var zapLevel zapcore.Level
	if err := zapLevel.UnmarshalText([]byte(level)); err != nil {
		return nil, err
	}
	config.Level = zap.NewAtomicLevelAt(zapLevel)

	logger, err := config.Build()
	if err != nil {
		return nil, err
	}

	return &Logger{logger}, nil
}

func (l *Logger) WithRequestID(ctx context.Context) *zap.Logger {
	if reqID, ok := ctx.Value("request_id").(string); ok {
		return l.With(zap.String("request_id", reqID))
	}
	return l.Logger
}
