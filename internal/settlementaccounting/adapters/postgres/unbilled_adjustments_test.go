package postgres_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// 本文件对真实 PostgreSQL 16 钉「还没出账的调整」这一读法与带着别的对账单上调整的那张单的往返。截单编排凭这份读法
// 决定哪些调整随单带走：读多了同一笔调整收两遍，读少了那笔钱永远不出账。

var unbilledFormedAt = time.Date(2026, 9, 2, 9, 0, 0, 0, time.UTC)

type unbilledFixture struct {
	db          *bentopg.DB
	charges     *adapter.CustomerCharges
	statements  *adapter.CustomerStatements
	adjustments *adapter.ChargeAdjustments
	inclusions  *adapter.SubsequentInclusions
	tenant      domain.TenantID
	within      func(func(context.Context) error)
}

// newUnbilledFixture 摆出一户上各种处境的调整。账户一：
//   - charge-billed 在未作废的 STMT-OPEN 上。它上面四笔调整：adj-billed 已写在 STMT-OPEN 的调整行里；adj-carried 已由
//     后面的 STMT-NEXT 带走；adj-included 已记过后续纳入；adj-prior 出账之后才形成，哪儿都还没进。
//   - charge-current 还没出账，上面一笔 adj-current。
//   - charge-voided 在已作废的 STMT-VOIDED 上，那张单的调整行里写着 adj-on-voided。
//
// 账户二：charge-elsewhere 在未作废的 STMT-ELSEWHERE 上，上面一笔 adj-elsewhere。
func newUnbilledFixture(t *testing.T) unbilledFixture {
	t.Helper()
	db := catalogueDB(t)
	fixture := unbilledFixture{db: db, tenant: saTenant(t, "tenant-1")}
	var err error
	if fixture.charges, err = adapter.NewCustomerCharges(db); err != nil {
		t.Fatal(err)
	}
	if fixture.statements, err = adapter.NewCustomerStatements(db); err != nil {
		t.Fatal(err)
	}
	if fixture.adjustments, err = adapter.NewChargeAdjustments(db); err != nil {
		t.Fatal(err)
	}
	if fixture.inclusions, err = adapter.NewSubsequentInclusions(db); err != nil {
		t.Fatal(err)
	}
	fixture.within = func(write func(context.Context) error) {
		t.Helper()
		saWithin(t, db.Transactor(), t.Context(), write)
	}

	open := carryingStatement(t, "STMT-OPEN", "ACCOUNT/syn-1", "2026-08", "charge-billed", time.Hour,
		domain.StatementAdjustmentLine{
			Adjustment: saValue(t, domain.NewChargeAdjustmentID, "adj-billed"),
			Charge:     saValue(t, domain.NewCustomerChargeID, "charge-billed"),
			Direction:  domain.AdjustmentDebit, AmountMinor: 900,
		})
	voided := carryingStatement(t, "STMT-VOIDED", "ACCOUNT/syn-1", "2026-07", "charge-voided", 30*time.Minute,
		domain.StatementAdjustmentLine{
			Adjustment: saValue(t, domain.NewChargeAdjustmentID, "adj-on-voided"),
			Charge:     saValue(t, domain.NewCustomerChargeID, "charge-voided"),
			Direction:  domain.AdjustmentDebit, AmountMinor: 900,
		})
	voidedStatement, err := voided.Statement.Void(saValue(t, domain.NewStatementVoidBasisReference, "void-basis-1"), statementVoidAt)
	if err != nil {
		t.Fatalf("作废：%v", err)
	}
	voided.Statement = voidedStatement
	next := carryingStatement(t, "STMT-NEXT", "ACCOUNT/syn-1", "2026-09", "charge-next", 24*time.Hour,
		domain.StatementAdjustmentLine{
			Adjustment: saValue(t, domain.NewChargeAdjustmentID, "adj-carried"),
			Charge:     saValue(t, domain.NewCustomerChargeID, "charge-billed"),
			Direction:  domain.AdjustmentDebit, AmountMinor: 900,
			OriginalStatement: saValue(t, domain.NewStatementNumber, "STMT-OPEN"),
		})
	elsewhere := carryingStatement(t, "STMT-ELSEWHERE", "ACCOUNT/syn-2", "2026-08", "charge-elsewhere", time.Hour)

	included := correctionAdjustment(t, "adj-included", "charge-billed", unbilledFormedAt)
	inclusion, err := domain.IncludeAdjustmentInSubsequentPeriod(open.Statement, included,
		saValue(t, domain.NewInclusionReference, "inclusion-unbilled-1"),
		saValue(t, domain.NewBillingPeriodReference, "2026-10"), inclusionAt)
	if err != nil {
		t.Fatalf("形成纳入：%v", err)
	}

	fixture.within(func(txCtx context.Context) error {
		for _, charge := range []domain.CustomerCharge{
			confirmedChargeOn(t, "charge-billed", "ACCOUNT/syn-1"),
			confirmedChargeOn(t, "charge-current", "ACCOUNT/syn-1"),
			confirmedChargeOn(t, "charge-voided", "ACCOUNT/syn-1"),
			confirmedChargeOn(t, "charge-elsewhere", "ACCOUNT/syn-2"),
		} {
			if _, err := fixture.charges.SaveConfirmed(txCtx, fixture.tenant, charge); err != nil {
				return err
			}
		}
		for _, statement := range []ports.StatementRecord{open, voided, next, elsewhere} {
			if _, err := fixture.statements.Save(txCtx, statement); err != nil {
				return err
			}
		}
		for index, seed := range []struct{ id, charge string }{
			{"adj-billed", "charge-billed"},
			{"adj-carried", "charge-billed"},
			{"adj-current", "charge-current"},
			{"adj-prior", "charge-billed"},
			{"adj-on-voided", "charge-voided"},
			{"adj-elsewhere", "charge-elsewhere"},
		} {
			at := unbilledFormedAt.Add(time.Duration(index) * time.Hour)
			if _, err := fixture.adjustments.Save(txCtx, ports.ChargeAdjustmentRecord{
				Key:        adjustmentKey(t, "tenant-1", seed.id),
				Adjustment: correctionAdjustment(t, seed.id, seed.charge, at),
				RecordedAt: at,
			}); err != nil {
				return err
			}
		}
		if _, err := fixture.adjustments.Save(txCtx, ports.ChargeAdjustmentRecord{
			Key: adjustmentKey(t, "tenant-1", "adj-included"), Adjustment: included, RecordedAt: unbilledFormedAt,
		}); err != nil {
			return err
		}
		_, err := fixture.inclusions.Save(txCtx, ports.InclusionRecord{
			Key:           ports.InclusionKey{TenantID: fixture.tenant, Inclusion: inclusion.Inclusion()},
			ContentDigest: "digest-inclusion-unbilled-1",
			Inclusion:     inclusion,
			RecordedAt:    inclusionAt,
		})
		return err
	})
	return fixture
}

