package postgres_test

import (
	"context"
	"errors"
	"testing"

	bentopg "go.idp.xyz/idp-bento-go/postgres"
	adapter "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/partycommercial/application"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 证发布里的授予通道（ADR-0312）：授权册与发布登记册共用同一个 DB（事务归属按 DB 实例判）。

func newGrantPublicationStores(t *testing.T) (*adapter.AuthorityGrants, *adapter.CommercialPublications, func(func(context.Context) error)) {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	grants, err := adapter.NewAuthorityGrants(db)
	if err != nil {
		t.Fatalf("构造授权治理册：%v", err)
	}
	publications, err := adapter.NewCommercialPublications(db)
	if err != nil {
		t.Fatalf("构造发布登记册：%v", err)
	}
	transactor := db.Transactor()
	return grants, publications, func(work func(context.Context) error) {
		t.Helper()
		mustWithinPublicationTransaction(t, transactor, t.Context(), work)
	}
}

func saveGrantViaPublication(
	t *testing.T,
	publications *adapter.CommercialPublications,
	within func(func(context.Context) error),
	grant domain.AuthorityGrant,
) ports.DeclarationSaveOutcome {
	t.Helper()
	var outcome ports.DeclarationSaveOutcome
	within(func(ctx context.Context) error {
		var err error
		outcome, err = publications.SaveAuthorityGrant(ctx, grant)
		return err
	})
	return outcome
}

// Covers: AT-PC-063 真库那一半——发布里的授予通道落在授权册同一张表：首次是已保存，同授予重放是已登记，
// 同版本换等级是内容冲突且不覆盖（读回仍是首次登记的那一条）；无事务拒。
func TestAnAuthorityGrantPublishedThroughTheDeclarationChannelReplaysAndConflicts(t *testing.T) {
	grants, publications, within := newGrantPublicationStores(t)
	first := persistedGrant(t, "tenant-1", "auth-amend", domain.SourceDataAmendmentAction, "CUSTOMER_SELF_SERVICE", "scope-a")

	if got := saveGrantViaPublication(t, publications, within, first); got != ports.DeclarationSaved {
		t.Fatalf("首次 = %s, want SAVED", got)
	}
	if got := saveGrantViaPublication(t, publications, within, first); got != ports.DeclarationAlreadyRegistered {
		t.Fatalf("重放 = %s, want ALREADY_REGISTERED", got)
	}
	changed := persistedGrant(t, "tenant-1", "auth-amend", domain.SourceDataAmendmentAction, "OTHER_LEVEL", "scope-a")
	if got := saveGrantViaPublication(t, publications, within, changed); got != ports.DeclarationContentConflict {
		t.Fatalf("换等级 = %s, want CONTENT_CONFLICT", got)
	}

	loaded, err := grants.LoadEffectiveGrants(t.Context(), pcTenant(t, "tenant-1"),
		pcValue(t, domain.NewCommercialScopeReference, "scope-a"), grantJudgedAt)
	if err != nil || len(loaded) != 1 || loaded[0].Level().String() != "CUSTOMER_SELF_SERVICE" {
		t.Fatalf("读回 = %+v err = %v，冲突不得覆盖首次登记", loaded, err)
	}

	if _, err := publications.SaveAuthorityGrant(t.Context(), first); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务登记授予应返回 ErrTransactionRequired，实得：%v", err)
	}
}

// Covers: AT-PC-064——经授予通道登进去之后，授权判断对该等级的客户账户答允许并指名那一版；等级不同的请求答不允许；
// 没登授予的租户如实答未配置，不是不允许。
func TestAPublishedSourceDataAmendmentGrantAllowsItsLevelAndLeavesOtherTenantsUnconfigured(t *testing.T) {
	grants, publications, within := newGrantPublicationStores(t)
	grant := persistedGrant(t, "tenant-1", "auth-amend", domain.SourceDataAmendmentAction, "CUSTOMER_SELF_SERVICE", "scope-a")
	if got := saveGrantViaPublication(t, publications, within, grant); got != ports.DeclarationSaved {
		t.Fatalf("首次 = %s, want SAVED", got)
	}

	handler := application.NewAdjudicateCommercialAuthorizationHandler(grants)
	request := func(level string) domain.AuthorizationRequest {
		requester, err := domain.RequestedByCustomerAccount(pcValue(t, domain.NewCustomerAccountID, "account-1"))
		if err != nil {
			t.Fatalf("requester: %v", err)
		}
		built, err := domain.NewAuthorizationRequestBy(
			requester,
			domain.SourceDataAmendmentAction,
			pcValue(t, domain.NewLegalEntityReference, "legal-1"),
			pcValue(t, domain.NewAuthorityLevel, level),
			pcValue(t, domain.NewCommercialScopeReference, "scope-a"),
			pcValue(t, domain.NewStructuredReason, "CUSTOMER_INSTRUCTION"),
			pcValue(t, domain.NewEvidenceReference, "evidence-1"),
			grantJudgedAt,
		)
		if err != nil {
			t.Fatalf("authorization request: %v", err)
		}
		return built
	}

	authorized, err := handler.Handle(t.Context(), pcTenant(t, "tenant-1"), request("CUSTOMER_SELF_SERVICE"))
	if err != nil {
		t.Fatalf("同等级应允许：%v", err)
	}
	if authorized.GrantVersion().ObjectID().String() != "auth-amend" {
		t.Fatalf("采用版本 = %q, want auth-amend", authorized.GrantVersion().ObjectID())
	}
	if _, err := handler.Handle(t.Context(), pcTenant(t, "tenant-1"), request("OTHER_LEVEL")); !errors.Is(err, domain.ErrNotAuthorized) {
		t.Fatalf("异等级 err = %v, want ErrNotAuthorized", err)
	}
	if _, err := handler.Handle(t.Context(), pcTenant(t, "tenant-2"), request("CUSTOMER_SELF_SERVICE")); !errors.Is(err, domain.ErrAuthorityRulesNotConfigured) {
		t.Fatalf("没登授予的租户 err = %v, want ErrAuthorityRulesNotConfigured", err)
	}
}
