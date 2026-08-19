package veinbox

import (
	"context"
	"encoding/json"
	"fmt"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	"go.idp.xyz/idp-bento-go/eventing"
	"go.idp.xyz/idp-bento-go/postgres/inbox"

	"go.idp.xyz/idp-parcel/internal/platform/inboxconsume"
)

// declarationSubmissionConsumerName 是本消费者在 inbox 键上的稳定名。Inbox 键只由
// （消费者名 + 来源 + 事件 ID）认领，与同源案件消费账分开；改名等于换消费者。
const declarationSubmissionConsumerName = "visibility-exception/derive-projection-from-declaration-submission"

// DeclarationSubmissionFormedEventType 是本消费者认的事件类型：CC 的申报提交版本形成。
// 消费方自己写出这个字符串，不导入提供方 outbox 适配器的未导出常量。
//
// 一封信带全体成员——载荷是提交幂等键三维加提交版本维，成员快照由处理方按键重读
// 本体取回。这条事实没有对象级替代品，按 ADR-0066 在消费侧循环拆分。信封 ID 今天
// 不含版本维（同键第二版今天造不出来，修复已挂「原案内更正」编排票），版本身份的
// 权威来源是载荷的 versionId，本消费链按它设计，不等信封换 ID。
const DeclarationSubmissionFormedEventType eventing.EventType = "customs-compliance.declaration-submission.formed"

// FormedDeclarationSubmission 是译码后的提交版本引用——只有引用，提交本体（含成员
// 快照与卷宗）由处理方按引用重新取（权威事实留在 customs-compliance）。
type FormedDeclarationSubmission struct {
	TenantID  string
	UnitID    string
	Procedure string
	VersionID string
}

// FormedDeclarationSubmissionHandler 是本消费者转交的处理方。真实装配接
// DeriveOnDeclarationSubmissionAdapter。
type FormedDeclarationSubmissionHandler interface {
	HandleFormedDeclarationSubmission(ctx context.Context, formed FormedDeclarationSubmission) error
}

// DeclarationSubmissionConsumer 把 CC 申报提交版本信封推进消费门。
type DeclarationSubmissionConsumer struct {
	gate *inboxconsume.Gate[FormedDeclarationSubmission]
}

func NewDeclarationSubmissionConsumer(
	transactor bentoapp.Transactor,
	store *inbox.Store,
	handler FormedDeclarationSubmissionHandler,
) (*DeclarationSubmissionConsumer, error) {
	if transactor == nil {
		return nil, fmt.Errorf("visibility exception inbox: transactor is nil")
	}
	if store == nil {
		return nil, fmt.Errorf("visibility exception inbox: inbox store is nil")
	}
	if handler == nil {
		return nil, fmt.Errorf("visibility exception inbox: handler is nil")
	}
	gate, err := inboxconsume.New(inboxconsume.Spec[FormedDeclarationSubmission]{
		Transactor:     transactor,
		Store:          store,
		Name:           declarationSubmissionConsumerName,
		EventType:      DeclarationSubmissionFormedEventType,
		Decode:         decodeFormedDeclarationSubmission,
		Handle:         handler.HandleFormedDeclarationSubmission,
		UnexpectedType: "visibility exception inbox",
		HandleVerb:     "handle formed declaration submission",
	})
	if err != nil {
		return nil, err
	}
	return &DeclarationSubmissionConsumer{gate: gate}, nil
}

// Consume 处理一份投递。舞步在 inboxconsume。
func (consumer *DeclarationSubmissionConsumer) Consume(ctx context.Context, envelope eventing.Envelope) error {
	return consumer.gate.Consume(ctx, envelope)
}

// decodeFormedDeclarationSubmission 译载荷。三维键加版本维缺一即毒丸——处理方按
// （租户+单元+程序）取回提交并按 versionId 核对版本身份，缺了永远取不着也核不了，
// 而重投同样内容不会长出字段来。
func decodeFormedDeclarationSubmission(payload []byte) (FormedDeclarationSubmission, error) {
	var body struct {
		TenantID  string `json:"tenantId"`
		UnitID    string `json:"unitId"`
		Procedure string `json:"procedure"`
		VersionID string `json:"versionId"`
	}
	if err := json.Unmarshal(payload, &body); err != nil {
		return FormedDeclarationSubmission{}, fmt.Errorf("%w: %v", ErrPoisonEnvelope, err)
	}
	if body.TenantID == "" || body.UnitID == "" || body.Procedure == "" || body.VersionID == "" {
		return FormedDeclarationSubmission{}, fmt.Errorf("%w: missing declaration submission key fields", ErrPoisonEnvelope)
	}
	return FormedDeclarationSubmission{
		TenantID:  body.TenantID,
		UnitID:    body.UnitID,
		Procedure: body.Procedure,
		VersionID: body.VersionID,
	}, nil
}