// carryingStatement 造一张只有一行费用的已发布对账单，调整行照给。总额照行勾稽。publishedAfter 是发布时刻相对
// statementCutAt 的偏移：各张单的先后由它定。
func carryingStatement(
	t *testing.T,
	number, account, period, charge string,
	publishedAfter time.Duration,
	adjustments ...domain.StatementAdjustmentLine,
) ports.StatementRecord {
	t.Helper()
	total := int64(45600)
	for _, adjustment := range adjustments {
		if adjustment.Direction == domain.AdjustmentDebit {
			total += adjustment.AmountMinor
		} else {
			total -= adjustment.AmountMinor
		}
	}
	statement, err := domain.RehydratePublishedStatement(domain.RehydratePublishedStatementSpec{
		Number:      saValue(t, domain.NewStatementNumber, number),
		Account:     saValue(t, domain.NewSettlementAccountID, account),
		Period:      saValue(t, domain.NewBillingPeriodReference, period),
		Currency:    saValue(t, domain.NewCurrencyCode, "CNY"),
		Lines:       []domain.StatementLine{{Charge: saValue(t, domain.NewCustomerChargeID, charge), AmountMinor: 45600}},
		Adjustments: adjustments,
		TotalMinor:  total,
		PublishedAt: statementCutAt.Add(publishedAfter),
	})
	if err != nil {
		t.Fatalf("构造对账单 %s：%v", number, err)
	}
	return ports.StatementRecord{
		Key:           ports.StatementKey{TenantID: saTenant(t, "tenant-1"), Number: saValue(t, domain.NewStatementNumber, number)},
		ContentDigest: "digest-" + number,
		Statement:     statement,
		DraftVersion:  saValue(t, domain.NewStatementDraftVersion, "v-"+number),
		RecordedAt:    statementCutAt.Add(publishedAfter),
	}
}

