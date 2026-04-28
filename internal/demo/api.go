package demo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type APIServer struct {
	Store  *Store
	Broker *Broker
	Logger *slog.Logger
	system SystemStatus
}

func NewAPIServer(store *Store, broker *Broker, system SystemStatus, logger *slog.Logger) *APIServer {
	if logger == nil {
		logger = slog.Default()
	}
	return &APIServer{
		Store:  store,
		Broker: broker,
		Logger: logger,
		system: system,
	}
}

func (server *APIServer) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", server.handleHealth)
	mux.HandleFunc("GET /api/snapshot", server.handleSnapshot)
	mux.HandleFunc("GET /api/traces", server.handleTraces)
	mux.HandleFunc("GET /api/traces/", server.handleTrace)
	mux.HandleFunc("GET /api/events", server.handleEvents)
	mux.HandleFunc("GET /api/dlq", server.handleDLQ)
	mux.HandleFunc("GET /api/metrics", server.handleMetrics)
	mux.HandleFunc("GET /api/stream", server.handleStream)
	mux.HandleFunc("POST /api/events", server.handlePublishEvent)
	mux.HandleFunc("POST /api/demo/sample", server.handleSample)
	mux.HandleFunc("POST /api/demo/burst", server.handleBurst)
	mux.HandleFunc("POST /api/demo/failure", server.handleFailure)
	mux.HandleFunc("POST /api/demo/reset", server.handleReset)
	return withCORS(mux)
}

func (server *APIServer) handleHealth(writer http.ResponseWriter, request *http.Request) {
	writeJSON(writer, http.StatusOK, map[string]string{"status": "ok"})
}

func (server *APIServer) handleSnapshot(writer http.ResponseWriter, request *http.Request) {
	writeJSON(writer, http.StatusOK, server.Store.Snapshot(server.system))
}

func (server *APIServer) handleTraces(writer http.ResponseWriter, request *http.Request) {
	writeJSON(writer, http.StatusOK, server.Store.Snapshot(server.system).Traces)
}

func (server *APIServer) handleTrace(writer http.ResponseWriter, request *http.Request) {
	traceID := strings.TrimPrefix(request.URL.Path, "/api/traces/")
	if traceID == "" {
		http.Error(writer, "trace id is required", http.StatusBadRequest)
		return
	}
	for _, trace := range server.Store.Snapshot(server.system).Traces {
		if trace.TraceID == traceID {
			writeJSON(writer, http.StatusOK, trace)
			return
		}
	}
	http.NotFound(writer, request)
}

func (server *APIServer) handleEvents(writer http.ResponseWriter, request *http.Request) {
	writeJSON(writer, http.StatusOK, server.Store.Snapshot(server.system).Events)
}

func (server *APIServer) handleDLQ(writer http.ResponseWriter, request *http.Request) {
	writeJSON(writer, http.StatusOK, server.Store.Snapshot(server.system).DLQ)
}

func (server *APIServer) handleMetrics(writer http.ResponseWriter, request *http.Request) {
	writeJSON(writer, http.StatusOK, server.Store.Snapshot(server.system).Metrics)
}

func (server *APIServer) handlePublishEvent(writer http.ResponseWriter, request *http.Request) {
	var event MediaDeliveryEvent
	if err := json.NewDecoder(request.Body).Decode(&event); err != nil {
		http.Error(writer, fmt.Sprintf("decode event: %v", err), http.StatusBadRequest)
		return
	}
	if err := server.publish(request.Context(), event); err != nil {
		http.Error(writer, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(writer, http.StatusAccepted, event)
}

func (server *APIServer) handleSample(writer http.ResponseWriter, request *http.Request) {
	event := NewSampleEvent(1)
	if err := server.publish(request.Context(), event); err != nil {
		http.Error(writer, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(writer, http.StatusAccepted, event)
}

func (server *APIServer) handleBurst(writer http.ResponseWriter, request *http.Request) {
	count := 5
	if rawCount := request.URL.Query().Get("count"); rawCount != "" {
		parsed, err := strconv.Atoi(rawCount)
		if err != nil || parsed < 1 || parsed > 25 {
			http.Error(writer, "count must be an integer from 1 to 25", http.StatusBadRequest)
			return
		}
		count = parsed
	}

	events := make([]MediaDeliveryEvent, 0, count)
	for index := 0; index < count; index++ {
		event := NewSampleEvent(index + 1)
		if err := server.publish(request.Context(), event); err != nil {
			http.Error(writer, err.Error(), http.StatusBadRequest)
			return
		}
		events = append(events, event)
	}
	writeJSON(writer, http.StatusAccepted, events)
}

func (server *APIServer) handleFailure(writer http.ResponseWriter, request *http.Request) {
	event := NewSampleEvent(99)
	event.AssetID = "asset_fail-process_1099"
	event.CorrelationID = "corr_fail_process_" + strconv.FormatInt(time.Now().UnixNano(), 10)
	if err := server.publish(request.Context(), event); err != nil {
		http.Error(writer, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(writer, http.StatusAccepted, event)
}

func (server *APIServer) handleReset(writer http.ResponseWriter, request *http.Request) {
	if err := server.Store.Reset(); err != nil {
		http.Error(writer, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]string{"status": "reset"})
}

func (server *APIServer) handleStream(writer http.ResponseWriter, request *http.Request) {
	writer.Header().Set("Content-Type", "text/event-stream")
	writer.Header().Set("Cache-Control", "no-cache")
	writer.Header().Set("Connection", "keep-alive")
	writer.Header().Set("X-Accel-Buffering", "no")

	flusher, ok := writer.(http.Flusher)
	if !ok {
		http.Error(writer, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	updates, cancel := server.Store.Subscribe()
	defer cancel()

	sendSnapshot := func() error {
		raw, err := json.Marshal(server.Store.Snapshot(server.system))
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintf(writer, "event: snapshot\ndata: %s\n\n", raw); err != nil {
			return err
		}
		flusher.Flush()
		return nil
	}

	if err := sendSnapshot(); err != nil {
		return
	}

	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-request.Context().Done():
			return
		case <-updates:
			if err := sendSnapshot(); err != nil {
				return
			}
		case <-ticker.C:
			if _, err := writer.Write([]byte(": heartbeat\n\n")); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func (server *APIServer) publish(ctx context.Context, event MediaDeliveryEvent) error {
	if server.Broker == nil {
		return errors.New("broker is unavailable")
	}
	event.ApplyDefaults()
	if err := server.Store.RecordPublished(event); err != nil {
		return err
	}
	_, err := server.Broker.Publish(event)
	return err
}

func writeJSON(writer http.ResponseWriter, statusCode int, value any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(statusCode)
	_ = json.NewEncoder(writer).Encode(value)
}

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Access-Control-Allow-Origin", "*")
		writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		writer.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if request.Method == http.MethodOptions {
			writer.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(writer, request)
	})
}
