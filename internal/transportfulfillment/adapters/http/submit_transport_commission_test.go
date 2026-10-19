package tfhttp_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// ---- 委托提交口的端口替身 ----

type commissionRegistryDouble struct {
	records map[string]ports.TransportCommissionRecord
}

func (double *commissionRegistryDouble) FindByKey(
	_ context.Context,
	key ports.TransportCommissionKey,
) (ports.TransportCommissionRecord, bool, error) {
	record, found := double.records[key.TenantID.String()+"|"+key.Commission.String()]
	return record, found, nil
}

func (double *commissionRegistryDouble) Save(
	_ context.Context,
	record ports.TransportCommissionRecord,
) (ports.CommissionSaveOutcome, error) {
	storeKey := record.Key.TenantID.String() + "|" + record.Key.Commission.String()
	if _, exists := double.records[storeKey]; exists {
		return ports.CommissionAlreadyRegistered, nil
	}
	double.records[storeKey] = record
	return ports.CommissionSaved, nil
}

func (double *commissionRegistryDouble) Replace(context.Context, ports.TransportCommissionRecord) (bool, error) {
	return false, nil
}

// agreementCheckStub 按服务方答：协议约定的供应商是 supplier，服务方不是它就答服务方不符。
type agreementCheckStub struct {
	supplier string
}

func (stub agreementCheckStub) CheckSupplierAgreement(
	_ context.Context,
	_ domain.TenantID,
	agreement domain.SupplierAgreementVersion,
	provider domain.ServiceProviderReference,
	_ time.Time,
) (ports.SupplierAgreementCheck, error) {
	supplier, _ := domain.NewServiceProviderReference(stub.supplier)
	if provider != supplier {
		return ports.SupplierAgreementCheck{Standing: ports.SupplierAgreementProviderMismatch, Supplier: supplier}, nil
	}
	snapshot, _ := domain.NewAgreementSnapshotReference(agreement.Object() + "/" + agreement.Version())
	legalEntity, _ := domain.NewProcurementLegalEntityReference("SYN-LE-01")
	return ports.SupplierAgreementCheck{
		Standing: ports.SupplierAgreementRegistered, Snapshot: snapshot, LegalEntity: legalEntity, Supplier: supplier,
	}, nil
}

type commissionHandoffStub struct{}

func (commissionHandoffStub) HandOffTransportCommission(context.Context, ports.TransportCommissionIntent) error {
	return nil
}

type fixedCommissionClock struct{}

func (fixedCommissionClock) Now() time.Time { return time.Date(2026, 10, 19, 9, 0, 0, 0, time.UTC) }

func commissionSubmissionEndpoint(t *testing.T) http.Handler {
	t.Helper()
	handler := application.NewCommissionTransportHandler(application.CommissionTransportDeps{
		Commissions: &commissionRegistryDouble{records: map[string]ports.TransportCommissionRecord{}},
		Agreements:  agreementCheckStub{supplier: "SYN-PARTY-CARRIER-TRUNK-01"},
		Downstream:  commissionHandoffStub{},
		Clock:       fixedCommissionClock{},
	})
	return tfhttp.NewSubmitTransportCommissionEndpoint(operatorIntake(t, authenticatorFake{}), handler)
}

const trunkCommissionBody = `{"commission": "SYN-COMMISSION-01", "provider": "SYN-PARTY-CARRIER-TRUNK-01",
	"agreement": "SYN-SUPPLIER-TRUNK-CN-SG-01", "agreementVersion": "v1", "conditions": "SYN-CONDITIONS-01",
	"role": "SYN-ROLE-CONTRACTED-CARRIER", "responsibility": "SYN-RESPONSIBILITY-01", "members": ["SYN-PARCEL-01"],
	"submittedAt": "2026-10-19T08:00:00Z"}`

