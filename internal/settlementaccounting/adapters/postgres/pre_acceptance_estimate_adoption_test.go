package postgres_test

import (
	"context"
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

func estimateAdoption(t *testing.T, account, scope string) domain.PreAcceptanceEstimateAdoption {
	t.Helper()
	adoption, err := domain.NewPreAcceptanceEstimateAdoption(
		mustAccountRef(t, domain.NewSettlementAccountID, account),
		domain.PerParcelSellTrialSum,
		mustAccountRef(t, domain.NewEstimatePricingScope, scope),
	)
	if err != nil {
		t.Fatalf("adoption: %v", err)
	}
	return adoption
}

// Covers: ADR-0179 决定五（照 ADR-0158）——采用册是（租户、结算账户）键下的不可变元组：首登已登记、同一份重放、换计价范围是冲突且
// 不覆盖；指向的账户不在账户册上由外键拒、行不落；读口按（租户、账户）作答，册上没有这一户答 false。写口在事务外被调按框架合同拒。
func TestAPreAcceptanceEstimateAdoptionIsAnImmutableRowOnARegisteredAccount(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	accounts, err := adapter.NewSettlementAccounts(db)
	if err != nil {
		t.Fatalf("构造结算账户登记册：%v", err)
	}
	adoptions, err := adapter.NewPreAcceptanceEstimateAdoptions(db)
	if err != nil {
		t.Fatalf("构造采用册：%v", err)
	}
	tenant := settlementTenant(t)
	at := time.Date(2026, 10, 13, 9, 0, 0, 0, time.UTC)
	ctx := t.Context()
	save := func(adoption domain.PreAcceptanceEstimateAdoption) (ports.PreAcceptanceEstimateAdoptionEffect, error) {
		var effect ports.PreAcceptanceEstimateAdoptionEffect
		err := db.Transactor().WithinTransaction(ctx, func(txCtx context.Context) error {
			var saveErr error
			effect, saveErr = adoptions.SaveAdoption(txCtx, tenant, adoption, at)
			return saveErr
		})
		return effect, err
	}
	account := mustAccountRef(t, domain.NewSettlementAccountID, "ACCT-1")

	if _, err := adoptions.SaveAdoption(ctx, tenant, estimateAdoption(t, "ACCT-1", "SCOPE-SELL-1"), at); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("事务外 SaveAdoption 应答 ErrTransactionRequired，实得：%v", err)
	}
	if _, err := save(estimateAdoption(t, "ACCT-1", "SCOPE-SELL-1")); !errors.Is(err, domain.ErrCatalogueTargetMissing) {
		t.Fatalf("账户不在册上 err = %v, 想要 ErrCatalogueTargetMissing", err)
	}
	if _, found, err := adoptions.FindAdoption(ctx, tenant, account); err != nil || found {
		t.Fatalf("账户不在册上之后 found = %t err = %v, 想要一行都没落", found, err)
	}

	if err := db.Transactor().WithinTransaction(ctx, func(txCtx context.Context) error {
		_, saveErr := accounts.Save(txCtx, tenant, settlementAccountOutsideATransaction(t), at)
		return saveErr
	}); err != nil {
		t.Fatalf("登记账户：%v", err)
	}

	for _, step := range []struct {
		name   string
		scope  string
		effect ports.PreAcceptanceEstimateAdoptionEffect
	}{
		{"首登", "SCOPE-SELL-1", ports.PreAcceptanceEstimateAdoptionRegistered},
		{"重放", "SCOPE-SELL-1", ports.PreAcceptanceEstimateAdoptionReplay},
		{"换计价范围", "SCOPE-SELL-2", ports.PreAcceptanceEstimateAdoptionConflict},
	} {
		effect, err := save(estimateAdoption(t, "ACCT-1", step.scope))
		if err != nil || effect != step.effect {
			t.Fatalf("%s：effect = %d err = %v, 想要 %d", step.name, effect, err, step.effect)
		}
	}

	stored, found, err := adoptions.FindAdoption(ctx, tenant, account)
	if err != nil || !found || !stored.SameRegistration(estimateAdoption(t, "ACCT-1", "SCOPE-SELL-1")) {
		t.Fatalf("stored = %+v found = %t err = %v, 想要首登那一份、冲突不覆盖", stored, found, err)
	}
	if _, found, err := adoptions.FindAdoption(ctx, tenant, mustAccountRef(t, domain.NewSettlementAccountID, "ACCT-9")); err != nil || found {
		t.Fatalf("没登的账户 found = %t err = %v, 想要答 false", found, err)
	}
	other := mustAccountRef(t, domain.NewTenantID, "tenant-b")
	if _, found, err := adoptions.FindAdoption(ctx, other, account); err != nil || found {
		t.Fatalf("别的租户 found = %t err = %v, 想要答 false", found, err)
	}
}
