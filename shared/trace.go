package shared

import (
	"context"
	"encoding/json"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

func InjectTraceContext(ctx context.Context) map[string]string {
	carrier := make(map[string]string)
	otel.GetTextMapPropagator().Inject(ctx, propagation.MapCarrier(carrier))
	return carrier
}

func InjectTraceContextJSON(ctx context.Context) string {
	carrier := InjectTraceContext(ctx)
	jsonCarrier, _ := json.Marshal(carrier)
	return string(jsonCarrier)
}

func ExtractTraceContext(ctx context.Context, traceMap map[string]string) context.Context {
	return otel.GetTextMapPropagator().Extract(ctx, propagation.MapCarrier(traceMap))
}

func Trace(ctx context.Context, layer, funcName, spanName string) (context.Context, trace.Span, string) {
	tracer := otel.Tracer(fmt.Sprintf("%s/%s", layer, funcName))
	ctx, span := tracer.Start(ctx, spanName)
	return ctx, span, span.SpanContext().SpanID().String()
}

func Info(ctx context.Context) {
	span := trace.SpanFromContext(ctx)
	sc := span.SpanContext()
	if !sc.IsValid() {
		fmt.Println("No valid span context found")
		return
	}
	fmt.Printf("TraceID: %s, SpanID: %s, TraceFlags: %s\n",
		sc.TraceID().String(),
		sc.SpanID().String(),
		sc.TraceFlags().String(),
	)
}

// dipakai untuk convert trace context json ke map[string]string
func ConvertToMap(raw any) map[string]string {
	traceMap := make(map[string]string)
	switch t := raw.(type) {
	case map[string]any:
		for k, v := range t {
			traceMap[k] = fmt.Sprintf("%v", v)
		}
	case string:
		_ = json.Unmarshal([]byte(t), &traceMap)
	}
	return traceMap
}
