package identityhttp_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	identityhttp "go.idp.xyz/idp-parcel/internal/accessidentity/adapters/http"
)

const catalogueTenant = "SYN-TENANT-01"

var catalogueAt = time.Date(2026, 9, 25, 2, 0, 0, 0, time.UTC)

type catalogueFake struct {
	tenant string
	rows   []accessidentity.DeviceCatalogueRow
	err    error
	calls  int
}

func (fake *catalogueFake) ListDevices(_ context.Context, tenantID string) ([]accessidentity.DeviceCatalogueRow, error) {
	fake.calls++
	fake.tenant = tenantID
	if fake.err != nil {
		return nil, fake.err
	}
	return fake.rows, nil
}

type authenticatorFake struct {
	tenant string
	err    error
}

func (fake authenticatorFake) AuthenticateCatalogueRead(context.Context, string) (string, error) {
	return fake.tenant, fake.err
}

type readProbe struct {
	io.Reader
	read bool
}

func (probe *readProbe) Read(p []byte) (int, error) {
	probe.read = true
	return probe.Reader.Read(p)
}

type recordingMinter struct {
	inner   *accessidentity.OperatorMinter
	request accessidentity.OperatorRequest
}

func (minter *recordingMinter) MintOperator(
	ctx context.Context,
	credential accessidentity.OperatorCredential,
	request accessidentity.OperatorRequest,
) (accessidentity.OperatorEnvelope, error) {
	minter.request = request
	return minter.inner.MintOperator(ctx, credential, request)
}

func TestQueryWorkDevicesListsTheEnvelopeTenant(t *testing.T) {
	scope := catalogueScope(t, accessidentity.WorkScopeNode, "SYN-NODE-SZX-01")
	registered, err := accessidentity.NewDeviceCatalogueRow("SYN-DEVICE-A", scope, nil)
	if err != nil {
		t.Fatal(err)
	}
	offAt := catalogueAt
	deactivated, err := accessidentity.NewDeviceCatalogueRow("SYN-DEVICE-B", scope, &offAt)
	if err != nil {
		t.Fatal(err)
	}
	catalogue := &catalogueFake{rows: []accessidentity.DeviceCatalogueRow{registered, deactivated}}
	handler := catalogueHandler(t, authenticatorFake{tenant: catalogueTenant}, catalogue)

	body := &readProbe{Reader: strings.NewReader(`{"tenant":"SYN-TENANT-02"}`)}
	request := httptest.NewRequest(http.MethodGet, identityhttp.WorkDeviceCataloguePath+"?tenant=SYN-TENANT-02", body)
	request.Header.Set("Authorization", "Bearer presented.operator.token")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK || body.read {
		t.Fatalf("status %d bodyRead %v", response.Code, body.read)
	}
	if catalogue.tenant != catalogueTenant {
		t.Fatalf("册收到的租户 = %s, want 信封上的租户", catalogue.tenant)
	}
	var listed struct {
		Outcome string           `json:"outcome"`
		Devices []map[string]any `json:"devices"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	if listed.Outcome != "WORK_DEVICES_LISTED" || len(listed.Devices) != 2 {
		t.Fatalf("body = %s", response.Body.String())
	}
	assertDeviceKeys(t, listed.Devices[0], map[string]any{
		"deviceId": "SYN-DEVICE-A", "workScopeKind": "NODE", "workScopeRef": "SYN-NODE-SZX-01", "status": "REGISTERED",
	})
	assertDeviceKeys(t, listed.Devices[1], map[string]any{
		"deviceId": "SYN-DEVICE-B", "workScopeKind": "NODE", "workScopeRef": "SYN-NODE-SZX-01", "status": "DEACTIVATED",
		"deactivatedAt": offAt.UTC().Format(time.RFC3339Nano),
	})
}

func TestQueryWorkDevicesEmptyCatalogueIsAnEmptyList(t *testing.T) {
	handler := catalogueHandler(t, authenticatorFake{tenant: catalogueTenant}, &catalogueFake{})
	response := serve(handler, http.MethodGet)
	if response.Code != http.StatusOK {
		t.Fatal(response.Code, response.Body.String())
	}
	var listed struct {
		Devices []map[string]any `json:"devices"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	if listed.Devices == nil || len(listed.Devices) != 0 {
		t.Fatalf("空册 = %#v, want 空数组", listed.Devices)
	}
}

func TestQueryWorkDevicesAnswerGrades(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{"issuer unconfigured", accessidentity.ErrAccessChannelNotConfigured, http.StatusForbidden, "ACCESS_CHANNEL_NOT_CONFIGURED"},
		{"credential rejected", accessidentity.ErrCredentialRejected, http.StatusUnauthorized, "OPERATOR_CREDENTIAL_REJECTED"},
		{"not granted", accessidentity.ErrOperatorNotGranted, http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
		{"verifier unavailable", accessidentity.ErrCredentialVerifierUnavailable, http.StatusServiceUnavailable, "IDENTITY_DEPENDENCY_UNAVAILABLE"},
		{"registry unavailable", accessidentity.ErrOperatorRegistryUnavailable, http.StatusServiceUnavailable, "IDENTITY_DEPENDENCY_UNAVAILABLE"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			catalogue := &catalogueFake{}
			handler := catalogueHandler(t, authenticatorFake{err: testCase.err}, catalogue)
			response := serve(handler, http.MethodGet)
			if response.Code != testCase.status || problemCode(t, response) != testCase.code {
				t.Fatalf("status %d code %s body %s", response.Code, problemCode(t, response), response.Body.String())
			}
			if catalogue.calls != 0 {
				t.Fatal("认证没过却去读了册")
			}
		})
	}

	handler := catalogueHandler(t, authenticatorFake{tenant: catalogueTenant}, &catalogueFake{err: errors.New("db down")})
	response := serve(handler, http.MethodGet)
	if response.Code != http.StatusInternalServerError || problemCode(t, response) != "NO_ANSWER_FORMED" {
		t.Fatalf("读册失败 = %d %s", response.Code, response.Body.String())
	}

	post := serve(catalogueHandler(t, authenticatorFake{tenant: catalogueTenant}, &catalogueFake{}), http.MethodPost)
	if post.Code != http.StatusMethodNotAllowed || post.Header().Get("Allow") != http.MethodGet || problemCode(t, post) != "METHOD_NOT_ALLOWED" {
		t.Fatalf("POST = %d allow %s body %s", post.Code, post.Header().Get("Allow"), post.Body.String())
	}
}

