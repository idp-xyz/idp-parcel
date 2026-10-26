package nodeoperations_test

import (
	"context"
	"errors"
	"testing"
	"time"

	nodomain "go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	noports "go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
	psnode "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/nodeoperations"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// unitBookDouble 是节点作业单元册读面的替身（隔离合成 `S`）：按对象串交回登记的答复，记下被问的（租户，对象，时点）。
// 未登记的对象交回「不是单元」——那是册对「没有这个实例」的语义，不是替身的发明。
type unitBookDouble struct {
	answers     map[string]noports.ConsolidationMembershipAt
	err         error
	askedTenant string
	askedObject string
	askedAt     time.Time
	calls       int
}

func (double *unitBookDouble) ReadConsolidationMembershipAt(
	_ context.Context,
	tenant nodomain.TenantID,
	object string,
	at time.Time,
) (noports.ConsolidationMembershipAt, error) {
	double.calls++
	double.askedTenant = tenant.String()
	double.askedObject = object
	double.askedAt = at
	if double.err != nil {
		return noports.ConsolidationMembershipAt{}, double.err
	}
	return double.answers[object], nil
}

func newUnitView(t *testing.T, double *unitBookDouble) psnode.ConsolidationUnitView {
	t.Helper()
	view, err := psnode.NewConsolidationUnitView(double)
	if err != nil {
		t.Fatalf("构造单元读口：%v", err)
	}
	return view
}

var unitAskedAt = time.Date(2026, 10, 22, 9, 0, 0, 0, time.UTC)

// Covers: AT-PS-156、AT-PS-157、AT-PS-158 的判据半边——判据只是节点作业单元册的那一答：册上是单元（不论装着几件）就是单元，册上没有就不是；租户、对象串与时点原样
// 成为节点作业的问法，不另立串形或前缀规则。
func TestTheUnitViewTranslatesTheUnitBooksAnswerWithoutAnyRuleOfItsOwn(t *testing.T) {
	double := &unitBookDouble{answers: map[string]noports.ConsolidationMembershipAt{
		"SYN-BAG-1":     {Unit: true, Members: []noports.ConsolidationMemberAt{}},
		"SYN-BAG-2":     {Unit: true, Members: []noports.ConsolidationMemberAt{{Member: handlingUnit(t, "SYN-PARCEL-A")}}},
		"SYN-PARCEL-01": {},
	}}
	view := newUnitView(t, double)

	for object, want := range map[string]bool{
		"SYN-BAG-1": true, "SYN-BAG-2": true, "SYN-PARCEL-01": false,
		"SYN-BAG-NOT-IN-BOOK": false, // 串长得像袋号，册上没有：不是单元
	} {
		got, err := view.IsConsolidationUnitAt(t.Context(),
			value(t, psdomain.NewTenantID, "tenant-1"), value(t, psdomain.NewDeclaredParcelID, object), unitAskedAt)
		if err != nil {
			t.Fatalf("%s：%v", object, err)
		}
		if got != want {
			t.Fatalf("%s 是单元 = %v, want %v", object, got, want)
		}
		if double.askedTenant != "tenant-1" || double.askedObject != object || !double.askedAt.Equal(unitAskedAt) {
			t.Fatalf("%s 的问法 = %q / %q / %v", object, double.askedTenant, double.askedObject, double.askedAt)
		}
	}
}

// Covers: AT-PS-159——册读不动原样交回，不包 ErrConsolidationUnitAnswerUntranslatable，也不读成「不是单元」：调用方据此分到续办一格。
func TestAnUnreadableUnitBookIsNotAnAnswerAndNotUntranslatable(t *testing.T) {
	unreadable := errors.New("unit book unavailable")
	view := newUnitView(t, &unitBookDouble{err: unreadable})

	unit, err := view.IsConsolidationUnitAt(t.Context(),
		value(t, psdomain.NewTenantID, "tenant-1"), value(t, psdomain.NewDeclaredParcelID, "SYN-BAG-1"), unitAskedAt)
	if err == nil {
		t.Fatalf("读不动却答出 unit = %v", unit)
	}
	if !errors.Is(err, unreadable) {
		t.Fatalf("err = %v, want 带着读不动的原因", err)
	}
	if errors.Is(err, psports.ErrConsolidationUnitAnswerUntranslatable) {
		t.Fatalf("读不动被分进了译不回：%v", err)
	}
	if unit {
		t.Fatal("读不动不该答出单元")
	}
}

// Covers: AT-PS-159——答了却立不成本口的形状是确定性的：答「不是单元」却带着成员（册或翻译自相矛盾）、租户译不过去，都包住
// ErrConsolidationUnitAnswerUntranslatable。
func TestAnAnswerThatContradictsItselfIsUntranslatable(t *testing.T) {
	contradictory := &unitBookDouble{answers: map[string]noports.ConsolidationMembershipAt{
		"SYN-BAG-1": {Unit: false, Members: []noports.ConsolidationMemberAt{{Member: handlingUnit(t, "SYN-PARCEL-A")}}},
	}}
	view := newUnitView(t, contradictory)

	_, err := view.IsConsolidationUnitAt(t.Context(),
		value(t, psdomain.NewTenantID, "tenant-1"), value(t, psdomain.NewDeclaredParcelID, "SYN-BAG-1"), unitAskedAt)
	if !errors.Is(err, psports.ErrConsolidationUnitAnswerUntranslatable) {
		t.Fatalf("err = %v, want ErrConsolidationUnitAnswerUntranslatable", err)
	}

	untouched := &unitBookDouble{}
	_, err = newUnitView(t, untouched).IsConsolidationUnitAt(t.Context(),
		psdomain.TenantID{}, value(t, psdomain.NewDeclaredParcelID, "SYN-BAG-1"), unitAskedAt)
	if !errors.Is(err, psports.ErrConsolidationUnitAnswerUntranslatable) {
		t.Fatalf("租户译不过去：err = %v, want ErrConsolidationUnitAnswerUntranslatable", err)
	}
	if untouched.calls != 0 {
		t.Fatalf("租户都译不过去还去问了单元册 %d 次", untouched.calls)
	}
}

func TestTheUnitViewRefusesToBeBuiltWithoutTheBook(t *testing.T) {
	if _, err := psnode.NewConsolidationUnitView(nil); err == nil {
		t.Fatal("缺单元册读面却构造成功：装配漏接会长成每封信都读不动")
	}
}

func handlingUnit(t *testing.T, raw string) nodomain.HandlingUnitID {
	t.Helper()
	return value(t, nodomain.NewHandlingUnitID, raw)
}
