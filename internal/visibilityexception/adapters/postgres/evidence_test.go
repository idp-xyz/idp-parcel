package postgres_test

import (
	"context"
	"testing"
	"time"

	adapter "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

// 本文件对真实 PostgreSQL 16 证证据项与披露版本两张表（0024）：往返、按（提供方+指纹）
// 幂等、披露版本锚定原件并只增不改、库上三条 CHECK 与外键在场。

var evidenceBaseAt = time.Date(2026, 8, 12, 14, 0, 0, 0, time.UTC)

func evidenceStore(t *testing.T, fixture *registrarFixture) *adapter.EvidenceItems {
	t.Helper()
	store, err := adapter.NewEvidenceItems(fixture.db)
	if err != nil {
		t.Fatalf("构造证据登记册：%v", err)
	}
	return store
}

func submittedEvidence(t *testing.T, id, provider, digest string) domain.EvidenceItem {
	t.Helper()
	item, err := domain.SubmitEvidence(
		build(t, domain.NewEvidenceItemID, id),
		build(t, domain.NewEvidenceProviderReference, provider),
		build(t, domain.NewEvidenceContentDigest, digest),
		evidenceBaseAt,
	)
	if err != nil {
		t.Fatalf("受理证据：%v", err)
	}
	return item
}

func saveEvidence(t *testing.T, fixture *registrarFixture, store *adapter.EvidenceItems, tenant domain.TenantID, item domain.EvidenceItem) ports.EvidenceSaveOutcome {
	t.Helper()
	var outcome ports.EvidenceSaveOutcome
	if err := fixture.transactor.WithinTransaction(t.Context(), func(txCtx context.Context) error {
		var err error
		outcome, err = store.Save(txCtx, tenant, item)
		return err
	}); err != nil {
		t.Fatalf("事务内保存证据：%v", err)
	}
	return outcome
}

// Covers: 证据项往返与幂等界线——按标识与按（提供方+指纹）都读得回同一项，评价起点原样
// 是 RECEIVED 且无依据；同（提供方+指纹）再存交回 AlreadyRecorded 不覆盖；另一提供方同样
// 的指纹是另一项；另一租户不可见。
func TestEvidenceItemsRoundTripAndAreKeyedByProviderAndDigest(t *testing.T) {
	fixture := newRegistrarFixture(t)
	tenant := registrarTenant(t)
	store := evidenceStore(t, fixture)
	ctx := t.Context()

	item := submittedEvidence(t, "evd-1", "customer-1", "sha256:photo")
	if outcome := saveEvidence(t, fixture, store, tenant, item); outcome != ports.EvidenceSaved {
		t.Fatalf("首存应为 Saved，实得 %d", outcome)
	}

	byID, found, err := store.FindByID(ctx, tenant, item.ID())
	if err != nil || !found {
		t.Fatalf("按标识读回：%v found=%v", err, found)
	}
	if byID.Provider().String() != "customer-1" || byID.Digest().String() != "sha256:photo" ||
		!byID.SubmittedAt().Equal(evidenceBaseAt) || byID.Appraisal() != domain.EvidenceReceived {
		t.Fatalf("证据项没原样读回：%+v", byID.Snapshot())
	}
	if _, appraised := byID.AppraisalBasis(); appraised {
		t.Fatal("RECEIVED 的证据项读回带了评价依据")
	}
	byKey, found, err := store.FindByProviderDigest(ctx, tenant, item.Provider(), item.Digest())
	if err != nil || !found || byKey.ID() != item.ID() {
		t.Fatalf("按（提供方+指纹）读回：%v found=%v id=%s", err, found, byKey.ID())
	}

	// 同键另一个标识：不是本轮写的，AlreadyRecorded；原行不动。
	duplicate := submittedEvidence(t, "evd-2", "customer-1", "sha256:photo")
	if outcome := saveEvidence(t, fixture, store, tenant, duplicate); outcome != ports.EvidenceAlreadyRecorded {
		t.Fatalf("同（提供方+指纹）再存应为 AlreadyRecorded，实得 %d", outcome)
	}
	if _, found, _ := store.FindByID(ctx, tenant, duplicate.ID()); found {
		t.Fatal("被拒的重复项仍落了库")
	}

	// 另一提供方同样的字节是另一项。
	other := submittedEvidence(t, "evd-3", "carrier-9", "sha256:photo")
	if outcome := saveEvidence(t, fixture, store, tenant, other); outcome != ports.EvidenceSaved {
		t.Fatalf("另一提供方的同指纹应为独立一项，实得 %d", outcome)
	}

	if _, found, err := store.FindByID(ctx, build(t, domain.NewTenantID, "tenant-b"), item.ID()); err != nil || found {
		t.Fatalf("跨租户可见：err=%v found=%v", err, found)
	}
}

// Covers: 披露版本锚定原件、只增不改、按（证据项+脱敏指纹）幂等；FindDisclosure 读回经
// 构造门重验；证据项不在场无版本可读。
func TestEvidenceDisclosureVersionsAnchorTheOriginalAndOnlyAppend(t *testing.T) {
	fixture := newRegistrarFixture(t)
	tenant := registrarTenant(t)
	store := evidenceStore(t, fixture)
	ctx := t.Context()

	item := submittedEvidence(t, "evd-1", "customer-1", "sha256:photo")
	saveEvidence(t, fixture, store, tenant, item)
	redacted := build(t, domain.NewEvidenceContentDigest, "sha256:photo-redacted")

	if _, found, err := store.FindDisclosure(ctx, tenant, item.ID(), redacted); err != nil || found {
		t.Fatalf("准备之前应无版本：err=%v found=%v", err, found)
	}

	version, err := domain.PrepareDisclosure(item, redacted, "insurer-1", evidenceBaseAt.Add(time.Hour))
	if err != nil {
		t.Fatalf("形成披露版本：%v", err)
	}
	var outcome ports.EvidenceSaveOutcome
	if err := fixture.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		var err error
		outcome, err = store.SaveDisclosure(txCtx, tenant, version)
		return err
	}); err != nil || outcome != ports.EvidenceSaved {
		t.Fatalf("首存披露版本：err=%v outcome=%d", err, outcome)
	}

	read, found, err := store.FindDisclosure(ctx, tenant, item.ID(), redacted)
	if err != nil || !found {
		t.Fatalf("读回披露版本：%v found=%v", err, found)
	}
	if read.Item() != item.ID() || read.Original() != item.Digest() || read.Redacted() != redacted ||
		read.Scope() != "insurer-1" || !read.PreparedAt().Equal(evidenceBaseAt.Add(time.Hour)) {
		t.Fatalf("披露版本没原样读回：%+v", read)
	}

	if err := fixture.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		var err error
		outcome, err = store.SaveDisclosure(txCtx, tenant, version)
		return err
	}); err != nil || outcome != ports.EvidenceAlreadyRecorded {
		t.Fatalf("重存同一版本：err=%v outcome=%d，想要 AlreadyRecorded", err, outcome)
	}

	// 证据项不在场：外键拒下——给不存在的原件造披露版本正是「来源不明的附件」。
	ghost := submittedEvidence(t, "evd-ghost", "customer-1", "sha256:ghost")
	ghostVersion, err := domain.PrepareDisclosure(ghost, build(t, domain.NewEvidenceContentDigest, "sha256:ghost-redacted"), "x", evidenceBaseAt)
	if err != nil {
		t.Fatalf("形成幽灵版本：%v", err)
	}
	if err := fixture.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		_, err := store.SaveDisclosure(txCtx, tenant, ghostVersion)
		return err
	}); err == nil {
		t.Fatal("挂在不存在证据项上的披露版本被接受了")
	}
	if _, found, err := store.FindDisclosure(ctx, tenant, ghost.ID(), ghostVersion.Redacted()); err != nil || found {
		t.Fatalf("幽灵版本读回：err=%v found=%v", err, found)
	}
}