func TestCatalogueAuthenticatorAsksForOperationsReadWithoutAdmission(t *testing.T) {
	read := standingWithFace(t, accessidentity.CapabilityMasterDataAndOperationsRead)
	recorded := &recordingMinter{inner: testMinter(t, verifierFake{subject: catalogueSubject(t)}, registryFake{standing: read, found: true})}
	authenticator, err := identityhttp.NewOperatorCatalogueAuthenticator(recorded)
	if err != nil {
		t.Fatal(err)
	}
	tenant, err := authenticator.AuthenticateCatalogueRead(context.Background(), "presented.operator.token")
	if err != nil || tenant != catalogueTenant {
		t.Fatalf("tenant %s err %v", tenant, err)
	}
	if recorded.request.Face != accessidentity.CapabilityMasterDataAndOperationsRead ||
		recorded.request.TenantID != "" || recorded.request.Admission != nil || recorded.request.DecisionKind != "" {
		t.Fatalf("铸造请求 = %+v", recorded.request)
	}

	workOnly := standingWithWorkFact(t)
	workAuth, err := identityhttp.NewOperatorCatalogueAuthenticator(testMinter(t, verifierFake{subject: catalogueSubject(t)}, registryFake{standing: workOnly, found: true}))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := workAuth.AuthenticateCatalogueRead(context.Background(), "token"); !errors.Is(err, accessidentity.ErrOperatorNotGranted) {
		t.Fatalf("只有作业事实授予 err = %v", err)
	}

	unconfigured, err := identityhttp.NewOperatorCatalogueAuthenticator(testMinter(t, accessidentity.UnconfiguredOperatorCredentialVerifier{}, registryFake{}))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := unconfigured.AuthenticateCatalogueRead(context.Background(), "token"); !errors.Is(err, accessidentity.ErrAccessChannelNotConfigured) {
		t.Fatalf("发行方未配置 err = %v", err)
	}
}

func TestWorkDeviceCatalogueConstructorsRejectNil(t *testing.T) {
	if _, err := identityhttp.NewOperatorCatalogueAuthenticator(nil); err == nil {
		t.Fatal("nil minter")
	}
	if _, err := identityhttp.NewQueryWorkDevicesEndpoint(nil, &catalogueFake{}); err == nil {
		t.Fatal("nil authenticator")
	}
	if _, err := identityhttp.NewQueryWorkDevicesEndpoint(authenticatorFake{tenant: catalogueTenant}, nil); err == nil {
		t.Fatal("nil catalogue")
	}
}

