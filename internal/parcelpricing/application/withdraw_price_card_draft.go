package application

import (
	"context"
	"errors"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
)

// 已批准价卡草稿的撤下。撤下只让这一版不能再发布，不放行任何东西，所以不读审批职责规则，也不查报价册——报价的发出与答复
// 还不在本上下文。谁可以撤下跟发布同一张登记写面，由接入渠道认；这里记下的撤下人是命令里的操作者主体，不从原因里读身份。

// WithdrawPriceCardDraftOutcome 是撤下的答复。尚未批准答`未批准`，已发布答`已发布`（去处是后继截止或退役，不是撤下）。
// 空原因与含控制字符的原因不受理。已撤下再撤一次不改原来的人、时刻与原因。
type WithdrawPriceCardDraftOutcome uint8

const (
	WithdrawPriceCardDraftOutcomeInvalid WithdrawPriceCardDraftOutcome = iota
	PriceCardDraftWithdrawn
	PriceCardWithdrawalDraftNotFound
	PriceCardWithdrawalDraftNotApproved
	PriceCardWithdrawalDraftPublished
	PriceCardWithdrawalNotAccepted
	PriceCardWithdrawalDraftChanged
)

func (outcome WithdrawPriceCardDraftOutcome) String() string {
	switch outcome {
	case PriceCardDraftWithdrawn:
		return "DRAFT_WITHDRAWN"
	case PriceCardWithdrawalDraftNotFound:
		return "DRAFT_NOT_FOUND"
	case PriceCardWithdrawalDraftNotApproved:
		return "DRAFT_NOT_APPROVED"
	case PriceCardWithdrawalDraftPublished:
		return "DRAFT_PUBLISHED"
	case PriceCardWithdrawalNotAccepted:
		return "NOT_ACCEPTED"
	case PriceCardWithdrawalDraftChanged:
		return "DRAFT_CHANGED"
	default:
		return ""
	}
}

// WithdrawPriceCardDraftCommand 按（租户，方案版本）指名草稿。撤下人是操作者主体，原因由调用方交来；时刻不在命令里，取时钟。
type WithdrawPriceCardDraftCommand struct {
	Tenant     domain.TenantID
	Plan       domain.VersionReference
	Withdrawer domain.OperatorSubject
	Reason     string
}

// WithdrawPriceCardDraftResult 是撤下的结果。Draft 在这次写下了撤下、或册上本来就是已撤下时在场。
type WithdrawPriceCardDraftResult struct {
	Outcome  WithdrawPriceCardDraftOutcome
	Draft    domain.PriceCardDraft
	HasDraft bool
}

// WithdrawPriceCardDraftHandler 只依赖草稿册与时钟。不接审批职责规则：撤下不另套那一道门。
type WithdrawPriceCardDraftHandler struct {
	drafts ports.PriceCardDraftProgress
	clock  ports.Clock
}

func NewWithdrawPriceCardDraftHandler(drafts ports.PriceCardDraftProgress, clock ports.Clock) (*WithdrawPriceCardDraftHandler, error) {
	if drafts == nil || clock == nil {
		return nil, errors.New("parcel pricing: price card draft withdrawal needs a draft register and a clock")
	}
	return &WithdrawPriceCardDraftHandler{drafts: drafts, clock: clock}, nil
}

// Handle 先按草稿此刻的状态答。只有`已批准`才记撤下人、服务端时刻与原因并写回。已撤下再来一次交回册上那一份，不覆盖痕迹。
func (handler *WithdrawPriceCardDraftHandler) Handle(ctx context.Context, command WithdrawPriceCardDraftCommand) (WithdrawPriceCardDraftResult, error) {
	draft, found, err := handler.drafts.LoadPriceCardDraft(ctx, command.Tenant, command.Plan)
	if err != nil {
		return WithdrawPriceCardDraftResult{}, fmt.Errorf("withdraw price card draft: %w", err)
	}
	if !found {
		return WithdrawPriceCardDraftResult{Outcome: PriceCardWithdrawalDraftNotFound}, nil
	}
	switch draft.Status() {
	case domain.PriceCardDraftStatusDraft, domain.PriceCardDraftStatusValidated:
		return WithdrawPriceCardDraftResult{Outcome: PriceCardWithdrawalDraftNotApproved}, nil
	case domain.PriceCardDraftStatusPublished:
		return WithdrawPriceCardDraftResult{Outcome: PriceCardWithdrawalDraftPublished}, nil
	case domain.PriceCardDraftStatusWithdrawn:
		return WithdrawPriceCardDraftResult{Outcome: PriceCardDraftWithdrawn, Draft: draft, HasDraft: true}, nil
	}

	withdrawn, err := draft.Withdraw(command.Withdrawer, command.Reason, handler.clock.Now())
	switch {
	case errors.Is(err, domain.ErrPriceCardWithdrawalReasonRejected):
		return WithdrawPriceCardDraftResult{Outcome: PriceCardWithdrawalNotAccepted}, nil
	case err != nil:
		return WithdrawPriceCardDraftResult{}, fmt.Errorf("withdraw price card draft: %w", err)
	}
	outcome, err := handler.drafts.AdvancePriceCardDraft(ctx, withdrawn)
	if err != nil {
		return WithdrawPriceCardDraftResult{}, fmt.Errorf("withdraw price card draft: %w", err)
	}
	switch outcome {
	case ports.PriceCardDraftAdvanced:
		return WithdrawPriceCardDraftResult{Outcome: PriceCardDraftWithdrawn, Draft: withdrawn, HasDraft: true}, nil
	case ports.PriceCardDraftAdvanceNotFound:
		return WithdrawPriceCardDraftResult{Outcome: PriceCardWithdrawalDraftNotFound}, nil
	case ports.PriceCardDraftAdvanceSuperseded:
		return WithdrawPriceCardDraftResult{Outcome: PriceCardWithdrawalDraftChanged}, nil
	default:
		return WithdrawPriceCardDraftResult{}, fmt.Errorf("withdraw price card draft: unexpected advance outcome %q", outcome)
	}
}
