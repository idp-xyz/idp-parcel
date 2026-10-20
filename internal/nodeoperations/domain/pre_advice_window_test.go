package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
)

// 本文件证预报有效时窗的收口（票 ps-external-mark-relations/14 裁定）：收口是追加给开放终点那一行的另一份事实，写明收口时刻与
// 依据；那一行登记的内容不变，读「某时刻适用哪一行」时收口时刻当作它的终点。夹具全为合成（S 级）。

var (
	closureWindowFrom = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	closureAt         = time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	closureRecordedAt = time.Date(2026, 6, 20, 9, 0, 0, 0, time.UTC)
)

func closureSpec(t *testing.T, windowFrom, closedAt time.Time) domain.PreAdviceWindowClosureSpec {
	t.Helper()
	return domain.PreAdviceWindowClosureSpec{
		TenantID:     mustBuild(t, domain.NewTenantID, "SYN-TENANT-01"),
		WindowFrom:   windowFrom,
		ClosedAt:     closedAt,
		Basis:        "SYN-BASIS-PSXM14-CLOSURE",
		RegisteredAt: closureRecordedAt,
	}
}

func formClosure(t *testing.T, windowFrom, closedAt time.Time) domain.PreAdviceWindowClosure {
	t.Helper()
	closure, err := domain.FormPreAdviceWindowClosure(closureSpec(t, windowFrom, closedAt))
	if err != nil {
		t.Fatalf("form closure: %v", err)
	}
	return closure
}

func formWindow(t *testing.T, from, until time.Time) domain.PreAdviceValidityWindow {
	t.Helper()
	window, err := domain.FormPreAdviceValidityWindow(windowSpec(t, from, until))
	if err != nil {
		t.Fatalf("form window: %v", err)
	}
	return window
}

// Covers: 裁定「收口之后，起点不早于收口时刻的新行照常登记」「读某时刻适用哪一行时，把收口时刻当作那一行的终点」——收口之后
// 那一行在收口时刻之前照旧生效、自收口时刻起不再生效（半开）；登记的内容不变（终点仍是没有，原样重登仍是同一内容）；与起点
// 不早于收口时刻的后继不再重叠，与更早起点的后继照旧重叠。
func TestAClosureEndsAnOpenWindowAtItsClosingInstant(t *testing.T) {
	open := formWindow(t, closureWindowFrom, time.Time{})
	closed, err := open.Close(formClosure(t, closureWindowFrom, closureAt))
	if err != nil {
		t.Fatalf("close: %v", err)
	}
	recorded, isClosed := closed.Closure()
	if !isClosed || !recorded.ClosedAt().Equal(closureAt) || recorded.Basis() != "SYN-BASIS-PSXM14-CLOSURE" ||
		!recorded.RegisteredAt().Equal(closureRecordedAt) || !recorded.WindowFrom().Equal(closureWindowFrom) {
		t.Fatalf("closure = %+v %v", recorded, isClosed)
	}
	if _, isClosed := open.Closure(); isClosed {
		t.Fatal("收口改到了收口之前的那一份值")
	}
	if !closed.EffectiveAt(closureAt.Add(-time.Nanosecond)) || closed.EffectiveAt(closureAt) || closed.EffectiveAt(closureAt.AddDate(1, 0, 0)) {
		t.Fatal("收口之后应只在收口时刻之前生效")
	}
	if _, bounded := closed.EffectiveUntil(); bounded {
		t.Fatal("收口不改登记的内容：登记时没有终点，读回仍没有")
	}
	if !closed.SameContent(open) {
		t.Fatal("收口之后原样重登仍应是同一份登记内容")
	}

	successor := formWindow(t, closureAt, time.Time{})
	if closed.Overlaps(successor) || successor.Overlaps(closed) {
		t.Fatal("起点恰在收口时刻的后继不该与收口之后的那一行重叠")
	}
	if !open.Overlaps(successor) {
		t.Fatal("收口之前，后继与开放终点的那一行重叠")
	}
	early := formWindow(t, closureAt.Add(-time.Minute), time.Time{})
	if !closed.Overlaps(early) || !early.Overlaps(closed) {
		t.Fatal("起点早于收口时刻的后继照旧重叠")
	}
}

// Covers: 裁定「收口时刻早于前一行起点……答坏输入」——收口时刻不晚于那一行的起点（早于或恰等于，半开区间会空）、缺依据、缺租户、
// 缺那一行的起点、缺收口时刻、缺登记时刻，都形不成收口；内容比对不看登记时刻（重放判据）。
func TestAClosureNeedsAnInstantAfterItsWindowStartsAndABasis(t *testing.T) {
	refused := map[string]func(*domain.PreAdviceWindowClosureSpec){
		"收口时刻早于起点": func(spec *domain.PreAdviceWindowClosureSpec) { spec.ClosedAt = closureWindowFrom.Add(-time.Second) },
		"收口时刻恰在起点": func(spec *domain.PreAdviceWindowClosureSpec) { spec.ClosedAt = closureWindowFrom },
		"缺依据":      func(spec *domain.PreAdviceWindowClosureSpec) { spec.Basis = " \t" },
		"缺租户":      func(spec *domain.PreAdviceWindowClosureSpec) { spec.TenantID = domain.TenantID{} },
		"缺那一行的起点":  func(spec *domain.PreAdviceWindowClosureSpec) { spec.WindowFrom = time.Time{} },
		"缺收口时刻":    func(spec *domain.PreAdviceWindowClosureSpec) { spec.ClosedAt = time.Time{} },
		"缺登记时刻":    func(spec *domain.PreAdviceWindowClosureSpec) { spec.RegisteredAt = time.Time{} },
	}
	for name, mutate := range refused {
		spec := closureSpec(t, closureWindowFrom, closureAt)
		mutate(&spec)
		if _, err := domain.FormPreAdviceWindowClosure(spec); !errors.Is(err, domain.ErrInvalidPreAdviceWindowClosure) {
			t.Fatalf("%s: err = %v, want ErrInvalidPreAdviceWindowClosure", name, err)
		}
	}

	first := formClosure(t, closureWindowFrom, closureAt)
	again := closureSpec(t, closureWindowFrom, closureAt)
	again.RegisteredAt = again.RegisteredAt.Add(time.Hour)
	replayed, err := domain.FormPreAdviceWindowClosure(again)
	if err != nil {
		t.Fatal(err)
	}
	if !first.SameContent(replayed) {
		t.Fatal("只差登记时刻应算同一份收口")
	}
	if first.SameContent(formClosure(t, closureWindowFrom, closureAt.Add(time.Hour))) {
		t.Fatal("收口时刻不同不该算同一份收口")
	}
}

