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

// 本文件对真实 PostgreSQL 16 证发布登记册的行为：整册往返后登记册代数完好（选用、
// 引用、视图修订）、重复登记与同号异文冲突由撞键读回判定且事务保持可用、草稿不入册
// 由库内 CHECK 拦住、租户隔离、无事务拒、回滚无痕。

var (
	approvedAtFixture = time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC)
	publishedAtRow    = time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	effectiveAtRow    = time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC)
)

func TestAPublishedVersionRoundTripsThroughTheRegistry(t *testing.T) {
	repository, transactor, _ := newPublications(t)
	ctx := t.Context()

	contract := effectiveContract(t, "contract-1", "v1", "digest-1")
	mustSaveVersion(t, transactor, ctx, repository, contract)

	registry, err := repository.LoadForScope(ctx, pcTenant(t, "tenant-1"), pcScope(t))
	if err != nil {
		t.Fatalf("整册读回：%v", err)
	}
	if registry.Count() != 1 {
		t.Fatalf("count = %d, want 1", registry.Count())
	}
	found, exists := registry.Lookup(
		pcTenant(t, "tenant-1"), domain.CustomerContractObject,
		pcValue(t, domain.NewCommercialObjectID, "contract-1"),
		pcValue(t, domain.NewCommercialVersionLabel, "v1"))
	if !exists {
		t.Fatal("已登记的版本读不回来")
	}
	if !found.SameVersionAs(contract) {
		t.Fatalf("版本往返变形：%+v", found)
	}
	if found.Status() != domain.CommercialVersionEffective {
		t.Fatalf("status = %s", found.Status())
	}
	reference, referenced := found.ReferenceTo(domain.AcceptanceRulePackageObject)
	if !referenced || reference.String() != "rules-1" {
		t.Fatal("指名引用没有随版本往返")
	}
	if _, bounded := found.Effective().EndsAt(); !bounded {
		t.Fatal("有界区间读回成了开放结束")
	}
	if revision := registry.ViewRevision(pcTenant(t, "tenant-1"), pcScope(t)); revision.String() == "" {
		t.Fatal("读回的登记册派生不出视图修订")
	}
}

// TestReplayAndConflictSplitByContent 证撞键后的判定：同键同内容是重放（已登记）、
// 同键异文是需要商业责任方修正的冲突——两者都不是 error，撞键后同事务立刻可读。
func TestReplayAndConflictSplitByContent(t *testing.T) {
	repository, transactor, _ := newPublications(t)
	ctx := t.Context()

	original := effectiveContract(t, "contract-1", "v1", "digest-1")
	mustSaveVersion(t, transactor, ctx, repository, original)

	mustWithinPublicationTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		outcome, err := repository.SaveVersion(txCtx, original)
		if err != nil {
			return err
		}
		if outcome != ports.PublicationAlreadyRegistered {
			t.Fatalf("replay outcome = %s, want ALREADY_REGISTERED", outcome)
		}

		forged := effectiveContract(t, "contract-1", "v1", "digest-forged")
		outcome, err = repository.SaveVersion(txCtx, forged)
		if err != nil {
			return err
		}
		if outcome != ports.PublicationContentConflict {
			t.Fatalf("conflict outcome = %s, want CONTENT_CONFLICT", outcome)
		}
		return nil
	})

	registry, err := repository.LoadForScope(ctx, pcTenant(t, "tenant-1"), pcScope(t))
	if err != nil {
		t.Fatalf("读回：%v", err)
	}
	found, _ := registry.Lookup(
		pcTenant(t, "tenant-1"), domain.CustomerContractObject,
		pcValue(t, domain.NewCommercialObjectID, "contract-1"),
		pcValue(t, domain.NewCommercialVersionLabel, "v1"))
	if found.ContentDigest().String() != "digest-1" {
		t.Fatal("冒名的同号版本覆盖了原发布")
	}
}

