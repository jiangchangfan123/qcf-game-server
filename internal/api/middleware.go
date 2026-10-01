package api

import (
	"GameServer/internal/pkg/logger"
	"encoding/json"
	"fmt"
	"net/http"
	"runtime/debug"
	"strings"
	"sync"
	"time"
)

// ====== 响应工具 ======

func jsonOK(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(data)
}

func jsonError(w http.ResponseWriter, httpCode int, bizCode int, msg string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(httpCode)
	json.NewEncoder(w).Encode(map[string]interface{}{"code": bizCode, "msg": msg})
}

// ====== Panic Recovery ======

func recovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				logger.Log.Errorf("[Recovery] panic: %v\n%s", err, debug.Stack())
				jsonError(w, http.StatusInternalServerError, 500, "服务器内部错误")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// ====== 请求日志 ======

func requestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, statusCode: http.StatusOK}
		next.ServeHTTP(rec, r)

		// 只记 API 请求，静态文件不记
		if strings.HasPrefix(r.URL.Path, "/api/") {
			logger.Log.Infof("[HTTP] %s %s | %d | %v | %s",
				r.Method, r.URL.Path, rec.statusCode, time.Since(start), realIP(r))
		}
	})
}

type statusRecorder struct {
	http.ResponseWriter
	statusCode int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.statusCode = code
	r.ResponseWriter.WriteHeader(code)
}

func realIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if idx := strings.IndexByte(xff, ','); idx > 0 {
			return strings.TrimSpace(xff[:idx])
		}
		return xff
	}
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}
	addr := r.RemoteAddr
	if idx := strings.LastIndex(addr, ":"); idx > 0 {
		return addr[:idx]
	}
	return addr
}

// ====== HTTP 方法守卫 ======

func methodGuard(allowed ...string) func(http.Handler) http.Handler {
	allowedSet := make(map[string]struct{}, len(allowed))
	for _, m := range allowed {
		allowedSet[strings.ToUpper(m)] = struct{}{}
	}
	allowStr := strings.Join(allowed, ", ")

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if _, ok := allowedSet[strings.ToUpper(r.Method)]; !ok {
				w.Header().Set("Allow", allowStr)
				jsonError(w, http.StatusMethodNotAllowed, 405,
					fmt.Sprintf("不允许的请求方法: %s", r.Method))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// ====== Body 大小限制 ======

func bodyLimit(maxBytes int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
			next.ServeHTTP(w, r)
		})
	}
}

// ====== IP 限流（令牌桶） ======

type ipLimiter struct {
	mu       sync.Mutex
	buckets  map[string]*ipBucket
	rate     float64
	burst    float64
}

type ipBucket struct {
	mu        sync.Mutex
	tokens    float64
	maxTokens float64
	rate      float64
	lastTime  time.Time
}

func newIPLimiter(rate float64, burst int) *ipLimiter {
	rl := &ipLimiter{
		buckets: make(map[string]*ipBucket),
		rate:    rate,
		burst:   float64(burst),
	}
	go rl.cleanup()
	return rl
}

func (rl *ipLimiter) allow(ip string) bool {
	rl.mu.Lock()
	b, exists := rl.buckets[ip]
	if !exists {
		b = &ipBucket{
			tokens:    rl.burst,
			maxTokens: rl.burst,
			rate:      rl.rate,
			lastTime:  time.Now(),
		}
		rl.buckets[ip] = b
	}
	rl.mu.Unlock()

	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(b.lastTime).Seconds()
	b.lastTime = now

	b.tokens += elapsed * b.rate
	if b.tokens > b.maxTokens {
		b.tokens = b.maxTokens
	}
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

func (rl *ipLimiter) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		rl.mu.Lock()
		cutoff := time.Now().Add(-10 * time.Minute)
		for ip, b := range rl.buckets {
			b.mu.Lock()
			if b.lastTime.Before(cutoff) {
				delete(rl.buckets, ip)
			}
			b.mu.Unlock()
		}
		rl.mu.Unlock()
	}
}

func rateLimit(rate float64, burst int) func(http.Handler) http.Handler {
	limiter := newIPLimiter(rate, burst)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := realIP(r)
			if !limiter.allow(ip) {
				logger.Log.Warnf("[RateLimit] %s %s | %s", r.Method, r.URL.Path, ip)
				w.Header().Set("Retry-After", "1")
				jsonError(w, http.StatusTooManyRequests, 429, "请求过于频繁，请稍后再试")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// ====== CORS ======

func cors(allowedOrigins []string) func(http.Handler) http.Handler {
	// 如果包含 "*"，开发模式放行所有
	wildcard := false
	originSet := make(map[string]struct{}, len(allowedOrigins))
	for _, o := range allowedOrigins {
		if o == "*" {
			wildcard = true
		}
		originSet[o] = struct{}{}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")

			if wildcard {
				w.Header().Set("Access-Control-Allow-Origin", "*")
			} else if _, ok := originSet[origin]; ok {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Credentials", "true")
			}

			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
			w.Header().Set("Access-Control-Max-Age", "86400")

			if r.Method == "OPTIONS" {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}