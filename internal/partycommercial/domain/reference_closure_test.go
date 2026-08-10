package domain_test

import (
	"testing"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

func closureKey(t *testing.T, scope string, required ...domain.CommercialObjectKind) domain.ClosureResolutionKey {
	t.Helper()
	base := resolutionKey(t, scope, required[0])
	return domain.ClosureResolutionKey{
		TenantID:             base.TenantID,
		CustomerAccountID:    base.CustomerAccountID,
		LegalEntityCandidate: base.LegalEntityCandidate,
		Scope:                base.Scope,
		Purpose:              base.Purpose,
		Anchor:               base.Anchor,
		RequiredBases:        required,
	}
}

func seedClosure(t *testing.T, registry *domain.CommercialRegistry, scope string, kinds ...domain.CommercialObjectKind) {
	t.Helper()
	for index, kind := range kinds {
		effectiveIn(t, registry, kind, "object-"+kind.String(), "v1", "sha256:"+kind.String(), scope)
		_ = index
	}
}

var closureBases = []domain.CommercialObjectKind{
	domain.CustomerContractObject,
	domain.AcceptanceRulePackageObject,
	domain.SettlementPolicyObject,
}

// Covers: UC-PC-002 步骤 4 — 一次解析出合同引用的完整依据集合，每项各自唯一，并返回
// 采用版本、有效区间与当前修订标识。
func TestClosureResolvesEveryRequiredBasisUniquely(t *testing.T) {
	registry := domain.NewCommercialRegistry()
	seedClosure(t, registry, "scope-a", closureBases...)

	closure := domain.ResolveCommercialClosure(registry, closureKey(t, "scope-a", closureBases...))

	if closure.Outcome() != domain.UniquelyResolved {
		t.Fatalf("outcome = %q, want UNIQUELY_RESOLVED", closure.Outcome())
	}
	for _, kind := range closureBases {
		adopted, present := closure.AdoptedFor(kind)
		if !present || adopted.Kind() != kind {
			t.Fatalf("closure has no adopted version for %q", kind)
		}
	}
	if len(closure.Adopted()) != len(closureBases) {
		t.Fatalf("closure holds %d adopted versions, want %d", len(closure.Adopted()), len(closureBases))
	}
	if revision, present := closure.ViewRevision(); !present || revision.String() == "" {
		t.Fatal("closure carries no authority view revision")
	}
	if closure.ResolutionID().String() == "" {
		t.Fatal("a resolved closure carries no identity")
	}
}

// Covers: UC-PC-002 步骤 4「引用闭包不完整时未决或冲突」— 缺一项即整体不成立，且必须
// 指出缺的是哪一项，不得返回一个残缺的闭包让调用方自己发现。
func TestAnIncompleteClosureFailsAsAWholeAndNamesTheGap(t *testing.T) {
	registry := domain.NewCommercialRegistry()
	seedClosure(t, registry, "scope-a", domain.CustomerContractObject, domain.AcceptanceRulePackageObject)

	closure := domain.ResolveCommercialClosure(registry, closureKey(t, "scope-a", closureBases...))

	if closure.Outcome() != domain.NoApplicableBasis {
		t.Fatalf("outcome = %q, want NO_APPLICABLE_BASIS", closure.Outcome())
	}
	if len(closure.Adopted()) != 0 {
		t.Fatal("an incomplete closure still handed back partially adopted versions")
	}
	unresolved := closure.UnresolvedBases()
	if len(unresolved) != 1 || unresolved[0] != domain.SettlementPolicyObject {
		t.Fatalf("unresolved = %v, want exactly the settlement policy", unresolved)
	}
}

// Covers: UC-PC-002 — 任一必需依据存在多个候选时整体是适用冲突，不是部分成功。
func TestOneConflictingBasisMakesTheWholeClosureConflict(t *testing.T) {
	registry := domain.NewCommercialRegistry()
	seedClosure(t, registry, "scope-a", closureBases...)
	effectiveIn(t, registry, domain.SettlementPolicyObject, "rival-policy", "v1", "sha256:rival", "scope-a")

	closure := domain.ResolveCommercialClosure(registry, closureKey(t, "scope-a", closureBases...))

	if closure.Outcome() != domain.ApplicabilityConflict {
		t.Fatalf("outcome = %q, want APPLICABILITY_CONFLICT", closure.Outcome())
	}
	if len(closure.Adopted()) != 0 {
		t.Fatal("a conflicting closure still adopted the unambiguous members")
	}
	conflicting := closure.ConflictingBases()
	if len(conflicting) != 1 || conflicting[0] != domain.SettlementPolicyObject {
		t.Fatalf("conflicting = %v, want exactly the settlement policy", conflicting)
	}
}

// Covers: UC-PC-002 结果语义排序 — 同时存在冲突与缺失时报冲突，因为冲突要商业责任方
// 修正，而缺失只说明该范围没有对象；把冲突降级为缺失会让需要修正的问题看起来无需处理。
func TestConflictOutranksAMissingBasis(t *testing.T) {
	registry := domain.NewCommercialRegistry()
	seedClosure(t, registry, "scope-a", domain.CustomerContractObject, domain.SettlementPolicyObject)
	effectiveIn(t, registry, domain.SettlementPolicyObject, "rival-policy", "v1", "sha256:rival", "scope-a")

	closure := domain.ResolveCommercialClosure(registry, closureKey(t, "scope-a", closureBases...))

	if closure.Outcome() != domain.ApplicabilityConflict {
		t.Fatalf("outcome = %q, want APPLICABILITY_CONFLICT to outrank the missing rule package", closure.Outcome())
	}
	if len(closure.UnresolvedBases()) != 1 {
		t.Fatal("the closure lost track of the missing basis while reporting the conflict")
	}
}

// Covers: 本次设计约束 — 闭包装的是异构采用引用，重复声明同一必需依据是键的错误，
// 不是解析结果。
func TestClosureKeyRefusesADuplicateRequiredBasis(t *testing.T) {
	registry := domain.NewCommercialRegistry()
	seedClosure(t, registry, "scope-a", closureBases...)

	key := closureKey(t, "scope-a", domain.CustomerContractObject, domain.CustomerContractObject)
	if got := domain.ResolveCommercialClosure(registry, key).Outcome(); got != domain.InputNotAccepted {
		t.Fatalf("outcome = %q, want INPUT_NOT_ACCEPTED", got)
	}
}

// Covers: UC-PC-002 — 空的必需依据集合不构成一次解析请求。
func TestClosureKeyRequiresAtLeastOneBasis(t *testing.T) {
	registry := domain.NewCommercialRegistry()
	key := closureKey(t, "scope-a", domain.CustomerContractObject)
	key.RequiredBases = nil

	if got := domain.ResolveCommercialClosure(registry, key).Outcome(); got != domain.InputNotAccepted {
		t.Fatalf("outcome = %q, want INPUT_NOT_ACCEPTED", got)
	}
}

// Covers: UC-PC-002 — 权威读不到时整体未决，与「该范围没有适用对象」不同。
func TestUnreadableAuthorityMakesTheClosurePending(t *testing.T) {
	if got := domain.ResolveCommercialClosure(nil, closureKey(t, "scope-a", closureBases...)).Outcome(); got != domain.ResolutionPending {
		t.Fatalf("outcome = %q, want RESOLUTION_PENDING", got)
	}
}