// confirmedChargeOn 造一笔挂在指定结算账户上的已确认费用，CNY 45600。
func confirmedChargeOn(t *testing.T, id, account string) domain.CustomerCharge {
	t.Helper()
	charge, err := domain.FormCustomerCharge(domain.CustomerChargeSpec{
		ID:                 saValue(t, domain.NewCustomerChargeID, id),
		FeeItem:            saValue(t, domain.NewFeeItemReference, "BASE_FREIGHT"),
		Evaluation:         saValue(t, domain.NewSellEvaluationReference, "evaluation-sell/"+id),
		OriginalCurrency:   saValue(t, domain.NewCurrencyCode, "CNY"),
		OriginalMinor:      45600,
		SettlementCurrency: saValue(t, domain.NewCurrencyCode, "CNY"),
		SettlementMinor:    45600,
		Stage:              domain.ChargeEstimated,
		FormedAt:           chargeFormedAt,
	})
	if err != nil {
		t.Fatalf("构造预估费用：%v", err)
	}
	facts := saConfirmationFacts(t)
	facts.SettlementAccount = saValue(t, domain.NewSettlementAccountID, account)
	confirmed, err := charge.Confirm(facts, saValue(t, domain.NewConfirmationBasisReference, "DELIVERY_FINALIZED/"+id), chargeConfirmedAt)
	if err != nil {
		t.Fatalf("确认费用：%v", err)
	}
	return confirmed
}

func (fixture unbilledFixture) unbilled(t *testing.T, tenant domain.TenantID, account, currency string, includePrior bool, charges ...string) []string {
	t.Helper()
	ids := make([]domain.CustomerChargeID, 0, len(charges))
	for _, charge := range charges {
		ids = append(ids, saValue(t, domain.NewCustomerChargeID, charge))
	}
	records, err := fixture.adjustments.ListUnbilledForCutoff(t.Context(), tenant,
		saValue(t, domain.NewSettlementAccountID, account), saValue(t, domain.NewCurrencyCode, currency), ids, includePrior)
	if err != nil {
		t.Fatalf("列还没出账的调整：%v", err)
	}
	listed := make([]string, 0, len(records))
	for _, record := range records {
		listed = append(listed, record.Key.Adjustment.String())
	}
	return listed
}

