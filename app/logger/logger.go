package logger

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

var logger *zap.Logger

func New(lokiURL string, logFilename string) *zap.Logger {
	// Pastikan direktori log ada
	if err := os.MkdirAll(logFilename, 0755); err != nil {
		panic(err)
	}

	config := zapcore.EncoderConfig{
		TimeKey:        "ts",
		LevelKey:       "level",
		NameKey:        "logger",
		CallerKey:      "caller",
		FunctionKey:    "",
		MessageKey:     "msg",
		StacktraceKey:  "stacktrace",
		LineEnding:     "\n",
		EncodeLevel:    zapcore.LowercaseLevelEncoder,
		EncodeTime:     zapcore.ISO8601TimeEncoder,
		EncodeDuration: zapcore.MillisDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	}

	logFile := filepath.Join("/var/log", logFilename)

	// Konfigurasi rotasi log
	lumberjackLogger := &lumberjack.Logger{
		Filename:   logFile,
		MaxSize:    10,   // MB
		MaxBackups: 3,    // Jumlah file backup
		MaxAge:     28,   // Hari
		Compress:   true, // Kompres file lama
	}

	// Buat core untuk file dan console
	core := zapcore.NewTee(
		// File output dengan format JSON
		zapcore.NewCore(
			zapcore.NewJSONEncoder(config),
			zapcore.AddSync(lumberjackLogger),
			zap.InfoLevel,
		),
		// Console output
		zapcore.NewCore(
			zapcore.NewConsoleEncoder(config),
			zapcore.AddSync(os.Stdout),
			zap.DebugLevel,
		),
	)

	// Buat logger dengan caller info dan stacktrace
	logger = zap.New(
		core,
		zap.AddCaller(),
		zap.AddStacktrace(zap.ErrorLevel),
	)

	// Pastikan log disimpan saat aplikasi berhenti
	zap.ReplaceGlobals(logger)

	// Log startup message
	logger.Info("Logger initialized",
		zap.String("log_file", logFile),
		zap.Time("startup_time", time.Now().UTC()),
	)

	return logger
}

// WithTrace returns a logger with trace context fields.
// If spanId is empty, the span_id field will be omitted from the log entry.
func WithTrace(ctx context.Context, spanId string) *zap.Logger {
	span := trace.SpanFromContext(ctx)
	if !span.SpanContext().IsValid() {
		return logger
	}

	fields := make([]zap.Field, 0, 2) // Pre-allocate for 2 fields
	fields = append(fields, zap.String("trace_id", span.SpanContext().TraceID().String()))

	if spanId != "" {
		fields = append(fields, zap.String("span_id", spanId))
	}

	return logger.With(fields...)
}
