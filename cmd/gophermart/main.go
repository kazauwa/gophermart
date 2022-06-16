package main

import (
	"context"
	"flag"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/caarlos0/env/v6"
	"github.com/gin-contrib/logger"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/shopspring/decimal"
	"golang.org/x/sync/errgroup"

	"github.com/kazauwa/gophermart/internal/gophermart"
	"github.com/kazauwa/gophermart/internal/handlers"
	"github.com/kazauwa/gophermart/internal/storage"
)

func parseFlags(cfg *gophermart.Config) {
	address := flag.String("a", "localhost:8080", "bind address")
	databaseURI := flag.String("d", "postgres://127.0.0.1:5432/postgres", "database DSN")
	accrualAddress := flag.String("r", "http://localhost:9090", "accrual system address")
	cookieSecret := flag.String("s", "", "secret for encrypting session")
	pollInterval := flag.Duration("p", time.Second*2, "poll interval")

	flag.Parse()
	cfg.RunAddr = *address
	cfg.DatabaseURI = *databaseURI
	cfg.AccrualSystemAddr = *accrualAddress
	cfg.CookieSecret = *cookieSecret
	cfg.PollInterval = *pollInterval
}

func main() {
	zerolog.SetGlobalLevel(zerolog.InfoLevel)
	if gin.IsDebugging() {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	}
	cfg := gophermart.NewConfig()

	decimal.MarshalJSONWithoutQuotes = true

	parseFlags(cfg)
	err := env.Parse(cfg)
	if err != nil {
		log.Fatal().Err(err).Caller().Msg("cannot parse env")
		os.Exit(1)
	}

	router := gin.New()
	router.Use(logger.SetLogger())
	router.Use(gin.Recovery())
	store := cookie.NewStore([]byte(cfg.CookieSecret))
	router.Use(sessions.Sessions("_gophermart_s", store))
	// TODO: configure via env
	err = router.SetTrustedProxies(nil)
	if err != nil {
		log.Fatal().Err(err).Caller().Msg("Cannot start service")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = storage.NewPostgres(ctx, cfg.DatabaseURI)
	if err != nil {
		log.Fatal().Err(err).Caller().Msg("Cannot start service")
	}

	app, err := handlers.GetGophermartApp(cfg, storage.GetDB())
	if err != nil {
		log.Fatal().Err(err).Caller().Msg("Cannot start service")
	}
	app.CreateRouter(router)

	server := &http.Server{
		Addr:    cfg.RunAddr,
		Handler: router,
	}

	go func() {
		err := server.ListenAndServe()
		if err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Caller().Msg("Cannot start service")
		}
	}()

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGTERM, syscall.SIGINT, syscall.SIGQUIT)

	errg, errgCtx := errgroup.WithContext(ctx)

	defer func() {
		if err := errg.Wait(); err != nil {
			log.Fatal().Err(err).Caller().Msg("Fatal error")
		}
	}()

	errg.Go(func() error {
		return app.ScheduleTasks(errgCtx)
	})

	select {
	case <-errgCtx.Done():
		return
	case <-signals:
		log.Info().Msg("Shutting down...")
		cancel()
		ctx, cancelTimeout := context.WithTimeout(context.Background(), time.Second*5)
		defer cancelTimeout()

		err = server.Shutdown(ctx)
		if err != nil {
			log.Fatal().Err(err).Caller().Msg("Unable to shutdown gracefully")
		}
		log.Info().Msg("Exiting")
	}
}
