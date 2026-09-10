package postgres_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/partycommercial/application"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证授权治理册：空表译未配置、命中译已授权并指名版本、
// 同范围仅复核译不允许、跨租户读不到、过期与从未登记同格、撞键重放/冲突、无事务拒。

var grantJudgedAt = time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

func TestAnEmptyGrantBookIsNotConfigured(t *testing.T) {
	grants, _, _ := newAuthorityGrants(t)
	handler := application.NewAdjudicateCommercialAuthorizationHandler(grants)

	_, err := handler.Handle(t.Context(), pcTenant(t, "tenant-1"), grantRejectionRequest(t, "scope-a", grantJudgedAt))
	if !errors.Is(err, domain.ErrAuthorityRulesNotConfigured) {
		t.Fatalf("error = %v, want ErrAuthorityRulesNotConfigured", err)
	}
}

func TestAMatchingGrantAuthorizesAndNamesTheVersion(t *testing.T) {
	grants, transactor, _ := newAuthorityGrants(t)
	grant := persistedGrant(t, "tenant-1", "auth-reject", domain.ActiveRejectionAction, "level-commercial", "scope-a")
	mustSaveGrant(t, transactor, t.Context(), grants, grant)

	handler := application.NewAdjudicateCommercialAuthorizationHandler(grants)
	authorized, err := handler.Handle(t.Context(), pcTenant(t, "tenant-1"), grantRejectionRequest(t, "scope-a", grantJudgedAt))
	if err != nil {
		t.Fatalf("adjudicate: %v", err)
	}
	if authorized.GrantVersion().ObjectID().String() != "auth-reject" {
		t.Fatalf("采用版本 = %q, want auth-reject", authorized.GrantVersion().ObjectID())
	}
	if authorized.Reason().String() != "COMMERCIAL_RISK" {
		t.Fatal("已授权丢了结构化原因")
	}
}

func TestAReviewGrantInTheSameScopeRefusesActiveRejection(t *testing.T) {
	grants, transactor, _ := newAuthorityGrants(t)
	mustSaveGrant(t, transactor, t.Context(), grants,
		persistedGrant(t, "tenant-1", "auth-review", domain.ManualReviewAction, "level-commercial", "scope-a"))

	handler := application.NewAdjudicateCommercialAuthorizationHandler(grants)
	_, err := handler.Handle(t.Context(), pcTenant(t, "tenant-1"), grantRejectionRequest(t, "scope-a", grantJudgedAt))
	if !errors.Is(err, domain.ErrNotAuthorized) {
		t.Fatalf("error = %v, want ErrNotAuthorized", err)
	}
}

func TestAnotherTenantReadsNoneOfTheseGrants(t *testing.T) {
	grants, transactor, _ := newAuthorityGrants(t)
	mustSaveGrant(t, transactor, t.Context(), grants,
		persistedGrant(t, "tenant-1", "auth-reject", domain.ActiveRejectionAction, "level-commercial", "scope-a"))

	handler := application.NewAdjudicateCommercialAuthorizationHandler(grants)
	_, err := handler.Handle(t.Context(), pcTenant(t, "tenant-b"), grantRejectionRequest(t, "scope-a", grantJudgedAt))
	if !errors.Is(err, domain.ErrAuthorityRulesNotConfigured) {
		t.Fatalf("error = %v；跨租户应与询问一个不存在的范围同形", err)
	}
}

func TestAnExpiredGrantIsNotConfiguredRatherThanRefused(t *testing.T) {
	grants, transactor, _ := newAuthorityGrants(t)
	mustSaveGrant(t, transactor, t.Context(), grants,
		persistedGrant(t, "tenant-1", "auth-reject", domain.ActiveRejectionAction, "level-commercial", "scope-a"))

	handler := application.NewAdjudicateCommercialAuthorizationHandler(grants)
	late := time.Date(2028, 1, 1, 0, 0, 0, 0, time.UTC)
	_, err := handler.Handle(t.Context(), pcTenant(t, "tenant-1"), grantRejectionRequest(t, "scope-a", late))
	if !errors.Is(err, domain.ErrAuthorityRulesNotConfigured) {
		t.Fatalf("error = %v, want ErrAuthorityRulesNotConfigured（规则全部过期与从未登记同格）", err)
	}
}

