package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"go.idp.xyz/idp-bento-go/eventing"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	"go.idp.xyz/idp-parcel/internal/platform/outboxintent"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// settlementAccountRegisteredEventType 是「结算账户已登记」的事件类型，parcel-shipment 那扇作用域续办门按它
// 收信（票 routing-first-cut/19）。与消费侧各写各的字面，理由同 party-commercial 的「参数已登记」：导入对方的
// 常量等于让消费方依赖提供方的内部形状；两串漂开撞 dispatch.no_subscriber，由 cmd/parcel-dispatch 那条用本
// 适配器铸封的往返用例揪红。
const settlementAccountRegisteredEventType eventing.EventType = "settlement-accounting.settlement-account.registered"

// settlementAccountRegisteredEventIDPort 是信封 ID 指纹的口名前缀，只求本来源名下几只口的 ID 空间互不相交。
const settlementAccountRegisteredEventIDPort = "settlement-account"

// settlementAccountRegistrationKind 是载荷里的登记种类原词。消费门不按它分派，它进载荷是为了让这封信与
// party-commercial 那几类「参数已登记」同一份两键契约、事后能分出是哪一类登记解开的等待。
const settlementAccountRegistrationKind = "SETTLEMENT_ACCOUNT"

// OutboxSettlementAccountRegisteredHandoff 把「结算账户已登记」写入 Outbox，实现
// ports.SettlementAccountRegisteredHandoff。入队一步由 outboxintent.EnqueueOnce 承担。
type OutboxSettlementAccountRegisteredHandoff struct {
	db    *bentopg.DB
	store *outbox.Store
	clock ports.Clock
}

func NewOutboxSettlementAccountRegisteredHandoff(
	db *bentopg.DB,
	store *outbox.Store,
	clock ports.Clock,
) (*OutboxSettlementAccountRegisteredHandoff, error) {
	if db == nil {
		return nil, fmt.Errorf("settlement accounting postgres: db is nil")
	}
	if store == nil {
		return nil, fmt.Errorf("settlement accounting postgres: outbox store is nil")
	}
	if clock == nil {
		return nil, fmt.Errorf("settlement accounting postgres: clock is nil")
	}
	return &OutboxSettlementAccountRegisteredHandoff{db: db, store: store, clock: clock}, nil
}

var _ ports.SettlementAccountRegisteredHandoff = (*OutboxSettlementAccountRegisteredHandoff)(nil)

// settlementAccountRegisteredPayload 只有租户与登记种类两键，与 party-commercial「参数已登记」载荷同形：
// 消费门按租户取回停在`等待运营登记`的委托逐份重驱，账户是哪一户它不需要知道——重驱时作用域照旧问账户册。
type settlementAccountRegisteredPayload struct {
	TenantID         string `json:"tenantId"`
	RegistrationKind string `json:"registrationKind"`
}

// settlementAccountRegisteredEventID 取（租户 / 账户）的定长指纹：一户一封，重放同一户由 EnqueueOnce 按 ID 吞掉
// （ADR-0043）。账户标识多长归实例半边，指纹形的理由在 outboxintent.FingerprintEventID 头注。
func settlementAccountRegisteredEventID(intent ports.SettlementAccountRegisteredIntent) eventing.EventID {
	return outboxintent.FingerprintEventID(settlementAccountRegisteredEventIDPort,
		intent.Tenant.String(), intent.Account.String())
}

// HandOffSettlementAccountRegistered 把一份意图入队。租户、账户或登记时刻缺席是装配缺陷，响亮报错不入队——
// 这个口只该在账户行刚落库的那一格被调。
func (handoff *OutboxSettlementAccountRegisteredHandoff) HandOffSettlementAccountRegistered(
	ctx context.Context,
	intent ports.SettlementAccountRegisteredIntent,
) error {
	if intent.Tenant.String() == "" || intent.Account.String() == "" {
		return fmt.Errorf("hand off settlement account registered: tenant and account are required")
	}
	if intent.RegisteredAt.IsZero() {
		return fmt.Errorf("hand off settlement account registered: registration moment is required")
	}

	payload, err := json.Marshal(settlementAccountRegisteredPayload{
		TenantID:         intent.Tenant.String(),
		RegistrationKind: settlementAccountRegistrationKind,
	})
	if err != nil {
		return fmt.Errorf("hand off settlement account registered: %w", err)
	}

	envelope := eventing.Envelope{
		SpecVersion: eventing.SpecVersion,
		ID:          settlementAccountRegisteredEventID(intent),
		Source:      saEventSource,
		Type:        settlementAccountRegisteredEventType,
		Version:     1,
		Scope:       intent.Tenant.String(),
		Subject:     settlementAccountRegistrationKind + "/" + intent.Account.String(),
		// 分区装租户，与 party-commercial「参数已登记」同一个字符串：两口的信封驱的是同一扇门背后同一张
		// 等待队列，同队让同一租户的重驱一轮接一轮、不会两轮并发去推同一份委托。
		PartitionKey: intent.Tenant.String(),
		OccurredAt:   intent.RegisteredAt.UTC(),
		RecordedAt:   handoff.clock.Now().UTC(),
		ContentType:  eventing.JSONContentType,
		Payload:      payload,
	}

	if err := outboxintent.EnqueueOnce(ctx, handoff.db, handoff.store, envelope); err != nil {
		if errors.Is(err, eventing.ErrInvalidEnvelope) {
			return fmt.Errorf("hand off settlement account registered: %w: %w", ports.ErrSettlementAccountHandoffRejected, err)
		}
		return fmt.Errorf("hand off settlement account registered: %w", err)
	}
	return nil
}
