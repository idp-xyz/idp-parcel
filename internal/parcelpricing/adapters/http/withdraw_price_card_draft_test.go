package pricinghttp_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	pricinghttp "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/http"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/application"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// 撤下口把信封译成命令：撤下人是认证出的操作者，不是载荷里的自报；原因原样交进命令，空不空由用例答。

func TestTheWithdrawalIntakeTakesTheWithdrawerFromTheEnvelope(t *testing.T) {
	intake, err := pricinghttp.NewOperatorRegistryIntake(grantedAuthenticator{})
	if err != nil {
		t.Fatal(err)
	}
	body := `{"planId": "SYN-PLAN-DRAFT-01", "planVersion": "v1", "reason": "  客户拒绝这套价  "}`
	command, err := intake.IntakePriceCardDraftWithdrawal(context.Background(), progressRequest("/pricing-price-card-draft-withdrawals", "t", body))
	if err != nil {
		t.Fatalf("withdrawal intake: %v", err)
	}
	if command.Tenant.String() != "SYN-TENANT-01" || command.Withdrawer.Reference() != "https://id.syn.example/dex#SYN-OPERATOR-02" ||
		command.Reason != "  客户拒绝这套价  " || command.Plan.ID() != "SYN-PLAN-DRAFT-01" || command.Plan.Version() != "v1" {
		t.Fatalf("command = %+v", command)
	}
	for name, raw := range map[string]string{
		"自报撤下人": `{"planId": "SYN-PLAN-DRAFT-01", "planVersion": "v1", "reason": "客户拒绝", "withdrawer": "someone-else"}`,
		"自报租户":  `{"planId": "SYN-PLAN-DRAFT-01", "planVersion": "v1", "reason": "客户拒绝", "tenant": "SYN-TENANT-02"}`,
		"自报时刻":  `{"planId": "SYN-PLAN-DRAFT-01", "planVersion": "v1", "reason": "客户拒绝", "withdrawnAt": "2026-10-10T05:00:00Z"}`,
		"缺版本":   `{"planId": "SYN-PLAN-DRAFT-01", "reason": "客户拒绝"}`,
	} {
		if _, err := intake.IntakePriceCardDraftWithdrawal(context.Background(), progressRequest("/x", "t", raw)); !errors.Is(err, pricinghttp.ErrMalformedRequest) {
			t.Fatalf("%s: err = %v, want ErrMalformedRequest", name, err)
		}
	}
}

type withdrawalIntakeDouble struct{}

func (withdrawalIntakeDouble) IntakePriceCardDraftWithdrawal(context.Context, *http.Request) (application.WithdrawPriceCardDraftCommand, error) {
	return application.WithdrawPriceCardDraftCommand{}, nil
}

type withdrawerDouble struct {
	result application.WithdrawPriceCardDraftResult
	err    error
}

func (double withdrawerDouble) Handle(context.Context, application.WithdrawPriceCardDraftCommand) (application.WithdrawPriceCardDraftResult, error) {
	return double.result, double.err
}

func TestTheWithdrawalEndpointAnswersEveryCell(t *testing.T) {
	approved := approvedDraftForAnswer(t)
	withdrawer, err := domain.NewOperatorSubject("https://id.syn.example/dex#SYN-OPERATOR-03", nil)
	if err != nil {
		t.Fatal(err)
	}
	withdrawn, err := approved.Withdraw(withdrawer, "客户拒绝这套价", time.Date(2026, 10, 10, 5, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	status, answer := serve(t, pricinghttp.NewWithdrawPriceCardDraftEndpoint(withdrawalIntakeDouble{}, withdrawerDouble{
		result: application.WithdrawPriceCardDraftResult{Outcome: application.PriceCardDraftWithdrawn, Draft: withdrawn, HasDraft: true},
	}), progressRequest("/pricing-price-card-draft-withdrawals", "t", draftReference))
	if status != http.StatusOK || answer.Outcome != "DRAFT_WITHDRAWN" || answer.Draft == nil || answer.Draft.Status != "WITHDRAWN" {
		t.Fatalf("withdrawn: %d %+v", status, answer)
	}
	recorder := httptest.NewRecorder()
	pricinghttp.NewWithdrawPriceCardDraftEndpoint(withdrawalIntakeDouble{}, withdrawerDouble{
		result: application.WithdrawPriceCardDraftResult{Outcome: application.PriceCardDraftWithdrawn, Draft: withdrawn, HasDraft: true},
	}).ServeHTTP(recorder, progressRequest("/pricing-price-card-draft-withdrawals", "t", draftReference))
	body := recorder.Body.String()
	for _, part := range []string{
		`"withdrawnBy":"https://id.syn.example/dex#SYN-OPERATOR-03"`,
		`"withdrawalReason":"客户拒绝这套价"`,
		`"withdrawnAt":"2026-10-10T05:00:00Z"`,
	} {
		if !strings.Contains(body, part) {
			t.Fatalf("撤下痕迹缺 %s：%s", part, body)
		}
	}
	for _, outcome := range []application.WithdrawPriceCardDraftOutcome{
		application.PriceCardWithdrawalDraftNotApproved,
		application.PriceCardWithdrawalDraftPublished,
		application.PriceCardWithdrawalNotAccepted,
		application.PriceCardWithdrawalDraftNotFound,
		application.PriceCardWithdrawalDraftChanged,
	} {
		status, answer := serve(t, pricinghttp.NewWithdrawPriceCardDraftEndpoint(withdrawalIntakeDouble{}, withdrawerDouble{
			result: application.WithdrawPriceCardDraftResult{Outcome: outcome},
		}), progressRequest("/x", "t", draftReference))
		if status != http.StatusOK || answer.Outcome != outcome.String() || answer.Draft != nil {
			t.Fatalf("%s: %d %+v", outcome, status, answer)
		}
	}
	if status, answer := serve(t, pricinghttp.NewWithdrawPriceCardDraftEndpoint(withdrawalIntakeDouble{}, withdrawerDouble{err: errors.New("down")}),
		progressRequest("/x", "t", draftReference)); status != http.StatusInternalServerError || answer.Error.Code != "NO_ANSWER_FORMED" {
		t.Fatalf("no answer formed: %d %+v", status, answer)
	}
	if status, answer := serve(t, pricinghttp.NewWithdrawPriceCardDraftEndpoint(pricinghttp.UnconfiguredIntake{}, withdrawerDouble{}),
		progressRequest("/x", "t", draftReference)); status != http.StatusForbidden || answer.Error.Code != "ACCESS_CHANNEL_NOT_CONFIGURED" {
		t.Fatalf("unconfigured: %d %+v", status, answer)
	}
}
