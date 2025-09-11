package handler

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"net/http"
	"observability-go/logger"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	"github.com/gofiber/fiber/v2"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.uber.org/zap"
)

func RegisterRoutes(app *fiber.App, log *zap.Logger) {
	tracer := otel.Tracer("app-1")

	// Normal hello
	app.Get("/hello", func(c *fiber.Ctx) error {
		ctx := c.UserContext()
		ctx, span := tracer.Start(ctx, "GET /hello")
		defer span.End()
		currentSpanId := span.SpanContext().SpanID().String()

		logger.WithTrace(ctx, currentSpanId).Info("handling /hello")
		simulateSlowFunction(ctx)

		logger.WithTrace(ctx, currentSpanId).Info("hello success")
		return c.JSON(fiber.Map{"message": "hello"})
	})

	// Random delay endpoint
	app.Get("/random-delay", func(c *fiber.Ctx) error {
		ctx := c.UserContext()
		ctx, span := tracer.Start(ctx, "GET /random-delay")
		defer span.End()

		logger.WithTrace(ctx, span.SpanContext().SpanID().String()).Info("random-delay working")

		delay := simulateRandomDelay(ctx)
		return c.JSON(fiber.Map{"delay_ms": delay})
	})

	// Random error endpoint
	app.Get("/random-error", func(c *fiber.Ctx) error {
		ctx := c.UserContext()
		ctx, span := tracer.Start(ctx, "GET /random-error")
		defer span.End()
		currentSpanId := span.SpanContext().SpanID().String()

		logger.WithTrace(ctx, currentSpanId).Info("random-error working")

		if err := simulateRandomError(ctx); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			logger.WithTrace(ctx, currentSpanId).Error("error in /random-error", zap.Error(err))
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}

		logger.WithTrace(ctx, currentSpanId).Info("random-error success")
		return c.JSON(fiber.Map{"message": "success"})
	})

	// Multi-function call (chained spans)
	app.Get("/chain", func(c *fiber.Ctx) error {
		ctx := c.UserContext()
		ctx, span := tracer.Start(ctx, "GET /chain")
		defer span.End()
		currentSpanId := span.SpanContext().SpanID().String()

		logger.WithTrace(ctx, currentSpanId).Info("chain working")

		step1(ctx)
		step2(ctx)
		step3(ctx)

		return c.JSON(fiber.Map{"message": "chain done"})
	})

	// New endpoint that calls app-2
	app.Get("/call-app2", func(c *fiber.Ctx) error {
		ctx := c.UserContext()
		ctx, span := tracer.Start(ctx, "GET /call-app2")
		defer span.End()
		currentSpanId := span.SpanContext().SpanID().String()

		logger.WithTrace(ctx, currentSpanId).Info("Calling app-2 service")

		// Create HTTP client with OpenTelemetry transport
		client := &http.Client{
			Transport: otelhttp.NewTransport(http.DefaultTransport),
		}

		// Create request with context
		req, err := http.NewRequestWithContext(
			ctx,
			"POST",
			"http://app-2:8081/process",
			nil,
		)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "Failed to create request to app-2")
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Failed to create request to app-2",
			})
		}

		// Add any headers if needed
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Request-ID", c.Get("X-Request-ID"))

		// Make the request
		resp, err := client.Do(req)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "Failed to call app-2")
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": fmt.Sprintf("Failed to call app-2: %v", err),
			})
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			errMsg := fmt.Sprintf("app-2 returned status: %d", resp.StatusCode)
			span.RecordError(errors.New(errMsg))
			span.SetStatus(codes.Error, errMsg)
			return c.Status(resp.StatusCode).JSON(fiber.Map{
				"error": errMsg,
			})
		}

		logger.WithTrace(ctx, currentSpanId).Info("Successfully called app-2")
		return c.JSON(fiber.Map{
			"message": "Successfully called app-2",
			"status":  "success",
		})
	})
}

// --- Simulated Functions ---

func simulateSlowFunction(ctx context.Context) {
	_, span := otel.Tracer("app-1").Start(ctx, "simulateSlowFunction")
	defer span.End()

	delay := 200
	span.SetAttributes(attribute.Int("delay_ms", delay))
	logger.WithTrace(ctx, span.SpanContext().SpanID().String()).Info("simulateSlowFunction working")
	time.Sleep(time.Duration(delay) * time.Millisecond)
}

func simulateRandomDelay(ctx context.Context) int {
	_, span := otel.Tracer("app-1").Start(ctx, "simulateRandomDelay")
	defer span.End()

	delay := rand.Intn(1000) // 0–1000 ms
	time.Sleep(time.Duration(delay) * time.Millisecond)
	span.SetAttributes(attribute.Int("delay_ms", delay))
	logger.WithTrace(ctx, span.SpanContext().SpanID().String()).Info("simulateRandomDelay working", zap.Int("delay_ms", delay))
	return delay
}

func simulateRandomError(ctx context.Context) error {
	_, span := otel.Tracer("app-1").Start(ctx, "simulateRandomError")
	defer span.End()

	logger.WithTrace(ctx, span.SpanContext().SpanID().String()).Info("simulateRandomError working")
	if rand.Intn(2) == 0 {
		span.RecordError(errors.New("simulated random error"))
		span.SetStatus(codes.Error, "simulated random error")
		return errors.New("simulated random error")
	}
	return nil
}

// --- Chained functions to see span breakdown ---

func step1(ctx context.Context) {
	_, span := otel.Tracer("app-1").Start(ctx, "step1")
	defer span.End()

	logger.WithTrace(ctx, span.SpanContext().SpanID().String()).Info("step1 working")
	time.Sleep(100 * time.Millisecond)
	step1Subtask(ctx)
}

func step1Subtask(ctx context.Context) {
	_, span := otel.Tracer("app-1").Start(ctx, "step1Subtask")
	defer span.End()

	logger.WithTrace(ctx, span.SpanContext().SpanID().String()).Info("step1Subtask working")
	time.Sleep(50 * time.Millisecond)
}

func step2(ctx context.Context) {
	_, span := otel.Tracer("app-1").Start(ctx, "step2")
	defer span.End()

	logger.WithTrace(ctx, span.SpanContext().SpanID().String()).Info("step2 working")
	time.Sleep(200 * time.Millisecond)
}

func step3(ctx context.Context) {
	_, span := otel.Tracer("app-1").Start(ctx, "step3")
	defer span.End()

	logger.WithTrace(ctx, span.SpanContext().SpanID().String()).Info("step3 working")
	time.Sleep(150 * time.Millisecond)
}
