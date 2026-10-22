package application_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// laterStatementCharge 是归属日 2026-09-01 的已确认费用：晚于 publishCommand 点名的本期最后一个归属日。
func laterStatementCharge(t *testing.T, id string, amount int64) domain.CustomerCharge {
	t.Helper()
	attributed, err := unattributedStatementCharge(t, id, amount).WithAttributionDate(
		domain.AttributionDate{Year: 2026, Month: time.September, Day: 1})
	if err != nil {
		t.Fatal(err)
	}
	return attributed
}

type exclusionView struct {
	charge  string
	cause   application.StatementExclusionCause
	date    string
	billed  string
	missing []domain.AttributionRegister
}

func exclusionsOf(result application.StatementResult) []exclusionView {
	var views []exclusionView
	for _, exclusion := range result.Exclusions() {
		view := exclusionView{charge: exclusion.Charge.String(), cause: exclusion.Cause, billed: exclusion.BilledOn.String(), missing: exclusion.Missing}
		if exclusion.AttributionDate.Valid() {
			view.date = exclusion.AttributionDate.String()
		}
		views = append(views, view)
	}
	return views
}

func statementLines(t *testing.T, result application.StatementResult) []string {
	t.Helper()
	record, ok := result.Statement()
	if !ok {
		t.Fatal("没有对账单")
	}
	var lines []string
	for _, line := range record.Statement.Lines() {
		lines = append(lines, line.Charge.String())
	}
	return lines
}

// Covers: `AT-SA-212` 前半「命令列出的已确认费用里，有一件的归属日晚于本期最后一个归属日 → 那一件不入本期
// 草稿并被指名，其余合格费用照常纳入；不按调用方列出的清单放行」。申报总额与入本期的行勾稽。
func TestACutAdmitsByTheLastAttributionDateAndNamesTheLaterCharge(t *testing.T) {
	fixture := newStatementFixture(t)
	fixture.charges.charges["charge-later"] = laterStatementCharge(t, "charge-later", 3000)
	command := publishCommand(t)
	command.ChargeIDs = []string{"charge-A", "charge-later", "charge-B"}

	result, err := fixture.handler.Publish(context.Background(), command)
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome() != application.StatementPublished {
		t.Fatalf("outcome = %s", result.Outcome())
	}
	if lines := statementLines(t, result); !reflect.DeepEqual(lines, []string{"charge-A", "charge-B"}) {
		t.Fatalf("lines = %v", lines)
	}
	want := []exclusionView{{charge: "charge-later", cause: application.ExcludedAfterPeriod, date: "2026-09-01"}}
	if got := exclusionsOf(result); !reflect.DeepEqual(got, want) {
		t.Fatalf("exclusions = %+v, want %+v", got, want)
	}

	t.Run("the declared total reconciles with the admitted lines only", func(t *testing.T) {
		fresh := newStatementFixture(t)
		fresh.charges.charges["charge-later"] = laterStatementCharge(t, "charge-later", 3000)
		listed := command
		listed.DeclaredTotalMinor = 23000
		result, err := fresh.handler.Publish(context.Background(), listed)
		if err != nil {
			t.Fatal(err)
		}
		if result.Outcome() != application.StatementImbalanceOutcome || len(result.Exclusions()) != 1 {
			t.Fatalf("outcome = %s exclusions = %d：按列出的清单申报总额，不平即阻断，晚于本期的照样指名",
				result.Outcome(), len(result.Exclusions()))
		}
	})
}

// Covers: `AT-SA-212` 后半「一件都入不了本期而其中没有判不出归属日的（全部晚于本期、全是已出账或两者混着），
// 不形成草稿，逐件指名晚于本期或已出账，不答未配置」。
func TestACutWithNothingInPeriodFormsNoDraftAndNamesEachCharge(t *testing.T) {
	fixture := newStatementFixture(t)
	fixture.charges.charges["charge-later"] = laterStatementCharge(t, "charge-later", 3000)
	if _, err := fixture.handler.Publish(context.Background(), publishCommand(t)); err != nil {
		t.Fatal(err)
	}
	intents := len(fixture.handoff.intents)
	command := publishCommand(t)
	command.Number = "STMT-2026-08-002"
	command.ChargeIDs = []string{"charge-later", "charge-A"}
	command.DeclaredTotalMinor = 0

	result, err := fixture.handler.Publish(context.Background(), command)
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome() != application.StatementNoChargeInPeriod {
		t.Fatalf("outcome = %s, want NO_CHARGE_IN_PERIOD", result.Outcome())
	}
	want := []exclusionView{
		{charge: "charge-later", cause: application.ExcludedAfterPeriod, date: "2026-09-01"},
		{charge: "charge-A", cause: application.ExcludedAlreadyBilled, billed: "STMT-2026-08-001"},
	}
	if got := exclusionsOf(result); !reflect.DeepEqual(got, want) {
		t.Fatalf("exclusions = %+v, want %+v", got, want)
	}
	if _, has := result.Statement(); has || len(fixture.statements.records) != 1 || len(fixture.handoff.intents) != intents {
		t.Fatalf("不该形成草稿：statements = %d intents = %d", len(fixture.statements.records), len(fixture.handoff.intents))
	}
}

