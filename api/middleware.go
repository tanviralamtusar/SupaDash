package api

import (
	"golang.org/x/time/rate"
	"net/http"
	"strings"
	"supadash/database"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// rateLimiter struct holds the IP limiters
type ipRateLimiter struct {
	ips   sync.Map
	limit rate.Limit
	burst int
}

func newIPRateLimiter(r rate.Limit, b int) *ipRateLimiter {
	limiter := &ipRateLimiter{
		ips:   sync.Map{},
		limit: r,
		burst: b,
	}

	// Background cleanup of old entries
	go func() {
		for {
			time.Sleep(10 * time.Minute)
			limiter.ips.Range(func(key, value interface{}) bool {
				v := value.(*rateLimiterEntry)
				if time.Since(v.lastSeen) > 10*time.Minute {
					limiter.ips.Delete(key)
				}
				return true
			})
		}
	}()

	return limiter
}

type rateLimiterEntry struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

func (i *ipRateLimiter) getLimiter(ip string) *rate.Limiter {
	v, exists := i.ips.Load(ip)
	if !exists {
		limiter := rate.NewLimiter(i.limit, i.burst)
		i.ips.Store(ip, &rateLimiterEntry{limiter, time.Now()})
		return limiter
	}

	entry := v.(*rateLimiterEntry)
	entry.lastSeen = time.Now()
	return entry.limiter
}

// RateLimitMiddleware applies an IP-based rate limit
func RateLimitMiddleware(requestsPerSecond float64, burst int) gin.HandlerFunc {
	limiter := newIPRateLimiter(rate.Limit(requestsPerSecond), burst)

	return func(c *gin.Context) {
		clientIP := c.ClientIP()
		l := limiter.getLimiter(clientIP)

		if !l.Allow() {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error": "too many requests, please try again later",
			})
			return
		}

		c.Next()
	}
}

// RequireOrgRole enforces that the user has one of the required roles in the organization
func (a *Api) RequireOrgRole(roles ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		account, err := a.GetAccountFromRequest(c)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
			return
		}

		slug := c.Param("slug")
		if slug == "" {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "Missing organization slug"})
			return
		}

		membership, err := a.queries.GetOrganizationMembershipBySlug(c.Request.Context(), database.GetOrganizationMembershipBySlugParams{
			Slug:      slug,
			AccountID: account.ID,
		})
		if err != nil {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "Forbidden: not a member of this organization"})
			return
		}

		hasRole := false
		for _, role := range roles {
			// Case-insensitive: roles have historically been stored in
			// mixed case (e.g. "OWNER" at org creation, request-supplied
			// case on team invites).
			if strings.EqualFold(membership.Role, role) {
				hasRole = true
				break
			}
		}

		if !hasRole {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "Forbidden: insufficient role permissions"})
			return
		}

		// Save membership role in context for later handlers
		c.Set("userRole", membership.Role)
		c.Next()
	}
}

// RequireProjectRole enforces that the user has one of the required roles in the project's organization
func (a *Api) RequireProjectRole(roles ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		account, err := a.GetAccountFromRequest(c)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
			return
		}

		ref := c.Param("ref")
		if ref == "" {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "Missing project ref"})
			return
		}

		membership, err := a.queries.GetOrganizationMembershipByProjectRef(c.Request.Context(), database.GetOrganizationMembershipByProjectRefParams{
			ProjectRef: ref,
			AccountID:  account.ID,
		})
		if err != nil {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "Forbidden: not a member of this project's organization"})
			return
		}

		hasRole := false
		for _, role := range roles {
			// Case-insensitive: see RequireOrgRole
			if strings.EqualFold(membership.Role, role) {
				hasRole = true
				break
			}
		}

		if !hasRole {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "Forbidden: insufficient role permissions"})
			return
		}

		// Save membership role in context
		c.Set("userRole", membership.Role)
		c.Next()
	}
}
