package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"go.idp.xyz/idp-bento-go/eventing"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	"go.idp.xyz/idp-parcel/internal/pilotgovernance/domain"
	"go.idp.xyz/idp-parcel/internal/pilotgovernance/ports"
	"go.idp.xyz/idp-parcel/internal/platform/outboxintent"
)

// pgEventSource 是本上下文在信封 Source 位上的稳定名。
const pgEventSource = "idp-parcel/pilot-governance"

const (
	suspensionEventType = "pilot-governance.suspension.recorded"
	resumptionEventType = "pilot-governance.resumption.recorded"
	takeoverEventType   = "pilot-governance.takeover.recorded"
)

// OutboxGovernanceHandoff 把治理决定写入 Outbox，实现 ports.GovernanceHandoff。
// 入队一步由 outboxintent.EnqueueOnce 承担。
type OutboxGovernanceHandoff struct {
	db    *bentopg.DB
	store *outbox.Store
	clock ports.Clock
}

func NewOutboxGovernanceHandoff(
	db *bentopg.DB,
	store *outbox.Store,
	clock ports.Clock,
) (*OutboxGovernanceHandoff, error) {
	if db == nil {
		return nil, fmt.Errorf("pilot governance postgres: db is nil")
	}
	if store == nil {
		return nil, fmt.Errorf("pilot governance postgres: outbox store is nil")
	}
	if clock == nil {
		return nil, fmt.Errorf("pilot governance postgres: clock is nil")
	}
	return &OutboxGovernanceHandoff{db: db, store: store, clock: clock}, nil
}

var _ ports.GovernanceHandoff = (*OutboxGovernanceHandoff)(nil)

type suspensionPayload struct {
	SuspensionID string `json:"suspensionId"`
	Scope        string `json:"scope"`
}

type resumptionPayload struct {
	SuspensionID string `json:"suspensionId"`
}

type takeoverPayload struct {
	ObjectScope string `json:"objectScope"`
	Capability  string `json:"capability"`
	FactKind    string `json:"factKind"`
	Authority   string `json:"authority"`
}

func suspensionEventID(id domain.SuspensionID) string {
	return "suspension/" + id.String()
}

func resumptionEventID(id domain.SuspensionID) string {
	return "resumption/" + id.String()
}

// suspensionPartitionKey 是暂停与恢复共用的分区键——取被解除的那个暂停标识。
//
// ID 管幂等、分区键管顺序，两者不是一回事。暂停与恢复是同一个治理对象的先后两拍：
// 两种类型的信封 ID 前缀天然错开（都入队、不丢），但逐事件分区让恢复可能先于暂停
// 送达，受影响上下文会先被恢复到一个从未进入过的状态、再被暂停关死，而派发日志里
// 什么错都没有。
func suspensionPartitionKey(id domain.SuspensionID) string {
	return "suspension/" + id.String()
}

// takeoverEventID 取区间四维身份加生效起点——意图由结果标识认领（ADR-0043），而接管行
// 的身份是「区间四维身份加生效区间」（见 Takeovers 库注）。同一范围先后两次接管（原权威
// 区间关闭后另立新权威）是两行两份意图：ID 只取范围三维时两份算出同一个字符串，第二份
// 被 EnqueueOnce 静默吞掉，受影响上下文永远不知道权威已再次易手。
func takeoverEventID(interval domain.AuthorityInterval) string {
	return "takeover/" + interval.ObjectScope + "/" + interval.Capability + "/" + interval.FactKind +
		"/" + interval.Authority + "/" + interval.From.UTC().Format(time.RFC3339)
}

// takeoverPartitionKey 取范围三维，不取权威方与生效起点。
//
// ID 管幂等、分区键管顺序：同一范围的权威更替是一条链，后立的权威区间排在先立的后面；
// 权威方进分区键每次接管就自成一区，链就断了。
func takeoverPartitionKey(interval domain.AuthorityInterval) string {
	return "takeover/" + interval.ObjectScope + "/" + interval.Capability + "/" + interval.FactKind
}

