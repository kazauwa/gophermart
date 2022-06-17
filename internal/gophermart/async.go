package gophermart

import (
	"context"
	"errors"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/rs/zerolog/log"

	"github.com/kazauwa/gophermart/internal/models"
	"github.com/kazauwa/gophermart/internal/utils"
)

func (g *Gophermart) ScheduleTasks(ctx context.Context) error {
	pollTicker := time.NewTicker(g.cfg.PollInterval)
	defer pollTicker.Stop()
	errg, innerCtx := errgroup.WithContext(ctx)

	events := make(chan struct{})
	errg.Go(func() error {
		for range events {
			if err := g.updateUserBalance(innerCtx); err != nil {
				return err
			}
		}
		return nil
	})

	for {
		select {
		case <-pollTicker.C:
			select {
			case events <- struct{}{}:
			default:
			}
		case <-innerCtx.Done():
			close(events)
			err := errg.Wait()
			return err
		}
	}
}

func (g *Gophermart) updateUserBalance(ctx context.Context) error {
	orders, err := models.GetUnprocessedOrders(ctx)
	if err != nil {
		return err
	}

	if len(orders) == 0 {
		return nil
	}

	for _, order := range orders {
		orderInfo, err := utils.GetOrderInfo(ctx, g.cfg.AccrualSystemAddr, g.client, order.ID)
		var rateLimitedError *utils.RateLimitedError
		var orderDoesNotExistError *utils.OrderDoesNotExistError

		switch {
		case errors.As(err, &rateLimitedError):
			time.Sleep(time.Duration(rateLimitedError.RetryAfter))
		case errors.As(err, &orderDoesNotExistError):
			continue
		case err != nil:
			log.Err(err).Caller().Msg("error accessing accrual system")
			return err
		}

		switch orderInfo.Status {
		case utils.Invalid:
			if err := order.SetFailed(ctx, order.ID); err != nil {
				log.Err(err).Caller().Msg("error processing order")
				return err
			}

		case utils.Processed:
			user := models.NewUser()
			err := user.GetByID(ctx, order.UserID)
			if err != nil {
				log.Err(err).Caller().Msg("error fetching user")
				return err
			}

			if err := user.Deposit(ctx, order.ID, orderInfo.Accrual); err != nil {
				log.Err(err).Caller().Msg("error depositing points to user balance")
				return err
			}
		default:
		}
	}
	return nil
}
