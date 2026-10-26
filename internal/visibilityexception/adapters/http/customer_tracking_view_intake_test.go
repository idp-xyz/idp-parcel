package visibilityhttp_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	visibilityhttp "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/http"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
)

// 本文件证首方客户渠道在 `/customer-tracking-view` 的 Intake（票 demo-chain-gaps/96；ADR-0139 决定四；UC-VE-008）：查询键的租户与
// 账户只取自认证结果，请求只给包裹定位，查询串封闭；认证的各格经端点答各自的码。认证方是替身。

// viewerDouble 是认证方替身：记下被出示的令牌，答给定的身份或错误。
type viewerDouble struct {
	viewer    visibilityhttp.CustomerViewer
	err       error
	presented *string
}

func (double viewerDouble) AuthenticateTrackingViewQuery(_ context.Context, credential visibilityhttp.PresentedCustomerCredential) (visibilityhttp.CustomerViewer, error) {
	if double.presented != nil {
		*double.presented = credential.Token
	}
	return double.viewer, double.err
}

func viewerOf(t *testing.T, customer string) visibilityhttp.CustomerViewer {
	t.Helper()
	return visibilityhttp.CustomerViewer{
		Tenant:   value(t, domain.NewTenantID, "tenant-1"),
		Customer: value(t, domain.NewCustomerAccountReference, customer),
	}
}

func customerIntake(t *testing.T, authenticator visibilityhttp.CustomerTrackingViewAuthenticator) *visibilityhttp.CustomerTrackingViewIntake {
	t.Helper()
	intake, err := visibilityhttp.NewCustomerTrackingViewIntake(authenticator)
	if err != nil {
		t.Fatal(err)
	}
	return intake
}

func customerQuery(target, token string) *http.Request {
	request := httptest.NewRequest(http.MethodGet, target, nil)
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	return request
}

// Covers: AT-VE-150、AT-VE-153 的 HTTP 半边——查询键的租户与账户取自认证结果，包裹取自 `parcel`；令牌原样交给认证方。
func TestCustomerTrackingViewIntakeKeysTheQueryByTheAuthenticatedAccount(t *testing.T) {
	var presented string
	intake := customerIntake(t, viewerDouble{viewer: viewerOf(t, "customer-1"), presented: &presented})
	query, err := intake.IntakeQuery(context.Background(), customerQuery("/customer-tracking-view?parcel=parcel-1", "presented.customer.token"))
	if err != nil {
		t.Fatalf("IntakeQuery: %v", err)
	}
	if query != queryOf(t, "customer-1", "parcel-1") {
		t.Fatalf("query = %+v", query)
	}
	if presented != "presented.customer.token" {
		t.Fatalf("交给认证方的令牌 = %q", presented)
	}
}

// Covers: AT-VE-171——查询串里自报的账户、租户或任何别的一格都答坏请求，不悄悄忽略；没有包裹定位、给了两次、给了空白也答坏请求；
// 解不开的段落（分号分隔、坏百分号转义）同样答坏请求，不丢掉那一段后照常作答。
func TestCustomerTrackingViewQueryStringIsClosed(t *testing.T) {
	intake := customerIntake(t, viewerDouble{viewer: viewerOf(t, "customer-1")})
	for name, target := range map[string]string{
		"self-declared account": "/customer-tracking-view?parcel=parcel-1&customerAccount=customer-2",
		"self-declared tenant":  "/customer-tracking-view?parcel=parcel-1&tenant=tenant-2",
		"no parcel":             "/customer-tracking-view",
		"parcel twice":          "/customer-tracking-view?parcel=parcel-1&parcel=parcel-2",
		"blank parcel":          "/customer-tracking-view?parcel=%20",
		"semicolon segment":     "/customer-tracking-view?parcel=parcel-1&customerAccount=customer-2;x",
		"bad percent escape":    "/customer-tracking-view?parcel=parcel-1&%zz=customer-2",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := intake.IntakeQuery(context.Background(), customerQuery(target, "t")); !errors.Is(err, visibilityhttp.ErrMalformedRequest) {
				t.Fatalf("err = %v, want ErrMalformedRequest", err)
			}
		})
	}
}

// Covers: 先认证、后读查询串——认证没过的调用方带一个坏查询串，答的是认证那一格，看不到查询串校验的结果。
func TestCustomerTrackingViewIntakeAuthenticatesBeforeReadingTheQueryString(t *testing.T) {
	intake := customerIntake(t, viewerDouble{err: visibilityhttp.ErrCustomerClientNotGranted})
	_, err := intake.IntakeQuery(context.Background(), customerQuery("/customer-tracking-view?customerAccount=customer-2", "t"))
	if !errors.Is(err, visibilityhttp.ErrCustomerClientNotGranted) || errors.Is(err, visibilityhttp.ErrMalformedRequest) {
		t.Fatalf("err = %v, want 只认 ErrCustomerClientNotGranted", err)
	}
}

