package customshttp_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	customshttp "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/http"
	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

// stubCredentials 是读口替身：记录收到的键，交回预置的册子或故障。
type stubCredentials struct {
	entries []ports.CredentialCatalogueEntry
	err     error

	gotTenant string
	gotLimit  int
}

func (stub *stubCredentials) ListCredentials(
	_ context.Context, tenant domain.TenantID, limit int,
) ([]ports.CredentialCatalogueEntry, error) {
	stub.gotTenant, stub.gotLimit = tenant.String(), limit
	return stub.entries, stub.err
}

// unreachableCredentials 断言读口未被触到：传输形状的拒绝与未配置格都发生在读库之前。
type unreachableCredentials struct{ t *testing.T }

func (stub unreachableCredentials) ListCredentials(
	context.Context, domain.TenantID, int,
) ([]ports.CredentialCatalogueEntry, error) {
	stub.t.Fatal("a refused request reached the credential register")
	return nil, nil
}

func credentialRequest(method string) *http.Request {
	return httptest.NewRequest(method, "/customs-credentials", nil)
}

func credentialOf(t *testing.T, id string, uses int) domain.RegulatoryCredential {
	t.Helper()
	credential, err := domain.RegisterCredential(
		endpointValue(t, domain.NewCredentialID, id),
		endpointValue(t, domain.NewRegulatoryAuthorityReference, "SYN-AUTHORITY-01"),
		endpointValue(t, domain.NewCredentialHolderReference, "SYN-HOLDER-01"),
		endpointValue(t, domain.NewCustomsProcedureReference, "SYN-PROC-01"),
		catalogueBaseAt, catalogueBaseAt.Add(30*24*time.Hour), uses,
	)
	if err != nil {
		t.Fatalf("构造凭证：%v", err)
	}
	return credential
}

// Covers: ADR-0022 — 方法不对不是业务答案：405 + Allow，读口不被触到。
func TestCredentialQueryRefusesNonGetMethods(t *testing.T) {
	endpoint := customshttp.NewQueryCredentialsEndpoint(
		grantedCatalogueIntake{tenant: "TENANT-1", limit: 50},
		unreachableCredentials{t: t},
	)
	response := httptest.NewRecorder()
	endpoint.ServeHTTP(response, credentialRequest(http.MethodPost))

	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusMethodNotAllowed)
	}
	if allow := response.Header().Get("Allow"); allow != http.MethodGet {
		t.Fatalf("Allow = %q, want GET", allow)
	}
}

// Covers: ADR-0055/ADR-0077 Decision 三 — 未配置 Intake 答 403，读口不被触到。
func TestUnconfiguredCredentialIntakeRefusesTheQuery(t *testing.T) {
	endpoint := customshttp.NewQueryCredentialsEndpoint(
		customshttp.UnconfiguredIntake{}, unreachableCredentials{t: t},
	)
	response := httptest.NewRecorder()
	endpoint.ServeHTTP(response, credentialRequest(http.MethodGet))

	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusForbidden)
	}
	if got := problemCode(t, response); got != "ACCESS_CHANNEL_NOT_CONFIGURED" {
		t.Fatalf("code = %q, want ACCESS_CHANNEL_NOT_CONFIGURED", got)
	}
	assertNoOutcome(t, response)
}

