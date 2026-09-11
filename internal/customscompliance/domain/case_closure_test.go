package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
)

var closureCutoffAt = time.Date(2026, 8, 12, 15, 0, 0, 0, time.UTC)

func obligation(name string, state domain.ObligationItemState) domain.ClosureObligationItem {
	item := domain.ClosureObligationItem{
		Obligation: name,
		Scope:      "declaration-unit-1",
		State:      state,
		Basis:      "basis/" + name,
	}
	if state == domain.ObligationHandedOver {
		item.HandedTo = "successor-team"
	}
	return item
}

// Covers: CC CONTEXT「关务案件关闭核对……可以证明案件可关闭或指出未决项，但不等于已经形成关闭决定」与 CONTEXT「只有每项依据均为已终结或已被有权接收方有效承接时，案件才可关闭；任一未解决或冲突项都阻止关闭」——未决清单列全、关闭被独立哨兵挡；
// 承接项必须指名接收方（CONTEXT「发送交接、技术送达……都不能证明全部责任已经移交」）。
func TestClosureIsBlockedByAnyUnresolvedItem(t *testing.T) {
	blocked, err := domain.VerifyClosure(mustValue(t, domain.NewCustomsCaseID, "case-1"), closureCutoffAt, []domain.ClosureObligationItem{
		obligation("declaration-submitted", domain.ObligationConcluded),
		obligation("duty-settled", domain.ObligationUnresolved),
		obligation("disposition-executed", domain.ObligationHandedOver),
	}, closureCutoffAt.Add(time.Hour))
	if err != nil {
		t.Fatalf("verify closure: %v", err)
	}
	if blocked.Closable() {
		t.Fatal("有未解决项还说可关闭")
	}
	if len(blocked.UnresolvedItems()) != 1 || blocked.UnresolvedItems()[0].Obligation != "duty-settled" {
		t.Fatalf("unresolved = %v", blocked.UnresolvedItems())
	}
	if _, err := domain.CloseCase(blocked, "customs-owner", closureCutoffAt.Add(2*time.Hour)); !errors.Is(err, domain.ErrClosureBlocked) {
		t.Fatalf("err = %v; 未决项没有挡住关闭", err)
	}

	handedWithoutReceiver := domain.ClosureObligationItem{
		Obligation: "duty-settled",
		Scope:      "declaration-unit-1",
		State:      domain.ObligationHandedOver,
		Basis:      "basis/duty-settled",
	}
	if _, err := domain.VerifyClosure(mustValue(t, domain.NewCustomsCaseID, "case-1"), closureCutoffAt,
		[]domain.ClosureObligationItem{handedWithoutReceiver}, closureCutoffAt.Add(time.Hour)); !errors.Is(err, domain.ErrInvalidClosure) {
		t.Fatalf("err = %v; 不指名接收方的承接被收下了", err)
	}
}

// Covers: CC CONTEXT 生命周期「进行中 → 已关闭：只在当前关闭核对覆盖全部适用义务……由有权责任角色形成关闭决定时成立」「已关闭 → 重新打开……依据使当前关闭期成立的关闭决定、受影响关闭依据项、相应原责任来源及当前授权形成受控重开决定；原关闭记录和关闭期间事实继续保留」
// ——可关闭的核对加决定人成立关闭；重开三件必备且受影响项必须指向真实义务；重开是
// 追加，原关闭记录不动。
func TestReopeningIsControlledAndAppendsOnly(t *testing.T) {
	verification, err := domain.VerifyClosure(mustValue(t, domain.NewCustomsCaseID, "case-1"), closureCutoffAt, []domain.ClosureObligationItem{
		obligation("declaration-submitted", domain.ObligationConcluded),
		obligation("duty-settled", domain.ObligationConcluded),
	}, closureCutoffAt.Add(time.Hour))
	if err != nil {
		t.Fatalf("verify closure: %v", err)
	}
	closure, err := domain.CloseCase(verification, "customs-owner", closureCutoffAt.Add(2*time.Hour))
	if err != nil {
		t.Fatalf("close case: %v", err)
	}

	if err := closure.Reopen(domain.ControlledReopening{
		LateFact:      "late-regulatory-correction/9",
		AffectedItems: []string{"duty-settled"},
		Authority:     "reopen-authority/1",
		ReopenedAt:    closureCutoffAt.Add(30 * 24 * time.Hour),
	}); err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if len(closure.Reopenings()) != 1 {
		t.Fatalf("reopenings = %d", len(closure.Reopenings()))
	}
	if closure.ClosedAt().IsZero() || !closure.Verification().Closable() {
		t.Fatal("重开改写了原关闭记录")
	}

	if err := closure.Reopen(domain.ControlledReopening{
		LateFact:      "another-late-fact",
		AffectedItems: []string{"never-existed"},
		Authority:     "reopen-authority/2",
		ReopenedAt:    closureCutoffAt.Add(31 * 24 * time.Hour),
	}); !errors.Is(err, domain.ErrInvalidReopening) {
		t.Fatalf("err = %v; 指不着真实义务的重开被收下了", err)
	}
	if err := closure.Reopen(domain.ControlledReopening{
		LateFact:      "missing-authority",
		AffectedItems: []string{"duty-settled"},
		ReopenedAt:    closureCutoffAt.Add(31 * 24 * time.Hour),
	}); !errors.Is(err, domain.ErrInvalidReopening) {
		t.Fatalf("err = %v; 没有授权的重开被收下了", err)
	}
	if err := closure.Reopen(domain.ControlledReopening{
		LateFact:      "time-travel",
		AffectedItems: []string{"duty-settled"},
		Authority:     "reopen-authority/3",
		ReopenedAt:    closureCutoffAt.Add(time.Hour),
	}); !errors.Is(err, domain.ErrInvalidReopening) {
		t.Fatalf("err = %v; 早于关闭的重开被收下了", err)
	}
}
