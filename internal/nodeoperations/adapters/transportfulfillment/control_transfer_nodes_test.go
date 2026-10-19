package transportfulfillment_test

import (
	"testing"

	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// Covers: `AT-NO-113`、`AT-NO-120` 的适配面（ADR-0213 决定一、二，ADR-0214 决定四末条、决定七）——交接声明的交出、接收节点原样译成
// 本上下文的节点引用；没声明的格留零值，不补；去向节点 NO 不读。
func TestTheSourceTranslatesTheDeclaredNodes(t *testing.T) {
	declared, err := tfdomain.FormTransportHandover(tfdomain.TransportHandoverSpec{
		TenantID:          value(t, tfdomain.NewTenantID, "tenant-1"),
		Object:            value(t, tfdomain.NewCarriedObjectReference, "parcel-1"),
		Scope:             value(t, tfdomain.NewHandoverScopeReference, "handover-scope-1"),
		ReleasedBy:        value(t, tfdomain.NewHandoverPartyReference, "node-origin"),
		ReceivedBy:        value(t, tfdomain.NewHandoverPartyReference, "carrier-1"),
		Verdict:           tfdomain.ObjectHandedOver,
		ReleasingEvidence: value(t, tfdomain.NewHandoverEvidenceReference, "LOAD-SCAN/7"),
		ReceivingEvidence: value(t, tfdomain.NewHandoverEvidenceReference, "CARRIER-ACK/9"),
		Rule:              value(t, tfdomain.NewHandoverRuleReference, "handover-rules/v1"),
		Version:           value(t, tfdomain.NewHandoverResultVersion, "v1"),
		JudgedAt:          handedOverAt,
		Nodes: tfdomain.HandoverNodeDeclarations{
			Releasing: value(t, tfdomain.NewNetworkNodeReference, "SYN-NODE-SHA-HUB"),
			Receiving: value(t, tfdomain.NewNetworkNodeReference, "SYN-NODE-SZX-GATE"),
			Onward:    value(t, tfdomain.NewNetworkNodeReference, "SYN-NODE-SIN-HUB"),
		},
	})
	if err != nil {
		t.Fatalf("form handover with nodes: %v", err)
	}
	answer, found, err := find(t, newReader(declared), "parcel-1", "v1")
	if err != nil || !found || answer.ReleasingNode.String() != "SYN-NODE-SHA-HUB" || answer.ReceivingNode.String() != "SYN-NODE-SZX-GATE" {
		t.Fatalf("声明了节点 = %+v found=%v err=%v", answer, found, err)
	}

	bare, found, err := find(t, newReader(formed(t, tfdomain.ObjectHandedOver, "parcel-1", "v1")), "parcel-1", "v1")
	if err != nil || !found || bare.ReleasingNode.String() != "" || bare.ReceivingNode.String() != "" {
		t.Fatalf("没声明 = %+v found=%v err=%v", bare, found, err)
	}
}