// Covers: 挂在点名费用上、还没出账的调整总交回；挂在同一账户未作废对账单上的只在要求随单带走时交回，按形成先后。
// 已写进一张未作废对账单的调整行的（含已被后面的单带走的）、已记过后续纳入的不再交；那张单作废之后，上面的调整又算
// 没出账。别的账户、别的币种、别的租户看不见。
func TestUnbilledAdjustmentsAreListedOnceAndOnlyForTheirAccount(t *testing.T) {
	fixture := newUnbilledFixture(t)
	for _, tc := range []struct {
		name         string
		tenant       domain.TenantID
		account      string
		currency     string
		includePrior bool
		charges      []string
		want         []string
	}{
		{"只带本期费用上的", fixture.tenant, "ACCOUNT/syn-1", "CNY", false, []string{"charge-current"}, []string{"adj-current"}},
		{"连原账单上的一起带", fixture.tenant, "ACCOUNT/syn-1", "CNY", true, []string{"charge-current"}, []string{"adj-current", "adj-prior"}},
		{"本期没有带调整的费用", fixture.tenant, "ACCOUNT/syn-1", "CNY", true, nil, []string{"adj-prior"}},
		{"不点名也不带原账单的", fixture.tenant, "ACCOUNT/syn-1", "CNY", false, nil, []string{}},
		{"作废那张单上的费用重新点名", fixture.tenant, "ACCOUNT/syn-1", "CNY", false, []string{"charge-voided"}, []string{"adj-on-voided"}},
		{"作废那张单不算原账单", fixture.tenant, "ACCOUNT/syn-1", "CNY", true, nil, []string{"adj-prior"}},
		{"另一个账户", fixture.tenant, "ACCOUNT/syn-2", "CNY", true, nil, []string{"adj-elsewhere"}},
		{"另一个币种", fixture.tenant, "ACCOUNT/syn-1", "USD", true, []string{"charge-current"}, []string{}},
		{"另一个租户", saTenant(t, "tenant-2"), "ACCOUNT/syn-1", "CNY", true, []string{"charge-current"}, []string{}},
	} {
		if got := fixture.unbilled(t, tc.tenant, tc.account, tc.currency, tc.includePrior, tc.charges...); !slices.Equal(got, tc.want) {
			t.Fatalf("%s：交回 %v，want %v", tc.name, got, tc.want)
		}
	}
}

// Covers: 带调整的那张单作废，它带走的那笔调整的后续纳入随之撤回：撤回只在原行上记时刻与依据，重撤不改；撤回之后这笔
// 调整又算没出账，可以由后面的单再记一笔纳入——「一笔调整只纳入一次」只管没撤回的。没撤回时再纳一笔照旧被挡。写口在事务
// 外按框架合同拒。
func TestAWithdrawnInclusionFreesItsAdjustmentToBeCarriedAgain(t *testing.T) {
	fixture := newUnbilledFixture(t)
	key := ports.InclusionKey{TenantID: fixture.tenant, Inclusion: saValue(t, domain.NewInclusionReference, "inclusion-unbilled-1")}
	if _, err := fixture.inclusions.Withdraw(t.Context(), key, "STATEMENT_VOIDED/STMT-NEXT", statementVoidAt); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("事务外撤回 err = %v", err)
	}

	open, _, err := fixture.statements.FindByKey(t.Context(), ports.StatementKey{
		TenantID: fixture.tenant, Number: saValue(t, domain.NewStatementNumber, "STMT-OPEN"),
	})
	if err != nil {
		t.Fatal(err)
	}
	again := func(id, period string) ports.InclusionRecord {
		t.Helper()
		inclusion, err := domain.IncludeAdjustmentInSubsequentPeriod(open.Statement,
			correctionAdjustment(t, "adj-included", "charge-billed", unbilledFormedAt),
			saValue(t, domain.NewInclusionReference, id), saValue(t, domain.NewBillingPeriodReference, period), inclusionAt)
		if err != nil {
			t.Fatalf("形成纳入：%v", err)
		}
		return ports.InclusionRecord{
			Key:           ports.InclusionKey{TenantID: fixture.tenant, Inclusion: inclusion.Inclusion()},
			ContentDigest: "digest-" + id, Inclusion: inclusion, RecordedAt: inclusionAt,
		}
	}
	var blocked, reincluded ports.InclusionSaveOutcome
	var withdrawn, rewithdrawn bool
	fixture.within(func(txCtx context.Context) error {
		var err error
		if blocked, err = fixture.inclusions.Save(txCtx, again("inclusion-unbilled-2", "2026-11")); err != nil {
			return err
		}
		if withdrawn, err = fixture.inclusions.Withdraw(txCtx, key, "STATEMENT_VOIDED/STMT-NEXT", statementVoidAt); err != nil {
			return err
		}
		if rewithdrawn, err = fixture.inclusions.Withdraw(txCtx, key, "STATEMENT_VOIDED/OTHER", statementVoidAt.Add(time.Hour)); err != nil {
			return err
		}
		return nil
	})
	if blocked != ports.InclusionAlreadyRecorded || !withdrawn || rewithdrawn {
		t.Fatalf("没撤回时再纳 = %d，撤回 = %v，重撤 = %v；want 被挡、撤下、重撤不改", blocked, withdrawn, rewithdrawn)
	}
	if got := fixture.unbilled(t, fixture.tenant, "ACCOUNT/syn-1", "CNY", true); !slices.Equal(got, []string{"adj-included", "adj-prior"}) {
		t.Fatalf("撤回之后还没出账的调整 = %v，want adj-included 回来了", got)
	}
	fixture.within(func(txCtx context.Context) error {
		var err error
		reincluded, err = fixture.inclusions.Save(txCtx, again("inclusion-unbilled-3", "2026-11"))
		return err
	})
	if reincluded != ports.InclusionSaved {
		t.Fatalf("撤回之后再纳 = %d，want 记得下", reincluded)
	}
	if got := fixture.unbilled(t, fixture.tenant, "ACCOUNT/syn-1", "CNY", true); !slices.Equal(got, []string{"adj-prior"}) {
		t.Fatalf("再纳之后还没出账的调整 = %v", got)
	}
}

