package governancehttp_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	governancehttp "go.idp.xyz/idp-parcel/internal/pilotgovernance/adapters/http"
	"go.idp.xyz/idp-parcel/internal/pilotgovernance/ports"
)

// 本文件对治理登记册端点（票 admin-skeleton-closure-batch/02）证传输面：方法门、
// register 封闭三册缺席按坏请求拒且不触读口、未配置 Intake 对全部分支 403、行体
// 逐字段转写且开放区间上界如实缺席、空册答空数组、读失败答 5xx。

var registryEndpointAt = time.Date(2026, 8, 21, 12, 0, 0, 0, time.UTC)

type stubRegistryReader struct {
	intervals   []ports.AuthorityIntervalRegistryRow
	suspensions []ports.SuspensionRegistryRow
	resumptions []ports.ResumptionRegistryRow
	err         error

	gotLimit int
}

func (stub *stubRegistryReader) ListAuthorityIntervals(
	_ context.Context, limit int,
) ([]ports.AuthorityIntervalRegistryRow, error) {
	stub.gotLimit = limit
	return stub.intervals, stub.err
}

func (stub *stubRegistryReader) ListSuspensions(
	_ context.Context, limit int,
) ([]ports.SuspensionRegistryRow, error) {
	stub.gotLimit = limit
	return stub.suspensions, stub.err
}

func (stub *stubRegistryReader) ListResumptions(
	_ context.Context, limit int,
) ([]ports.ResumptionRegistryRow, error) {
	stub.gotLimit = limit
	return stub.resumptions, stub.err
}

// unreachableRegistryReader 断言读口未被触到：传输形状的拒绝与未配置格都发生在
// 读库之前。
type unreachableRegistryReader struct{ t *testing.T }

func (reader unreachableRegistryReader) ListAuthorityIntervals(
	_ context.Context, _ int,
) ([]ports.AuthorityIntervalRegistryRow, error) {
	reader.t.Fatal("读口不该被触到")
	return nil, nil
}

func (reader unreachableRegistryReader) ListSuspensions(
	_ context.Context, _ int,
) ([]ports.SuspensionRegistryRow, error) {
	reader.t.Fatal("读口不该被触到")
	return nil, nil
}

func (reader unreachableRegistryReader) ListResumptions(
	_ context.Context, _ int,
) ([]ports.ResumptionRegistryRow, error) {
	reader.t.Fatal("读口不该被触到")
	return nil, nil
}

func grantedIntake(t *testing.T, limit int) governancehttp.IsolatedOperationsReadIntake {
	t.Helper()
	intake, err := governancehttp.NewIsolatedOperationsReadIntake("SYN-SCOPE-ISOLATED-READ", limit)
	if err != nil {
		t.Fatalf("构造注入 Intake：%v", err)
	}
	return intake
}

func registryBody(t *testing.T, recorder *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("响应不是 JSON：%v\n%s", err, recorder.Body.String())
	}
	return body
}

func TestGovernanceRegistersEndpointRejectsNonGetAndUnknownRegister(t *testing.T) {
	endpoint := governancehttp.NewQueryGovernanceRegistersEndpoint(
		grantedIntake(t, 50), unreachableRegistryReader{t: t})

	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder,
		httptest.NewRequest(http.MethodPost, "/governance-registers?register=suspension", nil))
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST 答 %d, want 405", recorder.Code)
	}
	if allow := recorder.Header().Get("Allow"); allow != http.MethodGet {
		t.Fatalf("Allow = %q, want GET", allow)
	}

	// 阶段评审与接管第二批未开（票 12），不在封闭集内——今天问就是坏请求。
	for _, target := range []string{
		"/governance-registers",
		"/governance-registers?register=stage-review",
		"/governance-registers?register=takeover",
	} {
		recorder := httptest.NewRecorder()
		endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, target, nil))
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("%s 答 %d, want 400（缺席按坏请求拒：替调用方默认一册就是替它猜）",
				target, recorder.Code)
		}
	}
}