// Covers: 裁定「对已有终点的行收口、重复收口……答坏输入或已有结果」的领域半边——只有终点为空、还没收过口的那一行收得了口；登记时
// 带终点的行与已收过口的行都答不开放（重复收口是不是原样，由编排比内容）；收口指的不是这一行（起点或租户对不上）是调用方读错了
// 行，答形不成。
func TestOnlyAnOpenWindowCanBeClosed(t *testing.T) {
	bounded := formWindow(t, closureWindowFrom, closureAt.AddDate(0, 3, 0))
	if _, err := bounded.Close(formClosure(t, closureWindowFrom, closureAt)); !errors.Is(err, domain.ErrPreAdviceWindowNotOpen) {
		t.Fatalf("登记时带终点的行: err = %v, want ErrPreAdviceWindowNotOpen", err)
	}

	closed, err := formWindow(t, closureWindowFrom, time.Time{}).Close(formClosure(t, closureWindowFrom, closureAt))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := closed.Close(formClosure(t, closureWindowFrom, closureAt)); !errors.Is(err, domain.ErrPreAdviceWindowNotOpen) {
		t.Fatalf("已收过口的行原样再收: err = %v, want ErrPreAdviceWindowNotOpen", err)
	}
	if _, err := closed.Close(formClosure(t, closureWindowFrom, closureAt.AddDate(0, 1, 0))); !errors.Is(err, domain.ErrPreAdviceWindowNotOpen) {
		t.Fatalf("已收过口的行换时刻再收: err = %v, want ErrPreAdviceWindowNotOpen", err)
	}
	if recorded, _ := closed.Closure(); !recorded.ClosedAt().Equal(closureAt) {
		t.Fatalf("再收改到了第一份收口: %v", recorded.ClosedAt())
	}

	open := formWindow(t, closureWindowFrom, time.Time{})
	if _, err := open.Close(formClosure(t, closureWindowFrom.Add(time.Hour), closureAt)); !errors.Is(err, domain.ErrInvalidPreAdviceWindowClosure) {
		t.Fatalf("收口指的是别的起点: err = %v, want ErrInvalidPreAdviceWindowClosure", err)
	}
	otherTenant := closureSpec(t, closureWindowFrom, closureAt)
	otherTenant.TenantID = mustBuild(t, domain.NewTenantID, "SYN-TENANT-02")
	foreign, err := domain.FormPreAdviceWindowClosure(otherTenant)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := open.Close(foreign); !errors.Is(err, domain.ErrInvalidPreAdviceWindowClosure) {
		t.Fatalf("收口属别的租户: err = %v, want ErrInvalidPreAdviceWindowClosure", err)
	}
	if _, err := open.Close(domain.PreAdviceWindowClosure{}); !errors.Is(err, domain.ErrInvalidPreAdviceWindowClosure) {
		t.Fatalf("零值收口: err = %v, want ErrInvalidPreAdviceWindowClosure", err)
	}
}

// Covers: 完成判据「推导：收口之后的业务时间不再落进前一行的时窗」——同一份预报、同一个号码，业务时间早于收口时刻时照前一行判
// 已证；业务时间恰在或晚于收口时刻时，收口之后的那一行对它不生效，交进推导答未配置（没有后继时），不再拿前一行的时长判。
func TestTheDerivationStopsUsingAWindowFromItsClosure(t *testing.T) {
	open := registeredWindow(t)
	for name, testCase := range map[string]struct {
		closedAt time.Time
		state    domain.DeliveryScopeCheckState
	}{
		"收口晚于业务时间": {scopeReceivedAt.Add(time.Minute), domain.DeliveryScopeVerified},
		"收口恰在业务时间": {scopeReceivedAt, domain.DeliveryScopeUnconfigured},
		"收口早于业务时间": {scopeReceivedAt.Add(-time.Minute), domain.DeliveryScopeUnconfigured},
	} {
		closed, err := open.Close(formClosure(t, open.EffectiveFrom(), testCase.closedAt))
		if err != nil {
			t.Fatalf("%s: close: %v", name, err)
		}
		input := preAdviceDerivation(t)
		input.Window = closed
		if got := domain.DeriveVerifiableDeliveryScope(input).Check().State(); got != testCase.state {
			t.Fatalf("%s: check = %s, want %s", name, got, testCase.state)
		}
	}
}
