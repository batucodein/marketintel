package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/joho/godotenv"

	"github.com/batuhan/marketintel/internal/auth"
	"github.com/batuhan/marketintel/internal/config"
	"github.com/batuhan/marketintel/internal/dashboard"
	"github.com/batuhan/marketintel/internal/datasource"
	"github.com/batuhan/marketintel/internal/discovery"
	"github.com/batuhan/marketintel/internal/domain"
	"github.com/batuhan/marketintel/internal/outreach"
	outreachcampaign "github.com/batuhan/marketintel/internal/outreach/campaign"
	outreachchannel "github.com/batuhan/marketintel/internal/outreach/channel"
	gmailmailer "github.com/batuhan/marketintel/internal/outreach/channel/gmail"
	imapsmtp "github.com/batuhan/marketintel/internal/outreach/channel/imapsmtp"
	outreachcompliance "github.com/batuhan/marketintel/internal/outreach/compliance"
	outreachcontact "github.com/batuhan/marketintel/internal/outreach/contact"
	outreachconv "github.com/batuhan/marketintel/internal/outreach/conversation"
	outreachcontactgroup "github.com/batuhan/marketintel/internal/outreach/contactgroup"
	outreachcrm "github.com/batuhan/marketintel/internal/outreach/crm"
	outreachevents "github.com/batuhan/marketintel/internal/outreach/events"
	outreachgroup "github.com/batuhan/marketintel/internal/outreach/group"
	outreachsimulation "github.com/batuhan/marketintel/internal/outreach/simulation"
	"github.com/batuhan/marketintel/internal/outreach/internalsched"
	outreachleadctx "github.com/batuhan/marketintel/internal/outreach/leadctx"
	outreachpoller "github.com/batuhan/marketintel/internal/outreach/poller"
	outreachsender "github.com/batuhan/marketintel/internal/outreach/sender"
	outreachsequence "github.com/batuhan/marketintel/internal/outreach/sequence"
	"github.com/batuhan/marketintel/internal/platform/ai"
	"github.com/batuhan/marketintel/internal/platform/ailog"
	"github.com/batuhan/marketintel/internal/platform/crypto"
	"github.com/batuhan/marketintel/internal/platform/db"
	"github.com/batuhan/marketintel/internal/platform/oauth"
	"github.com/batuhan/marketintel/internal/scoring"
	"github.com/batuhan/marketintel/internal/server"
	"github.com/batuhan/marketintel/migrations"
)

