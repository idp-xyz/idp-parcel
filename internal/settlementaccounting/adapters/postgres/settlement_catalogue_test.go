package postgres_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	adapter "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

func TestSettlementCataloguesRefuseToRunOutsideATransaction(t *testing.T) {
	catalogues := newCatalogues(t)
	tenant := registerTenant(t)
	at := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	authority := catalogueAuthority(t)
	account := cataloguePayable(t, "ACCT-1")
	rule := catalogueRule(t, "rule-1/v1")
	facts := catalogueFacts(t, "CHARGE-1", "ACCT-1")

	if _, err := catalogues.SaveSupplierAuditAuthority(t.Context(), tenant, authority, at); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("事务外审核授权 Save 应答 ErrTransactionRequired，实得：%v", err)
	}
	if _, err := catalogues.SaveSupplierPayableAccount(t.Context(), tenant, account, at); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("事务外应付账户 Save 应答 ErrTransactionRequired，实得：%v", err)
	}
	if _, err := catalogues.SaveClaimAmountRule(t.Context(), tenant, rule, at); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("事务外金额规则 Save 应答 ErrTransactionRequired，实得：%v", err)
	}
	if _, err := catalogues.SaveChargeConfirmationFacts(t.Context(), tenant, facts, at); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("事务外确认事实 Save 应答 ErrTransactionRequired，实得：%v", err)
	}
}

func TestEmptyCataloguesStayUnconfigured(t *testing.T) {
	catalogues := newCatalogues(t)
	tenant := registerTenant(t)
	ctx := t.Context()

	if _, found, err := catalogues.LoadSupplierAuditAuthority(ctx, tenant, catalogueSupplier(t), catalogueEntity(t)); err != nil || found {
		t.Fatalf("空授权册 found=%v err=%v", found, err)
	}
	if _, found, err := catalogues.LoadSupplierPayableAccount(ctx, tenant, catalogueSupplier(t), catalogueEntity(t), catalogueCurrency(t)); err != nil || found {
		t.Fatalf("空应付账户册 found=%v err=%v", found, err)
	}
	if _, found, err := catalogues.LoadClaimAmountRule(ctx, tenant, catalogueResponsibility(t)); err != nil || found {
		t.Fatalf("空金额规则册 found=%v err=%v", found, err)
	}
	if _, found, err := catalogues.LoadConfirmedChargeFacts(ctx, tenant, catalogueCharge(t, "CHARGE-1")); err != nil || found {
		t.Fatalf("空确认事实册 found=%v err=%v", found, err)
	}
}

