package tfinbox

import (
	"context"
	"encoding/json"
	"fmt"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	"go.idp.xyz/idp-bento-go/eventing"
	"go.idp.xyz/idp-bento-go/postgres/inbox"

	"go.idp.xyz/idp-parcel/internal/platform/inboxconsume"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 本文件是本上下文消费自己交出的两封门铃、补形成实际履约发生项的两扇门（ADR-0231 决定一、五）：采用成立与委托换版。处理方直接是应用
// 编排，编排答出的处置（入账 / 重投 / 缺陷）在这里落成 inbox 的三种动作；未决与缺陷沿用自动形成那两扇门的两个哨兵——停住的是同一个
// 形成后半，续办相同。

// 两个消费者名各自分家，理由同自动形成那两扇门。改名等于换消费者。
const (
	catchUpOnAdoptionConsumerName = "transport-fulfillment/catch-up-actual-fulfillment-on-segment-commission-adoption"
	catchUpOnRevisionConsumerName = "transport-fulfillment/catch-up-actual-fulfillment-on-commission-agreement-revision"
)

// 两封门铃的类型。串在本包写出，不去拿提供方 postgres 适配器里的那两个常量（本包不依赖它）；相等由 cmd/parcel-dispatch 用生产交出口入队
// 的真库用例钉住。
const (
	SegmentCommissionAdoptionRegisteredEventType eventing.EventType = "transport-fulfillment.segment-commission-adoption.registered"
	CommissionAgreementRevisedEventType          eventing.EventType = "transport-fulfillment.transport-commission.agreement-revised"
)

// ActualFulfillmentCatchUp 是两扇门转交的那一口，真实装配接 application.FormActualFulfillmentOnParticipationEndHandler。
type ActualFulfillmentCatchUp interface {
	CatchUpOnAdoption(ctx context.Context, command application.CatchUpActualFulfillmentOnAdoptionCommand) (application.CatchUpActualFulfillmentResult, error)
	CatchUpOnRevision(ctx context.Context, command application.CatchUpActualFulfillmentOnRevisionCommand) (application.CatchUpActualFulfillmentResult, error)
}

// RegisteredSegmentCommissionAdoption 是译码后的采用门铃键。委托只进运维读的那句话：补形成按段回读此刻的采用连线。
type RegisteredSegmentCommissionAdoption struct {
	TenantID   string
	Segment    string
	Commission string
}

// RevisedCommissionAgreement 是译码后的换版门铃键。
type RevisedCommissionAgreement struct {
	TenantID   string
	Commission string
	Revision   int
}

// ActualFulfillmentCatchUpOnAdoptionConsumer 把采用门铃推进消费门，交给实际履约补形成。
type ActualFulfillmentCatchUpOnAdoptionConsumer struct {
	gate *inboxconsume.Gate[RegisteredSegmentCommissionAdoption]
}

func NewActualFulfillmentCatchUpOnAdoptionConsumer(
	transactor bentoapp.Transactor,
	store *inbox.Store,
	catchUp ActualFulfillmentCatchUp,
) (*ActualFulfillmentCatchUpOnAdoptionConsumer, error) {
	if err := catchUpConsumerDeps(transactor, store, catchUp); err != nil {
		return nil, err
	}
	gate, err := inboxconsume.New(inboxconsume.Spec[RegisteredSegmentCommissionAdoption]{
		Transactor: transactor,
		Store:      store,
		Name:       catchUpOnAdoptionConsumerName,
		EventType:  SegmentCommissionAdoptionRegisteredEventType,
		Decode:     decodeRegisteredSegmentCommissionAdoption,
		Handle: func(ctx context.Context, registered RegisteredSegmentCommissionAdoption) error {
			tenant, err := domain.NewTenantID(registered.TenantID)
			if err != nil {
				return fmt.Errorf("%w: tenant: %v", ErrActualFulfillmentFormationDefect, err)
			}
			return settleCatchUp(fmt.Sprintf("segment %s adopted %s", registered.Segment, registered.Commission))(
				catchUp.CatchUpOnAdoption(ctx, application.CatchUpActualFulfillmentOnAdoptionCommand{TenantID: tenant, Segment: registered.Segment}))
		},
		UnexpectedType: "transport fulfillment inbox",
		HandleVerb:     "catch up actual fulfillment occurrences on segment commission adoption",
	})
	if err != nil {
		return nil, err
	}
	return &ActualFulfillmentCatchUpOnAdoptionConsumer{gate: gate}, nil
}

// Consume 处理一份投递。舞步在 inboxconsume。
func (consumer *ActualFulfillmentCatchUpOnAdoptionConsumer) Consume(ctx context.Context, envelope eventing.Envelope) error {
	return consumer.gate.Consume(ctx, envelope)
}

// ActualFulfillmentCatchUpOnRevisionConsumer 把换版门铃推进消费门，交给实际履约补形成。
type ActualFulfillmentCatchUpOnRevisionConsumer struct {
	gate *inboxconsume.Gate[RevisedCommissionAgreement]
}

func NewActualFulfillmentCatchUpOnRevisionConsumer(
	transactor bentoapp.Transactor,
	store *inbox.Store,
	catchUp ActualFulfillmentCatchUp,
) (*ActualFulfillmentCatchUpOnRevisionConsumer, error) {
	if err := catchUpConsumerDeps(transactor, store, catchUp); err != nil {
		return nil, err
	}
	gate, err := inboxconsume.New(inboxconsume.Spec[RevisedCommissionAgreement]{
		Transactor: transactor,
		Store:      store,
		Name:       catchUpOnRevisionConsumerName,
		EventType:  CommissionAgreementRevisedEventType,
		Decode:     decodeRevisedCommissionAgreement,
		Handle: func(ctx context.Context, revised RevisedCommissionAgreement) error {
			tenant, err := domain.NewTenantID(revised.TenantID)
			if err != nil {
				return fmt.Errorf("%w: tenant: %v", ErrActualFulfillmentFormationDefect, err)
			}
			return settleCatchUp(fmt.Sprintf("commission %s revision %d", revised.Commission, revised.Revision))(
				catchUp.CatchUpOnRevision(ctx, application.CatchUpActualFulfillmentOnRevisionCommand{
					TenantID: tenant, Commission: revised.Commission, Revision: revised.Revision,
				}))
		},
		UnexpectedType: "transport fulfillment inbox",
		HandleVerb:     "catch up actual fulfillment occurrences on commission agreement revision",
	})
	if err != nil {
		return nil, err
	}
	return &ActualFulfillmentCatchUpOnRevisionConsumer{gate: gate}, nil
}

// Consume 处理一份投递。舞步在 inboxconsume。
func (consumer *ActualFulfillmentCatchUpOnRevisionConsumer) Consume(ctx context.Context, envelope eventing.Envelope) error {
	return consumer.gate.Consume(ctx, envelope)
}

func catchUpConsumerDeps(transactor bentoapp.Transactor, store *inbox.Store, catchUp ActualFulfillmentCatchUp) error {
	if transactor == nil {
		return fmt.Errorf("transport fulfillment inbox: transactor is nil")
	}
	if store == nil {
		return fmt.Errorf("transport fulfillment inbox: inbox store is nil")
	}
	if catchUp == nil {
		return fmt.Errorf("transport fulfillment inbox: actual fulfillment catch-up is nil")
	}
	return nil
}

// settleCatchUp 按编排答出的处置落账，理由同 settleFormation：处理方与消费门同一事务，未决与缺陷都整笔回滚。subject 是给运维读的那一封
// 门铃的键。
func settleCatchUp(subject string) func(application.CatchUpActualFulfillmentResult, error) error {
	return func(result application.CatchUpActualFulfillmentResult, err error) error {
		if err != nil {
			return err
		}
		switch result.Disposition() {
		case application.FormationSettled:
			return nil
		case application.FormationRetry:
			return fmt.Errorf("%w: %s: %s %s", ErrActualFulfillmentFormationUndecided, subject, result.Outcome(), result.UndecidedReason())
		default:
			return fmt.Errorf("%w: %s: %s", ErrActualFulfillmentFormationDefect, subject, describeCatchUp(result))
		}
	}
}

// describeCatchUp 写出整封的那一格，外加第一件不入账的那一件。
func describeCatchUp(result application.CatchUpActualFulfillmentResult) string {
	for _, formed := range result.Formations() {
		if formed.Disposition() != application.FormationSettled {
			return fmt.Sprintf("%s; first unsettled: %s", result.Outcome(), describeFormation(formed))
		}
	}
	return result.Outcome().String()
}

// decodeRegisteredSegmentCommissionAdoption 译载荷。三维缺一即毒丸：重投同样内容也不会长出字段来。
func decodeRegisteredSegmentCommissionAdoption(payload []byte) (RegisteredSegmentCommissionAdoption, error) {
	var body struct {
		TenantID   string `json:"tenantId"`
		Segment    string `json:"segment"`
		Commission string `json:"commission"`
	}
	if err := json.Unmarshal(payload, &body); err != nil {
		return RegisteredSegmentCommissionAdoption{}, fmt.Errorf("%w: %v", ErrPoisonEnvelope, err)
	}
	if body.TenantID == "" || body.Segment == "" || body.Commission == "" {
		return RegisteredSegmentCommissionAdoption{}, fmt.Errorf("%w: missing segment commission adoption key fields", ErrPoisonEnvelope)
	}
	return RegisteredSegmentCommissionAdoption(body), nil
}

// decodeRevisedCommissionAgreement 译载荷。三维缺一、号不是换版的号（不大于 1）即毒丸。
func decodeRevisedCommissionAgreement(payload []byte) (RevisedCommissionAgreement, error) {
	var body struct {
		TenantID   string `json:"tenantId"`
		Commission string `json:"commission"`
		Revision   int    `json:"revision"`
	}
	if err := json.Unmarshal(payload, &body); err != nil {
		return RevisedCommissionAgreement{}, fmt.Errorf("%w: %v", ErrPoisonEnvelope, err)
	}
	if body.TenantID == "" || body.Commission == "" || body.Revision <= 1 {
		return RevisedCommissionAgreement{}, fmt.Errorf("%w: missing commission agreement revision key fields", ErrPoisonEnvelope)
	}
	return RevisedCommissionAgreement(body), nil
}
