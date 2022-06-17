package main

import (
	"context"
	"flag"
	"os"
	"time"

	"github.com/caarlos0/env/v6"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/shopspring/decimal"

	"github.com/kazauwa/gophermart/internal/gophermart"
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

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = storage.NewPostgres(ctx, cfg.DatabaseURI)
	if err != nil {
		log.Fatal().Err(err).Caller().Msg("Cannot start service")
	}

	app := gophermart.GetGophermartApp(cfg)
	app.Serve()
}
