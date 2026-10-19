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

// UncheckedPendingIdentityRecordedEventType 是落下未核对行时本上下文自投的重核门铃（ADR-0199 决定三第三个触发）。消费方自己写出
// 这个字符串，不导入写出方 outbox 适配器的常量，同 PS 门铃那一个；两串由组合根的一条对照用例钉住。
const UncheckedPendingIdentityRecordedEventType eventing.EventType = "node-operations.unchecked-pending-identity.recorded"

// RecordedUncheckedPendingIdentity 是译码后的门铃：只指名那一版判断。门铃不是身份答案，重核照旧经身份核对读口问 PS。
type RecordedUncheckedPendingIdentity struct {
	TenantID domain.TenantID
	Version  domain.IntakeResultVersion
}

// UncheckedPendingIdentityHandler 是本消费门转交的处理方。真实装配接只看指名那一件的重核（RecheckNamedPendingIdentity）。
type UncheckedPendingIdentityHandler interface {
	HandleUncheckedPendingIdentity(ctx context.Context, recorded RecordedUncheckedPendingIdentity) error
}

// UncheckedPendingIdentityRecordedConsumer 把自投门铃推进消费门。它与 PS 门铃那一扇门同用一个消费者名（ADR-0199 决定三「NO 用同一个
// inbox 消费者接」）：重核是同一个消费者的事，只是这一路只看指名那一件。同名不会互相顶替——inbox 键由（消费者名，来源，事件 ID）
// 认领，两种门铃的来源不同（本上下文对 parcel-shipment），ID 也各自带口名前缀。
type UncheckedPendingIdentityRecordedConsumer struct {
	gate *inboxconsume.Gate[RecordedUncheckedPendingIdentity]
}

func NewUncheckedPendingIdentityRecordedConsumer(
	transactor bentoapp.Transactor,
	store *inbox.Store,
	handler UncheckedPendingIdentityHandler,
) (*UncheckedPendingIdentityRecordedConsumer, error) {
	if transactor == nil {
		return nil, fmt.Errorf("node operations inbox: transactor is nil")
	}
	if store == nil {
		return nil, fmt.Errorf("node operations inbox: inbox store is nil")
	}
	if handler == nil {
		return nil, fmt.Errorf("node operations inbox: handler is nil")
	}
	gate, err := inboxconsume.New(inboxconsume.Spec[RecordedUncheckedPendingIdentity]{
		Transactor:     transactor,
		Store:          store,
		Name:           recheckPendingIdentitiesConsumerName,
		EventType:      UncheckedPendingIdentityRecordedEventType,
		Decode:         decodeRecordedUncheckedPendingIdentity,
		Handle:         handler.HandleUncheckedPendingIdentity,
		UnexpectedType: "node operations inbox",
		HandleVerb:     "recheck the named unchecked pending identity",
	})
	if err != nil {
		return nil, err
	}
	return &UncheckedPendingIdentityRecordedConsumer{gate: gate}, nil
}

func (consumer *UncheckedPendingIdentityRecordedConsumer) Consume(ctx context.Context, envelope eventing.Envelope) error {
	return consumer.gate.Consume(ctx, envelope)
}

// decodeRecordedUncheckedPendingIdentity 译载荷。tenantId 与 judgmentVersion 缺一或空白即毒丸：重核按这两维指名那一件，缺了永远指
// 不中。
func decodeRecordedUncheckedPendingIdentity(payload []byte) (RecordedUncheckedPendingIdentity, error) {
	var body struct {
		TenantID        string `json:"tenantId"`
		JudgmentVersion string `json:"judgmentVersion"`
	}
	if err := json.Unmarshal(payload, &body); err != nil {
		return RecordedUncheckedPendingIdentity{}, fmt.Errorf("%w: %v", ErrPoisonEnvelope, err)
	}
	tenant, err := domain.NewTenantID(body.TenantID)
	if err != nil {
		return RecordedUncheckedPendingIdentity{}, fmt.Errorf("%w: tenantId: %v", ErrPoisonEnvelope, err)
	}
	version, err := domain.NewIntakeResultVersion(body.JudgmentVersion)
	if err != nil {
		return RecordedUncheckedPendingIdentity{}, fmt.Errorf("%w: judgmentVersion: %v", ErrPoisonEnvelope, err)
	}
	return RecordedUncheckedPendingIdentity{TenantID: tenant, Version: version}, nil
}

// NamedRecheck 是只看指名那一件的重核入口，application.RecheckPendingIdentityHandler 满足它。
type NamedRecheck interface {
	Recheck(ctx context.Context, command application.RecheckPendingIdentityCommand) (application.RecheckPendingIdentityResult, error)
}

// RecheckNamedPendingIdentity 把自投门铃译成只看那一件的重核（ADR-0199 决定三）。重核的各格业务结果（追加了一版、判断不变、已被
// 接续）都是形成了的答案，门铃就此处理完；错误原样交回，消费门据此整笔回滚、信封重投——身份核对仍读不动时带着
// application.ErrParcelIdentityUnreadable，装配据此记未决。
type RecheckNamedPendingIdentity struct {
	recheck NamedRecheck
}

var _ UncheckedPendingIdentityHandler = (*RecheckNamedPendingIdentity)(nil)

func NewRecheckNamedPendingIdentity(recheck NamedRecheck) (*RecheckNamedPendingIdentity, error) {
	if recheck == nil {
		return nil, errors.New("node operations inbox: named recheck needs the recheck orchestration")
	}
	return &RecheckNamedPendingIdentity{recheck: recheck}, nil
}

// HandleUncheckedPendingIdentity 指名那一版重核。门铃与未核对行同一笔落下，指名的版本在册上找不到（ErrRecheckSubjectNotFound）
// 只可能是库被绕过编排改过：照错误交回，让它占着失败预算被人看见，不当成处理完了悄悄丢掉。
func (adapter *RecheckNamedPendingIdentity) HandleUncheckedPendingIdentity(
	ctx context.Context,
	recorded RecordedUncheckedPendingIdentity,
) error {
	if _, err := adapter.recheck.Recheck(ctx, application.RecheckPendingIdentityCommand{
		TenantID: recorded.TenantID,
		Version:  recorded.Version,
	}); err != nil {
		return err
	}
	return nil
}
