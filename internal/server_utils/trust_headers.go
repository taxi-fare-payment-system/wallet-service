package server_utils

import (
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

func ParseXUserID(c *gin.Context) (string, bool) {
	s := strings.TrimSpace(c.GetHeader("X-User-ID"))
	if s == "" {
		return "", false
	}
	return s, true
}

func XUserRole(c *gin.Context) string {
	return strings.TrimSpace(c.GetHeader("X-User-Role"))
}

func XSubCity(c *gin.Context) string {
	return strings.TrimSpace(c.GetHeader("X-Sub-City"))
}

func ParseXSubCity(c *gin.Context) (uint, bool) {
	s := strings.TrimSpace(c.GetHeader("X-Sub-City"))
	if s == "" {
		return 0, false
	}
	n, err := strconv.ParseUint(s, 10, 0)
	if err != nil || n == 0 {
		return 0, false
	}
	return uint(n), true
}

func IsPlatformAdminRole(role string) bool {
	switch strings.ToLower(role) {
	case "admin", "superadmin":
		return true
	default:
		return false
	}
}

func IsSuperadminRole(role string) bool {
	return strings.ToLower(strings.TrimSpace(role)) == "superadmin"
}
