package models

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/alexedwards/argon2id"
	"github.com/shopspring/decimal"

	"github.com/kazauwa/gophermart/internal/storage"
)

var ErrInsufficientBalance = errors.New("insufficient balance")

type User struct {
	ID       int             `json:"-"`
	Balance  decimal.Decimal `json:"balance"`
	Login    string          `json:"login"`
	password string
}

func NewUser() *User {
	return &User{}
}

func (u *User) SetPassword(password string) error {
	// TODO: прокидывать параметры из конфига
	passwordHash, err := argon2id.CreateHash(password, argon2id.DefaultParams)
	if err != nil {
		return err
	}
	u.password = passwordHash
	return nil
}

func (u *User) CheckPassword(password string) (bool, error) {
	return argon2id.ComparePasswordAndHash(password, u.password)
}

func (u *User) withdraw(sum decimal.Decimal) error {
	if sum.IsNegative() || sum.IsZero() {
		return fmt.Errorf("incorrect withdrawal amount")
	}

	if sum.GreaterThan(u.Balance) {
		return ErrInsufficientBalance
	}
	u.Balance = u.Balance.Sub(sum)

	return nil
}

func (u *User) Withdraw(ctx context.Context, orderID int64, sum decimal.Decimal) error {
	db := storage.GetDB()
	db.Lock.Lock()
	defer db.Lock.Unlock()

	if err := u.withdraw(sum); err != nil {
		return err
	}

	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return err
	}

	var rollbackErr error
	defer func() {
		rollbackErr = tx.Rollback(ctx)
	}()

	insertQuery := `INSERT INTO withdrawals (
		order_id, user_id, amount, processed_at
	  )
	  VALUES
		($1, $2, $3, $4)`

	if _, err = tx.Exec(ctx, insertQuery, orderID, u.ID, sum, time.Now()); err != nil {
		return err
	}

	updateQuery := "UPDATE users SET balance = $1 WHERE id = $2"
	if _, err = tx.Exec(ctx, updateQuery, u.Balance, u.ID); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}
	return rollbackErr
}

func (u *User) deposit(sum decimal.Decimal) error {
	if sum.IsNegative() || sum.IsZero() {
		return fmt.Errorf("incorrect deposit amount")
	}

	u.Balance = u.Balance.Add(sum)

	return nil
}

func (u *User) Deposit(ctx context.Context, orderID int64, accrual decimal.Decimal) error {
	db := storage.GetDB()
	db.Lock.Lock()
	defer db.Lock.Unlock()

	if err := u.deposit(accrual); err != nil {
		return err
	}

	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return err
	}

	var rollbackErr error
	defer func() {
		rollbackErr = tx.Rollback(ctx)
	}()

	orderUpdateQuery := `UPDATE orders SET status = 'PROCESSED', accrual = $1 WHERE id = $2`

	if _, err = tx.Exec(ctx, orderUpdateQuery, accrual, orderID); err != nil {
		return err
	}

	userUpdateQuery := "UPDATE users SET balance = $1 WHERE id = $2"
	if _, err = tx.Exec(ctx, userUpdateQuery, u.Balance, u.ID); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}
	return rollbackErr
}

func (u *User) getFromDB(ctx context.Context, query string, lookup interface{}) error {
	db := storage.GetDB()
	db.Lock.RLock()
	defer db.Lock.RUnlock()

	fmt.Println(u)
	err := db.Pool.QueryRow(ctx, query, lookup).Scan(&u.ID, &u.Balance, &u.Login, &u.password)
	if err != nil {
		return err
	}

	return nil
}

func (u *User) GetByLogin(ctx context.Context, login string) error {
	query := "SELECT id, balance, login, password FROM users WHERE login = $1"
	return u.getFromDB(ctx, query, login)
}

func (u *User) GetByID(ctx context.Context, id int) error {
	query := "SELECT id, balance, login, password FROM users WHERE id = $1"
	return u.getFromDB(ctx, query, id)
}

func (u *User) Insert(ctx context.Context) error {
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

	insertQuery := "INSERT INTO users (login, password) VALUES ($1, $2) RETURNING id"
	err = tx.QueryRow(ctx, insertQuery, u.Login, u.password).Scan(&u.ID)
	if err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}
	return rollbackErr
}

func (u *User) GetOrders(ctx context.Context) ([]*Order, error) {
	db := storage.GetDB()
	db.Lock.RLock()
	defer db.Lock.RUnlock()

	var nOrders int
	err := db.Pool.QueryRow(
		ctx,
		"SELECT count(id) FROM orders WHERE user_id = $1",
		u.ID,
	).Scan(&nOrders)
	if err != nil {
		return nil, err
	}

	orders := make([]*Order, 0, nOrders)
	selectQuery := "SELECT id, user_id, status, accrual, uploaded_at FROM orders WHERE user_id = $1"
	rows, err := db.Pool.Query(ctx, selectQuery, u.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		order := NewOrder()
		err = rows.Scan(&order.ID, &order.UserID, &order.Status, &order.Accrual, &order.UploadedAt)
		if err != nil {
			return nil, err
		}
		orders = append(orders, order)
	}
	return orders, nil
}

func (u *User) GetWithdrawalHistory(ctx context.Context) ([]*Withdrawal, error) {
	db := storage.GetDB()
	db.Lock.RLock()
	defer db.Lock.RUnlock()

	var nWithdrawals int
	err := db.Pool.QueryRow(
		ctx,
		"SELECT count(id) FROM withdrawals WHERE user_id = $1",
		u.ID,
	).Scan(&nWithdrawals)
	if err != nil {
		return nil, err
	}

	withdrawals := make([]*Withdrawal, 0, nWithdrawals)
	query := `SELECT id, order_id, amount, processed_at
		FROM withdrawals
		WHERE user_id = $1
		ORDER BY processed_at ASC`
	rows, err := db.Pool.Query(ctx, query, u.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		w := NewWithdrawal()
		if err = rows.Scan(&w.ID, &w.OrderID, &w.Sum, &w.ProcessedAt); err != nil {
			return nil, err
		}
		withdrawals = append(withdrawals, w)
	}
	return withdrawals, nil
}

func (u *User) TotalWithdrawn(ctx context.Context) (decimal.NullDecimal, error) {
	db := storage.GetDB()
	db.Lock.RLock()
	defer db.Lock.RUnlock()

	var sum decimal.NullDecimal
	query := "SELECT sum(amount) FROM withdrawals WHERE user_id = $1"
	err := db.Pool.QueryRow(ctx, query, u.ID).Scan(&sum)
	if err != nil {
		return sum, err
	}

	return sum, nil
}
