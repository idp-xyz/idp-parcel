package postgres_test

import (
	"context"
	"testing"
	"time"

	bentoapp "go.idp.xyz/idp-bento-go/application"

	adapter "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// 本文件对真实 PostgreSQL 16 证区间更正册（ADR-0038 / D-5）：更正随整册一次取回、
// 原版本区间不被改写、同一版本可有多条且选用最后一条、接纳新更正推动该范围的
// ViewRevision、同内容重放、他租更正不入本租户的册。

func TestValidityCorrectionRoundTripsWithTheRegistry(t *testing.T) {
	repository, transactor, _ := newPublications(t)
	ctx := t.Context()

	version := effectiveContract(t, "contract-1", "v1", "digest-1")
	mustSaveVersion(t, transactor, ctx, repository, version)
	correction := validityCorrectionOf(t, version,
		time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
		"ext-correction-1",
		time.Date(2026, 5, 2, 0, 0, 0, 0, time.UTC),
	)
	mustSaveValidityCorrection(t, transactor, ctx, repository, correction)

	registry, err := repository.LoadForScope(ctx, pcTenant(t, "tenant-1"), pcScope(t))
	if err != nil {
		t.Fatalf("整册读回：%v", err)
	}
	stored, found := registry.Lookup(
		version.Tenant(), version.Kind(), version.ObjectID(), version.Version())
	if !found {
		t.Fatal("更正后原版本从登记册消失了")
	}
	if stored.Effective() != version.Effective() {
		t.Fatal("更正覆盖了原版本上记录的历史区间")
	}
	held, ok := registry.ValidityCorrectionOf(
		version.Tenant(), version.Kind(), version.ObjectID(), version.Version())
	if !ok {
		t.Fatal("更正关系没有留下来")
	}
	if held.Reference() != correction.Reference() {
		t.Fatalf("correction reference = %q, want %q", held.Reference(), correction.Reference())
	}
	if held.CorrectedInterval() != correction.CorrectedInterval() {
		t.Fatal("更正后区间没有按外部源落下")
	}
}

// Covers: D-5 只增多条 + ADR-0038「接纳更正必须使该范围的 ViewRevision 变化」。
func TestASecondValidityCorrectionIsAppendedAndMovesTheScopeViewRevision(t *testing.T) {
	repository, transactor, _ := newPublications(t)
	ctx := t.Context()
	tenant, scope := pcTenant(t, "tenant-1"), pcScope(t)

	version := effectiveContract(t, "contract-1", "v1", "digest-1")
	mustSaveVersion(t, transactor, ctx, repository, version)
	first := validityCorrectionOf(t, version,
		time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
		"ext-correction-1",
		time.Date(2026, 5, 2, 0, 0, 0, 0, time.UTC),
	)
	mustSaveValidityCorrection(t, transactor, ctx, repository, first)

	beforeRegistry, err := repository.LoadForScope(ctx, tenant, scope)
	if err != nil {
		t.Fatalf("第二条更正前读回：%v", err)
	}
	before := beforeRegistry.ViewRevision(tenant, scope)

	second := validityCorrectionOf(t, version,
		time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC),
		"ext-correction-2",
		time.Date(2026, 5, 3, 0, 0, 0, 0, time.UTC),
	)
	mustSaveValidityCorrection(t, transactor, ctx, repository, second)

	afterRegistry, err := repository.LoadForScope(ctx, tenant, scope)
	if err != nil {
		t.Fatalf("第二条更正后读回：%v", err)
	}
	if afterRegistry.ViewRevision(tenant, scope) == before {
		t.Fatal("接纳新更正后范围修订却没动")
	}
	held, ok := afterRegistry.ValidityCorrectionOf(
		version.Tenant(), version.Kind(), version.ObjectID(), version.Version())
	if !ok {
		t.Fatal("选用更正读不回来")
	}
	if held.Reference() != second.Reference() {
		t.Fatal("选用更正不是登记顺序上的最后一条")
	}
	if afterRegistry.ViewRevision(tenant, scope) == beforeRegistry.ViewRevision(tenant, scope) {
		t.Fatal("含两条更正的视图与只含第一条的视图相同")
	}
}

// Covers: 更正 1:N 不得把同行的形态册放大。若装载把更正直接左连接进版本行，两条更正
// 会让同一份形态登记两次。
func TestMultipleCorrectionsDoNotDuplicateARegisteredForm(t *testing.T) {
	repository, transactor, _ := newPublications(t)
	ctx := t.Context()

	version := productVersionInTenant(t, "tenant-1", "product-1", "v1", "digest-product-1")
	mustSaveVersion(t, transactor, ctx, repository, version)
	mustSaveServiceProduct(t, transactor, ctx, repository, serviceProductOf(t, version))
	mustSaveValidityCorrection(t, transactor, ctx, repository, validityCorrectionOf(t, version,
		time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC),
		"ext-correction-1",
		time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC),
	))
	mustSaveValidityCorrection(t, transactor, ctx, repository, validityCorrectionOf(t, version,
		time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC),
		"ext-correction-2",
		time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC),
	))

	registry, err := repository.LoadForScope(ctx, pcTenant(t, "tenant-1"), pcScope(t))
	if err != nil {
		t.Fatalf("整册读回：%v", err)
	}
	if products := registry.ServiceProducts(); len(products) != 1 {
		t.Fatalf("读回 %d 份产品形态，want 1（更正把形态行放大了）", len(products))
	}
}

