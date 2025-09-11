package main

import (
	"context"
	"fmt"
	"observability-go/handler"
	"observability-go/logger"
	"os"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/pprof"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/gofiber/fiber/v2/middleware/requestid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/gofiber/adaptor/v2"
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

	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceNameKey.String(os.Getenv("SERVICE_NAME")),
		),
	)
	if err != nil {
		zapLogger.Fatal("failed to create resource", zap.Error(err))
	}

	tp := trace.NewTracerProvider(
		trace.WithBatcher(exp),
		trace.WithResource(res),
	)

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(
		propagation.NewCompositeTextMapPropagator(
			propagation.TraceContext{},
			propagation.Baggage{},
		),
	)

	return func() { _ = tp.Shutdown(ctx) }
}

func main() {
	zapLogger = logger.New("loki:3100", os.Getenv("LOG_FILE"))
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

	// Prometheus middleware to collect metrics
	app.Use(func(c *fiber.Ctx) error {
		start := time.Now()
		err := c.Next()

		path := c.Path()
		statusCode := strconv.Itoa(c.Response().StatusCode())

		// Add status code label to the metrics
		requestDuration.WithLabelValues(
			c.Method(),
			path,
			statusCode,
		).Observe(time.Since(start).Seconds())

		return err
	})

	// Add a test endpoint to generate 5xx errors
	app.Get("/error", func(c *fiber.Ctx) error {
		return c.Status(500).SendString("Internal Server Error")
	})

	// Prometheus metrics endpoint
	app.Get("/metrics", adaptor.HTTPHandler(promhttp.Handler()))

	handler.RegisterRoutes(app, zapLogger)

	zapLogger.Info(fmt.Sprintf("starting server on :%s", os.Getenv("PORT")))
	if err := app.Listen(fmt.Sprintf(":%s", os.Getenv("PORT"))); err != nil {
		zapLogger.Fatal("server failed", zap.Error(err))
	}
}