// HandOffGovernance 把一份意图入队。三种治理决定各认领各的信封：暂停按暂停标识、
// 恢复按被恢复的暂停标识、接管按区间四维身份加生效起点。缺席、混装或认领键空白是
// 装配缺陷，响亮报错不入队。分区键另取：暂停与恢复共队（被解除的暂停标识），接管
// 按范围三维排权威更替链。
func (handoff *OutboxGovernanceHandoff) HandOffGovernance(
	ctx context.Context,
	intent ports.GovernanceHandoffIntent,
) error {
	present := 0
	if intent.Suspension != nil {
		present++
	}
	if intent.Resumption != nil {
		present++
	}
	if intent.Takeover != nil {
		present++
	}
	if present != 1 {
		return fmt.Errorf("hand off governance: exactly one of suspension, resumption, takeover is required")
	}

	var (
		eventID, partitionKey, scope, subject string
		eventType                             eventing.EventType
		occurredAt                            = handoff.clock.Now().UTC()
		payload                               []byte
		err                                   error
	)
	switch {
	case intent.Suspension != nil:
		decision := intent.Suspension
		if decision.ID().String() == "" || decision.Scope().String() == "" {
			return fmt.Errorf("hand off governance: suspension id and scope are required")
		}
		eventID = suspensionEventID(decision.ID())
		partitionKey = suspensionPartitionKey(decision.ID())
		eventType = suspensionEventType
		scope = decision.Scope().String()
		subject = decision.ID().String()
		occurredAt = decision.OccurredAt().UTC()
		payload, err = json.Marshal(suspensionPayload{
			SuspensionID: decision.ID().String(),
			Scope:        decision.Scope().String(),
		})
	case intent.Resumption != nil:
		decision := intent.Resumption
		if decision.Suspension().String() == "" {
			return fmt.Errorf("hand off governance: resumption suspension id is required")
		}
		eventID = resumptionEventID(decision.Suspension())
		partitionKey = suspensionPartitionKey(decision.Suspension())
		eventType = resumptionEventType
		scope = decision.Suspension().String()
		subject = decision.Suspension().String()
		occurredAt = decision.DecidedAt().UTC()
		payload, err = json.Marshal(resumptionPayload{
			SuspensionID: decision.Suspension().String(),
		})
	default:
		record := intent.Takeover
		interval := record.Interval()
		// 认领键即区间身份：权威方或生效起点缺席时两次接管会算出同一个 ID，
		// 静默吞掉比响亮报错贵得多。
		if interval.ObjectScope == "" || interval.Capability == "" || interval.FactKind == "" ||
			interval.Authority == "" || interval.From.IsZero() {
			return fmt.Errorf("hand off governance: takeover interval identity is required")
		}
		eventID = takeoverEventID(interval)
		partitionKey = takeoverPartitionKey(interval)
		eventType = takeoverEventType
		scope = interval.ObjectScope
		subject = interval.ObjectScope + "/" + interval.Capability + "/" + interval.FactKind
		occurredAt = record.EffectiveAt().UTC()
		payload, err = json.Marshal(takeoverPayload{
			ObjectScope: interval.ObjectScope,
			Capability:  interval.Capability,
			FactKind:    interval.FactKind,
			Authority:   interval.Authority,
		})
	}
	if err != nil {
		return fmt.Errorf("hand off governance: %w", err)
	}

	now := handoff.clock.Now().UTC()
	envelope := eventing.Envelope{
		SpecVersion:  eventing.SpecVersion,
		ID:           eventing.EventID(eventID),
		Source:       pgEventSource,
		Type:         eventType,
		Version:      1,
		Scope:        scope,
		Subject:      subject,
		PartitionKey: partitionKey,
		OccurredAt:   occurredAt,
		RecordedAt:   now,
		ContentType:  eventing.JSONContentType,
		Payload:      payload,
	}

	if err := outboxintent.EnqueueOnce(ctx, handoff.db, handoff.store, envelope); err != nil {
		return fmt.Errorf("hand off governance: %w", err)
	}
	return nil
}
