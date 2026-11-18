package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// 钉「费用出账之后才形成的调整由后面的对账单带上」这一段的编排面。夹具：2026-08 那一期先发一张单，收 charge-A 与
// charge-B；之后在 charge-A 上先后形成一借一贷两笔调整，在还没出账的 charge-late 上形成一笔借项。2026-09 那一期只收
// charge-late。金额锚定本夹具：charge-late 3000，借 700、贷 300、借 700。

// adjustmentOn 造一笔已由唯一创建用例形成的计价纠错，方向与金额由用例给。
func adjustmentOn(
	t *testing.T,
	id, chargeID string,
	direction domain.AdjustmentDirection,
	minor int64,
	formedAt time.Time,
) ports.ChargeAdjustmentRecord {
	t.Helper()
	adjustment, err := domain.FormChargeAdjustment(domain.ChargeAdjustmentSpec{
		ID:                 billValue(t, domain.NewChargeAdjustmentID, id),
		Charge:             billValue(t, domain.NewCustomerChargeID, chargeID),
		Kind:               domain.PricingCorrection,
		Direction:          direction,
		Evaluation:         billValue(t, domain.NewSellEvaluationReference, "sell-evaluation/"+id),
		OriginalCurrency:   billValue(t, domain.NewCurrencyCode, "USD"),
		OriginalMinor:      minor,
		SettlementCurrency: billValue(t, domain.NewCurrencyCode, "USD"),
		SettlementMinor:    minor,
		FormedAt:           formedAt,
	})
	if err != nil {
		t.Fatalf("form adjustment %s: %v", id, err)
	}
	return ports.ChargeAdjustmentRecord{
		Key:        ports.ChargeAdjustmentKey{TenantID: billValue(t, domain.NewTenantID, "tenant-1"), Adjustment: adjustment.ID()},
		Adjustment: adjustment,
		RecordedAt: formedAt,
	}
}

// formUnbilled 把一笔调整登进替身：按标识读得到，也列在还没出账的那一份里。
func (fixture *statementFixture) formUnbilled(record ports.ChargeAdjustmentRecord) {
	fixture.adjustments.records["tenant-1|"+record.Key.Adjustment.String()] = record
	fixture.adjustments.unbilled = append(fixture.adjustments.unbilled, record)
}

// carryFixture 发出 2026-08 那张单，再登上三笔调整。
func carryFixture(t *testing.T) *statementFixture {
	t.Helper()
	fixture := newStatementFixture(t)
	fixture.adjustments.statements = fixture.statements
	fixture.adjustments.inclusions = fixture.inclusions
	first, err := fixture.handler.Publish(context.Background(), publishCommand(t))
	if err != nil || first.Outcome() != application.StatementPublished {
		t.Fatalf("首期：%v %q", err, first.Outcome())
	}
	formedAt := statementPublishAt.Add(36 * time.Hour)
	fixture.formUnbilled(adjustmentOn(t, "adj-prior-debit", "charge-A", domain.AdjustmentDebit, 700, formedAt))
	fixture.formUnbilled(adjustmentOn(t, "adj-prior-credit", "charge-A", domain.AdjustmentCredit, 300, formedAt.Add(time.Hour)))
	fixture.formUnbilled(adjustmentOn(t, "adj-current", "charge-late", domain.AdjustmentDebit, 700, formedAt.Add(2*time.Hour)))
	return fixture
}

// nextPeriodCommand 截 2026-09 那一期：只点名 charge-late，申报的是它的金额。
func nextPeriodCommand(t *testing.T) application.PublishStatementCommand {
	t.Helper()
	command := publishCommand(t)
	command.Period = "2026-09"
	command.ChargeIDs = []string{"charge-late"}
	command.LastAttributionDate = "2026-09-30"
	command.DeclaredTotalMinor = 3000
	command.CutOffAt = time.Date(2026, 9, 30, 23, 59, 0, 0, time.UTC)
	command.PublishedAt = time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	return command
}

