package main

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"os/signal"
	"syscall"
	"time"

	"observability-go/consumer-1/logger"

	"github.com/rabbitmq/amqp091-go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
	"go.uber.org/zap"
)

func initTracer() func() {
	// Configure OTLP over HTTP exporter to Tempo
	ctx := context.Background()
	httpClient := otlptracehttp.NewClient(
		otlptracehttp.WithEndpoint("tempo:4318"),
		otlptracehttp.WithInsecure(),
	)

	exp, err := otlptrace.New(ctx, httpClient)
	if err != nil {
		// fallback to no-op provider if exporter fails to initialize
		tp := trace.NewTracerProvider()
		otel.SetTracerProvider(tp)

		otel.SetTextMapPropagator(
			propagation.NewCompositeTextMapPropagator(
				propagation.TraceContext{},
				propagation.Baggage{},
			),
		)
		return func() { _ = tp.Shutdown(ctx) }
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceNameKey.String(os.Getenv("SERVICE_NAME")),
		),
	)
	if err != nil {
		res = resource.Empty()
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

// processMessage simulates message processing with multiple steps
func processMessage(ctx context.Context, log *zap.Logger, body []byte) error {
	// Start a new span for the processing
	_, span := otel.Tracer("consumer-1").Start(ctx, "ProcessMessage")
	defer span.End()

	// Step 1: Parse the message
	log.Info("Parsing message")
	// Simulate parsing time
	time.Sleep(time.Duration(rand.Intn(100)) * time.Millisecond)

	// Step 2: Validate the message
	log.Info("Validating message")
	if len(body) == 0 {
		return fmt.Errorf("empty message body")
	}
	time.Sleep(time.Duration(rand.Intn(150)) * time.Millisecond)

	// Simulate random error
	if rand.Intn(3) == 0 {
		err := fmt.Errorf("random processing error in consumer-1")
		span.RecordError(err)
		log.Error("Random processing error", zap.Error(err))
		return err
	}

	// Step 3: Process the message
	log.Info("Processing message",
		zap.Int("message_length", len(body)),
		zap.String("first_10_bytes", string(body[:min(10, len(body))])),
	)
	time.Sleep(time.Duration(rand.Intn(750)) * time.Millisecond)

	log.Info("Message processed successfully")
	return nil
}

// min returns the smaller of x or y
func min(x, y int) int {
	if x < y {
		return x
	}
	return y
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

			// Process the message
			if err := processMessage(ctx, traceLogger, d.Body); err != nil {
				traceLogger.Error("Failed to process message", zap.Error(err))
				d.Nack(false, true)
				// End the span after processing is complete
				if span != nil {
					span.End()
				}
				continue
			}

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
