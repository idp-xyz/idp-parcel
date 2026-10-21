package domain_test

import (
	"slices"
	"testing"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 本文件证「执行方看得到哪几格」那一处判法的领域半边（ADR-0237）：读数照旧记全，回给执行方的逐对象那一格只列没得出放行的对象。
// 对象都是合成串，证据只记 `S`。

type shownCell struct {
	object  string
	answer  domain.GateAnswer
	unit    bool
	members []string
}

func shownCells(findings []domain.GateFinding) []shownCell {
	var cells []shownCell
	for _, cell := range domain.ExecutorGateCells(findings) {
		shown := shownCell{object: cell.Object.String(), answer: cell.Answer, unit: cell.Unit}
		for _, member := range cell.Members {
			shown.members = append(shown.members, member.String())
		}
		cells = append(cells, shown)
	}
	return cells
}

func sameCells(left, right []shownCell) bool {
	return slices.EqualFunc(left, right, func(a, b shownCell) bool {
		return a.object == b.object && a.answer == b.answer && a.unit == b.unit && slices.Equal(a.members, b.members)
	})
}

// Covers: AT-TF-316、AT-TF-317 领域半边——五格里只有阻断、读不动、未决回给执行方，放行与不受门禁不列；读数本身照旧每件一格。
// 全都得出放行时回给执行方的那几格为空，整格照实。
func TestOnlyCellsThatDidNotPassAreShownToTheExecutor(t *testing.T) {
	findings := []domain.GateFinding{
		gateFinding(t, "parcel-admitted", domain.GateAdmitted),
		gateFinding(t, "parcel-blocked", domain.GateBlocked),
		gateFinding(t, "parcel-not-guarded", domain.GateNotGuarded),
		gateFinding(t, "parcel-undecided", domain.GateUndecided),
		gateFinding(t, "parcel-unreadable", domain.GateUnreadable),
	}
	reading, err := departureReadingOf(findings...)
	if err != nil {
		t.Fatalf("读数：%v", err)
	}
	if len(reading.Findings()) != len(findings) {
		t.Fatalf("读数只剩 %d 格，读数本身要记全", len(reading.Findings()))
	}
	want := []shownCell{
		{object: "parcel-blocked", answer: domain.GateBlocked},
		{object: "parcel-undecided", answer: domain.GateUndecided},
		{object: "parcel-unreadable", answer: domain.GateUnreadable},
	}
	if got := shownCells(reading.Findings()); !sameCells(got, want) {
		t.Fatalf("回给执行方的 = %+v，want %+v", got, want)
	}

	clean, err := departureReadingOf(gateFinding(t, "parcel-1", domain.GateAdmitted), gateFinding(t, "parcel-2", domain.GateNotGuarded))
	if err != nil {
		t.Fatalf("读数：%v", err)
	}
	if got := shownCells(clean.Findings()); len(got) != 0 || clean.Answer() != domain.GateAdmitted {
		t.Fatalf("全都得出放行：回给执行方 %+v、整格 %s，want 空、放行", got, clean.Answer())
	}
}

// Covers: AT-TF-318 领域半边——集运单元那一格只指名同样回给执行方的成员：袋里一件被扣、一件不受门禁，袋在列、成员只剩被扣那一件，
// 不受门禁那一件哪儿都不出现；套在外层袋里的内层袋同样在列，列出的成员沿单元一路找得到段里在控的那一件。袋全都得出放行时袋与成员
// 都不列；袋因本侧问不到的成员没得出放行时袋在列、成员为空。
func TestAUnitCellNamesOnlyTheMembersThatAreShownToo(t *testing.T) {
	held := gateFinding(t, "parcel-held", domain.GateBlocked)
	clean := gateFinding(t, "parcel-clean", domain.GateNotGuarded)
	inner := unitFinding(t, "bag-inner", []domain.GateFinding{clean, held})
	other := gateFinding(t, "parcel-other", domain.GateAdmitted)
	outer := unitFinding(t, "bag-outer", []domain.GateFinding{inner, other})

	reading, err := departureReadingOf(outer, inner, held, clean, other)
	if err != nil {
		t.Fatalf("读数：%v", err)
	}
	want := []shownCell{
		{object: "bag-inner", answer: domain.GateBlocked, unit: true, members: []string{"parcel-held"}},
		{object: "bag-outer", answer: domain.GateBlocked, unit: true, members: []string{"bag-inner"}},
		{object: "parcel-held", answer: domain.GateBlocked},
	}
	if got := shownCells(reading.Findings()); !sameCells(got, want) {
		t.Fatalf("回给执行方的 = %+v，want %+v", got, want)
	}

	cleanBag := unitFinding(t, "bag-clean", []domain.GateFinding{clean})
	if got := shownCells([]domain.GateFinding{cleanBag, clean}); len(got) != 0 {
		t.Fatalf("全都不受门禁的袋 = %+v，袋与成员都不该列", got)
	}

	unresolved := unitFinding(t, "bag-unresolved", []domain.GateFinding{clean},
		domain.ConsolidationMemberGap{Stop: domain.ConsolidationMemberIdentityUnresolved, Sources: []string{"hu-9"}})
	empty := unitFinding(t, "bag-empty", nil)
	want = []shownCell{
		{object: "bag-empty", answer: domain.GateUndecided, unit: true},
		{object: "bag-unresolved", answer: domain.GateUndecided, unit: true},
	}
	if got := shownCells([]domain.GateFinding{empty, unresolved, clean}); !sameCells(got, want) {
		t.Fatalf("问不到成员的袋与空袋 = %+v，want %+v", got, want)
	}
}
