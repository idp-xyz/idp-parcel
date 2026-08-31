package visibilityhttp_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	visibilityhttp "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/http"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

// triageReaderDouble 按注入行作答，并记录收到的租户与页大小——端点必须把 Intake
// 裁决的作用域与 limit 原样递给读口，不采信请求里的任何自报。
type triageReaderDouble struct {
	tenant domain.TenantID
	limit  int

	episodes []ports.SignalEpisodeCatalogueRow
	requests []ports.DispositionRequestCatalogueRow
	err      error
}

func (double *triageReaderDouble) record(tenant domain.TenantID, limit int) error {
	double.tenant = tenant
	double.limit = limit
	return double.err
}

func (double *triageReaderDouble) ListSignalEpisodes(
	_ context.Context, tenant domain.TenantID, limit int,
) ([]ports.SignalEpisodeCatalogueRow, error) {
	if err := double.record(tenant, limit); err != nil {
		return nil, err
	}
	return double.episodes, nil
}

func (double *triageReaderDouble) ListDispositionRequests(
	_ context.Context, tenant domain.TenantID, limit int,
) ([]ports.DispositionRequestCatalogueRow, error) {
	if err := double.record(tenant, limit); err != nil {
		return nil, err
	}
	return double.requests, nil
}

// unreachableTriageReader 是「被调即失败」的替身：未配置 Intake 的合同就是不构造
// 查询，读口若被触到，说明有请求穿过了未配置格。
type unreachableTriageReader struct{ t *testing.T }

func (reader unreachableTriageReader) ListSignalEpisodes(
	context.Context, domain.TenantID, int,
) ([]ports.SignalEpisodeCatalogueRow, error) {
	reader.t.Error("a request passed the unconfigured intake and reached the triage reader")
	return nil, nil
}

func (reader unreachableTriageReader) ListDispositionRequests(
	context.Context, domain.TenantID, int,
) ([]ports.DispositionRequestCatalogueRow, error) {
	reader.t.Error("a request passed the unconfigured intake and reached the triage reader")
	return nil, nil
}

var triageRegistries = []string{"signal-episode", "disposition-request"}

func etServe(
	t *testing.T,
	intake visibilityhttp.OperationsTrackingIntake,
	reader visibilityhttp.TriageReviewReader,
	method, target string,
) *httptest.ResponseRecorder {
	t.Helper()
	endpoint := visibilityhttp.NewQueryExceptionTriageRecordsEndpoint(intake, reader)
	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, httptest.NewRequest(method, target, nil))
	return recorder
}

// Covers: registry 是传输形状——缺席或集外按坏请求拒（两本册子行形状不同，替调用方
// 选就是猜）；方法检查同级先行。
func TestExceptionTriageRecordsRefuseWrongMethodAndForeignRegistry(t *testing.T) {
	intake := operationsIntakeDouble{query: operationsScope(t)}

	wrongMethod := etServe(t, intake, &triageReaderDouble{},
		http.MethodPost, "/exception-triage-records?registry=signal-episode")
	if wrongMethod.Code != http.StatusMethodNotAllowed ||
		wrongMethod.Header().Get("Allow") != http.MethodGet {
		t.Fatalf("status = %d allow = %q, want 405/GET",
			wrongMethod.Code, wrongMethod.Header().Get("Allow"))
	}

	for name, target := range map[string]string{
		"registry 缺席": "/exception-triage-records",
		"registry 集外": "/exception-triage-records?registry=exception-case",
	} {
		response := etServe(t, intake, &triageReaderDouble{}, http.MethodGet, target)
		if response.Code != http.StatusBadRequest ||
			problemCode(t, response) != "MALFORMED_REQUEST" {
			t.Fatalf("%s：status = %d body = %s，want 400 MALFORMED_REQUEST",
				name, response.Code, response.Body.String())
		}
	}
}

// Covers: 完成标准「未启用隔离读准入时答 403」——未配置 Intake 对两本册子同答
// 403 + ACCESS_CHANNEL_NOT_CONFIGURED，读口不被触到，4xx 不带 outcome。
func TestExceptionTriageRecordsUnconfiguredIntakeRefusesEveryRegistry(t *testing.T) {
	for _, registry := range triageRegistries {
		response := etServe(t, visibilityhttp.UnconfiguredIntake{},
			unreachableTriageReader{t: t},
			http.MethodGet, "/exception-triage-records?registry="+registry)
		if response.Code != http.StatusForbidden ||
			problemCode(t, response) != "ACCESS_CHANNEL_NOT_CONFIGURED" {
			t.Fatalf("registry=%s：status = %d body = %s，want 403 ACCESS_CHANNEL_NOT_CONFIGURED",
				registry, response.Code, response.Body.String())
		}
		assertNoOutcome(t, response)
	}
}

