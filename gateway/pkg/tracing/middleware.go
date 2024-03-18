package tracing

import (
	"context"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	tracesdk "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
	"go.opentelemetry.io/otel/trace"
)

type Exporter string

const (
	OTELExporter     Exporter = "otlp"
	DisabledExporter Exporter = "disabled"
)

const (
	otelEnvPropagators            = "OTEL_PROPAGATORS"
	otelEnvTraceSExporter         = "OTEL_TRACES_EXPORTER"
	otelEnvExporterLogPrettyPrint = "OTEL_EXPORTER_LOG_PRETTY_PRINT"
	otelEnvExporterLogTimestamps  = "OTEL_EXPORTER_LOG_TIMESTAMPS"
	otelEnvServiceName            = "OTEL_SERVICE_NAME"
	otelExpOTLPProtocol           = "OTEL_EXPORTER_OTLP_PROTOCOL"
)

type Shutdown func(context.Context)

func Provider(ctx context.Context, name, version, commit string) (shutdown Shutdown, err error) {
	var exporter Exporter
	if val, exists := os.LookupEnv(otelEnvTraceSExporter); exists {
		exporter = Exporter(val)
	} else {
		exporter = DisabledExporter
	}

	var exp tracesdk.TracerProviderOption
	switch exporter {
	case OTELExporter:
		// find available env variables for configuration
		// see: https://github.com/open-telemetry/opentelemetry-go/tree/main/exporters/otlp/otlptrace#environment-variables
		kind := get(otelExpOTLPProtocol, "grpc")

		var client tracesdk.SpanExporter
		switch kind {
		case "grpc":
			client, err = otlptracegrpc.New(ctx)
		case "http":
			client, err = otlptracehttp.New(ctx)
		}
		exp = tracesdk.WithBatcher(client)
	default:
		log.Println("tracing disabled")
		// We explicitly DO NOT set the global TracerProvider using otel.SetTracerProvider().
		// The unset TracerProvider returns a "non-recording" span, but still passes through context.
		// return no-op shutdown function
		return func(_ context.Context) {}, nil
	}
	if err != nil {
		return nil, err
	}

	propagators := strings.ToLower(get(otelEnvPropagators, "tracecontext,baggage"))
	otel.SetTextMapPropagator(
		propagation.NewCompositeTextMapPropagator(withPropagators(propagators)...),
	)

	resource, err := resource.New(
		context.Background(),
		resource.WithFromEnv(),
		resource.WithHost(),
		resource.WithOS(),
		resource.WithTelemetrySDK(),
		resource.WithAttributes(
			semconv.ServiceVersionKey.String(version),
			attribute.String("service.commit", commit),
			semconv.ServiceNameKey.String(get(otelEnvServiceName, name)),
		),
	)
	if err != nil {
		return nil, err
	}

	provider := tracesdk.NewTracerProvider(
		// Always be sure to batch in production.
		exp,
		tracesdk.WithResource(resource),
		tracesdk.WithSampler(tracesdk.AlwaysSample()),
	)

	// Register our TracerProvider as the global so any imported
	// instrumentation in the future will default to using it.
	otel.SetTracerProvider(provider)

	shutdown = func(ctx context.Context) {
		// Do not let the application hang forever when it is shutdown.
		ctx, cancel := context.WithTimeout(ctx, time.Second*5)
		defer cancel()

		err := provider.Shutdown(ctx)
		if err != nil {
			log.Printf("failed to shutdown tracing provider: %v", err)
		}
	}

	return shutdown, nil
}

func Middleware(next http.HandlerFunc) http.HandlerFunc {
	_, ok := os.LookupEnv("OTEL_EXPORTER")
	if !ok {
		return next
	}
	log.Println("configuring proxy tracing middleware")

	propagator := otel.GetTextMapPropagator()

	return func(w http.ResponseWriter, r *http.Request) {
		// get the parent span from the request headers
		ctx := propagator.Extract(r.Context(), propagation.HeaderCarrier(r.Header))
		opts := []trace.SpanStartOption{
			trace.WithSpanKind(trace.SpanKindServer),
		}

		ctx, span := otel.Tracer("Gateway").Start(ctx, r.URL.Path, opts...)
		defer span.End()

		r = r.WithContext(ctx)
		// set the new span as the parent span in the outgoing request context
		// note that this will overwrite the uber-trace-id and traceparent headers
		propagator.Inject(ctx, propagation.HeaderCarrier(r.Header))
		next(w, r)
	}
}

func get(name, defaultValue string) string {
	value, ok := os.LookupEnv(name)
	if !ok {
		return defaultValue
	}
	return value
}

func withPropagators(propagators string) []propagation.TextMapPropagator {
	out := []propagation.TextMapPropagator{}

	if strings.Contains(propagators, "tracecontext") {
		out = append(out, propagation.TraceContext{})
	}

	if strings.Contains(propagators, "baggage") {
		out = append(out, propagation.Baggage{})
	}

	return out
}
