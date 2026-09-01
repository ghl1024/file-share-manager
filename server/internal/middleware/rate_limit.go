/*
- Copyright (c) 2026 HaydenGuo
- Project: file-share-manager
- Gitee: https://gitee.com/ghl1024/file-share-manager
- GitHub: https://github.com/ghl1024/file-share-manager
- CNB: https://cnb.cool/ghl1024/file-share-manager
- GitCode: https://gitcode.com/haydenguo/file-share-manager
- Author: https://hayden.pub
 */

package middleware

import (
	"sync"
	"time"

	"file-share-manager/server/internal/pkg/response"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

type clientVisitor struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// RateLimitMiddleware provides a simple IP-based rate limiting
// limit represents requests per window.
func RateLimitMiddleware(limit int, window time.Duration) gin.HandlerFunc {
	var mu sync.Mutex
	visitors := make(map[string]*clientVisitor)
	ratePerSec := rate.Limit(float64(limit) / window.Seconds())

	// Background cleanup routine
	go func() {
		for {
			time.Sleep(time.Minute)
			mu.Lock()
			for ip, v := range visitors {
				// Remove visitors unseen for more than 3 minutes
				if time.Since(v.lastSeen) > 3*time.Minute {
					delete(visitors, ip)
				}
			}
			mu.Unlock()
		}
	}()

	return func(c *gin.Context) {
		ip := c.ClientIP()
		mu.Lock()
		v, exists := visitors[ip]
		if !exists {
			// allow a burst of 'limit' requests initially
			limiter := rate.NewLimiter(ratePerSec, limit)
			visitors[ip] = &clientVisitor{limiter: limiter, lastSeen: time.Now()}
			v = visitors[ip]
		} else {
			v.lastSeen = time.Now()
		}

		// check if rate limit is exceeded
		if !v.limiter.Allow() {
			mu.Unlock()
			response.TooManyRequests(c, "Too many requests. Please try again later.")
			return
		}
		mu.Unlock()
		c.Next()
	}
}
