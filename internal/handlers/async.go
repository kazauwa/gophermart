package handlers

import (
	"context"
	"errors"
	"net/http"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/rs/zerolog/log"

	"github.com/kazauwa/gophermart/internal/gophermart"
	"github.com/kazauwa/gophermart/internal/storage"
	"github.com/kazauwa/gophermart/internal/utils"
)

func (g *Gophermart) ScheduleTasks(ctx context.Context) error {
	pollTicker := time.NewTicker(g.cfg.PollInterval)
	defer pollTicker.Stop()

	errg, innerCtx := errgroup.WithContext(ctx)

	pollWorker := utils.NewWorker()

	client := utils.NewHTTPClient()

	pollWorker.RegisterFunc(func() error {
		if err := updateUserBalance(innerCtx, g.cfg, client); err != nil {
			return err
		}
		return nil
	})
	errg.Go(pollWorker.Listen)

	for {
		select {
		case <-pollTicker.C:
			pollWorker.Do()
		case <-innerCtx.Done():
			pollWorker.Stop()
			err := errg.Wait()
			return err
		}
	}
}

func updateUserBalance(ctx context.Context, cfg *gophermart.Config, client *http.Client) error {
	db := storage.GetDB()
	orders, err := db.GetUnprocessedOrders(ctx)
	if err != nil {
		return err
	}

	if len(orders) == 0 {
		return nil
	}

	for _, order := range orders {
		orderInfo, err := utils.GetOrderInfo(ctx, cfg.AccrualSystemAddr, client, order.ID)
		var rateLimitedError *utils.RateLimitedError
		var orderDoesNotExistError *utils.OrderDoesNotExistError

		switch {
		case errors.As(err, &rateLimitedError): // TODO
			continue
		case errors.As(err, &orderDoesNotExistError): // TODO
			continue
		case err != nil:
			log.Err(err).Caller().Msg("error accessing accrual system")
			return err
		}

		switch orderInfo.Status {
		case utils.Invalid:
			if err := db.SetOrderFailed(ctx, order.ID); err != nil {
				log.Err(err).Caller().Msg("error processing order")
				return err
			}

		case utils.Processed:
			user, err := db.GetUserByID(ctx, order.UserID)
			if err != nil {
				log.Err(err).Caller().Msg("error fetching user")
				return err
			}

			if err := user.Deposit(orderInfo.Accrual); err != nil {
				log.Err(err).Caller().Msg("error depositing points to user balance")
				return err
			}

			if err := db.Deposit(ctx, user, order.ID, orderInfo.Accrual); err != nil {
				log.Err(err).Caller().Msg("error depositing points to user balance")
				return err
			}
		default:
		}
	}
	return nil
}