func catalogueHandler(t *testing.T, authenticator identityhttp.OperatorCatalogueAuthenticator, catalogue accessidentity.DeviceCatalogue) http.Handler {
	t.Helper()
	handler, err := identityhttp.NewQueryWorkDevicesEndpoint(authenticator, catalogue)
	if err != nil {
		t.Fatal(err)
	}
	return handler
}

func serve(handler http.Handler, method string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, identityhttp.WorkDeviceCataloguePath, nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func problemCode(t *testing.T, response *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	return body.Error.Code
}

func assertDeviceKeys(t *testing.T, got map[string]any, want map[string]any) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("键 = %v, want %v", got, want)
	}
	for key, value := range want {
		if got[key] != value {
			t.Fatalf("%s = %v, want %v; 整行 %v", key, got[key], value, got)
		}
	}
}

type verifierFake struct {
	subject accessidentity.OperatorSubject
	err     error
}

func (fake verifierFake) VerifyOperatorCredential(context.Context, accessidentity.OperatorCredential) (accessidentity.OperatorSubject, error) {
	if fake.err != nil {
		return accessidentity.OperatorSubject{}, fake.err
	}
	return fake.subject, nil
}

type registryFake struct {
	standing accessidentity.OperatorStanding
	found    bool
	err      error
}

func (fake registryFake) FindOperator(context.Context, accessidentity.OperatorSubject) (accessidentity.OperatorStanding, bool, error) {
	return fake.standing, fake.found, fake.err
}

func testMinter(t *testing.T, verifier accessidentity.OperatorCredentialVerifier, registry accessidentity.OperatorRegistry) *accessidentity.OperatorMinter {
	t.Helper()
	minter, err := accessidentity.NewOperatorMinter(verifier, registry, accessidentity.UnconfiguredAdmissionScope{}, func() time.Time { return catalogueAt })
	if err != nil {
		t.Fatal(err)
	}
	return minter
}

func catalogueSubject(t *testing.T) accessidentity.OperatorSubject {
	t.Helper()
	subject, err := accessidentity.NewOperatorSubject("https://id.syn.example/dex", "SYN-OPERATOR-01")
	if err != nil {
		t.Fatal(err)
	}
	return subject
}

func catalogueScope(t *testing.T, kind accessidentity.WorkScopeKind, reference string) accessidentity.WorkScope {
	t.Helper()
	scope, err := accessidentity.NewWorkScope(kind, reference)
	if err != nil {
		t.Fatal(err)
	}
	return scope
}

func standingWithFace(t *testing.T, face accessidentity.CapabilityFace) accessidentity.OperatorStanding {
	t.Helper()
	subject := catalogueSubject(t)
	binding, err := accessidentity.NewOperatorBinding(subject, catalogueTenant, "SYN-BASIS-BINDING")
	if err != nil {
		t.Fatal(err)
	}
	interval, err := accessidentity.NewEffectiveInterval(catalogueAt.Add(-time.Hour), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	grant, err := accessidentity.NewOperatorGrant(catalogueTenant, "SYN-GRANT-READ", subject, face, interval, "SYN-BASIS-GRANT")
	if err != nil {
		t.Fatal(err)
	}
	recorded, err := accessidentity.NewRecordedGrant(grant, nil)
	if err != nil {
		t.Fatal(err)
	}
	standing, err := accessidentity.NewOperatorStanding(binding, []accessidentity.RecordedGrant{recorded})
	if err != nil {
		t.Fatal(err)
	}
	return standing
}

func standingWithWorkFact(t *testing.T) accessidentity.OperatorStanding {
	t.Helper()
	subject := catalogueSubject(t)
	binding, err := accessidentity.NewOperatorBinding(subject, catalogueTenant, "SYN-BASIS-BINDING")
	if err != nil {
		t.Fatal(err)
	}
	interval, err := accessidentity.NewEffectiveInterval(catalogueAt.Add(-time.Hour), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	grant, err := accessidentity.NewWorkFactRegistrationGrant(catalogueTenant, "SYN-GRANT-WF", subject, catalogueScope(t, accessidentity.WorkScopeNode, "SYN-NODE-SZX-01"), interval, "SYN-BASIS-WF")
	if err != nil {
		t.Fatal(err)
	}
	recorded, err := accessidentity.NewRecordedGrant(grant, nil)
	if err != nil {
		t.Fatal(err)
	}
	standing, err := accessidentity.NewOperatorStanding(binding, []accessidentity.RecordedGrant{recorded})
	if err != nil {
		t.Fatal(err)
	}
	return standing
}
