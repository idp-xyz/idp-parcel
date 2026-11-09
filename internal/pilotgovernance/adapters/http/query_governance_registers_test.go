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
	"go.idp.xyz/idp-parcel/internal/pilotgovernance/domain"
	"go.idp.xyz/idp-parcel/internal/pilotgovernance/ports"
	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
)

// 本文件对治理登记册端点（票 admin-skeleton-closure-batch/02，接管册 ADR-0155）证
// 传输面：方法门、配置了的渠道上 register 缺席或不在封闭四册内按坏请求拒且不触读口、
// 未配置 Intake 对一切请求 403（register 写错也一样）、行体逐字段转写且开放区间上界如实
// 缺席、接管盘点 jsonb 不上列、空册答空数组、读失败答 5xx。阶段评审查阅仍不在封闭集内。

var registryEndpointAt = time.Date(2026, 8, 21, 12, 0, 0, 0, time.UTC)

type stubRegistryReader struct {
	intervals   []ports.AuthorityIntervalRegistryRow
	suspensions []ports.SuspensionRegistryRow
	resumptions []ports.ResumptionRegistryRow
	takeovers   []ports.TakeoverRegistryRow
	next        string
	total       int64
	err         error

	gotLimit int
	gotQuery cataloguepage.Query
}

func stubPage[Row any](stub *stubRegistryReader, limit int, query cataloguepage.Query, rows []Row,
) (cataloguepage.Result[Row], error) {
	stub.gotLimit, stub.gotQuery = limit, query
	return cataloguepage.Result[Row]{Rows: rows, Next: stub.next, Total: stub.total}, stub.err
}

func (stub *stubRegistryReader) ListAuthorityIntervals(
	_ context.Context, limit int, query cataloguepage.Query,
) (cataloguepage.Result[ports.AuthorityIntervalRegistryRow], error) {
	return stubPage(stub, limit, query, stub.intervals)
}

func (stub *stubRegistryReader) ListSuspensions(
	_ context.Context, limit int, query cataloguepage.Query,
) (cataloguepage.Result[ports.SuspensionRegistryRow], error) {
	return stubPage(stub, limit, query, stub.suspensions)
}

func (stub *stubRegistryReader) ListResumptions(
	_ context.Context, limit int, query cataloguepage.Query,
) (cataloguepage.Result[ports.ResumptionRegistryRow], error) {
	return stubPage(stub, limit, query, stub.resumptions)
}

func (stub *stubRegistryReader) ListTakeovers(
	_ context.Context, limit int, query cataloguepage.Query,
) (cataloguepage.Result[ports.TakeoverRegistryRow], error) {
	return stubPage(stub, limit, query, stub.takeovers)
}

// unreachableRegistryReader 断言读口未被触到：传输形状的拒绝与未配置格都发生在
// 读库之前。
type unreachableRegistryReader struct{ t *testing.T }

func (reader unreachableRegistryReader) ListAuthorityIntervals(
	_ context.Context, _ int, _ cataloguepage.Query,
) (cataloguepage.Result[ports.AuthorityIntervalRegistryRow], error) {
	reader.t.Fatal("读口不该被触到")
	return cataloguepage.Result[ports.AuthorityIntervalRegistryRow]{}, nil
}

func (reader unreachableRegistryReader) ListSuspensions(
	_ context.Context, _ int, _ cataloguepage.Query,
) (cataloguepage.Result[ports.SuspensionRegistryRow], error) {
	reader.t.Fatal("读口不该被触到")
	return cataloguepage.Result[ports.SuspensionRegistryRow]{}, nil
}

func (reader unreachableRegistryReader) ListResumptions(
	_ context.Context, _ int, _ cataloguepage.Query,
) (cataloguepage.Result[ports.ResumptionRegistryRow], error) {
	reader.t.Fatal("读口不该被触到")
	return cataloguepage.Result[ports.ResumptionRegistryRow]{}, nil
}

func (reader unreachableRegistryReader) ListTakeovers(
	_ context.Context, _ int, _ cataloguepage.Query,
) (cataloguepage.Result[ports.TakeoverRegistryRow], error) {
	reader.t.Fatal("读口不该被触到")
	return cataloguepage.Result[ports.TakeoverRegistryRow]{}, nil
}

// grantedRegistryIntake 是已认证、授予齐备的查阅出示：交回给定页大小的作用域，只供本包用例。
type grantedRegistryIntake struct{ limit int }

func (intake grantedRegistryIntake) IntakeRegistryQuery(context.Context, *http.Request) (governancehttp.RegistryQuery, error) {
	reference, err := domain.NewOperationsScopeReference("SYN-SCOPE-OPERATOR-READ")
	if err != nil {
		return governancehttp.RegistryQuery{}, err
	}
	scope, err := domain.NewOperationsQueryScope(reference)
	if err != nil {
		return governancehttp.RegistryQuery{}, err
	}
	return governancehttp.RegistryQuery{Scope: scope, Limit: intake.limit}, nil
}

