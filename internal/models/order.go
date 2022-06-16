package models

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/shopspring/decimal"

	"github.com/kazauwa/gophermart/internal/storage"
)

type OrderStatus = string

const (
	New        OrderStatus = "NEW"
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

func NewOrder() *Order {
	return &Order{
		Status:     New,
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

func (o *Order) Insert(ctx context.Context) error {
	db := storage.GetDB()
	db.Lock.Lock()
	defer db.Lock.Unlock()

	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return err
	}

	var rollbackErr error
	defer func() {
		rollbackErr = tx.Rollback(ctx)
	}()

	insertQuery := "INSERT INTO orders (id, user_id) VALUES ($1, $2)"

	if _, err = tx.Exec(ctx, insertQuery, o.ID, o.UserID); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}
	return rollbackErr
}

func (o *Order) SetFailed(ctx context.Context, orderID int64) error {
	db := storage.GetDB()
	db.Lock.RLock()
	defer db.Lock.RUnlock()

	orderUpdateQuery := `UPDATE orders SET status = 'FAILED' WHERE id = $1`
	if _, err := db.Pool.Exec(ctx, orderUpdateQuery, orderID); err != nil {
		return err
	}

	return nil
}

func (o *Order) GetByID(ctx context.Context, orderID int64) error {
	db := storage.GetDB()
	db.Lock.RLock()
	defer db.Lock.RUnlock()

	err := db.Pool.QueryRow(
		ctx,
		"SELECT id, user_id, status, accrual, uploaded_at FROM orders WHERE id = $1",
		orderID,
	).Scan(&o.ID, &o.UserID, &o.Status, &o.Accrual, &o.UploadedAt)
	if err != nil {
		return err
	}

	return nil
}

func GetUnprocessedOrders(ctx context.Context) ([]*Order, error) {
	db := storage.GetDB()
	db.Lock.RLock()
	defer db.Lock.RUnlock()

	var nOrders int
	err := db.Pool.QueryRow(
		ctx,
		"SELECT count(*) FROM orders WHERE status IN ('NEW', 'PROCESSING')",
	).Scan(&nOrders)
	if err != nil {
		return nil, err
	}

	orders := make([]*Order, 0, nOrders)
	selectQuery := `SELECT id, user_id, status, accrual, uploaded_at
					FROM orders
					WHERE status IN ('NEW', 'PROCESSING')`
	rows, err := db.Pool.Query(ctx, selectQuery)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		order := NewOrder()
		err = rows.Scan(
			&order.ID,
			&order.UserID,
			&order.Status,
			&order.Accrual,
			&order.UploadedAt,
		)
		if err != nil {
			return nil, err
		}
		orders = append(orders, order)
	}
	return orders, nil
}