func TestGovernanceRegistersEndpointAnswersForbiddenWhenIntakeUnconfigured(t *testing.T) {
	endpoint := governancehttp.NewQueryGovernanceRegistersEndpoint(
		governancehttp.UnconfiguredIntake{}, unreachableRegistryReader{t: t})
	for _, register := range []string{"authority-interval", "suspension", "resumption"} {
		recorder := httptest.NewRecorder()
		endpoint.ServeHTTP(recorder,
			httptest.NewRequest(http.MethodGet, "/governance-registers?register="+register, nil))
		if recorder.Code != http.StatusForbidden {
			t.Fatalf("未配置 Intake 对 %s 答 %d, want 403", register, recorder.Code)
		}
		var body map[string]any
		if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
			t.Fatalf("403 响应不是 JSON：%v", err)
		}
		detail, _ := body["error"].(map[string]any)
		if detail["code"] != "ACCESS_CHANNEL_NOT_CONFIGURED" {
			t.Fatalf("错误码 = %v", detail["code"])
		}
	}
}

func TestGovernanceRegistersEndpointTranscribesAuthorityIntervals(t *testing.T) {
	reader := &stubRegistryReader{intervals: []ports.AuthorityIntervalRegistryRow{
		{
			ObjectScope: "SYN-SCOPE/routing-pilot",
			Capability:  "SYN-CAP/route-planning",
			FactKind:    "SYN-FACT/route-plan",
			Authority:   "SYN-AUTH/pilot-engine",
			FromAt:      registryEndpointAt,
			InsertedAt:  registryEndpointAt.Add(time.Minute),
		},
		{
			ObjectScope: "SYN-SCOPE/routing-pilot",
			Capability:  "SYN-CAP/route-planning",
			FactKind:    "SYN-FACT/route-plan",
			Authority:   "SYN-AUTH/legacy-engine",
			FromAt:      registryEndpointAt.Add(-30 * 24 * time.Hour),
			ToAt:        registryEndpointAt,
			HasToAt:     true,
			InsertedAt:  registryEndpointAt,
		},
	}}
	endpoint := governancehttp.NewQueryGovernanceRegistersEndpoint(grantedIntake(t, 25), reader)

	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder,
		httptest.NewRequest(http.MethodGet, "/governance-registers?register=authority-interval", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("上列答 %d, want 200\n%s", recorder.Code, recorder.Body.String())
	}
	if reader.gotLimit != 25 {
		t.Fatalf("读口收到 limit=%d, want 25（注入值）", reader.gotLimit)
	}
	body := registryBody(t, recorder)
	if body["outcome"] != "AUTHORITY_INTERVALS_LISTED" {
		t.Fatalf("outcome = %v", body["outcome"])
	}
	intervals, ok := body["intervals"].([]any)
	if !ok || len(intervals) != 2 {
		t.Fatalf("intervals 形状变形：%v", body["intervals"])
	}
	open, _ := intervals[0].(map[string]any)
	if open["objectScope"] != "SYN-SCOPE/routing-pilot" || open["capability"] != "SYN-CAP/route-planning" ||
		open["factKind"] != "SYN-FACT/route-plan" || open["authority"] != "SYN-AUTH/pilot-engine" {
		t.Fatalf("四维身份转写变形：%v", open)
	}
	if _, present := open["toAt"]; present {
		t.Fatalf("开放区间长出了上界：%v", open)
	}
	ended, _ := intervals[1].(map[string]any)
	if _, present := ended["toAt"].(string); !present {
		t.Fatalf("已闭区间的上界没透出：%v", ended)
	}
}