// Covers: ADR-0077 Decision 四 / 票 sa-cc/10 完成判据 1 — 空册以空数组在场（不是 null 也不是
// 未配置）；租户与页大小从作用域来，不采信请求自报。
func TestAnEmptyCredentialRegisterAnswersAnEmptyArray(t *testing.T) {
	register := &stubCredentials{}
	endpoint := customshttp.NewQueryCredentialsEndpoint(
		grantedCatalogueIntake{tenant: "TENANT-1", limit: 25}, register,
	)
	response := httptest.NewRecorder()
	endpoint.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/customs-credentials?tenant=TENANT-9&limit=9999", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
	}
	if register.gotTenant != "TENANT-1" || register.gotLimit != 25 {
		t.Fatalf("读口收到的键走样：tenant=%q limit=%d（自报查询参数被采信了）",
			register.gotTenant, register.gotLimit)
	}
	var body struct {
		Outcome     string          `json:"outcome"`
		Credentials json.RawMessage `json:"credentials"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode %s: %v", response.Body.Bytes(), err)
	}
	if body.Outcome != "CREDENTIALS_LISTED" || string(body.Credentials) != "[]" {
		t.Fatalf("空册没有以空数组在场：%s", response.Body.String())
	}
}

// Covers: 0014 自注 / 票 sa-cc/10 做法 2 — 一版凭证逐字段转写；次数额度「来源未提供」以
// uses 缺席表达（不写 0——0 会被读成额度已用尽），有额度的凭证 uses 在场；登记时间与有效期
// 两端各自一列。
func TestCredentialListTranscribesEachVersionVerbatim(t *testing.T) {
	registeredAt := catalogueBaseAt.Add(-time.Hour)
	register := &stubCredentials{
		entries: []ports.CredentialCatalogueEntry{
			{Credential: credentialOf(t, "SYN-CRED-01", 3), RegisteredAt: registeredAt},
			{Credential: credentialOf(t, "SYN-CRED-02", 0), RegisteredAt: registeredAt},
		},
	}
	endpoint := customshttp.NewQueryCredentialsEndpoint(
		grantedCatalogueIntake{tenant: "TENANT-1", limit: 25}, register,
	)
	response := httptest.NewRecorder()
	endpoint.ServeHTTP(response, credentialRequest(http.MethodGet))

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
	}
	var body struct {
		Outcome     string `json:"outcome"`
		Credentials []struct {
			Credential   string `json:"credential"`
			Issuer       string `json:"issuer"`
			Holder       string `json:"holder"`
			Procedure    string `json:"procedure"`
			ValidFrom    string `json:"validFrom"`
			ValidTo      string `json:"validTo"`
			Uses         *int   `json:"uses"`
			RegisteredAt string `json:"registeredAt"`
		} `json:"credentials"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode %s: %v", response.Body.Bytes(), err)
	}
	if body.Outcome != "CREDENTIALS_LISTED" || len(body.Credentials) != 2 {
		t.Fatalf("响应走样：%s", response.Body.String())
	}
	quota, unprovided := body.Credentials[0], body.Credentials[1]
	if quota.Credential != "SYN-CRED-01" || quota.Issuer != "SYN-AUTHORITY-01" ||
		quota.Holder != "SYN-HOLDER-01" || quota.Procedure != "SYN-PROC-01" ||
		quota.ValidFrom != catalogueBaseAt.Format(time.RFC3339Nano) ||
		quota.ValidTo != catalogueBaseAt.Add(30*24*time.Hour).Format(time.RFC3339Nano) ||
		quota.RegisteredAt != registeredAt.Format(time.RFC3339Nano) {
		t.Fatalf("凭证七件转写走样：%+v", quota)
	}
	if quota.Uses == nil || *quota.Uses != 3 {
		t.Fatalf("有额度的凭证 uses 该在场且为 3：%+v", quota)
	}
	if unprovided.Credential != "SYN-CRED-02" || unprovided.Uses != nil {
		t.Fatalf("来源未提供额度的凭证 uses 该缺席，不该写成 0：%+v", unprovided)
	}
}

// Covers: ADR-0022/ADR-0029 — 读不回是答案未形成（5xx），不伪装成空册。
func TestAFailingCredentialReadIsNoAnswerRatherThanAnEmptyRegister(t *testing.T) {
	endpoint := customshttp.NewQueryCredentialsEndpoint(
		grantedCatalogueIntake{tenant: "TENANT-1", limit: 25},
		&stubCredentials{err: errors.New("connection refused")},
	)
	response := httptest.NewRecorder()
	endpoint.ServeHTTP(response, credentialRequest(http.MethodGet))

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusInternalServerError)
	}
	if got := problemCode(t, response); got != "NO_ANSWER_FORMED" {
		t.Fatalf("code = %q, want NO_ANSWER_FORMED", got)
	}
	assertNoOutcome(t, response)
}
