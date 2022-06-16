package middlewares

import (
	"errors"
	"net/http"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v4"

	"github.com/kazauwa/gophermart/internal/storage"
)

func AuthRequired(c *gin.Context) {
	session := sessions.Default(c)
	sessionValue := session.Get("user")
	userID, ok := sessionValue.(int)
	if sessionValue == nil || !ok {
		session.Delete("user")
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}

	user, err := storage.DB.GetUserByID(c.Request.Context(), userID)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		session.Delete("user")
		c.AbortWithStatus(http.StatusUnauthorized)
		return

	case err != nil:
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	c.Set("user", user)
	c.Next()
}
