package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/customscompliance/application"
	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

var manifestAt = time.Date(2026, 8, 13, 20, 0, 0, 0, time.UTC)

type manifestCandidateViewDouble struct {
	candidates []domain.AssociationCandidate
	err        error
}

func (double *manifestCandidateViewDouble) LoadAssociationCandidates(
	_ context.Context,
	_ domain.TenantID,
	_ domain.CustomsProcedureReference,
	_ domain.ManifestDirection,
) ([]domain.AssociationCandidate, error) {
	if double.err != nil {
		return nil, double.err
	}
	return double.candidates, nil
}

type manifestStoreDouble struct {
	byManifest map[string]domain.ExternalManifestReference
}

// manifestRow 是替身的行键：承运商责任加舱单身份，与真库的键同形。
func manifestRow(carrier domain.CarrierResponsibilityReference, manifest domain.ExternalManifestID) string {
	return carrier.String() + "|" + manifest.String()
}

func (double *manifestStoreDouble) FindByManifest(
	_ context.Context,
	_ domain.TenantID,
	carrier domain.CarrierResponsibilityReference,
	manifest domain.ExternalManifestID,
) (domain.ExternalManifestReference, bool, error) {
	reference, found := double.byManifest[manifestRow(carrier, manifest)]
	return reference, found, nil
}

func (double *manifestStoreDouble) Save(
	_ context.Context,
	_ domain.TenantID,
	reference domain.ExternalManifestReference,
) (ports.ManifestSaveOutcome, error) {
	row := manifestRow(reference.Carrier(), reference.Manifest())
	if _, exists := double.byManifest[row]; exists {
		return ports.ManifestAlreadyRecorded, nil
	}
	double.byManifest[row] = reference
	return ports.ManifestSaved, nil
}

func (double *manifestStoreDouble) Update(
	_ context.Context,
	_ domain.TenantID,
	reference domain.ExternalManifestReference,
) (ports.ManifestUpdateOutcome, error) {
	row := manifestRow(reference.Carrier(), reference.Manifest())
	prior, _ := reference.PriorVersion()
	if current, exists := double.byManifest[row]; !exists || current.Version() != prior {
		return ports.ManifestVersionMoved, nil
	}
	double.byManifest[row] = reference
	return ports.ManifestAdvanced, nil
}

type manifestDownstreamDouble struct {
	intents []ports.ManifestHandoffIntent
}

func (double *manifestDownstreamDouble) HandOffManifest(
	_ context.Context,
	intent ports.ManifestHandoffIntent,
) error {
	double.intents = append(double.intents, intent)
	return nil
}

type manifestFixture struct {
	handler    *application.ReceiveManifestHandler
	candidates *manifestCandidateViewDouble
	store      *manifestStoreDouble
}

func newManifestFixture(t *testing.T) *manifestFixture {
	t.Helper()
	fixture := &manifestFixture{
		candidates: &manifestCandidateViewDouble{},
		store:      &manifestStoreDouble{byManifest: map[string]domain.ExternalManifestReference{}},
	}
	fixture.handler = application.NewReceiveManifestHandler(application.ReceiveManifestDeps{
		Candidates: fixture.candidates,
		Store:      fixture.store,
		Downstream: &manifestDownstreamDouble{},
		Clock:      fixedClock{at: manifestAt},
	})
	return fixture
}

func manifestSpec(t *testing.T, version string) domain.ExternalManifestReferenceSpec {
	t.Helper()
	return domain.ExternalManifestReferenceSpec{
		Manifest:   mustValue(t, domain.NewExternalManifestID, "carrier-manifest-1"),
		Version:    mustValue(t, domain.NewManifestSourceVersion, version),
		Carrier:    mustValue(t, domain.NewCarrierResponsibilityReference, "carrier-responsibility-1"),
		Procedure:  mustValue(t, domain.NewCustomsProcedureReference, "US-IMPORT/TYPE-86"),
		Direction:  domain.ImportManifest,
		Scope:      mustValue(t, domain.NewDecisionScopeReference, "movement-1"),
		SourceFact: "CARRIER-FEED/manifest-filed",
		AcceptedAt: manifestAt.Add(-time.Hour),
	}
}

