package health

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

// HealthChecker manages health checks for the application
type HealthChecker struct {
	db    *sql.DB
	redis *redis.Client
}

// NewHealthChecker creates a new health checker
func NewHealthChecker(db *sql.DB, redis *redis.Client) *HealthChecker {
	return &HealthChecker{
		db:    db,
		redis: redis,
	}
}

// HealthStatus represents the health status of a component
type HealthStatus struct {
	Status  string            `json:"status"`
	Details map[string]string `json:"details,omitempty"`
}

// HealthResponse represents the overall health response
type HealthResponse struct {
	Status     string                  `json:"status"`
	Timestamp  time.Time               `json:"timestamp"`
	Components map[string]HealthStatus `json:"components"`
}

// CheckHealth performs health checks on all components
func (h *HealthChecker) CheckHealth(c *gin.Context) {
	ctx := c.Request.Context()
	response := HealthResponse{
		Timestamp:  time.Now(),
		Components: make(map[string]HealthStatus),
	}

	// Check database
	dbStatus := h.checkDatabase(ctx)
	response.Components["database"] = dbStatus

	// Check Redis
	redisStatus := h.checkRedis(ctx)
	response.Components["redis"] = redisStatus

	// Determine overall status
	overallStatus := "healthy"
	for _, component := range response.Components {
		if component.Status != "healthy" {
			overallStatus = "unhealthy"
			break
		}
	}
	response.Status = overallStatus

	// Set HTTP status code based on health
	statusCode := http.StatusOK
	if overallStatus != "healthy" {
		statusCode = http.StatusServiceUnavailable
	}

	c.JSON(statusCode, response)
}

// CheckReadiness performs readiness checks
func (h *HealthChecker) CheckReadiness(c *gin.Context) {
	ctx := c.Request.Context()
	
	// For readiness, we check if all critical services are available
	dbReady := h.isDatabaseReady(ctx)
	redisReady := h.isRedisReady(ctx)

	if dbReady && redisReady {
		c.JSON(http.StatusOK, gin.H{
			"status": "ready",
			"timestamp": time.Now(),
		})
	} else {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"status": "not ready",
			"timestamp": time.Now(),
			"details": gin.H{
				"database": dbReady,
				"redis":    redisReady,
			},
		})
	}
}

// CheckLiveness performs liveness checks
func (h *HealthChecker) CheckLiveness(c *gin.Context) {
	// Liveness check is simple - if we can respond, we're alive
	c.JSON(http.StatusOK, gin.H{
		"status": "alive",
		"timestamp": time.Now(),
	})
}

func (h *HealthChecker) checkDatabase(ctx context.Context) HealthStatus {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := h.db.PingContext(ctx); err != nil {
		return HealthStatus{
			Status: "unhealthy",
			Details: map[string]string{
				"error": err.Error(),
			},
		}
	}

	// Check if we can perform a simple query
	var result int
	err := h.db.QueryRowContext(ctx, "SELECT 1").Scan(&result)
	if err != nil {
		return HealthStatus{
			Status: "unhealthy",
			Details: map[string]string{
				"error": fmt.Sprintf("query failed: %v", err),
			},
		}
	}

	return HealthStatus{
		Status: "healthy",
		Details: map[string]string{
			"connection": "ok",
		},
	}
}

func (h *HealthChecker) checkRedis(ctx context.Context) HealthStatus {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// Test Redis connection with PING
	result := h.redis.Ping(ctx)
	if result.Err() != nil {
		return HealthStatus{
			Status: "unhealthy",
			Details: map[string]string{
				"error": result.Err().Error(),
			},
		}
	}

	return HealthStatus{
		Status: "healthy",
		Details: map[string]string{
			"connection": "ok",
			"response":   result.Val(),
		},
	}
}

func (h *HealthChecker) isDatabaseReady(ctx context.Context) bool {
	status := h.checkDatabase(ctx)
	return status.Status == "healthy"
}

func (h *HealthChecker) isRedisReady(ctx context.Context) bool {
	status := h.checkRedis(ctx)
	return status.Status == "healthy"
}