func postCommission(t *testing.T, endpoint http.Handler, body string) (int, map[string]any) {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/transport-fulfillment-transport-commission-submissions", strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer presented.operator.token")
	response := httptest.NewRecorder()
	endpoint.ServeHTTP(response, request)
	var answer map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &answer); err != nil {
		t.Fatalf("decode %s: %v", response.Body.String(), err)
	}
	return response.Code, answer
}

// Covers: ADR-0202 决定一、三的线格式——委托提交口按「运营决定」认证、租户只取认证结果；协议只收对象与版本标签两格，
// 快照与采购责任法人回显的是核对结果；新提交答 201，同一份原样重交答 200 已有委托；服务方不符答 200 并带协议约定的
// 供应商；载荷自报租户或法人、时刻写坏都是坏报文（400），不进编排。
func TestTheTransportCommissionSubmissionFaceAnswersOnTheWire(t *testing.T) {
	endpoint := commissionSubmissionEndpoint(t)

	status, answer := postCommission(t, endpoint, trunkCommissionBody)
	if status != http.StatusCreated || answer["outcome"] != "COMMISSION_SUBMITTED" {
		t.Fatalf("首交 = %d %v", status, answer)
	}
	commission, _ := answer["commission"].(map[string]any)
	if commission["agreement"] != "SYN-SUPPLIER-TRUNK-CN-SG-01/v1" || commission["legalEntity"] != "SYN-LE-01" ||
		commission["provider"] != "SYN-PARTY-CARRIER-TRUNK-01" || commission["submittedAt"] != "2026-10-19T08:00:00Z" {
		t.Fatalf("回显的委托 = %v", commission)
	}

	if status, answer := postCommission(t, endpoint, trunkCommissionBody); status != http.StatusOK || answer["outcome"] != "EXISTING_COMMISSION" {
		t.Fatalf("原样重交 = %d %v", status, answer)
	}

	mismatch := strings.Replace(strings.Replace(trunkCommissionBody, `"SYN-COMMISSION-01"`, `"SYN-COMMISSION-02"`, 1),
		`"provider": "SYN-PARTY-CARRIER-TRUNK-01"`, `"provider": "SYN-PARTY-CARRIER-LM-01"`, 1)
	status, answer = postCommission(t, endpoint, mismatch)
	if status != http.StatusOK || answer["outcome"] != "PROVIDER_MISMATCH" || answer["agreementSupplier"] != "SYN-PARTY-CARRIER-TRUNK-01" {
		t.Fatalf("服务方不符 = %d %v", status, answer)
	}
	if _, present := answer["commission"]; present {
		t.Fatalf("服务方不符却回显了委托：%v", answer)
	}

	for name, body := range map[string]string{
		"self-reported tenant":       strings.Replace(trunkCommissionBody, `{"commission"`, `{"tenantId": "SYN-TENANT-02", "commission"`, 1),
		"self-reported legal entity": strings.Replace(trunkCommissionBody, `{"commission"`, `{"legalEntity": "SYN-LE-99", "commission"`, 1),
		"unparseable instant":        strings.Replace(trunkCommissionBody, `"2026-10-19T08:00:00Z"`, `"yesterday"`, 1),
	} {
		status, answer := postCommission(t, endpoint, body)
		problem, _ := answer["error"].(map[string]any)
		if status != http.StatusBadRequest || problem["code"] != "MALFORMED_REQUEST" {
			t.Fatalf("%s = %d %v, want 400 MALFORMED_REQUEST", name, status, answer)
		}
	}

	missingTime := strings.Replace(trunkCommissionBody, `"2026-10-19T08:00:00Z"`, `""`, 1)
	missingTime = strings.Replace(missingTime, `"SYN-COMMISSION-01"`, `"SYN-COMMISSION-03"`, 1)
	if status, answer := postCommission(t, endpoint, missingTime); status != http.StatusOK || answer["outcome"] != "INPUT_NOT_ACCEPTED" {
		t.Fatalf("缺提交时刻 = %d %v, want 200 INPUT_NOT_ACCEPTED", status, answer)
	}
}
