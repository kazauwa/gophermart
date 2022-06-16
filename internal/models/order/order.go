package order

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/shopspring/decimal"
)

type OrderStatus = string

const (
	Registered OrderStatus = "NEW"
	Invalid    OrderStatus = "INVALID"
	Processing OrderStatus = "PROCESSING"
	Processed  OrderStatus = "PROCESSED"
)

type Order struct {
	ID         int64           `json:"number"`
	UserID     int             `json:"-"`
	Status     OrderStatus     `json:"status"`
	Accrual    decimal.Decimal `json:"accrual,omitempty"`
	UploadedAt time.Time       `json:"uploaded_at"`
}

func New() *Order {
	return &Order{
		Status:     Registered,
		UploadedAt: time.Now(),
	}
}

func (o *Order) MarshalJSON() ([]byte, error) {
	var accrual *decimal.Decimal
	if !o.Accrual.IsZero() {
		accrual = &o.Accrual
	}

	type shadowOrder Order
	return json.Marshal(&struct {
		ID         string           `json:"number"`
		Accrual    *decimal.Decimal `json:"accrual,omitempty"`
		UploadedAt string           `json:"uploaded_at"`
		*shadowOrder
	}{
		ID:          fmt.Sprint(o.ID),
		UploadedAt:  o.UploadedAt.Format(time.RFC3339),
		Accrual:     accrual,
		shadowOrder: (*shadowOrder)(o),
	})
}
