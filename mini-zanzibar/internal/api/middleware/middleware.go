package middleware

import (
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"golang.org/x/time/rate"
)

// Logger returns a gin.HandlerFunc that logs requests using zap
func Logger(logger *zap.SugaredLogger) gin.HandlerFunc {
	fmt.Printf("=== Logger middleware initialized ===\n")
	return gin.HandlerFunc(func(c *gin.Context) {
		fmt.Printf("=== Logger middleware executing for %s %s ===\n", c.Request.Method, c.Request.URL.Path)
		c.Next()
	})
	// TODO: Implement custom zap-based logging middleware
	// This should log request details, response status, and timing
}

// Recovery returns a gin.HandlerFunc that recovers from panics
func Recovery(logger *zap.SugaredLogger) gin.HandlerFunc {
	return gin.RecoveryWithWriter(gin.DefaultWriter)
	// TODO: Implement custom recovery middleware that logs panics using zap
}

// CORS returns a gin.HandlerFunc that handles CORS
func CORS() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT, DELETE")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}

// RateLimit returns a gin.HandlerFunc that implements rate limiting
func RateLimit(requests int, window time.Duration) gin.HandlerFunc {
	// Create a map to store rate limiters per IP
	limiters := make(map[string]*rate.Limiter)
	var mu sync.RWMutex

	// Calculate rate limit per second
	ratePerSecond := rate.Limit(float64(requests) / window.Seconds())

	return func(c *gin.Context) {
		ip := c.ClientIP()

		mu.RLock()
		limiter, exists := limiters[ip]
		mu.RUnlock()

		if !exists {
			mu.Lock()
			// Double-check pattern to avoid race condition
			if limiter, exists = limiters[ip]; !exists {
				limiter = rate.NewLimiter(ratePerSecond, requests)
				limiters[ip] = limiter
			}
			mu.Unlock()
		}

		if !limiter.Allow() {
			c.JSON(http.StatusTooManyRequests, gin.H{
				"error":       "Rate limit exceeded",
				"retry_after": "60s",
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

// UserContext extracts user information from headers and sets it in the context
// This middleware handles authentication context from the auth service
func UserContext() gin.HandlerFunc {
	return func(c *gin.Context) {
		fmt.Printf("=== UserContext middleware executing ===\n")

		// Debug: Log all headers
		for key, values := range c.Request.Header {
			for _, value := range values {
				if key == "X-User-ID" || key == "X-User-Role" {
					fmt.Printf("DEBUG: Header %s: %s\n", key, value)
				}
			}
		}

		// Extract user ID from header (set by auth service)
		userID := c.GetHeader("X-User-ID")
		fmt.Printf("DEBUG: Extracted X-User-ID: '%s'\n", userID)

		if userID != "" {
			c.Set("user", userID)
			fmt.Printf("DEBUG: Set user context to: '%s'\n", userID)
		} else {
			fmt.Printf("DEBUG: No X-User-ID header found\n")
		}

		fmt.Printf("=== UserContext middleware completed ===\n")
		c.Next()
	}
}
