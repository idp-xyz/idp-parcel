package tfhttp_test

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// Covers: AT-TF-304 的线格式那一半（ADR-0231 决定六）——查阅页实际履约未形成那一格逐格转写：段、对象、委托与作答的那一版的号、哪一格、
// 问的那一版协议、参与终点、首次与最近一次答复时刻；缺采购上下文那一行不出协议两键；租户取认证结果，读口收到的就是它；空册答 []。
func TestTheActualFulfillmentUnformedRegistryTranscribesRows(t *testing.T) {
	ended := time.Date(2026, 10, 19, 6, 0, 0, 0, time.UTC)
	answered := time.Date(2026, 10, 21, 3, 0, 0, 0, time.UTC)
	tenant, _ := domain.NewTenantID("TENANT-1")
	segment, _ := domain.NewFulfillmentSegmentReference("SYN-SEG-88")
	object, _ := domain.NewCarriedObjectReference("SYN-PARCEL-88")
	other, _ := domain.NewCarriedObjectReference("SYN-PARCEL-88-LEGACY")
	commission, _ := domain.NewTransportCommissionReference("SYN-COMMISSION-88")
	agreement, _ := domain.NewSupplierAgreementVersion("SYN-SUPPLIER-TRUNK-CN-SG-01", "v1")
	register := &stubReviewCatalogue{unformed: []ports.ActualFulfillmentUnformedRecord{
		{
			Entry: ports.ActualFulfillmentUnformedEntry{
				Key:        ports.ActualFulfillmentUnformedKey{TenantID: tenant, Segment: segment, Object: object},
				Commission: commission, Revision: 2, EndedAt: ended, Reason: ports.UnformedAgreementTerminated, Agreement: agreement,
			},
			FirstAnsweredAt: answered, LastAnsweredAt: answered.Add(time.Hour), State: ports.ActualFulfillmentUnformed,
		},
		{
			Entry: ports.ActualFulfillmentUnformedEntry{
				Key:        ports.ActualFulfillmentUnformedKey{TenantID: tenant, Segment: segment, Object: other},
				Commission: commission, Revision: 1, EndedAt: ended, Reason: ports.UnformedProcurementContextMissing,
			},
			FirstAnsweredAt: answered, LastAnsweredAt: answered, State: ports.ActualFulfillmentUnformed,
		},
	}}
	response := serveGet(recordsEndpoint(register), "/transport-fulfillment-records?registry=actual-fulfillment-unformed")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
	}
	if register.gotTenant != "TENANT-1" || register.gotLimit != 25 {
		t.Fatalf("读口收到的键走样：tenant=%q limit=%d", register.gotTenant, register.gotLimit)
	}
	var body struct {
		Outcome  string           `json:"outcome"`
		Unformed []map[string]any `json:"unformed"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Outcome != "ACTUAL_FULFILLMENT_UNFORMED_LISTED" || len(body.Unformed) != 2 {
		t.Fatalf("答复 = %s", response.Body.String())
	}
	row := body.Unformed[0]
	if row["segment"] != "SYN-SEG-88" || row["object"] != "SYN-PARCEL-88" || row["commission"] != "SYN-COMMISSION-88" ||
		row["commissionRevision"] != float64(2) || row["reason"] != "AGREEMENT_TERMINATED" ||
		row["agreementObject"] != "SYN-SUPPLIER-TRUNK-CN-SG-01" || row["agreementVersion"] != "v1" ||
		row["endedAt"] != "2026-10-19T06:00:00Z" || row["firstAnsweredAt"] != "2026-10-21T03:00:00Z" ||
		row["lastAnsweredAt"] != "2026-10-21T04:00:00Z" {
		t.Fatalf("那一行走样：%+v", row)
	}
	if legacy := body.Unformed[1]; legacy["reason"] != "PROCUREMENT_CONTEXT_MISSING" {
		t.Fatalf("缺采购上下文那一行 = %+v", legacy)
	} else if _, asked := legacy["agreementObject"]; asked {
		t.Fatalf("没问 PC 的那一行带了协议：%+v", legacy)
	}

	empty := serveGet(recordsEndpoint(&stubReviewCatalogue{}), "/transport-fulfillment-records?registry=actual-fulfillment-unformed")
	if empty.Code != http.StatusOK || !json.Valid(empty.Body.Bytes()) {
		t.Fatalf("空册 = %d %s", empty.Code, empty.Body.String())
	}
	var emptyBody map[string]json.RawMessage
	_ = json.Unmarshal(empty.Body.Bytes(), &emptyBody)
	if string(emptyBody["unformed"]) != "[]" {
		t.Fatalf("空册 unformed = %s, want []", emptyBody["unformed"])
	}
}
