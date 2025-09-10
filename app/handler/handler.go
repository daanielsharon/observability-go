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
	tracer := otel.Tracer("fiber-handler")

	// Normal hello
	app.Get("/hello", func(c *fiber.Ctx) error {
		ctx := c.UserContext()
		ctx, span := tracer.Start(ctx, "GET /hello")
		defer span.End()

		logger.WithTrace(ctx).Info("handling /hello")
		simulateSlowFunction(ctx)

		logger.WithTrace(ctx).Info("hello success")
		return c.JSON(fiber.Map{"message": "hello"})
	})

	// Random delay endpoint
	app.Get("/random-delay", func(c *fiber.Ctx) error {
		ctx := c.UserContext()
		ctx, span := tracer.Start(ctx, "GET /random-delay")
		defer span.End()

		logger.WithTrace(ctx).Info("random-delay working")

		delay := simulateRandomDelay(ctx)
		return c.JSON(fiber.Map{"delay_ms": delay})
	})

	// Random error endpoint
	app.Get("/random-error", func(c *fiber.Ctx) error {
		ctx := c.UserContext()
		ctx, span := tracer.Start(ctx, "GET /random-error")
		defer span.End()

		logger.WithTrace(ctx).Info("random-error working")

		if err := simulateRandomError(ctx); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			logger.WithTrace(ctx).Error("error in /random-error", zap.Error(err))
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}

		logger.WithTrace(ctx).Info("random-error success")
		return c.JSON(fiber.Map{"message": "success"})
	})

	// Multi-function call (chained spans)
	app.Get("/chain", func(c *fiber.Ctx) error {
		ctx := c.UserContext()
		ctx, span := tracer.Start(ctx, "GET /chain")
		defer span.End()

		logger.WithTrace(ctx).Info("chain working")

		step1(ctx)
		step2(ctx)
		step3(ctx)

		return c.JSON(fiber.Map{"message": "chain done"})
	})
}

// --- Simulated Functions ---

func simulateSlowFunction(ctx context.Context) {
	_, span := otel.Tracer("fiber-handler").Start(ctx, "simulateSlowFunction")
	defer span.End()

	delay := 200
	span.SetAttributes(attribute.Int("delay_ms", delay))
	logger.WithTrace(ctx).Info("simulateSlowFunction working")
	time.Sleep(time.Duration(delay) * time.Millisecond)
}

func simulateRandomDelay(ctx context.Context) int {
	_, span := otel.Tracer("fiber-handler").Start(ctx, "simulateRandomDelay")
	defer span.End()

	delay := rand.Intn(1000) // 0–1000 ms
	time.Sleep(time.Duration(delay) * time.Millisecond)
	span.SetAttributes(attribute.Int("delay_ms", delay))
	logger.WithTrace(ctx).Info("simulateRandomDelay working", zap.Int("delay_ms", delay))
	return delay
}

func simulateRandomError(ctx context.Context) error {
	_, span := otel.Tracer("fiber-handler").Start(ctx, "simulateRandomError")
	defer span.End()

	logger.WithTrace(ctx).Info("simulateRandomError working")
	if rand.Intn(2) == 0 {
		span.RecordError(errors.New("simulated random error"))
		span.SetStatus(codes.Error, "simulated random error")
		return errors.New("simulated random error")
	}
	return nil
}

// --- Chained functions to see span breakdown ---

func step1(ctx context.Context) {
	_, span := otel.Tracer("fiber-handler").Start(ctx, "step1")
	defer span.End()

	logger.WithTrace(ctx).Info("step1 working")
	time.Sleep(100 * time.Millisecond)
	step1Subtask(ctx)
}

func step1Subtask(ctx context.Context) {
	_, span := otel.Tracer("fiber-handler").Start(ctx, "step1Subtask")
	defer span.End()

	logger.WithTrace(ctx).Info("step1Subtask working")
	time.Sleep(50 * time.Millisecond)
}

func step2(ctx context.Context) {
	_, span := otel.Tracer("fiber-handler").Start(ctx, "step2")
	defer span.End()

	logger.WithTrace(ctx).Info("step2 working")
	time.Sleep(200 * time.Millisecond)
}

func step3(ctx context.Context) {
	_, span := otel.Tracer("fiber-handler").Start(ctx, "step3")
	defer span.End()

	logger.WithTrace(ctx).Info("step3 working")
	time.Sleep(150 * time.Millisecond)
}