func manifestCandidate(t *testing.T, unit, scope string) domain.AssociationCandidate {
	t.Helper()
	return domain.AssociationCandidate{
		Unit:      mustValue(t, domain.NewDeclarationUnitID, unit),
		Procedure: mustValue(t, domain.NewCustomsProcedureReference, "US-IMPORT/TYPE-86"),
		Direction: domain.ImportManifest,
		Scope:     mustValue(t, domain.NewDecisionScopeReference, scope),
	}
}

// Covers: CC CONTEXT 生命周期「逐范围唯一匹配 → 形成业务关联和当前采用关系；无法唯一匹配时保持待关联，不创建占位对象或按最近客户、班次猜测」的编排面——恰一候选关联、零候选与
// 多候选都待关联入册；同舱单同版本重放返原。点名 `AT-CC-380`「唯一关联→形成逐对象、
// 逐范围关系」、`AT-CC-381`「可能关联两个客户……→形成待关联或冲突，不以先到请求取得
// 全部范围」与 `AT-CC-387`「同一来源身份、版本和内容重复到达→返回已有」；同舱单异
// 版本经接收入口是冲突（版本推进
// 走修订入口才留得下前版指回）。
func TestManifestsAssociateOnlyOnUniqueMatch(t *testing.T) {
	fixture := newManifestFixture(t)
	tenant := mustValue(t, domain.NewTenantID, "tenant-1")

	pending, err := fixture.handler.Receive(context.Background(), application.ReceiveManifestCommand{
		TenantID: tenant,
		Spec:     manifestSpec(t, "v1"),
	})
	if err != nil {
		t.Fatalf("pending receive: %v", err)
	}
	if pending.Outcome() != application.ManifestPendingAssociation {
		t.Fatalf("outcome = %q; 零候选必须待关联", pending.Outcome())
	}
	reference, _ := pending.Reference()
	if _, associated := reference.Association(); associated {
		t.Fatal("零候选还关联上了")
	}

	replay, err := fixture.handler.Receive(context.Background(), application.ReceiveManifestCommand{
		TenantID: tenant,
		Spec:     manifestSpec(t, "v1"),
	})
	if err != nil {
		t.Fatalf("replay receive: %v", err)
	}
	if replay.Outcome() != application.ManifestExisting {
		t.Fatalf("replay = %q", replay.Outcome())
	}

	conflict, err := fixture.handler.Receive(context.Background(), application.ReceiveManifestCommand{
		TenantID: tenant,
		Spec:     manifestSpec(t, "v2"),
	})
	if err != nil {
		t.Fatalf("conflict receive: %v", err)
	}
	if conflict.Outcome() != application.ManifestVersionConflict {
		t.Fatalf("conflict = %q; 版本推进必须走修订入口", conflict.Outcome())
	}

	fixture.candidates.candidates = []domain.AssociationCandidate{
		manifestCandidate(t, "unit-1", "movement-1"),
		manifestCandidate(t, "unit-2", "movement-1"),
	}
	multi := newManifestFixture(t)
	multi.candidates.candidates = fixture.candidates.candidates
	ambiguous, err := multi.handler.Receive(context.Background(), application.ReceiveManifestCommand{
		TenantID: tenant,
		Spec:     manifestSpec(t, "v1"),
	})
	if err != nil {
		t.Fatalf("ambiguous receive: %v", err)
	}
	if ambiguous.Outcome() != application.ManifestPendingAssociation {
		t.Fatalf("outcome = %q; 多候选分不出唯一必须待关联不猜", ambiguous.Outcome())
	}

	unique := newManifestFixture(t)
	unique.candidates.candidates = []domain.AssociationCandidate{
		manifestCandidate(t, "unit-1", "movement-1"),
		manifestCandidate(t, "unit-2", "movement-9"),
	}
	matched, err := unique.handler.Receive(context.Background(), application.ReceiveManifestCommand{
		TenantID: tenant,
		Spec:     manifestSpec(t, "v1"),
	})
	if err != nil {
		t.Fatalf("matched receive: %v", err)
	}
	if matched.Outcome() != application.ManifestAssociated {
		t.Fatalf("outcome = %q", matched.Outcome())
	}
	matchedReference, _ := matched.Reference()
	unit, associated := matchedReference.Association()
	if !associated || unit.String() != "unit-1" {
		t.Fatalf("association = %v/%v", unit, associated)
	}
}

