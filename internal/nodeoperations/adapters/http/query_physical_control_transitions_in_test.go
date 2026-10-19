package nodeopshttp_test

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	nodeopshttp "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/http"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
)

// Covers: `AT-NO-123` 的传输面（ADR-0214 决定四末条、七）——转入那一行带控制四件、不带转出两件与信息格；交出节点声明不符的转出带
// mismatchedReleasingNode，值是声明的那一个；声明相符的转出整键不出现。
func TestTransfersInAndTheReleasingNodeMismatchAreListed(t *testing.T) {
	established := catalogueBaseAt
	released := catalogueBaseAt.Add(4 * time.Hour)
	catalogue := controlTransitionsCatalogue{stubReviewCatalogue: &stubReviewCatalogue{}, transitions: []ports.PhysicalControlTransitionCatalogueRow{
		{
			HandoverObject: "SYN-PARCEL-62", HandoverScope: "SYN-SCOPE-62-TRUNK", HandoverVersion: "v1",
			Direction: "OUT", Outcome: "ADVANCED", Unit: "SYN-HU-62", Node: "SYN-NODE-SZX-GATE", ControlKind: "HANDOVER_IN",
			ControlBasis: "HANDOVER-IN/13:SYN-PARCEL-62/14:SYN-SCOPE-62-IN/2:v1", ControlEstablishedAt: &established,
			TransferReference: "TRANSPORT-HANDOVER/v1", TransferredAt: &released, MismatchedReleasingNode: "SYN-NODE-SIN-HUB",
			OccurredAt: released, RecordedAt: released.Add(time.Minute),
		},
		{
			HandoverObject: "SYN-PARCEL-62", HandoverScope: "SYN-SCOPE-62-IN", HandoverVersion: "v1",
			Direction: "IN", Outcome: "ADVANCED", Unit: "SYN-HU-62", Node: "SYN-NODE-SZX-GATE", ControlKind: "HANDOVER_IN",
			ControlBasis: "HANDOVER-IN/13:SYN-PARCEL-62/14:SYN-SCOPE-62-IN/2:v1", ControlEstablishedAt: &established,
			OccurredAt: established, RecordedAt: established.Add(time.Minute),
		},
	}}
	response := serveGet(nodeopshttp.NewQueryNodeOperationsRecordsEndpoint(grantedIntake(), catalogue),
		"/node-operations-records?registry=physical-control")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
	}
	var body struct {
		Transitions []map[string]json.RawMessage `json:"transitions"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil || len(body.Transitions) != 2 {
		t.Fatalf("decode %s: %v", response.Body.String(), err)
	}
	out, in := body.Transitions[0], body.Transitions[1]
	if string(out["mismatchedReleasingNode"]) != `"SYN-NODE-SIN-HUB"` || string(out["controlKind"]) != `"HANDOVER_IN"` {
		t.Fatalf("再转出那一行 = %s", response.Body.String())
	}
	if string(in["direction"]) != `"IN"` || string(in["node"]) != `"SYN-NODE-SZX-GATE"` ||
		string(in["controlEstablishedAt"]) != `"2026-08-28T09:00:00Z"` {
		t.Fatalf("转入那一行 = %s", response.Body.String())
	}
	for _, absent := range []string{"reason", "transferReference", "transferredAt", "mismatchedReleasingNode"} {
		if _, present := in[absent]; present {
			t.Fatalf("转入那一行不该带 %s：%s", absent, response.Body.String())
		}
	}
}
