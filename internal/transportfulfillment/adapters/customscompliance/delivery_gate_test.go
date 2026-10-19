package customscompliance_test

import (
	"errors"
	"testing"
	"time"

	tfcustoms "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/customscompliance"
	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	tfports "go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 本文件证消费侧适配器以交付意图问关务（ADR-0213 决定五）：真库那一半接关务的生产编排与真表，夹具同出发那一侧的
// newDepartureGateDB，隔离合成 S。

func (fixture *departureGateDB) deliver(at string) tfdomain.GateFinding {
	fixture.t.Helper()
	finding, err := fixture.delivery.ReadDeliveryGate(fixture.t.Context(), deliveryQuery(fixture.t, at))
	if err != nil {
		fixture.t.Fatalf("问关务：%v", err)
	}
	return finding
}

func deliveryQuery(t *testing.T, at string) tfports.DeliveryGateQuery {
	t.Helper()
	return tfports.DeliveryGateQuery{
		TenantID: ref(t, tfdomain.NewTenantID, departureTenant),
		Object:   ref(t, tfdomain.NewCarriedObjectReference, departureParcel),
		Node:     ref(t, tfdomain.NewNetworkNodeReference, at),
		At:       departureGateAt,
	}
}

// Covers: `AT-TF-256`、`AT-TF-257`、`AT-TF-258` 适配器半边，票 demo-chain-gaps/66 演示一条的关务那一半——在新加坡枢纽交付：
// 对象不在任何案卷，关务只适用最终交付那一道、答不受门禁，带回关务判断标识；交付意图没有去向，跨关务区域那一道不列。落在
// 包裹上的最终交付限制答阻断，依据指回那条限制；进了申报单元而单元没登门禁目录，关务的未配置译成未决。
func TestAFinalDeliveryIsJudgedAtTheDeliveryNode(t *testing.T) {
	fixture := newDepartureGateDB(t)
	expectFinding(t, fixture.deliver("SYN-NODE-SIN-HUB"), tfdomain.GateNotGuarded, "FINAL_DELIVERY", "OUTSIDE_CUSTOMS_BOOKS")

	fixture.inUnit()
	expectFinding(t, fixture.deliver("SYN-NODE-SIN-HUB"), tfdomain.GateUndecided, "FINAL_DELIVERY", "GATE_CATALOG_NOT_REGISTERED")

	fixture.seed(`INSERT INTO customs_compliance.regulatory_restriction
			(tenant_id, restriction_id, decision_id, scope_ref, constrains, effective_at)
		 VALUES ($1, 'SYN-RESTRICTION-DLV-01', 'SYN-DECISION-HOLD', $2, jsonb_build_array('FINAL_DELIVERY'), $3)`,
		departureTenant, departureParcel, departureGateAt.Add(-3*time.Hour))
	blocked := fixture.deliver("SYN-NODE-SIN-HUB")
	expectFinding(t, blocked, tfdomain.GateBlocked, "FINAL_DELIVERY", "RESTRICTION_IN_FORCE")
	if sources := blocked.Bases()[0].Sources; len(sources) != 1 || sources[0] != "SYN-RESTRICTION-DLV-01" {
		t.Fatalf("阻断依据要指回那条限制：%v", sources)
	}
}

// Covers: `AT-TF-259` 适配器半边——问不出答复只交 error，口径同出发、交出两侧：答复越出封闭集是译不回来，并认得出交付端口的
// 哨兵。
func TestAFinalDeliveryThatDoesNotFormIsAnErrorNotAFinding(t *testing.T) {
	zero := &stubJudge{}
	view, err := tfcustoms.NewDeliveryGateView(zero)
	if err != nil {
		t.Fatalf("构造：%v", err)
	}
	if _, err := view.ReadDeliveryGate(t.Context(), deliveryQuery(t, "SYN-NODE-SIN-HUB")); !errors.Is(err, tfports.ErrDeliveryGateAnswerUntranslatable) {
		t.Fatalf("零值答复：err = %v, want 交付端口的译不回来", err)
	}
	if _, err := tfcustoms.NewDeliveryGateView(nil); err == nil {
		t.Fatal("没有判断口也装得起来")
	}
}
