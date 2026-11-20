package inbox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	"go.idp.xyz/idp-bento-go/eventing"
	"go.idp.xyz/idp-bento-go/postgres/inbox"

	"go.idp.xyz/idp-parcel/internal/platform/inboxconsume"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

const acceptanceDecisionConsumerName = "warehouse-fulfillment/acceptance-decision"

// AcceptanceDecisionFormedEventType 是本消费者认的事件类型。字符串与小包托运交出的那一封相同，本包自己写，不导入生产方。
const AcceptanceDecisionFormedEventType eventing.EventType = "parcel-shipment.acceptance-decision.formed"

// ErrPoisonEnvelope 表示重投同样内容也不会变好。
var ErrPoisonEnvelope = errors.New("warehouse fulfillment inbox: poison envelope")

// AcceptanceDecision 是信封上推进装箱记录要用的引用。
type AcceptanceDecision struct {
	TenantID         string
	Source           string
	SourceRequestKey string
	DecisionID       string
	State            string
}

// AcceptanceRecorder 是接受决定入账。
type AcceptanceRecorder interface {
	Record(ctx context.Context, command application.AcceptanceCommand) (application.AcceptanceResult, error)
}

// AcceptanceDecisionConsumer 把接受决定信封推进仓配的消费门。
type AcceptanceDecisionConsumer struct {
	gate *inboxconsume.Gate[AcceptanceDecision]
}

func NewAcceptanceDecisionConsumer(
	transactor bentoapp.Transactor,
	store *inbox.Store,
	recorder AcceptanceRecorder,
) (*AcceptanceDecisionConsumer, error) {
	if recorder == nil {
		return nil, fmt.Errorf("warehouse fulfillment inbox: acceptance recorder is nil")
	}
	gate, err := inboxconsume.New(inboxconsume.Spec[AcceptanceDecision]{
		Transactor:     transactor,
		Store:          store,
		Name:           acceptanceDecisionConsumerName,
		EventType:      AcceptanceDecisionFormedEventType,
		Decode:         decodeAcceptanceDecision,
		Handle:         recordAcceptance(recorder),
		UnexpectedType: "warehouse fulfillment inbox",
		HandleVerb:     "record acceptance decision",
	})
	if err != nil {
		return nil, err
	}
	return &AcceptanceDecisionConsumer{gate: gate}, nil
}

func (consumer *AcceptanceDecisionConsumer) Consume(ctx context.Context, envelope eventing.Envelope) error {
	return consumer.gate.Consume(ctx, envelope)
}

func tenantOf(value string) (domain.TenantID, error) {
	return domain.NewTenantID(value)
}

func recordAcceptance(recorder AcceptanceRecorder) func(context.Context, AcceptanceDecision) error {
	return func(ctx context.Context, decision AcceptanceDecision) error {
		tenant, err := tenantOf(decision.TenantID)
		if err != nil {
			return fmt.Errorf("%w: %v", ErrPoisonEnvelope, err)
		}
		result, err := recorder.Record(ctx, application.AcceptanceCommand{
			Tenant: tenant, DecisionID: decision.DecisionID, Source: decision.Source,
			SourceRequestKey: decision.SourceRequestKey, State: decision.State,
		})
		if err != nil {
			return err
		}
		switch result.Outcome() {
		case application.AcceptanceAccepted, application.AcceptanceRejected, application.AcceptancePosted, application.AcceptanceExisting, application.AcceptanceConflict:
			return nil
		default:
			return fmt.Errorf("%w: %s", ErrPoisonEnvelope, result.Outcome())
		}
	}
}

func decodeAcceptanceDecision(payload []byte) (AcceptanceDecision, error) {
	var body struct {
		TenantID         string `json:"tenantId"`
		Source           string `json:"source"`
		SourceRequestKey string `json:"sourceRequestKey"`
		DecisionID       string `json:"decisionId"`
		State            string `json:"state"`
	}
	if err := json.Unmarshal(payload, &body); err != nil {
		return AcceptanceDecision{}, fmt.Errorf("%w: %v", ErrPoisonEnvelope, err)
	}
	if body.TenantID == "" || body.Source == "" || body.SourceRequestKey == "" || body.DecisionID == "" ||
		(body.State != "ACCEPTED" && body.State != "REJECTED") {
		return AcceptanceDecision{}, fmt.Errorf("%w: missing acceptance decision fields", ErrPoisonEnvelope)
	}
	return AcceptanceDecision{
		TenantID: body.TenantID, Source: body.Source, SourceRequestKey: body.SourceRequestKey,
		DecisionID: body.DecisionID, State: body.State,
	}, nil
}
