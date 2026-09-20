package server

import (
	"context"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

const GlobalCtxKey = "skyhook_global_ctx"

// GlobalCtxMiddleware injects the application-level global context into gin.Context
func GlobalCtxMiddleware(appCtx context.Context) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(GlobalCtxKey, appCtx)
		c.Next()
	}
}

// GetGlobalCtx retrieves the application global context from gin.Context,
// or falls back to the HTTP request context if not found.
func GetGlobalCtx(c *gin.Context) context.Context {
	if val, exists := c.Get(GlobalCtxKey); exists {
		if ctx, ok := val.(context.Context); ok {
			return ctx
		}
	}
	return c.Request.Context()
}

// AuthMiddleware validates API token via Bearer header or X-SkyHook-Token
func AuthMiddleware(expectedToken string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if expectedToken == "" {
			c.Next()
			return
		}

		clientToken := ""

		// 1. Check Authorization: Bearer <token>
		authHeader := c.GetHeader("Authorization")
		if strings.HasPrefix(authHeader, "Bearer ") {
			clientToken = strings.TrimPrefix(authHeader, "Bearer ")
		}

		// 2. Check X-SkyHook-Token: <token>
		if clientToken == "" {
			clientToken = c.GetHeader("X-SkyHook-Token")
		}

		if clientToken == "" || clientToken != expectedToken {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "unauthorized: missing or invalid authentication token",
			})
			return
		}

		c.Next()
	}
}
