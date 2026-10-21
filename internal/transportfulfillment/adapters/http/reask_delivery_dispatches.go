package tfhttp

import (
	"context"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 按派送待形成册批量重问的运营决定口（ADR-0230 决定四、五）：运营人员在管理台上把册上待形成的那几件交回派送发起执行器再问一次所有者。
// 请求里只有问法（全部 / 到期）与一次最多问几件；问哪几件由册定，各件怎么答由执行器定，这一口不收任何段、对象或工作范围。

// DeliveryDispatchReaskIntake 把已认证的一次重问翻成命令：租户取认证结果，问法与件数从请求收。
type DeliveryDispatchReaskIntake interface {
	IntakeDeliveryDispatchReask(ctx context.Context, request *http.Request) (application.ReaskPendingDeliveryDispatchesCommand, error)
}

// DeliveryDispatchReaskPayload 是线格式。scope 取 `ALL` 或 `DUE`；词不在集合内、limit 越界都由编排答`未受理`（200），不是坏报文。
type DeliveryDispatchReaskPayload struct {
	Scope string `json:"scope"`
	Limit int    `json:"limit"`
}

func (payload DeliveryDispatchReaskPayload) Command(tenant domain.TenantID) (application.ReaskPendingDeliveryDispatchesCommand, error) {
	if tenant.String() == "" {
		return application.ReaskPendingDeliveryDispatchesCommand{}, ErrOperatorIdentityMissing
	}
	return application.ReaskPendingDeliveryDispatchesCommand{
		TenantID: tenant,
		Scope:    application.ParseReaskScope(payload.Scope),
		Limit:    payload.Limit,
	}, nil
}

// DeliveryDispatchReasker 是本口转交的编排。
type DeliveryDispatchReasker interface {
	Reask(
		ctx context.Context,
		command application.ReaskPendingDeliveryDispatchesCommand,
	) (application.ReaskPendingDeliveryDispatchesResult, error)
}

var _ DeliveryDispatchReaskIntake = UnconfiguredIntake{}

func (UnconfiguredIntake) IntakeDeliveryDispatchReask(context.Context, *http.Request) (application.ReaskPendingDeliveryDispatchesCommand, error) {
	return application.ReaskPendingDeliveryDispatchesCommand{}, ErrAccessChannelNotConfigured
}

// NewReaskDeliveryDispatchesEndpoint 交回按册重问的 HTTP 入口。
func NewReaskDeliveryDispatchesEndpoint(intake DeliveryDispatchReaskIntake, reasker DeliveryDispatchReasker) http.Handler {
	return commandEndpoint(func(request *http.Request) (application.ReaskPendingDeliveryDispatchesResult, error, bool) {
		command, err := intake.IntakeDeliveryDispatchReask(request.Context(), request)
		if err != nil {
			return application.ReaskPendingDeliveryDispatchesResult{}, err, false
		}
		result, err := reasker.Reask(request.Context(), command)
		return result, err, true
	}, writeDeliveryDispatchReaskOutcome)
}

// deliveryDispatchReaskResponse 是封闭响应形状。到期重问带上所凭的那一版间隔与算出的到期时刻；逐件的答复与补拍口同一套格子
// （outcome、refusal、missing、undecidedReason、task），各件照执行器那一拍的答复透出。
type deliveryDispatchReaskResponse struct {
	Outcome         string                      `json:"outcome"`
	UndecidedReason string                      `json:"undecidedReason,omitempty"`
	AskedAt         string                      `json:"askedAt,omitempty"`
	CadenceVersion  string                      `json:"cadenceVersion,omitempty"`
	IntervalMinutes int64                       `json:"intervalMinutes,omitempty"`
	DueBy           string                      `json:"dueBy,omitempty"`
	Items           []deliveryDispatchReaskItem `json:"items"`
	// Remaining 一律在场：这一次没问到的件数，零也照写——零就是册上该问的这一次都问到了。
	Remaining int `json:"remaining"`
}

type deliveryDispatchReaskItem struct {
	Segment         string   `json:"segment"`
	Object          string   `json:"object"`
	Outcome         string   `json:"outcome"`
	Refusal         string   `json:"refusal,omitempty"`
	Missing         []string `json:"missing,omitempty"`
	UndecidedReason string   `json:"undecidedReason,omitempty"`
	Task            string   `json:"task,omitempty"`
}

func writeDeliveryDispatchReaskOutcome(response http.ResponseWriter, result application.ReaskPendingDeliveryDispatchesResult) {
	outcome := result.Outcome().String()
	if outcome == "" {
		writeProblem(response, http.StatusInternalServerError, codeUnnamedOutcome)
		return
	}
	body := deliveryDispatchReaskResponse{
		Outcome:         outcome,
		UndecidedReason: result.UndecidedReason().String(),
		AskedAt:         rfc3339(result.AskedAt()),
		Items:           []deliveryDispatchReaskItem{},
		Remaining:       result.Remaining(),
	}
	if dueBy, cadence, has := result.DueBy(); has {
		body.CadenceVersion = cadence.Cadence.Version().String()
		body.IntervalMinutes = int64(cadence.Cadence.Interval().Minutes())
		body.DueBy = rfc3339(dueBy)
	}
	for _, item := range result.Items() {
		answer := item.Answer
		entry := deliveryDispatchReaskItem{
			Segment:         item.Segment.String(),
			Object:          item.Object.String(),
			Outcome:         answer.Outcome().String(),
			Refusal:         answer.Refusal().String(),
			UndecidedReason: answer.UndecidedReason().String(),
		}
		for _, requirement := range answer.Missing() {
			entry.Missing = append(entry.Missing, requirement.String())
		}
		if record, present := answer.Record(); present {
			entry.Task = record.Task.Task().String()
		}
		body.Items = append(body.Items, entry)
	}
	// ADR-0022：重问是形成了的答案，各件新形成与否在逐件里说，整口一律 200。
	writeJSON(response, http.StatusOK, body)
}
