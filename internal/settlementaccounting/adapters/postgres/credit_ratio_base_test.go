package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	adapter "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// 本文件对真实 PostgreSQL 16 证比例基数的取数路径（ADR-0129 决定二）：入账余额读运营余额登记、上一结算周期
// 已确认费用合计读最近一张已发布对账单的费用行之和；尚无事实答 found=false 而不是 0；作用域互不可见；
// 集外基数上抛。

type ratioBaseFixture struct {
	bases      *adapter.CreditRatioBases
	balances   *adapter.OperationalBalances
	statements *adapter.CustomerStatements
	transactor bentoapp.Transactor
}

func newRatioBases(t *testing.T) ratioBaseFixture {
	t.Helper()

	db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	bases, err := adapter.NewCreditRatioBases(db)
	if err != nil {
		t.Fatalf("构造基数视图：%v", err)
	}
	freezes, err := adapter.NewFreezeLedgers(db)
	if err != nil {
		t.Fatalf("构造冻结册仓储：%v", err)
	}
	balances, err := adapter.NewOperationalBalances(db, freezes)
	if err != nil {
		t.Fatalf("构造余额视图：%v", err)
	}
	statements, err := adapter.NewCustomerStatements(db)
	if err != nil {
		t.Fatalf("构造对账单库：%v", err)
	}
	return ratioBaseFixture{bases: bases, balances: balances, statements: statements, transactor: db.Transactor()}
}

func (fixture ratioBaseFixture) recordPosted(t *testing.T, tenant domain.TenantID, scope domain.SettlementScope, posted int64) {
	t.Helper()
	saWithin(t, fixture.transactor, t.Context(), func(txCtx context.Context) error {
		outcome, err := fixture.balances.RecordBalance(txCtx, tenant, scope, adapter.BalancePosting{
			PostedMinor: posted, AsOf: positionAsOf,
		})
		if err != nil {
			return err
		}
		if outcome != adapter.PositionRecorded {
			t.Errorf("登记余额：outcome = %s, want RECORDED", outcome)
		}
		return nil
	})
}

// publish 发布一张对账单：费用行两条（1000 + 500）、调整行一借一贷（+200 −300），总额 1400——费用行之和 1500 与
// 总额 1400 不同，正好分得开「取的是费用行」还是「取的是净总额」。
func (fixture ratioBaseFixture) publish(
	t *testing.T,
	tenant, number, account, currency string,
	publishedAt time.Time,
	lineMinors ...int64,
) ports.StatementRecord {
	t.Helper()
	lines := make([]domain.StatementLine, 0, len(lineMinors))
	total := int64(0)
	for index, minor := range lineMinors {
		lines = append(lines, domain.StatementLine{
			Charge:      saValue(t, domain.NewCustomerChargeID, "charge-"+number+"-"+string(rune('a'+index))),
			AmountMinor: minor,
		})
		total += minor
	}
	statement, err := domain.RehydratePublishedStatement(domain.RehydratePublishedStatementSpec{
		Number:   saValue(t, domain.NewStatementNumber, number),
		Account:  saValue(t, domain.NewSettlementAccountID, account),
		Period:   saValue(t, domain.NewBillingPeriodReference, "period-"+number),
		Currency: saValue(t, domain.NewCurrencyCode, currency),
		Lines:    lines,
		Adjustments: []domain.StatementAdjustmentLine{
			{
				Adjustment:  saValue(t, domain.NewChargeAdjustmentID, "adj-"+number+"-debit"),
				Charge:      lines[0].Charge,
				Direction:   domain.AdjustmentDebit,
				AmountMinor: 200,
			},
			{
				Adjustment:  saValue(t, domain.NewChargeAdjustmentID, "adj-"+number+"-credit"),
				Charge:      lines[0].Charge,
				Direction:   domain.AdjustmentCredit,
				AmountMinor: 300,
			},
		},
		TotalMinor:  total - 100,
		PublishedAt: publishedAt,
	})
	if err != nil {
		t.Fatalf("构造对账单 %s：%v", number, err)
	}
	record := ports.StatementRecord{
		Key:           ports.StatementKey{TenantID: saTenant(t, tenant), Number: saValue(t, domain.NewStatementNumber, number)},
		ContentDigest: "digest-" + number,
		Statement:     statement,
		RecordedAt:    publishedAt,
	}
	saWithin(t, fixture.transactor, t.Context(), func(txCtx context.Context) error {
		outcome, err := fixture.statements.Save(txCtx, record)
		if err != nil {
			return err
		}
		if outcome != ports.StatementSaved {
			t.Errorf("发布对账单 %s：outcome = %d, want SAVED", number, outcome)
		}
		return nil
	})
	return record
}

