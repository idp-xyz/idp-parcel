package tfhttp

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
)

// 本文件是承运订舱答复的手工录入口（dox/decisions.md「产品形态」）：没有对接承运商系统的租户，承运方用电话或邮件答复
// 订舱，操作员照着录进来。进的是承运答复口后面的同一只编排——一次订舱至多一份定局、候补之后可接定局、同一答复标识
// 同内容是重放异内容是冲突，一条不少；差别只在答复方不由出示者的绑定给，而取订舱所属委托的服务提供方。

// OperatorDecisionBookingAnswerEntry 是人工录入承运方对订舱的答复。
const OperatorDecisionBookingAnswerEntry OperatorDecision = "BOOKING_ANSWER_ENTRY"

// ManualBookingAnswerPrefix 是人工录入的答复在答复标识上的前缀：操作员给的标识与承运系统给的不在一个名字空间里，
// 同号的两份不会被读成同一份的重放。它不改变「一次订舱至多一份定局」：人先录了定局，承运系统随后交来的那一份照样是
// 冲突、不落——同时走两条路的租户要知道，先落的那一份算数。承运答复口拒收带这个前缀的答复标识。
const ManualBookingAnswerPrefix = "BKGMANUAL/"

// ManualBookingAnswerPayload 是手工录入口的线格式：与承运答复口同一份形状（booking、answer、outcome、quantity、basis、
// decidedAt），另必带 original——所凭原件（邮件、通话记录）的编号或存档引用。decidedAt 是承运方答复的时刻，不得晚于
// 现在。
type ManualBookingAnswerPayload struct {
	Booking   string `json:"booking"`
	Answer    string `json:"answer"`
	Outcome   string `json:"outcome"`
	Quantity  int64  `json:"quantity"`
	Basis     string `json:"basis"`
	DecidedAt string `json:"decidedAt"`
	Original  string `json:"original"`
}

// ManualBookingAnswerIntake 是手工录入口的 Intake。
type ManualBookingAnswerIntake interface {
	IntakeManualBookingAnswer(ctx context.Context, request *http.Request, now time.Time) (application.AnswerBookingCommand, error)
}

var _ ManualBookingAnswerIntake = (*OperatorDecisionIntake)(nil)

// IntakeManualBookingAnswer 按人工录入订舱答复这一种决定认证。租户与录入人只取认证结果；答复方不在命令里。
func (intake *OperatorDecisionIntake) IntakeManualBookingAnswer(ctx context.Context, request *http.Request, now time.Time) (application.AnswerBookingCommand, error) {
	none := application.AnswerBookingCommand{}
	operator, err := intake.identityFor(ctx, request, OperatorDecisionBookingAnswerEntry)
	if err != nil {
		return none, err
	}
	var payload ManualBookingAnswerPayload
	if err := decodeClosedPayload(request.Body, &payload); err != nil {
		return none, err
	}
	original, answer := strings.TrimSpace(payload.Original), strings.TrimSpace(payload.Answer)
	hasControl := func(text string) bool { return strings.IndexFunc(text, unicode.IsControl) >= 0 }
	if original == "" || len(original) > 500 || hasControl(original) || answer == "" || len(answer) > 200 || hasControl(answer) {
		return none, fmt.Errorf("%w: answer and original are required for a manually entered booking answer", ErrMalformedRequest)
	}
	outcome, err := parseCarrierAnswerOutcome(payload.Outcome)
	if err != nil {
		return none, err
	}
	decidedAt, err := parseOptionalInstant("decidedAt", payload.DecidedAt)
	if err != nil {
		return none, err
	}
	if decidedAt.After(now) {
		return none, fmt.Errorf("%w: decidedAt is in the future", ErrMalformedRequest)
	}
	return application.AnswerBookingCommand{
		TenantID:   operator.Tenant,
		Booking:    payload.Booking,
		Acceptance: ManualBookingAnswerPrefix + answer,
		Outcome:    outcome,
		Quantity:   payload.Quantity,
		Basis:      payload.Basis,
		DecidedAt:  decidedAt,
		EnteredBy:  "OPERATOR/" + operator.Operator,
		Original:   original,
	}, nil
}

// NewEnterBookingAnswerEndpoint 交回手工录入口（POST /transport-fulfillment-booking-answer-manual-entries）。答复与承运
// 答复口同一份形状。
func NewEnterBookingAnswerEndpoint(intake ManualBookingAnswerIntake, answerer BookingAnswerer, now func() time.Time) http.Handler {
	return commandEndpoint(func(request *http.Request) (application.CommissionResult, error, bool) {
		command, err := intake.IntakeManualBookingAnswer(request.Context(), request, now())
		if err != nil {
			return application.CommissionResult{}, err, false
		}
		result, err := answerer.AnswerBooking(request.Context(), command)
		return result, err, true
	}, writeBookingAnswerOutcome)
}