// Covers: `AT-SA-062`「账户时区跨日 → 按账户时区和截单时刻形成范围，不按服务器时区猜测」与 `AT-SA-213` 末句「登记
// 补上之后再截，当场判出归属日并落库，此后不变」：来源发生时点 2026-08-31T17:30Z 在 UTC 落在本期，在这一户的
// Asia/Shanghai 已是 9 月 1 日，晚于本期；判出的这一天落在费用上，对账安排换版也不回改它。
func TestACutJudgesAnUnattributedChargeInTheAccountsTimeZoneAndRecordsIt(t *testing.T) {
	fixture := newStatementFixture(t)
	fixture.forms.register(t, "fee-freight", domain.ChargeAttributionSourceOccurred)
	fixture.arrangements.register(t, "ACCOUNT/syn-1", "Asia/Shanghai", 23*60+59)
	fixture.charges.charges["charge-unattributed"] = unattributedStatementCharge(t, "charge-unattributed", 3000)
	command := publishCommand(t)
	command.ChargeIDs = []string{"charge-A", "charge-B", "charge-unattributed"}

	result, err := fixture.handler.Publish(context.Background(), command)
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome() != application.StatementPublished {
		t.Fatalf("outcome = %s", result.Outcome())
	}
	want := []exclusionView{{charge: "charge-unattributed", cause: application.ExcludedAfterPeriod, date: "2026-09-01"}}
	if got := exclusionsOf(result); !reflect.DeepEqual(got, want) {
		t.Fatalf("exclusions = %+v, want %+v", got, want)
	}
	date, attributed := fixture.charges.charges["charge-unattributed"].AttributionDate()
	if !attributed || date.String() != "2026-09-01" || fixture.charges.records != 1 {
		t.Fatalf("落库的归属日 = %s attributed=%v records=%d", date, attributed, fixture.charges.records)
	}

	// 换成 UTC 的一版再截：已形成的归属日照旧只读不判，仍是 9 月 1 日。
	fixture.arrangements.register(t, "ACCOUNT/syn-1", "UTC", 23*60+59)
	again := command
	again.Number = "STMT-2026-08-002"
	again.ChargeIDs = []string{"charge-unattributed"}
	again.DeclaredTotalMinor = 0
	result, err = fixture.handler.Publish(context.Background(), again)
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome() != application.StatementNoChargeInPeriod || fixture.charges.records != 1 {
		t.Fatalf("outcome = %s records = %d：已形成的归属日被重判了", result.Outcome(), fixture.charges.records)
	}
}

// Covers: `AT-SA-213` 前半「截单命令列出的已确认费用没有归属日，本账户的对账安排或该费用项目的归属日形态没登记 →
// 那一件不入本期并被指名缺哪一册，其余合格费用照常纳入、草稿照常形成」，不用签收日或服务器时区顶上。
func TestACutNamesTheRegisterAnUnjudgeableChargeIsMissing(t *testing.T) {
	fixture := newStatementFixture(t)
	fixture.forms.register(t, "fee-freight", domain.ChargeAttributionSourceOccurred)
	fixture.charges.charges["charge-unattributed"] = unattributedStatementCharge(t, "charge-unattributed", 3000)
	command := publishCommand(t)
	command.ChargeIDs = []string{"charge-unattributed", "charge-A", "charge-B"}

	result, err := fixture.handler.Publish(context.Background(), command)
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome() != application.StatementPublished {
		t.Fatalf("outcome = %s", result.Outcome())
	}
	want := []exclusionView{{
		charge: "charge-unattributed", cause: application.ExcludedAttributionUnconfigured,
		missing: []domain.AttributionRegister{domain.StatementArrangementRegister},
	}}
	if got := exclusionsOf(result); !reflect.DeepEqual(got, want) {
		t.Fatalf("exclusions = %+v, want %+v", got, want)
	}
	if want[0].cause.String() != "CHARGE_ATTRIBUTION_UNCONFIGURED" {
		t.Fatalf("判不出的那一件带出的码 = %s", want[0].cause)
	}
	if _, attributed := fixture.charges.charges["charge-unattributed"].AttributionDate(); attributed || fixture.charges.records != 0 {
		t.Fatal("判不出却落了归属日")
	}
}

