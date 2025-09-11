package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"observability-go/consumer-1/logger"

	"github.com/rabbitmq/amqp091-go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/trace"
	"go.uber.org/zap"
)

func initTracer() func() {
	// Initialize a simple tracer provider without exporters
	tp := trace.NewTracerProvider()
	otel.SetTracerProvider(tp)

	// Set up OpenTelemetry propagation with both TraceContext and Baggage
	otel.SetTextMapPropagator(
		propagation.NewCompositeTextMapPropagator(
			propagation.TraceContext{},
			propagation.Baggage{},
		),
	)

	return func() { _ = tp.Shutdown(context.Background()) }
}

// Custom carrier for RabbitMQ headers
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

func main() {
	cleanup := initTracer()
	defer cleanup()

	// Initialize logger
	zapLogger := logger.New("loki:3100", os.Getenv("LOG_FILE"))
	defer zapLogger.Sync()

	conn, err := amqp091.Dial("amqp://guest:guest@rabbitmq:5672")
	if err != nil {
		zapLogger.Error("Failed to connect to RabbitMQ", zap.Error(err))
		return
	}
	// connection will be closed on graceful shutdown

	ch, err := conn.Channel()
	if err != nil {
		zapLogger.Error("Failed to open a channel", zap.Error(err))
		return
	}
	// channel will be closed on graceful shutdown

	// Declare the incoming queue
	qIn, err := ch.QueueDeclare(
		"task_queue", // name
		true,         // durable
		false,        // delete when unused
		false,        // exclusive
		false,        // no-wait
		nil,          // arguments
	)
	if err != nil {
		zapLogger.Error("Failed to declare incoming queue", zap.Error(err))
		return
	}

	msgs, err := ch.Consume(
		qIn.Name, // queue
		"",       // consumer
		false,    // auto-ack
		false,    // exclusive
		false,    // no-local
		false,    // no-wait
		nil,      // args
	)
	if err != nil {
		zapLogger.Error("Failed to register a consumer", zap.Error(err))
		return
	}

	// Set up signal handling for graceful shutdown
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	zapLogger.Info("[Consumer 1] Waiting for messages. To exit press CTRL+C")

	go func() {
		for d := range msgs {
			// Extract trace context from headers if available
			ctx := context.Background()
			if len(d.Headers) > 0 {
				carrier := &RabbitMQCarrier{headers: d.Headers}
				ctx = otel.GetTextMapPropagator().Extract(ctx, carrier)
			}

			// Start a new span for processing
			tracer := otel.Tracer("consumer-1")
			ctx, span := tracer.Start(ctx, "Process Message")
			currentSpanId := ""
			if span != nil && span.SpanContext().IsValid() {
				currentSpanId = span.SpanContext().SpanID().String()
			}

			// Use logger with trace context
			traceLogger := logger.WithTrace(ctx, currentSpanId)
			traceLogger.Info("[Consumer 1] Received a message", zap.String("message", string(d.Body)))

			// Simulate processing
			time.Sleep(1 * time.Second)

			// Prepare headers for trace context propagation
			headers := make(amqp091.Table)
			carrier := &RabbitMQCarrier{headers: headers}
			otel.GetTextMapPropagator().Inject(ctx, carrier)

			// Forward the message to consumer-2 with trace context
			err := ch.Publish(
				"",             // exchange
				"task_queue_2", // routing key
				false,          // mandatory
				false,          // immediate
				amqp091.Publishing{
					ContentType: d.ContentType,
					Body:        d.Body,
					Headers:     headers,
				},
			)
			if err != nil {
				traceLogger.Error("[Consumer 1] Failed to forward message", zap.Error(err))
			} else {
				traceLogger.Info("[Consumer 1] Forwarded message to consumer-2")
			}

			// End the span after processing is complete
			if span != nil {
				span.End()
			}

			// Acknowledge the original message
			d.Ack(false)
		}
	}()

	// Wait for termination signal
	<-stop
	zapLogger.Info("[Consumer 1] Received termination signal, shutting down gracefully")

	// Close the channel and connection
	if err := ch.Close(); err != nil {
		zapLogger.Error("[Consumer 1] Error closing channel", zap.Error(err))
	}
	if err := conn.Close(); err != nil {
		zapLogger.Error("[Consumer 1] Error closing connection", zap.Error(err))
	}

	zapLogger.Info("[Consumer 1] Shutdown complete")
}
