package server

import (
	"context"
	"net/http"
	"runtime/debug"
	"time"

	"log/slog"
	"wallet_service/internal/handlers"
	"wallet_service/internal/server_utils"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func NewRouter(
	logger *slog.Logger,
	sqlDB pingableDB,
	walletHandlers *handlers.WalletHandlers,
	topupHandlers *handlers.TopupHandlers,
	payFareHandlers *handlers.PayFareHandlers,
	transactionsHandlers *handlers.TransactionsHandlers,
	adminHandlers *handlers.AdminHandlers,
	withdrawDeleteHandlers *handlers.WithdrawDeleteHandlers,
	assistantHandlers *handlers.AssistantHandlers,
) *gin.Engine {
	r := gin.New()
	r.Use(GinRecoveryMiddleware(logger))
	r.Use(GinRequestIDMiddleware())
	r.Use(GinAccessLogMiddleware(logger))

	base := "/api/v1/wallet"

	r.GET(base+"/healthz", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})
	r.GET(base+"/readyz", func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()
		if err := sqlDB.PingContext(ctx); err != nil {
			c.JSON(503, gin.H{"status": "not_ready"})
			return
		}
		c.JSON(200, gin.H{"status": "ok"})
	})

	r.GET(base+"/banks/chapa", withdrawDeleteHandlers.ListChapaBanks)
	r.GET(base+"/assistant/:assistantId/earnings", assistantHandlers.ListEarnings)
	r.GET(base+"/transactions", transactionsHandlers.ListTransactions)

	r.POST(base, walletHandlers.CreateWallet)
	r.GET(base+"/users/:userId", walletHandlers.GetWalletByUser)
	r.GET(base+"/:id", walletHandlers.GetWallet)

	r.PUT(base+"/:wallet_id/topup", topupHandlers.TopupWallet)
	r.POST(base+"/finalize-topup", topupHandlers.FinalizeTopup)

	r.PUT(base+"/:wallet_id/pay-fare", payFareHandlers.PayFare)

	r.PUT(base+"/:wallet_id/withdraw", withdrawDeleteHandlers.Withdraw)
	r.PUT(base+"/:wallet_id/freeze", adminHandlers.FreezeWallet)
	r.DELETE(base+"/:wallet_id", withdrawDeleteHandlers.DeleteWallet)

	r.GET(base+"/admin/wallets", adminHandlers.FindWallets)

	return r
}

type pingableDB interface {
	PingContext(ctx context.Context) error
}

func GinRequestIDMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		rid := c.GetHeader("X-Request-ID")
		if rid == "" {
			rid = uuid.NewString()
		}
		c.Writer.Header().Set("X-Request-ID", rid)
		c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), server_utils.RequestIDKey{}, rid))
		c.Next()
	}
}

func GinAccessLogMiddleware(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		logger.Info("http_request",
			slog.String("request_id", server_utils.RequestIDFromContext(c.Request.Context())),
			slog.String("method", c.Request.Method),
			slog.String("path", c.FullPath()),
			slog.Int("status", c.Writer.Status()),
			slog.Duration("duration", time.Since(start)),
		)
	}
}

func GinRecoveryMiddleware(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if rec := recover(); rec != nil {
				logger.Error("panic_recovered",
					slog.Any("panic", rec),
					slog.String("request_id", server_utils.RequestIDFromContext(c.Request.Context())),
					slog.String("stack", string(debug.Stack())),
				)
				c.AbortWithStatusJSON(http.StatusInternalServerError, server_utils.ErrorResponse{Message: "internal error"})
			}
		}()
		c.Next()
	}
}
