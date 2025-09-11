package handler

import (
	"context"
	"errors"
	"math/rand"
	"observability-go/logger"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/rabbitmq/amqp091-go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.uber.org/zap"
)

// RabbitMQCarrier is a custom carrier for RabbitMQ headers
type RabbitMQCarrier struct {
	headers amqp091.Table
}

func (c *RabbitMQCarrier) Get(key string) string {
	if val, ok := c.headers[key]; ok {
		if strVal, ok := val.(string); ok {
			return strVal
		}
	}
	return ""
}

func (c *RabbitMQCarrier) Set(key string, value string) {
	c.headers[key] = value
}

func (c *RabbitMQCarrier) Keys() []string {
	keys := make([]string, 0, len(c.headers))
	for k := range c.headers {
		keys = append(keys, k)
	}
	return keys
}

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
		// Get the context from the request
		ctx := c.UserContext()

		// Start a new span for this request
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

		// Connect to RabbitMQ
		conn, err := amqp091.Dial("amqp://guest:guest@rabbitmq:5672/")
		if err != nil {
			log.Error("Failed to connect to RabbitMQ",
				zap.String("trace_id", currentSpanId),
				zap.Error(err))
			return c.Status(500).JSON(fiber.Map{"error": "Failed to connect to message queue"})
		}
		defer conn.Close()

		ch, err := conn.Channel()
		if err != nil {
			log.Error("Failed to open a channel",
				zap.String("trace_id", currentSpanId),
				zap.Error(err))
			return c.Status(500).JSON(fiber.Map{"error": "Failed to create message channel"})
		}
		defer ch.Close()

		// Prepare message with trace context
		headers := make(amqp091.Table)
		carrier := &RabbitMQCarrier{headers: headers}
		otel.GetTextMapPropagator().Inject(ctx, carrier)

		// Publish message to consumer-1
		err = ch.Publish(
			"",           // exchange
			"task_queue", // routing key
			false,        // mandatory
			false,        // immediate
			amqp091.Publishing{
				ContentType: "text/plain",
				Body:        []byte("Hello from app-2"),
				Headers:     headers,
			},
		)

		if err != nil {
			log.Error("Failed to publish message",
				zap.String("trace_id", currentSpanId),
				zap.Error(err))
			return c.Status(500).JSON(fiber.Map{"error": "Failed to publish message"})
		}

		log.Info("Message sent to consumer-1",
			zap.String("trace_id", currentSpanId))

		// Return response with trace context
		return c.JSON(fiber.Map{
			"status":  "processed and forwarded to consumer-1",
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