func main() {
	_ = godotenv.Load()

	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: parseLogLevel("INFO"),
	})))

	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: parseLogLevel(cfg.LogLevel),
	})))

	if err := cfg.Validate(); err != nil {
		slog.Warn("config validation", "warning", err)
	}

	slog.Info("starting server",
		"app", cfg.AppName,
		"env", cfg.AppEnv,
		"port", cfg.Port,
	)

	// Database
	ctx := context.Background()
	pool, err := db.NewPool(ctx, cfg.PostgresDSN())
	if err != nil {
		slog.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	// Migrations
	if err := db.RunMigrations(cfg.PostgresDSN(), migrations.FS()); err != nil {
		slog.Error("failed to run migrations", "error", err)
		os.Exit(1)
	}

	// Auth module
	userRepo := auth.NewUserRepository(pool)
	jwtMgr := auth.NewJWTManager(cfg.SecretKey, cfg.AccessTokenExpireMinutes, cfg.RefreshTokenExpireDays)
	authSvc := auth.NewService(userRepo, jwtMgr)
	authHandler := auth.NewHandler(authSvc)
	userFetcher := auth.NewUserFetcherAdapter(userRepo)

	// AI providers
	providers := make(map[string]ai.Provider)
	if cfg.AzureOpenAIAPIKey != "" && cfg.AzureOpenAIEndpoint != "" {
		providers["azure_openai"] = ai.NewAzureOpenAI(cfg.AzureOpenAIAPIKey, cfg.AzureOpenAIEndpoint)
		slog.Info("ai: azure_openai configured")
	}
	if cfg.AnthropicAPIKey != "" {
		providers["anthropic"] = ai.NewClaude(cfg.AnthropicAPIKey)
		slog.Info("ai: anthropic configured")
	}
	if cfg.GoogleAIAPIKey != "" {
		providers["google"] = ai.NewGemini(cfg.GoogleAIAPIKey)
		slog.Info("ai: google configured")
	}

	var aiCache *ai.Cache
	if cfg.RedisURL != "" {
		c, err := ai.NewCache(cfg.RedisURL)
		if err != nil {
			slog.Warn("ai cache unavailable", "error", err)
		} else {
			aiCache = c
			defer c.Close()
		}
	}

	aiRouter := ai.NewRouter(providers, ai.DefaultTasks(), aiCache)

	// AI request logging
	aiLogRepo := ailog.NewRepository(pool)
	aiRouter.SetLogFunc(func(ctx context.Context, result *ai.AIResult, userID, searchID *uuid.UUID) {
		log := &domain.AIRequestLog{
			ID:          uuid.New(),
			UserID:      userID,
			SearchID:    searchID,
			RequestType: result.RequestType,
			Provider:    result.Provider,
			Model:       result.Model,
			InputTokens: &result.InputTokens,
			OutputTokens: &result.OutputTokens,
			CostUSD:     &result.CostUSD,
			LatencyMS:   &result.LatencyMS,
			CacheHit:    result.CacheHit,
		}
		if err := aiLogRepo.Create(ctx, log); err != nil {
			slog.Warn("failed to log AI request", "error", err)
		}
	})

	// Data source connectors
	var places domain.BusinessFinder
	if cfg.GooglePlacesAPIKey != "" {
		places = datasource.NewGooglePlaces(cfg.GooglePlacesAPIKey)
		slog.Info("datasource: google places configured")
	}

	// Scoring module
	scoringRepo := scoring.NewRepository(pool)

	// Discovery module — Excel-only flow
	discRepo := discovery.NewRepository(pool)
	classifier := discovery.NewClassifier(aiRouter)
	scorer := discovery.NewScorer(aiRouter)
	pipeline := discovery.NewPipeline(aiRouter, places, classifier, scorer, discRepo)
	pipeline.ConfigureScraper(cfg.JinaReaderBaseURL, cfg.ScraperTier2ThresholdBytes)
	mapper := discovery.NewAIMapper(aiRouter)
	discoveryHandler := discovery.NewHandler(pipeline, discRepo, mapper)
	scoringHandler := scoring.NewHandler(scoringRepo)

	// Dashboard module
	dashboardHandler := dashboard.NewHandler(pool, aiLogRepo)

	// Outreach module (CRM + email)
	tokenCipher, err := crypto.NewFromKeyOrSecret(cfg.TokenEncryptionKey, cfg.SecretKey)
	if err != nil {
		slog.Error("failed to init token cipher", "error", err)
		os.Exit(1)
	}
	var gmailOAuth *oauth.GmailOAuth
	if cfg.GmailClientID != "" && cfg.GmailClientSecret != "" {
		gmailOAuth = oauth.NewGmailOAuth(cfg.GmailClientID, cfg.GmailClientSecret, cfg.GmailOAuthRedirectURL)
		outreachchannel.DefaultRegistry.Register(domain.ChannelTypeGmailOAuth, gmailmailer.NewFactory(gmailOAuth, tokenCipher))
		slog.Info("gmail oauth configured")
	} else {
		slog.Warn("gmail oauth not configured — set GMAIL_CLIENT_ID and GMAIL_CLIENT_SECRET to enable")
	}

	// Generic IMAP/SMTP mailbox channel — lets users connect any custom-domain /
	// any-provider account with stored (encrypted) credentials. No external app
	// creds needed; the factory just decrypts per-channel config.
	outreachchannel.DefaultRegistry.Register(domain.ChannelTypeSMTP, imapsmtp.NewFactory(tokenCipher))

	// CORS origins list is comma-or-json; take the first HTTP/HTTPS one for the
	// post-OAuth redirect target. Fallback to localhost:3000.
	frontendURL := "http://localhost:3000"
	for _, o := range cfg.ParsedCORSOrigins() {
		if strings.HasPrefix(o, "http") {
			frontendURL = o
			break
		}
	}

	senderRepo := outreachsender.NewRepository(pool)
	senderHandler := outreachsender.NewHandler(senderRepo)

	// Plug sender profile into the discovery scorer so leads are ranked
	// against the user's specific positioning (target industries/countries,
	// deal-size band, deal breakers, moats) on top of market fit.
	pipeline.SetSenderLoader(senderRepo)
	contactRepo := outreachcontact.NewRepository(pool)
	contactHandler := outreachcontact.NewHandler(contactRepo)
	channelRepo := outreachchannel.NewRepository(pool)
	channelHandler := outreachchannel.NewHandler(channelRepo, gmailOAuth, tokenCipher, imapsmtp.NewConnector(tokenCipher), frontendURL)
	convRepo := outreachconv.NewRepository(pool)
	convService := outreachconv.NewService(convRepo, channelRepo, contactRepo, senderRepo, outreachchannel.DefaultRegistry, aiRouter, pool)
	convHandler := outreachconv.NewHandler(convRepo, convService)

	// Shared lead-context loader — used by conversation, campaign drafter,
	// and (P3) sequence engine to assemble prompt inputs.
	leadCtxLoader := outreachleadctx.NewLoader(pool)

	// Compliance — public unsubscribe handler + helpers used by senders.
	complianceHandler := outreachcompliance.NewHandler(channelRepo, contactRepo)

	// Campaigns (P2) — handler, drafter, scheduler.
	campaignRepo := outreachcampaign.NewRepository(pool)
	campaignSvc := outreachcampaign.NewService(
		campaignRepo, channelRepo, contactRepo, senderRepo, convRepo,
		outreachchannel.DefaultRegistry, aiRouter, leadCtxLoader, pool,
		cfg.PublicAPIURL,
	)
	campaignHandler := outreachcampaign.NewHandler(campaignRepo, campaignSvc)

	// CRM extras (P4) — tasks + notes. Constructed before the sequence
	// engine because the engine records notify_user actions as task rows.
	crmRepo := outreachcrm.NewRepository(pool)
	crmHandler := outreachcrm.NewHandler(crmRepo)

	// Sequences (P3) — handler + engine.
	sequenceRepo := outreachsequence.NewRepository(pool)
	sequenceSvc := outreachsequence.NewService(sequenceRepo)
	sequenceHandler := outreachsequence.NewHandler(sequenceRepo, sequenceSvc)
	sequenceEngine := outreachsequence.NewEngine(
		sequenceRepo, convRepo, contactRepo, channelRepo, senderRepo, crmRepo,
		outreachchannel.DefaultRegistry, aiRouter, leadCtxLoader, pool,
		cfg.PublicAPIURL,
	)

	// Bind the campaign service's sequence starter so that successful
	// campaign sends kick off a follow-up run when sequence_id is set.
	campaignSvc.SetSequenceStarter(sequenceSvc)
	// Let the draft assistant rewrite reply drafts via the conversation
	// reply prompt + guard (campaign already imports conversation; no cycle).
	campaignSvc.SetConvRefiner(convService)

	// Contact groups — named collections of contacts with a brand.
	contactGroupRepo := outreachcontactgroup.NewRepository(pool)
	contactGroupSvc := outreachcontactgroup.NewService(contactGroupRepo, contactRepo)
	contactGroupHandler := outreachcontactgroup.NewHandler(contactGroupRepo, contactGroupSvc, contactRepo)

	// Email Groups (P5) — facade over campaign + sequence + contact(group).
	groupSvc := outreachgroup.NewService(campaignSvc, campaignRepo, sequenceSvc, contactRepo, contactGroupRepo, pool)
	groupHandler := outreachgroup.NewHandler(groupSvc)

	// Realtime SSE broker — poller publishes inbound events here.
	eventBroker := outreachevents.NewBroker()
	eventsHandler := outreachevents.NewHandler(eventBroker)
	groupSvc.SetBroker(eventBroker) // directive bulk-apply progress events

	// Simulation Lab — AI-persona test runs. Self-contained: no real sends.
	simulationRepo := outreachsimulation.NewRepository(pool)
	simulationSvc := outreachsimulation.NewService(simulationRepo, scoringRepo, leadCtxLoader, senderRepo, aiRouter, eventBroker)
	simulationHandler := outreachsimulation.NewHandler(simulationRepo, simulationSvc)

	outreachHandler := outreach.NewHandler(senderHandler, contactHandler, channelHandler, convHandler, campaignHandler, sequenceHandler, groupHandler, contactGroupHandler, simulationHandler, crmHandler, eventsHandler, complianceHandler)

	// Inbound poller — wrapped as a Tickable component instead of running as
	// a long-lived goroutine. Cloud Scheduler drives the cadence in prod via
	// POST /internal/scheduler/tick; locally a dev can hit the endpoint with
	// the same INTERNAL_API_TOKEN, or curl /internal/scheduler/tick on demand.
	mailPoller := outreachpoller.NewPoller(channelRepo, outreachchannel.DefaultRegistry, convRepo, contactRepo, eventBroker, aiRouter, pool, 2*time.Minute)

	internalHandler := internalsched.NewHandler()
	internalHandler.Register(&internalsched.PollerComponent{P: mailPoller})
	internalHandler.Register(outreachcampaign.NewDrafter(campaignSvc))
	internalHandler.Register(outreachcampaign.NewScheduler(campaignSvc))
	internalHandler.Register(sequenceEngine)

	internalToken := cfg.InternalAPIToken
	if internalToken == "" {
		// Dev fallback: derive from SecretKey so /internal/* still works locally
		// without requiring an extra env var.
		internalToken = "dev-" + cfg.SecretKey
		slog.Info("INTERNAL_API_TOKEN not set — using dev fallback derived from SECRET_KEY")
	}

	// Server
	srv := server.New(
		jwtMgr, userFetcher,
		authHandler, discoveryHandler, scoringHandler, dashboardHandler, outreachHandler,
		internalHandler, internalToken,
		cfg.ParsedCORSOrigins(),
	)

	httpSrv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Port),
		Handler:      srv,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 120 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Graceful shutdown
	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		slog.Info("listening", "addr", httpSrv.Addr)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	<-shutdown
	slog.Info("shutting down gracefully...")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := httpSrv.Shutdown(ctx); err != nil {
		slog.Error("forced shutdown", "error", err)
	}

	pool.Close()
	slog.Info("server stopped")
}

func parseLogLevel(level string) slog.Level {
	switch strings.ToUpper(level) {
	case "DEBUG":
		return slog.LevelDebug
	case "WARN", "WARNING":
		return slog.LevelWarn
	case "ERROR":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