// Covers: 只有调整行、没有费用行的对账单存得下、读得回，净额可以为负；费用行与调整行都没有的被库约束拒。上一结算周期
// 已确认费用合计跳过这种单，仍取最近一张带费用行的。
func TestAnAdjustmentOnlyStatementRoundTripsAndIsNotAChargePeriod(t *testing.T) {
	fixture := newUnbilledFixture(t)
	credit := carryingStatement(t, "STMT-CREDIT", "ACCOUNT/syn-1", "2026-10", "charge-ignored", 48*time.Hour,
		domain.StatementAdjustmentLine{
			Adjustment: saValue(t, domain.NewChargeAdjustmentID, "adj-prior"),
			Charge:     saValue(t, domain.NewCustomerChargeID, "charge-billed"),
			Direction:  domain.AdjustmentCredit, AmountMinor: 900,
			OriginalStatement: saValue(t, domain.NewStatementNumber, "STMT-OPEN"),
		})
	statement, err := domain.RehydratePublishedStatement(domain.RehydratePublishedStatementSpec{
		Number: credit.Statement.Number(), Account: credit.Statement.Account(), Period: credit.Statement.Period(),
		Currency: credit.Statement.Currency(), Adjustments: credit.Statement.AdjustmentLines(), TotalMinor: -900,
		// 比夹具里各张单都晚：它是这一户最近的一张。
		PublishedAt: statementCutAt.Add(48 * time.Hour),
	})
	if err != nil {
		t.Fatalf("只有调整行的对账单：%v", err)
	}
	credit.Statement = statement
	fixture.within(func(txCtx context.Context) error {
		_, err := fixture.statements.Save(txCtx, credit)
		return err
	})
	found, present, err := fixture.statements.FindByKey(t.Context(), credit.Key)
	if err != nil || !present || found.Statement.TotalMinor() != -900 || len(found.Statement.Lines()) != 0 ||
		len(found.Statement.AdjustmentLines()) != 1 {
		t.Fatalf("读回 = %+v present=%v err=%v", found.Statement, present, err)
	}

	bases, err := adapter.NewCreditRatioBases(fixture.db)
	if err != nil {
		t.Fatal(err)
	}
	scope, err := domain.NewSettlementScope(saValue(t, domain.NewLegalEntityReference, "LEGAL-ENTITY/syn-1"),
		saValue(t, domain.NewSettlementAccountID, "ACCOUNT/syn-1"), saValue(t, domain.NewCurrencyCode, "CNY"))
	if err != nil {
		t.Fatal(err)
	}
	base, known, err := bases.LoadCreditRatioBase(t.Context(), fixture.tenant, scope, domain.PriorPeriodConfirmedChargesBase)
	if err != nil || !known || base != 45600 {
		t.Fatalf("上一周期已确认费用合计 = %d known=%v err=%v，want 45600（最近一张带费用行的单）", base, known, err)
	}

	err = fixture.db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		executor, err := fixture.db.RequireExecutor(txCtx)
		if err != nil {
			return err
		}
		_, err = executor.Exec(txCtx, `INSERT INTO settlement_accounting.customer_statement
			(tenant_id, statement_number, account_id, period_ref, currency, total_minor, content_digest, lines, adjustment_lines,
			 published_at, recorded_at)
			VALUES ('tenant-1', 'STMT-EMPTY', 'ACCOUNT/syn-1', '2026-12', 'CNY', 0, 'digest-empty', '[]'::jsonb, '[]'::jsonb, now(), now())`)
		return err
	})
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.ConstraintName != "customer_statement_lines_or_adjustments_present" {
		t.Fatalf("费用行与调整行都没有的单没被这条约束拒：%v", err)
	}
}

