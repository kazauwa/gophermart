package gophermart

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-contrib/logger"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
	"golang.org/x/sync/errgroup"
)

type Gophermart struct {
	cfg    *Config
	client *http.Client
}

func newHTTPClient() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConns = 100
	transport.MaxConnsPerHost = 100
	transport.MaxIdleConnsPerHost = 100

	return &http.Client{
		Timeout:   10 * time.Second,
		Transport: transport,
	}
}

func GetGophermartApp(cfg *Config) *Gophermart {
	return &Gophermart{
		cfg:    cfg,
		client: newHTTPClient(),
	}
}

func (g *Gophermart) Serve() {
	router := gin.New()
	router.Use(logger.SetLogger())
	router.Use(gin.Recovery())
	store := cookie.NewStore([]byte(g.cfg.CookieSecret))
	router.Use(sessions.Sessions("_gophermart_s", store))
	// TODO: configure via env
	err := router.SetTrustedProxies(nil)
	if err != nil {
		log.Fatal().Err(err).Caller().Msg("Cannot start service")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	g.CreateRouter(router)

	server := &http.Server{
		Addr:    g.cfg.RunAddr,
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
		return g.ScheduleTasks(errgCtx)
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
