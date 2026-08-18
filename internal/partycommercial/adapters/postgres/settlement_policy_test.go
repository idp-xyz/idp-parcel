package postgres_test

import (
	"context"
	"testing"

	bentoapp "go.idp.xyz/idp-bento-go/application"

	adapter "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// 本文件对真实 PostgreSQL 16 证结算政策册（ADR-0044）：六维平铺往返、同内容重放、
// 异内容冲突、租户隔离、缺政策是合法缺席、登记推动 ViewRevision、method 封闭集 CHECK。

func TestSettlementPolicyRoundTripsWithTheRegistry(t *testing.T) {
	repository, transactor, _ := newPublications(t)
	ctx := t.Context()

	version := effectiveVersionOfKind(t, domain.SettlementPolicyObject, "settle-1", "v1", "digest-settle-1")
	mustSaveVersion(t, transactor, ctx, repository, version)
	policy := settlementPolicyOn(t, version, domain.TermsMethod, "charge-express")
	mustSaveSettlementPolicy(t, transactor, ctx, repository, policy)

	registry, err := repository.LoadForScope(ctx, pcTenant(t, "tenant-1"), pcScope(t))
	if err != nil {
		t.Fatalf("整册读回：%v", err)
	}
	policies := registry.SettlementPolicies()
	if len(policies) != 1 {
		t.Fatalf("读回 %d 份结算政策，want 1", len(policies))
	}
	if policies[0].Method() != domain.TermsMethod {
		t.Fatalf("method = %q", policies[0].Method())
	}
	if policies[0].Applicability().ChargeScope().String() != "charge-express" {
		t.Fatalf("charge scope = %q", policies[0].Applicability().ChargeScope())
	}
	if !policies[0].Version().SameVersionAs(version) {
		t.Fatal("政策挂回了另一个版本")
	}
}

func TestRegisteringASettlementPolicyMovesTheScopeViewRevision(t *testing.T) {
	repository, transactor, _ := newPublications(t)
	ctx := t.Context()
	tenant, scope := pcTenant(t, "tenant-1"), pcScope(t)

	version := effectiveVersionOfKind(t, domain.SettlementPolicyObject, "settle-1", "v1", "digest-settle-1")
	mustSaveVersion(t, transactor, ctx, repository, version)
	beforeRegistry, err := repository.LoadForScope(ctx, tenant, scope)
	if err != nil {
		t.Fatalf("登记政策前读回：%v", err)
	}
	before := beforeRegistry.ViewRevision(tenant, scope)

	mustSaveSettlementPolicy(t, transactor, ctx, repository,
		settlementPolicyOn(t, version, domain.PrepaidMethod, "charge-express"))

	afterRegistry, err := repository.LoadForScope(ctx, tenant, scope)
	if err != nil {
		t.Fatalf("登记政策后读回：%v", err)
	}
	if afterRegistry.ViewRevision(tenant, scope) == before {
		t.Fatal("结算政策从缺席变为在场，范围修订却没动")
	}
}

func TestASettlementVersionWithoutARegisteredPolicyStillLoads(t *testing.T) {
	repository, transactor, _ := newPublications(t)
	ctx := t.Context()

	version := effectiveVersionOfKind(t, domain.SettlementPolicyObject, "settle-1", "v1", "digest-settle-1")
	mustSaveVersion(t, transactor, ctx, repository, version)

	registry, err := repository.LoadForScope(ctx, pcTenant(t, "tenant-1"), pcScope(t))
	if err != nil {
		t.Fatalf("整册读回：%v", err)
	}
	if registry.Count() != 1 {
		t.Fatalf("版本数 = %d, want 1", registry.Count())
	}
	if policies := registry.SettlementPolicies(); len(policies) != 0 {
		t.Fatalf("没登记政策却读回 %d 份", len(policies))
	}
}

func TestSavingTheSameSettlementPolicyTwiceIsAReplay(t *testing.T) {
	repository, transactor, _ := newPublications(t)
	ctx := t.Context()

	version := effectiveVersionOfKind(t, domain.SettlementPolicyObject, "settle-1", "v1", "digest-settle-1")
	mustSaveVersion(t, transactor, ctx, repository, version)
	policy := settlementPolicyOn(t, version, domain.PrepaidMethod, "charge-express")
	mustSaveSettlementPolicy(t, transactor, ctx, repository, policy)

	mustWithinPublicationTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		outcome, err := repository.SaveSettlementPolicy(txCtx, policy)
		if err != nil {
			return err
		}
		if outcome != ports.SettlementPolicyAlreadyRegistered {
			t.Fatalf("replay outcome = %q, want ALREADY_REGISTERED", outcome)
		}
		return nil
	})
}

