package application_test

import (
	"context"
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

func (double *manifestStoreDouble) FindByManifest(
	_ context.Context,
	_ domain.TenantID,
	manifest domain.ExternalManifestID,
) (domain.ExternalManifestReference, bool, error) {
	reference, found := double.byManifest[manifest.String()]
	return reference, found, nil
}

func (double *manifestStoreDouble) Save(
	_ context.Context,
	_ domain.TenantID,
	reference domain.ExternalManifestReference,
) (ports.ManifestSaveOutcome, error) {
	if _, exists := double.byManifest[reference.Manifest().String()]; exists {
		return ports.ManifestAlreadyRecorded, nil
	}
	double.byManifest[reference.Manifest().String()] = reference
	return ports.ManifestSaved, nil
}

func (double *manifestStoreDouble) Update(
	_ context.Context,
	_ domain.TenantID,
	reference domain.ExternalManifestReference,
) error {
	double.byManifest[reference.Manifest().String()] = reference
	return nil
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

// Covers: CC CONTEXT 生命周期 258「能够唯一匹配→形成业务关联；无法唯一匹配时保持
// 待关联，不创建占位对象或按最近客户、班次猜测」的编排面——恰一候选关联、零候选与
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

// Covers: CC CONTEXT 硬句 150 的编排面——修订换版本换范围指回前版，关联不随版本自动
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
