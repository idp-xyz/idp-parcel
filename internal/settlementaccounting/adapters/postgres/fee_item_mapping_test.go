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

func feeItemMapping(t *testing.T, code, feeItem, basis string) domain.FeeItemMapping {
	t.Helper()
	mapping, err := domain.NewFeeItemMapping(
		mustAccountRef(t, domain.NewPricingChargeCode, code),
		mustAccountRef(t, domain.NewFeeItemReference, feeItem),
		mustAccountRef(t, domain.NewFeeItemMappingBasisReference, basis),
	)
	if err != nil {
		t.Fatalf("mapping: %v", err)
	}
	return mapping
}

// Covers: ADR-0190 决定五的映射册半边——（租户、计价费用代码）键下只增不改：首登已登记、同一份重放、换费用项目或换依据
// 是冲突且不覆盖；读口按（租户、代码）作答，没登的代码与别的租户都答未找到——不拿价格代码顶费用项目。写口在事务外
// 被调按框架合同拒。
func TestAFeeItemMappingIsAppendOnlyPerTenantAndChargeCode(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	mappings, err := adapter.NewFeeItemMappings(db)
	if err != nil {
		t.Fatalf("构造费用项目映射册：%v", err)
	}
	tenant := settlementTenant(t)
	at := time.Date(2026, 10, 19, 9, 0, 0, 0, time.UTC)
	ctx := t.Context()
	save := func(tenant domain.TenantID, mapping domain.FeeItemMapping) (ports.CatalogueRegistrationEffect, error) {
		var effect ports.CatalogueRegistrationEffect
		err := db.Transactor().WithinTransaction(ctx, func(txCtx context.Context) error {
			var saveErr error
			effect, saveErr = mappings.SaveFeeItemMapping(txCtx, tenant, mapping, at)
			return saveErr
		})
		return effect, err
	}
	freight := feeItemMapping(t, "BASE_FREIGHT", "SYN-FEE-FREIGHT", "SYN-CLAUSE-1")
	code := freight.ChargeCode()

	if _, err := mappings.SaveFeeItemMapping(ctx, tenant, freight, at); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("事务外 SaveFeeItemMapping 应答 ErrTransactionRequired，实得：%v", err)
	}
	if _, found, err := mappings.FindFeeItemMapping(ctx, tenant, code); err != nil || found {
		t.Fatalf("事务外的写不该留下行：found = %t err = %v", found, err)
	}

	for _, step := range []struct {
		name    string
		mapping domain.FeeItemMapping
		effect  ports.CatalogueRegistrationEffect
	}{
		{"首登", freight, ports.CatalogueRegistered},
		{"重放", feeItemMapping(t, "BASE_FREIGHT", "SYN-FEE-FREIGHT", "SYN-CLAUSE-1"), ports.CatalogueReplay},
		{"换费用项目", feeItemMapping(t, "BASE_FREIGHT", "SYN-FEE-SURCHARGE", "SYN-CLAUSE-1"), ports.CatalogueConflict},
		{"换依据", feeItemMapping(t, "BASE_FREIGHT", "SYN-FEE-FREIGHT", "SYN-CLAUSE-2"), ports.CatalogueConflict},
		{"另一个代码", feeItemMapping(t, "FUEL_SURCHARGE", "SYN-FEE-FREIGHT", "SYN-CLAUSE-1"), ports.CatalogueRegistered},
	} {
		effect, err := save(tenant, step.mapping)
		if err != nil || effect != step.effect {
			t.Fatalf("%s：effect = %d err = %v，想要 %d", step.name, effect, err, step.effect)
		}
	}

	stored, found, err := mappings.FindFeeItemMapping(ctx, tenant, code)
	if err != nil || !found || !stored.SameRegistration(freight) {
		t.Fatalf("stored = %+v found = %t err = %v，想要首登那一份、冲突不覆盖", stored, found, err)
	}
	if _, found, err := mappings.FindFeeItemMapping(ctx, tenant, mustAccountRef(t, domain.NewPricingChargeCode, "REMOTE_AREA")); err != nil || found {
		t.Fatalf("没登的代码 found = %t err = %v，想要答未找到", found, err)
	}

	other := mustAccountRef(t, domain.NewTenantID, "tenant-b")
	if _, found, err := mappings.FindFeeItemMapping(ctx, other, code); err != nil || found {
		t.Fatalf("别的租户 found = %t err = %v，想要答未找到", found, err)
	}
	// 另一个租户同一个代码映到别的项目是另一行，不撞本租户那一行。
	if effect, err := save(other, feeItemMapping(t, "BASE_FREIGHT", "SYN-FEE-OTHER", "SYN-CLAUSE-9")); err != nil || effect != ports.CatalogueRegistered {
		t.Fatalf("别的租户同一个代码：effect = %d err = %v，想要已登记", effect, err)
	}
	if stored, _, _ := mappings.FindFeeItemMapping(ctx, tenant, code); !stored.SameRegistration(freight) {
		t.Fatalf("别的租户登记之后本租户读到 %+v", stored)
	}
}