func TestRecordingTheSameGrantTwiceIsAReplay(t *testing.T) {
	grants, transactor, _ := newAuthorityGrants(t)
	grant := persistedGrant(t, "tenant-1", "auth-reject", domain.ActiveRejectionAction, "level-commercial", "scope-a")
	mustSaveGrant(t, transactor, t.Context(), grants, grant)

	var outcome ports.GrantSaveOutcome
	mustWithinPublicationTransaction(t, transactor, t.Context(), func(txCtx context.Context) error {
		var err error
		outcome, err = grants.SaveGrant(txCtx, grant)
		return err
	})
	if outcome != ports.GrantAlreadyRegistered {
		t.Fatalf("outcome = %s, want ALREADY_REGISTERED", outcome)
	}
}

func TestAChangedGrantAtTheSameKeyConflicts(t *testing.T) {
	grants, transactor, _ := newAuthorityGrants(t)
	mustSaveGrant(t, transactor, t.Context(), grants,
		persistedGrant(t, "tenant-1", "auth-reject", domain.ActiveRejectionAction, "level-commercial", "scope-a"))

	changed := persistedGrant(t, "tenant-1", "auth-reject", domain.ManualReviewAction, "level-commercial", "scope-a")
	var outcome ports.GrantSaveOutcome
	mustWithinPublicationTransaction(t, transactor, t.Context(), func(txCtx context.Context) error {
		var err error
		outcome, err = grants.SaveGrant(txCtx, changed)
		return err
	})
	if outcome != ports.GrantContentConflict {
		t.Fatalf("outcome = %s, want CONTENT_CONFLICT", outcome)
	}
}

func TestGrantWritesRefuseToRunOutsideATransaction(t *testing.T) {
	grants, _, _ := newAuthorityGrants(t)
	err := func() error {
		_, err := grants.SaveGrant(t.Context(),
			persistedGrant(t, "tenant-1", "auth-reject", domain.ActiveRejectionAction, "level-commercial", "scope-a"))
		return err
	}()
	if !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务登记授权应返回 ErrTransactionRequired，实得：%v", err)
	}
}

func TestTwoMatchingGrantsReturnTheFirstInStableOrder(t *testing.T) {
	grants, transactor, _ := newAuthorityGrants(t)
	mustSaveGrant(t, transactor, t.Context(), grants,
		persistedGrant(t, "tenant-1", "auth-b", domain.ActiveRejectionAction, "level-commercial", "scope-a"))
	mustSaveGrant(t, transactor, t.Context(), grants,
		persistedGrant(t, "tenant-1", "auth-a", domain.ActiveRejectionAction, "level-commercial", "scope-a"))

	loaded, err := grants.LoadEffectiveGrants(t.Context(),
		pcTenant(t, "tenant-1"),
		pcValue(t, domain.NewCommercialScopeReference, "scope-a"),
		grantJudgedAt)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(loaded) != 2 {
		t.Fatalf("loaded %d grants, want 2", len(loaded))
	}
	// 今天 Authorize 返回首个命中，命中顺序即装载顺序。ORDER BY object_id 让顺序稳定，
	// 但「采用哪一版」仍依赖这个顺序——不是一条与顺序无关的生产采用规则。
	if loaded[0].Version().ObjectID().String() != "auth-a" {
		t.Fatalf("first loaded = %q, want auth-a（按 object_id 排序，不是登记先后）",
			loaded[0].Version().ObjectID())
	}

	handler := application.NewAdjudicateCommercialAuthorizationHandler(grants)
	authorized, err := handler.Handle(t.Context(), pcTenant(t, "tenant-1"), grantRejectionRequest(t, "scope-a", grantJudgedAt))
	if err != nil {
		t.Fatalf("adjudicate: %v", err)
	}
	if authorized.GrantVersion().ObjectID().String() != "auth-a" {
		t.Fatalf("采用 %q；今天是装载顺序的第一个命中，不是生产采用规则",
			authorized.GrantVersion().ObjectID())
	}
}

// Covers: ADR-0116 Decision 一 —— 「资料修订」进 0003 动作 CHECK 的三值重建（0025），登记后按范围时点读回
// 仍是同一格；旧的两格不受影响。
func TestASourceDataAmendmentGrantRoundTripsThroughTheBook(t *testing.T) {
	grants, transactor, _ := newAuthorityGrants(t)
	mustSaveGrant(t, transactor, t.Context(), grants,
		persistedGrant(t, "tenant-1", "auth-amend", domain.SourceDataAmendmentAction, "level-commercial", "scope-a"))

	loaded, err := grants.LoadEffectiveGrants(t.Context(),
		pcTenant(t, "tenant-1"),
		pcValue(t, domain.NewCommercialScopeReference, "scope-a"),
		grantJudgedAt)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(loaded) != 1 || loaded[0].Action() != domain.SourceDataAmendmentAction {
		t.Fatalf("loaded = %+v, want one SOURCE_DATA_AMENDMENT grant", loaded)
	}

	// 同范围只有资料修订授权时，主动拒绝落`不允许`而不是`未配置`：范围已被表过态。
	handler := application.NewAdjudicateCommercialAuthorizationHandler(grants)
	_, err = handler.Handle(t.Context(), pcTenant(t, "tenant-1"), grantRejectionRequest(t, "scope-a", grantJudgedAt))
	if !errors.Is(err, domain.ErrNotAuthorized) {
		t.Fatalf("error = %v, want ErrNotAuthorized", err)
	}
}

