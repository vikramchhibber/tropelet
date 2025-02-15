package shared

import (
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type Logger = *zap.SugaredLogger

func InitializeLogger() Logger {
	// Logger configuration to include time, level and message
	encoderCfg := zapcore.EncoderConfig{
		TimeKey:    "time",
		LevelKey:   "level",
		MessageKey: "msg",
		LineEnding: zapcore.DefaultLineEnding,
		// Colored output for log level in upper case
		EncodeLevel: zapcore.CapitalColorLevelEncoder,
		EncodeTime:  zapcore.ISO8601TimeEncoder,
	}

	// Create a Core that writes logs to the console
	core := zapcore.NewCore(
		zapcore.NewConsoleEncoder(encoderCfg),
		zapcore.Lock(os.Stdout),
		// TODO: Candidate for config
		zap.NewAtomicLevelAt(zap.DebugLevel),
	)

	// Build the logger with this core
	return zap.New(core).Sugar()
}
