package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

func TestAFixedResolutionRoundTripsByID(t *testing.T) {
	repository, transactor, _ := newResolutions(t)
	ctx := t.Context()
	closure := uniqueClosure(t)

	mustSaveResolution(t, transactor, ctx, repository, closure)

	found, ok, err := repository.LoadResolution(ctx, closure.ResolutionKey().TenantID, closure.ResolutionID())
	if err != nil {
		t.Fatalf("按标识取回：%v", err)
	}
	if !ok {
		t.Fatal("第一阶段写入后第二阶段 found=false")
	}
	if found.ResolutionID() != closure.ResolutionID() {
		t.Fatalf("resolution ID = %q, want %q", found.ResolutionID(), closure.ResolutionID())
	}
	if found.Outcome() != domain.UniquelyResolved {
		t.Fatalf("outcome = %q", found.Outcome())
	}
	if _, ok := found.AdoptedFor(domain.AcceptanceRulePackageObject); !ok {
		t.Fatal("读回的闭包丢了接单规则包——第二阶段要的正是它")
	}
}

func TestResolutionReplayAndConflictSplitByContent(t *testing.T) {
	repository, transactor, pool := newResolutions(t)
	ctx := t.Context()
	original := uniqueClosure(t)
	mustSaveResolution(t, transactor, ctx, repository, original)

	var replayOutcome ports.ResolutionSaveOutcome
	mustWithinResolutionTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		var err error
		replayOutcome, err = repository.Save(txCtx, original)
		return err
	})
	if replayOutcome != ports.ResolutionAlreadyRecorded {
		t.Fatalf("replay outcome = %s, want ALREADY_RECORDED", replayOutcome)
	}

	if _, err := pool.Exec(ctx,
		`UPDATE party_commercial.commercial_resolution
		    SET content_digest = 'forged-digest'
		  WHERE tenant_id = $1 AND resolution_id = $2`,
		original.ResolutionKey().TenantID.String(),
		original.ResolutionID().String(),
	); err != nil {
		t.Fatalf("伪造摘要：%v", err)
	}

	var conflictOutcome ports.ResolutionSaveOutcome
	mustWithinResolutionTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		var err error
		conflictOutcome, err = repository.Save(txCtx, original)
		return err
	})
	if conflictOutcome != ports.ResolutionContentConflict {
		t.Fatalf("conflict outcome = %s, want CONTENT_CONFLICT", conflictOutcome)
	}
}

func TestResolutionTenantsAreInvisibleToEachOther(t *testing.T) {
	repository, transactor, _ := newResolutions(t)
	ctx := t.Context()
	closure := uniqueClosure(t)
	mustSaveResolution(t, transactor, ctx, repository, closure)

	found, ok, err := repository.LoadResolution(ctx, pcTenant(t, "tenant-b"), closure.ResolutionID())
	if err != nil {
		t.Fatalf("他租户取回：%v", err)
	}
	if ok {
		t.Fatalf("他租户读到了本租户的解析：%q", found.ResolutionID())
	}
}

func TestResolutionWritesRefuseToRunOutsideATransaction(t *testing.T) {
	repository, _, _ := newResolutions(t)
	if _, err := repository.Save(t.Context(), uniqueClosure(t)); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务固定应返回 ErrTransactionRequired，实得：%v", err)
	}
}

func TestResolutionRollbackLeavesNothingBehind(t *testing.T) {
	repository, transactor, _ := newResolutions(t)
	ctx := t.Context()
	closure := uniqueClosure(t)
	rollback := errors.New("回滚")

	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		if _, err := repository.Save(txCtx, closure); err != nil {
			return err
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatalf("事务应以回滚错误结束，实得：%v", err)
	}

	_, ok, err := repository.LoadResolution(ctx, closure.ResolutionKey().TenantID, closure.ResolutionID())
	if err != nil {
		t.Fatalf("读回：%v", err)
	}
	if ok {
		t.Fatal("回滚后解析仍在")
	}
}

func TestResolutionSnapshotNullDoesNotPassTheThreeValuedCheck(t *testing.T) {
	_, _, pool := newResolutions(t)
	ctx := t.Context()

	if _, err := pool.Exec(ctx,
		`INSERT INTO party_commercial.commercial_resolution
			(tenant_id, resolution_id, customer_account_id, outcome, content_digest, snapshot)
		 VALUES ('tenant-1', 'CLO-null-1', 'customer-1', 1, 'digest-1', NULL)`); err == nil {
		t.Fatal("一行「快照列为 NULL」按 jsonb 三值缝溜进了解析库")
	}

	if _, err := pool.Exec(ctx,
		`INSERT INTO party_commercial.commercial_resolution
			(tenant_id, resolution_id, customer_account_id, outcome, content_digest, snapshot)
		 VALUES ('tenant-1', 'CLO-null-2', 'customer-1', 1, 'digest-1', 'null'::jsonb)`); err == nil {
		t.Fatal("一行「快照为 json null」按 jsonb 三值缝溜进了解析库")
	}

	if _, err := pool.Exec(ctx,
		`INSERT INTO party_commercial.commercial_resolution
			(tenant_id, resolution_id, customer_account_id, outcome, content_digest, snapshot)
		 VALUES ('tenant-1', 'CLO-not-unique', 'customer-1', 4, 'digest-1', '{}'::jsonb)`); err == nil {
		t.Fatal("一行「非唯一解析」进了只收唯一已解析的表")
	}
}