// Covers: pc-gaps/13 完成判据 3——0003 / 0025 钉死字面量的 authorization_grant_action_closed 经 0032 重建后收
// CONTROLLED_CLOSURE / REOPENING：两格经 SaveGrant → LoadEffectiveGrants 往返，裁定编排照四格代数答——范围里只登了
// 另一等级的重开授权时，本等级请求重开是不允许（机制只看有无、不比等级高低，裁决 ①）；集外词仍拒在 CHECK；
// contract_delegation 的 action CHECK 未扩——关闭 / 重开的委派行进不了库（CONTEXT：合同委派不参与）。
func TestClosureAndReopeningGrantsRoundTripAndTheChecksHold(t *testing.T) {
	// 授权册与发布登记册要共用同一个 DB：事务归属按 DB 实例判，两个实例各开一套事务互不相认。
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
	ctx := t.Context()
	tenant := pcTenant(t, "tenant-1")
	mustSaveGrant(t, transactor, ctx, grants,
		persistedGrant(t, "tenant-1", "auth-close", domain.ControlledClosureAction, "level-commercial", "scope-a"))
	mustSaveGrant(t, transactor, ctx, grants,
		persistedGrant(t, "tenant-1", "auth-reopen", domain.ReopeningAction, "level-senior", "scope-a"))

	loaded, err := grants.LoadEffectiveGrants(ctx, tenant, pcValue(t, domain.NewCommercialScopeReference, "scope-a"), grantJudgedAt)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	actions := map[domain.AuthorizedAction]string{}
	for _, grant := range loaded {
		actions[grant.Action()] = grant.Version().ObjectID().String()
	}
	if len(loaded) != 2 || actions[domain.ControlledClosureAction] != "auth-close" || actions[domain.ReopeningAction] != "auth-reopen" {
		t.Fatalf("loaded = %+v, want one CONTROLLED_CLOSURE and one REOPENING grant", loaded)
	}

	handler := application.NewAdjudicateCommercialAuthorizationHandler(grants)
	if _, err := handler.Handle(ctx, tenant, grantRequestFor(t, domain.ReopeningAction, "level-commercial", "scope-a", grantJudgedAt)); !errors.Is(err, domain.ErrNotAuthorized) {
		t.Fatalf("error = %v; 持关闭授权的等级请求重开应不允许——关闭权不蕴含重开权，机制也不替等级排序", err)
	}
	authorized, err := handler.Handle(ctx, tenant, grantRequestFor(t, domain.ReopeningAction, "level-senior", "scope-a", grantJudgedAt))
	if err != nil || authorized.GrantVersion().ObjectID().String() != "auth-reopen" {
		t.Fatalf("authorized = %+v err = %v", authorized, err)
	}

	if _, err := pool.Exec(ctx,
		`INSERT INTO party_commercial.authorization_grant
			(tenant_id, object_id, version_label, action, legal_entity_ref, authority_level, scope_ref,
			 effective_starts_at, content_digest, snapshot)
		 VALUES ('tenant-1', 'auth-x', 'v1', 'WITHDRAWAL', 'legal-1', 'level-commercial', 'scope-a',
			 now(), 'sha256:x', '{}'::jsonb)`); err == nil {
		t.Fatal("封闭集外的授权动作进了表")
	}

	// 委派册的动作 CHECK 不随本票放宽：先经写口落一条合法的资料修订委派（父行随之在场），再裸插一行关闭委派。
	contract := effectiveContract(t, "contract-1", "v1", "digest-c1")
	mustSaveVersion(t, transactor, ctx, publications, contract)
	mustSaveDelegations(t, transactor, ctx, publications, delegationsOn(t, contract,
		delegationRow(t, accountDelegatorRow(t, "account-1"), "level-commercial", "scope-1", delegationInterval(t)),
	))
	for _, action := range []string{"CONTROLLED_CLOSURE", "REOPENING"} {
		_, err := pool.Exec(ctx,
			`INSERT INTO party_commercial.contract_delegation
				(tenant_id, object_kind, object_id, version_label, action, scope_ref, authority_level,
				 delegator_kind, delegator_ref, effective_starts_at)
			 VALUES ('tenant-1', 2, 'contract-1', 'v1', $1, 'scope-1', 'level-clerk', 'CUSTOMER_ACCOUNT', 'account-1', now())`,
			action)
		if err == nil || !strings.Contains(err.Error(), "contract_delegation_action_delegable") {
			t.Fatalf("%s: err = %v; 委派册的动作 CHECK 收下了关闭 / 重开", action, err)
		}
	}
}