func grantedIntake(t *testing.T, limit int) grantedRegistryIntake {
	t.Helper()
	return grantedRegistryIntake{limit: limit}
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

	// 阶段评审查阅仍未开，不在封闭集内——今天问就是坏请求。缺席参数与空值同拒。
	for _, target := range []string{
		"/governance-registers",
		"/governance-registers?register=",
		"/governance-registers?register=stage-review",
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
	var answered string
	for _, register := range []string{"authority-interval", "suspension", "resumption", "takeover"} {
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
		answered = recorder.Body.String()
	}

	// register 也在 Intake 之后判：缺席、空值或写错时，未配置的渠道与写对的一问同答，答复不随请求内容变（ADR-0055 决定三）。
	for _, target := range []string{
		"/governance-registers",
		"/governance-registers?register=",
		"/governance-registers?register=stage-review",
	} {
		recorder := httptest.NewRecorder()
		endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, target, nil))
		if recorder.Code != http.StatusForbidden || recorder.Body.String() != answered {
			t.Fatalf("未配置 Intake 对 %s 答 %d %s，want 与写对的一问同答 403 %s", target, recorder.Code, recorder.Body, answered)
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
	// 状态维那一格 open 恒在场，与 toAt 在场与否同说一件事。
	if open["open"] != true || ended["open"] != false {
		t.Fatalf("open 要恒在场、开放区间为 true、已有终点的为 false：%v / %v", open["open"], ended["open"])
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
			Resumed:       true,
		}, {
			SuspensionID:  "SYN-GOV-SUS-0002",
			TriggerSource: "SYN-TRIGGER/shadow-diff-alarm",
			Basis:         "SYN-BASIS/stage-review-no-go",
			Evidence:      "SYN-EVIDENCE/incident-260805",
			Scope:         "SYN-PILOT-SCOPE@v3",
			ExecutedBy:    "SYN-GOV-OPERATOR",
			OccurredAt:    registryEndpointAt.Add(48 * time.Hour),
			EffectiveAt:   registryEndpointAt.Add(49 * time.Hour),
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
	if len(suspensions) != 2 {
		t.Fatalf("suspensions 形状变形：%v", body["suspensions"])
	}
	suspended, _ := suspensions[0].(map[string]any)
	if suspended["suspensionId"] != "SYN-GOV-SUS-0001" ||
		suspended["scope"] != "SYN-PILOT-SCOPE@v3" ||
		suspended["inTransitNote"] != "SYN-NOTE/in-transit objects stay with current authority" {
		t.Fatalf("暂停行转写变形：%v", suspended)
	}
	// 状态维那一格 resumed 恒在场：有恢复决定的为 true，没有的为 false 而不是缺席。
	if still, _ := suspensions[1].(map[string]any); suspended["resumed"] != true || still["resumed"] != false {
		t.Fatalf("resumed 要恒在场、照读口转写：%v / %v", suspended["resumed"], still["resumed"])
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

func TestGovernanceRegistersEndpointTranscribesTakeoversWithoutInventory(t *testing.T) {
	reader := &stubRegistryReader{
		takeovers: []ports.TakeoverRegistryRow{{
			ObjectScope:      "SYN-SCOPE/lane",
			Capability:       "shipment-intake",
			FactKind:         "acceptance-decision",
			Authority:        "SYN-AUTH/successor",
			FromAt:           registryEndpointAt,
			StopEvidence:     "SYN-EVIDENCE/writer-stopped",
			AcceptedFacts:    "SYN-FACTS/cutover",
			PendingExternals: "SYN-EXT/none",
			ActualControl:    "SYN-CTRL/ops-desk",
			Responsibilities: "SYN-DUTY/ops-owner",
			NextAction:       "SYN-NEXT/hold",
			InventoryTakenAt: registryEndpointAt.Add(time.Hour),
			EffectiveAt:      registryEndpointAt.Add(2 * time.Hour),
		}},
	}
	endpoint := governancehttp.NewQueryGovernanceRegistersEndpoint(grantedIntake(t, 25), reader)
	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder,
		httptest.NewRequest(http.MethodGet, "/governance-registers?register=takeover", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("接管册答 %d, want 200\n%s", recorder.Code, recorder.Body.String())
	}
	body := registryBody(t, recorder)
	if body["outcome"] != "TAKEOVERS_LISTED" {
		t.Fatalf("outcome = %v", body["outcome"])
	}
	takeovers, _ := body["takeovers"].([]any)
	if len(takeovers) != 1 {
		t.Fatalf("takeovers 形状变形：%v", body["takeovers"])
	}
	row, _ := takeovers[0].(map[string]any)
	if row["objectScope"] != "SYN-SCOPE/lane" ||
		row["stopEvidence"] != "SYN-EVIDENCE/writer-stopped" ||
		row["nextAction"] != "SYN-NEXT/hold" {
		t.Fatalf("接管行转写变形：%v", row)
	}
	if _, present := row["toAt"]; present {
		t.Fatalf("开放区间不该带上界：%v", row)
	}
	if _, present := row["inventory"]; present {
		t.Fatalf("盘点 jsonb 泄进了列面：%v", row)
	}
	if _, present := row["inventoryTakenAt"].(string); !present {
		t.Fatalf("盘点时刻没透出：%v", row)
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
	for _, register := range []string{"authority-interval", "suspension", "resumption", "takeover"} {
		recorder := httptest.NewRecorder()
		endpoint.ServeHTTP(recorder,
			httptest.NewRequest(http.MethodGet, "/governance-registers?register="+register, nil))
		if recorder.Code != http.StatusInternalServerError {
			t.Fatalf("读失败对 %s 答 %d, want 500", register, recorder.Code)
		}
	}
}