func (fixture ratioBaseFixture) void(t *testing.T, record ports.StatementRecord, voidedAt time.Time) {
	t.Helper()
	voided, err := record.Statement.Void(saValue(t, domain.NewStatementVoidBasisReference, "void-"+record.Key.Number.String()), voidedAt)
	if err != nil {
		t.Fatalf("作废对账单：%v", err)
	}
	record.Statement = voided
	saWithin(t, fixture.transactor, t.Context(), func(txCtx context.Context) error {
		replaced, err := fixture.statements.Replace(txCtx, record)
		if err != nil {
			return err
		}
		if !replaced {
			t.Error("作废没有写进去")
		}
		return nil
	})
}

func (fixture ratioBaseFixture) load(t *testing.T, tenant string, scope domain.SettlementScope, base domain.CreditRatioBase) (int64, bool) {
	t.Helper()
	minor, established, err := fixture.bases.LoadCreditRatioBase(t.Context(), saTenant(t, tenant), scope, base)
	if err != nil {
		t.Fatalf("取基数 %s：%v", base, err)
	}
	return minor, established
}

// Covers: ADR-0129 决定二 POSTED_BALANCE——取值就是运营余额登记面的 posted_minor，键取作用域全部四维；未登记
// 答尚无事实而不是 0；负入账余额如实交负数（折不折由领域说）；别的账户的登记不进本作用域。
func TestPostedBalanceBaseReadsTheRegisteredPostedMinorOrNothing(t *testing.T) {
	fixture := newRatioBases(t)
	tenant := saTenant(t, "tenant-1")
	scope := saScope(t)

	if minor, established := fixture.load(t, "tenant-1", scope, domain.PostedBalanceBase); established || minor != 0 {
		t.Fatalf("未登记余额的作用域答了 (%d, %v)，want (0, false)——尚无事实不是 0", minor, established)
	}

	fixture.recordPosted(t, tenant, scope, 250_000)
	fixture.recordPosted(t, tenant, saScopeWithAccount(t, "account-2"), 9)
	if minor, established := fixture.load(t, "tenant-1", scope, domain.PostedBalanceBase); !established || minor != 250_000 {
		t.Fatalf("入账余额 = (%d, %v), want (250000, true)", minor, established)
	}
	if minor, established := fixture.load(t, "tenant-2", scope, domain.PostedBalanceBase); established || minor != 0 {
		t.Fatalf("他租户读到了本租户的余额：(%d, %v)", minor, established)
	}

	negative := saScopeWithAccount(t, "account-in-debit")
	fixture.recordPosted(t, tenant, negative, -40_000)
	if minor, established := fixture.load(t, "tenant-1", negative, domain.PostedBalanceBase); !established || minor != -40_000 {
		t.Fatalf("负入账余额 = (%d, %v), want (-40000, true)——账本如实交数，折成 0 是领域的事", minor, established)
	}
}

