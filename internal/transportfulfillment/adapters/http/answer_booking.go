package tfhttp

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 承运订舱答复口（UC-TF-004 步 4；ADR-0202 决定二、五）。承运系统对一次订舱作答，挂集成客户端族、按承运订舱答复这一类
// 事实认证。它是承运方回传口，路径取控制事实那组的 `/transport-fulfillment/...` 而不是运营决定的 `transport-fulfillment-`
// 前缀——读路径就分得出这一口的提交方在产品之外。

// BookingAnswerIntake 把已认证的承运答复请求翻译成答复命令。
type BookingAnswerIntake interface {
	IntakeBookingAnswer(ctx context.Context, request *http.Request) (application.AnswerBookingCommand, error)
}

// BookingAnswerer 是本适配器转交的应用编排。
type BookingAnswerer interface {
	AnswerBooking(ctx context.Context, command application.AnswerBookingCommand) (application.CommissionResult, error)
}

// BookingAnswerPayload 是承运答复口的线格式。载荷里没有租户也没有来源：两者都取自集成客户端绑定，写了即拒（封闭解码）。
// `answer` 是承运方给这份答复的标识，同一份答复重发要带同一个；`outcome` 取五值之一；`quantity` 只随接受给，是接受量；
// `basis` 随其余四值给，是原因或候补的依据。答复时刻照 ADR-0023 由提交方给。
type BookingAnswerPayload struct {
	Booking   string `json:"booking"`
	Answer    string `json:"answer"`
	Outcome   string `json:"outcome"`
	Quantity  int64  `json:"quantity"`
	Basis     string `json:"basis"`
	DecidedAt string `json:"decidedAt"`
}

// Command 把载荷连同认证出的身份翻成答复命令。答复值不在五值之内、时刻解不出是坏报文（400）；各格成不成立、答复方是不是
// 服务提供方由编排答。
func (payload BookingAnswerPayload) Command(identity CarrierAnswerIdentity) (application.AnswerBookingCommand, error) {
	if identity.Tenant.String() == "" || identity.Source == "" {
		return application.AnswerBookingCommand{}, errCarrierAnswerIdentityMissing
	}
	outcome, err := parseCarrierAnswerOutcome(payload.Outcome)
	if err != nil {
		return application.AnswerBookingCommand{}, err
	}
	decidedAt, err := parseOptionalInstant("decidedAt", payload.DecidedAt)
	if err != nil {
		return application.AnswerBookingCommand{}, err
	}
	return application.AnswerBookingCommand{
		TenantID:   identity.Tenant,
		Source:     identity.Source,
		Booking:    payload.Booking,
		Acceptance: payload.Answer,
		Outcome:    outcome,
		Quantity:   payload.Quantity,
		Basis:      payload.Basis,
		DecidedAt:  decidedAt,
	}, nil
}

// errCarrierAnswerIdentityMissing 表示调用方没交来租户或来源身份：不是载荷的错，是 Intake 没拿到集成客户端信封就来翻译。
var errCarrierAnswerIdentityMissing = errors.New("transport fulfillment http: carrier answer identity (tenant and source) is required")

func parseCarrierAnswerOutcome(raw string) (domain.CarrierAcceptanceOutcome, error) {
	for _, outcome := range []domain.CarrierAcceptanceOutcome{
		domain.BookingAccepted, domain.BookingRefused, domain.BookingExpired, domain.BookingWithdrawn, domain.BookingWaitlisted,
	} {
		if outcome.String() == raw {
			return outcome, nil
		}
	}
	return domain.CarrierAcceptanceOutcomeInvalid, fmt.Errorf("%w: outcome=%q is not a carrier answer word", ErrMalformedRequest, raw)
}

// NewAnswerBookingEndpoint 交回承运答复的 HTTP 入口。
func NewAnswerBookingEndpoint(intake BookingAnswerIntake, answerer BookingAnswerer) http.Handler {
	return commandEndpoint(func(request *http.Request) (application.CommissionResult, error, bool) {
		command, err := intake.IntakeBookingAnswer(request.Context(), request)
		if err != nil {
			return application.CommissionResult{}, err, false
		}
		result, err := answerer.AnswerBooking(request.Context(), command)
		return result, err, true
	}, writeBookingAnswerOutcome)
}

// bookingAnswerResponse 是承运答复口的封闭响应形状。订舱不在册与答复方不是服务提供方各自成格：前者续办是核订舱标识或
// 等运营侧录入订舱，后者是这份答复不归它答。冲突不回显在册的答复——在册那一份可能出自另一次出示，回显就成了读口。
type bookingAnswerResponse struct {
	Outcome               string             `json:"outcome"`
	UndecidedReason       string             `json:"undecidedReason,omitempty"`
	ContinuationReference string             `json:"continuationReference,omitempty"`
	Answer                *bookingAnswerBody `json:"answer,omitempty"`
}

// bookingAnswerBody 逐字段透出在册那一份答复；重放回显的是在册那一份。时间一律 RFC 3339 UTC。
type bookingAnswerBody struct {
	Booking    string `json:"booking"`
	Answer     string `json:"answer"`
	Outcome    string `json:"outcome"`
	Quantity   int64  `json:"quantity,omitempty"`
	Basis      string `json:"basis,omitempty"`
	DecidedAt  string `json:"decidedAt"`
	RecordedAt string `json:"recordedAt"`
}

func writeBookingAnswerOutcome(response http.ResponseWriter, result application.CommissionResult) {
	outcome := result.Outcome().String()
	if outcome == "" {
		writeProblem(response, http.StatusInternalServerError, codeUnnamedOutcome)
		return
	}
	body := bookingAnswerResponse{
		Outcome:               outcome,
		UndecidedReason:       result.UndecidedReason().String(),
		ContinuationReference: result.ContinuationReference(),
	}
	if record, present := result.Answer(); present {
		acceptance := record.Acceptance
		body.Answer = &bookingAnswerBody{
			Booking:    acceptance.Booking().String(),
			Answer:     acceptance.Acceptance().String(),
			Outcome:    acceptance.Outcome().String(),
			Quantity:   acceptance.AcceptedQuantity(),
			DecidedAt:  rfc3339(acceptance.DecidedAt()),
			RecordedAt: rfc3339(record.RecordedAt),
		}
		if basis, present := acceptance.Basis(); present {
			body.Answer.Basis = basis.String()
		}
	}
	// ADR-0022：只有新登了一份答复用 201；已有答复、冲突、订舱不在册、来源不是服务方、未受理、未决都是形成了的答案（200）。
	status := http.StatusOK
	if result.Outcome() == application.BookingAnswered {
		status = http.StatusCreated
	}
	writeJSON(response, status, body)
}