// Covers: `AT-SA-213` 中段「命令列出的费用一件都入不了本期、且其中有判不出归属日的，不形成草稿，整笔答未配置并
// 指名缺哪一册」，与末句「登记补上之后再截，当场判出归属日并落库」。
func TestACutWithNothingInPeriodAndAnUnjudgeableChargeIsUnconfigured(t *testing.T) {
	fixture := newStatementFixture(t)
	fixture.charges.charges["charge-later"] = laterStatementCharge(t, "charge-later", 3000)
	fixture.charges.charges["charge-unattributed"] = unattributedStatementCharge(t, "charge-unattributed", 3000)
	command := publishCommand(t)
	command.ChargeIDs = []string{"charge-later", "charge-unattributed"}
	command.DeclaredTotalMinor = 3000

	result, err := fixture.handler.Publish(context.Background(), command)
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome() != application.StatementCutUnconfigured {
		t.Fatalf("outcome = %s, want STATEMENT_CUT_UNCONFIGURED", result.Outcome())
	}
	want := []exclusionView{
		{charge: "charge-later", cause: application.ExcludedAfterPeriod, date: "2026-09-01"},
		{charge: "charge-unattributed", cause: application.ExcludedAttributionUnconfigured,
			missing: []domain.AttributionRegister{domain.StatementArrangementRegister, domain.ChargeAttributionFormRegister}},
	}
	if got := exclusionsOf(result); !reflect.DeepEqual(got, want) {
		t.Fatalf("exclusions = %+v, want %+v", got, want)
	}
	if len(fixture.statements.records) != 0 || len(fixture.handoff.intents) != 0 {
		t.Fatal("整笔未配置却形成了对账单")
	}

	fixture.forms.register(t, "fee-freight", domain.ChargeAttributionSourceOccurred)
	fixture.arrangements.register(t, "ACCOUNT/syn-1", "UTC", 23*60+59)
	result, err = fixture.handler.Publish(context.Background(), command)
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome() != application.StatementPublished {
		t.Fatalf("补登之后 outcome = %s", result.Outcome())
	}
	if lines := statementLines(t, result); !reflect.DeepEqual(lines, []string{"charge-unattributed"}) {
		t.Fatalf("lines = %v", lines)
	}
	if date, attributed := fixture.charges.charges["charge-unattributed"].AttributionDate(); !attributed || date.String() != "2026-08-31" {
		t.Fatalf("落库的归属日 = %s attributed=%v", date, attributed)
	}
}