// Covers: ADR-0139 决定四——Intake 的各格经端点答各自的码，都不带 outcome；查阅口没有准入那一格。
func TestCustomerTrackingViewEndpointAnswersTheCustomerGrades(t *testing.T) {
	for name, testCase := range map[string]struct {
		err    error
		status int
		code   string
	}{
		"not configured":      {visibilityhttp.ErrAccessChannelNotConfigured, http.StatusForbidden, "ACCESS_CHANNEL_NOT_CONFIGURED"},
		"credential rejected": {visibilityhttp.ErrCustomerClientCredentialRejected, http.StatusUnauthorized, "CUSTOMER_CLIENT_CREDENTIAL_REJECTED"},
		"not granted":         {visibilityhttp.ErrCustomerClientNotGranted, http.StatusForbidden, "CUSTOMER_CLIENT_NOT_GRANTED"},
		"dependency down":     {visibilityhttp.ErrIdentityDependencyUnavailable, http.StatusServiceUnavailable, "IDENTITY_DEPENDENCY_UNAVAILABLE"},
		"unexpected":          {errors.New("tenant reference did not translate"), http.StatusInternalServerError, "INTAKE_FAILED"},
	} {
		t.Run(name, func(t *testing.T) {
			endpoint := visibilityhttp.NewQueryCustomerTrackingViewEndpoint(customerIntake(t, viewerDouble{err: testCase.err}), readerDouble{}, noAttributions())
			recorder := httptest.NewRecorder()
			endpoint.ServeHTTP(recorder, customerQuery("/customer-tracking-view?parcel=parcel-1", "t"))
			if recorder.Code != testCase.status || problemCode(t, recorder) != testCase.code {
				t.Fatalf("answer = %d %s, want %d %s", recorder.Code, recorder.Body.String(), testCase.status, testCase.code)
			}
			assertNoOutcome(t, recorder)
		})
	}
}

// Covers: AT-VE-150、AT-VE-151——经首方 Intake：本账户的包裹答当前视图；别的账户名下确有视图的包裹，与根本不存在的包裹，答同一格
// VIEW_NOT_FOUND、同一份字节。
func TestCustomerTrackingViewThroughTheFirstPartyIntakeCannotProbeOtherAccounts(t *testing.T) {
	reader := readerDouble{views: map[readerKey]domain.CustomerTrackingView{
		{value(t, domain.NewTenantID, "tenant-1"), value(t, domain.NewCustomerAccountReference, "customer-1"), value(t, domain.NewTrackedParcelReference, "parcel-1")}: mixedView(t),
		{value(t, domain.NewTenantID, "tenant-1"), value(t, domain.NewCustomerAccountReference, "customer-2"), value(t, domain.NewTrackedParcelReference, "parcel-2")}: mixedView(t),
	}}
	attributions := &attributionDouble{owners: map[string]string{"parcel-1": "customer-1", "parcel-2": "customer-2"}}
	endpoint := visibilityhttp.NewQueryCustomerTrackingViewEndpoint(customerIntake(t, viewerDouble{viewer: viewerOf(t, "customer-1")}), reader, attributions)
	get := func(parcel string) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		endpoint.ServeHTTP(recorder, customerQuery("/customer-tracking-view?parcel="+parcel, "t"))
		return recorder
	}

	if own := get("parcel-1"); own.Code != http.StatusOK || outcomeOfBody(t, own) != "CURRENT_VIEW" {
		t.Fatalf("本账户包裹 = %d %s", own.Code, own.Body.String())
	}
	other, missing := get("parcel-2"), get("parcel-404")
	if other.Code != http.StatusOK || outcomeOfBody(t, other) != "VIEW_NOT_FOUND" {
		t.Fatalf("别的账户的包裹 = %d %s", other.Code, other.Body.String())
	}
	if other.Body.String() != missing.Body.String() || other.Code != missing.Code {
		t.Fatalf("别的账户的包裹 %d %q 与不存在的包裹 %d %q 可分辨", other.Code, other.Body.String(), missing.Code, missing.Body.String())
	}
}

func outcomeOfBody(t *testing.T, response *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Outcome string `json:"outcome"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode %s: %v", response.Body.Bytes(), err)
	}
	return body.Outcome
}

func TestCustomerTrackingViewIntakeNeedsAnAuthenticator(t *testing.T) {
	if _, err := visibilityhttp.NewCustomerTrackingViewIntake(nil); err == nil {
		t.Fatal("没有认证方的 Intake 构造出来了")
	}
}