// Covers: 发作期册 200 + 行体逐字段透出——已分诊行带成对 triage 三件与 ended 两件，
// 活跃未分诊行两组键都缺席（缺席即答案，不代填）；作用域与页大小来自 Intake 裁决。
func TestExceptionTriageRecordsListSignalEpisodesVerbatim(t *testing.T) {
	intake := operationsIntakeDouble{query: operationsScope(t)}
	endedAt := operationsBaseAt.Add(2 * time.Hour)
	triagedAt := operationsBaseAt.Add(30 * time.Minute)
	reader := &triageReaderDouble{
		episodes: []ports.SignalEpisodeCatalogueRow{
			{
				EpisodeID:    "episode-1",
				Parcel:       "parcel-1",
				Kind:         "DELIVERY_FAILED",
				Rule:         "triage/v1",
				Confidence:   "CARRIER_CONFIRMED",
				Hits:         3,
				StartedAt:    operationsBaseAt,
				LastHitAt:    operationsBaseAt.Add(time.Hour),
				ReleaseBasis: "release/1",
				EndedAt:      &endedAt,
				PriorEpisode: "episode-0",
				Outcome:      "AUTO_ESTABLISH",
				OutcomeRule:  "triage/v1",
				TriagedAt:    &triagedAt,
			},
			{
				EpisodeID:  "episode-2",
				Parcel:     "parcel-2",
				Kind:       "STALLED",
				Rule:       "triage/v1",
				Confidence: "SINGLE_SOURCE",
				Hits:       1,
				StartedAt:  operationsBaseAt,
				LastHitAt:  operationsBaseAt,
			},
		},
	}

	response := etServe(t, intake, reader,
		http.MethodGet, "/exception-triage-records?registry=signal-episode")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
	}
	var body struct {
		Outcome  string `json:"outcome"`
		Episodes []struct {
			EpisodeID    string `json:"episodeId"`
			Parcel       string `json:"parcel"`
			Kind         string `json:"kind"`
			Confidence   string `json:"confidence"`
			Hits         int64  `json:"hits"`
			StartedAt    string `json:"startedAt"`
			LastHitAt    string `json:"lastHitAt"`
			ReleaseBasis string `json:"releaseBasis"`
			EndedAt      any    `json:"endedAt"`
			PriorEpisode string `json:"priorEpisode"`
			Triage       *struct {
				Outcome   string `json:"outcome"`
				Rule      string `json:"rule"`
				TriagedAt string `json:"triagedAt"`
			} `json:"triage"`
		} `json:"episodes"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Outcome != "SIGNAL_EPISODES_LISTED" || len(body.Episodes) != 2 {
		t.Fatalf("outcome = %q episodes = %d", body.Outcome, len(body.Episodes))
	}
	concluded := body.Episodes[0]
	if concluded.EpisodeID != "episode-1" || concluded.Kind != "DELIVERY_FAILED" ||
		concluded.Confidence != "CARRIER_CONFIRMED" || concluded.Hits != 3 ||
		concluded.StartedAt != operationsBaseAt.Format(time.RFC3339Nano) ||
		concluded.ReleaseBasis != "release/1" || concluded.PriorEpisode != "episode-0" ||
		concluded.EndedAt != endedAt.Format(time.RFC3339Nano) {
		t.Fatalf("已分诊行走样：%+v", concluded)
	}
	if concluded.Triage == nil || concluded.Triage.Outcome != "AUTO_ESTABLISH" ||
		concluded.Triage.Rule != "triage/v1" ||
		concluded.Triage.TriagedAt != triagedAt.Format(time.RFC3339Nano) {
		t.Fatalf("分诊结论三件应成对在场：%+v", concluded.Triage)
	}
	active := body.Episodes[1]
	// 活跃未分诊行：ended 与 triage 缺席即答案——「进行中」不由读面代判成任何结论。
	if active.EpisodeID != "episode-2" || active.EndedAt != nil || active.Triage != nil {
		t.Fatalf("活跃行不得带 ended/triage：%+v", active)
	}
	if reader.tenant.String() != "tenant-1" || reader.limit != 50 {
		t.Fatalf("读口收到 tenant=%q limit=%d，应来自 Intake 裁决（tenant-1/50）",
			reader.tenant, reader.limit)
	}

	empty := etServe(t, intake, &triageReaderDouble{},
		http.MethodGet, "/exception-triage-records?registry=signal-episode")
	var emptyBody struct {
		Episodes []json.RawMessage `json:"episodes"`
	}
	if err := json.Unmarshal(empty.Body.Bytes(), &emptyBody); err != nil {
		t.Fatalf("decode empty: %v", err)
	}
	if empty.Code != http.StatusOK || emptyBody.Episodes == nil {
		t.Fatalf("空册 = %d %s；want 200 + 空数组", empty.Code, empty.Body.String())
	}
}

// Covers: 处置请求册 200 + 行体逐字段透出——已判断行带 judgment 两件、取消与替代
// 指针；未判断行三组键缺席（尚无答复不是拒绝）。
func TestExceptionTriageRecordsListDispositionRequestsVerbatim(t *testing.T) {
	intake := operationsIntakeDouble{query: operationsScope(t)}
	window := operationsBaseAt.Add(24 * time.Hour)
	judgedAt := operationsBaseAt.Add(time.Hour)
	reader := &triageReaderDouble{
		requests: []ports.DispositionRequestCatalogueRow{
			{
				RequestID:        "request-1",
				CaseID:           "case-1",
				TargetContext:    "transport-fulfillment",
				Action:           "HOLD",
				Scope:            "parcel-1",
				Reason:           "damage suspected",
				Evidence:         "evidence/1",
				IntentVersion:    2,
				SentAt:           operationsBaseAt,
				AcceptanceWindow: &window,
				Judgment:         "PARTIALLY_ACCEPTED",
				JudgedAt:         &judgedAt,
				Cancellation:     "CANCELLED_CONFIRMED",
				SupersededBy:     "request-2",
			},
			{
				RequestID:     "request-3",
				CaseID:        "case-2",
				TargetContext: "node-operations",
				Action:        "REINSPECT",
				Scope:         "parcel-2",
				Reason:        "weight mismatch",
				Evidence:      "evidence/2",
				IntentVersion: 1,
				SentAt:        operationsBaseAt,
			},
		},
	}

	response := etServe(t, intake, reader,
		http.MethodGet, "/exception-triage-records?registry=disposition-request")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
	}
	var body struct {
		Outcome  string `json:"outcome"`
		Requests []struct {
			RequestID        string `json:"requestId"`
			CaseID           string `json:"caseId"`
			TargetContext    string `json:"targetContext"`
			Action           string `json:"action"`
			IntentVersion    int64  `json:"intentVersion"`
			SentAt           string `json:"sentAt"`
			AcceptanceWindow any    `json:"acceptanceWindow"`
			Judgment         any    `json:"judgment"`
			JudgedAt         any    `json:"judgedAt"`
			Cancellation     any    `json:"cancellation"`
			SupersededBy     any    `json:"supersededBy"`
		} `json:"requests"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Outcome != "DISPOSITION_REQUESTS_LISTED" || len(body.Requests) != 2 {
		t.Fatalf("outcome = %q requests = %d", body.Outcome, len(body.Requests))
	}
	judged := body.Requests[0]
	if judged.RequestID != "request-1" || judged.TargetContext != "transport-fulfillment" ||
		judged.Action != "HOLD" || judged.IntentVersion != 2 ||
		judged.SentAt != operationsBaseAt.Format(time.RFC3339Nano) ||
		judged.AcceptanceWindow != window.Format(time.RFC3339Nano) ||
		judged.Judgment != "PARTIALLY_ACCEPTED" ||
		judged.JudgedAt != judgedAt.Format(time.RFC3339Nano) ||
		judged.Cancellation != "CANCELLED_CONFIRMED" || judged.SupersededBy != "request-2" {
		t.Fatalf("已判断行走样：%+v", judged)
	}
	pending := body.Requests[1]
	// 尚无答复的请求：judgment/cancellation/supersededBy 全缺席——部分接受也是四走向
	// 之一，读面不把「还没答」演成任何一种。
	if pending.RequestID != "request-3" || pending.Judgment != nil ||
		pending.JudgedAt != nil || pending.Cancellation != nil ||
		pending.SupersededBy != nil || pending.AcceptanceWindow != nil {
		t.Fatalf("未判断行不得带答复键：%+v", pending)
	}
}

// Covers: 读不回是答案未形成（500 + NO_ANSWER_FORMED），不伪装成空册。
func TestExceptionTriageRecordsReaderFailureFormsNoAnswer(t *testing.T) {
	intake := operationsIntakeDouble{query: operationsScope(t)}
	for _, registry := range triageRegistries {
		response := etServe(t, intake,
			&triageReaderDouble{err: context.DeadlineExceeded},
			http.MethodGet, "/exception-triage-records?registry="+registry)
		if response.Code != http.StatusInternalServerError ||
			problemCode(t, response) != "NO_ANSWER_FORMED" {
			t.Fatalf("registry=%s：status = %d body = %s，want 500 NO_ANSWER_FORMED",
				registry, response.Code, response.Body.String())
		}
		assertNoOutcome(t, response)
	}
}
