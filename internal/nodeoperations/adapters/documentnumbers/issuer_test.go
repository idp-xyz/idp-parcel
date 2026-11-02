package documentnumbers_test

import (
	"context"
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/documentnumbers"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
	"go.idp.xyz/idp-parcel/internal/platform/docnumber"
)

type assignerDouble struct {
	assignment docnumber.Assignment
	err        error
	got        docnumber.IssueRequest
}

func (double *assignerDouble) Assign(_ context.Context, request docnumber.IssueRequest) (docnumber.Assignment, error) {
	double.got = request
	return double.assignment, double.err
}

// Covers: 取号编排的各格答案逐一译回本上下文的词，请求原样交过去（租户、类型、开立网点的业务时区、判撞那一问）；编排的错误原样交回；
// 编排答出词表之外的格是实现坏了，交回错误而不是猜一格。
func TestTheIssuerTranslatesTheAssignersAnswers(t *testing.T) {
	tenant, err := domain.NewTenantID("SYN-TENANT-01")
	if err != nil {
		t.Fatal(err)
	}
	request := ports.DocumentNumberRequest{
		Tenant: tenant, Type: domain.DeliveryPreAdviceDocumentNumberType.Code, IssuerTimeZone: "Asia/Shanghai",
		IsUsed: func(context.Context, string) (bool, error) { return false, nil },
	}
	for name, testCase := range map[string]struct {
		assignment docnumber.Assignment
		want       ports.DocumentNumberIssueOutcome
		number     string
	}{
		"取到号":       {docnumber.Assignment{Outcome: docnumber.NumberAssigned, Number: "PA-20261102-00045"}, ports.DocumentNumberIssued, "PA-20261102-00045"},
		"单据编号时区未配置": {docnumber.Assignment{Outcome: docnumber.NumberingTimeZoneNotConfigured}, ports.DocumentNumberTimeZoneNotConfigured, ""},
		"撞号":        {docnumber.Assignment{Outcome: docnumber.NumberingRuleCollision, Skipped: docnumber.MaxSkippedNumbers}, ports.DocumentNumberRuleCollision, ""},
	} {
		double := &assignerDouble{assignment: testCase.assignment}
		issuer, err := documentnumbers.NewIssuer(double)
		if err != nil {
			t.Fatal(err)
		}
		issued, err := issuer.IssueDocumentNumber(context.Background(), request)
		if err != nil || issued.Outcome != testCase.want || issued.Number.String() != testCase.number {
			t.Fatalf("%s：%+v %v", name, issued, err)
		}
		if double.got.Tenant != "SYN-TENANT-01" || double.got.Type != "DELIVERY_PRE_ADVICE" || double.got.IssuerTimeZone != "Asia/Shanghai" || double.got.IsUsed == nil {
			t.Fatalf("%s：交给编排的请求 = %+v", name, double.got)
		}
	}

	broken := errors.New("transaction required")
	issuer, _ := documentnumbers.NewIssuer(&assignerDouble{err: broken})
	if _, err := issuer.IssueDocumentNumber(context.Background(), request); !errors.Is(err, broken) {
		t.Fatalf("编排的错误：%v", err)
	}
	issuer, _ = documentnumbers.NewIssuer(&assignerDouble{assignment: docnumber.Assignment{Outcome: "SOMETHING_ELSE"}})
	if _, err := issuer.IssueDocumentNumber(context.Background(), request); err == nil {
		t.Fatal("词表之外的答案被译成了一格")
	}
	if _, err := documentnumbers.NewIssuer(nil); err == nil {
		t.Fatal("没有编排也构造成了")
	}
}
