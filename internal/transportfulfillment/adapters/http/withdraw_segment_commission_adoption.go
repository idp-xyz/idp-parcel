package tfhttp

import (
	"context"
	"net/http"
	"strings"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 解除履约段采用口（ADR-0232 决定一至三；票 demo-chain-gaps/57）。运营人员撤回「这一段在这份委托下履约」那条声明，是管理台上作出的
// 生产决定，挂操作者渠道「运营决定」能力面的解除履约段采用一种。载荷只有内容没有身份：租户与操作者都从信封来。

// SegmentCommissionAdoptionWithdrawalIntake 把已认证的运营决定请求翻译成解除命令。
type SegmentCommissionAdoptionWithdrawalIntake interface {
	IntakeSegmentCommissionAdoptionWithdrawal(ctx context.Context, request *http.Request) (application.WithdrawSegmentCommissionAdoptionCommand, error)
}

// SegmentCommissionAdoptionWithdrawer 是本适配器转交的应用编排。
type SegmentCommissionAdoptionWithdrawer interface {
	Withdraw(ctx context.Context, command application.WithdrawSegmentCommissionAdoptionCommand) (application.AdoptionWithdrawalResult, error)
}

// SegmentCommissionAdoptionWithdrawalPayload 是解除口的线格式：哪一段、解的是哪一代采用（采用口回显的 generation）、那一代采用的是
// 哪一份委托。三格都必填；代次缺了答未受理。委托的开始此后怎么变不收：由其余各段的采用定。
type SegmentCommissionAdoptionWithdrawalPayload struct {
	Segment    string `json:"segment"`
	Commission string `json:"commission"`
	Generation int    `json:"generation"`
}

// Command 把载荷连同信封给的租户与操作者翻成解除命令。各格成不成形由编排答未受理（200）。
func (payload SegmentCommissionAdoptionWithdrawalPayload) Command(operator OperatorIdentity) (application.WithdrawSegmentCommissionAdoptionCommand, error) {
	if operator.Tenant.String() == "" || strings.TrimSpace(operator.Operator) == "" {
		return application.WithdrawSegmentCommissionAdoptionCommand{}, ErrOperatorIdentityMissing
	}
	return application.WithdrawSegmentCommissionAdoptionCommand{
		TenantID:   operator.Tenant,
		Operator:   operator.Operator,
		Segment:    payload.Segment,
		Commission: payload.Commission,
		Generation: payload.Generation,
	}, nil
}

// NewWithdrawSegmentCommissionAdoptionEndpoint 交回解除口的 HTTP 入口。
func NewWithdrawSegmentCommissionAdoptionEndpoint(intake SegmentCommissionAdoptionWithdrawalIntake, withdrawer SegmentCommissionAdoptionWithdrawer) http.Handler {
	return commandEndpoint(func(request *http.Request) (application.AdoptionWithdrawalResult, error, bool) {
		command, err := intake.IntakeSegmentCommissionAdoptionWithdrawal(request.Context(), request)
		if err != nil {
			return application.AdoptionWithdrawalResult{}, err, false
		}
		result, err := withdrawer.Withdraw(request.Context(), command)
		return result, err, true
	}, writeAdoptionWithdrawalOutcome)
}

// adoptionWithdrawalResponse 是解除口的封闭响应形状。`withdrawal` 随解除成立、重放与点名那一代已解除而委托不符出现；`adoption` 随
// 解除成立、代次不是当前、委托不符与已形成发生项出现，是段上那一份当前采用（带它的代次）；`commission` 只随解除成立出现，是委托此后的开始（撤回时不带开始两格）；`formedOccurrences` 只随
// `OCCURRENCES_FORMED` 出现，是这一段上按这份委托形成、此刻仍有效的各项，逐项失效之后再来解除。
type adoptionWithdrawalResponse struct {
	Outcome               string                         `json:"outcome"`
	UndecidedReason       string                         `json:"undecidedReason,omitempty"`
	ContinuationReference string                         `json:"continuationReference,omitempty"`
	Withdrawal            *adoptionWithdrawalBody        `json:"withdrawal,omitempty"`
	Adoption              *segmentCommissionAdoptionBody `json:"adoption,omitempty"`
	Commission            *commissionTransitionBody      `json:"commission,omitempty"`
	FormedOccurrences     []chargeOccurrenceBody         `json:"formedOccurrences,omitempty"`
}

// adoptionWithdrawalBody 逐字段透出一次解除：被解除的那一代、谁解的、委托的开始之前与之后（撤回时之后两格不出现）和变法。
type adoptionWithdrawalBody struct {
	Segment          string `json:"segment"`
	Generation       int    `json:"generation"`
	Commission       string `json:"commission"`
	WithdrawnBy      string `json:"withdrawnBy"`
	StartBeforeBasis string `json:"startBeforeBasis"`
	StartBeforeAt    string `json:"startBeforeAt"`
	StartChange      string `json:"startChange"`
	StartAfterBasis  string `json:"startAfterBasis,omitempty"`
	StartAfterAt     string `json:"startAfterAt,omitempty"`
	RecordedAt       string `json:"recordedAt"`
}

func writeAdoptionWithdrawalOutcome(response http.ResponseWriter, result application.AdoptionWithdrawalResult) {
	outcome := result.Outcome().String()
	if outcome == "" {
		writeProblem(response, http.StatusInternalServerError, codeUnnamedOutcome)
		return
	}
	body := adoptionWithdrawalResponse{
		Outcome:               outcome,
		UndecidedReason:       result.UndecidedReason().String(),
		ContinuationReference: result.ContinuationReference(),
	}
	if record, present := result.Withdrawal(); present {
		body.Withdrawal = adoptionWithdrawalBodyOf(record)
	}
	if record, present := result.Adoption(); present {
		body.Adoption = segmentCommissionAdoptionBodyOf(record)
	}
	if record, present := result.Commission(); present {
		body.Commission = commissionTransitionBodyOf(record.Commission, record.RecordedAt)
	}
	for _, record := range result.FormedOccurrences() {
		body.FormedOccurrences = append(body.FormedOccurrences, chargeOccurrenceBodyOf(record))
	}
	// ADR-0022：只有这次真解除了一段才用 201；重放、不在册、代次不是当前、委托不符、已形成发生项、未受理、未决都是形成了的答案（200）。
	status := http.StatusOK
	if result.Outcome() == application.AdoptionWithdrawn {
		status = http.StatusCreated
	}
	writeJSON(response, status, body)
}

func adoptionWithdrawalBodyOf(record ports.SegmentCommissionAdoptionWithdrawalRecord) *adoptionWithdrawalBody {
	withdrawal := record.Withdrawal
	beforeBasis, beforeAt := withdrawal.StartBefore()
	echoed := adoptionWithdrawalBody{
		Segment:          withdrawal.Segment().String(),
		Generation:       withdrawal.Generation(),
		Commission:       withdrawal.Commission().String(),
		WithdrawnBy:      withdrawal.WithdrawnBy().String(),
		StartBeforeBasis: beforeBasis.String(),
		StartBeforeAt:    rfc3339(beforeAt),
		StartChange:      withdrawal.StartChange().String(),
		RecordedAt:       rfc3339(record.RecordedAt),
	}
	if afterBasis, afterAt, present := withdrawal.StartAfter(); present {
		echoed.StartAfterBasis, echoed.StartAfterAt = afterBasis.String(), rfc3339(afterAt)
	}
	return &echoed
}

func segmentCommissionAdoptionBodyOf(record ports.SegmentCommissionAdoptionRecord) *segmentCommissionAdoptionBody {
	adoption := record.Adoption
	basis, startedAt := adoption.Started()
	return &segmentCommissionAdoptionBody{
		Segment:    adoption.Segment().String(),
		Generation: record.Generation,
		Commission: adoption.Commission().String(),
		Provider:   adoption.Provider().String(),
		StartBasis: basis.String(),
		StartedAt:  rfc3339(startedAt),
		RecordedAt: rfc3339(record.RecordedAt),
	}
}
