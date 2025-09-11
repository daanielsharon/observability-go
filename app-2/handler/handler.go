package handler

import (
	"context"
	"errors"
	"math/rand"
	"observability-go/logger"
	"time"

	"github.com/gofiber/fiber/v2"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.uber.org/zap"
)

func RegisterRoutes(app *fiber.App, log *zap.Logger) {
	tracer := otel.Tracer("app-2")

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

	// New endpoint for inter-service communication
	app.Post("/process", func(c *fiber.Ctx) error {
		// Extract context from the incoming request
		ctx := c.UserContext()
		ctx, span := tracer.Start(ctx, "POST /process")
		defer span.End()
		currentSpanId := span.SpanContext().SpanID().String()

		logger.WithTrace(ctx, currentSpanId).Info("Received process request")

		// Simulate some processing
		simulateRandomDelay(ctx)

		// Add some attributes to the span
		span.SetAttributes(
			attribute.String("processor", "app-2"),
			attribute.String("request.id", c.Get("X-Request-ID")),
		)

		// Return response with trace context
		return c.JSON(fiber.Map{
			"status":  "processed",
			"service": "app-2",
		})
	})
}

// --- Simulated Functions ---

func simulateRandomDelay(ctx context.Context) int {
	_, span := otel.Tracer("app-2").Start(ctx, "simulateRandomDelay")
	defer span.End()

	delay := rand.Intn(1000) // 0–1000 ms
	time.Sleep(time.Duration(delay) * time.Millisecond)
	span.SetAttributes(attribute.Int("delay_ms", delay))
	logger.WithTrace(ctx, span.SpanContext().SpanID().String()).Info("simulateRandomDelay working", zap.Int("delay_ms", delay))
	return delay
}

func simulateRandomError(ctx context.Context) error {
	_, span := otel.Tracer("app-2").Start(ctx, "simulateRandomError")
	defer span.End()

	logger.WithTrace(ctx, span.SpanContext().SpanID().String()).Info("simulateRandomError working")
	if rand.Intn(2) == 0 {
		span.RecordError(errors.New("simulated random error"))
		span.SetStatus(codes.Error, "simulated random error")
		return errors.New("simulated random error")
	}
	return nil
}