// Covers: CC CONTEXT「原来源、版本、范围和关联历史不得覆盖」的编排面——修订换版本换范围指回前版，关联不随版本自动
// 搬移（新版本重新走唯一匹配）；同版本重复修订按已有作答；没有引用无从修订。点名
// `AT-CC-389`「承运商明确提供 V2 更正 V1→形成新版本和更正关系，保留 V1、原范围和
// 原结果历史」。
func TestRevisionsAdvanceVersionsAndRematchAssociations(t *testing.T) {
	fixture := newManifestFixture(t)
	tenant := mustValue(t, domain.NewTenantID, "tenant-1")
	fixture.candidates.candidates = []domain.AssociationCandidate{
		manifestCandidate(t, "unit-1", "movement-1"),
	}

	received, err := fixture.handler.Receive(context.Background(), application.ReceiveManifestCommand{
		TenantID: tenant,
		Spec:     manifestSpec(t, "v1"),
	})
	if err != nil {
		t.Fatalf("receive: %v", err)
	}
	if received.Outcome() != application.ManifestAssociated {
		t.Fatalf("outcome = %q", received.Outcome())
	}

	fixture.candidates.candidates = nil
	revised, err := fixture.handler.Revise(context.Background(), application.ReviseManifestCommand{
		TenantID:   tenant,
		Carrier:    mustValue(t, domain.NewCarrierResponsibilityReference, "carrier-responsibility-1"),
		Manifest:   mustValue(t, domain.NewExternalManifestID, "carrier-manifest-1"),
		Version:    mustValue(t, domain.NewManifestSourceVersion, "v2"),
		Scope:      mustValue(t, domain.NewDecisionScopeReference, "movement-2"),
		SourceFact: "CARRIER-FEED/manifest-amended",
		At:         manifestAt,
	})
	if err != nil {
		t.Fatalf("revise: %v", err)
	}
	if revised.Outcome() != application.ManifestRevised {
		t.Fatalf("outcome = %q", revised.Outcome())
	}
	revisedReference, _ := revised.Reference()
	prior, hasPrior := revisedReference.PriorVersion()
	if !hasPrior || prior.String() != "v1" {
		t.Fatalf("prior = %v/%v; 新版本必须指回前版", prior, hasPrior)
	}
	if _, associated := revisedReference.Association(); associated {
		t.Fatal("关联随版本自动搬移了——新版本要重新唯一匹配")
	}

	same, err := fixture.handler.Revise(context.Background(), application.ReviseManifestCommand{
		TenantID:   tenant,
		Carrier:    mustValue(t, domain.NewCarrierResponsibilityReference, "carrier-responsibility-1"),
		Manifest:   mustValue(t, domain.NewExternalManifestID, "carrier-manifest-1"),
		Version:    mustValue(t, domain.NewManifestSourceVersion, "v2"),
		Scope:      mustValue(t, domain.NewDecisionScopeReference, "movement-2"),
		SourceFact: "CARRIER-FEED/duplicate",
		At:         manifestAt.Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("revise replay: %v", err)
	}
	if same.Outcome() != application.ManifestExisting {
		t.Fatalf("replay = %q", same.Outcome())
	}

	missing, err := fixture.handler.Revise(context.Background(), application.ReviseManifestCommand{
		TenantID:   tenant,
		Carrier:    mustValue(t, domain.NewCarrierResponsibilityReference, "carrier-responsibility-1"),
		Manifest:   mustValue(t, domain.NewExternalManifestID, "carrier-manifest-9"),
		Version:    mustValue(t, domain.NewManifestSourceVersion, "v1"),
		Scope:      mustValue(t, domain.NewDecisionScopeReference, "movement-1"),
		SourceFact: "CARRIER-FEED/unknown",
		At:         manifestAt,
	})
	if err != nil {
		t.Fatalf("revise missing: %v", err)
	}
	if missing.Outcome() != application.ManifestNotFound {
		t.Fatalf("missing = %q", missing.Outcome())
	}
}

// Covers: UC-CC-012「承运商来源只能作用于其被授权的……范围。越权来源未受理且不得确认候选对象是否存在」——
// 外部舱单身份只在承运商自己的编号空间里唯一。一家承运商入册某个号之后，另一家交同一个号：首登在它自己的那一格里
// 照常入册，答复与这个号从没人用过时一样，读不到也占不住前一家的引用；它推进这个号的版本，动的只是它自己那一条。
func TestTwoCarriersUsingTheSameManifestNumberNeverSeeOrTouchEachOther(t *testing.T) {
	fixture := newManifestFixture(t)
	tenant := mustValue(t, domain.NewTenantID, "tenant-1")
	first := mustValue(t, domain.NewCarrierResponsibilityReference, "carrier-responsibility-1")
	second := mustValue(t, domain.NewCarrierResponsibilityReference, "carrier-responsibility-2")
	number := mustValue(t, domain.NewExternalManifestID, "carrier-manifest-1")
	if _, err := fixture.handler.Receive(context.Background(), application.ReceiveManifestCommand{
		TenantID: tenant,
		Spec:     manifestSpec(t, "v1"),
	}); err != nil {
		t.Fatalf("receive: %v", err)
	}

	sameNumber := manifestSpec(t, "v7")
	sameNumber.Carrier = second
	sameNumber.Scope = mustValue(t, domain.NewDecisionScopeReference, "movement-7")
	other, err := fixture.handler.Receive(context.Background(), application.ReceiveManifestCommand{TenantID: tenant, Spec: sameNumber})
	if err != nil {
		t.Fatalf("second carrier receive: %v", err)
	}
	if other.Outcome() != application.ManifestPendingAssociation {
		t.Fatalf("outcome = %q, want PENDING_ASSOCIATION——答复不该因为别家用过这个号而不同", other.Outcome())
	}
	if reference, _ := other.Reference(); reference.Carrier() != second || reference.Version().String() != "v7" {
		t.Fatalf("交回的不是第二家自己的引用：%+v", reference)
	}

	advanced, err := fixture.handler.Revise(context.Background(), application.ReviseManifestCommand{
		TenantID:   tenant,
		Carrier:    second,
		Manifest:   number,
		Version:    mustValue(t, domain.NewManifestSourceVersion, "v8"),
		Scope:      mustValue(t, domain.NewDecisionScopeReference, "movement-8"),
		SourceFact: "SECOND-FEED/manifest-amended",
		At:         manifestAt,
	})
	if err != nil || advanced.Outcome() != application.ManifestRevised {
		t.Fatalf("second carrier revise: outcome = %q err = %v", advanced.Outcome(), err)
	}
	kept := fixture.store.byManifest[manifestRow(first, number)]
	if kept.Version().String() != "v1" || kept.Scope().String() != "movement-1" {
		t.Fatalf("第一家的引用被第二家动了：version=%q scope=%q", kept.Version(), kept.Scope())
	}

	unknown, err := fixture.handler.Revise(context.Background(), application.ReviseManifestCommand{
		TenantID:   tenant,
		Carrier:    mustValue(t, domain.NewCarrierResponsibilityReference, "carrier-responsibility-3"),
		Manifest:   number,
		Version:    mustValue(t, domain.NewManifestSourceVersion, "v2"),
		Scope:      mustValue(t, domain.NewDecisionScopeReference, "movement-9"),
		SourceFact: "THIRD-FEED/manifest-amended",
		At:         manifestAt,
	})
	if err != nil || unknown.Outcome() != application.ManifestNotFound {
		t.Fatalf("没交过这个号的承运商推进：outcome = %q err = %v, want NOT_FOUND", unknown.Outcome(), err)
	}
}

// Covers: UC-CC-012「同一来源身份/版本携带不同内容形成冲突」与「不按接收顺序决定」——同一家承运商的同一个号、同一个
// 版本再次到达而范围不同，答内容冲突、不交回在册内容、在册那一份不动；内容相同只是报文身份换了的才是重放。
func TestTheSameVersionArrivingWithDifferentContentIsAConflictNotAReplay(t *testing.T) {
	fixture := newManifestFixture(t)
	tenant := mustValue(t, domain.NewTenantID, "tenant-1")
	if _, err := fixture.handler.Receive(context.Background(), application.ReceiveManifestCommand{TenantID: tenant, Spec: manifestSpec(t, "v1")}); err != nil {
		t.Fatalf("receive: %v", err)
	}

	resent := manifestSpec(t, "v1")
	resent.SourceFact = "CARRIER-FEED/manifest-filed-again"
	replayed, err := fixture.handler.Receive(context.Background(), application.ReceiveManifestCommand{TenantID: tenant, Spec: resent})
	if err != nil || replayed.Outcome() != application.ManifestExisting {
		t.Fatalf("同内容重发：outcome = %q err = %v, want EXISTING", replayed.Outcome(), err)
	}

	changed := manifestSpec(t, "v1")
	changed.Scope = mustValue(t, domain.NewDecisionScopeReference, "movement-2")
	conflicted, err := fixture.handler.Receive(context.Background(), application.ReceiveManifestCommand{TenantID: tenant, Spec: changed})
	if err != nil || conflicted.Outcome() != application.ManifestContentConflict {
		t.Fatalf("同版本异范围：outcome = %q err = %v, want CONTENT_CONFLICT", conflicted.Outcome(), err)
	}
	if _, has := conflicted.Reference(); has {
		t.Fatal("内容冲突不该交回在册引用")
	}
	carrier := mustValue(t, domain.NewCarrierResponsibilityReference, "carrier-responsibility-1")
	if kept := fixture.store.byManifest[manifestRow(carrier, changed.Manifest)]; kept.Scope().String() != "movement-1" {
		t.Fatalf("在册范围 = %q，被后到的同版本覆盖了", kept.Scope())
	}
}

// Covers: UC-CC-012「更正、替代……乱序到达时全部保留，……不按接收顺序决定」的并发面——两笔推进都读到 v1：先落的一笔
// 成为 v2；后落的一笔指回的前版已不在册，答版本冲突，不静默盖掉 v2。
func TestALateRevisionBasedOnAStaleVersionDoesNotOverwriteTheEarlierOne(t *testing.T) {
	fixture := newManifestFixture(t)
	tenant := mustValue(t, domain.NewTenantID, "tenant-1")
	received, err := fixture.handler.Receive(context.Background(), application.ReceiveManifestCommand{TenantID: tenant, Spec: manifestSpec(t, "v1")})
	if err != nil {
		t.Fatalf("receive: %v", err)
	}
	stale, _ := received.Reference()
	carrier := mustValue(t, domain.NewCarrierResponsibilityReference, "carrier-responsibility-1")
	number := mustValue(t, domain.NewExternalManifestID, "carrier-manifest-1")

	// 第三笔推进在读到 v1 之后、落库之前被第二笔抢先：用一只在读时交回旧版的库模拟这个窗口。
	racing := application.NewReceiveManifestHandler(application.ReceiveManifestDeps{
		Candidates: fixture.candidates,
		Store:      staleReadStore{manifestStoreDouble: fixture.store, stale: stale},
		Downstream: &manifestDownstreamDouble{},
		Clock:      fixedClock{at: manifestAt},
	})
	revise := func(handler *application.ReceiveManifestHandler, version, scope string) application.ManifestResult {
		t.Helper()
		result, err := handler.Revise(context.Background(), application.ReviseManifestCommand{
			TenantID:   tenant,
			Carrier:    carrier,
			Manifest:   number,
			Version:    mustValue(t, domain.NewManifestSourceVersion, version),
			Scope:      mustValue(t, domain.NewDecisionScopeReference, scope),
			SourceFact: "CARRIER-FEED/" + version,
			At:         manifestAt,
		})
		if err != nil {
			t.Fatalf("revise %s: %v", version, err)
		}
		return result
	}
	if first := revise(fixture.handler, "v2", "movement-2"); first.Outcome() != application.ManifestRevised {
		t.Fatalf("先落的推进：outcome = %q", first.Outcome())
	}
	if late := revise(racing, "v3", "movement-3"); late.Outcome() != application.ManifestVersionConflict {
		t.Fatalf("后落的推进：outcome = %q, want VERSION_CONFLICT——它指回的 v1 已经不是在册版本", late.Outcome())
	}
	if kept := fixture.store.byManifest[manifestRow(carrier, number)]; kept.Version().String() != "v2" {
		t.Fatalf("在册版本 = %q，v2 被后到的 v3 盖掉了", kept.Version())
	}
}

// staleReadStore 读时交回一份早先读到的旧版，写仍落到真替身上：模拟「读到前版之后、落新版之前另一笔先落了」。
type staleReadStore struct {
	*manifestStoreDouble
	stale domain.ExternalManifestReference
}

func (store staleReadStore) FindByManifest(
	context.Context, domain.TenantID, domain.CarrierResponsibilityReference, domain.ExternalManifestID,
) (domain.ExternalManifestReference, bool, error) {
	return store.stale, true, nil
}

type manualManifestLedgerDouble struct {
	entries []ports.ManualManifestEntry
	err     error
}

func (double *manualManifestLedgerDouble) RecordManualManifestEntry(_ context.Context, entry ports.ManualManifestEntry) error {
	if double.err != nil {
		return double.err
	}
	double.entries = append(double.entries, entry)
	return nil
}

func manualManifestFixture(t *testing.T, ledger *manualManifestLedgerDouble) *manifestFixture {
	t.Helper()
	fixture := newManifestFixture(t)
	fixture.handler = application.NewReceiveManifestHandler(application.ReceiveManifestDeps{
		Candidates: fixture.candidates, Store: fixture.store, Downstream: &manifestDownstreamDouble{},
		Clock: fixedClock{at: manifestAt}, ManualEntries: ledger,
	})
	return fixture
}

// Covers: dox/decisions.md「产品形态」——操作员照承运商给的材料录入外部舱单引用：进同一只编排、落同一条引用，首登与
// 版本推进各记一条留痕（哪一版、谁录的、凭哪份原件）；原样重录是重放、不记第二条；承运商系统随后送来同一版同内容也是
// 重放——一份舱单只有一条引用，不因谁录的分成两条。有录入人没有原件（或反过来）不受理；留痕落不下答未决；没接留痕册
// 是装配缺陷、上抛。
func TestAManuallyEnteredManifestReferenceIsTheSameReferenceWithAnEntryTrailPerVersion(t *testing.T) {
	ledger := &manualManifestLedgerDouble{}
	fixture := manualManifestFixture(t, ledger)
	ctx := context.Background()
	tenant := mustValue(t, domain.NewTenantID, "tenant-1")
	manual := application.ReceiveManifestCommand{TenantID: tenant, Spec: manifestSpec(t, "v1"),
		EnteredBy: "OPERATOR/https://issuer#alice", Original: "SYN-CARRIER-MANIFEST-MAIL-1"}

	received, err := fixture.handler.Receive(ctx, manual)
	if err != nil || received.Outcome() != application.ManifestPendingAssociation || len(ledger.entries) != 1 {
		t.Fatalf("人工首登：err=%v outcome=%q 留痕 %d 条", err, received.Outcome(), len(ledger.entries))
	}
	if entry := ledger.entries[0]; entry.Version.String() != "v1" || entry.Manifest.String() != "carrier-manifest-1" ||
		entry.Carrier.String() != "carrier-responsibility-1" || entry.EnteredBy != "OPERATOR/https://issuer#alice" || entry.Original != "SYN-CARRIER-MANIFEST-MAIL-1" {
		t.Fatalf("留痕 = %+v", entry)
	}
	if replay, err := fixture.handler.Receive(ctx, manual); err != nil || replay.Outcome() != application.ManifestExisting || len(ledger.entries) != 1 {
		t.Fatalf("原样重录：err=%v outcome=%q 留痕 %d 条", err, replay.Outcome(), len(ledger.entries))
	}
	if fromCarrier, err := fixture.handler.Receive(ctx, application.ReceiveManifestCommand{TenantID: tenant, Spec: manifestSpec(t, "v1")}); err != nil ||
		fromCarrier.Outcome() != application.ManifestExisting {
		t.Fatalf("承运商系统送来同一版：err=%v outcome=%q", err, fromCarrier.Outcome())
	}

	revise := application.ReviseManifestCommand{
		TenantID: tenant, Carrier: mustValue(t, domain.NewCarrierResponsibilityReference, "carrier-responsibility-1"),
		Manifest: mustValue(t, domain.NewExternalManifestID, "carrier-manifest-1"), Version: mustValue(t, domain.NewManifestSourceVersion, "v2"),
		Scope: mustValue(t, domain.NewDecisionScopeReference, "movement-2"), SourceFact: "SYN-CARRIER-MANIFEST-MAIL-2", At: manifestAt,
		EnteredBy: "OPERATOR/https://issuer#bob", Original: "SYN-CARRIER-MANIFEST-MAIL-2",
	}
	if revised, err := fixture.handler.Revise(ctx, revise); err != nil || revised.Outcome() != application.ManifestRevised ||
		len(ledger.entries) != 2 || ledger.entries[1].Version.String() != "v2" || ledger.entries[1].EnteredBy != "OPERATOR/https://issuer#bob" {
		t.Fatalf("人工版本推进：err=%v outcome=%q 留痕 %+v", err, revised.Outcome(), ledger.entries)
	}

	// 同一版再推进：范围相同是重放；范围不同是内容冲突，不答已在册——后到的一方不该以为自己那一份落了。
	if same, err := fixture.handler.Revise(ctx, revise); err != nil || same.Outcome() != application.ManifestExisting {
		t.Fatalf("同一版同范围再推进：err=%v outcome=%q", err, same.Outcome())
	}
	fromCarrier := revise
	fromCarrier.EnteredBy, fromCarrier.Original = "", ""
	fromCarrier.Scope = mustValue(t, domain.NewDecisionScopeReference, "movement-other")
	if conflict, err := fixture.handler.Revise(ctx, fromCarrier); err != nil || conflict.Outcome() != application.ManifestContentConflict || len(ledger.entries) != 2 {
		t.Fatalf("承运商系统对同一版声明另一个范围：err=%v outcome=%q", err, conflict.Outcome())
	}
	// 推回先前录过的版本号：是又一次录入，再记一条留痕。
	back := revise
	back.Version, back.Original = mustValue(t, domain.NewManifestSourceVersion, "v1"), "SYN-CARRIER-MANIFEST-MAIL-3"
	if again, err := fixture.handler.Revise(ctx, back); err != nil || again.Outcome() != application.ManifestRevised || len(ledger.entries) != 3 {
		t.Fatalf("推回先前的版本号：err=%v outcome=%q 留痕 %d 条", err, again.Outcome(), len(ledger.entries))
	}

	for name, mutate := range map[string]func(*application.ReceiveManifestCommand){
		"有录入人没有原件": func(command *application.ReceiveManifestCommand) { command.Original = " " },
		"有原件没有录入人": func(command *application.ReceiveManifestCommand) { command.EnteredBy = "" },
	} {
		fresh := manualManifestFixture(t, &manualManifestLedgerDouble{})
		command := manual
		mutate(&command)
		if refused, err := fresh.handler.Receive(ctx, command); err != nil || refused.Outcome() != application.ManifestNotAccepted || len(fresh.store.byManifest) != 0 {
			t.Fatalf("%s：err=%v outcome=%q", name, err, refused.Outcome())
		}
	}
	failing := manualManifestFixture(t, &manualManifestLedgerDouble{err: errors.New("ledger down")})
	if undecided, err := failing.handler.Receive(ctx, manual); err != nil || undecided.Outcome() != application.ManifestUndecided {
		t.Fatalf("留痕落不下：err=%v outcome=%q", err, undecided.Outcome())
	}
	if _, err := newManifestFixture(t).handler.Receive(ctx, manual); err == nil {
		t.Fatal("没接留痕册的人工录入没有上抛")
	}
}
