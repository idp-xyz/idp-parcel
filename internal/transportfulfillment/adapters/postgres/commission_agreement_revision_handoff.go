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
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// CommissionAgreementRevisedEventType 是「委托换了版」那封门铃的类型（ADR-0231 决定五）。消费方是本上下文自己的实际履约补形成；导出只为
// inbox 消费门与 cmd/parcel-dispatch 那条同时装配两边的用例对照两串相等。
const CommissionAgreementRevisedEventType = "transport-fulfillment.transport-commission.agreement-revised"

// OutboxCommissionAgreementRevisionHandoff 把一次换版的门铃写入 Outbox，实现 ports.CommissionAgreementRevisionHandoff。入队一步由
// outboxintent.EnqueueOnce 承担。
type OutboxCommissionAgreementRevisionHandoff struct {
	db    *bentopg.DB
	store *outbox.Store
	clock ports.Clock
}

func NewOutboxCommissionAgreementRevisionHandoff(
	db *bentopg.DB,
	store *outbox.Store,
	clock ports.Clock,
) (*OutboxCommissionAgreementRevisionHandoff, error) {
	if db == nil {
		return nil, fmt.Errorf("transport fulfillment postgres: db is nil")
	}
	if store == nil {
		return nil, fmt.Errorf("transport fulfillment postgres: outbox store is nil")
	}
	if clock == nil {
		return nil, fmt.Errorf("transport fulfillment postgres: clock is nil")
	}
	return &OutboxCommissionAgreementRevisionHandoff{db: db, store: store, clock: clock}, nil
}

var _ ports.CommissionAgreementRevisionHandoff = (*OutboxCommissionAgreementRevisionHandoff)(nil)

// commissionAgreementRevisionPayload 只带键：租户、委托与换版的号。后继、起用时刻与快照一样不带，它们只有委托一处权威，消费方按键回读。
type commissionAgreementRevisionPayload struct {
	TenantID   string `json:"tenantId"`
	Commission string `json:"commission"`
	Revision   int    `json:"revision"`
}

// HandOffCommissionAgreementRevision 把一封门铃入队。信封 ID 由（租户、委托、号）经指纹铸出，每一次换版各一封。分区取「租户/运输委托」：
// 同一份委托的几次换版按先后排一条队，补形成看到的委托不会比它手里那一封更旧。键缺席是装配缺陷，响亮报错不入队。
func (handoff *OutboxCommissionAgreementRevisionHandoff) HandOffCommissionAgreementRevision(
	ctx context.Context,
	intent ports.CommissionAgreementRevisedIntent,
) error {
	key := intent.Record.Key
	revision := intent.Record.Revision
	if key.TenantID.String() == "" || key.Commission.String() == "" || revision.Number() <= 1 {
		return fmt.Errorf("hand off commission agreement revision: revision key is required")
	}
	payload, err := json.Marshal(commissionAgreementRevisionPayload{
		TenantID:   key.TenantID.String(),
		Commission: key.Commission.String(),
		Revision:   revision.Number(),
	})
	if err != nil {
		return fmt.Errorf("hand off commission agreement revision: %w", err)
	}
	envelope := eventing.Envelope{
		SpecVersion: eventing.SpecVersion,
		ID: outboxintent.FingerprintEventID("commission-agreement-revised",
			key.TenantID.String(), key.Commission.String(), strconv.Itoa(revision.Number())),
		Source:       tfEventSource,
		Type:         CommissionAgreementRevisedEventType,
		Version:      1,
		Scope:        key.TenantID.String(),
		Subject:      key.Commission.String(),
		PartitionKey: key.TenantID.String() + "/transport-commission/" + key.Commission.String() + "/agreement-revision",
		OccurredAt:   revision.RegisteredAt().UTC(),
		RecordedAt:   handoff.clock.Now().UTC(),
		ContentType:  eventing.JSONContentType,
		Payload:      payload,
	}
	if err := outboxintent.EnqueueOnce(ctx, handoff.db, handoff.store, envelope); err != nil {
		return fmt.Errorf("hand off commission agreement revision: %w", err)
	}
	return nil
}