// Covers: 同内容重放。开放结束区间走 UNIQUE NULLS NOT DISTINCT，两行 NULL 不得被当成不同键。
func TestSavingTheSameValidityCorrectionTwiceIsAReplay(t *testing.T) {
	repository, transactor, _ := newPublications(t)
	ctx := t.Context()

	version := effectiveContract(t, "contract-1", "v1", "digest-1")
	mustSaveVersion(t, transactor, ctx, repository, version)
	correction := unboundedValidityCorrectionOf(t, version,
		time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		"ext-correction-open",
		time.Date(2026, 5, 2, 0, 0, 0, 0, time.UTC),
	)
	mustSaveValidityCorrection(t, transactor, ctx, repository, correction)

	mustWithinPublicationTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		outcome, err := repository.SaveValidityCorrection(txCtx, correction)
		if err != nil {
			return err
		}
		if outcome != ports.ValidityCorrectionAlreadyRegistered {
			t.Fatalf("replay outcome = %q, want ALREADY_REGISTERED", outcome)
		}
		return nil
	})
}

// Covers: `AT-PC-014` 的更正册半边（ADR-0040 / ADR-0003）：他租更正不得出现在本租户的册里。
func TestAnotherTenantsValidityCorrectionDoesNotEnterThisScopesRegistry(t *testing.T) {
	repository, transactor, _ := newPublications(t)
	ctx := t.Context()

	mine := effectiveVersionInTenant(t, "tenant-1", domain.CustomerContractObject, "contract-1", "v1", "digest-mine")
	theirs := effectiveVersionInTenant(t, "tenant-2", domain.CustomerContractObject, "contract-1", "v1", "digest-theirs")
	mustSaveVersion(t, transactor, ctx, repository, mine)
	mustSaveVersion(t, transactor, ctx, repository, theirs)
	mustSaveValidityCorrection(t, transactor, ctx, repository, validityCorrectionOf(t, theirs,
		time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
		"ext-correction-theirs",
		time.Date(2026, 5, 2, 0, 0, 0, 0, time.UTC),
	))

	registry, err := repository.LoadForScope(ctx, pcTenant(t, "tenant-1"), pcScope(t))
	if err != nil {
		t.Fatalf("整册读回：%v", err)
	}
	if registry.Count() != 1 {
		t.Fatalf("本租户册里有 %d 个版本，want 1（他租版本漏进来了）", registry.Count())
	}
	if _, ok := registry.ValidityCorrectionOf(mine.Tenant(), mine.Kind(), mine.ObjectID(), mine.Version()); ok {
		t.Fatal("他租登记的更正进了本租户的册")
	}
}

func TestAVersionWithoutARegisteredCorrectionStillLoads(t *testing.T) {
	repository, transactor, _ := newPublications(t)
	ctx := t.Context()

	version := effectiveContract(t, "contract-1", "v1", "digest-1")
	mustSaveVersion(t, transactor, ctx, repository, version)

	registry, err := repository.LoadForScope(ctx, pcTenant(t, "tenant-1"), pcScope(t))
	if err != nil {
		t.Fatalf("整册读回：%v", err)
	}
	if registry.Count() != 1 {
		t.Fatalf("版本数 = %d, want 1", registry.Count())
	}
	if _, ok := registry.ValidityCorrectionOf(
		version.Tenant(), version.Kind(), version.ObjectID(), version.Version()); ok {
		t.Fatal("没登记更正却读回了一条")
	}
}

func TestSavingAValidityCorrectionWithoutItsVersionIsRefused(t *testing.T) {
	repository, transactor, _ := newPublications(t)
	ctx := t.Context()

	version := effectiveContract(t, "contract-1", "v1", "digest-1")
	correction := validityCorrectionOf(t, version,
		time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
		"ext-correction-1",
		time.Date(2026, 5, 2, 0, 0, 0, 0, time.UTC),
	)

	err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		_, err := repository.SaveValidityCorrection(txCtx, correction)
		return err
	})
	if err == nil {
		t.Fatal("版本不在册的更正进了更正册")
	}
}

// ---- 夹具 ----

func mustSaveValidityCorrection(
	t *testing.T,
	transactor bentoapp.Transactor,
	ctx context.Context,
	repository *adapter.CommercialPublications,
	correction domain.ValidityCorrection,
) {
	t.Helper()
	mustWithinPublicationTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		outcome, err := repository.SaveValidityCorrection(txCtx, correction)
		if err != nil {
			return err
		}
		if outcome != ports.ValidityCorrectionSaved {
			t.Fatalf("save outcome = %q, want SAVED", outcome)
		}
		return nil
	})
}

func validityCorrectionOf(
	t *testing.T,
	version domain.CommercialVersion,
	startsAt, endsAt time.Time,
	reference string,
	at time.Time,
) domain.ValidityCorrection {
	t.Helper()
	interval, err := domain.NewEffectiveInterval(startsAt, endsAt)
	if err != nil {
		t.Fatalf("更正区间：%v", err)
	}
	correction, err := version.CorrectEffectiveInterval(
		interval,
		pcValue(t, domain.NewValidityCorrectionReference, reference),
		at,
	)
	if err != nil {
		t.Fatalf("构造区间更正：%v", err)
	}
	return correction
}

func unboundedValidityCorrectionOf(
	t *testing.T,
	version domain.CommercialVersion,
	startsAt time.Time,
	reference string,
	at time.Time,
) domain.ValidityCorrection {
	t.Helper()
	return validityCorrectionOf(t, version, startsAt, time.Time{}, reference, at)
}

func effectiveVersionInTenant(
	t *testing.T,
	tenant string,
	kind domain.CommercialObjectKind,
	objectID, label, digest string,
) domain.CommercialVersion {
	t.Helper()

	interval, err := domain.NewEffectiveInterval(effectiveAtRow, effectiveAtRow.Add(90*24*time.Hour))
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
		TenantID:      pcTenant(t, tenant),
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
		t.Fatalf("重建版本：%v", err)
	}
	return version
}
