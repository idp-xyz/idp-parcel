package customscompliance_test

import (
	"errors"
	"testing"

	tfcustoms "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/customscompliance"
	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	tfports "go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 本文件证消费侧适配器以交出意图问关务（ADR-0213 决定四）：真库那一半接关务的生产编排与真表，夹具同出发那一侧
// （departure_gate_test.go 的 newDepartureGateDB，四个节点已登区域），隔离合成 S。

func (fixture *departureGateDB) release(at, onward string) tfdomain.GateFinding {
	fixture.t.Helper()
	finding, err := fixture.handover.ReadHandoverGate(fixture.t.Context(), handoverQuery(fixture.t, at, onward))
	if err != nil {
		fixture.t.Fatalf("问关务：%v", err)
	}
	return finding
}

func handoverQuery(t *testing.T, at, onward string) tfports.HandoverGateQuery {
	t.Helper()
	query := tfports.HandoverGateQuery{
		TenantID: ref(t, tfdomain.NewTenantID, departureTenant),
		Object:   ref(t, tfdomain.NewCarriedObjectReference, departureParcel),
		Node:     ref(t, tfdomain.NewNetworkNodeReference, at),
		At:       departureGateAt,
	}
	if onward != "" {
		query.Onward = ref(t, tfdomain.NewNetworkNodeReference, onward)
	}
	return query
}

// Covers: AT-TF-236、AT-TF-238 适配器半边，票 demo-chain-gaps/65 演示一条的关务那一半——深圳口岸交给干线承运商、去向新加坡：
// 对象不在任何案卷，出库那一道不受门禁、跨关务区域那一道未决，整次未决，带回的只有跨区那一条依据；同区去向只有出库那一道，
// 不受门禁；没声明去向就不带去向问，跨区那一道不列。
func TestAnOutboundReleaseIsJudgedOnTheReleasingAndOnwardNodes(t *testing.T) {
	fixture := newDepartureGateDB(t)
	crossBorder := fixture.release("SYN-NODE-SZX-GATE", "SYN-NODE-SIN-HUB")
	expectFinding(t, crossBorder, tfdomain.GateUndecided, "CROSS_CUSTOMS_MOVEMENT", "CUSTOMS_SCOPE_UNRESOLVED")
	expectFinding(t, fixture.release("SYN-NODE-SZX-GATE", "SYN-NODE-SHA-HUB"),
		tfdomain.GateNotGuarded, "OUTBOUND_RELEASE", "OUTSIDE_CUSTOMS_BOOKS")
	expectFinding(t, fixture.release("SYN-NODE-SZX-GATE", ""),
		tfdomain.GateNotGuarded, "OUTBOUND_RELEASE", "OUTSIDE_CUSTOMS_BOOKS")
}

// Covers: AT-TF-240 适配器半边——问不出答复只交 error，口径同出发那一侧：答复越出封闭集是译不回来，并认得出交出端口的哨兵；去向与交出节点相同立不成
// 交出意图，那一问不发出去。
func TestAnOutboundReleaseThatDoesNotFormIsAnErrorNotAFinding(t *testing.T) {
	zero := &stubJudge{}
	view, err := tfcustoms.NewHandoverGateView(zero)
	if err != nil {
		t.Fatalf("构造：%v", err)
	}
	if _, err := view.ReadHandoverGate(t.Context(), handoverQuery(t, "SYN-NODE-SZX-GATE", "SYN-NODE-SIN-HUB")); !errors.Is(err, tfports.ErrHandoverGateAnswerUntranslatable) {
		t.Fatalf("零值答复：err = %v, want 交出端口的译不回来", err)
	}
	if _, err := view.ReadHandoverGate(t.Context(), handoverQuery(t, "SYN-NODE-SZX-GATE", "SYN-NODE-SZX-GATE")); !errors.Is(err, tfports.ErrHandoverGateAnswerUntranslatable) || zero.calls != 1 {
		t.Fatalf("去向与交出节点相同：err = %v calls=%d，立不成意图就不该问", err, zero.calls)
	}
	if _, err := tfcustoms.NewHandoverGateView(nil); err == nil {
		t.Fatal("没有判断口也装得起来")
	}
}
