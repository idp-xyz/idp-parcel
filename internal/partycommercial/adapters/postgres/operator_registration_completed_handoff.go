package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"go.idp.xyz/idp-bento-go/eventing"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
	"go.idp.xyz/idp-parcel/internal/platform/outboxintent"
)

// eventSource 是本上下文写进信封的来源名。本上下文此前没有 Outbox 口，随首个交接适配器立下。
const eventSource = "idp-parcel/party-commercial"

// operatorRegistrationCompletedEventType 是「参数已登记」意图的事件类型（ADR-0094 决定四的续办
// 触发）。与消费侧（parcel-shipment 的 adapters/inbox）各写各的字面：两边在不同上下文，导入对方的
// 常量等于让消费方依赖提供方的内部形状。契约原文记在票 first-tenant-runway/07 的 Comments；两串
// 对不上要由消费侧的译码用例揪红——那条用例随消费门一起立，是 PS 半边的活。
const operatorRegistrationCompletedEventType = "party-commercial.commercial-authority.operator-registration-completed"

// OutboxOperatorRegistrationCompletedHandoff 把「参数已登记」写入 Outbox，实现
// ports.OperatorRegistrationCompletedHandoff。入队一步由 outboxintent.EnqueueOnce 承担。
type OutboxOperatorRegistrationCompletedHandoff struct {
	db    *bentopg.DB
	store *outbox.Store
	clock ports.Clock
}

func NewOutboxOperatorRegistrationCompletedHandoff(
	db *bentopg.DB,
	store *outbox.Store,
	clock ports.Clock,
) (*OutboxOperatorRegistrationCompletedHandoff, error) {
	if db == nil {
		return nil, fmt.Errorf("party commercial postgres: db is nil")
	}
	if store == nil {
		return nil, fmt.Errorf("party commercial postgres: outbox store is nil")
	}
	if clock == nil {
		return nil, fmt.Errorf("party commercial postgres: clock is nil")
	}
	return &OutboxOperatorRegistrationCompletedHandoff{db: db, store: store, clock: clock}, nil
}

var _ ports.OperatorRegistrationCompletedHandoff = (*OutboxOperatorRegistrationCompletedHandoff)(nil)

// operatorRegistrationCompletedPayload 是意图载荷的传输形状：只有租户与登记种类。消费门按租户
// 取回停在`等待运营登记`的委托逐份重驱，不按种类分派也不读规则正文——多带一个字段就是多一处
// 消费方可能开始依赖的形状（票 first-tenant-runway/07 D4 原话「载荷只带租户与登记种类」）。
type operatorRegistrationCompletedPayload struct {
	TenantID         string `json:"tenantId"`
	RegistrationKind string `json:"registrationKind"`
}

// operatorRegistrationCompletedEventID 由租户加种类加承载版本认领意图（ADR-0043）：同一版规则包
// 的时点声明重放重发同一封；同一规则包发新版本再声明一次是另一份登记、另一个 ID。种类段在里面，
// 是因为将来授权登记若也从某一版商业对象发出，两类不得互相吞没。
func operatorRegistrationCompletedEventID(intent ports.OperatorRegistrationCompletedIntent) string {
	version := intent.Registration
	return version.Tenant().String() + "/" +
		intent.Kind.String() + "/" +
		version.ObjectID().String() + "/" +
		version.Version().String() + "/operator-registration-completed"
}

// HandOffOperatorRegistrationCompleted 把一份意图入队。种类集合外、承载版本零值或登记时刻零值
// 都是装配缺陷——这个口只该在声明刚落库的那一格被调，缺件到了这里说明编排接错了地方，响亮报错
// 不入队；入一封没有依据的信封只会让消费门白跑一轮重驱。
func (handoff *OutboxOperatorRegistrationCompletedHandoff) HandOffOperatorRegistrationCompleted(
	ctx context.Context,
	intent ports.OperatorRegistrationCompletedIntent,
) error {
	if intent.Kind.String() == "" {
		return fmt.Errorf("hand off operator registration completed: registration kind is outside the closed set")
	}
	version := intent.Registration
	if version.Tenant().String() == "" || version.ObjectID().String() == "" || version.Version().String() == "" {
		return fmt.Errorf("hand off operator registration completed: registering version is required")
	}
	if intent.RegisteredAt.IsZero() {
		return fmt.Errorf("hand off operator registration completed: registration moment is required")
	}

	payload, err := json.Marshal(operatorRegistrationCompletedPayload{
		TenantID:         version.Tenant().String(),
		RegistrationKind: intent.Kind.String(),
	})
	if err != nil {
		return fmt.Errorf("hand off operator registration completed: %w", err)
	}

	envelope := eventing.Envelope{
		SpecVersion: eventing.SpecVersion,
		ID:          eventing.EventID(operatorRegistrationCompletedEventID(intent)),
		Source:      eventSource,
		Type:        operatorRegistrationCompletedEventType,
		Version:     1,
		Scope:       version.Tenant().String(),
		Subject:     version.Kind().String() + "/" + version.ObjectID().String() + "@" + version.Version().String(),
		// 分区装租户：同一租户的参数登记信封排一条队，消费门对该租户的重驱一轮接一轮，不会两轮
		// 并发；不同租户互不阻塞。它与 ID 不同源——ID 带版本维管幂等，分区键只到租户管顺序。
		PartitionKey: version.Tenant().String(),
		// 领域发生时间是登记落库的那一刻（发布时刻），不是入队时刻——两者分开与「委托已提交」
		// 同一条纪律。
		OccurredAt:  intent.RegisteredAt.UTC(),
		RecordedAt:  handoff.clock.Now().UTC(),
		ContentType: eventing.JSONContentType,
		Payload:     payload,
	}

	if err := outboxintent.EnqueueOnce(ctx, handoff.db, handoff.store, envelope); err != nil {
		return fmt.Errorf("hand off operator registration completed: %w", err)
	}
	return nil
}
