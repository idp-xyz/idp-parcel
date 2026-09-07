package postgres_test

import (
	"context"
	"errors"
	"fmt"
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
