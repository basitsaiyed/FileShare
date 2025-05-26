package middleware

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

type clientLimiter struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

var (
	clients   = make(map[string]*clientLimiter)
	mu        sync.Mutex
	rateLimit = rate.Every(1 * time.Second) // 1 req/sec
	burst     = 5
)

func cleanupClients() {
	for {
		time.Sleep(time.Minute)
		mu.Lock()
		for ip, c := range clients {
			if time.Since(c.lastSeen) > 3*time.Minute {
				delete(clients, ip)
			}
		}
		mu.Unlock()
	}
}

func getLimiter(ip string) *rate.Limiter {
	mu.Lock()
	defer mu.Unlock()

	cl, exists := clients[ip]
	if !exists {
		limiter := rate.NewLimiter(rateLimit, burst)
		clients[ip] = &clientLimiter{limiter, time.Now()}
		return limiter
	}

	cl.lastSeen = time.Now()
	return cl.limiter
}

func RateLimitMiddleware() gin.HandlerFunc {
	go cleanupClients()

	return func(c *gin.Context) {
		ip := c.ClientIP()
		limiter := getLimiter(ip)

		if !limiter.Allow() {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error": "Too many requests",
			})
			return
		}

		c.Next()
	}
}
