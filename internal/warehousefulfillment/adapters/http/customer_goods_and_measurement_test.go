package warehousehttp_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
	whaccess "go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/accessidentity"
	warehousehttp "go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/http"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

const customerGoodsBody = `{
	"accessRequestId": "SYN-ACCESS-GOODS-0001",
	"goodsCode": "SYN-GOODS-01",
	"name": "合成商品",
	"baseUnit": "件",
	"packagingLevels": [{"name": "件", "factor": 1}]
}`

func customerGoodsIntakeHolding(
	t *testing.T,
	account string,
	capabilities ...accessidentity.CustomerCapability,
) (*warehousehttp.CustomerGoodsIntake, *recordingAdmission) {
	t.Helper()
	at := time.Date(2026, 11, 20, 6, 0, 0, 0, time.UTC)
	subject, err := accessidentity.NewCustomerClientSubject(customerIssuerIPA, "SYN-CUSTOMER-CLIENT-GOODS")
	if err != nil {
		t.Fatal(err)
	}
	reference, err := accessidentity.NewCredentialReference("SYN-CREDENTIAL-REF/customer-goods")
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
	minter, err := accessidentity.NewCustomerClientMinter(
		fixedCustomerVerifier{subject: subject},
		holdingCustomerRegistry{standing: standing},
		admission,
		func() time.Time { return at },
	)
	if err != nil {
		t.Fatal(err)
	}
	authenticator, err := whaccess.NewCustomerGoodsAuthenticator(minter)
	if err != nil {
		t.Fatal(err)
	}
	intake, err := warehousehttp.NewCustomerGoodsIntake(authenticator)
	if err != nil {
		t.Fatal(err)
	}
	return intake, admission
}

type capturingGoodsRegistrar struct {
	calls   int
	command application.RegisterGoodsCommand
	result  application.RegisterGoodsResult
	has     bool
	err     error
}

func (registrar *capturingGoodsRegistrar) Register(_ context.Context, command application.RegisterGoodsCommand) (application.RegisterGoodsResult, error) {
	registrar.calls++
	registrar.command = command
	if registrar.err != nil {
		return application.RegisterGoodsResult{}, registrar.err
	}
	if registrar.has {
		return registrar.result, nil
	}
	return application.RegisterGoodsResult{}, errors.New("no answer")
}

