package postgres

import (
	"context"
	"encoding/json"
	"fmt"

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

func takeoverEventID(interval domain.AuthorityInterval) string {
	return "takeover/" + interval.ObjectScope + "/" + interval.Capability + "/" + interval.FactKind
}

// HandOffGovernance 把一份意图入队。三种治理决定各认领各的信封：暂停按暂停标识、
// 恢复按被恢复的暂停标识、接管按区间四维（对象范围×能力×事实类型）。缺席、混装
// 或认领键空白是装配缺陷，响亮报错不入队。
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
		eventID, scope, subject string
		eventType               eventing.EventType
		occurredAt              = handoff.clock.Now().UTC()
		payload                 []byte
		err                     error
	)
	switch {
	case intent.Suspension != nil:
		decision := intent.Suspension
		if decision.ID().String() == "" || decision.Scope().String() == "" {
			return fmt.Errorf("hand off governance: suspension id and scope are required")
		}
		eventID = suspensionEventID(decision.ID())
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
		if interval.ObjectScope == "" || interval.Capability == "" || interval.FactKind == "" {
			return fmt.Errorf("hand off governance: takeover interval identity is required")
		}
		eventID = takeoverEventID(interval)
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
		PartitionKey: eventID,
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
