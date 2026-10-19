package tfhttp

import (
	"context"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 订舱提交口（UC-TF-004 步 1、3；ADR-0202 决定一、五）。运营人员在一份已提交的委托下，就一个在册的具体班次向服务提供方
// 申请舱位，是管理台上作出的生产决定，挂操作者渠道的「运营决定」能力面、按订舱提交这一种决定认证；路径取读面册名前缀
// `transport-fulfillment-`，理由同委托提交口。

// BookingRequestSubmissionIntake 把已认证的运营写请求翻译成订舱提交命令。
type BookingRequestSubmissionIntake interface {
	IntakeBookingRequestSubmission(ctx context.Context, request *http.Request) (application.SubmitBookingCommand, error)
}

// BookingRequestSubmitter 是本适配器转交的应用编排。
type BookingRequestSubmitter interface {
	SubmitBooking(ctx context.Context, command application.SubmitBookingCommand) (application.CommissionResult, error)
}

// BookingRequestSubmissionPayload 是订舱提交口的线格式。载荷只有内容、没有身份：租户从信封来，载荷带租户即拒（封闭解码）。
// 班次是具体班次的标识（物化来的写作「周期安排代码/运行日期」），不收周期安排——周期安排不能被订舱直接引用。申请时刻照
// ADR-0023 由提交方给。
type BookingRequestSubmissionPayload struct {
	Booking     string `json:"booking"`
	Commission  string `json:"commission"`
	Schedule    string `json:"schedule"`
	Quantity    int64  `json:"quantity"`
	Unit        string `json:"unit"`
	RequestedAt string `json:"requestedAt"`
}

// Command 把载荷连同信封给的租户翻成订舱提交命令。时刻解不出是坏报文（400）；各格成不成立、委托与班次在不在册由编排答。
func (payload BookingRequestSubmissionPayload) Command(tenant domain.TenantID) (application.SubmitBookingCommand, error) {
	if tenant.String() == "" {
		return application.SubmitBookingCommand{}, ErrOperatorIdentityMissing
	}
	requestedAt, err := parseOptionalInstant("requestedAt", payload.RequestedAt)
	if err != nil {
		return application.SubmitBookingCommand{}, err
	}
	return application.SubmitBookingCommand{
		TenantID:    tenant,
		Booking:     payload.Booking,
		Commission:  payload.Commission,
		Schedule:    payload.Schedule,
		Quantity:    payload.Quantity,
		Unit:        payload.Unit,
		RequestedAt: requestedAt,
	}, nil
}

// NewSubmitBookingRequestEndpoint 交回订舱提交的 HTTP 入口。
func NewSubmitBookingRequestEndpoint(intake BookingRequestSubmissionIntake, submitter BookingRequestSubmitter) http.Handler {
	return commandEndpoint(func(request *http.Request) (application.CommissionResult, error, bool) {
		command, err := intake.IntakeBookingRequestSubmission(request.Context(), request)
		if err != nil {
			return application.CommissionResult{}, err, false
		}
		result, err := submitter.SubmitBooking(request.Context(), command)
		return result, err, true
	}, writeBookingRequestSubmissionOutcome)
}

// bookingRequestSubmissionResponse 是订舱提交口的封闭响应形状。委托不在册、委托已取消、班次不在册各自成格，续办两两不同
// （去提交委托或改引用 / 另起委托 / 去登记班次或改引在册班次），传输层不合并。订舱只是申请：答复里没有承运方的任何结果，
// 有没有被接受看承运答复口。
type bookingRequestSubmissionResponse struct {
	Outcome               string              `json:"outcome"`
	UndecidedReason       string              `json:"undecidedReason,omitempty"`
	ContinuationReference string              `json:"continuationReference,omitempty"`
	Booking               *bookingRequestBody `json:"booking,omitempty"`
}

// bookingRequestBody 逐字段透出在册那一份订舱；重放回显的是在册那一份，不是这次送来的。时间一律 RFC 3339 UTC。
type bookingRequestBody struct {
	Booking     string `json:"booking"`
	Commission  string `json:"commission"`
	Schedule    string `json:"schedule"`
	Quantity    int64  `json:"quantity"`
	Unit        string `json:"unit"`
	RequestedAt string `json:"requestedAt"`
	CancelledAt string `json:"cancelledAt,omitempty"`
	RecordedAt  string `json:"recordedAt"`
}

func writeBookingRequestSubmissionOutcome(response http.ResponseWriter, result application.CommissionResult) {
	outcome := result.Outcome().String()
	if outcome == "" {
		writeProblem(response, http.StatusInternalServerError, codeUnnamedOutcome)
		return
	}
	body := bookingRequestSubmissionResponse{
		Outcome:               outcome,
		UndecidedReason:       result.UndecidedReason().String(),
		ContinuationReference: result.ContinuationReference(),
	}
	if record, present := result.Booking(); present {
		booking := record.Booking
		quantity, unit := booking.Quantity()
		body.Booking = &bookingRequestBody{
			Booking:     booking.Booking().String(),
			Commission:  booking.Commission().String(),
			Schedule:    booking.Schedule().String(),
			Quantity:    quantity,
			Unit:        unit.String(),
			RequestedAt: rfc3339(booking.RequestedAt()),
			RecordedAt:  rfc3339(record.RecordedAt),
		}
		if cancelledAt, cancelled := booking.Cancelled(); cancelled {
			body.Booking.CancelledAt = rfc3339(cancelledAt)
		}
	}
	// ADR-0022：只有新提交了一次订舱用 201；已有订舱、冲突、引用不在册各格、未受理、未决都是形成了的答案（200）。
	status := http.StatusOK
	if result.Outcome() == application.BookingSubmitted {
		status = http.StatusCreated
	}
	writeJSON(response, status, body)
}
