package ratelimit

import (
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

type windowRule struct {
	Window time.Duration
	Limit  int
}

type visitor struct {
	mu       sync.Mutex
	hits     map[time.Duration][]time.Time
	lastSeen time.Time
}

type Middleware struct {
	mu       sync.Mutex
	visitors map[string]*visitor
	rules    []windowRule
}

func New() *Middleware {
	m := &Middleware{
		visitors: make(map[string]*visitor),
		rules: []windowRule{
			{Window: time.Second, Limit: 20},
			{Window: time.Minute, Limit: 120},
			{Window: time.Hour, Limit: 1200},
		},
	}

	go m.cleanupLoop()
	return m
}

func (m *Middleware) Handler() gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := clientIP(c.Request.RemoteAddr)
		if ip == "" {
			ip = c.ClientIP()
		}

		allowed, retryAfter := m.allow(ip)
		if !allowed {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error":       "rate limit exceeded",
				"retry_after": retryAfter.Seconds(),
			})
			return
		}

		c.Next()
	}
}

func (m *Middleware) allow(ip string) (bool, time.Duration) {
	now := time.Now()

	m.mu.Lock()
	v, ok := m.visitors[ip]
	if !ok {
		v = &visitor{hits: make(map[time.Duration][]time.Time)}
		m.visitors[ip] = v
	}
	m.mu.Unlock()

	v.mu.Lock()
	defer v.mu.Unlock()
	v.lastSeen = now

	var retryAfter time.Duration
	for _, rule := range m.rules {
		history := v.hits[rule.Window]
		cutoff := now.Add(-rule.Window)
		trimmed := history[:0]
		for _, ts := range history {
			if ts.After(cutoff) {
				trimmed = append(trimmed, ts)
			}
		}
		v.hits[rule.Window] = trimmed

		if len(trimmed) >= rule.Limit {
			wait := trimmed[0].Add(rule.Window).Sub(now)
			if wait < 0 {
				wait = 0
			}
			if wait > retryAfter {
				retryAfter = wait
			}
		}
	}

	if retryAfter > 0 {
		return false, retryAfter
	}

	for _, rule := range m.rules {
		v.hits[rule.Window] = append(v.hits[rule.Window], now)
	}

	return true, 0
}

func (m *Middleware) cleanupLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		expiredBefore := time.Now().Add(-2 * time.Hour)
		m.mu.Lock()
		for ip, v := range m.visitors {
			v.mu.Lock()
			lastSeen := v.lastSeen
			v.mu.Unlock()
			if lastSeen.Before(expiredBefore) {
				delete(m.visitors, ip)
			}
		}
		m.mu.Unlock()
	}
}

func clientIP(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return ""
	}
	return host
}