// Covers: 到点截单申报的只是费用合计；还没出账的调整由截单编排读一次、随单带走并加进总额。挂在原账单费用上的每一笔
// 各记一笔后续纳入，关联原账单、归入这张单的账期——同一笔费用上有两笔就是两笔纳入；挂在本期费用上的不记纳入。原账单
// 不动。
func TestACycleCutCarriesUnbilledAdjustmentsAndIncludesEachPriorOne(t *testing.T) {
	fixture := carryFixture(t)
	command := nextPeriodCommand(t)
	command.CarryUnbilledAdjustments = true

	result, err := fixture.handler.Publish(context.Background(), command)
	if err != nil || result.Outcome() != application.StatementPublished {
		t.Fatalf("次期：%v %q", err, result.Outcome())
	}
	record, _ := result.Statement()
	if record.Statement.TotalMinor() != 4100 {
		t.Fatalf("总额 = %d，want 4100（3000 + 700 − 300 + 700）", record.Statement.TotalMinor())
	}
	lines := record.Statement.AdjustmentLines()
	if len(lines) != 3 {
		t.Fatalf("调整行 = %d 条，want 3", len(lines))
	}
	for _, line := range lines {
		carried := line.Charge.String() == "charge-A"
		if carried != (line.OriginalStatement.String() == "STMT-2026-08-001") {
			t.Fatalf("调整行 %s 挂在 %s 上，原账单 = %q", line.Adjustment, line.Charge, line.OriginalStatement)
		}
	}

	if len(fixture.inclusions.records) != 2 {
		t.Fatalf("后续纳入 = %d 笔，want 2（原账单费用上的两笔调整各一笔）", len(fixture.inclusions.records))
	}
	for _, id := range []string{"adj-prior-debit", "adj-prior-credit"} {
		inclusion, found := fixture.inclusions.records["tenant-1|ADJINC-"+record.Key.Number.String()+"-"+id]
		if !found {
			t.Fatalf("调整 %s 没有记纳入：%v", id, fixture.inclusions.records)
		}
		adjustment, _ := inclusion.Inclusion.Adjustment()
		if adjustment.String() != id || inclusion.Inclusion.Charge().String() != "charge-A" ||
			inclusion.Inclusion.Statement().String() != "STMT-2026-08-001" ||
			inclusion.Inclusion.OriginalPeriod().String() != "2026-08" ||
			inclusion.Inclusion.SubsequentPeriod().String() != "2026-09" {
			t.Fatalf("纳入 %s 没有关联原账单与这张单的账期：%+v", id, inclusion.Inclusion)
		}
	}
	// 首期的单一份、两笔纳入各一份、次期的单一份。
	if len(fixture.handoff.intents) != 4 {
		t.Fatalf("交出去的意图 = %d 份，want 4", len(fixture.handoff.intents))
	}
	original, _, err := fixture.statements.FindByKey(context.Background(), ports.StatementKey{
		TenantID: record.Key.TenantID, Number: billValue(t, domain.NewStatementNumber, "STMT-2026-08-001"),
	})
	if err != nil || original.Statement.TotalMinor() != 20000 || len(original.Statement.AdjustmentLines()) != 0 {
		t.Fatalf("原账单被改了：%d / %d 条调整行 err = %v", original.Statement.TotalMinor(), len(original.Statement.AdjustmentLines()), err)
	}

	t.Run("voiding the carrier withdraws its inclusions and the next statement carries them again", func(t *testing.T) {
		voided, err := fixture.handler.Void(context.Background(), application.VoidStatementCommand{
			TenantID: record.Key.TenantID, Number: record.Key.Number.String(), Basis: "void-basis-carrier",
			VoidedAt: command.PublishedAt.Add(time.Hour),
		})
		if err != nil || voided.Outcome() != application.StatementVoidedOutcome {
			t.Fatalf("作废：%v %q", err, voided.Outcome())
		}
		if len(fixture.inclusions.withdrawn) != 2 {
			t.Fatalf("撤回了 %d 笔纳入，want 这张单带走的两笔", len(fixture.inclusions.withdrawn))
		}
		for index, basis := range fixture.inclusions.withdrawn {
			if basis != "STATEMENT_VOIDED/"+record.Key.Number.String() {
				t.Fatalf("纳入 %s 的撤回依据 = %q", index, basis)
			}
		}

		// 替代单：charge-late 随原单作废回到没出账，三笔调整也都回来了。
		replacement := nextPeriodCommand(t)
		replacement.Period = "2026-10"
		replacement.LastAttributionDate = "2026-10-31"
		replacement.CutOffAt = time.Date(2026, 10, 31, 23, 59, 0, 0, time.UTC)
		replacement.PublishedAt = time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)
		replacement.CarryUnbilledAdjustments = true
		reissued, err := fixture.handler.Publish(context.Background(), replacement)
		if err != nil || reissued.Outcome() != application.StatementPublished {
			t.Fatalf("替代单：%v %q", err, reissued.Outcome())
		}
		again, _ := reissued.Statement()
		if again.Statement.TotalMinor() != 4100 || len(again.Statement.AdjustmentLines()) != 3 {
			t.Fatalf("替代单总额 %d、调整行 %d 条；want 4100、3 条", again.Statement.TotalMinor(), len(again.Statement.AdjustmentLines()))
		}
		for _, id := range []string{"adj-prior-debit", "adj-prior-credit"} {
			if _, found := fixture.inclusions.records["tenant-1|ADJINC-"+again.Key.Number.String()+"-"+id]; !found {
				t.Fatalf("替代单没有给 %s 另记一笔纳入：%v", id, fixture.inclusions.records)
			}
		}
		if len(fixture.inclusions.records) != 4 || len(fixture.inclusions.withdrawn) != 2 {
			t.Fatalf("纳入 %d 笔、撤回 %d 笔；want 4 笔里撤回 2 笔", len(fixture.inclusions.records), len(fixture.inclusions.withdrawn))
		}
	})
}

