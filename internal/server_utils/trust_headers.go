package server_utils

import (
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

func ParseXUserID(c *gin.Context) (int64, bool) {
	s := strings.TrimSpace(c.GetHeader("X-User-ID"))
	if s == "" {
		return 0, false
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil || n <= 0 {
		return 0, false
	}
	return n, true
}

func XUserRole(c *gin.Context) string {
	return strings.TrimSpace(c.GetHeader("X-User-Role"))
}

func XSubCity(c *gin.Context) string {
	return strings.TrimSpace(c.GetHeader("X-Sub-City"))
}

func IsPlatformAdminRole(role string) bool {
	switch strings.ToLower(role) {
	case "admin", "superadmin":
		return true
	default:
		return false
	}
}
