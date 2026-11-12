package postgres_test

import (
	"context"
	"reflect"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// Covers: 票 declared-goods-lines/20、ADR-0379 决定三——委托详情带上当前提交版本最近一次受理判断记下的接受前财务控制结论：已冻结、
// 已占信用、受限（可用余额不足）、明确无控制四种照登记读回，同一版判过两次取最近那次；没判过即零值。读的是复核详情那一份
// （LoadRecordedJudgments 的财务控制一格），两边逐字段相同。列表行不读判断表，不带这一格。
func TestTheDetailCarriesTheRecordedFinancialControlConclusion(t *testing.T) {
	fixture := newDispositionQueueFixture(t)
	ctx := t.Context()
	record := func(requestID string, result domain.FinancialControlResult) {
		t.Helper()
		mustWithinTransaction(t, fixture.transactor, ctx, func(txCtx context.Context) error {
			return fixture.judgments.RecordFinancialControlResult(txCtx,
				mustBuild(t, domain.NewTenantID, "tenant-1"), taskRequestID(t, requestID), taskVersion(t, "version-1"), result)
		})
	}

	cases := []struct {
		requestID string
		outcome   string
		seed      func(requestID string)
	}{
		{"REQ-FC-HELD", "HELD", func(requestID string) {
			// 先受限、后放行：最近一次是冻结成立那一次。
			record(requestID, executedControlResult(t, requestID+"/1", taskAsOfFirst,
				controlItem(t, domain.PrepaidFreezeControlItem, 1, domain.ControlItemRestricted, "AVAILABLE_BALANCE_INSUFFICIENT")))
			record(requestID, financialControlResult(t, domain.FinancialControlHeld, requestID+"/2", "", taskAsOfFirst.Add(time.Hour)))
		}},
		{"REQ-FC-CREDIT", "CREDIT_EXPOSED", func(requestID string) {
			record(requestID, executedControlResult(t, requestID, taskAsOfFirst,
				controlItem(t, domain.CreditCheckControlItem, 1, domain.ControlItemSatisfied, "")))
		}},
		{"REQ-FC-RESTRICTED", "RESTRICTED", func(requestID string) {
			record(requestID, executedControlResult(t, requestID, taskAsOfFirst,
				controlItem(t, domain.PrepaidFreezeControlItem, 1, domain.ControlItemRestricted, "AVAILABLE_BALANCE_INSUFFICIENT")))
		}},
		{"REQ-FC-NONE", "NOT_APPLICABLE", func(requestID string) {
			record(requestID, financialControlResult(t, domain.FinancialControlNotApplicable, "", "CONTRACT-NO-CONTROL-1", taskAsOfFirst))
		}},
		{"REQ-FC-NEVER", "", func(string) {}},
	}
	for index, testCase := range cases {
		seedAdvanced(t, fixture.repository, fixture.transactor, "tenant-1", "customer-1", "fc-"+testCase.requestID, testCase.requestID,
			submittedAtFixture.Add(time.Duration(index)*time.Minute), nil)
		testCase.seed(testCase.requestID)
	}

	scope := viewScope(t, "tenant-1", "customer-1")
	for _, testCase := range cases {
		detail, found, err := fixture.views.FindVisibleByID(ctx, scope, taskRequestID(t, testCase.requestID))
		if err != nil || !found {
			t.Fatalf("%s：found=%v err=%v", testCase.requestID, found, err)
		}
		if got := detail.FinancialControl.Outcome().String(); got != testCase.outcome {
			t.Fatalf("%s：结论 = %q, 想要 %q", testCase.requestID, got, testCase.outcome)
		}
		recorded, err := fixture.judgments.LoadRecordedJudgments(ctx,
			mustBuild(t, domain.NewTenantID, "tenant-1"), taskRequestID(t, testCase.requestID), detail.SubmissionVersionID)
		if err != nil {
			t.Fatalf("%s：读判断：%v", testCase.requestID, err)
		}
		if !reflect.DeepEqual(detail.FinancialControl, recorded.FinancialControl) {
			t.Fatalf("%s：委托详情 = %+v, 复核详情 = %+v, 想要同一份", testCase.requestID, detail.FinancialControl, recorded.FinancialControl)
		}
	}
	restricted, _, _ := fixture.views.FindVisibleByID(ctx, scope, taskRequestID(t, "REQ-FC-RESTRICTED"))
	if items := restricted.FinancialControl.Items(); len(items) != 1 || items[0].Basis().String() != "AVAILABLE_BALANCE_INSUFFICIENT" ||
		restricted.FinancialControl.Basis().String() != "AVAILABLE_BALANCE_INSUFFICIENT" {
		t.Fatalf("受限的原因 = %+v", restricted.FinancialControl)
	}
}
