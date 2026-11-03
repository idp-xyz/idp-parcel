package application_test

import (
	"context"
	"testing"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// rangeCommand 是新形载荷：点名决定身份、结果与这一行上的一段。basis 与 resolves 为空即不带。
func rangeCommand(
	t *testing.T,
	line, decision string,
	verdict domain.AuditVerdict,
	from, to int64,
	basis, resolves string,
) application.AuditBillLineCommand {
	t.Helper()
	span, err := domain.NewAmountRange(from, to)
	if err != nil {
		t.Fatalf("range: %v", err)
	}
	command := application.AuditBillLineCommand{
		TenantID: billValue(t, domain.NewTenantID, "tenant-1"),
		Claim:    billValue(t, domain.NewBillClaimID, "bill-1"),
		Version:  billValue(t, domain.NewBillClaimVersion, "bill-1/v1"),
		Line:     billValue(t, domain.NewBillLineReference, line),
		Decision: billValue(t, domain.NewAuditDecisionID, decision),
		Verdict:  verdict,
		Range:    span,
	}
	if basis != "" {
		command.Basis = billValue(t, domain.NewAuditBasisReference, basis)
	}
	if resolves != "" {
		command.Resolves = billValue(t, domain.NewAuditDecisionID, resolves)
	}
	return command
}

func auditOnce(t *testing.T, fixture *billFixture, command application.AuditBillLineCommand) application.AuditResult {
	t.Helper()
	result, err := fixture.handler.Audit(context.Background(), command)
	if err != nil {
		t.Fatalf("audit %s: %v", command.Decision, err)
	}
	return result
}

// Covers: `AT-SA-234`（编排半边）——line-1 已匹配 12000 拆成三段：通过的一段形成应付并取号，争议与拒绝记下而不取号、不交下游；
// 重放答原决定；重叠答范围已裁决；换个身份提交同一段同一结果答先到的那份；越权上限比这一行的已匹配金额而不比这一段；争议
// 与拒绝只看审核授权，不看应付账户与上限。
func TestALineIsAuditedInRangesWithThreeVerdicts(t *testing.T) {
	fixture := receivedFixture(t)
	handoffs := len(fixture.handoff.intents)

	passed := auditOnce(t, fixture, rangeCommand(t, "line-1", "d-pass", domain.AuditPassed, 0, 6000, "", ""))
	if passed.Outcome() != application.PayableFormed {
		t.Fatalf("pass outcome = %q", passed.Outcome())
	}
	payable, _ := passed.Payable()
	if _, minor := payable.Payable.Amount(); minor != 6000 || payable.Key.Payable.String() != "d-pass" {
		t.Fatalf("应付 %s 金额 %d：金额取那一段的长度，身份取决定身份", payable.Key.Payable, minor)
	}
	if _, numbered := payable.Numbering.Number(); !numbered {
		t.Fatal("通过的一段没取应付号")
	}

	disputed := auditOnce(t, fixture, rangeCommand(t, "line-1", "d-dispute", domain.AuditDisputed, 6000, 9000, "weight-proof", ""))
	rejected := auditOnce(t, fixture, rangeCommand(t, "line-1", "d-reject", domain.AuditRejected, 9000, 12000, "not-ordered", ""))
	if disputed.Outcome() != application.AuditDisputeRecorded || rejected.Outcome() != application.AuditRejectionRecorded {
		t.Fatalf("dispute = %q reject = %q", disputed.Outcome(), rejected.Outcome())
	}
	if decision, has := disputed.Decision(); !has || decision.Decision.Auditor().String() != "auditor-1" {
		t.Fatalf("争议没带回决定或没记审核授权方：%+v", decision)
	}
	if len(fixture.payables.records) != 1 || len(fixture.numbers.requestsOf(domain.AuditedPayableDocumentNumberType.Code)) != 1 {
		t.Fatal("争议或拒绝形成了应付或取了号")
	}
	if len(fixture.handoff.intents) != handoffs+1 {
		t.Fatalf("交接 %d 份，只该多应付那一份", len(fixture.handoff.intents)-handoffs)
	}

	t.Run("a replay answers the original decision", func(t *testing.T) {
		replay := auditOnce(t, fixture, rangeCommand(t, "line-1", "d-dispute", domain.AuditDisputed, 6000, 9000, "weight-proof", ""))
		if replay.Outcome() != application.AuditDecisionExistingResult {
			t.Fatalf("outcome = %q", replay.Outcome())
		}
		passReplay := auditOnce(t, fixture, rangeCommand(t, "line-1", "d-pass", domain.AuditPassed, 0, 6000, "", ""))
		if passReplay.Outcome() != application.PayableExistingResult {
			t.Fatalf("通过的重放 outcome = %q，应带回原应付", passReplay.Outcome())
		}
	})

	t.Run("the same identity with other content is a conflict", func(t *testing.T) {
		if got := auditOnce(t, fixture, rangeCommand(t, "line-1", "d-dispute", domain.AuditRejected, 6000, 9000, "x", "")).Outcome(); got != application.AuditDecisionConflict {
			t.Fatalf("outcome = %q", got)
		}
		if got := auditOnce(t, fixture, rangeCommand(t, "line-1", "d-reject", domain.AuditPassed, 9000, 12000, "", "")).Outcome(); got != application.PayableConflict {
			t.Fatalf("这次是通过时 outcome = %q，沿用应付冲突", got)
		}
	})

	t.Run("a decided range is final", func(t *testing.T) {
		for name, command := range map[string]application.AuditBillLineCommand{
			"re-pass the rejected range":   rangeCommand(t, "line-1", "x1", domain.AuditPassed, 9000, 9500, "", ""),
			"reject inside the passed one": rangeCommand(t, "line-1", "x2", domain.AuditRejected, 100, 200, "late", ""),
			"pass the disputed range":      rangeCommand(t, "line-1", "x3", domain.AuditPassed, 6000, 7000, "", ""),
		} {
			if got := auditOnce(t, fixture, command).Outcome(); got != application.AuditRangeAlreadyDecided {
				t.Fatalf("%s: outcome = %q", name, got)
			}
		}
	})

	t.Run("the same change under another identity answers the first", func(t *testing.T) {
		again := auditOnce(t, fixture, rangeCommand(t, "line-1", "d-reject-again", domain.AuditRejected, 9000, 12000, "not-ordered", ""))
		decision, _ := again.Decision()
		if again.Outcome() != application.AuditDecisionExistingResult || decision.Key.Decision.String() != "d-reject" {
			t.Fatalf("outcome = %q decision = %s", again.Outcome(), decision.Key.Decision)
		}
	})

	t.Run("mixing the two payload shapes is not accepted", func(t *testing.T) {
		mixed := rangeCommand(t, "line-1", "x9", domain.AuditPassed, 0, 1, "", "")
		mixed.Payable = billValue(t, domain.NewPayableID, "payable-x")
		if got := auditOnce(t, fixture, mixed).Outcome(); got != application.AuditNotAccepted {
			t.Fatalf("outcome = %q", got)
		}
	})
}

// Covers: `AT-SA-234`——价差行 line-2（主张 3000、预期 2500）只能通过到预期成本，超出答这一行不可审核并带出已匹配与已通过金额；
// 超出的那一段只能争议或拒绝。
func TestAVarianceLinePassesOnlyItsMatchedAmount(t *testing.T) {
	fixture := receivedFixture(t)
	if got := auditOnce(t, fixture, rangeCommand(t, "line-2", "v-pass", domain.AuditPassed, 0, 2500, "", "")).Outcome(); got != application.PayableFormed {
		t.Fatalf("预期成本撑得住的那部分 outcome = %q", got)
	}
	over := auditOnce(t, fixture, rangeCommand(t, "line-2", "v-over", domain.AuditPassed, 2500, 2600, "", ""))
	matched, passed := over.MatchedAmounts()
	if over.Outcome() != application.LineNotAuditable || matched != 2500 || passed != 2500 {
		t.Fatalf("outcome = %q matched = %d passed = %d", over.Outcome(), matched, passed)
	}
	if got := auditOnce(t, fixture, rangeCommand(t, "line-2", "v-dispute", domain.AuditDisputed, 2500, 3000, "price-dispute", "")).Outcome(); got != application.AuditDisputeRecorded {
		t.Fatalf("超出的那一段 outcome = %q", got)
	}
}

// Covers: `AT-SA-235`（编排半边）——争议以再审解除：再审落在原争议之内，通过的那部分形成同一行上的第二份应付并取号；没覆盖到的
// 部分仍在争议中，可再审；再审出了原争议、点名的不是争议，都答未受理；同一争议的再审重叠答范围已裁决。
func TestADisputedRangeIsReauditedOnlyWithinItself(t *testing.T) {
	fixture := receivedFixture(t)
	auditOnce(t, fixture, rangeCommand(t, "line-1", "pass", domain.AuditPassed, 0, 6000, "", ""))
	auditOnce(t, fixture, rangeCommand(t, "line-1", "dispute", domain.AuditDisputed, 6000, 12000, "weight-proof", ""))

	resolved := auditOnce(t, fixture, rangeCommand(t, "line-1", "re-pass", domain.AuditPassed, 6000, 10000, "weight-confirmed", "dispute"))
	if resolved.Outcome() != application.PayableFormed {
		t.Fatalf("re-audit pass outcome = %q", resolved.Outcome())
	}
	if len(fixture.payables.records) != 2 {
		t.Fatalf("一行两段通过应有两份应付，实得 %d", len(fixture.payables.records))
	}

	refused := map[string]struct {
		command application.AuditBillLineCommand
		want    application.AuditOutcome
	}{
		"outside the dispute":            {rangeCommand(t, "line-1", "r1", domain.AuditRejected, 5000, 7000, "b", "dispute"), application.AuditNotAccepted},
		"resolving a pass":               {rangeCommand(t, "line-1", "r2", domain.AuditRejected, 0, 100, "b", "pass"), application.AuditNotAccepted},
		"overlapping an earlier reaudit": {rangeCommand(t, "line-1", "r3", domain.AuditRejected, 9000, 11000, "b", "dispute"), application.AuditRangeAlreadyDecided},
	}
	for name, testCase := range refused {
		if got := auditOnce(t, fixture, testCase.command).Outcome(); got != testCase.want {
			t.Fatalf("%s: outcome = %q, want %q", name, got, testCase.want)
		}
	}

	rest := auditOnce(t, fixture, rangeCommand(t, "line-1", "re-reject", domain.AuditRejected, 10000, 12000, "refused", "dispute"))
	if rest.Outcome() != application.AuditRejectionRecorded {
		t.Fatalf("剩下的争议 outcome = %q", rest.Outcome())
	}
}

// Covers: `AT-SA-234`——越权上限比这一行的已匹配金额：上限 11999 低于 line-1 的 12000，拆出一段 100 也要升级、不取号；争议与
// 拒绝不形成应付，不看上限与应付账户，审核授权没登照样停在未决。
func TestTheCeilingComparesTheLineNotTheRange(t *testing.T) {
	fixture := receivedFixture(t)
	ceiling, err := domain.NewAuditEscalationCeiling(11_999)
	if err != nil {
		t.Fatalf("ceiling: %v", err)
	}
	fixture.escalation.ceiling = ceiling
	if got := auditOnce(t, fixture, rangeCommand(t, "line-1", "small", domain.AuditPassed, 0, 100, "", "")).Outcome(); got != application.AuditMustEscalate {
		t.Fatalf("outcome = %q，拆小段不该绕过上限", got)
	}
	if len(fixture.numbers.requestsOf(domain.AuditedPayableDocumentNumberType.Code)) != 0 {
		t.Fatal("要升级却取了号")
	}

	fixture.escalation.configured, fixture.accounts.configured = false, false
	if got := auditOnce(t, fixture, rangeCommand(t, "line-1", "dispute", domain.AuditDisputed, 0, 100, "b", "")).Outcome(); got != application.AuditDisputeRecorded {
		t.Fatalf("争议不看上限与应付账户，outcome = %q", got)
	}
	fixture.authority.configured = false
	result := auditOnce(t, fixture, rangeCommand(t, "line-1", "reject", domain.AuditRejected, 100, 200, "b", ""))
	if result.Outcome() != application.AuditUndecided || result.UndecidedReason() != application.AuditAuthorityUnconfigured {
		t.Fatalf("无授权不得拒绝：outcome = %q reason = %q", result.Outcome(), result.UndecidedReason())
	}
}
