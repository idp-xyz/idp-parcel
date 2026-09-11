package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
)

var manifestAcceptedAt = time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)

func manifestReference(t *testing.T) domain.ExternalManifestReference {
	t.Helper()
	reference, err := domain.AcceptManifestReference(domain.ExternalManifestReferenceSpec{
		Manifest:   mustValue(t, domain.NewExternalManifestID, "carrier-manifest/MAWB-123"),
		Version:    mustValue(t, domain.NewManifestSourceVersion, "manifest/v1"),
		Carrier:    mustValue(t, domain.NewCarrierResponsibilityReference, "carrier-1/regulatory-manifest"),
		Procedure:  mustValue(t, domain.NewCustomsProcedureReference, "US-IMPORT/TYPE-86"),
		Direction:  domain.ImportManifest,
		Scope:      mustValue(t, domain.NewDecisionScopeReference, "manifest-scope-1"),
		SourceFact: "carrier-report/submitted",
		AcceptedAt: manifestAcceptedAt,
	})
	if err != nil {
		t.Fatalf("accept manifest reference: %v", err)
	}
	return reference
}

func candidate(t *testing.T, unit, scope string, direction domain.ManifestDirection) domain.AssociationCandidate {
	t.Helper()
	return domain.AssociationCandidate{
		Unit:      mustValue(t, domain.NewDeclarationUnitID, unit),
		Procedure: mustValue(t, domain.NewCustomsProcedureReference, "US-IMPORT/TYPE-86"),
		Direction: direction,
		Scope:     mustValue(t, domain.NewDecisionScopeReference, scope),
	}
}

// Covers: CC CONTEXT「接受外部监管舱单引用、来源版本、明确范围、变更关系和当前采用关系，不形成本地监管舱单草稿、提交版本或提交尝试」与八件形状——承运商责任、程序、方向、范围、来源事实缺一
// 立不起；类型上没有草稿/提交字段（只引用是结构性的）。
func TestAManifestReferenceIsAcceptedNotAuthored(t *testing.T) {
	reference := manifestReference(t)
	if reference.Carrier().String() != "carrier-1/regulatory-manifest" {
		t.Fatalf("carrier = %s", reference.Carrier())
	}
	if _, associated := reference.Association(); associated {
		t.Fatal("刚接受的引用凭空有了关联")
	}

	missingCarrier := domain.ExternalManifestReferenceSpec{
		Manifest:   mustValue(t, domain.NewExternalManifestID, "carrier-manifest/MAWB-123"),
		Version:    mustValue(t, domain.NewManifestSourceVersion, "manifest/v1"),
		Procedure:  mustValue(t, domain.NewCustomsProcedureReference, "US-IMPORT/TYPE-86"),
		Direction:  domain.ImportManifest,
		Scope:      mustValue(t, domain.NewDecisionScopeReference, "manifest-scope-1"),
		SourceFact: "carrier-report/submitted",
		AcceptedAt: manifestAcceptedAt,
	}
	if _, err := domain.AcceptManifestReference(missingCarrier); !errors.Is(err, domain.ErrInvalidManifestReference) {
		t.Fatalf("err = %v; 没有承运商责任的引用被收下了", err)
	}
}

// Covers: CC CONTEXT 生命周期「逐范围唯一匹配 → 形成业务关联和当前采用关系；无法唯一匹配时保持待关联，不创建占位对象或按最近客户、班次猜测」与 CONTEXT「同一编号、同袋、同总单……不自动证明它们是同一对象」——三维全符恰一个才关联；零匹配与多匹配都独立
// 哨兵拒（方向不符的候选不算匹配）。
func TestAssociationDemandsAUniqueThreeDimensionalMatch(t *testing.T) {
	reference := manifestReference(t)

	associated, err := reference.Associate([]domain.AssociationCandidate{
		candidate(t, "declaration-unit-1", "manifest-scope-1", domain.ImportManifest),
		candidate(t, "declaration-unit-2", "manifest-scope-9", domain.ImportManifest),
		candidate(t, "declaration-unit-3", "manifest-scope-1", domain.ExportManifest),
	})
	if err != nil {
		t.Fatalf("associate: %v", err)
	}
	unit, present := associated.Association()
	if !present || unit.String() != "declaration-unit-1" {
		t.Fatalf("association = %s present = %v", unit, present)
	}

	if _, err := reference.Associate([]domain.AssociationCandidate{
		candidate(t, "declaration-unit-2", "manifest-scope-9", domain.ImportManifest),
	}); !errors.Is(err, domain.ErrManifestNotUniquelyMatched) {
		t.Fatalf("err = %v; 零匹配创建了占位关联", err)
	}
	if _, err := reference.Associate([]domain.AssociationCandidate{
		candidate(t, "declaration-unit-1", "manifest-scope-1", domain.ImportManifest),
		candidate(t, "declaration-unit-4", "manifest-scope-1", domain.ImportManifest),
	}); !errors.Is(err, domain.ErrManifestNotUniquelyMatched) {
		t.Fatalf("err = %v; 多匹配靠猜选了一个", err)
	}
	if _, err := associated.Associate([]domain.AssociationCandidate{
		candidate(t, "declaration-unit-1", "manifest-scope-1", domain.ImportManifest),
	}); !errors.Is(err, domain.ErrInvalidManifestReference) {
		t.Fatalf("err = %v; 已关联的引用又关联了一次", err)
	}
}

// Covers: CC CONTEXT「外部监管舱单更正、撤销、替代或范围变化形成新的引用关系和当前采用判断，原来源、版本、范围和关联历史不得覆盖」——修订换版本换范围指回
// 原版、原引用不可变；关联不随版本自动搬移（新版本重新走唯一匹配）；重号修订拒。
func TestRevisionKeepsHistoryAndDropsTheStaleAssociation(t *testing.T) {
	reference := manifestReference(t)
	associated, err := reference.Associate([]domain.AssociationCandidate{
		candidate(t, "declaration-unit-1", "manifest-scope-1", domain.ImportManifest),
	})
	if err != nil {
		t.Fatalf("associate: %v", err)
	}

	revised, err := associated.Revise(
		mustValue(t, domain.NewManifestSourceVersion, "manifest/v2"),
		mustValue(t, domain.NewDecisionScopeReference, "manifest-scope-2"),
		"carrier-report/corrected",
		manifestAcceptedAt.Add(6*time.Hour),
	)
	if err != nil {
		t.Fatalf("revise: %v", err)
	}
	prior, present := revised.PriorVersion()
	if !present || prior.String() != "manifest/v1" {
		t.Fatalf("prior = %s present = %v", prior, present)
	}
	if _, stillAssociated := revised.Association(); stillAssociated {
		t.Fatal("关联随版本自动搬移了——新版本必须重新走唯一匹配")
	}
	if associated.Version().String() != "manifest/v1" {
		t.Fatal("原引用被改写了")
	}
	if unit, present := associated.Association(); !present || unit.String() != "declaration-unit-1" {
		t.Fatal("原关联历史被抹掉了")
	}

	if _, err := associated.Revise(
		associated.Version(),
		mustValue(t, domain.NewDecisionScopeReference, "manifest-scope-2"),
		"carrier-report/corrected",
		manifestAcceptedAt.Add(7*time.Hour),
	); !errors.Is(err, domain.ErrInvalidManifestReference) {
		t.Fatalf("err = %v; 重号修订分不出两版", err)
	}
}
