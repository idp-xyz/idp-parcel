package tfhttp_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
)

type capturingBookingAnswerer struct {
	commands []application.AnswerBookingCommand
}

func (fake *capturingBookingAnswerer) AnswerBooking(_ context.Context, command application.AnswerBookingCommand) (application.CommissionResult, error) {
	fake.commands = append(fake.commands, command)
	return application.CommissionResult{}, nil
}

// Covers: dox/decisions.md「产品形态」——订舱答复的手工录入口按它自己那一种决定认证；租户与录入人只取认证结果；答复
// 标识加 BKGMANUAL/ 前缀；命令不带答复方（由编排从订舱上取）；原件必填，答复时刻不得晚于现在；自报答复方或录入人、
// 答复值不在五值之内都是坏报文，编排一次也不被调到。
func TestTheManualBookingAnswerFaceAuthenticatesItsOwnKindAndNeverNamesTheAnsweringParty(t *testing.T) {
	var asked []tfhttp.OperatorDecision
	intake, err := tfhttp.NewOperatorDecisionIntake(authenticatorFake{asked: &asked})
	if err != nil {
		t.Fatal(err)
	}
	answerer := &capturingBookingAnswerer{}
	now := time.Date(2026, 11, 13, 9, 0, 0, 0, time.UTC)
	endpoint := tfhttp.NewEnterBookingAnswerEndpoint(intake, answerer, func() time.Time { return now })

	postTracking(endpoint, "t", `{"booking":"SYN-BOOKING-1","answer":" mail-1 ","outcome":"ACCEPTED","quantity":120,"decidedAt":"2026-11-13T08:00:00Z","original":" SYN-MAIL-1 "}`)
	if len(answerer.commands) != 1 || len(asked) != 1 || asked[0] != "BOOKING_ANSWER_ENTRY" {
		t.Fatalf("calls = %d, asked = %v", len(answerer.commands), asked)
	}
	command := answerer.commands[0]
	if command.TenantID.String() != operatorTenant || command.Source != "" || command.EnteredBy != "OPERATOR/"+operatorSubject ||
		command.Original != "SYN-MAIL-1" || command.Acceptance != "BKGMANUAL/mail-1" || command.Booking != "SYN-BOOKING-1" || command.Quantity != 120 {
		t.Fatalf("command = %+v", command)
	}
	for name, body := range map[string]string{
		"没有原件":    `{"booking":"B","answer":"A","outcome":"ACCEPTED","quantity":1}`,
		"没有答复标识":  `{"booking":"B","outcome":"ACCEPTED","quantity":1,"original":"O"}`,
		"答复时刻在未来": `{"booking":"B","answer":"A","outcome":"ACCEPTED","quantity":1,"original":"O","decidedAt":"2026-11-13T09:00:01Z"}`,
		"自报答复方":   `{"booking":"B","answer":"A","outcome":"ACCEPTED","quantity":1,"original":"O","source":"partner-1"}`,
		"自报录入人":   `{"booking":"B","answer":"A","outcome":"ACCEPTED","quantity":1,"original":"O","enteredBy":"x"}`,
		"答复值不在五值": `{"booking":"B","answer":"A","outcome":"MAYBE","original":"O"}`,
	} {
		if response := postTracking(endpoint, "t", body); response.Code != http.StatusBadRequest {
			t.Fatalf("%s = %d %s，想要 400", name, response.Code, response.Body.String())
		}
	}
	if response := postTracking(endpoint, "", `{"booking":"B","answer":"A","outcome":"ACCEPTED","quantity":1,"original":"O"}`); response.Code != http.StatusUnauthorized {
		t.Fatalf("没有凭据 = %d，想要 401", response.Code)
	}
	if len(answerer.commands) != 1 {
		t.Fatalf("坏报文到了编排：%d 次", len(answerer.commands))
	}
}