func newResolutions(t *testing.T) (*adapter.CommercialResolutions, bentoapp.Transactor, *pgxpool.Pool) {
	t.Helper()

	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	repository, err := adapter.NewCommercialResolutions(db)
	if err != nil {
		t.Fatalf("构造解析库：%v", err)
	}
	return repository, db.Transactor(), pool
}

func mustWithinResolutionTransaction(
	t *testing.T,
	transactor bentoapp.Transactor,
	ctx context.Context,
	fn func(context.Context) error,
) {
	t.Helper()
	if err := transactor.WithinTransaction(ctx, fn); err != nil {
		t.Fatalf("事务内写入失败：%v", err)
	}
}

func mustSaveResolution(
	t *testing.T,
	transactor bentoapp.Transactor,
	ctx context.Context,
	repository *adapter.CommercialResolutions,
	closure domain.CommercialClosure,
) {
	t.Helper()
	var savedOutcome ports.ResolutionSaveOutcome
	mustWithinResolutionTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		var err error
		savedOutcome, err = repository.Save(txCtx, closure)
		return err
	})
	if savedOutcome != ports.ResolutionSaved {
		t.Fatalf("save outcome = %s", savedOutcome)
	}
}

func uniqueClosure(t *testing.T) domain.CommercialClosure {
	t.Helper()

	registry := domain.NewCommercialRegistry()
	contract := effectiveContract(t, "contract-1", "v1", "digest-1")
	rules := effectiveRules(t, "rules-1", "v1", "digest-r1")
	if _, err := registry.Register(contract); err != nil {
		t.Fatalf("登记合同：%v", err)
	}
	if _, err := registry.Register(rules); err != nil {
		t.Fatalf("登记规则包：%v", err)
	}

	anchor, err := domain.NewSelectionAnchor(effectiveAtRow.Add(24*time.Hour),
		pcValue(t, domain.NewAnchorPolicyVersion, "anchor-policy-v1"))
	if err != nil {
		t.Fatalf("选择锚点：%v", err)
	}
	closure := domain.ResolveCommercialClosure(registry, domain.ClosureResolutionKey{
		TenantID:             pcTenant(t, "tenant-1"),
		CustomerAccountID:    pcValue(t, domain.NewCustomerAccountID, "customer-1"),
		LegalEntityCandidate: pcValue(t, domain.NewLegalEntityReference, "legal-1"),
		Scope:                pcScope(t),
		Purpose:              domain.AcceptanceControlPurpose,
		Anchor:               anchor,
		RequiredBases: []domain.CommercialObjectKind{
			domain.CustomerContractObject,
			domain.AcceptanceRulePackageObject,
		},
	}, nil)
	if closure.Outcome() != domain.UniquelyResolved {
		t.Fatalf("outcome = %q, want UNIQUELY_RESOLVED", closure.Outcome())
	}
	return closure
}

func effectiveRules(t *testing.T, objectID, label, digest string) domain.CommercialVersion {
	t.Helper()

	interval, err := domain.NewEffectiveInterval(effectiveAtRow, effectiveAtRow.Add(90*24*time.Hour))
	if err != nil {
		t.Fatalf("有效区间：%v", err)
	}
	approval, err := domain.NewApprovalBasis(
		pcValue(t, domain.NewApprovalReference, "approval-rules"),
		pcValue(t, domain.NewCommercialSourceReference, "source-rules"),
		approvedAtFixture,
	)
	if err != nil {
		t.Fatalf("批准依据：%v", err)
	}
	version, err := domain.RehydrateCommercialVersion(domain.RehydrateCommercialVersionSpec{
		TenantID:      pcTenant(t, "tenant-1"),
		Kind:          domain.AcceptanceRulePackageObject,
		ObjectID:      pcValue(t, domain.NewCommercialObjectID, objectID),
		Version:       pcValue(t, domain.NewCommercialVersionLabel, label),
		Scope:         pcScope(t),
		ContentDigest: pcValue(t, domain.NewCommercialContentDigest, digest),
		Effective:     interval,
		Status:        domain.CommercialVersionEffective,
		Approval:      approval,
		PublishedAt:   publishedAtRow,
		EffectiveAt:   effectiveAtRow,
	})
	if err != nil {
		t.Fatalf("重建规则包：%v", err)
	}
	return version
}
