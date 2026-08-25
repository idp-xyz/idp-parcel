package postgres_test

import (
	"errors"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	adapter "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// PBC-08 行为面负向证据（bento-gate-reeval 票 02）：漏网写口在无事务上下文必须被
// RequireExecutor 拒绝。个别写口把「缺格即拒」的入参门放在守卫之前（依据种类非空、
// 截至时点非零），那几格传最小有效值让请求走到守卫；其余传零值。本包其余写口的
// 同款证据在各自聚合的测试文件里，这里只补漏网的。
func TestRemainingSettlementWritesRefuseToRunOutsideATransaction(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	ctx := t.Context()
	asOf := time.Date(2026, 8, 25, 9, 0, 0, 0, time.UTC)

	applicability, err := adapter.NewAllocationRuleApplicability(db)
	if err != nil {
		t.Fatalf("构造分摊规则适用登记：%v", err)
	}
	if _, err := applicability.Register(ctx, domain.TenantID{}, domain.AllocationSourceReference{}, domain.AllocationRuleVersionReference{}, asOf); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务登记分摊规则适用应返回 ErrTransactionRequired，实得：%v", err)
	}

	conditions, err := adapter.NewChargeConfirmationConditions(db)
	if err != nil {
		t.Fatalf("构造确认条件库：%v", err)
	}
	if _, err := conditions.RegisterCondition(ctx, domain.TenantID{}, domain.FeeItemReference{}, "EVIDENCE", asOf); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务登记确认条件应返回 ErrTransactionRequired，实得：%v", err)
	}
	if _, err := conditions.RecordBasis(ctx, domain.TenantID{}, domain.CustomerChargeID{}, domain.FeeItemReference{}, "EVIDENCE", domain.ConfirmationBasisReference{}, asOf); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务登记确认依据应返回 ErrTransactionRequired，实得：%v", err)
	}

	ledgers, err := adapter.NewFreezeLedgers(db)
	if err != nil {
		t.Fatalf("构造冻结账本：%v", err)
	}
	balances, err := adapter.NewOperationalBalances(db, ledgers)
	if err != nil {
		t.Fatalf("构造运营余额库：%v", err)
	}
	if _, err := balances.RecordBalance(ctx, domain.TenantID{}, domain.SettlementScope{}, adapter.BalancePosting{AsOf: asOf}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务登记余额应返回 ErrTransactionRequired，实得：%v", err)
	}

	exposures, err := adapter.NewCreditExposureLedgers(db)
	if err != nil {
		t.Fatalf("构造信用暴露账本：%v", err)
	}
	standings, err := adapter.NewCreditStandings(db, exposures)
	if err != nil {
		t.Fatalf("构造信用状况库：%v", err)
	}
	if _, err := standings.RecordStanding(ctx, domain.TenantID{}, domain.SettlementScope{}, adapter.CreditPosition{AsOf: asOf}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务登记信用状况应返回 ErrTransactionRequired，实得：%v", err)
	}

	disputes, err := adapter.NewStatementDisputes(db)
	if err != nil {
		t.Fatalf("构造争议库：%v", err)
	}
	if _, err := disputes.Replace(ctx, ports.DisputeRecord{}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务改写争议应返回 ErrTransactionRequired，实得：%v", err)
	}

	costs, err := adapter.NewSupplierExpectedCosts(db)
	if err != nil {
		t.Fatalf("构造供应商预计成本库：%v", err)
	}
	if _, err := costs.Save(ctx, domain.TenantID{}, domain.SupplierExpectedCost{}, asOf); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务保存预计成本应返回 ErrTransactionRequired，实得：%v", err)
	}
}
