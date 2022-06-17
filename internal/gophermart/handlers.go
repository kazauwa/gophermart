package gophermart

import (
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgconn"
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v4"
	"github.com/rs/zerolog/log"
	"github.com/shopspring/decimal"

	"github.com/kazauwa/gophermart/internal/middlewares"
	"github.com/kazauwa/gophermart/internal/models"
	"github.com/kazauwa/gophermart/internal/utils"
)

func (g *Gophermart) CreateRouter(router *gin.Engine) {
	userAPI := router.Group("/api/user")
	authorizationAPI := userAPI.Group("/")
	authorizationAPI.POST("/register", g.registerUser)
	authorizationAPI.POST("/login", g.login)

	authorizedAPI := userAPI.Group("/", middlewares.AuthRequired)
	authorizedAPI.POST("/orders", g.uploadOrder)
	authorizedAPI.GET("/orders", g.listOrders)
	authorizedAPI.GET("/balance", g.getBalance)
	authorizedAPI.POST("/balance/withdraw", g.withdraw)
	authorizedAPI.GET("/balance/withdrawals", g.listWithdrawals)
}

func (g *Gophermart) registerUser(c *gin.Context) {
	var jsonRequest struct {
		Login    string `json:"login" binding:"required,min=3,max=64"`
		Password string `json:"password" binding:"required,min=8"`
	}

	if err := c.Bind(&jsonRequest); err != nil {
		log.Err(err).Caller().Msg("error parsing input")
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	user := models.NewUser()
	user.Login = jsonRequest.Login
	if err := user.SetPassword(jsonRequest.Password); err != nil {
		log.Err(err).Caller().Msg("")
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	err := user.Insert(c.Request.Context())
	var pgerror *pgconn.PgError
	switch {
	case errors.As(err, &pgerror) && pgerror.Code == pgerrcode.UniqueViolation:
		c.AbortWithStatusJSON(http.StatusConflict, gin.H{"error": "user already exists"})
		return

	case err != nil:
		log.Err(err).Caller().Msg("failed to insert user")
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	session := sessions.Default(c)
	session.Set("user", user.ID)
	if err := session.Save(); err != nil {
		log.Err(err).Caller().Msg("failed to save session")
		c.AbortWithStatus(http.StatusInternalServerError)
	}
	c.Status(http.StatusOK)
}

func (g *Gophermart) login(c *gin.Context) {
	var jsonRequest struct {
		Login    string `json:"login" binding:"required,min=3,max=64"`
		Password string `json:"password" binding:"required,min=8"`
	}

	if err := c.Bind(&jsonRequest); err != nil {
		log.Err(err).Caller().Msg("error parsing input")
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	user := models.NewUser()
	err := user.GetByLogin(c.Request.Context(), jsonRequest.Login)

	switch {
	case errors.Is(err, pgx.ErrNoRows):
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
		return

	case err != nil:
		log.Err(err).Caller().Msg("failed to fetch user")
		c.AbortWithStatus(http.StatusInternalServerError)
		return

	case user == nil:
		c.AbortWithStatusJSON(
			http.StatusUnauthorized,
			gin.H{"error": "invalid credentials"},
		)
		return
	}

	ok, err := user.CheckPassword(jsonRequest.Password)
	if err != nil {
		log.Err(err).Caller().Msg("failed to check password")
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	if !ok {
		c.AbortWithStatusJSON(
			http.StatusUnauthorized,
			gin.H{"error": "invalid credentials"},
		)
		return
	}

	session := sessions.Default(c)
	session.Set("user", user.ID)
	if err = session.Save(); err != nil {
		log.Err(err).Caller().Msg("failed to save session")
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	c.Status(http.StatusOK)
}

func (g *Gophermart) uploadOrder(c *gin.Context) {
	defer c.Request.Body.Close()
	buf, err := io.ReadAll(c.Request.Body)
	if err != nil {
		log.Err(err).Caller().Msg("error parsing input")
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	orderID, err := strconv.ParseInt(string(buf), 10, 64)
	if err != nil {
		log.Err(err).Caller().Msg("error parsing input")
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if !utils.IsValidLuhn(orderID) {
		log.Error().Caller().Int64("order_id", orderID).Msg("luhn validation failed")
		c.AbortWithStatus(http.StatusUnprocessableEntity)
		return
	}

	order := models.NewOrder()
	err = order.GetByID(c.Request.Context(), orderID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		log.Err(err).Caller().Msg("error fetching order from db")
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	userValue, _ := c.Get("user")
	currentUser, ok := userValue.(*models.User)
	if !ok {
		log.Error().Caller().Msg("malformed user in session")
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	switch {
	case errors.Is(err, pgx.ErrNoRows):
		order := models.NewOrder()
		order.ID = orderID
		order.UserID = currentUser.ID
		err = order.Insert(c.Request.Context())
		if err != nil {
			log.Err(err).Caller().Msg("error inserting order")
			c.AbortWithStatus(http.StatusInternalServerError)
			return
		}
		c.Status(http.StatusAccepted)
		return

	case order.UserID != currentUser.ID:
		c.AbortWithStatus(http.StatusConflict)
		return

	default:
		c.Status(http.StatusOK)
		return
	}
}

func (g *Gophermart) listOrders(c *gin.Context) {
	ctx := c.Request.Context()
	userValue, _ := c.Get("user")
	currentUser, ok := userValue.(*models.User)
	if !ok {
		log.Error().Caller().Msg("malformed user in session")
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	userOrders, err := currentUser.GetOrders(ctx)
	if err != nil {
		log.Err(err).Caller().Msg("cannot fetch user orders")
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	if len(userOrders) == 0 {
		c.Status(http.StatusNoContent)
		return
	}
	c.JSON(http.StatusOK, userOrders)
}

func (g *Gophermart) getBalance(c *gin.Context) {
	userValue, _ := c.Get("user")
	currentUser, ok := userValue.(*models.User)
	if !ok {
		log.Error().Caller().Msg("malformed user in session")
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	var response struct {
		Balance   decimal.Decimal `json:"current"`
		Withdrawn decimal.Decimal `json:"withdrawn"`
	}

	totalWithdrawn, err := currentUser.TotalWithdrawn(c.Request.Context())
	if err != nil {
		log.Err(err).Caller().Msg("cannot fetch withdrawal sum from db")
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	response.Balance = currentUser.Balance
	response.Withdrawn = totalWithdrawn.Decimal
	c.JSON(http.StatusOK, response)
}

func (g *Gophermart) withdraw(c *gin.Context) {
	userValue, _ := c.Get("user")
	currentUser, ok := userValue.(*models.User)
	if !ok {
		log.Error().Caller().Msg("malformed user in session")
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	var jsonRequest struct {
		OrderID string          `json:"order"`
		Sum     decimal.Decimal `json:"sum"`
	}

	if err := c.Bind(&jsonRequest); err != nil {
		log.Err(err).Caller().Msg("error parsing input")
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	orderID, err := strconv.ParseInt(jsonRequest.OrderID, 10, 64)
	if err != nil {
		log.Err(err).Caller().Msg("error parsing input")
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if !utils.IsValidLuhn(orderID) {
		log.Error().Caller().Int64("order_id", orderID).Msg("luhn validation failed")
		c.AbortWithStatus(http.StatusUnprocessableEntity)
		return
	}

	ctx := c.Request.Context()
	order := models.NewOrder()
	err = order.GetByID(ctx, orderID)
	switch {
	case err != nil && !errors.Is(err, pgx.ErrNoRows):
		log.Err(err).Caller().Msg("error looking up order id")
		c.AbortWithStatus(http.StatusInternalServerError)
		return

	case err == nil:
		c.AbortWithStatus(http.StatusConflict)
		return
	}

	err = currentUser.Withdraw(ctx, orderID, jsonRequest.Sum)
	var pgerror *pgconn.PgError
	switch {
	case errors.Is(err, models.ErrInsufficientBalance):
		c.AbortWithStatus(http.StatusPaymentRequired)
		return

	case errors.As(err, &pgerror) && pgerror.Code == pgerrcode.UniqueViolation:
		log.Error().Caller().Msg("withdrawal for order already registered")
		c.AbortWithStatus(http.StatusConflict)
		return

	case err != nil:
		log.Err(err).Caller().Msg("error withdrawing points")
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	c.Status(http.StatusOK)
}

func (g *Gophermart) listWithdrawals(c *gin.Context) {
	userValue, _ := c.Get("user")
	currentUser, ok := userValue.(*models.User)
	if !ok {
		log.Error().Caller().Msg("malformed user in session")
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	withdrawals, err := currentUser.GetWithdrawalHistory(c.Request.Context())
	if err != nil {
		log.Err(err).Caller().Msg("cannot fetch withdrawals from db")
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	if len(withdrawals) == 0 {
		c.Status(http.StatusNoContent)
		return
	}

	c.JSON(http.StatusOK, withdrawals)
}
