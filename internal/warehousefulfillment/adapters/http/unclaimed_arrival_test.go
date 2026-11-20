package warehousehttp_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
	warehousehttp "go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/http"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

type unclaimedAuthStub struct {
	calls int
	err   error
}

func (stub *unclaimedAuthStub) AuthenticateUnclaimedArrival(
	_ context.Context,
	_, deviceID, _, _ string,
) (domain.TenantID, domain.ReceiptOperatorID, domain.ReceiptDeviceID, error) {
	stub.calls++
	if stub.err != nil {
		return domain.TenantID{}, domain.ReceiptOperatorID{}, domain.ReceiptDeviceID{}, stub.err
	}
	tenant, _ := domain.NewTenantID("SYN-TENANT-01")
	op, _ := domain.NewReceiptOperatorID("https://id.syn.example/dex#SYN-OP-1")
	dev, _ := domain.NewReceiptDeviceID(deviceID)
	return tenant, op, dev, nil
}

type unclaimedRegistrarStub struct {
	calls int
	err   error
}

func (stub *unclaimedRegistrarStub) Register(context.Context, application.RegisterUnclaimedArrivalCommand) (application.RegisterUnclaimedArrivalResult, error) {
	stub.calls++
	return application.RegisterUnclaimedArrivalResult{}, stub.err
}

// Covers: AT-WF-055、AT-WF-067——待认领到货作业事实登记口：非 WAREHOUSE 范围答 400 不认证；
// 认证先于读载荷（渠道未配置或设备停用即便传空对象仍答 403 且不调用核心）；认证通过后
// 封闭集外的到货种类、自带 tenantId/deviceId/operator 或仓引用与载荷不等均答 400。
func TestUnclaimedArrivalEndpointEnforcesAuthBeforeBodyAndClosedKind(t *testing.T) {
	validDoc := `{"factId":"SYN-UCA-FACT-1","occurredAt":"2026-11-20T01:00:00Z","deviceSequence":1,"warehouse":"SYN-WH-01","kind":"INBOUND","observation":"外箱无货主标，带单号 SYN-OBS-SECRET"}`
	post := func(handler http.Handler, kind, ref, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/warehouse-fulfillment-unclaimed-arrival-registrations", strings.NewReader(body))
		req.Header.Set(httpapi.HeaderWorkScopeKind, kind)
		req.Header.Set(httpapi.HeaderWorkScopeReference, ref)
		req.Header.Set(httpapi.HeaderWorkDevice, "SYN-DEVICE-WH")
		req.Header.Set("Authorization", "Bearer presented.operator.token")
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		return res
	}

	for _, tc := range []struct {
		name string
		err  error
		code string
	}{
		{"channel unconfigured", warehousehttp.ErrAccessChannelNotConfigured, "ACCESS_CHANNEL_NOT_CONFIGURED"},
		{"device not in use", warehousehttp.ErrDeviceNotInUse, "DEVICE_NOT_IN_USE"},
	} {
		auth := &unclaimedAuthStub{err: tc.err}
		reg := &unclaimedRegistrarStub{}
		intake, err := warehousehttp.NewUnclaimedArrivalWorkFactIntake(auth)
		if err != nil {
			t.Fatal(err)
		}
		endpoint := warehousehttp.NewRegisterUnclaimedArrivalEndpoint(intake, reg)
		if r := post(endpoint, "NODE", "SYN-WH-01", `{}`); r.Code != http.StatusBadRequest || auth.calls != 0 {
			t.Fatalf("%s NODE 范围 = %d 认证 %d", tc.name, r.Code, auth.calls)
		}
		r := post(endpoint, "WAREHOUSE", "SYN-WH-01", `{}`)
		if r.Code != http.StatusForbidden || reg.calls != 0 || auth.calls != 1 {
			t.Fatalf("%s = %d %s 认证 %d 核心 %d", tc.name, r.Code, r.Body.String(), auth.calls, reg.calls)
		}
		var problem map[string]any
		if err := json.Unmarshal(r.Body.Bytes(), &problem); err != nil {
			t.Fatal(err)
		}
		if problem["code"] != tc.code {
			t.Fatalf("%s 问题码 = %v，want %s", tc.name, problem, tc.code)
		}
	}

	for _, badBody := range []string{
		strings.Replace(validDoc, `"INBOUND"`, `"OTHER"`, 1),
		validDoc[:len(validDoc)-1] + `,"tenantId":"SYN-TENANT-01"}`,
		validDoc[:len(validDoc)-1] + `,"deviceId":"SYN-DEVICE-WH"}`,
		validDoc[:len(validDoc)-1] + `,"operator":"SYN-OP"}`,
	} {
		auth := &unclaimedAuthStub{}
		reg := &unclaimedRegistrarStub{}
		intake, _ := warehousehttp.NewUnclaimedArrivalWorkFactIntake(auth)
		r := post(warehousehttp.NewRegisterUnclaimedArrivalEndpoint(intake, reg), "WAREHOUSE", "SYN-WH-01", badBody)
		if r.Code != http.StatusBadRequest || auth.calls != 1 || reg.calls != 0 {
			t.Fatalf("坏载荷 %s = %d 认证 %d 核心 %d", badBody, r.Code, auth.calls, reg.calls)
		}
	}

	// 核心返回 error 时 500 不泄露观察内容。
	auth := &unclaimedAuthStub{}
	reg := &unclaimedRegistrarStub{err: errors.New("db error: SYN-OBS-SECRET")}
	intake, _ := warehousehttp.NewUnclaimedArrivalWorkFactIntake(auth)
	r := post(warehousehttp.NewRegisterUnclaimedArrivalEndpoint(intake, reg), "WAREHOUSE", "SYN-WH-01", validDoc)
	if r.Code != http.StatusInternalServerError || strings.Contains(r.Body.String(), "SYN-OBS-SECRET") {
		t.Fatalf("500 答 %d %s", r.Code, r.Body.String())
	}
}
