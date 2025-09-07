package main

import (
	"context"
	"observability-go/handler"
	"observability-go/logger"

	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/pprof"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/gofiber/fiber/v2/middleware/requestid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/sdk/trace"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var (
	requestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name: "http_request_duration_seconds",
		Help: "Duration of HTTP requests.",
	}, []string{"method", "path", "status"})
	zapLogger *zap.Logger
)

func initTracer() func() {
	ctx := context.Background()
	conn, err := grpc.NewClient("tempo:4317",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)

	if err != nil {
		zapLogger.Fatal("failed to connect to Tempo", zap.Error(err))
	}

	exp, err := otlptracegrpc.New(ctx, otlptracegrpc.WithGRPCConn(conn))
	if err != nil {
		zapLogger.Fatal("failed to create exporter", zap.Error(err))
	}

	tp := trace.NewTracerProvider(
		trace.WithBatcher(exp),
	)
	otel.SetTracerProvider(tp)

	return func() { _ = tp.Shutdown(ctx) }
}

func main() {
	zapLogger = logger.New("loki:3100")
	cleanup := initTracer()
	defer cleanup()

	app := fiber.New()
	app.Use(requestid.New())

	// Initialize pprof with default options
	pprofConfig := pprof.Config{
		Next:   nil,
		Prefix: "/debug/pprof",
	}
	app.Use(pprof.New(pprofConfig))
	app.Use(recover.New())

	// Prometheus middleware
	app.Use(func(c *fiber.Ctx) error {
		start := time.Now()
		err := c.Next()

		route := ""
		if c.Route() != nil {
			route = c.Route().Path
		}

		requestDuration.WithLabelValues(
			c.Method(),
			route,
			strconv.Itoa(c.Response().StatusCode()),
		).Observe(time.Since(start).Seconds())

		return err
	})

	handler.RegisterRoutes(app, zapLogger)

	zapLogger.Info("starting server on :8080")
	if err := app.Listen(":8080"); err != nil {
		zapLogger.Fatal("server failed", zap.Error(err))
	}
}
