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

func sellBillingMomentAdoption(t *testing.T, account string, moment domain.ParcelBillingMoment, scope string) domain.SellBillingMomentAdoption {
	t.Helper()
	adoption, err := domain.NewSellBillingMomentAdoption(
		mustAccountRef(t, domain.NewSettlementAccountID, account),
		moment,
		mustAccountRef(t, domain.NewEstimatePricingScope, scope),
	)
	if err != nil {
		t.Fatalf("adoption: %v", err)
	}
	return adoption
}

// Covers: ADR-0190 决定三——SELL 计费时点采用册是（租户、结算账户）键下的不可变元组：首登已登记、同一份重放、换时点或换
// 计价范围是冲突且不覆盖；指向的账户不在账户册上由外键拒、行不落；读口按（租户、账户）作答，没登的账户与别的租户都答
// false（发起编排据此答未配置）。写口在事务外被调按框架合同拒。
func TestASellBillingMomentAdoptionIsAnImmutableRowOnARegisteredAccount(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	accounts, err := adapter.NewSettlementAccounts(db)
	if err != nil {
		t.Fatalf("构造结算账户登记册：%v", err)
	}
	adoptions, err := adapter.NewSellBillingMomentAdoptions(db)
	if err != nil {
		t.Fatalf("构造 SELL 计费时点采用册：%v", err)
	}
	tenant := settlementTenant(t)
	at := time.Date(2026, 10, 19, 9, 0, 0, 0, time.UTC)
	ctx := t.Context()
	save := func(adoption domain.SellBillingMomentAdoption) (ports.CatalogueRegistrationEffect, error) {
		var effect ports.CatalogueRegistrationEffect
		err := db.Transactor().WithinTransaction(ctx, func(txCtx context.Context) error {
			var saveErr error
			effect, saveErr = adoptions.SaveSellBillingMomentAdoption(txCtx, tenant, adoption, at)
			return saveErr
		})
		return effect, err
	}
	account := mustAccountRef(t, domain.NewSettlementAccountID, "ACCT-1")
	intake := sellBillingMomentAdoption(t, "ACCT-1", domain.BillingMomentNetworkIntakeAdopted, "SCOPE-SELL-1")

	if _, err := adoptions.SaveSellBillingMomentAdoption(ctx, tenant, intake, at); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("事务外 SaveSellBillingMomentAdoption 应答 ErrTransactionRequired，实得：%v", err)
	}
	if _, err := save(intake); !errors.Is(err, domain.ErrCatalogueTargetMissing) {
		t.Fatalf("账户不在册上 err = %v，想要 ErrCatalogueTargetMissing", err)
	}
	if _, found, err := adoptions.FindSellBillingMomentAdoption(ctx, tenant, account); err != nil || found {
		t.Fatalf("账户不在册上之后 found = %t err = %v，想要一行都没落", found, err)
	}

	if err := db.Transactor().WithinTransaction(ctx, func(txCtx context.Context) error {
		_, saveErr := accounts.Save(txCtx, tenant, settlementAccountOutsideATransaction(t), at)
		return saveErr
	}); err != nil {
		t.Fatalf("登记账户：%v", err)
	}

	for _, step := range []struct {
		name     string
		adoption domain.SellBillingMomentAdoption
		effect   ports.CatalogueRegistrationEffect
	}{
		{"首登", intake, ports.CatalogueRegistered},
		{"重放", sellBillingMomentAdoption(t, "ACCT-1", domain.BillingMomentNetworkIntakeAdopted, "SCOPE-SELL-1"), ports.CatalogueReplay},
		{"换时点", sellBillingMomentAdoption(t, "ACCT-1", domain.BillingMomentEffectiveDeliveryFinalized, "SCOPE-SELL-1"), ports.CatalogueConflict},
		{"换计价范围", sellBillingMomentAdoption(t, "ACCT-1", domain.BillingMomentNetworkIntakeAdopted, "SCOPE-SELL-2"), ports.CatalogueConflict},
	} {
		effect, err := save(step.adoption)
		if err != nil || effect != step.effect {
			t.Fatalf("%s：effect = %d err = %v，想要 %d", step.name, effect, err, step.effect)
		}
	}

	stored, found, err := adoptions.FindSellBillingMomentAdoption(ctx, tenant, account)
	if err != nil || !found || !stored.SameRegistration(intake) {
		t.Fatalf("stored = %+v found = %t err = %v，想要首登那一份、冲突不覆盖", stored, found, err)
	}
	if _, found, err := adoptions.FindSellBillingMomentAdoption(ctx, tenant, mustAccountRef(t, domain.NewSettlementAccountID, "ACCT-9")); err != nil || found {
		t.Fatalf("没登的账户 found = %t err = %v，想要答 false", found, err)
	}
	other := mustAccountRef(t, domain.NewTenantID, "tenant-b")
	if _, found, err := adoptions.FindSellBillingMomentAdoption(ctx, other, account); err != nil || found {
		t.Fatalf("别的租户 found = %t err = %v，想要答 false", found, err)
	}
}
