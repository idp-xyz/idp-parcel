package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"go.idp.xyz/idp-bento-go/eventing"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	"go.idp.xyz/idp-parcel/internal/platform/outboxintent"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

const chargeOccurrenceEventType = "warehouse-fulfillment.charge-occurrence.formed"

// ChargeOccurrenceEventType 是交给结算的发生项已形成。信封只带租户、发生项与版本。
const ChargeOccurrenceEventType = chargeOccurrenceEventType

// OutboxChargeHandoff 在形成或更正的同一事务把信封写入 Outbox。
type OutboxChargeHandoff struct {
	db    *bentopg.DB
	store *outbox.Store
	clock ports.Clock
}

func NewOutboxChargeHandoff(db *bentopg.DB, store *outbox.Store, clock ports.Clock) (*OutboxChargeHandoff, error) {
	if db == nil || store == nil || clock == nil {
		return nil, fmt.Errorf("warehouse fulfillment postgres: charge handoff needs a db, an outbox store and a clock")
	}
	return &OutboxChargeHandoff{db: db, store: store, clock: clock}, nil
}

func (handoff *OutboxChargeHandoff) HandOffChargeOccurrence(ctx context.Context, tenant, occurrenceID string, version int) error {
	body, err := json.Marshal(chargeOccurrencePayload{TenantID: tenant, OccurrenceID: occurrenceID, Version: version})
	if err != nil {
		return fmt.Errorf("hand off charge occurrence: %w", err)
	}
	versionText := strconv.Itoa(version)
	now := handoff.clock.Now().UTC()
	envelope := eventing.Envelope{
		SpecVersion: eventing.SpecVersion,
		ID:          outboxintent.FingerprintEventID("charge-occurrence", tenant, occurrenceID, versionText),
		Source:      "idp-parcel/warehouse-fulfillment", Type: chargeOccurrenceEventType, Version: 1,
		Scope: tenant, Subject: occurrenceID,
		PartitionKey: tenant + "/charge-occurrence/" + occurrenceID,
		OccurredAt:   now, RecordedAt: now, ContentType: eventing.JSONContentType, Payload: body,
	}
	if err := outboxintent.EnqueueOnce(ctx, handoff.db, handoff.store, envelope); err != nil {
		return fmt.Errorf("hand off charge occurrence: %w", err)
	}
	return nil
}

type chargeOccurrencePayload struct {
	TenantID     string `json:"tenantId"`
	OccurrenceID string `json:"occurrenceId"`
	Version      int    `json:"version"`
}
