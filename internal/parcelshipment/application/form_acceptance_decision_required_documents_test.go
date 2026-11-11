package application_test

import (
	"context"
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// 本文件钉受理决定这一步对「必要资料」组的接法（ADR-0376 决定一，票 declared-goods-lines/09）：规则包声明这一组适用时按快照上
// 的提交条件判，缺了停进等客户受控补充、把缺的各项记在任务上并保存；没声明就不出这一组的结果。

func fbaHeadhaulConditions(t *testing.T) domain.SubmissionConditions {
	t.Helper()
	conditions, err := domain.NewSubmissionConditions(
		domain.PlatformShipmentDeclaredCondition, domain.EveryParcelHasPlatformBoxLabelCondition, domain.PackingDeclaredCondition)
	if err != nil {
		t.Fatal(err)
	}
	return conditions
}

// Covers: 声明了「必要资料」组适用、夹具委托没报平台货件、箱标号与装箱——停在等客户补充（不拒单），任务记下缺的各项，委托
// 照停进外部续办的路保存。
func TestUnmetSubmissionConditionsPauseTheDecisionForACustomerSupplement(t *testing.T) {
	fixture := newDecisionFixture(t)
	fixture.commercial.applicable = []domain.AcceptanceCheckGroup{
		domain.PreAcceptanceFinancialControlCheck, domain.NetworkReachabilityCheck, domain.RequiredDocumentCheck,
	}
	fixture.commercial.conditions = fbaHeadhaulConditions(t)

	result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.State() == domain.ShipmentRequestRejected || result.PendingReason() != application.CustomerSupplementPending {
		t.Fatalf("state = %v pending = %v, want 停在等客户补充", result.State(), result.PendingReason())
	}
	if fixture.requests.saved == nil {
		t.Fatal("停进等客户补充却没保存")
	}
	gaps := fixture.requests.saved.AcceptanceDecisionTask().SupplementGaps()
	if len(gaps) != 4 {
		t.Fatalf("缺口 %d 项, want 4（平台货件、两箱箱标号、装箱）", len(gaps))
	}
}

// Covers: 规则包没声明「必要资料」组，快照上就算带着条件也不判——没声明的组不该多一道判断，委托照常接受。
func TestSubmissionConditionsAreNotJudgedWhenTheGroupIsNotDeclared(t *testing.T) {
	fixture := newDecisionFixture(t)
	fixture.commercial.conditions = fbaHeadhaulConditions(t)

	result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.State() != domain.ShipmentRequestAccepted {
		t.Fatalf("state = %v pending = %v, want 照常接受", result.State(), result.PendingReason())
	}
	decision, _ := result.AcceptanceDecision()
	for _, check := range decision.Checks() {
		if check.Group() == domain.RequiredDocumentCheck {
			t.Fatal("没声明的「必要资料」组出了结果")
		}
	}
}