func TestGovernanceRegistersEndpointTranscribesSuspensionsAndResumptions(t *testing.T) {
	reader := &stubRegistryReader{
		suspensions: []ports.SuspensionRegistryRow{{
			SuspensionID:  "SYN-GOV-SUS-0001",
			TriggerSource: "SYN-TRIGGER/shadow-diff-alarm",
			Basis:         "SYN-BASIS/stage-review-no-go",
			Evidence:      "SYN-EVIDENCE/incident-260803",
			Scope:         "SYN-PILOT-SCOPE@v3",
			ExecutedBy:    "SYN-GOV-OPERATOR",
			OccurredAt:    registryEndpointAt,
			EffectiveAt:   registryEndpointAt.Add(time.Hour),
			InTransitNote: "SYN-NOTE/in-transit objects stay with current authority",
		}},
		resumptions: []ports.ResumptionRegistryRow{{
			SuspensionID:     "SYN-GOV-SUS-0001",
			ReleaseEvidence:  "SYN-EVIDENCE/release-260804",
			ConsistencyCheck: "SYN-CHECK/ledger-consistent",
			InventoryTakenAt: registryEndpointAt.Add(25 * time.Hour),
			DecidedBy:        "SYN-GOV-OPERATOR",
			DecidedAt:        registryEndpointAt.Add(26 * time.Hour),
			EffectiveAt:      registryEndpointAt.Add(27 * time.Hour),
		}},
	}
	endpoint := governancehttp.NewQueryGovernanceRegistersEndpoint(grantedIntake(t, 25), reader)

	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder,
		httptest.NewRequest(http.MethodGet, "/governance-registers?register=suspension", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("暂停册答 %d, want 200", recorder.Code)
	}
	body := registryBody(t, recorder)
	if body["outcome"] != "SUSPENSIONS_LISTED" {
		t.Fatalf("outcome = %v", body["outcome"])
	}
	suspensions, _ := body["suspensions"].([]any)
	if len(suspensions) != 1 {
		t.Fatalf("suspensions 形状变形：%v", body["suspensions"])
	}
	suspended, _ := suspensions[0].(map[string]any)
	if suspended["suspensionId"] != "SYN-GOV-SUS-0001" ||
		suspended["scope"] != "SYN-PILOT-SCOPE@v3" ||
		suspended["inTransitNote"] != "SYN-NOTE/in-transit objects stay with current authority" {
		t.Fatalf("暂停行转写变形：%v", suspended)
	}

	recorder = httptest.NewRecorder()
	endpoint.ServeHTTP(recorder,
		httptest.NewRequest(http.MethodGet, "/governance-registers?register=resumption", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("恢复册答 %d, want 200", recorder.Code)
	}
	body = registryBody(t, recorder)
	if body["outcome"] != "RESUMPTIONS_LISTED" {
		t.Fatalf("outcome = %v", body["outcome"])
	}
	resumptions, _ := body["resumptions"].([]any)
	if len(resumptions) != 1 {
		t.Fatalf("resumptions 形状变形：%v", body["resumptions"])
	}
	resumed, _ := resumptions[0].(map[string]any)
	if resumed["suspensionId"] != "SYN-GOV-SUS-0001" ||
		resumed["releaseEvidence"] != "SYN-EVIDENCE/release-260804" ||
		resumed["consistencyCheck"] != "SYN-CHECK/ledger-consistent" {
		t.Fatalf("恢复行转写变形：%v", resumed)
	}
	if _, present := resumed["inventoryTakenAt"].(string); !present {
		t.Fatalf("盘点时刻没透出：%v", resumed)
	}
	if _, present := resumed["inventory"]; present {
		t.Fatalf("盘点 jsonb 泄进了列面：%v", resumed)
	}
}

func TestGovernanceRegistersEndpointAnswersEmptyRegistryAsEmptyArray(t *testing.T) {
	endpoint := governancehttp.NewQueryGovernanceRegistersEndpoint(
		grantedIntake(t, 25), &stubRegistryReader{})
	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder,
		httptest.NewRequest(http.MethodGet, "/governance-registers?register=authority-interval", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("空册答 %d, want 200", recorder.Code)
	}
	intervals, ok := registryBody(t, recorder)["intervals"].([]any)
	if !ok {
		t.Fatal("空册没交回数组")
	}
	if len(intervals) != 0 {
		t.Fatalf("空册交回 %d 行", len(intervals))
	}
}

func TestGovernanceRegistersEndpointAnswersServerErrorWhenReadFails(t *testing.T) {
	endpoint := governancehttp.NewQueryGovernanceRegistersEndpoint(
		grantedIntake(t, 25), &stubRegistryReader{err: errors.New("寄了")})
	for _, register := range []string{"authority-interval", "suspension", "resumption"} {
		recorder := httptest.NewRecorder()
		endpoint.ServeHTTP(recorder,
			httptest.NewRequest(http.MethodGet, "/governance-registers?register="+register, nil))
		if recorder.Code != http.StatusInternalServerError {
			t.Fatalf("读失败对 %s 答 %d, want 500", register, recorder.Code)
		}
	}
}
