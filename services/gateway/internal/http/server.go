package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Sakayori-Iroha-168/Software_Teamwork/services/gateway/internal/metrics"
	"github.com/Sakayori-Iroha-168/Software_Teamwork/services/gateway/internal/middleware"
	"github.com/Sakayori-Iroha-168/Software_Teamwork/services/gateway/internal/response"
	"github.com/Sakayori-Iroha-168/Software_Teamwork/services/gateway/internal/service"
)

type Config struct {
	Logger               *slog.Logger
	ServiceVersion       string
	Environment          string
	RequestTimeout       time.Duration
	MaxBodyBytes         int64
	CORSAllowedOrigins   []string
	CORSAllowedMethods   []string
	CORSAllowedHeaders   []string
	CORSAllowCredentials bool
	DownstreamTimeout    time.Duration
	InternalServiceToken string
	OwnerBaseURLs        map[string]string
	AuthClient           AuthClient
	SessionStore         service.SessionStore
	TokenHasher          service.TokenHasher
	HTTPClient           *http.Client
	ReadyCheck           func(context.Context) error
	MetricsReg           *metrics.Registry
}

type Server struct {
	logger               *slog.Logger
	serviceVersion       string
	environment          string
	internalServiceToken string
	authClient           AuthClient
	sessionStore         service.SessionStore
	tokenHasher          service.TokenHasher
	ownerBaseURLs        map[string]*url.URL
	httpClient           *http.Client
	streamHTTPClient     *http.Client
	readyCheck           func(context.Context) error
	mux                  *http.ServeMux
	handler              http.Handler
}

func NewServer(cfg Config) *Server {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.DownstreamTimeout <= 0 {
		cfg.DownstreamTimeout = 10 * time.Second
	}
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{Timeout: cfg.DownstreamTimeout}
	}
	s := &Server{
		logger:               cfg.Logger,
		serviceVersion:       cfg.ServiceVersion,
		environment:          cfg.Environment,
		internalServiceToken: strings.TrimSpace(cfg.InternalServiceToken),
		authClient:           cfg.AuthClient,
		sessionStore:         cfg.SessionStore,
		tokenHasher:          cfg.TokenHasher,
		ownerBaseURLs:        parseOwnerBaseURLs(cfg.OwnerBaseURLs),
		httpClient:           cfg.HTTPClient,
		streamHTTPClient:     cloneHTTPClientWithoutTimeout(cfg.HTTPClient),
		readyCheck:           cfg.ReadyCheck,
		mux:                  http.NewServeMux(),
	}
	s.routes()
	s.handler = middleware.Chain(
		s.mux,
		middleware.RequestID(),
		middleware.Metrics(cfg.MetricsReg, s.mux),
		middleware.Recover(cfg.Logger),
		middleware.TimeoutWithSkip(cfg.RequestTimeout, skipsFixedRequestTimeout),
		middleware.CORS(middleware.CORSConfig{
			AllowedOrigins:   cfg.CORSAllowedOrigins,
			AllowedMethods:   cfg.CORSAllowedMethods,
			AllowedHeaders:   cfg.CORSAllowedHeaders,
			AllowCredentials: cfg.CORSAllowCredentials,
		}),
		middleware.BodyLimit(cfg.MaxBodyBytes),
	)
	return s
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /healthz", s.handleHealth)
	s.mux.HandleFunc("GET /readyz", s.handleReady)
	s.mux.HandleFunc("POST /api/v1/users", s.handleCreateUser)
	s.mux.HandleFunc("POST /api/v1/sessions", s.handleCreateSession)
	s.mux.HandleFunc("GET /api/v1/users/me", s.handleCurrentUser)
	s.mux.HandleFunc("DELETE /api/v1/sessions/current", s.handleDeleteCurrentSession)
	for _, route := range activeProxyRoutes {
		route := route
		s.mux.HandleFunc(route.Method+" "+route.Pattern, s.handleProxy(route))
	}
	s.mux.HandleFunc("/", s.handleNotFound)
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.handler.ServeHTTP(w, r)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	response.WriteJSON(w, http.StatusOK, healthResponse{
		Status:      "ok",
		Service:     "gateway",
		Version:     s.serviceVersion,
		Environment: s.environment,
	}, middleware.RequestIDFromContext(r.Context()))
}

func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	if s.readyCheck != nil {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := s.readyCheck(ctx); err != nil {
			s.logger.WarnContext(r.Context(), "gateway dependencies are not ready",
				"service", "gateway",
				"request_id", middleware.RequestIDFromContext(r.Context()),
				"operation", "readyz",
				"status", "failed",
				"error", err,
			)
			response.WriteError(w, http.StatusServiceUnavailable, response.ErrorDetail{
				Code:      response.CodeDependency,
				Message:   "gateway dependencies are not ready",
				RequestID: middleware.RequestIDFromContext(r.Context()),
			})
			return
		}
	}
	response.WriteJSON(w, http.StatusOK, healthResponse{
		Status:      "ready",
		Service:     "gateway",
		Version:     s.serviceVersion,
		Environment: s.environment,
	}, middleware.RequestIDFromContext(r.Context()))
}

func (s *Server) handleNotFound(w http.ResponseWriter, r *http.Request) {
	response.WriteError(w, http.StatusNotFound, response.ErrorDetail{
		Code:      response.CodeNotFound,
		Message:   "route not found",
		RequestID: middleware.RequestIDFromContext(r.Context()),
	})
}

type healthResponse struct {
	Status      string `json:"status"`
	Service     string `json:"service"`
	Version     string `json:"version,omitempty"`
	Environment string `json:"environment,omitempty"`
}

func parseOwnerBaseURLs(values map[string]string) map[string]*url.URL {
	parsed := make(map[string]*url.URL, len(values))
	for owner, raw := range values {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		u, err := url.Parse(raw)
		if err != nil || u.Scheme == "" || u.Host == "" {
			continue
		}
		parsed[owner] = u
	}
	return parsed
}

func cloneHTTPClientWithoutTimeout(client *http.Client) *http.Client {
	if client == nil {
		return &http.Client{}
	}
	cloned := *client
	cloned.Timeout = 0
	return &cloned
}