// Covers: 带着别的对账单上调整的那张单存得下、读得回，调整行上的原账单随它往返；按费用找原账单只认同一账户、未作废的那张。
func TestAStatementCarryingAPriorAdjustmentRoundTripsAndItsOriginalIsFound(t *testing.T) {
	fixture := newUnbilledFixture(t)
	account := saValue(t, domain.NewSettlementAccountID, "ACCOUNT/syn-1")

	next, found, err := fixture.statements.FindByKey(t.Context(), ports.StatementKey{
		TenantID: fixture.tenant, Number: saValue(t, domain.NewStatementNumber, "STMT-NEXT"),
	})
	if err != nil || !found {
		t.Fatalf("读回带着原账单调整的那张单：found=%v err=%v", found, err)
	}
	lines := next.Statement.AdjustmentLines()
	if len(lines) != 1 || lines[0].Charge.String() != "charge-billed" || lines[0].OriginalStatement.String() != "STMT-OPEN" ||
		next.Statement.TotalMinor() != 46500 {
		t.Fatalf("往返之后 = %+v / %d", lines, next.Statement.TotalMinor())
	}
	open, _, err := fixture.statements.FindByKey(t.Context(), ports.StatementKey{
		TenantID: fixture.tenant, Number: saValue(t, domain.NewStatementNumber, "STMT-OPEN"),
	})
	if err != nil || len(open.Statement.AdjustmentLines()) != 1 || open.Statement.AdjustmentLines()[0].OriginalStatement.String() != "" {
		t.Fatalf("单内的调整行往返后带上了原账单：%+v err=%v", open.Statement.AdjustmentLines(), err)
	}

	for _, tc := range []struct {
		name    string
		account domain.SettlementAccountID
		charge  string
		want    string
	}{
		{"未作废的原账单", account, "charge-billed", "STMT-OPEN"},
		{"已作废的不算", account, "charge-voided", ""},
		{"别的账户的不算", saValue(t, domain.NewSettlementAccountID, "ACCOUNT/syn-2"), "charge-billed", ""},
		{"没出过账的", account, "charge-current", ""},
	} {
		record, found, err := fixture.statements.FindOpenStatementCarryingCharge(t.Context(), fixture.tenant, tc.account,
			saValue(t, domain.NewCustomerChargeID, tc.charge))
		if err != nil {
			t.Fatalf("%s：%v", tc.name, err)
		}
		if got := record.Key.Number.String(); found != (tc.want != "") || got != tc.want {
			t.Fatalf("%s：找到 %q（%v），want %q", tc.name, got, found, tc.want)
		}
	}
}