// 整单作废后替代单要重新收原单上的费用：作废单上的费用不算已出账。
func TestChargesOnAVoidedStatementCanBeBilledAgain(t *testing.T) {
	fixture := newStatementFixture(t)
	if _, err := fixture.handler.Publish(context.Background(), publishCommand(t)); err != nil {
		t.Fatal(err)
	}
	tenant, _ := domain.NewTenantID("tenant-1")
	if _, err := fixture.handler.Void(context.Background(), application.VoidStatementCommand{
		TenantID: tenant, Number: "STMT-2026-08-001", Basis: "void-basis-1", VoidedAt: statementPublishAt.Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	replacement := publishCommand(t)
	replacement.Number = "STMT-2026-08-001R"
	result, err := fixture.handler.Publish(context.Background(), replacement)
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome() != application.StatementPublished || len(result.Exclusions()) != 0 {
		t.Fatalf("outcome = %s exclusions = %+v", result.Outcome(), exclusionsOf(result))
	}
}

// 出账锁按费用标识的次序取，与命令列出的次序无关：两笔截单以不同次序列了同几件费用，按列出次序加锁会互等成死锁。
func TestBillingLocksAreTakenInChargeOrder(t *testing.T) {
	fixture := newStatementFixture(t)
	command := publishCommand(t)
	command.ChargeIDs = []string{"charge-B", "charge-A"}
	if _, err := fixture.handler.Publish(context.Background(), command); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(fixture.billing.locks, []string{"charge-A", "charge-B"}) {
		t.Fatalf("locks = %v", fixture.billing.locks)
	}
}

// 同单号的另一笔截单在本笔读单号之后、加锁之前提交：本笔读到的「已出账」就在同一张单上，这是并发重放，
// 按那张单作答，不答本期无费用。
func TestARacingReplayOfTheSameNumberAnswersTheExistingStatement(t *testing.T) {
	fixture := newStatementFixture(t)
	if _, err := fixture.handler.Publish(context.Background(), publishCommand(t)); err != nil {
		t.Fatal(err)
	}
	fixture.statements.missFinds = 1
	result, err := fixture.handler.Publish(context.Background(), publishCommand(t))
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome() != application.StatementExistingResult {
		t.Fatalf("outcome = %s, want EXISTING_STATEMENT", result.Outcome())
	}
}

// Covers: ADR-0014 在「发布对账单」一口：SAC-1 那时写下的对账单没有本期最后一个归属日一格，按 SAC-1 那一版
// 比对，同一条命令照样答已有对账单，不因多了这一格被读成冲突。
func TestAStatementStoredUnderSAC1ReplaysUnderItsOwnShape(t *testing.T) {
	fixture := newStatementFixture(t)
	command := publishCommand(t)
	if _, err := fixture.handler.Publish(context.Background(), command); err != nil {
		t.Fatal(err)
	}
	_, sac1, err := domain.CanonicalizePublishStatementPayload(command.Account, command.Period, command.Version,
		command.Currency, command.DeclaredTotalMinor, command.CutOffAt, command.ChargeIDs)
	if err != nil {
		t.Fatal(err)
	}
	for key, record := range fixture.statements.records {
		record.ContentDigest = sac1
		fixture.statements.records[key] = record
	}
	result, err := fixture.handler.Publish(context.Background(), command)
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome() != application.StatementExistingResult {
		t.Fatalf("outcome = %s, want EXISTING_STATEMENT", result.Outcome())
	}
	// SAC-1 那一版没有这一格，按它比就不看这一格：那时的截单本不按归属日纳入，换哪一天都是同一张单的重放。
	changed := command
	changed.LastAttributionDate = "2026-08-30"
	result, err = fixture.handler.Publish(context.Background(), changed)
	if err != nil || result.Outcome() != application.StatementExistingResult {
		t.Fatalf("outcome = %s err = %v", result.Outcome(), err)
	}
}

func TestACutStopsOnItsDependenciesAndRefusesAMalformedLastDay(t *testing.T) {
	t.Run("a malformed last attribution date is not accepted", func(t *testing.T) {
		fixture := newStatementFixture(t)
		for _, raw := range []string{"", "2026-08-32", "2026-08-31T00:00:00+08:00"} {
			command := publishCommand(t)
			command.LastAttributionDate = raw
			result, err := fixture.handler.Publish(context.Background(), command)
			if err != nil || result.Outcome() != application.StatementNotAccepted {
				t.Fatalf("%q：outcome = %s err = %v", raw, result.Outcome(), err)
			}
		}
	})
	t.Run("an unreadable attribution register is undecided", func(t *testing.T) {
		fixture := newStatementFixture(t)
		fixture.arrangements.err = errors.New("arrangements down")
		fixture.charges.charges["charge-unattributed"] = unattributedStatementCharge(t, "charge-unattributed", 3000)
		command := publishCommand(t)
		command.ChargeIDs = []string{"charge-A", "charge-unattributed"}
		result, err := fixture.handler.Publish(context.Background(), command)
		if err != nil || result.Outcome() != application.StatementUndecided ||
			result.UndecidedReason() != application.StatementAttributionViewUnavailable {
			t.Fatalf("outcome = %s reason = %s err = %v", result.Outcome(), result.UndecidedReason(), err)
		}
	})
	t.Run("a billing lock failure is undecided", func(t *testing.T) {
		fixture := newStatementFixture(t)
		fixture.billing.err = errors.New("lock timeout")
		result, err := fixture.handler.Publish(context.Background(), publishCommand(t))
		if err != nil || result.Outcome() != application.StatementUndecided ||
			result.UndecidedReason() != application.StatementStoreUnavailable {
			t.Fatalf("outcome = %s reason = %s err = %v", result.Outcome(), result.UndecidedReason(), err)
		}
	})
	t.Run("a failed date record is undecided", func(t *testing.T) {
		fixture := newStatementFixture(t)
		fixture.forms.register(t, "fee-freight", domain.ChargeAttributionSourceOccurred)
		fixture.arrangements.register(t, "ACCOUNT/syn-1", "UTC", 23*60+59)
		fixture.charges.recordErr = errors.New("charge store down")
		fixture.charges.charges["charge-unattributed"] = unattributedStatementCharge(t, "charge-unattributed", 3000)
		command := publishCommand(t)
		command.ChargeIDs = []string{"charge-unattributed"}
		result, err := fixture.handler.Publish(context.Background(), command)
		if err != nil || result.Outcome() != application.StatementUndecided ||
			result.UndecidedReason() != application.StatementChargeStoreUnavailable {
			t.Fatalf("outcome = %s reason = %s err = %v", result.Outcome(), result.UndecidedReason(), err)
		}
	})
}
