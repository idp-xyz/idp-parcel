package noinbox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	"go.idp.xyz/idp-bento-go/eventing"
	"go.idp.xyz/idp-bento-go/postgres/inbox"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/application"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/platform/inboxconsume"
)

// DeliveryScopeEstablishedEventType 是「交付范围成立」时本上下文自投的重核门铃（ADR-0212 决定六）。消费方自己写出这个字符串，不
// 导入写出方 outbox 适配器的常量，同别的门铃；两串由组合根的一条对照用例钉住。
const DeliveryScopeEstablishedEventType eventing.EventType = "node-operations.delivery-scope.established"

// EstablishedDeliveryScope 是译码后的门铃：租户与交接单；Version 非零即归入那一封，只指名那一版。门铃不是身份答案，重核照旧经身份
// 核对读口问 PS。
type EstablishedDeliveryScope struct {
	TenantID domain.TenantID
	Note     domain.DeliveryHandoverNoteID
	Version  domain.IntakeResultVersion
}

// DeliveryScopeEstablishedHandler 是本消费门转交的处理方。真实装配接 RecheckWithinEstablishedScope。
type DeliveryScopeEstablishedHandler interface {
	HandleDeliveryScopeEstablished(ctx context.Context, established EstablishedDeliveryScope) error
}

// DeliveryScopeEstablishedConsumer 是重核消费者的又一扇门（ADR-0212 决定六「NO 用同一个 inbox 消费者接」），与别的门铃同用一个
// 消费者名。同名不会互相顶替：inbox 键由（消费者名，来源，事件 ID）认领，本上下文各种门铃的 ID 各带口名前缀。处理方交回错误（身份
// 核对读不动等）整笔回滚，inbox 无账，信封退避重投。
type DeliveryScopeEstablishedConsumer struct {
	gate *inboxconsume.Gate[EstablishedDeliveryScope]
}

func NewDeliveryScopeEstablishedConsumer(
	transactor bentoapp.Transactor,
	store *inbox.Store,
	handler DeliveryScopeEstablishedHandler,
) (*DeliveryScopeEstablishedConsumer, error) {
	if transactor == nil {
		return nil, fmt.Errorf("node operations inbox: transactor is nil")
	}
	if store == nil {
		return nil, fmt.Errorf("node operations inbox: inbox store is nil")
	}
	if handler == nil {
		return nil, fmt.Errorf("node operations inbox: handler is nil")
	}
	gate, err := inboxconsume.New(inboxconsume.Spec[EstablishedDeliveryScope]{
		Transactor:     transactor,
		Store:          store,
		Name:           recheckPendingIdentitiesConsumerName,
		EventType:      DeliveryScopeEstablishedEventType,
		Decode:         decodeEstablishedDeliveryScope,
		Handle:         handler.HandleDeliveryScopeEstablished,
		UnexpectedType: "node operations inbox",
		HandleVerb:     "recheck pending identities within the established delivery scope",
	})
	if err != nil {
		return nil, err
	}
	return &DeliveryScopeEstablishedConsumer{gate: gate}, nil
}

func (consumer *DeliveryScopeEstablishedConsumer) Consume(ctx context.Context, envelope eventing.Envelope) error {
	return consumer.gate.Consume(ctx, envelope)
}

// decodeEstablishedDeliveryScope 译载荷。租户与交接单缺一或空白即毒丸：重核按这两维找名下的实物，缺了永远找不着；判断版本给了却
// 空白同样是毒丸——那一封本该只指名一版。
func decodeEstablishedDeliveryScope(payload []byte) (EstablishedDeliveryScope, error) {
	var body struct {
		TenantID        string  `json:"tenantId"`
		HandoverNote    string  `json:"handoverNote"`
		JudgmentVersion *string `json:"judgmentVersion"`
	}
	if err := json.Unmarshal(payload, &body); err != nil {
		return EstablishedDeliveryScope{}, fmt.Errorf("%w: %v", ErrPoisonEnvelope, err)
	}
	tenant, err := domain.NewTenantID(body.TenantID)
	if err != nil {
		return EstablishedDeliveryScope{}, fmt.Errorf("%w: tenantId: %v", ErrPoisonEnvelope, err)
	}
	note, err := domain.NewDeliveryHandoverNoteID(body.HandoverNote)
	if err != nil {
		return EstablishedDeliveryScope{}, fmt.Errorf("%w: handoverNote: %v", ErrPoisonEnvelope, err)
	}
	established := EstablishedDeliveryScope{TenantID: tenant, Note: note}
	if body.JudgmentVersion != nil {
		if established.Version, err = domain.NewIntakeResultVersion(*body.JudgmentVersion); err != nil {
			return EstablishedDeliveryScope{}, fmt.Errorf("%w: judgmentVersion: %v", ErrPoisonEnvelope, err)
		}
	}
	return established, nil
}

// HandoverNoteRecheck 是「交付范围成立」门铃要的两个重核入口，application.RecheckPendingIdentityHandler 满足它。
type HandoverNoteRecheck interface {
	NamedRecheck
	RecheckByHandoverNote(ctx context.Context, tenant domain.TenantID, note domain.DeliveryHandoverNoteID) ([]application.RecheckPendingIdentityResult, error)
}

// RecheckWithinEstablishedScope 把门铃译成重核（ADR-0212 决定六）：只指名交接单的那一封按交接单重核名下的重核对象，带判断版本的那
// 一封只重核那一版。重核的各格业务结果都是形成了的答案，门铃就此处理完；错误原样交回，消费门据此整笔回滚、信封重投——身份核对仍
// 读不动时带着 application.ErrParcelIdentityUnreadable，装配据此记未决。
type RecheckWithinEstablishedScope struct {
	recheck HandoverNoteRecheck
}

var _ DeliveryScopeEstablishedHandler = (*RecheckWithinEstablishedScope)(nil)

func NewRecheckWithinEstablishedScope(recheck HandoverNoteRecheck) (*RecheckWithinEstablishedScope, error) {
	if recheck == nil {
		return nil, errors.New("node operations inbox: recheck within the established scope needs the recheck orchestration")
	}
	return &RecheckWithinEstablishedScope{recheck: recheck}, nil
}

// HandleDeliveryScopeEstablished 按门铃的形状分派。指名的那一版已被接续（重核过了、或另一封门铃先到）由重核答已有结果，不是错误。
func (adapter *RecheckWithinEstablishedScope) HandleDeliveryScopeEstablished(ctx context.Context, established EstablishedDeliveryScope) error {
	if established.Version.String() != "" {
		_, err := adapter.recheck.Recheck(ctx, application.RecheckPendingIdentityCommand{
			TenantID: established.TenantID,
			Version:  established.Version,
		})
		return err
	}
	_, err := adapter.recheck.RecheckByHandoverNote(ctx, established.TenantID, established.Note)
	return err
}
