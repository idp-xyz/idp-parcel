package tfhttp

import (
	"context"
	"net/http"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 取消运输委托口（ADR-0204 决定三；ADR-0202 决定一）。运营人员停掉一份尚未开始的运输意图，是管理台上作出的生产决定，挂
// 操作者渠道的「运营决定」能力面、按取消运输委托这一种认证：能提交委托的不因此能取消它。路径取读面册名前缀
// `transport-fulfillment-`，理由同委托提交口。

// TransportCommissionCancellationIntake 把已认证的运营写请求翻译成取消委托命令。
type TransportCommissionCancellationIntake interface {
	IntakeTransportCommissionCancellation(ctx context.Context, request *http.Request) (application.CancelCommissionCommand, error)
}

// TransportCommissionCanceller 是本适配器转交的应用编排。
type TransportCommissionCanceller interface {
	CancelCommission(ctx context.Context, command application.CancelCommissionCommand) (application.CommissionResult, error)
}

// TransportCommissionCancellationPayload 是取消委托口的线格式。载荷只有内容、没有身份（封闭解码）。取消时刻照 ADR-0023 由
// 提交方给。
type TransportCommissionCancellationPayload struct {
	Commission  string `json:"commission"`
	CancelledAt string `json:"cancelledAt"`
}

// Command 把载荷连同信封给的租户翻成取消委托命令。时刻解不出是坏报文（400）；取不取消得了由编排答。
func (payload TransportCommissionCancellationPayload) Command(tenant domain.TenantID) (application.CancelCommissionCommand, error) {
	if tenant.String() == "" {
		return application.CancelCommissionCommand{}, ErrOperatorIdentityMissing
	}
	cancelledAt, err := parseOptionalInstant("cancelledAt", payload.CancelledAt)
	if err != nil {
		return application.CancelCommissionCommand{}, err
	}
	return application.CancelCommissionCommand{TenantID: tenant, Commission: payload.Commission, CancelledAt: cancelledAt}, nil
}

// NewCancelTransportCommissionEndpoint 交回取消委托的 HTTP 入口。
func NewCancelTransportCommissionEndpoint(intake TransportCommissionCancellationIntake, canceller TransportCommissionCanceller) http.Handler {
	return commandEndpoint(func(request *http.Request) (application.CommissionResult, error, bool) {
		command, err := intake.IntakeTransportCommissionCancellation(request.Context(), request)
		if err != nil {
			return application.CommissionResult{}, err, false
		}
		result, err := canceller.CancelCommission(request.Context(), command)
		return result, err, true
	}, writeTransportCommissionCancellationOutcome)
}

// transportCommissionCancellationResponse 是取消委托口的封闭响应形状。`TRANSPORT_ALREADY_STARTED` 带回在册那一份委托与它
// 的开始依据——续办是按事实形成中断、改降、折返等实际结果，不是改单；已取消的重放回显原取消。
type transportCommissionCancellationResponse struct {
	Outcome               string                    `json:"outcome"`
	UndecidedReason       string                    `json:"undecidedReason,omitempty"`
	ContinuationReference string                    `json:"continuationReference,omitempty"`
	Commission            *commissionTransitionBody `json:"commission,omitempty"`
}

// commissionTransitionBody 透出委托的状态转换那几格：开始的依据与时刻、取消时刻。快照各格看委托提交口的回显，这里不重复。
type commissionTransitionBody struct {
	Commission   string `json:"commission"`
	Provider     string `json:"provider"`
	SubmittedAt  string `json:"submittedAt"`
	StartedBasis string `json:"startedBasis,omitempty"`
	StartedAt    string `json:"startedAt,omitempty"`
	CancelledAt  string `json:"cancelledAt,omitempty"`
	RecordedAt   string `json:"recordedAt"`
}

func commissionTransitionBodyOf(commission domain.TransportCommission, recordedAt time.Time) *commissionTransitionBody {
	body := &commissionTransitionBody{
		Commission:  commission.Commission().String(),
		Provider:    commission.Provider().String(),
		SubmittedAt: rfc3339(commission.SubmittedAt()),
		RecordedAt:  rfc3339(recordedAt),
	}
	if basis, at, started := commission.TransportStarted(); started {
		body.StartedBasis = basis.String()
		body.StartedAt = rfc3339(at)
	}
	if at, cancelled := commission.Cancelled(); cancelled {
		body.CancelledAt = rfc3339(at)
	}
	return body
}

func writeTransportCommissionCancellationOutcome(response http.ResponseWriter, result application.CommissionResult) {
	outcome := result.Outcome().String()
	if outcome == "" {
		writeProblem(response, http.StatusInternalServerError, codeUnnamedOutcome)
		return
	}
	body := transportCommissionCancellationResponse{
		Outcome:               outcome,
		UndecidedReason:       result.UndecidedReason().String(),
		ContinuationReference: result.ContinuationReference(),
	}
	if record, present := result.Commission(); present {
		body.Commission = commissionTransitionBodyOf(record.Commission, record.RecordedAt)
	}
	// ADR-0022：只有这次真把委托取消了用 201；已开始、已取消的重放、不在册、未受理、未决都是形成了的答案（200）。
	status := http.StatusOK
	if result.Outcome() == application.CommissionCancelled {
		status = http.StatusCreated
	}
	writeJSON(response, status, body)
}