func postCustomerGoods(handler http.Handler, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, "/v1/warehouse/goods", strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer presented.customer.token")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func getCustomerGoods(handler http.Handler, target string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, target, nil)
	request.Header.Set("Authorization", "Bearer presented.customer.token")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

// Covers: AT-WF-014、ADR-0384 决定一、决定六、决定八——货主商品维护口问 GOODS_MAINTENANCE 准入（两格都是字面量），
// 来源请求键是 goods: 加去两端空白的接入请求标识；自带租户、货主、登记责任方或以保留前缀开头都答 400 且不写册。
func TestCustomerGoodsMaintenanceUsesOwnAdmissionPrefixAndRejectsCarriedIdentity(t *testing.T) {
	intake, admission := customerGoodsIntakeHolding(t, "SYN-OWNER-01", accessidentity.CustomerCapabilityGoodsMaintenance)
	registrar := &capturingGoodsRegistrar{err: errors.New("core down")}
	handler := warehousehttp.NewMaintainCustomerGoodsEndpoint(intake, registrar)

	for _, body := range []string{
		`{"tenantId":"SYN-TENANT-02",` + customerGoodsBody[1:],
		`{"ownerAccount":"SYN-OWNER-02",` + customerGoodsBody[1:],
		`{"registrant":"someone",` + customerGoodsBody[1:],
		`{"evidenceReference":"SYN-EV",` + customerGoodsBody[1:],
		strings.Replace(customerGoodsBody, "SYN-ACCESS-GOODS-0001", "withdrawal:abc", 1),
		strings.Replace(customerGoodsBody, "SYN-ACCESS-GOODS-0001", "goods:abc", 1),
	} {
		registrar.calls = 0
		response := postCustomerGoods(handler, body)
		assertProblemDetails(t, body, response, http.StatusBadRequest, "MALFORMED_REQUEST")
		if registrar.calls != 0 {
			t.Fatalf("%s：坏载荷仍进了登记编排", body)
		}
	}

	registrar.calls = 0
	admission.asked = nil
	okBody := strings.Replace(customerGoodsBody, "SYN-ACCESS-GOODS-0001", "  SYN-ACCESS-GOODS-0001  ", 1)
	response := postCustomerGoods(handler, okBody)
	if response.Code != http.StatusInternalServerError || registrar.calls != 1 {
		t.Fatalf("有效请求答 %d，核心 %d", response.Code, registrar.calls)
	}
	wantAdmission := accessidentity.AdmissionRequirement{
		Capability: accessidentity.CustomerCapabilityGoodsMaintenance.String(),
		FactKind:   accessidentity.CustomerCapabilityGoodsMaintenance.String(),
	}
	if len(admission.asked) != 1 || admission.asked[0] != wantAdmission {
		t.Fatalf("准入 = %+v，want %+v", admission.asked, wantAdmission)
	}
	if registrar.command.Owner.String() != "SYN-OWNER-01" ||
		registrar.command.Submission.Path() != domain.SubmissionByOwner ||
		registrar.command.Submission.Key().String() != "goods:SYN-ACCESS-GOODS-0001" {
		t.Fatalf("命令 = owner=%s path=%s key=%s", registrar.command.Owner, registrar.command.Submission.Path(), registrar.command.Submission.Key())
	}
}

// Covers: AT-WF-014——没有 GOODS_MAINTENANCE 授予答 403 CUSTOMER_CLIENT_NOT_GRANTED，不读载荷也不问准入。
func TestCustomerGoodsMaintenanceRejectsMissingGrant(t *testing.T) {
	intake, admission := customerGoodsIntakeHolding(t, "SYN-OWNER-01", accessidentity.CustomerCapabilityWarehouseRead)
	registrar := &capturingGoodsRegistrar{}
	response := postCustomerGoods(warehousehttp.NewMaintainCustomerGoodsEndpoint(intake, registrar), `not json`)
	assertProblemDetails(t, "missing grant", response, http.StatusForbidden, "CUSTOMER_CLIENT_NOT_GRANTED")
	if registrar.calls != 0 || len(admission.asked) != 0 {
		t.Fatalf("未授予仍调用核心 %d 或问准入 %+v", registrar.calls, admission.asked)
	}
}

// Covers: UC-WF-002「权限、隔离与审计」与 AT-WF-023——货主读口带出代录途径与依据引用、不带登记责任方，也不带仓测上的设备与操作者；
// 查询串自带 ownerAccount 或 tenantId 答 400。
func TestCustomerGoodsReadsHideRegistrantAndMeasurementActors(t *testing.T) {
	intake, admission := customerGoodsIntakeHolding(t, "SYN-OWNER-01", accessidentity.CustomerCapabilityWarehouseRead)
	now := time.Date(2026, 11, 20, 7, 0, 0, 0, time.UTC)
	row := ports.GoodsRow{
		OwnerAccount:      "SYN-OWNER-01",
		GoodsCode:         "SYN-GOODS-01",
		Revision:          2,
		Name:              "合成商品",
		BaseUnit:          "件",
		PackagingLevels:   []ports.GoodsLevelRow{{Name: "件", Factor: 1}},
		SubmissionPath:    "ON_BEHALF",
		EvidenceReference: "SYN-EVIDENCE-ON-BEHALF",
		Registrant:        "https://id.syn.example/dex#SYN-OPERATOR-SECRET",
		RegisteredAt:      now,
		WarehouseMeasurements: []ports.GoodsLevelMeasurementRow{
			{
				Level:    "件",
				Measured: true,
				Current: &ports.WarehouseMeasurementRow{
					FactID:         "SYN-MEAS-01",
					WarehouseCode:  "SYN-WH-01",
					Device:         "SYN-DEVICE-SECRET",
					Operator:       "https://id.syn.example/dex#SYN-OP-MEAS-SECRET",
					OccurredAt:     now,
					DeviceSequence: 3,
					ReceivedAt:     now.Add(time.Second),
					Dimensions:     ports.GoodsDimensionsRow{Length: "30", Width: "20", Height: "10", Unit: "CM"},
					Weight:         ports.GoodsWeightRow{Value: "1.5", Unit: "KG"},
				},
			},
		},
	}
	reader := staticGoodsReader{row: row}
	currentEndpoint := warehousehttp.NewGetCustomerCurrentGoodsEndpoint(intake, reader)

	badQuery := getCustomerGoods(currentEndpoint, "/v1/warehouse/reads/goods-current?ownerAccount=SYN-OWNER-02&goodsCode=SYN-GOODS-01")
	assertProblemDetails(t, "carried ownerAccount", badQuery, http.StatusBadRequest, "MALFORMED_REQUEST")

	response := getCustomerGoods(currentEndpoint, "/v1/warehouse/reads/goods-current?goodsCode=SYN-GOODS-01")
	if response.Code != http.StatusOK {
		t.Fatalf("答 %d %s", response.Code, response.Body.String())
	}
	bodyText := response.Body.String()
	for _, hidden := range []string{"registrant", "SYN-OPERATOR-SECRET", "SYN-DEVICE-SECRET", "SYN-OP-MEAS-SECRET"} {
		if strings.Contains(bodyText, hidden) {
			t.Fatalf("货主读口泄露了 %q：%s", hidden, bodyText)
		}
	}
	if !strings.Contains(bodyText, `"submissionPath":"ON_BEHALF"`) || !strings.Contains(bodyText, `"evidenceReference":"SYN-EVIDENCE-ON-BEHALF"`) {
		t.Fatalf("货主读口缺少代录途径或依据引用：%s", bodyText)
	}
	if len(admission.asked) != 0 {
		t.Fatalf("货主读口不该判准入：%+v", admission.asked)
	}
}

type staticGoodsReader struct {
	row ports.GoodsRow
}

func (reader staticGoodsReader) ListGoods(context.Context, domain.TenantID, domain.OwnerAccountID, int, cataloguepage.Query) (cataloguepage.Result[ports.GoodsRow], error) {
	return cataloguepage.Result[ports.GoodsRow]{Rows: []ports.GoodsRow{reader.row}, Total: 1}, nil
}

func (reader staticGoodsReader) CurrentGoods(context.Context, domain.TenantID, domain.OwnerAccountID, domain.GoodsCode) (ports.GoodsRow, bool, error) {
	return reader.row, true, nil
}

func (reader staticGoodsReader) GoodsRevisions(context.Context, domain.TenantID, domain.OwnerAccountID, domain.GoodsCode) ([]ports.GoodsRow, error) {
	return []ports.GoodsRow{reader.row}, nil
}

func (reader staticGoodsReader) FindGoods(context.Context, domain.TenantID, domain.OwnerAccountID, domain.IdentifierCode) (ports.GoodsRow, bool, error) {
	return reader.row, true, nil
}

// Covers: 仓测作业事实 HTTP 入口的信封约束：范围种类非 WAREHOUSE 直接答 400 且不认证；
// 认证先于读请求体（未配置、设备停用或未授予在读载荷之前即答其格）；认证通过后自带 tenantId/deviceId/operator
// 或范围引用不等于载荷 warehouse 答 400 且不调用核心。
func TestWarehouseMeasurementEndpointEnforcesWorkFactEnvelope(t *testing.T) {
	doc := `{"factId":"SYN-MEAS-01","occurredAt":"2026-11-20T01:00:00Z","deviceSequence":1,"warehouse":"SYN-WH-01","targetKind":"GOODS_LEVEL","ownerAccount":"SYN-OWNER-01","goodsCode":"SYN-GOODS-01","packagingLevel":"件","dimensions":{"length":"30","width":"20","height":"10","unit":"CM"},"weight":{"value":"1.5","unit":"KG"}}`
	post := func(handler http.Handler, kind, ref, body string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPost, "/warehouse-fulfillment-warehouse-measurements", strings.NewReader(body))
		request.Header.Set(httpapi.HeaderWorkScopeKind, kind)
		request.Header.Set(httpapi.HeaderWorkScopeReference, ref)
		request.Header.Set(httpapi.HeaderWorkDevice, "SYN-DEVICE-WH")
		request.Header.Set("Authorization", "Bearer presented.operator.token")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		return response
	}

	// 认证先于读载荷：渠道未配置或设备停用时即使请求体为空对象，仍答 403 且不调用核心。
	for _, tc := range []struct {
		name string
		err  error
		code string
	}{
		{"channel unconfigured", warehousehttp.ErrAccessChannelNotConfigured, "ACCESS_CHANNEL_NOT_CONFIGURED"},
		{"device not in use", warehousehttp.ErrDeviceNotInUse, "DEVICE_NOT_IN_USE"},
	} {
		auth := &measurementAuthStub{err: tc.err}
		rec := &measurementRecorderStub{}
		intake, err := warehousehttp.NewWarehouseMeasurementWorkFactIntake(auth)
		if err != nil {
			t.Fatal(err)
		}
		endpoint := warehousehttp.NewRecordWarehouseMeasurementEndpoint(intake, rec)
		if r := post(endpoint, "NODE", "SYN-WH-01", `{}`); r.Code != http.StatusBadRequest || auth.calls != 0 {
			t.Fatalf("%s NODE 范围：答 %d 认证 %d", tc.name, r.Code, auth.calls)
		}
		r := post(endpoint, "WAREHOUSE", "SYN-WH-01", `{}`)
		if r.Code != http.StatusForbidden || r.Header().Get("Content-Type") != httpapi.ProblemMediaType || rec.calls != 0 || auth.calls != 1 {
			t.Fatalf("%s：答 %d %s 认证 %d 核心 %d", tc.name, r.Code, r.Body.String(), auth.calls, rec.calls)
		}
		var problem map[string]any
		if err := json.Unmarshal(r.Body.Bytes(), &problem); err != nil {
			t.Fatal(err)
		}
		if problem["code"] != tc.code {
			t.Fatalf("%s 问题码 = %v，want %s", tc.name, problem, tc.code)
		}
	}

	// 认证通过后，自带身份格或仓引用不等于载荷 warehouse 均答 400 且不调核心。
	for _, extra := range []string{`"tenantId":"SYN-TENANT-01"`, `"deviceId":"SYN-DEVICE-WH"`, `"operator":"SYN-OP"`} {
		auth := &measurementAuthStub{}
		rec := &measurementRecorderStub{}
		intake, err := warehousehttp.NewWarehouseMeasurementWorkFactIntake(auth)
		if err != nil {
			t.Fatal(err)
		}
		badBody := doc[:len(doc)-1] + "," + extra + "}"
		response := post(warehousehttp.NewRecordWarehouseMeasurementEndpoint(intake, rec), "WAREHOUSE", "SYN-WH-01", badBody)
		if response.Code != http.StatusBadRequest || auth.calls != 1 || rec.calls != 0 {
			t.Fatalf("%s：答 %d 认证 %d 核心 %d", extra, response.Code, auth.calls, rec.calls)
		}
	}
	auth := &measurementAuthStub{}
	rec := &measurementRecorderStub{}
	intake, err := warehousehttp.NewWarehouseMeasurementWorkFactIntake(auth)
	if err != nil {
		t.Fatal(err)
	}
	if r := post(warehousehttp.NewRecordWarehouseMeasurementEndpoint(intake, rec), "WAREHOUSE", "SYN-WH-02", doc); r.Code != http.StatusBadRequest || auth.calls != 1 || rec.calls != 0 {
		t.Fatalf("仓引用不等：答 %d 认证 %d 核心 %d", r.Code, auth.calls, rec.calls)
	}
}

type measurementAuthStub struct {
	calls int
	err   error
}

func (stub *measurementAuthStub) AuthenticateWarehouseMeasurement(
	_ context.Context,
	_, deviceID, _, _ string,
) (domain.TenantID, domain.MeasurementOperatorID, domain.MeasurementDeviceID, error) {
	stub.calls++
	if stub.err != nil {
		return domain.TenantID{}, domain.MeasurementOperatorID{}, domain.MeasurementDeviceID{}, stub.err
	}
	tenant, _ := domain.NewTenantID("SYN-TENANT-01")
	op, _ := domain.NewMeasurementOperatorID("https://id.syn.example/dex#SYN-OP-1")
	dev, _ := domain.NewMeasurementDeviceID(deviceID)
	return tenant, op, dev, nil
}

type measurementRecorderStub struct {
	calls int
}

func (stub *measurementRecorderStub) Record(context.Context, application.RecordWarehouseMeasurementCommand) (application.RecordWarehouseMeasurementResult, error) {
	stub.calls++
	return application.RecordWarehouseMeasurementResult{}, errors.New("not wired")
}