func grantRequestFor(t *testing.T, action domain.AuthorizedAction, level, scope string, at time.Time) domain.AuthorizationRequest {
	t.Helper()
	request, err := domain.NewAuthorizationRequest(
		action,
		pcValue(t, domain.NewLegalEntityReference, "legal-1"),
		pcValue(t, domain.NewAuthorityLevel, level),
		pcValue(t, domain.NewCommercialScopeReference, scope),
		pcValue(t, domain.NewStructuredReason, "CUSTOMER_INSTRUCTION"),
		pcValue(t, domain.NewEvidenceReference, "evidence-1"),
		at,
	)
	if err != nil {
		t.Fatalf("authorization request: %v", err)
	}
	return request
}

func newAuthorityGrants(t *testing.T) (*adapter.AuthorityGrants, bentoapp.Transactor, *pgxpool.Pool) {
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
	return grants, db.Transactor(), pool
}

func mustSaveGrant(
	t *testing.T,
	transactor bentoapp.Transactor,
	ctx context.Context,
	repository *adapter.AuthorityGrants,
	grant domain.AuthorityGrant,
) {
	t.Helper()
	mustWithinPublicationTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		outcome, err := repository.SaveGrant(txCtx, grant)
		if err != nil {
			return err
		}
		if outcome != ports.GrantSaved {
			return fmt.Errorf("save outcome = %s", outcome)
		}
		return nil
	})
}

func grantRejectionRequest(t *testing.T, scope string, at time.Time) domain.AuthorizationRequest {
	t.Helper()
	request, err := domain.NewAuthorizationRequest(
		domain.ActiveRejectionAction,
		pcValue(t, domain.NewLegalEntityReference, "legal-1"),
		pcValue(t, domain.NewAuthorityLevel, "level-commercial"),
		pcValue(t, domain.NewCommercialScopeReference, scope),
		pcValue(t, domain.NewStructuredReason, "COMMERCIAL_RISK"),
		pcValue(t, domain.NewEvidenceReference, "evidence-1"),
		at,
	)
	if err != nil {
		t.Fatalf("authorization request: %v", err)
	}
	return request
}

func persistedGrant(
	t *testing.T,
	tenant, objectID string,
	action domain.AuthorizedAction,
	level, scope string,
) domain.AuthorityGrant {
	t.Helper()
	interval, err := domain.NewEffectiveInterval(effectiveAtRow, effectiveAtRow.Add(90*24*time.Hour))
	if err != nil {
		t.Fatalf("有效区间：%v", err)
	}
	approval, err := domain.NewApprovalBasis(
		pcValue(t, domain.NewApprovalReference, "approval-"+objectID),
		pcValue(t, domain.NewCommercialSourceReference, "source-"+objectID),
		approvedAtFixture,
	)
	if err != nil {
		t.Fatalf("批准依据：%v", err)
	}
	version, err := domain.RehydrateCommercialVersion(domain.RehydrateCommercialVersionSpec{
		TenantID:      pcTenant(t, tenant),
		Kind:          domain.AuthorizationRuleObject,
		ObjectID:      pcValue(t, domain.NewCommercialObjectID, objectID),
		Version:       pcValue(t, domain.NewCommercialVersionLabel, "v1"),
		Scope:         pcValue(t, domain.NewCommercialScopeReference, scope),
		ContentDigest: pcValue(t, domain.NewCommercialContentDigest, "sha256:"+objectID),
		Effective:     interval,
		Status:        domain.CommercialVersionEffective,
		Approval:      approval,
		PublishedAt:   publishedAtRow,
		EffectiveAt:   effectiveAtRow,
	})
	if err != nil {
		t.Fatalf("重建授权规则版本：%v", err)
	}
	grant, err := domain.NewAuthorityGrant(
		version,
		action,
		pcValue(t, domain.NewLegalEntityReference, "legal-1"),
		pcValue(t, domain.NewAuthorityLevel, level),
		pcValue(t, domain.NewCommercialScopeReference, scope),
		interval,
	)
	if err != nil {
		t.Fatalf("形成授权：%v", err)
	}
	return grant
}
