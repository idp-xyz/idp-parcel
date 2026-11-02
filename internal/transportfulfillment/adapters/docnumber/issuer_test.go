package docnumber_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	platform "go.idp.xyz/idp-parcel/internal/platform/docnumber"
	tfdocnumber "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/docnumber"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// assignerStub 记下收到的请求，按 rendered 逐个问调用方「是否已用」，再交回 answer。
type assignerStub struct {
	answer   platform.Assignment
	err      error
	rendered []string
	got      platform.IssueRequest
}

func (stub *assignerStub) Assign(ctx context.Context, request platform.IssueRequest) (platform.Assignment, error) {
	stub.got = request
	for _, number := range stub.rendered {
		if _, err := request.IsUsed(ctx, number); err != nil {
			return platform.Assignment{}, err
		}
	}
	return stub.answer, stub.err
}

func request(t *testing.T, asked *[]string) ports.DocumentNumberRequest {
	t.Helper()
	tenant, _ := domain.NewTenantID("SYN-TENANT-01")
	return ports.DocumentNumberRequest{
		TenantID: tenant,
		Type:     domain.BookingDocumentNumberType.Code,
		IsUsed: func(_ context.Context, number domain.DocumentNumber) (bool, error) {
			*asked = append(*asked, number.String())
			return false, nil
		},
	}
}

// Covers: ADR-0355 决定四、七——消费侧适配器只做形状翻译：租户与类型照交、不传开立方业务时区；三种答案逐格译回；「是否已用」
// 原样转给本上下文的表；渲染出的号立不成单据号时交错误而不说成已用。
func TestTheIssuerTranslatesTheThreeAssignmentOutcomes(t *testing.T) {
	var asked []string
	stub := &assignerStub{answer: platform.Assignment{Outcome: platform.NumberAssigned, Number: "BK-202611-00009"}, rendered: []string{"BK-202611-00008"}}
	issuer, err := tfdocnumber.NewIssuer(stub)
	if err != nil {
		t.Fatal(err)
	}
	issue, err := issuer.IssueDocumentNumber(context.Background(), request(t, &asked))
	if err != nil || issue.Outcome != ports.DocumentNumberIssued || issue.Number.String() != "BK-202611-00009" {
		t.Fatalf("取到号：%+v %v", issue, err)
	}
	if stub.got.Tenant != "SYN-TENANT-01" || stub.got.Type != "BOOKING" || stub.got.IssuerTimeZone != "" {
		t.Fatalf("交给编号组件的请求走样：%+v", stub.got)
	}
	if len(asked) != 1 || asked[0] != "BK-202611-00008" {
		t.Fatalf("是否已用没有转给本上下文：%v", asked)
	}

	for outcome, want := range map[platform.AssignmentOutcome]ports.DocumentNumberIssueOutcome{
		platform.NumberingTimeZoneNotConfigured: ports.DocumentNumberTimeZoneNotConfigured,
		platform.NumberingRuleCollision:         ports.DocumentNumberRuleCollision,
	} {
		issuer, _ := tfdocnumber.NewIssuer(&assignerStub{answer: platform.Assignment{Outcome: outcome}})
		issue, err := issuer.IssueDocumentNumber(context.Background(), request(t, &asked))
		if err != nil || issue.Outcome != want {
			t.Fatalf("%s 译成 %s %v", outcome, issue.Outcome, err)
		}
	}

	for name, stub := range map[string]*assignerStub{
		"编号组件交错误":  {err: errors.New("sequence down")},
		"认不出的答案":   {answer: platform.Assignment{Outcome: "SOMETHING_ELSE"}},
		"取到号却没有号":  {answer: platform.Assignment{Outcome: platform.NumberAssigned}},
		"渲染出存不下的号": {rendered: []string{strings.Repeat("9", 600)}, answer: platform.Assignment{Outcome: platform.NumberAssigned, Number: "BK-1"}},
	} {
		issuer, _ := tfdocnumber.NewIssuer(stub)
		if _, err := issuer.IssueDocumentNumber(context.Background(), request(t, &asked)); err == nil {
			t.Fatalf("%s 应交错误", name)
		}
	}
	if _, err := tfdocnumber.NewIssuer(nil); err == nil {
		t.Fatal("没有编号组件不该装得起来")
	}
}
