package utils

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/rs/zerolog/log"
	"github.com/shopspring/decimal"
)

type OrderStatus = string

const (
	Registered OrderStatus = "REGISTERED"
	Invalid    OrderStatus = "INVALID"
	Processing OrderStatus = "PROCESSING"
	Processed  OrderStatus = "PROCESSED"
)

type OrderInfo struct {
	OrderID string          `json:"order"`
	Status  OrderStatus     `json:"status"`
	Accrual decimal.Decimal `json:"accrual"`
}

type RateLimitedError struct {
	RetryAfter int
}

func (e *RateLimitedError) Error() string {
	return "request was rate limited"
}

type OrderDoesNotExistError struct {
	OrderID int64
}

func (e *OrderDoesNotExistError) Error() string {
	return fmt.Sprintf("order %d does not exist", e.OrderID)
}

func makeRequest(
	ctx context.Context,
	accrualAddress string,
	client *http.Client,
	orderID int64,
) (*http.Response, error) {
	url := fmt.Sprintf("%s/api/orders/%d", accrualAddress, orderID)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	return response, nil
}

func GetOrderInfo(
	ctx context.Context,
	accrualAddress string,
	client *http.Client,
	orderID int64,
) (*OrderInfo, error) {
	response, err := makeRequest(ctx, accrualAddress, client, orderID)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	switch response.StatusCode {

	case http.StatusTooManyRequests:
		retryAfter, err := strconv.Atoi(response.Header.Get("Retry-After"))
		if err != nil {
			return nil, err
		}

		return nil, &RateLimitedError{RetryAfter: retryAfter}

	case http.StatusInternalServerError:
		return nil, fmt.Errorf("acrrual system returned error")

	case http.StatusNoContent:
		return nil, &OrderDoesNotExistError{OrderID: orderID}

	case http.StatusOK:
		var orderInfo OrderInfo
		decoder := json.NewDecoder(response.Body)
		if err := decoder.Decode(&orderInfo); err != nil {
			return nil, err
		}

		return &orderInfo, nil
	}

	log.Error().Caller().Int(
		"status_code", response.StatusCode,
	).Int64(
		"order_id", orderID,
	).Msg("unkown reponse from accrual")
	return nil, fmt.Errorf("unkown response")
}
