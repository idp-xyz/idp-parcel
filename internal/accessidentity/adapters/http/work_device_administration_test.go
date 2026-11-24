package identityhttp_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	bentoapp "go.idp.xyz/idp-bento-go/application"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	identityhttp "go.idp.xyz/idp-parcel/internal/accessidentity/adapters/http"
)

type stubAdminAuth struct {
	admin identityhttp.OperatorAccessAdministrator
	err   error
}

func (auth stubAdminAuth) AuthenticateAccessAdministration(context.Context, string) (identityhttp.OperatorAccessAdministrator, error) {
	return auth.admin, auth.err
}

type stubDeviceTransactor struct{ opened int }

func (tx *stubDeviceTransactor) WithinTransaction(ctx context.Context, fn bentoapp.TxFunc) error {
	tx.opened++
	return fn(ctx)
}

type stubDeviceRegistrar struct {
	outcome     accessidentity.DeviceRegistrationOutcome
	err         error
	registered  accessidentity.DeviceRegistration
	deactivated accessidentity.DeviceDeactivation
	registerCnt int
	deactivCnt  int
}

func (stub *stubDeviceRegistrar) RegisterDevice(_ context.Context, reg accessidentity.DeviceRegistration) (accessidentity.DeviceRegistrationOutcome, error) {
	stub.registerCnt++
	stub.registered = reg
	return stub.outcome, stub.err
}

func (stub *stubDeviceRegistrar) RegisterDeactivation(_ context.Context, deact accessidentity.DeviceDeactivation) (accessidentity.DeviceRegistrationOutcome, error) {
	stub.deactivCnt++
	stub.deactivated = deact
	return stub.outcome, stub.err
}

func TestWorkDeviceAdministrationRegistersAndDeactivatesInATransaction(t *testing.T) {
	auth := stubAdminAuth{admin: identityhttp.OperatorAccessAdministrator{TenantID: "SYN-TENANT-01"}}
	tx := &stubDeviceTransactor{}
	reg := &stubDeviceRegistrar{outcome: accessidentity.DeviceRegistrationRecorded}

	registerEndpoint, err := identityhttp.NewRegisterWorkDeviceEndpoint(auth, tx, reg)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, identityhttp.WorkDeviceRegistrationsPath, strings.NewReader(
		`{"deviceId":"WEB-WH01-01","workScope":{"kind":"WAREHOUSE","reference":"SYN-WH-01"},"basis":"WEB-DESK-01"}`,
	))
	req.Header.Set("Authorization", "Bearer tok")
	rec := httptest.NewRecorder()
	registerEndpoint.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated || tx.opened != 1 || reg.registerCnt != 1 ||
		reg.registered.TenantID() != "SYN-TENANT-01" || reg.registered.DeviceID() != "WEB-WH01-01" ||
		reg.registered.WorkScope().Kind() != accessidentity.WorkScopeWarehouse || reg.registered.WorkScope().Reference() != "SYN-WH-01" {
		t.Fatalf("register: code=%d body=%s tx=%d reg=%+v", rec.Code, rec.Body.String(), tx.opened, reg.registered)
	}

	// 携带 tenantId 是未知字段，答 400 且不开事务。
	badReq := httptest.NewRequest(http.MethodPost, identityhttp.WorkDeviceRegistrationsPath, strings.NewReader(
		`{"tenantId":"OTHER","deviceId":"WEB-WH01-01","workScope":{"kind":"WAREHOUSE","reference":"SYN-WH-01"},"basis":"B"}`,
	))
	badRec := httptest.NewRecorder()
	registerEndpoint.ServeHTTP(badRec, badReq)
	if badRec.Code != http.StatusBadRequest || tx.opened != 1 {
		t.Fatalf("carried tenant: code=%d tx=%d", badRec.Code, tx.opened)
	}

	reg.outcome = accessidentity.DeviceNotRegistered
	deactivateEndpoint, err := identityhttp.NewDeactivateWorkDeviceEndpoint(auth, tx, reg)
	if err != nil {
		t.Fatal(err)
	}
	deactReq := httptest.NewRequest(http.MethodPost, identityhttp.WorkDeviceDeactivationsPath, strings.NewReader(
		`{"deviceId":"WEB-WH01-01","deactivatedAt":"2026-11-24T08:00:00Z","basis":"RETIRE-01"}`,
	))
	deactRec := httptest.NewRecorder()
	deactivateEndpoint.ServeHTTP(deactRec, deactReq)
	var body struct {
		Outcome string `json:"outcome"`
	}
	if err := json.Unmarshal(deactRec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if deactRec.Code != http.StatusOK || body.Outcome != "DEVICE_NOT_REGISTERED" || reg.deactivCnt != 1 {
		t.Fatalf("deactivate: code=%d body=%s", deactRec.Code, deactRec.Body.String())
	}

	reg.err = errors.New("db down")
	failRec := httptest.NewRecorder()
	deactivateEndpoint.ServeHTTP(failRec, httptest.NewRequest(http.MethodPost, identityhttp.WorkDeviceDeactivationsPath, strings.NewReader(
		`{"deviceId":"WEB-WH01-01","deactivatedAt":"2026-11-24T08:00:00Z","basis":"RETIRE-01"}`,
	)))
	if failRec.Code != http.StatusInternalServerError {
		t.Fatalf("db down: code=%d", failRec.Code)
	}
}
