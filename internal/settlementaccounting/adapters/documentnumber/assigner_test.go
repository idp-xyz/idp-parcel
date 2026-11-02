package documentnumber_test

import (
	"context"
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/platform/docnumber"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/documentnumber"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

type issuerDouble struct {
	answer   docnumber.Assignment
	err      error
	requests []docnumber.IssueRequest
}

func (double *issuerDouble) Assign(_ context.Context, request docnumber.IssueRequest) (docnumber.Assignment, error) {
	double.requests = append(double.requests, request)
	return double.answer, double.err
}

// Covers: ADR-0025 全函数翻译——编号组件的三格答案逐格落到端口的三格；请求的租户、类型、开立方业务时区与「是否已用」原样交过去；
// 编号组件的错误原样上抛（调用方据此回滚开立事务）；多出来的一格不悄悄落到哪一格上。
func TestTheAssignerTranslatesEveryAssignmentOutcome(t *testing.T) {
	tenant, err := domain.NewTenantID("SYN-TENANT-01")
	if err != nil {
		t.Fatal(err)
	}
	used := func(context.Context, string) (bool, error) { return false, nil }
	request := ports.DocumentNumberRequest{
		Tenant: tenant, Type: domain.StatementDisputeDocumentNumberType.Code, IssuerTimeZone: "Asia/Shanghai", IsUsed: used,
	}
	for name, tc := range map[string]struct {
		answer docnumber.Assignment
		want   ports.DocumentNumberAssignment
	}{
		"assigned": {
			docnumber.Assignment{Outcome: docnumber.NumberAssigned, Number: "DSP-202611-00001"},
			ports.DocumentNumberAssignment{Outcome: ports.DocumentNumberAssigned, Number: "DSP-202611-00001"},
		},
		"time zone unconfigured": {
			docnumber.Assignment{Outcome: docnumber.NumberingTimeZoneNotConfigured},
			ports.DocumentNumberAssignment{Outcome: ports.DocumentNumberTimeZoneUnconfigured},
		},
		"collision": {
			docnumber.Assignment{Outcome: docnumber.NumberingRuleCollision, Skipped: docnumber.MaxSkippedNumbers},
			ports.DocumentNumberAssignment{Outcome: ports.DocumentNumberRuleCollision},
		},
	} {
		issuer := &issuerDouble{answer: tc.answer}
		assigner, err := documentnumber.NewAssigner(issuer)
		if err != nil {
			t.Fatal(err)
		}
		got, err := assigner.AssignDocumentNumber(t.Context(), request)
		if err != nil || got != tc.want {
			t.Fatalf("%s: %+v %v", name, got, err)
		}
		sent := issuer.requests[0]
		if sent.Tenant != "SYN-TENANT-01" || sent.Type != "STATEMENT_DISPUTE" || sent.IssuerTimeZone != "Asia/Shanghai" || sent.IsUsed == nil {
			t.Fatalf("%s: 交给编号组件的请求 = %+v", name, sent)
		}
	}

	failing, _ := documentnumber.NewAssigner(&issuerDouble{err: docnumber.ErrTransactionRequired})
	if _, err := failing.AssignDocumentNumber(t.Context(), request); !errors.Is(err, docnumber.ErrTransactionRequired) {
		t.Fatalf("错误没原样上抛：%v", err)
	}
	unknown, _ := documentnumber.NewAssigner(&issuerDouble{answer: docnumber.Assignment{Outcome: "SOMETHING_NEW"}})
	if got, err := unknown.AssignDocumentNumber(t.Context(), request); err == nil {
		t.Fatalf("多出来的一格被落到了 %+v", got)
	}
	if _, err := documentnumber.NewAssigner(nil); err == nil {
		t.Fatal("没有编号组件也装了起来")
	}
}
