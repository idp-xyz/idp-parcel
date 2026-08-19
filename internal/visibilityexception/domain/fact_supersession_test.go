package domain_test

import (
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
)

// supersessionFact 造一份指名（事实引用+版本+前身）的已接受事实。前身空串即首登。
func supersessionFact(t *testing.T, factRef, version, supersedes string) domain.AcceptedSourceFact {
	t.Helper()
	spec := factSpec(t)
	spec.Source = domain.SourceTransportFulfillment
	spec.Fact = mustValue(t, domain.NewSourceFactReference, factRef)
	spec.Version = mustValue(t, domain.NewSourceFactVersion, version)
	if supersedes != "" {
		spec.Supersedes = mustValue(t, domain.NewSourceFactVersion, supersedes)
	}
	fact, err := domain.NewAcceptedSourceFact(spec)
	if err != nil {
		t.Fatalf("构造已接受事实 %s@%s：%v", factRef, version, err)
	}
	return fact
}

func effectiveVersions(facts []domain.AcceptedSourceFact) []string {
	versions := make([]string, 0, len(facts))
	for _, fact := range facts {
		versions = append(versions, fact.Fact().String()+"@"+fact.Version().String())
	}
	return versions
}

// Covers: VE CONTEXT「来源事实替代关系」词条——关系由源上下文随更正给出、指名被
// 取代的那一版；首登无前身如实答无。指名自己为前身是坏引用：沿用原版本号就是覆盖，
// 不是更正。
func TestAnAcceptedFactCarriesItsSourceGivenSupersession(t *testing.T) {
	corrected := supersessionFact(t, "effective-delivery/parcel-1/attempt-1", "delivery/v2", "delivery/v1")
	predecessor, given := corrected.Supersedes()
	if !given || predecessor.String() != "delivery/v1" {
		t.Fatalf("supersedes = %q given = %v; 源上下文给出的前身引用必须随事实保全",
			predecessor, given)
	}

	first := supersessionFact(t, "effective-delivery/parcel-1/attempt-1", "delivery/v1", "")
	if _, given := first.Supersedes(); given {
		t.Fatal("首登事实凭空长出了前身")
	}

	selfNaming := factSpec(t)
	selfNaming.Supersedes = selfNaming.Version
	if _, err := domain.NewAcceptedSourceFact(selfNaming); !errors.Is(err, domain.ErrInvalidSourceFact) {
		t.Fatalf("err = %v; 指名自己为前身的「替代」被收下了", err)
	}
}

// Covers: VE CONTEXT「『已被替代』是替代关系的派生问答……当且仅当另一份已接受事实
// 指名它为前身」与「替代先于被替代到达也不改变结论」——判断按在场事实集重新回答，
// 与输入顺序无关；被替代者不参与当前有效，未被指名者全数保留。
func TestSupersededFactsLeaveTheEffectiveSet(t *testing.T) {
	first := supersessionFact(t, "effective-delivery/parcel-1/attempt-1", "delivery/v1", "")
	corrected := supersessionFact(t, "effective-delivery/parcel-1/attempt-1", "delivery/v2", "delivery/v1")
	unrelated := supersessionFact(t, "node-intake/parcel-1", "intake/v1", "")

	for name, ordered := range map[string][]domain.AcceptedSourceFact{
		"前身先到": {first, corrected, unrelated},
		"替代先到": {corrected, unrelated, first},
	} {
		t.Run(name, func(t *testing.T) {
			effective := domain.CurrentlyEffective(ordered)
			if len(effective) != 2 {
				t.Fatalf("当前有效 = %v, want 2 份（v1 已被替代）", effectiveVersions(effective))
			}
			for _, fact := range effective {
				if fact.Version().String() == "delivery/v1" {
					t.Fatal("被指名为前身的 v1 仍在当前有效集里")
				}
			}
		})
	}

	// 链式更正：v3 指名 v2、v2 指名 v1，只有链尾当前有效。被替代的 v2 自己的指名
	// 不因此收回——源上下文给出的关系只增不删，v1 照样不回场。
	tail := supersessionFact(t, "effective-delivery/parcel-1/attempt-1", "delivery/v3", "delivery/v2")
	effective := domain.CurrentlyEffective([]domain.AcceptedSourceFact{first, corrected, tail})
	if len(effective) != 1 || effective[0].Version().String() != "delivery/v3" {
		t.Fatalf("当前有效 = %v, want 只剩 delivery/v3", effectiveVersions(effective))
	}
}

// Covers: VE CONTEXT「关系只在同一源上下文、同一事实引用的版本之间成立……跨源上下文
// 或跨事实引用的分歧仍是冲突，按冲突裁决处理，不得借替代关系代为裁决」——跨引用与
// 跨源的指名不构成替代，被指名者留在当前有效集。
func TestSupersessionDoesNotCrossFactReferenceOrSource(t *testing.T) {
	delivery := supersessionFact(t, "effective-delivery/parcel-1/attempt-1", "delivery/v1", "")
	crossReference := supersessionFact(t, "transport-handover/parcel-1/scope-1", "handover/v2", "delivery/v1")

	effective := domain.CurrentlyEffective([]domain.AcceptedSourceFact{delivery, crossReference})
	if len(effective) != 2 {
		t.Fatalf("当前有效 = %v; 跨事实引用的指名不得当替代采信", effectiveVersions(effective))
	}

	crossSourceSpec := factSpec(t)
	crossSourceSpec.Source = domain.SourceNodeOperations
	crossSourceSpec.Fact = mustValue(t, domain.NewSourceFactReference, "effective-delivery/parcel-1/attempt-1")
	crossSourceSpec.Version = mustValue(t, domain.NewSourceFactVersion, "intake/v9")
	crossSourceSpec.Supersedes = mustValue(t, domain.NewSourceFactVersion, "delivery/v1")
	crossSource, err := domain.NewAcceptedSourceFact(crossSourceSpec)
	if err != nil {
		t.Fatalf("构造跨源事实：%v", err)
	}
	effective = domain.CurrentlyEffective([]domain.AcceptedSourceFact{delivery, crossSource})
	if len(effective) != 2 {
		t.Fatalf("当前有效 = %v; 跨源上下文的指名不得当替代采信", effectiveVersions(effective))
	}
}

// Covers: `AT-VE-043`「含同一前身被两份事实同时指名的替代链分叉→保留双方和冲突关系，
// 不择一」——分叉不改变「前身已被替代」，各后继全部留在当前有效集；哪一份该作当前
// 呈现由冲突机制回答，本问答不替源上下文裁决。
func TestAForkedSupersessionChainKeepsAllSuccessors(t *testing.T) {
	first := supersessionFact(t, "effective-delivery/parcel-1/attempt-1", "delivery/v1", "")
	left := supersessionFact(t, "effective-delivery/parcel-1/attempt-1", "delivery/v2a", "delivery/v1")
	right := supersessionFact(t, "effective-delivery/parcel-1/attempt-1", "delivery/v2b", "delivery/v1")

	effective := domain.CurrentlyEffective([]domain.AcceptedSourceFact{first, left, right})
	if len(effective) != 2 {
		t.Fatalf("当前有效 = %v, want 两个后继都保留（不择一）", effectiveVersions(effective))
	}
	kept := map[string]bool{}
	for _, fact := range effective {
		kept[fact.Version().String()] = true
	}
	if !kept["delivery/v2a"] || !kept["delivery/v2b"] {
		t.Fatalf("当前有效 = %v; 分叉两方必须都保留", effectiveVersions(effective))
	}
}