// TestTheLastTwoObjectKindsReachTheRegistry 证信用政策与授权规则这两类能进登记册。
//
// 它盯的是**领域封闭集与库内 CHECK 的对齐**，不是这两类今天真有发布方：0001 写下
// `BETWEEN 1 AND 7` 时领域只有七类，此后加到九类，两个新值因此被库拒在门外。这一格
// 谁也踩不到——授权规则的生效授予走 authorization_grant、信用政策还没有存储口——所以
// 它只会在有人第一次发布这两类的那天炸，而那时炸的是一次事务内的 CHECK 违反。
func TestTheLastTwoObjectKindsReachTheRegistry(t *testing.T) {
	repository, transactor, _ := newPublications(t)
	ctx := t.Context()

	for name, kind := range map[string]domain.CommercialObjectKind{
		"信用政策": domain.CreditPolicyObject,
		"授权规则": domain.AuthorizationRuleObject,
	} {
		t.Run(name, func(t *testing.T) {
			objectID := "object-" + kind.String()
			mustSaveVersion(t, transactor, ctx, repository,
				effectiveVersionOfKind(t, kind, objectID, "v1", "digest-"+objectID))

			registry, err := repository.LoadForScope(ctx, pcTenant(t, "tenant-1"), pcScope(t))
			if err != nil {
				t.Fatalf("整册读回：%v", err)
			}
			if _, exists := registry.Lookup(
				pcTenant(t, "tenant-1"), kind,
				pcValue(t, domain.NewCommercialObjectID, objectID),
				pcValue(t, domain.NewCommercialVersionLabel, "v1"),
			); !exists {
				t.Fatalf("%s 登记后读不回来——库内 CHECK 与领域封闭集又对不上了", kind)
			}
		})
	}
}

// TestAnObjectKindOutsideTheClosedSetIsRefused 是上一条的另一半：放宽不等于放开。
// 绕过领域直插一个封闭集之外的类别仍须被库拒——否则上一条的「对齐」会退化成「取消约束」。
// 下面那个字面量是**当前上界加一**，封闭集每拓宽一次它就要跟着走一格（0004 从 7 到 9、
// 0019 从 9 到 10 时都动过）。写死一个数是有意的——按符号取会让它跟着领域自动漂，而本条
// 要证的恰是「库上那道 CHECK 与领域集合对齐」，两边都跟着同一个符号走就证不出对齐了。
//
// **这不是本仓禁的那种计数，是它明写的唯一例外。** AGENTS.md「改文档」一节禁计数禁得很硬，
// 紧接着留了一格：「数本身就是论点时」必须写。这里的数就是论点。读到它的人若因为「计数会
// 过期」而顺手改成按符号取，清掉的是这条测试的全部证明力。
//
// 那条例外还附带一句「必须锚住取证时的 SHA」，**而这一处不适用**：锚 SHA 是给一次性举证用
// 的（举完那几处被修好，举证自己就成了假话）；本条是一道持续生效的门，每次运行都重新取证，
// 没有可钉的时刻。套错这半条会让人以为它该带一个很快就过期的 SHA。
func TestAnObjectKindOutsideTheClosedSetIsRefused(t *testing.T) {
	_, _, pool := newPublications(t)

	_, err := pool.Exec(t.Context(),
		`INSERT INTO party_commercial.commercial_version
			(tenant_id, object_kind, object_id, version_label, scope_ref,
			 content_digest, effective_starts_at, effective_ends_at, status,
			 snapshot, published_at)
		 VALUES ('tenant-1', 11, 'object-x', 'v1', 'scope-1',
		         'digest-x', now(), NULL, 3, '{}', now())`)
	if err == nil {
		t.Fatal("封闭集之外的对象类别进了发布登记册")
	}
}

// TestTenantsAreInvisibleToEachOther 证跨租户合法同号（ADR-0040）互不可见：他租户
// 读回空册，与「从未发布过」长得完全一样。
func TestTenantsAreInvisibleToEachOther(t *testing.T) {
	repository, transactor, _ := newPublications(t)
	ctx := t.Context()

	mustSaveVersion(t, transactor, ctx, repository, effectiveContract(t, "contract-1", "v1", "digest-1"))

	registry, err := repository.LoadForScope(ctx, pcTenant(t, "tenant-b"), pcScope(t))
	if err != nil {
		t.Fatalf("他租户读册：%v", err)
	}
	if registry.Count() != 0 {
		t.Error("他租户读到了本租户的发布")
	}
}

// TestDraftsNeverReachTheTable 证草稿不入册由库内 CHECK 拦住：绕过领域直插一行
// status=DRAFT 被数据库拒——本表存在的理由正是「发布后正文不可覆盖」。
func TestDraftsNeverReachTheTable(t *testing.T) {
	_, _, pool := newPublications(t)
	ctx := t.Context()

	_, err := pool.Exec(ctx,
		`INSERT INTO party_commercial.commercial_version
			(tenant_id, object_kind, object_id, version_label, scope_ref,
			 content_digest, effective_starts_at, effective_ends_at, status,
			 snapshot, published_at)
		 VALUES ('tenant-1', 2, 'contract-1', 'v1', 'scope-1',
		         'digest-1', now(), NULL, 1, '{}', now())`)
	if err == nil {
		t.Fatal("一行草稿进了发布登记册")
	}
}

