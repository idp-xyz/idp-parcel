package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/pilotgovernance/domain"
)

// 本文件证覆盖关系边的形状门与相容性判定：缺件与自指拒绝、承继有向而互不相干对称、
// 同对/反向对的冗余·相悖·独立三格、以及承继格照样拦新准入。

var relationAt = time.Date(2026, 8, 21, 8, 0, 0, 0, time.UTC)

func relationSpec(t *testing.T, successor, predecessor string, kind domain.ScopeVersionRelationKind) domain.ScopeVersionRelationSpec {
	t.Helper()
	return domain.ScopeVersionRelationSpec{
		Successor:    mustValue(t, domain.NewScopeVersionReference, successor),
		Predecessor:  mustValue(t, domain.NewScopeVersionReference, predecessor),
		Kind:         kind,
		Objective:    "enter-limited-production",
		Candidates:   mustValue(t, domain.NewCandidateVersionSetID, "candidate-set-1"),
		RegisteredAt: relationAt,
	}
}

func mustRelation(t *testing.T, successor, predecessor string, kind domain.ScopeVersionRelationKind) domain.ScopeVersionRelation {
	t.Helper()
	relation, err := domain.RegisterScopeVersionRelation(relationSpec(t, successor, predecessor, kind))
	if err != nil {
		t.Fatalf("登记关系边：%v", err)
	}
	return relation
}

func TestScopeVersionRelationRequiresEveryPartAndRefusesSelfEdges(t *testing.T) {
	complete := relationSpec(t, "pilot-scope/v2", "pilot-scope/v1", domain.ScopeInheritsSuspensions)
	if _, err := domain.RegisterScopeVersionRelation(complete); err != nil {
		t.Fatalf("齐件的边被拒：%v", err)
	}

	for name, mutate := range map[string]func(*domain.ScopeVersionRelationSpec){
		"后继空":  func(spec *domain.ScopeVersionRelationSpec) { spec.Successor = domain.ScopeVersionReference{} },
		"前代空":  func(spec *domain.ScopeVersionRelationSpec) { spec.Predecessor = domain.ScopeVersionReference{} },
		"种类无效": func(spec *domain.ScopeVersionRelationSpec) { spec.Kind = domain.ScopeVersionRelationKindInvalid },
		"目标空":  func(spec *domain.ScopeVersionRelationSpec) { spec.Objective = "   " },
		"候选组空": func(spec *domain.ScopeVersionRelationSpec) { spec.Candidates = domain.CandidateVersionSetID{} },
		"时点零":  func(spec *domain.ScopeVersionRelationSpec) { spec.RegisteredAt = time.Time{} },
		"自指边": func(spec *domain.ScopeVersionRelationSpec) {
			spec.Predecessor = mustValue(t, domain.NewScopeVersionReference, "pilot-scope/v2")
		},
	} {
		spec := relationSpec(t, "pilot-scope/v2", "pilot-scope/v1", domain.ScopeInheritsSuspensions)
		mutate(&spec)
		if _, err := domain.RegisterScopeVersionRelation(spec); !errors.Is(err, domain.ErrInvalidScopeRelation) {
			t.Errorf("%s：err = %v, 想要 ErrInvalidScopeRelation", name, err)
		}
	}
}

func TestCompareScopeVersionRelationsCoversAllPairings(t *testing.T) {
	inherits21 := mustRelation(t, "pilot-scope/v2", "pilot-scope/v1", domain.ScopeInheritsSuspensions)
	unrelated21 := mustRelation(t, "pilot-scope/v2", "pilot-scope/v1", domain.ScopeUnrelated)
	inherits12 := mustRelation(t, "pilot-scope/v1", "pilot-scope/v2", domain.ScopeInheritsSuspensions)
	unrelated12 := mustRelation(t, "pilot-scope/v1", "pilot-scope/v2", domain.ScopeUnrelated)
	inherits31 := mustRelation(t, "pilot-scope/v3", "pilot-scope/v1", domain.ScopeInheritsSuspensions)

	for name, probe := range map[string]struct {
		existing, declared domain.ScopeVersionRelation
		wants              domain.RelationComparison
	}{
		"同对同种是冗余":     {inherits21, inherits21, domain.RelationRedundant},
		"同对异种相悖":      {inherits21, unrelated21, domain.RelationsContradictory},
		"反向互不相干是同一事实": {unrelated12, unrelated21, domain.RelationRedundant},
		"反向承继撞互不相干相悖": {inherits12, unrelated21, domain.RelationsContradictory},
		"反向互不相干撞承继相悖": {unrelated12, inherits21, domain.RelationsContradictory},
		"双向承继各自成立":    {inherits12, inherits21, domain.RelationsIndependent},
		"不同对之间没有可比性":  {inherits31, inherits21, domain.RelationComparisonInvalid},
	} {
		if got := domain.CompareScopeVersionRelations(probe.existing, probe.declared); got != probe.wants {
			t.Errorf("%s：got %d, want %d", name, got, probe.wants)
		}
	}
}

func TestInheritedScopeGroundBlocksAdmission(t *testing.T) {
	if !domain.AdmissionSuspendedByInheritedScope.Blocks() {
		t.Fatal("承继格不拦新准入——承继边登了等于没登")
	}
	if domain.AdmissionSuspendedByInheritedScope.String() != "SUSPENDED_BY_INHERITED_SCOPE" {
		t.Fatalf("String() = %q", domain.AdmissionSuspendedByInheritedScope.String())
	}
}