// Covers: 0024 的 CHECK——评价三值封闭、依据与评价成对、脱敏指纹不得等于原件。领域门拦在
// 前面，库是第二道网，这里直接写行证网在。
func TestEvidenceRowsAreCheckedByTheDatabase(t *testing.T) {
	fixture := newRegistrarFixture(t)
	ctx := t.Context()

	if _, err := fixture.pool.Exec(ctx,
		`INSERT INTO visibility_exception.evidence_item
			(tenant_id, evidence_id, provider_ref, content_digest, submitted_at, appraisal, appraisal_basis)
		 VALUES ('tenant-a', 'evd-1', 'p', 'd', now(), 'RECEIVED', 'why')`); err == nil {
		t.Fatal("库接受了带依据的 RECEIVED")
	}
	if _, err := fixture.pool.Exec(ctx,
		`INSERT INTO visibility_exception.evidence_item
			(tenant_id, evidence_id, provider_ref, content_digest, submitted_at, appraisal, appraisal_basis)
		 VALUES ('tenant-a', 'evd-1', 'p', 'd', now(), 'CREDITED', NULL)`); err == nil {
		t.Fatal("库接受了没有依据的 CREDITED")
	}
	if _, err := fixture.pool.Exec(ctx,
		`INSERT INTO visibility_exception.evidence_item
			(tenant_id, evidence_id, provider_ref, content_digest, submitted_at, appraisal, appraisal_basis)
		 VALUES ('tenant-a', 'evd-1', 'p', 'd', now(), 'TRUSTED', 'why')`); err == nil {
		t.Fatal("库接受了三值之外的评价")
	}
	if _, err := fixture.pool.Exec(ctx,
		`INSERT INTO visibility_exception.evidence_item
			(tenant_id, evidence_id, provider_ref, content_digest, submitted_at, appraisal, appraisal_basis)
		 VALUES ('tenant-a', 'evd-1', 'p', 'd', now(), 'RECEIVED', NULL)`); err != nil {
		t.Fatalf("合法证据行写不进：%v", err)
	}
	if _, err := fixture.pool.Exec(ctx,
		`INSERT INTO visibility_exception.evidence_disclosure_version
			(tenant_id, evidence_id, redacted_digest, original_digest, scope, prepared_at)
		 VALUES ('tenant-a', 'evd-1', 'd', 'd', 'scope', now())`); err == nil {
		t.Fatal("库接受了脱敏指纹等于原件的披露版本")
	}
}
