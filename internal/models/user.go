package models

import (
	"errors"
	"fmt"

	"github.com/alexedwards/argon2id"
	"github.com/shopspring/decimal"
)

var ErrInsufficientBalance = errors.New("insufficient balance")

type User struct {
	ID       int             `json:"-"`
	Balance  decimal.Decimal `json:"balance"`
	Login    string          `json:"login"`
	password string
}

func NewUser(id int, login string, password string, balance decimal.Decimal) *User {
	return &User{
		ID:       id,
		Login:    login,
		password: password,
		Balance:  balance,
	}
}

func (u *User) Withdraw(sum decimal.Decimal) error {
	if sum.IsNegative() || sum.IsZero() {
		return fmt.Errorf("incorrect withdrawal amount")
	}

	if sum.GreaterThan(u.Balance) {
		return ErrInsufficientBalance
	}
	u.Balance = u.Balance.Sub(sum)

	return nil
}

func (u *User) Deposit(sum decimal.Decimal) error {
	if sum.IsNegative() || sum.IsZero() {
		return fmt.Errorf("incorrect deposit amount")
	}

	u.Balance = u.Balance.Add(sum)

	return nil
}

func (u *User) CheckPassword(password string) (bool, error) {
	return argon2id.ComparePasswordAndHash(password, u.password)
}