// Covers: 本期一笔费用都没有、这一户却有出账之后才形成的调整：照样出一张只有调整行的单，净额为负就是贷记。挂在还没
// 出账的费用上的调整不带——它随那笔费用出账。
func TestAClosingWithoutChargesStillPublishesTheCarriedAdjustments(t *testing.T) {
	fixture := carryFixture(t)
	fixture.formUnbilled(adjustmentOn(t, "adj-prior-credit-b", "charge-B", domain.AdjustmentCredit, 900, statementPublishAt.Add(40*time.Hour)))
	command := nextPeriodCommand(t)
	command.ChargeIDs = nil
	command.DeclaredTotalMinor = 0
	command.CarryUnbilledAdjustments = true

	result, err := fixture.handler.Publish(context.Background(), command)
	if err != nil || result.Outcome() != application.StatementPublished {
		t.Fatalf("只有调整的那一期：%v %q", err, result.Outcome())
	}
	record, _ := result.Statement()
	if record.Statement.TotalMinor() != -500 || len(record.Statement.Lines()) != 0 || len(record.Statement.AdjustmentLines()) != 3 {
		t.Fatalf("总额 %d、费用行 %d、调整行 %d；want −500（700 − 300 − 900）、0、3",
			record.Statement.TotalMinor(), len(record.Statement.Lines()), len(record.Statement.AdjustmentLines()))
	}
	for _, line := range record.Statement.AdjustmentLines() {
		if line.Adjustment.String() == "adj-current" || line.OriginalStatement.String() != "STMT-2026-08-001" {
			t.Fatalf("调整行 %s 的原账单 = %q", line.Adjustment, line.OriginalStatement)
		}
	}
	if len(fixture.inclusions.records) != 3 {
		t.Fatalf("后续纳入 = %d 笔，want 3", len(fixture.inclusions.records))
	}

	t.Run("without the switch a period with no charge publishes nothing", func(t *testing.T) {
		fresh := carryFixture(t)
		manual := nextPeriodCommand(t)
		manual.ChargeIDs = nil
		manual.DeclaredTotalMinor = 0
		result, err := fresh.handler.Publish(context.Background(), manual)
		if err != nil || result.Outcome() != application.StatementNoChargeInPeriod || len(fresh.statements.records) != 1 {
			t.Fatalf("没开开关：%v %q，对账单 %d 张", err, result.Outcome(), len(fresh.statements.records))
		}
	})
}

