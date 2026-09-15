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

// stubDutyVerifications 是读口替身：记录收到的键，交回预置的册子或故障。
type stubDutyVerifications struct {
	records []ports.DutyVerificationRecord
	err     error

	gotTenant string
	gotLimit  int
}

func (stub *stubDutyVerifications) ListDutyVerifications(
	_ context.Context, tenant domain.TenantID, limit int,
) ([]ports.DutyVerificationRecord, error) {
	stub.gotTenant, stub.gotLimit = tenant.String(), limit
	return stub.records, stub.err
}

// unreachableDutyVerifications 断言读口未被触到。
type unreachableDutyVerifications struct{ t *testing.T }

func (stub unreachableDutyVerifications) ListDutyVerifications(
	context.Context, domain.TenantID, int,
) ([]ports.DutyVerificationRecord, error) {
	stub.t.Fatal("a refused request reached the duty verification register")
	return nil, nil
}

func verificationRequest(method string) *http.Request {
	return httptest.NewRequest(method, "/customs-duty-verifications", nil)
}

func verificationRecordOf(
	t *testing.T,
	digest string,
	coverage domain.DutyCoverage,
	delta domain.DutyDelta,
	validity domain.DutyFactValidity,
	verifiedAt time.Time,
) ports.DutyVerificationRecord {
	t.Helper()
	duty := endpointValue(t, domain.NewAssessedDutyReference, "SYN-DUTY-01/v1")
	funds := endpointValue(t, domain.NewExternalFundsFactReference, "SYN-FUNDS-01")
	fundsVersion := endpointValue(t, domain.NewFundsFactVersion, "SYN-FUNDS-01/v1")
	scope := endpointValue(t, domain.NewDecisionScopeReference, "SYN-UNIT-01")
	procedure := endpointValue(t, domain.NewCustomsProcedureReference, "SYN-PROC-IMPORT")
	verification, err := domain.VerifyDutyPayment(duty, funds, fundsVersion, scope, procedure, coverage, delta, validity, verifiedAt)
	if err != nil {
		t.Fatalf("构造核对：%v", err)
	}
	return ports.DutyVerificationRecord{
		Key: ports.DutyVerificationKey{
			TenantID: endpointValue(t, domain.NewTenantID, "TENANT-1"),
			Duty:     duty, Funds: funds, Scope: scope, Digest: digest,
		},
		Verification: verification,
		Basis:        "SYN-RULE-01: remittance quotes assessment",
	}
}

// Covers: ADR-0022 — 方法不对不是业务答案：405 + Allow，读口不被触到。
func TestDutyVerificationQueryRefusesNonGetMethods(t *testing.T) {
	endpoint := customshttp.NewQueryDutyVerificationsEndpoint(
		grantedCatalogueIntake{tenant: "TENANT-1", limit: 50},
		unreachableDutyVerifications{t: t},
	)
	response := httptest.NewRecorder()
	endpoint.ServeHTTP(response, verificationRequest(http.MethodPost))

	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusMethodNotAllowed)
	}
	if allow := response.Header().Get("Allow"); allow != http.MethodGet {
		t.Fatalf("Allow = %q, want GET", allow)
	}
}

// Covers: ADR-0055/ADR-0077 Decision 三 — 未配置 Intake 答 403，读口不被触到。
func TestUnconfiguredDutyVerificationIntakeRefusesTheQuery(t *testing.T) {
	endpoint := customshttp.NewQueryDutyVerificationsEndpoint(
		customshttp.UnconfiguredIntake{}, unreachableDutyVerifications{t: t},
	)
	response := httptest.NewRecorder()
	endpoint.ServeHTTP(response, verificationRequest(http.MethodGet))

	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusForbidden)
	}
	if got := problemCode(t, response); got != "ACCESS_CHANNEL_NOT_CONFIGURED" {
		t.Fatalf("code = %q, want ACCESS_CHANNEL_NOT_CONFIGURED", got)
	}
	assertNoOutcome(t, response)
}