func TestADifferentSettlementMethodOnTheSameVersionConflicts(t *testing.T) {
	repository, transactor, _ := newPublications(t)
	ctx := t.Context()

	version := effectiveVersionOfKind(t, domain.SettlementPolicyObject, "settle-1", "v1", "digest-settle-1")
	mustSaveVersion(t, transactor, ctx, repository, version)
	mustSaveSettlementPolicy(t, transactor, ctx, repository,
		settlementPolicyOn(t, version, domain.PrepaidMethod, "charge-express"))

	changed := settlementPolicyOn(t, version, domain.TermsMethod, "charge-express")
	mustWithinPublicationTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		outcome, err := repository.SaveSettlementPolicy(txCtx, changed)
		if err != nil {
			return err
		}
		if outcome != ports.SettlementPolicyContentConflict {
			t.Fatalf("conflict outcome = %q, want CONTENT_CONFLICT", outcome)
		}
		return nil
	})
}

func TestAnotherTenantsSettlementPolicyDoesNotEnterThisScopesRegistry(t *testing.T) {
	repository, transactor, _ := newPublications(t)
	ctx := t.Context()

	mine := policyVersionInTenant(t, "tenant-1", domain.SettlementPolicyObject, "settle-1", "v1", "digest-mine")
	theirs := policyVersionInTenant(t, "tenant-2", domain.SettlementPolicyObject, "settle-1", "v1", "digest-theirs")
	mustSaveVersion(t, transactor, ctx, repository, mine)
	mustSaveVersion(t, transactor, ctx, repository, theirs)
	mustSaveSettlementPolicy(t, transactor, ctx, repository,
		settlementPolicyOn(t, theirs, domain.TermsMethod, "charge-express"))

	registry, err := repository.LoadForScope(ctx, pcTenant(t, "tenant-1"), pcScope(t))
	if err != nil {
		t.Fatalf("整册读回：%v", err)
	}
	if registry.Count() != 1 {
		t.Fatalf("本租户册里有 %d 个版本，want 1", registry.Count())
	}
	if policies := registry.SettlementPolicies(); len(policies) != 0 {
		t.Fatalf("他租登记的结算政策进了本租户的册：%d 份", len(policies))
	}
}

func TestASettlementMethodOutsideTheClosedSetIsRefused(t *testing.T) {
	repository, transactor, pool := newPublications(t)
	ctx := t.Context()
	version := effectiveVersionOfKind(t, domain.SettlementPolicyObject, "settle-1", "v1", "digest-settle-1")
	mustSaveVersion(t, transactor, ctx, repository, version)

	_, err := pool.Exec(ctx,
		`INSERT INTO party_commercial.commercial_settlement_policy
			(tenant_id, object_kind, object_id, version_label,
			 method, legal_entity_ref, counterparty_ref, contract_label,
			 charge_scope_ref, currency_code, effective_starts_at)
		 VALUES ('tenant-1', 7, 'settle-1', 'v1',
		         'CUSTOMER_DEFAULT', 'legal-1', 'customer-1', 'contract-1/v1',
		         'charge-express', 'SYN', now())`)
	if err == nil {
		t.Fatal("封闭集之外的结算方式进了结算政策册")
	}
}

func mustSaveSettlementPolicy(
	t *testing.T,
	transactor bentoapp.Transactor,
	ctx context.Context,
	repository *adapter.CommercialPublications,
	policy domain.SettlementPolicy,
) {
	t.Helper()
	mustWithinPublicationTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		outcome, err := repository.SaveSettlementPolicy(txCtx, policy)
		if err != nil {
			return err
		}
		if outcome != ports.SettlementPolicySaved {
			t.Fatalf("save outcome = %q, want SAVED", outcome)
		}
		return nil
	})
}

func settlementPolicyOn(
	t *testing.T,
	version domain.CommercialVersion,
	method domain.SettlementMethod,
	chargeScope string,
) domain.SettlementPolicy {
	t.Helper()
	applicability, err := domain.NewSettlementApplicability(
		pcValue(t, domain.NewLegalEntityReference, "legal-1"),
		pcValue(t, domain.NewCounterpartyReference, "customer-1"),
		pcValue(t, domain.NewCommercialVersionLabel, "contract-1/v1"),
		pcValue(t, domain.NewChargeScopeReference, chargeScope),
		pcValue(t, domain.NewCurrencyCode, "SYN"),
		version.Effective(),
	)
	if err != nil {
		t.Fatalf("结算适用范围：%v", err)
	}
	policy, err := domain.NewSettlementPolicy(version, method, applicability)
	if err != nil {
		t.Fatalf("new settlement policy: %v", err)
	}
	return policy
}
