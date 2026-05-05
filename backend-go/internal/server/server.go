package server

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/cors"
	"github.com/go-chi/httprate"

	"github.com/batuhan/marketintel/internal/auth"
	"github.com/batuhan/marketintel/internal/dashboard"
	"github.com/batuhan/marketintel/internal/discovery"
	"github.com/batuhan/marketintel/internal/outreach"
	"github.com/batuhan/marketintel/internal/outreach/internalsched"
	"github.com/batuhan/marketintel/internal/platform/middleware"
	"github.com/batuhan/marketintel/internal/scoring"
)

type Server struct {
	router           chi.Router
	authHandler      *auth.Handler
	discoveryHandler *discovery.Handler
	scoringHandler   *scoring.Handler
	dashboardHandler *dashboard.Handler
	outreachHandler  *outreach.Handler
	internalHandler  *internalsched.Handler
	internalToken    string
	authMw           func(http.Handler) http.Handler
}

func New(
	jwtValidator middleware.JWTValidator,
	userFetcher middleware.UserFetcher,
	authHandler *auth.Handler,
	discoveryHandler *discovery.Handler,
	scoringHandler *scoring.Handler,
	dashboardHandler *dashboard.Handler,
	outreachHandler *outreach.Handler,
	internalHandler *internalsched.Handler,
	internalToken string,
	corsOrigins []string,
) *Server {
	s := &Server{
		router:           chi.NewRouter(),
		authHandler:      authHandler,
		discoveryHandler: discoveryHandler,
		scoringHandler:   scoringHandler,
		dashboardHandler: dashboardHandler,
		outreachHandler:  outreachHandler,
		internalHandler:  internalHandler,
		internalToken:    internalToken,
		authMw:           middleware.RequireAuth(jwtValidator, userFetcher),
	}
	s.setupMiddleware(corsOrigins)
	s.setupRoutes()
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.router.ServeHTTP(w, r)
}

func (s *Server) setupMiddleware(corsOrigins []string) {
	s.router.Use(middleware.Recovery)
	s.router.Use(middleware.RequestLogging)
	s.router.Use(cors.Handler(cors.Options{
		AllowedOrigins:   corsOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: true,
		MaxAge:           300,
	}))
	// Global rate limit: 100 requests per minute per IP
	s.router.Use(httprate.LimitByIP(100, time.Minute))
}

func (s *Server) setupRoutes() {
	s.router.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok"}`))
	})

	// Public Gmail OAuth callback — must NOT be behind auth middleware because
	// Google calls it directly. User identification is via the OAuth `state` parameter.
	// Registered BEFORE the authenticated /outreach mount so this specific route wins.
	if s.outreachHandler != nil {
		s.router.Get("/outreach/channels/gmail/callback", s.outreachHandler.ChannelHandler().GmailCallback)
	}

	// Public unsubscribe pages — recipients click these links from their inbox.
	// No JWT; the URL itself carries an HMAC-signed token that proves intent.
	if s.outreachHandler != nil && s.outreachHandler.ComplianceHandler() != nil {
		s.router.Mount("/unsubscribe", s.outreachHandler.ComplianceHandler().Routes())
	}

	// Public auth routes (stricter rate limit: 10 attempts per minute per IP)
	s.router.Route("/auth", func(r chi.Router) {
		r.Use(httprate.LimitByIP(10, time.Minute))
		r.Post("/register", s.authHandler.Register)
		r.Post("/login", s.authHandler.Login)
		r.Post("/refresh", s.authHandler.Refresh)

		r.Group(func(r chi.Router) {
			r.Use(s.authMw)
			r.Get("/me", s.authHandler.Me)
			r.Patch("/me", s.authHandler.UpdateMe)
		})
	})

	// Protected API routes
	s.router.Group(func(r chi.Router) {
		r.Use(s.authMw)

		if s.discoveryHandler != nil {
			r.Mount("/discover", s.discoveryHandler.Routes())
		}
		if s.scoringHandler != nil {
			r.Mount("/markets", s.scoringHandler.MarketRoutes())
			r.Mount("/scoring", s.scoringHandler.ScoringRoutes())
		}
		if s.dashboardHandler != nil {
			r.Mount("/dashboard", s.dashboardHandler.Routes())
		}
		if s.outreachHandler != nil {
			r.Mount("/outreach", s.outreachHandler.Routes())
		}
	})

	// /internal/* — protected by a shared-secret header rather than user JWT.
	// Cloud Scheduler hits POST /internal/scheduler/tick every minute.
	if s.internalHandler != nil {
		s.router.Group(func(r chi.Router) {
			r.Use(internalsched.AuthMiddleware(s.internalToken))
			r.Mount("/internal", s.internalHandler.Routes())
		})
	}
}