// TestPublicationWritesRefuseToRunOutsideATransaction 证写入不会在缺少事务时改用
// 连接池。
func TestPublicationWritesRefuseToRunOutsideATransaction(t *testing.T) {
	repository, _, _ := newPublications(t)
	ctx := t.Context()

	if _, err := repository.SaveVersion(ctx, effectiveContract(t, "contract-1", "v1", "digest-1")); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务登记应返回 ErrTransactionRequired，实得：%v", err)
	}
}

// TestPublicationRollbackLeavesNothingBehind 证登记与它所在的事务同生共死。
func TestPublicationRollbackLeavesNothingBehind(t *testing.T) {
	repository, transactor, _ := newPublications(t)
	ctx := t.Context()
	rollback := errors.New("回滚")

	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		if _, err := repository.SaveVersion(txCtx, effectiveContract(t, "contract-1", "v1", "digest-1")); err != nil {
			return err
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatalf("事务应以回滚错误结束，实得：%v", err)
	}

	registry, err := repository.LoadForScope(ctx, pcTenant(t, "tenant-1"), pcScope(t))
	if err != nil {
		t.Fatalf("读回：%v", err)
	}
	if registry.Count() != 0 {
		t.Error("回滚后登记仍在")
	}
}

// ---- 夹具 ----

func newPublications(t *testing.T) (*adapter.CommercialPublications, bentoapp.Transactor, *pgxpool.Pool) {
	t.Helper()

	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	repository, err := adapter.NewCommercialPublications(db)
	if err != nil {
		t.Fatalf("构造发布登记册：%v", err)
	}
	return repository, db.Transactor(), pool
}

func mustWithinPublicationTransaction(
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

func mustSaveVersion(
	t *testing.T,
	transactor bentoapp.Transactor,
	ctx context.Context,
	repository *adapter.CommercialPublications,
	version domain.CommercialVersion,
) {
	t.Helper()
	mustWithinPublicationTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		outcome, err := repository.SaveVersion(txCtx, version)
		if err != nil {
			return err
		}
		if outcome != ports.PublicationSaved {
			t.Fatalf("save outcome = %s", outcome)
		}
		return nil
	})
}

func pcValue[T any](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	value, err := construct(raw)
	if err != nil {
		t.Fatalf("construct %q: %v", raw, err)
	}
	return value
}

func pcTenant(t *testing.T, tenant string) domain.TenantID {
	t.Helper()
	return pcValue(t, domain.NewTenantID, tenant)
}

func pcScope(t *testing.T) domain.CommercialScopeReference {
	t.Helper()
	return pcValue(t, domain.NewCommercialScopeReference, "scope-1")
}

// effectiveContract 经领域重建门造一份`已生效`合同版本（带批准、有界区间与一条指名
// 引用）。走重建门而不是逐步走发布生命周期：夹具要的是一份合法的已发布版本，而重建
// 门的校验正是「什么算合法」的单一权威。
func effectiveContract(t *testing.T, objectID, label, digest string) domain.CommercialVersion {
	t.Helper()
	return effectiveVersionOfKind(t, domain.CustomerContractObject, objectID, label, digest)
}

func effectiveVersionOfKind(
	t *testing.T,
	kind domain.CommercialObjectKind,
	objectID, label, digest string,
) domain.CommercialVersion {
	t.Helper()

	interval, err := domain.NewEffectiveInterval(
		effectiveAtRow, effectiveAtRow.Add(90*24*time.Hour))
	if err != nil {
		t.Fatalf("有效区间：%v", err)
	}
	approval, err := domain.NewApprovalBasis(
		pcValue(t, domain.NewApprovalReference, "approval-1"),
		pcValue(t, domain.NewCommercialSourceReference, "source-1"),
		approvedAtFixture,
	)
	if err != nil {
		t.Fatalf("批准依据：%v", err)
	}
	version, err := domain.RehydrateCommercialVersion(domain.RehydrateCommercialVersionSpec{
		TenantID:      pcTenant(t, "tenant-1"),
		Kind:          kind,
		ObjectID:      pcValue(t, domain.NewCommercialObjectID, objectID),
		Version:       pcValue(t, domain.NewCommercialVersionLabel, label),
		Scope:         pcScope(t),
		ContentDigest: pcValue(t, domain.NewCommercialContentDigest, digest),
		Effective:     interval,
		Status:        domain.CommercialVersionEffective,
		Approval:      approval,
		PublishedAt:   publishedAtRow,
		EffectiveAt:   effectiveAtRow,
		References: map[domain.CommercialObjectKind]domain.CommercialObjectID{
			domain.AcceptanceRulePackageObject: pcValue(t, domain.NewCommercialObjectID, "rules-1"),
		},
	})
	if err != nil {
		t.Fatalf("重建 %s 版本：%v", kind, err)
	}
	return version
}
