package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"go.idp.xyz/idp-bento-go/eventing"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	"go.idp.xyz/idp-parcel/internal/platform/outboxintent"
)

const customerSupplementEventType = "parcel-shipment.customer-supplement.required"

// OutboxCustomerSupplementHandoff 把「委托待受控补充」写进 Outbox。
type OutboxCustomerSupplementHandoff struct {
	db    *bentopg.DB
	store *outbox.Store
	clock ports.Clock
}

func NewOutboxCustomerSupplementHandoff(db *bentopg.DB, store *outbox.Store, clock ports.Clock) (*OutboxCustomerSupplementHandoff, error) {
	if db == nil || store == nil || clock == nil {
		return nil, fmt.Errorf("parcel shipment postgres: customer supplement handoff needs a db, an outbox store and a clock")
	}
	return &OutboxCustomerSupplementHandoff{db: db, store: store, clock: clock}, nil
}

var _ ports.CustomerSupplementHandoff = (*OutboxCustomerSupplementHandoff)(nil)

func (handoff *OutboxCustomerSupplementHandoff) HandOffCustomerSupplement(ctx context.Context, intent ports.CustomerSupplementHandoffIntent) error {
	gaps := make([]customerSupplementGapPayload, 0, len(intent.Gaps))
	for _, gap := range intent.Gaps {
		gaps = append(gaps, customerSupplementGapPayload{Group: gap.Group, Parcel: gap.Parcel, Reason: gap.Reason})
	}
	body, err := json.Marshal(customerSupplementPayload{
		TenantID:          intent.Identity.TenantID().String(),
		CustomerAccountID: intent.Identity.CustomerAccountID().String(),
		Source:            intent.Identity.Source().String(),
		SourceRequestKey:  intent.Identity.RequestKey().String(),
		ShipmentRequestID: intent.ShipmentRequestID.String(),
		SubmissionVersion: intent.SubmissionVersion.String(),
		Gaps:              gaps,
	})
	if err != nil {
		return fmt.Errorf("hand off customer supplement: %w", err)
	}
	now := handoff.clock.Now().UTC()
	envelope := eventing.Envelope{
		SpecVersion: eventing.SpecVersion, ID: eventing.EventID(intent.SubmissionVersion.String()),
		Source: eventSource, Type: customerSupplementEventType, Version: 1,
		Scope: intent.Identity.TenantID().String(), Subject: intent.ShipmentRequestID.String(),
		PartitionKey: intent.Identity.TenantID().String() + "/" + intent.ShipmentRequestID.String(),
		OccurredAt:   now, RecordedAt: now, ContentType: eventing.JSONContentType, Payload: body,
	}
	if err := outboxintent.EnqueueOnce(ctx, handoff.db, handoff.store, envelope); err != nil {
		return fmt.Errorf("hand off customer supplement: %w", err)
	}
	return nil
}

type customerSupplementPayload struct {
	TenantID          string                         `json:"tenantId"`
	CustomerAccountID string                         `json:"customerAccountId"`
	Source            string                         `json:"source"`
	SourceRequestKey  string                         `json:"sourceRequestKey"`
	ShipmentRequestID string                         `json:"shipmentRequestId"`
	SubmissionVersion string                         `json:"submissionVersion"`
	Gaps              []customerSupplementGapPayload `json:"gaps"`
}

type customerSupplementGapPayload struct {
	Group  string `json:"group"`
	Parcel string `json:"parcel"`
	Reason string `json:"reason"`
}
