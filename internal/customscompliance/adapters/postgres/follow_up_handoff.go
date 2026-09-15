package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"go.idp.xyz/idp-bento-go/eventing"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
	"go.idp.xyz/idp-parcel/internal/platform/outboxintent"
)

// 后续动作目标在同一个目标键上有三拍都交意图（立目标 → 拟替代 → 替代生效），三拍
// 各认领各的信封、各有各的事件类型。状态段照 statement_handoff 的 /voided 现成形状：
// 首拍裸键，后两拍各带后缀。
const (
	followUpEventType          = "customs-compliance.follow-up.recorded"
	followUpProposedEventType  = "customs-compliance.follow-up.replacement-proposed"
	followUpEffectiveEventType = "customs-compliance.follow-up.replacement-effective"
)

// OutboxFollowUpHandoff 把后续动作目标写入 Outbox，实现 ports.FollowUpHandoff。
// 入队一步由 outboxintent.EnqueueOnce 承担。
type OutboxFollowUpHandoff struct {
	db    *bentopg.DB
	store *outbox.Store
	clock ports.Clock
}

func NewOutboxFollowUpHandoff(
	db *bentopg.DB,
	store *outbox.Store,
	clock ports.Clock,
) (*OutboxFollowUpHandoff, error) {
	if db == nil {
		return nil, fmt.Errorf("customs compliance postgres: db is nil")
	}
	if store == nil {
		return nil, fmt.Errorf("customs compliance postgres: outbox store is nil")
	}
	if clock == nil {
		return nil, fmt.Errorf("customs compliance postgres: clock is nil")
	}
	return &OutboxFollowUpHandoff{db: db, store: store, clock: clock}, nil
}

var _ ports.FollowUpHandoff = (*OutboxFollowUpHandoff)(nil)

// followUpPayload 是意图载荷的传输形状：只有下游 FindTarget 所需的目标键四维，不带
// 替代关系或范围明细。
type followUpPayload struct {
	TenantID  string `json:"tenantId"`
	Trigger   string `json:"trigger"`
	VersionID string `json:"versionId"`
	Kind      string `json:"kind"`
}

// followUpPartitionKey 取整个目标键，不取状态段。
//
// ID 管幂等、分区键管顺序，两者不是一回事。同一目标的三拍是一条链（立目标 → 拟替代 →
// 替代生效），状态段进分区键每拍就自成一区，生效可能先于拟替代送达——下游读到的替代
// 关系从此没有先后可言。目标键四维之内则一维不能少：少了哪一维都会把不同目标压进一队。
func followUpPartitionKey(key ports.FollowUpTargetKey) string {
	return key.TenantID.String() + "/" + key.Trigger.String() + "/" +
		key.Version.String() + "/" + key.Kind.String()
}

// followUpEventIDPort 是本口在信封 ID 上的口名前缀。
const followUpEventIDPort = "follow-up"

// 三拍在信封 ID 上的状态段：与各拍事件类型的尾词同词。
const (
	followUpBeatRecorded             = "recorded"
	followUpBeatReplacementProposed  = "replacement-proposed"
	followUpBeatReplacementEffective = "replacement-effective"
)

// followUpEventID 把目标键四维加状态段折成 outboxintent.FingerprintEventID 的定长形（票 sa-cc/34 裁决 3）。状态段
// 必须作一维进哈希：少了它三拍就算出同一个 ID，后两拍被 outboxintent.EnqueueOnce 当成首拍的重放静默吞掉。
func followUpEventID(key ports.FollowUpTargetKey, beat string) eventing.EventID {
	return outboxintent.FingerprintEventID(followUpEventIDPort,
		key.TenantID.String(), key.Trigger.String(), key.Version.String(), key.Kind.String(), beat)
}

// followUpHandoffShape 是一拍的信封身份：哪一份（ID）、哪一类（类型）、何时发生。
type followUpHandoffShape struct {
	eventID    eventing.EventID
	eventType  eventing.EventType
	occurredAt time.Time
}

// followUpHandoffIdentity 按意图携带的状态选拍。ADR-0043 说意图由结果标识认领，而
// 这一口的结果是「目标处在哪一拍」：ID 少了状态段，三拍就算出同一份，而
// outboxintent.EnqueueOnce 先查后插——后两拍静默不入队，编排却收到「交接成功」。
func followUpHandoffIdentity(intent ports.FollowUpHandoffIntent) followUpHandoffShape {
	switch {
	case intent.Relation == nil:
		return followUpHandoffShape{
			eventID:    followUpEventID(intent.Key, followUpBeatRecorded),
			eventType:  followUpEventType,
			occurredAt: intent.Target.FormedAt().UTC(),
		}
	case !intent.Relation.Effective():
		// 拟替代在领域上没有自己的时刻（ProposeReplacement 不收时间），取目标形成
		// 时刻——顺序由分区序列守，不靠这个时间戳。
		return followUpHandoffShape{
			eventID:    followUpEventID(intent.Key, followUpBeatReplacementProposed),
			eventType:  followUpProposedEventType,
			occurredAt: intent.Target.FormedAt().UTC(),
		}
	default:
		at, _ := intent.Relation.EffectiveAt()
		return followUpHandoffShape{
			eventID:    followUpEventID(intent.Key, followUpBeatReplacementEffective),
			eventType:  followUpEffectiveEventType,
			occurredAt: at.UTC(),
		}
	}
}

// HandOffFollowUp 把一份意图入队。信封 ID 取目标键加状态段——意图由「目标的这一拍」
// 认领（ADR-0043）。载荷仍是指针式的（只带目标键四维，下游按键重读当前状态）。键缺席
// 是装配缺陷，响亮报错不入队。
func (handoff *OutboxFollowUpHandoff) HandOffFollowUp(
	ctx context.Context,
	intent ports.FollowUpHandoffIntent,
) error {
	key := intent.Key
	if key.TenantID.String() == "" ||
		key.Trigger.String() == "" ||
		key.Version.String() == "" ||
		key.Kind.String() == "" {
		return fmt.Errorf("hand off follow-up: receive key is required")
	}

	payload, err := json.Marshal(followUpPayload{
		TenantID:  key.TenantID.String(),
		Trigger:   key.Trigger.String(),
		VersionID: key.Version.String(),
		Kind:      key.Kind.String(),
	})
	if err != nil {
		return fmt.Errorf("hand off follow-up: %w", err)
	}

	now := handoff.clock.Now().UTC()
	shape := followUpHandoffIdentity(intent)
	envelope := eventing.Envelope{
		SpecVersion:  eventing.SpecVersion,
		ID:           shape.eventID,
		Source:       ccEventSource,
		Type:         shape.eventType,
		Version:      1,
		Scope:        key.TenantID.String(),
		Subject:      key.Version.String() + "/" + key.Kind.String(),
		PartitionKey: followUpPartitionKey(key),
		OccurredAt:   shape.occurredAt,
		RecordedAt:   now,
		ContentType:  eventing.JSONContentType,
		Payload:      payload,
	}

	if err := outboxintent.EnqueueOnce(ctx, handoff.db, handoff.store, envelope); err != nil {
		return fmt.Errorf("hand off follow-up: %w", err)
	}
	return nil
}