// Covers: ADR-0077 Decision 四 / 票 sa-cc/10 完成判据 1 — 空册以空数组在场；租户与页大小
// 从作用域来，不采信请求自报。
func TestAnEmptyDutyVerificationRegisterAnswersAnEmptyArray(t *testing.T) {
	register := &stubDutyVerifications{}
	endpoint := customshttp.NewQueryDutyVerificationsEndpoint(
		grantedCatalogueIntake{tenant: "TENANT-1", limit: 25}, register,
	)
	response := httptest.NewRecorder()
	endpoint.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/customs-duty-verifications?tenant=TENANT-9&limit=9999", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
	}
	if register.gotTenant != "TENANT-1" || register.gotLimit != 25 {
		t.Fatalf("读口收到的键走样：tenant=%q limit=%d", register.gotTenant, register.gotLimit)
	}
	var body struct {
		Outcome       string          `json:"outcome"`
		Verifications json.RawMessage `json:"verifications"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode %s: %v", response.Body.Bytes(), err)
	}
	if body.Outcome != "DUTY_VERIFICATIONS_LISTED" || string(body.Verifications) != "[]" {
		t.Fatalf("空册没有以空数组在场：%s", response.Body.String())
	}
}

// Covers: ADR-0137 决定三 / CONTEXT「不能实现为一组互斥总状态」 — 三轴逐键原值、没有任何合成总状态列；同键
// 多版本各自成行、版本指纹与关联依据原样透出；付款人维按哪个程序的规则判随行透出（票 sa-cc/22 完成判据 (2)
// 「读回带程序」里的 HTTP JSON 那一处）；比的是资金事实的哪一版随行透出（票 sa-cc/19：记录列 `funds_version`
// 在读面上可见，读者才分得出哪版核对判的是已被取代的那一版事实）。响应形封闭：键集就是这些，多一个「status」
// 都是把三轴折回互斥总状态。
func TestDutyVerificationListTranscribesThreeAxesVerbatimWithoutFolding(t *testing.T) {
	later := catalogueBaseAt.Add(2 * time.Hour)
	register := &stubDutyVerifications{
		records: []ports.DutyVerificationRecord{
			verificationRecordOf(t, "digest-v1", domain.CoveragePartial, domain.DeltaShort, domain.FundsFactPending, catalogueBaseAt),
			verificationRecordOf(t, "digest-v2", domain.CoverageFull, domain.DeltaNone, domain.FundsFactValid, later),
		},
	}
	endpoint := customshttp.NewQueryDutyVerificationsEndpoint(
		grantedCatalogueIntake{tenant: "TENANT-1", limit: 25}, register,
	)
	response := httptest.NewRecorder()
	endpoint.ServeHTTP(response, verificationRequest(http.MethodGet))

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
	}
	var body struct {
		Outcome       string                       `json:"outcome"`
		Verifications []map[string]json.RawMessage `json:"verifications"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode %s: %v", response.Body.Bytes(), err)
	}
	if body.Outcome != "DUTY_VERIFICATIONS_LISTED" || len(body.Verifications) != 2 {
		t.Fatalf("响应走样：%s", response.Body.String())
	}
	want := map[string]string{
		"duty": "SYN-DUTY-01/v1", "funds": "SYN-FUNDS-01", "fundsVersion": "SYN-FUNDS-01/v1", "scope": "SYN-UNIT-01", "procedure": "SYN-PROC-IMPORT",
		"version": "digest-v1", "coverage": "PARTIAL", "delta": "SHORT", "validity": "PENDING",
		"basis": "SYN-RULE-01: remittance quotes assessment", "verifiedAt": catalogueBaseAt.Format(time.RFC3339Nano),
	}
	first := body.Verifications[0]
	if len(first) != len(want) {
		t.Fatalf("响应键集不封闭（要恰好 %d 键，三轴之外不得多出合成列）：%s", len(want), response.Body.String())
	}
	for key, value := range want {
		var got string
		if err := json.Unmarshal(first[key], &got); err != nil || got != value {
			t.Fatalf("%s = %s，要 %q", key, first[key], value)
		}
	}
	var secondVersion, secondCoverage string
	_ = json.Unmarshal(body.Verifications[1]["version"], &secondVersion)
	_ = json.Unmarshal(body.Verifications[1]["coverage"], &secondCoverage)
	if secondVersion != "digest-v2" || secondCoverage != "COVERED" {
		t.Fatalf("次版没有各自成行：%s", response.Body.String())
	}
}

// Covers: ADR-0022/ADR-0029 — 读不回是答案未形成（5xx），不伪装成空册。
func TestAFailingDutyVerificationReadIsNoAnswerRatherThanAnEmptyRegister(t *testing.T) {
	endpoint := customshttp.NewQueryDutyVerificationsEndpoint(
		grantedCatalogueIntake{tenant: "TENANT-1", limit: 25},
		&stubDutyVerifications{err: errors.New("connection refused")},
	)
	response := httptest.NewRecorder()
	endpoint.ServeHTTP(response, verificationRequest(http.MethodGet))

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusInternalServerError)
	}
	if got := problemCode(t, response); got != "NO_ANSWER_FORMED" {
		t.Fatalf("code = %q, want NO_ANSWER_FORMED", got)
	}
	assertNoOutcome(t, response)
}
