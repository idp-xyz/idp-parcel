package domain_test

import (
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 本文件证出发意图的起讫与声明的段怎么核（ADR-0237 决定五）的领域半边：判法只在 BindDepartureEntries，回给执行方那一层只在
// DepartureGateReading.ExecutorCells。节点都是合成串，证据只记 `S`。

// declaringHandover 是 object 的一版`已交接`，声明给定的交出与去向节点；空串即不声明那一格。
func declaringHandover(t *testing.T, object, releasing, onward string) domain.TransportHandover {
	t.Helper()
	spec := handoverSpec(t, object, domain.ObjectHandedOver)
	if releasing != "" {
		spec.Nodes.Releasing = mustValue(t, domain.NewNetworkNodeReference, releasing)
	}
	if onward != "" {
		spec.Nodes.Onward = mustValue(t, domain.NewNetworkNodeReference, onward)
	}
	handover, err := domain.FormTransportHandover(spec)
	if err != nil {
		t.Fatalf("交接 %s：%v", object, err)
	}
	return handover
}

// Covers: AT-TF-114、AT-TF-317（前者对不上那一支、后者未绑定那一支）领域半边——班次 NODE-SHA → NODE-SZX：每件两格都声明且相同
// 答已绑定；声明了的那一格只要有一格不同就是对不上，哪怕另一格没声明，也不论排在说不出的那一件之前还是之后；没有对不上而有说不出
// 的（没声明全、凭收寄进段或认不出）答未绑定；段里一件也认不出时同样未绑定。
func TestBindDepartureEntriesTellsBoundUnboundAndMismatchedApart(t *testing.T) {
	direction, err := domain.NewScheduleDirection("CONN-SHA-SZX", "NODE-SHA", "NODE-SZX")
	if err != nil {
		t.Fatalf("方向：%v", err)
	}
	bound := declaringHandover(t, "parcel-1", "NODE-SHA", "NODE-SZX")
	noOnward := declaringHandover(t, "parcel-2", "NODE-SHA", "")
	noNodes := declaringHandover(t, "parcel-3", "", "")
	wrongOrigin := declaringHandover(t, "parcel-4", "NODE-SZX", "NODE-SIN")
	wrongOriginOnly := declaringHandover(t, "parcel-5", "NODE-SZX", "")
	wrongOnwardOnly := declaringHandover(t, "parcel-6", "", "NODE-SIN")

	cases := []struct {
		name       string
		entries    []domain.TransportHandover
		unresolved bool
		want       domain.DepartureEntryBinding
	}{
		{"每件都对得上", []domain.TransportHandover{bound, bound}, false, domain.DepartureEntryBound},
		{"一件没声明去向", []domain.TransportHandover{bound, noOnward}, false, domain.DepartureEntryUnbound},
		{"一件一格都没声明", []domain.TransportHandover{noNodes}, false, domain.DepartureEntryUnbound},
		{"一件认不出或凭收寄进段", []domain.TransportHandover{bound}, true, domain.DepartureEntryUnbound},
		{"一件也认不出", nil, true, domain.DepartureEntryUnbound},
		{"起讫都对不上", []domain.TransportHandover{wrongOrigin}, false, domain.DepartureEntryMismatched},
		{"只声明了交出节点且对不上", []domain.TransportHandover{wrongOriginOnly}, false, domain.DepartureEntryMismatched},
		{"只声明了去向节点且对不上", []domain.TransportHandover{wrongOnwardOnly}, false, domain.DepartureEntryMismatched},
		{"说不出在前、对不上在后", []domain.TransportHandover{noOnward, wrongOrigin}, false, domain.DepartureEntryMismatched},
		{"对不上在前、说不出在后", []domain.TransportHandover{wrongOrigin, noNodes}, true, domain.DepartureEntryMismatched},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := domain.BindDepartureEntries(direction, tc.entries, tc.unresolved); got != tc.want {
				t.Fatalf("核对 = %s, want %s", got, tc.want)
			}
		})
	}
}

// Covers: AT-TF-317 领域半边——起讫没能与段核上的读数照样问了关务、每件一格、整格照推；回给执行方的那一层一件也不给，与它是不是
// 阻断无关。已核上的读数照 ExecutorGateCells 给。「未绑定」只在问了关务的读数上立得起：停在本侧的读数带它是读数不成形。
func TestAnUnboundReadingKeepsEveryFindingButShowsTheExecutorNone(t *testing.T) {
	findings := []domain.DepartureGateFinding{
		gateFinding(t, "parcel-held", domain.GateBlocked),
		gateFinding(t, "parcel-clean", domain.GateNotGuarded),
	}
	spec := domain.DepartureGateReadingSpec{
		From:     mustValue(t, domain.NewNetworkNodeReference, "NODE-SHA"),
		To:       mustValue(t, domain.NewNetworkNodeReference, "NODE-SZX"),
		Findings: findings,
	}
	boundReading, err := domain.FormDepartureGateReading(spec)
	if err != nil {
		t.Fatalf("读数：%v", err)
	}
	if cells := boundReading.ExecutorCells(); boundReading.EntryUnbound() || len(cells) != 1 || cells[0].Object.String() != "parcel-held" {
		t.Fatalf("已核上的读数回给执行方 %+v，want 只有被挡那一件", cells)
	}

	spec.EntryUnbound = true
	unbound, err := domain.FormDepartureGateReading(spec)
	if err != nil {
		t.Fatalf("未绑定的读数：%v", err)
	}
	if !unbound.EntryUnbound() || unbound.Answer() != domain.GateBlocked || len(unbound.Findings()) != 2 {
		t.Fatalf("未绑定的读数 = %s，%d 格，want 阻断、每件一格", unbound.Answer(), len(unbound.Findings()))
	}
	if cells := unbound.ExecutorCells(); len(cells) != 0 {
		t.Fatalf("未绑定的读数回给执行方 %+v，want 一件不列", cells)
	}
	if back, err := domain.RehydrateDepartureGateReading(spec, domain.GateBlocked); err != nil || !back.EntryUnbound() {
		t.Fatalf("重建：%v，未绑定 = %v", err, back.EntryUnbound())
	}

	if _, err := domain.FormDepartureGateReading(domain.DepartureGateReadingSpec{
		Stops: []domain.DepartureGateStop{domain.DepartureScheduleNotServingSegment}, EntryUnbound: true,
	}); !errors.Is(err, domain.ErrInvalidDepartureGateReading) {
		t.Fatalf("停在本侧还带「未绑定」 err = %v, want ErrInvalidDepartureGateReading", err)
	}
}
