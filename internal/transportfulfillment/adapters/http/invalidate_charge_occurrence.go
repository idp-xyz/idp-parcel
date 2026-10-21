package tfhttp

import (
	"context"
	"net/http"
	"strings"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
)

// 运输收费发生项失效口（ADR-0232 决定四；票 demo-chain-gaps/57）。运营人员说一项已登记的发生项所说的范围不成立，是管理台上作出的
// 生产决定，挂操作者渠道「运营决定」能力面的运输收费发生项失效一种。载荷只有内容没有身份：租户与操作者都从信封来。

// ChargeOccurrenceInvalidationIntake 把已认证的运营决定请求翻译成失效命令。
type ChargeOccurrenceInvalidationIntake interface {
	IntakeChargeOccurrenceInvalidation(ctx context.Context, request *http.Request) (application.InvalidateChargeOccurrenceCommand, error)
}

// ChargeOccurrenceInvalidator 是本适配器转交的应用编排。
type ChargeOccurrenceInvalidator interface {
	Invalidate(ctx context.Context, command application.InvalidateChargeOccurrenceCommand) (application.ChargeOccurrenceInvalidationResult, error)
}

// ChargeOccurrenceInvalidationPayload 是失效口的线格式。version 是作决定的人看到的那一版（链头），basis 是他凭的那一条引用（证据
// 或决定编号），原样记作修订依据。新版本号、修订时刻与修订者都不收：前者由前一版派生，后两者是这一次决定落册的时刻与认证出的人。
type ChargeOccurrenceInvalidationPayload struct {
	Occurrence string `json:"occurrence"`
	Version    string `json:"version"`
	Basis      string `json:"basis"`
}

// Command 把载荷连同信封给的租户与操作者翻成失效命令。各格成不成形由编排答未受理（200）。
func (payload ChargeOccurrenceInvalidationPayload) Command(operator OperatorIdentity) (application.InvalidateChargeOccurrenceCommand, error) {
	if operator.Tenant.String() == "" || strings.TrimSpace(operator.Operator) == "" {
		return application.InvalidateChargeOccurrenceCommand{}, ErrOperatorIdentityMissing
	}
	return application.InvalidateChargeOccurrenceCommand{
		TenantID:   operator.Tenant,
		Operator:   operator.Operator,
		Occurrence: payload.Occurrence,
		Version:    payload.Version,
		Basis:      payload.Basis,
	}, nil
}

// NewInvalidateChargeOccurrenceEndpoint 交回失效口的 HTTP 入口。
func NewInvalidateChargeOccurrenceEndpoint(intake ChargeOccurrenceInvalidationIntake, invalidator ChargeOccurrenceInvalidator) http.Handler {
	return commandEndpoint(func(request *http.Request) (application.ChargeOccurrenceInvalidationResult, error, bool) {
		command, err := intake.IntakeChargeOccurrenceInvalidation(request.Context(), request)
		if err != nil {
			return application.ChargeOccurrenceInvalidationResult{}, err, false
		}
		result, err := invalidator.Invalidate(request.Context(), command)
		return result, err, true
	}, writeChargeOccurrenceInvalidationOutcome)
}

// chargeOccurrenceInvalidationResponse 是失效口的封闭响应形状。occurrence 在失效成立与重放时是那一版失效版本，在点名的不是链头与
// 已失效时是此刻的链头，字段与登记口回显的发生项同形。
type chargeOccurrenceInvalidationResponse struct {
	Outcome               string                `json:"outcome"`
	UndecidedReason       string                `json:"undecidedReason,omitempty"`
	ContinuationReference string                `json:"continuationReference,omitempty"`
	Occurrence            *chargeOccurrenceBody `json:"occurrence,omitempty"`
}

func writeChargeOccurrenceInvalidationOutcome(response http.ResponseWriter, result application.ChargeOccurrenceInvalidationResult) {
	outcome := result.Outcome().String()
	if outcome == "" {
		writeProblem(response, http.StatusInternalServerError, codeUnnamedOutcome)
		return
	}
	body := chargeOccurrenceInvalidationResponse{
		Outcome:               outcome,
		UndecidedReason:       result.UndecidedReason().String(),
		ContinuationReference: result.ContinuationReference(),
	}
	if record, present := result.Record(); present {
		occurrence := chargeOccurrenceBodyOf(record)
		body.Occurrence = &occurrence
	}
	// ADR-0022：只有这次真落了失效版本用 201；重放、点名的不是链头、已失效、不在册、未受理、未决都是形成了的答案（200）。
	status := http.StatusOK
	if result.Outcome() == application.ChargeOccurrenceInvalidated {
		status = http.StatusCreated
	}
	writeJSON(response, status, body)
}
