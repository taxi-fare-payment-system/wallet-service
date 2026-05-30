package handlers

import (
	"strings"

	"wallet_service/internal/messaging"
	"wallet_service/internal/server_utils"

	"github.com/gin-gonic/gin"
)

func emitAudit(c *gin.Context, bus *messaging.Publisher, entry messaging.AuditEntry) {
	if bus == nil || entry.Action == "" {
		return
	}
	if entry.ActorUserID == "" {
		if id, ok := server_utils.ParseXUserID(c); ok {
			entry.ActorUserID = id
		}
	}
	if entry.ActorUserRole == "" {
		entry.ActorUserRole = strings.ToLower(server_utils.XUserRole(c))
	}
	if entry.ActorUserID == "" {
		entry.ActorUserID = "system"
	}
	if entry.ActorUserRole == "" {
		entry.ActorUserRole = "system"
	}
	_ = bus.PublishAuditLog(c.Request.Context(), entry)
}
