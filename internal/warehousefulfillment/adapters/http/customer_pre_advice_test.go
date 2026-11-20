package warehousehttp_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	whaccess "go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/accessidentity"
	warehousehttp "go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/http"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
)

// 货主口的负例与读口形状。合成主体只记 S。写口的成功答复走受理用例，这里不另写一条判定次序。

const customerIssuerIPA = "https://auth.syn.example/customers"

type countingPreAdviceWrites struct {
	calls int
}

func (writes *countingPreAdviceWrites) Accept(context.Context, application.AcceptPreAdviceCommand) (application.AcceptPreAdviceResult, error) {
	writes.calls++
	return application.AcceptPreAdviceResult{}, nil
}

func (writes *countingPreAdviceWrites) UpdateNotice(context.Context, application.UpdatePreAdviceNoticeCommand) (application.AcceptPreAdviceResult, error) {
	writes.calls++
	return application.AcceptPreAdviceResult{}, nil
}

func (writes *countingPreAdviceWrites) Withdraw(context.Context, application.WithdrawPreAdviceCommand) (application.AcceptPreAdviceResult, error) {
	writes.calls++
	return application.AcceptPreAdviceResult{}, nil
}

type fixedCustomerVerifier struct {
	subject accessidentity.CustomerClientSubject
}

func (verifier fixedCustomerVerifier) VerifyCustomerClientCredential(context.Context, accessidentity.IntegrationClientCredential) (accessidentity.VerifiedCustomerClientToken, error) {
	return accessidentity.NewVerifiedCustomerClientToken(verifier.subject, false), nil
}

type holdingCustomerRegistry struct {
	standing accessidentity.CustomerClientStanding
}

func (registry holdingCustomerRegistry) FindCustomerClient(context.Context, accessidentity.CustomerClientSubject) (accessidentity.CustomerClientStanding, bool, error) {
	return registry.standing, true, nil
}

type recordingAdmission struct {
	asked []accessidentity.AdmissionRequirement
}

func (admission *recordingAdmission) Admits(_ context.Context, _ string, requirement accessidentity.AdmissionRequirement, _ time.Time) (bool, error) {
	admission.asked = append(admission.asked, requirement)
	return true, nil
}