// Covers: 到点截单的发布意图不看调整：同一期重放答原单原号，哪怕这之间又形成了一笔调整。
func TestAReplayedCycleCutAnswersTheOriginalStatement(t *testing.T) {
	fixture := carryFixture(t)
	command := nextPeriodCommand(t)
	command.CarryUnbilledAdjustments = true
	first, err := fixture.handler.Publish(context.Background(), command)
	if err != nil || first.Outcome() != application.StatementPublished {
		t.Fatalf("次期：%v %q", err, first.Outcome())
	}
	record, _ := first.Statement()
	t.Run("a replay answers the original statement even after another adjustment formed", func(t *testing.T) {
		fixture.formUnbilled(adjustmentOn(t, "adj-after", "charge-A", domain.AdjustmentDebit, 100, statementPublishAt.Add(40*24*time.Hour)))
		replay, err := fixture.handler.Publish(context.Background(), command)
		if err != nil || replay.Outcome() != application.StatementExistingResult {
			t.Fatalf("重放：%v %q——到点截单的发布意图不随调整的多少变成另一份内容", err, replay.Outcome())
		}
		if again, _ := replay.Statement(); again.Key.Number != record.Key.Number || len(fixture.inclusions.records) != 2 {
			t.Fatalf("重放答 %s，纳入 %d 笔", again.Key.Number, len(fixture.inclusions.records))
		}
	})
}

// Covers: 运营发布口不带别的对账单上的调整；本期费用上还没出账的调整随单带走，申报总额要把它算进去，少报即勾稽不平。
func TestAManualCutOffCarriesOnlyTheAdjustmentsOnItsOwnCharges(t *testing.T) {
	fixture := carryFixture(t)

	short := nextPeriodCommand(t)
	result, err := fixture.handler.Publish(context.Background(), short)
	if err != nil || result.Outcome() != application.StatementImbalanceOutcome {
		t.Fatalf("只报费用合计：%v %q，want 勾稽不平", err, result.Outcome())
	}

	command := nextPeriodCommand(t)
	command.DeclaredTotalMinor = 3700
	result, err = fixture.handler.Publish(context.Background(), command)
	if err != nil || result.Outcome() != application.StatementPublished {
		t.Fatalf("次期：%v %q", err, result.Outcome())
	}
	record, _ := result.Statement()
	lines := record.Statement.AdjustmentLines()
	if record.Statement.TotalMinor() != 3700 || len(lines) != 1 || lines[0].Adjustment.String() != "adj-current" ||
		lines[0].OriginalStatement.String() != "" {
		t.Fatalf("总额 %d，调整行 %+v；want 3700、只有本期费用上的那一笔", record.Statement.TotalMinor(), lines)
	}
	if len(fixture.inclusions.records) != 0 {
		t.Fatalf("运营发布口记了 %d 笔后续纳入", len(fixture.inclusions.records))
	}
}

// Covers: 随单带走的调整挂的费用既不在本期、也不在任何未作废的对账单上，是两侧说法矛盾：答未决，不落单、不取号。
func TestACarriedAdjustmentWithoutAnOriginalStatementStopsTheCut(t *testing.T) {
	fixture := carryFixture(t)
	// 真册只交回费用在未作废对账单上的调整，走不到这一格；这里让替身不看对账单册，把矛盾造出来。
	fixture.adjustments.statements = nil
	fixture.formUnbilled(adjustmentOn(t, "adj-stray", "charge-stray", domain.AdjustmentDebit, 50, statementPublishAt.Add(48*time.Hour)))
	command := nextPeriodCommand(t)
	command.CarryUnbilledAdjustments = true
	taken := len(fixture.numbers.requests)

	result, err := fixture.handler.Publish(context.Background(), command)
	if err != nil || result.Outcome() != application.StatementUndecided || result.UndecidedReason() != application.AdjustmentViewUnavailable {
		t.Fatalf("结果：%v %q / %q", err, result.Outcome(), result.UndecidedReason())
	}
	if len(fixture.statements.records) != 1 || len(fixture.numbers.requests) != taken || len(fixture.inclusions.records) != 0 {
		t.Fatalf("停下却落了：对账单 %d 张，取号 %d 次，纳入 %d 笔",
			len(fixture.statements.records), len(fixture.numbers.requests)-taken, len(fixture.inclusions.records))
	}
}

// Covers: 纳入的意图没交出去：号已取走、单已落下，交回错误让调用方整笔回滚。重放答已有对账单，不会回来补发这一份。
func TestAnInclusionIntentThatIsNotHandedOffRollsTheCutBack(t *testing.T) {
	fixture := carryFixture(t)
	command := nextPeriodCommand(t)
	command.CarryUnbilledAdjustments = true
	fixture.handoff.err = errors.New("downstream unavailable")

	if _, err := fixture.handler.Publish(context.Background(), command); !errors.Is(err, application.ErrNumberedIssueInterrupted) {
		t.Fatalf("err = %v，want 交回错误让事务回滚", err)
	}
}