func TestRegisteredCataloguesAnswerAndADifferentBodyConflicts(t *testing.T) {
	db := catalogueDB(t)
	catalogues := mustCatalogues(t, db)
	tenant := registerTenant(t)
	at := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	seedSettlementAccount(t, db, tenant, "ACCT-1")

	err := db.Transactor().WithinTransaction(t.Context(), func(ctx context.Context) error {
		if effect, err := catalogues.SaveSupplierAuditAuthority(ctx, tenant, catalogueAuthority(t), at); err != nil || effect != ports.CatalogueRegistered {
			return errOrEffect(err, effect, ports.CatalogueRegistered)
		}
		if effect, err := catalogues.SaveSupplierAuditAuthority(ctx, tenant, catalogueAuthority(t), at); err != nil || effect != ports.CatalogueReplay {
			return errOrEffect(err, effect, ports.CatalogueReplay)
		}
		other := catalogueAuthorityAuditor(t, "AUDITOR-2")
		if effect, err := catalogues.SaveSupplierAuditAuthority(ctx, tenant, other, at); err != nil || effect != ports.CatalogueConflict {
			return errOrEffect(err, effect, ports.CatalogueConflict)
		}
		if effect, err := catalogues.SaveSupplierPayableAccount(ctx, tenant, cataloguePayable(t, "ACCT-1"), at); err != nil || effect != ports.CatalogueRegistered {
			return errOrEffect(err, effect, ports.CatalogueRegistered)
		}
		if effect, err := catalogues.SaveClaimAmountRule(ctx, tenant, catalogueRule(t, "rule-1/v1"), at); err != nil || effect != ports.CatalogueRegistered {
			return errOrEffect(err, effect, ports.CatalogueRegistered)
		}
		if effect, err := catalogues.SaveClaimAmountRule(ctx, tenant, catalogueRule(t, "rule-1/v2"), at); err != nil || effect != ports.CatalogueConflict {
			return errOrEffect(err, effect, ports.CatalogueConflict)
		}
		if effect, err := catalogues.SaveChargeConfirmationFacts(ctx, tenant, catalogueFacts(t, "CHARGE-1", "ACCT-1"), at); err != nil || effect != ports.CatalogueRegistered {
			return errOrEffect(err, effect, ports.CatalogueRegistered)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	auditor, found, err := catalogues.LoadSupplierAuditAuthority(t.Context(), tenant, catalogueSupplier(t), catalogueEntity(t))
	if err != nil || !found || auditor.String() != "AUDITOR-1" {
		t.Fatalf("auditor=%s found=%v err=%v", auditor, found, err)
	}
	account, found, err := catalogues.LoadSupplierPayableAccount(t.Context(), tenant, catalogueSupplier(t), catalogueEntity(t), catalogueCurrency(t))
	if err != nil || !found || account.String() != "ACCT-1" {
		t.Fatalf("account=%s found=%v err=%v", account, found, err)
	}
	rule, found, err := catalogues.LoadClaimAmountRule(t.Context(), tenant, catalogueResponsibility(t))
	if err != nil || !found || rule.String() != "rule-1/v1" {
		t.Fatalf("rule=%s found=%v err=%v", rule, found, err)
	}
	facts, found, err := catalogues.LoadConfirmedChargeFacts(t.Context(), tenant, catalogueCharge(t, "CHARGE-1"))
	if err != nil || !found || facts.SettlementAccount.String() != "ACCT-1" || facts.Direction != domain.ChargeReceivable {
		t.Fatalf("facts found=%v err=%v account=%s", found, err, facts.SettlementAccount)
	}
}

func TestAPayableMappingDoesNotInventAnAccount(t *testing.T) {
	db := catalogueDB(t)
	catalogues := mustCatalogues(t, db)
	err := db.Transactor().WithinTransaction(t.Context(), func(ctx context.Context) error {
		_, err := catalogues.SaveSupplierPayableAccount(ctx, registerTenant(t), cataloguePayable(t, "ACCT-MISSING"), time.Now())
		return err
	})
	if !errors.Is(err, domain.ErrCatalogueTargetMissing) {
		t.Fatalf("没有账户行仍登记了查问：%v", err)
	}
	_, found, err := catalogues.LoadSupplierPayableAccount(t.Context(), registerTenant(t), catalogueSupplier(t), catalogueEntity(t), catalogueCurrency(t))
	if err != nil || found {
		t.Fatalf("缺失账户被读成已登记 found=%v err=%v", found, err)
	}
}

func errOrEffect(err error, got, want ports.CatalogueRegistrationEffect) error {
	if err != nil {
		return err
	}
	return fmt.Errorf("effect = %d, want %d", got, want)
}

func newCatalogues(t *testing.T) *adapter.SettlementCatalogues {
	t.Helper()
	return mustCatalogues(t, catalogueDB(t))
}

func catalogueDB(t *testing.T) *bentopg.DB {
	t.Helper()
	db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("db: %v", err)
	}
	return db
}

func mustCatalogues(t *testing.T, db *bentopg.DB) *adapter.SettlementCatalogues {
	t.Helper()
	catalogues, err := adapter.NewSettlementCatalogues(db)
	if err != nil {
		t.Fatal(err)
	}
	return catalogues
}

func seedSettlementAccount(t *testing.T, db *bentopg.DB, tenant domain.TenantID, accountID string) {
	t.Helper()
	accounts, err := adapter.NewSettlementAccounts(db)
	if err != nil {
		t.Fatal(err)
	}
	key, err := domain.NewSettlementAccountKey(
		catalogueEntity(t),
		catalogueRef(t, domain.NewSettlementCounterpartyReference, "CP-1"),
		domain.ChargePayable,
		catalogueCurrency(t),
		catalogueRef(t, domain.NewSettlementPolicyReference, "POL-1"),
	)
	if err != nil {
		t.Fatal(err)
	}
	account, err := domain.NewSettlementAccount(
		catalogueRef(t, domain.NewSettlementAccountID, accountID),
		key,
		domain.SettlementCounterpartyReference{},
		false,
		catalogueRef(t, domain.NewResponsibilityBasis, "CONTRACT-1"),
	)
	if err != nil {
		t.Fatal(err)
	}
	err = db.Transactor().WithinTransaction(t.Context(), func(ctx context.Context) error {
		_, err := accounts.Save(ctx, tenant, account, time.Now().UTC())
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

func registerTenant(t *testing.T) domain.TenantID {
	t.Helper()
	return catalogueRef(t, domain.NewTenantID, "tenant-a")
}

func catalogueSupplier(t *testing.T) domain.SupplierPartyReference {
	t.Helper()
	return catalogueRef(t, domain.NewSupplierPartyReference, "SUP-1")
}

func catalogueEntity(t *testing.T) domain.LegalEntityReference {
	t.Helper()
	return catalogueRef(t, domain.NewLegalEntityReference, "LE-1")
}

func catalogueCurrency(t *testing.T) domain.CurrencyCode {
	t.Helper()
	return catalogueRef(t, domain.NewCurrencyCode, "CNY")
}

func catalogueResponsibility(t *testing.T) domain.ResponsibilityConclusionReference {
	t.Helper()
	return catalogueRef(t, domain.NewResponsibilityConclusionReference, "RESP-1")
}

func catalogueCharge(t *testing.T, id string) domain.CustomerChargeID {
	t.Helper()
	return catalogueRef(t, domain.NewCustomerChargeID, id)
}

func catalogueAuthority(t *testing.T) domain.SupplierAuditAuthorityRegistration {
	t.Helper()
	return catalogueAuthorityAuditor(t, "AUDITOR-1")
}

func catalogueAuthorityAuditor(t *testing.T, auditor string) domain.SupplierAuditAuthorityRegistration {
	t.Helper()
	registration, err := domain.NewSupplierAuditAuthorityRegistration(
		catalogueSupplier(t), catalogueEntity(t), catalogueRef(t, domain.NewAuditorReference, auditor))
	if err != nil {
		t.Fatal(err)
	}
	return registration
}

func cataloguePayable(t *testing.T, accountID string) domain.SupplierPayableAccountRegistration {
	t.Helper()
	registration, err := domain.NewSupplierPayableAccountRegistration(
		catalogueSupplier(t), catalogueEntity(t), catalogueCurrency(t),
		catalogueRef(t, domain.NewSettlementAccountID, accountID))
	if err != nil {
		t.Fatal(err)
	}
	return registration
}

func catalogueRule(t *testing.T, version string) domain.ClaimAmountRuleRegistration {
	t.Helper()
	registration, err := domain.NewClaimAmountRuleRegistration(
		catalogueResponsibility(t), catalogueRef(t, domain.NewAmountRuleVersionReference, version))
	if err != nil {
		t.Fatal(err)
	}
	return registration
}

func catalogueFacts(t *testing.T, chargeID, accountID string) domain.ChargeConfirmationFactRegistration {
	t.Helper()
	facts := domain.ConfirmedChargeFacts{
		ResponsibleEntity:    catalogueEntity(t),
		Counterparty:         catalogueRef(t, domain.NewSettlementCounterpartyReference, "CP-1"),
		Direction:            domain.ChargeReceivable,
		SettlementAccount:    catalogueRef(t, domain.NewSettlementAccountID, accountID),
		ContractBasis:        catalogueRef(t, domain.NewContractBasisReference, "CONTRACT-1"),
		PrimaryChargingScope: catalogueRef(t, domain.NewChargingScopeReference, "PARCEL-1"),
		SourceFact:           catalogueRef(t, domain.NewSourceFactReference, "FACT-1"),
	}
	registration, err := domain.NewChargeConfirmationFactRegistration(catalogueCharge(t, chargeID), facts)
	if err != nil {
		t.Fatal(err)
	}
	return registration
}

func catalogueRef[T any](t *testing.T, parse func(string) (T, error), value string) T {
	t.Helper()
	parsed, err := parse(value)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}
