package tfhttp

import (
	"context"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 取消订舱口（ADR-0204 决定三；ADR-0202 决定一）。运营人员停掉一次尚未消耗的订舱，是管理台上作出的生产决定，挂操作者
// 渠道的「运营决定」能力面、按取消订舱这一种认证：能订舱的不因此能取消。路径取读面册名前缀 `transport-fulfillment-`，
// 理由同委托提交口。

// BookingRequestCancellationIntake 把已认证的运营写请求翻译成取消订舱命令。
type BookingRequestCancellationIntake interface {
	IntakeBookingRequestCancellation(ctx context.Context, request *http.Request) (application.CancelBookingCommand, error)
}

// BookingRequestCanceller 是本适配器转交的应用编排。
type BookingRequestCanceller interface {
	CancelBooking(ctx context.Context, command application.CancelBookingCommand) (application.CommissionResult, error)
}

// BookingRequestCancellationPayload 是取消订舱口的线格式。载荷只有内容、没有身份（封闭解码）。取消时刻照 ADR-0023 由提交方给。
type BookingRequestCancellationPayload struct {
	Booking     string `json:"booking"`
	CancelledAt string `json:"cancelledAt"`
}

// Command 把载荷连同信封给的租户翻成取消订舱命令。时刻解不出是坏报文（400）；取不取消得了由编排答。
func (payload BookingRequestCancellationPayload) Command(tenant domain.TenantID) (application.CancelBookingCommand, error) {
	if tenant.String() == "" {
		return application.CancelBookingCommand{}, ErrOperatorIdentityMissing
	}
	cancelledAt, err := parseOptionalInstant("cancelledAt", payload.CancelledAt)
	if err != nil {
		return application.CancelBookingCommand{}, err
	}
	return application.CancelBookingCommand{TenantID: tenant, Booking: payload.Booking, CancelledAt: cancelledAt}, nil
}

// NewCancelBookingRequestEndpoint 交回取消订舱的 HTTP 入口。
func NewCancelBookingRequestEndpoint(intake BookingRequestCancellationIntake, canceller BookingRequestCanceller) http.Handler {
	return commandEndpoint(func(request *http.Request) (application.CommissionResult, error, bool) {
		command, err := intake.IntakeBookingRequestCancellation(request.Context(), request)
		if err != nil {
			return application.CommissionResult{}, err, false
		}
		result, err := canceller.CancelBooking(request.Context(), command)
		return result, err, true
	}, writeBookingRequestCancellationOutcome)
}

// bookingRequestCancellationResponse 是取消订舱口的封闭响应形状。`TRANSPORT_ALREADY_STARTED` 带回在册那一份订舱（未取消）：
// 它在所属委托下的运输开始之前就已提出，可能正被那一程消耗；已取消的重放回显原取消。
type bookingRequestCancellationResponse struct {
	Outcome               string              `json:"outcome"`
	UndecidedReason       string              `json:"undecidedReason,omitempty"`
	ContinuationReference string              `json:"continuationReference,omitempty"`
	Booking               *bookingRequestBody `json:"booking,omitempty"`
}

func writeBookingRequestCancellationOutcome(response http.ResponseWriter, result application.CommissionResult) {
	outcome := result.Outcome().String()
	if outcome == "" {
		writeProblem(response, http.StatusInternalServerError, codeUnnamedOutcome)
		return
	}
	body := bookingRequestCancellationResponse{
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
	// ADR-0022：只有这次真把订舱取消了用 201；已开始、已取消的重放、不在册、未受理、未决都是形成了的答案（200）。
	status := http.StatusOK
	if result.Outcome() == application.BookingCancelled {
		status = http.StatusCreated
	}
	writeJSON(response, status, body)
}
