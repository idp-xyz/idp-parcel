package domain_test

import (
	"testing"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
)

func lineageOf(t *testing.T, ids ...string) domain.ContainmentLineage {
	t.Helper()
	lineage := make(domain.ContainmentLineage, 0, len(ids))
	for _, id := range ids {
		lineage = append(lineage, mustValue(t, domain.NewConsolidationUnitID, id))
	}
	return lineage
}

// Covers: NO CONTEXT「集运包含关系必须无循环；……嵌套集运时通过直接父级链路派生间接包含」——沿目标单元的直接父级
// 链路（自身在先、到根为止）看拟移入的成员：成员就是目标自身或其某一级祖先即成环，交回从目标到那一级的那一段；不在
// 链路上不成环（`AT-NO-039`「集运单元嵌套形成循环→拒绝成员关系，原关系不被覆盖」的判法半边）。成员与单元按同串认。
func TestAMemberAlreadyOnTheLineageClosesACycle(t *testing.T) {
	lineage := lineageOf(t, "bag-c", "bag-b", "bag-a")

	for name, attempt := range map[string]struct {
		member  string
		closes  bool
		segment []string
	}{
		"成员是根":      {"bag-a", true, []string{"bag-c", "bag-b", "bag-a"}},
		"成员是中间一级祖先": {"bag-b", true, []string{"bag-c", "bag-b"}},
		"成员就是目标自身":  {"bag-c", true, []string{"bag-c"}},
		"成员是另一只袋":   {"bag-x", false, nil},
		"成员是一件包裹":   {"hu-1", false, nil},
	} {
		t.Run(name, func(t *testing.T) {
			segment, closes := lineage.ClosesCycle(mustValue(t, domain.NewHandlingUnitID, attempt.member))
			if closes != attempt.closes {
				t.Fatalf("closes = %v，要 %v", closes, attempt.closes)
			}
			if len(segment) != len(attempt.segment) {
				t.Fatalf("segment = %v，要 %v", segment, attempt.segment)
			}
			for index, id := range attempt.segment {
				if segment[index].String() != id {
					t.Fatalf("segment[%d] = %s，要 %s", index, segment[index], id)
				}
			}
		})
	}

	if segment, closes := domain.ContainmentLineage(nil).ClosesCycle(mustValue(t, domain.NewHandlingUnitID, "bag-a")); closes || segment != nil {
		t.Fatalf("空链路判出了环：%v %v", segment, closes)
	}
}
