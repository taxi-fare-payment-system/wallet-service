package messaging

import (
	"context"
	"time"

	"github.com/google/uuid"
)

const AuditRecordedRoutingKey = "analytics.audit.recorded"

type AuditEntry struct {
	Action        string
	ActorUserID   string
	ActorUserRole string
	TargetType    string
	TargetID      string
	SubCityID     *uint
	Metadata      map[string]any
	OccurredAt    time.Time
}

func (p *Publisher) PublishAuditLog(_ context.Context, entry AuditEntry) error {
	if p == nil || entry.Action == "" {
		return nil
	}

	occurredAt := entry.OccurredAt
	if occurredAt.IsZero() {
		occurredAt = time.Now().UTC()
	}

	payload := map[string]any{
		"id":              uuid.NewString(),
		"action":          entry.Action,
		"actor_user_id":   entry.ActorUserID,
		"actor_user_role": entry.ActorUserRole,
		"occurred_at":     occurredAt.Format(time.RFC3339),
	}
	if entry.TargetType != "" {
		payload["target_type"] = entry.TargetType
	}
	if entry.TargetID != "" {
		payload["target_id"] = entry.TargetID
	}
	if entry.SubCityID != nil {
		payload["sub_city_id"] = *entry.SubCityID
	}
	if len(entry.Metadata) > 0 {
		payload["metadata"] = entry.Metadata
	}

	return p.publishJSON(p.analyticsExchange, AuditRecordedRoutingKey, payload)
}