func customerIntakeHolding(t *testing.T, account string, capabilities ...accessidentity.CustomerCapability) (*warehousehttp.CustomerInboundPreAdviceIntake, *recordingAdmission) {
	t.Helper()
	at := time.Date(2026, 11, 20, 6, 0, 0, 0, time.UTC)
	subject, err := accessidentity.NewCustomerClientSubject(customerIssuerIPA, "SYN-CUSTOMER-CLIENT-IPA")
	if err != nil {
		t.Fatal(err)
	}
	reference, err := accessidentity.NewCredentialReference("SYN-CREDENTIAL-REF/customer-ipa")
	if err != nil {
		t.Fatal(err)
	}
	binding, err := accessidentity.NewCustomerClientBinding(subject, tenantA, account, reference, false, "SYN-BASIS")
	if err != nil {
		t.Fatal(err)
	}
	interval, err := accessidentity.NewEffectiveInterval(at.Add(-time.Hour), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	var grants []accessidentity.RecordedCustomerClientGrant
	for _, capability := range capabilities {
		grant, err := accessidentity.NewCustomerClientGrant(tenantA, "SYN-CUSTOMER-GRANT-"+capability.String(), subject, capability, interval, "SYN-BASIS")
		if err != nil {
			t.Fatal(err)
		}
		recorded, err := accessidentity.NewRecordedCustomerClientGrant(grant, nil)
		if err != nil {
			t.Fatal(err)
		}
		grants = append(grants, recorded)
	}
	standing, err := accessidentity.NewCustomerClientStanding(binding, grants)
	if err != nil {
		t.Fatal(err)
	}
	admission := &recordingAdmission{}
	minter, err := accessidentity.NewCustomerClientMinter(fixedCustomerVerifier{subject: subject}, holdingCustomerRegistry{standing: standing}, admission, func() time.Time { return at })
	if err != nil {
		t.Fatal(err)
	}
	authenticator, err := whaccess.NewCustomerInboundPreAdviceAuthenticator(minter)
	if err != nil {
		t.Fatal(err)
	}
	intake, err := warehousehttp.NewCustomerInboundPreAdviceIntake(authenticator)
	if err != nil {
		t.Fatal(err)
	}
	return intake, admission
}

func postCustomer(handler http.Handler, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, "/v1/warehouse/inbound-pre-advices", strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer presented")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

const customerPreAdviceBody = `{"accessRequestId":"SYN-ACCESS-IPA-0001","warehouseCode":"SYN-WH-01","arrivalMode":"PALLET","inboundMode":"BY_GOODS","expectedAt":"2026-11-21T00:00:00Z","goodsLines":[{"goodsCode":"SYN-GOODS-01","unit":"件","quantity":1}]}`

// Covers: AT-WF-035——载荷自带租户或货主答 400，不落册。标识以保留前缀开头同样 400，不落册。
func TestCustomerPreAdviceRejectsCarriedIdentityAndReservedPrefixes(t *testing.T) {
	intake, admission := customerIntakeHolding(t, "SYN-OWNER-01", accessidentity.CustomerCapabilityInboundPreAdviceSubmission)
	writes := &countingPreAdviceWrites{}
	handler := warehousehttp.NewSubmitInboundPreAdviceEndpoint(intake, writes)
	for _, body := range []string{
		`{"tenantId":"SYN-TENANT-02",` + customerPreAdviceBody[1:],
		`{"ownerAccount":"SYN-OWNER-02",` + customerPreAdviceBody[1:],
		`{"registrant":"someone",` + customerPreAdviceBody[1:],
		strings.Replace(customerPreAdviceBody, "SYN-ACCESS-IPA-0001", "withdrawal:abc", 1),
		strings.Replace(customerPreAdviceBody, "SYN-ACCESS-IPA-0001", "inbound-pre-advice:abc", 1),
	} {
		writes.calls = 0
		response := postCustomer(handler, body)
		assertProblemDetails(t, body, response, http.StatusBadRequest, "MALFORMED_REQUEST")
		if writes.calls != 0 {
			t.Fatalf("%s：坏载荷仍落了册", body)
		}
	}
	if len(admission.asked) == 0 {
		t.Fatal("写口该问过准入才读到坏标识；一份都没问，说明认证没走到推导")
	}
	for _, requirement := range admission.asked {
		if requirement.Capability != "INBOUND_PRE_ADVICE_SUBMISSION" || requirement.FactKind != "INBOUND_PRE_ADVICE_SUBMISSION" {
			t.Fatalf("准入要求 = %+v，两格都要是这一口的字面量", requirement)
		}
	}
}

// Covers: AT-WF-035——只持提交授予去撤回，答未授予，不读册，也不问准入。
func TestCustomerWithdrawalWithOnlyTheSubmissionGrantDoesNotReadTheBook(t *testing.T) {
	intake, admission := customerIntakeHolding(t, "SYN-OWNER-01", accessidentity.CustomerCapabilityInboundPreAdviceSubmission)
	writes := &countingPreAdviceWrites{}
	handler := warehousehttp.NewWithdrawInboundPreAdviceEndpoint(intake, writes)
	response := postCustomer(handler, `{"accessRequestId":"SYN-ACCESS-IPA-0001","preAdviceId":"IPA-1"}`)
	assertProblemDetails(t, "只持提交授予", response, http.StatusForbidden, "CUSTOMER_CLIENT_NOT_GRANTED")
	if writes.calls != 0 {
		t.Fatal("未授予仍读了册")
	}
	if len(admission.asked) != 0 {
		t.Fatalf("未授予仍问了准入：%+v", admission.asked)
	}
}

// Covers: 货主读口不带登记责任方；没有单据号改带 documentNumberAbsence；代录的带依据引用；别的货主与查无此预报都是 found 为假。
func TestCustomerPreAdviceReadsOmitTheRegistrantAndHideAnotherOwnersAdvice(t *testing.T) {
	intake, admission := customerIntakeHolding(t, "SYN-OWNER-01", accessidentity.CustomerCapabilityWarehouseRead)
	unnumbered := samplePreAdvice(t, "")
	numbered := samplePreAdvice(t, "IB-20261120-00001")
	queries := &scriptedPreAdviceQueries{one: readingOf(unnumbered), oneFound: true, byNumber: readingOf(numbered), byNumberFound: true}
	one := warehousehttp.NewGetCustomerInboundPreAdviceEndpoint(intake, queries)
	request := httptest.NewRequest(http.MethodGet, "/v1/warehouse/reads/inbound-pre-advice?preAdviceId=IPA-1", nil)
	request.Header.Set("Authorization", "Bearer presented")
	response := httptest.NewRecorder()
	one.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("读一份：%d %s", response.Code, response.Body.String())
	}
	object := decodeObject(t, response)
	var advice map[string]json.RawMessage
	if err := json.Unmarshal(object["preAdvice"], &advice); err != nil {
		t.Fatal(err)
	}
	if _, present := advice["registrant"]; present {
		t.Fatalf("货主读口带了登记责任方：%s", object["preAdvice"])
	}
	if _, present := advice["documentNumber"]; present {
		t.Fatal("没有单据号不该出现 documentNumber")
	}
	if stringMember(t, advice, "documentNumberAbsence") != "NUMBERING_TIME_ZONE_NOT_CONFIGURED" {
		t.Fatalf("无号原因 = %s", advice["documentNumberAbsence"])
	}
	if stringMember(t, advice, "submissionPath") == "" || stringMember(t, advice, "evidenceReference") == "" {
		t.Fatalf("途径或依据引用缺了：%s", object["preAdvice"])
	}
	var notices []map[string]json.RawMessage
	if err := json.Unmarshal(advice["notices"], &notices); err != nil || len(notices) == 0 {
		t.Fatalf("告知版本 = %s", advice["notices"])
	}
	if _, present := notices[0]["registrant"]; present {
		t.Fatal("告知版本带了登记责任方")
	}
	for _, absent := range []string{"receivedQuantity", "unclaimedArrival", "anotherArrivalPendingClaim", "device", "operator"} {
		if _, present := advice[absent]; present {
			t.Fatalf("编了 %s", absent)
		}
	}
	var cumulative []map[string]json.RawMessage
	if err := json.Unmarshal(advice["cumulativeReceived"], &cumulative); err != nil || len(cumulative) != 1 {
		t.Fatalf("累计实收 = %s", advice["cumulativeReceived"])
	}
	if stringMember(t, cumulative[0], "goodsCode") != "SYN-GOODS-01" || jsonNumber(t, cumulative[0], "quantity") != 0 {
		t.Fatalf("没收过的累计 = %s", advice["cumulativeReceived"])
	}
	if len(admission.asked) != 0 {
		t.Fatalf("读口问了准入：%+v", admission.asked)
	}

	carried := httptest.NewRequest(http.MethodGet, "/v1/warehouse/reads/inbound-pre-advice?preAdviceId=IPA-1&ownerAccount=SYN-OWNER-01", nil)
	carried.Header.Set("Authorization", "Bearer presented")
	refused := httptest.NewRecorder()
	queries.calls = 0
	one.ServeHTTP(refused, carried)
	assertProblemDetails(t, "查询自带货主", refused, http.StatusBadRequest, "MALFORMED_REQUEST")
	if queries.calls != 0 {
		t.Fatalf("坏查询仍去读了册：%d", queries.calls)
	}

	otherIntake, _ := customerIntakeHolding(t, "SYN-OWNER-02", accessidentity.CustomerCapabilityWarehouseRead)
	byNumber := warehousehttp.NewGetCustomerInboundPreAdviceByNumberEndpoint(otherIntake, queries)
	byNumberRequest := httptest.NewRequest(http.MethodGet, "/v1/warehouse/reads/inbound-pre-advice-by-number?documentNumber=IB-20261120-00001", nil)
	byNumberRequest.Header.Set("Authorization", "Bearer presented")
	hidden := httptest.NewRecorder()
	byNumber.ServeHTTP(hidden, byNumberRequest)
	hiddenObject := decodeObject(t, hidden)
	if string(hiddenObject["found"]) != "false" {
		t.Fatalf("别的货主 = %s", hidden.Body.String())
	}
	if _, present := hiddenObject["preAdvice"]; present {
		t.Fatal("别的货主的预报不该带出正文")
	}

	listIntake, _ := customerIntakeHolding(t, "SYN-OWNER-01", accessidentity.CustomerCapabilityWarehouseRead)
	list := warehousehttp.NewListCustomerInboundPreAdvicesEndpoint(listIntake, &scriptedPreAdviceQueries{})
	listRequest := httptest.NewRequest(http.MethodGet, "/v1/warehouse/reads/inbound-pre-advices", nil)
	listRequest.Header.Set("Authorization", "Bearer presented")
	listed := httptest.NewRecorder()
	list.ServeHTTP(listed, listRequest)
	if listed.Code != http.StatusOK || !strings.Contains(listed.Body.String(), `"preAdvices":[]`) {
		t.Fatalf("空列表 = %d %s", listed.Code, listed.Body.String())
	}
}
