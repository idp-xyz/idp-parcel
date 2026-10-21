package tfhttp_test

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// Covers: AT-TF-284（ADR-0230 决定三）——查阅页派送待形成那一格逐格转写：段、对象、入场依据、缺件（原样顺序）、首次与最近一次答复
// 时刻；租户取认证结果，读口收到的就是它。
func TestTheDeliveryDispatchPendingRegistryTranscribesRows(t *testing.T) {
	first := time.Date(2026, 10, 21, 3, 0, 0, 0, time.UTC)
	tenant, err := domain.NewTenantID("TENANT-1")
	if err != nil {
		t.Fatal(err)
	}
	segment, err := domain.NewFulfillmentSegmentReference("SYN-SEG-70")
	if err != nil {
		t.Fatal(err)
	}
	object, err := domain.NewCarriedObjectReference("SYN-PARCEL-70")
	if err != nil {
		t.Fatal(err)
	}
	basis, err := domain.NewParticipationBasisReference("TRANSPORT-HANDOVER-KEY/13:SYN-PARCEL-70/5:scope/2:v1")
	if err != nil {
		t.Fatal(err)
	}
	register := &stubReviewCatalogue{pendings: []ports.DeliveryDispatchPendingRecord{{
		Key:             ports.DeliveryDispatchPendingKey{TenantID: tenant, Segment: segment, Object: object},
		EntryBasis:      basis,
		Missing:         []string{"DELIVERY_PLACE", "DELIVERY_CONDITION"},
		FirstAnsweredAt: first,
		LastAnsweredAt:  first.Add(2 * time.Hour),
		State:           ports.DeliveryDispatchPending,
	}}}
	response := serveGet(recordsEndpoint(register), "/transport-fulfillment-records?registry=delivery-dispatch-pending")

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
	}
	if register.gotTenant != "TENANT-1" || register.gotLimit != 25 {
		t.Fatalf("读口收到的键走样：tenant=%q limit=%d", register.gotTenant, register.gotLimit)
	}
	var body struct {
		Outcome  string `json:"outcome"`
		Pendings []struct {
			Segment         string   `json:"segment"`
			Object          string   `json:"object"`
			EntryBasis      string   `json:"entryBasis"`
			Missing         []string `json:"missing"`
			FirstAnsweredAt string   `json:"firstAnsweredAt"`
			LastAnsweredAt  string   `json:"lastAnsweredAt"`
		} `json:"pendings"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Outcome != "DELIVERY_DISPATCH_PENDINGS_LISTED" || len(body.Pendings) != 1 {
		t.Fatalf("答复 = %s", response.Body.String())
	}
	row := body.Pendings[0]
	if row.Segment != "SYN-SEG-70" || row.Object != "SYN-PARCEL-70" || row.EntryBasis != basis.String() ||
		len(row.Missing) != 2 || row.Missing[0] != "DELIVERY_PLACE" || row.Missing[1] != "DELIVERY_CONDITION" ||
		row.FirstAnsweredAt != "2026-10-21T03:00:00Z" || row.LastAnsweredAt != "2026-10-21T05:00:00Z" {
		t.Fatalf("那一行走样：%+v", row)
	}
}
