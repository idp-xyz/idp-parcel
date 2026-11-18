package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"go.idp.xyz/idp-bento-go/eventing"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
	"go.idp.xyz/idp-parcel/internal/platform/outboxintent"
)

// ActualMeasurementChangedEventType 是实际测量登记或作废已经写下的事件类型。
const ActualMeasurementChangedEventType = "node-operations.actual-measurement.changed"

const (
	measurementChangedEventIDPort   = "actual-measurement-changed"
	measurementChangedPartitionPort = "actual-measurement-object"
)

// OutboxMeasurementChangeNotice 把一次测量变化写入 Outbox。
type OutboxMeasurementChangeNotice struct {
	db    *bentopg.DB
	store *outbox.Store
	clock ports.Clock
}

func NewOutboxMeasurementChangeNotice(
	db *bentopg.DB,
	store *outbox.Store,
	clock ports.Clock,
) (*OutboxMeasurementChangeNotice, error) {
	if db == nil || store == nil || clock == nil {
		return nil, fmt.Errorf("node operations postgres: measurement change notice is missing a dependency")
	}
	return &OutboxMeasurementChangeNotice{db: db, store: store, clock: clock}, nil
}

var _ ports.MeasurementChangeNotice = (*OutboxMeasurementChangeNotice)(nil)

type measurementChangedPayload struct {
	TenantID   string `json:"tenantId"`
	SourceID   string `json:"sourceId"`
	ObjectKind string `json:"objectKind"`
	ObjectID   string `json:"objectId"`
}

// NoticeMeasurementChanged 入队一封变化。
//
// ID 由（租户，来源身份）的指纹认领：同一次登记或作废重投撞同一封。分区键由（租户，对象种类，对象标识）
// 的指纹认领：同一对象的变化排一条队。
func (notice *OutboxMeasurementChangeNotice) NoticeMeasurementChanged(
	ctx context.Context,
	change ports.MeasurementChanged,
) error {
	if _, err := notice.db.RequireExecutor(ctx); err != nil {
		return fmt.Errorf("notice measurement changed: %w", err)
	}
	tenant := change.Tenant.String()
	if strings.TrimSpace(tenant) == "" || strings.TrimSpace(change.SourceID) == "" || change.ObjectKind.String() == "" || strings.TrimSpace(change.ObjectID) == "" {
		return fmt.Errorf("notice measurement changed: tenant, source and object are required")
	}
	payload, err := json.Marshal(measurementChangedPayload{
		TenantID: tenant, SourceID: change.SourceID,
		ObjectKind: change.ObjectKind.String(), ObjectID: change.ObjectID,
	})
	if err != nil {
		return fmt.Errorf("notice measurement changed: %w", err)
	}
	envelope := eventing.Envelope{
		SpecVersion: eventing.SpecVersion,
		ID:          outboxintent.FingerprintEventID(measurementChangedEventIDPort, tenant, change.SourceID),
		Source:      noEventSource,
		Type:        ActualMeasurementChangedEventType,
		Version:     1,
		Scope:       tenant,
		Subject:     change.SourceID,
		PartitionKey: string(outboxintent.FingerprintEventID(
			measurementChangedPartitionPort, tenant, change.ObjectKind.String(), change.ObjectID,
		)),
		OccurredAt:  change.OccurredAt.UTC(),
		RecordedAt:  notice.clock.Now().UTC(),
		ContentType: eventing.JSONContentType,
		Payload:     payload,
	}
	if err := outboxintent.EnqueueOnce(ctx, notice.db, notice.store, envelope); err != nil {
		return fmt.Errorf("notice measurement changed: %w", err)
	}
	return nil
}