// Covers: ADR-0129 决定二 PRIOR_PERIOD_CONFIRMED_CHARGES——取本账户最近一张已发布对账单的**费用行**之和（不是含调整的
// 净总额）；从未发布过答尚无事实；后发布的一张替代前一张成为「上一周期」；最近一张作废且无替代时答尚无事实，
// 不退回更早一个周期的数；替代单发布后又有了；别的账户、别的币种、他租户的对账单不进本作用域。
func TestPriorPeriodConfirmedChargesBaseReadsTheLatestPublishedStatementLines(t *testing.T) {
	fixture := newRatioBases(t)
	scope := saScopeWithAccount(t, "account-1")
	usd := "USD"

	if minor, established := fixture.load(t, "tenant-1", scope, domain.PriorPeriodConfirmedChargesBase); established || minor != 0 {
		t.Fatalf("从未发布过对账单却答了 (%d, %v)，want (0, false)", minor, established)
	}

	first := fixture.publish(t, "tenant-1", "STMT-1", "account-1", usd, statementCutAt.Add(time.Hour), 1_000, 500)
	if minor, established := fixture.load(t, "tenant-1", scope, domain.PriorPeriodConfirmedChargesBase); !established || minor != 1_500 {
		t.Fatalf("上一周期费用合计 = (%d, %v), want (1500, true)——费用行之和，不是含调整的净总额 1400", minor, established)
	}

	// 噪音：别的账户、别的币种、他租户各发一张，都不该进本作用域的分母。
	fixture.publish(t, "tenant-1", "STMT-OTHER-ACCOUNT", "account-2", usd, statementCutAt.Add(48*time.Hour), 99_000)
	fixture.publish(t, "tenant-1", "STMT-OTHER-CURRENCY", "account-1", "CNY", statementCutAt.Add(48*time.Hour), 77_000)
	fixture.publish(t, "tenant-2", "STMT-OTHER-TENANT", "account-1", usd, statementCutAt.Add(48*time.Hour), 55_000)
	if minor, established := fixture.load(t, "tenant-1", scope, domain.PriorPeriodConfirmedChargesBase); !established || minor != 1_500 {
		t.Fatalf("别的账户 / 币种 / 租户的对账单进了分母：(%d, %v)", minor, established)
	}

	second := fixture.publish(t, "tenant-1", "STMT-2", "account-1", usd, statementCutAt.Add(24*time.Hour), 3_000, 2_000, 1_000)
	if minor, established := fixture.load(t, "tenant-1", scope, domain.PriorPeriodConfirmedChargesBase); !established || minor != 6_000 {
		t.Fatalf("后发布的一张没有成为「上一周期」：(%d, %v), want (6000, true)", minor, established)
	}

	fixture.void(t, second, statementCutAt.Add(25*time.Hour))
	if minor, established := fixture.load(t, "tenant-1", scope, domain.PriorPeriodConfirmedChargesBase); established || minor != 0 {
		t.Fatalf("最近一张已作废且无替代却答了 (%d, %v)——不该退回更早一张（%d）当分母", minor, established, 1_500)
	}
	_ = first

	fixture.publish(t, "tenant-1", "STMT-2R", "account-1", usd, statementCutAt.Add(26*time.Hour), 4_000)
	if minor, established := fixture.load(t, "tenant-1", scope, domain.PriorPeriodConfirmedChargesBase); !established || minor != 4_000 {
		t.Fatalf("替代单发布后 = (%d, %v), want (4000, true)", minor, established)
	}
}

// Covers: 封闭集在领域、取数路径在适配器：集外基数（含未声明）没有一条路，上抛而不是猜一格。
func TestABaseOutsideTheSetHasNoDataPath(t *testing.T) {
	fixture := newRatioBases(t)
	for _, base := range []domain.CreditRatioBase{domain.CreditRatioBaseUndeclared, domain.CreditRatioBase(250)} {
		_, _, err := fixture.bases.LoadCreditRatioBase(t.Context(), saTenant(t, "tenant-1"), saScope(t), base)
		if !errors.Is(err, domain.ErrInvalidCreditBasis) {
			t.Fatalf("base %d: err = %v, want ErrInvalidCreditBasis", uint8(base), err)
		}
	}
}
