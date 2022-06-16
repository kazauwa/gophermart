package models

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/shopspring/decimal"
)

type Withdrawal struct {
	ID          int             `json:"-"`
	OrderID     int64           `json:"order"`
	Sum         decimal.Decimal `json:"sum"`
	ProcessedAt time.Time       `json:"processed_at"`
}

func NewWithdrawal(orderID int64, sum decimal.Decimal, processedAt time.Time) *Withdrawal {
	return &Withdrawal{
		OrderID:     orderID,
		Sum:         sum,
		ProcessedAt: processedAt,
	}
}

func (w *Withdrawal) MarshalJSON() ([]byte, error) {
	type shadowWithdrawal Withdrawal
	return json.Marshal(&struct {
		OrderID     string `json:"order"`
		ProcessedAt string `json:"processed_at"`
		*shadowWithdrawal
	}{
		OrderID:          fmt.Sprint(w.OrderID),
		ProcessedAt:      w.ProcessedAt.Format(time.RFC3339),
		shadowWithdrawal: (*shadowWithdrawal)(w),
	})
}
