package ccinbox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	"go.idp.xyz/idp-bento-go/eventing"
	"go.idp.xyz/idp-bento-go/postgres/inbox"

	"go.idp.xyz/idp-parcel/internal/customscompliance/application"
	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/platform/inboxconsume"
)

// 申报发送消费者（ADR-0342 决定一）：本上下文自己的「提交版本已形成」信封就是发送意图，dispatch 投到这里，由它把首次尝试发出去。
// 与 VE 投影那一路分名、各记各的 inbox 账。

const declarationSendConsumerName = "customs-compliance/send-declaration"

// DeclarationSubmissionFormedEventType 是本消费者认的事件类型。字符串由消费方自己写出，与提供方是否相等由 cmd/parcel-dispatch 的
// 真库装配用例钉。
const DeclarationSubmissionFormedEventType eventing.EventType = "customs-compliance.declaration-submission.formed"

// ErrDeclarationSendUndecided 表示发送停在依赖上或确知没发出去，重投会改变结果。生产装配把它登进 WithUndecidedSentinels。
var ErrDeclarationSendUndecided = errors.New("customs compliance inbox: sending the declaration is undecided")

type formedSubmission struct {
	TenantID  string
	VersionID string
}

// DeclarationSender 是本消费者转交的那一口（application.SendDeclarationHandler）。
type DeclarationSender interface {
	Send(ctx context.Context, tenant domain.TenantID, version domain.SubmissionVersionID) (application.SendDeclarationResult, error)
}

type DeclarationSendConsumer struct {
	gate *inboxconsume.Gate[formedSubmission]
}

func NewDeclarationSendConsumer(
	transactor bentoapp.Transactor,
	store *inbox.Store,
	sender DeclarationSender,
) (*DeclarationSendConsumer, error) {
	if transactor == nil || store == nil || sender == nil {
		return nil, fmt.Errorf("customs compliance inbox: declaration send consumer dependency is nil")
	}
	gate, err := inboxconsume.New(inboxconsume.Spec[formedSubmission]{
		Transactor: transactor,
		Store:      store,
		Name:       declarationSendConsumerName,
		EventType:  DeclarationSubmissionFormedEventType,
		Decode:     decodeFormedSubmission,
		Handle: func(ctx context.Context, formed formedSubmission) error {
			return handleFormedSubmission(ctx, sender, formed)
		},
		UnexpectedType: "customs compliance inbox",
		HandleVerb:     "send declaration",
	})
	if err != nil {
		return nil, err
	}
	return &DeclarationSendConsumer{gate: gate}, nil
}

func (consumer *DeclarationSendConsumer) Consume(ctx context.Context, envelope eventing.Envelope) error {
	return consumer.gate.Consume(ctx, envelope)
}

// handleFormedSubmission 交发送编排。通道没登、已发过、发出去了（不论答复）都入账；只有等依赖的那一格交回续办哨兵。
func handleFormedSubmission(ctx context.Context, sender DeclarationSender, formed formedSubmission) error {
	tenant, err := domain.NewTenantID(formed.TenantID)
	if err != nil {
		return fmt.Errorf("%w: tenant: %v", ErrPoisonEnvelope, err)
	}
	version, err := domain.NewSubmissionVersionID(formed.VersionID)
	if err != nil {
		return fmt.Errorf("%w: version: %v", ErrPoisonEnvelope, err)
	}
	result, err := sender.Send(ctx, tenant, version)
	if err != nil {
		return err
	}
	if result.Outcome() == application.DeclarationSendUndecided {
		return fmt.Errorf("%w: %v", ErrDeclarationSendUndecided, result.UndecidedCause())
	}
	return nil
}

func decodeFormedSubmission(payload []byte) (formedSubmission, error) {
	var body struct {
		TenantID  string `json:"tenantId"`
		VersionID string `json:"versionId"`
	}
	if err := json.Unmarshal(payload, &body); err != nil {
		return formedSubmission{}, fmt.Errorf("%w: %v", ErrPoisonEnvelope, err)
	}
	if body.TenantID == "" || body.VersionID == "" {
		return formedSubmission{}, fmt.Errorf("%w: missing submission version reference fields", ErrPoisonEnvelope)
	}
	return formedSubmission{TenantID: body.TenantID, VersionID: body.VersionID}, nil
}
