package commercialhttp_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	commercialhttp "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/http"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// 本文件对业务参与方修订历史端点（票 admin-web-group-legal-entities/12）证传输面：未配置 403、
// 多笔按读口交回的序逐字段转写（含名称）、不在册 200 + 空数组、跨租户不可见、方法门、路径缺参与方标识 400、
// 读失败 5xx。

// businessPartyRevisionReaderDouble 只对（租户，参与方）都对上的那一对交回行，其余一律空：
// 端点把作用域里的租户与路径里的参与方原样交给读口，这两格哪一格传错都会在这里显成空数组。
type businessPartyRevisionReaderDouble struct {
	tenant domain.TenantID
	party  string
	rows   []ports.BusinessPartyRevisionRow
	err    error
}

func (double *businessPartyRevisionReaderDouble) ListBusinessPartyRevisions(
	_ context.Context, tenant domain.TenantID, party domain.PartyID,
) ([]ports.BusinessPartyRevisionRow, error) {
	if double.err != nil {
		return nil, double.err
	}
	if tenant != double.tenant || party.String() != double.party {
		return []ports.BusinessPartyRevisionRow{}, nil
	}
	return double.rows, nil
}

var partyRevisionRegisteredAt = time.Date(2026, 9, 2, 8, 0, 0, 0, time.UTC)

func partyRevisionsRequest(party string) *http.Request {
	request := httptest.NewRequest(http.MethodGet, "/commercial-business-parties/"+party+"/revisions", nil)
	// 单测不经 chi 路由，路径参数由这里按路由会做的方式填进去；装配后的形状由 cmd/parcel-api 的
	// 隔离读用例经真路由再证一次。
	request.SetPathValue("partyId", party)
	return request
}

// 两笔之间名称换了一次：修订 2 既改名又停用，页面要能并排看出「从哪份换到哪份」。
func twoPartyRevisions() []ports.BusinessPartyRevisionRow {
	return []ports.BusinessPartyRevisionRow{
		{
			TenantID:      "tenant-1",
			PartyID:       "party-1",
			PartyName:     "参与方一",
			Revision:      1,
			Basis:         "basis-party-1",
			EffectiveFrom: partyRevisionRegisteredAt,
			RegisteredAt:  partyRevisionRegisteredAt,
		},
		{
			TenantID:          "tenant-1",
			PartyID:           "party-1",
			PartyName:         "参与方一（更名）",
			Revision:          2,
			Basis:             "basis-party-1-r2",
			EffectiveFrom:     partyRevisionRegisteredAt,
			DeactivatedAt:     partyRevisionRegisteredAt.Add(48 * time.Hour),
			DeactivationBasis: "basis-deact",
			HasDeactivation:   true,
			RegisteredAt:      partyRevisionRegisteredAt.Add(time.Hour),
		},
	}
}

// Covers: 票 12 完成判据「未配置 403」——Intake 未配置时不读读口、不带 outcome（ADR-0055）。
func TestBusinessPartyRevisionsEndpointRefusesWhenAccessChannelUnconfigured(t *testing.T) {
	reader := &businessPartyRevisionReaderDouble{err: errors.New("不该被调到")}
	endpoint := commercialhttp.NewQueryBusinessPartyRevisionsEndpoint(commercialhttp.UnconfiguredIntake{}, reader)

	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, partyRevisionsRequest("party-1"))
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body = %s", recorder.Code, recorder.Body)
	}
	var body map[string]json.RawMessage
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if _, has := body["outcome"]; has {
		t.Fatalf("未配置答复带了 outcome：%s", recorder.Body)
	}
	var problem struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &problem); err != nil || problem.Error.Code != "ACCESS_CHANNEL_NOT_CONFIGURED" {
		t.Fatalf("problem = %s (%v)", recorder.Body, err)
	}
}

// Covers: 票 12 完成判据「在册多笔按序」的传输半边——按读口交回的序逐笔转写，名称随每一笔在场，停用两件只在
// 停用那一笔在场（显式布尔判，不拿空串推），顶层回显参与方标识让页面核对答的是不是它问的那个。
func TestBusinessPartyRevisionsEndpointTranscribesRevisionsInOrder(t *testing.T) {
	reader := &businessPartyRevisionReaderDouble{
		tenant: catValue(t, domain.NewTenantID, "tenant-1"),
		party:  "party-1",
		rows:   twoPartyRevisions(),
	}
	endpoint := commercialhttp.NewQueryBusinessPartyRevisionsEndpoint(intakeDouble{query: catalogueQuery(t)}, reader)

	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, partyRevisionsRequest("party-1"))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body)
	}

	var body struct {
		Outcome   string `json:"outcome"`
		PartyID   string `json:"partyId"`
		Revisions []struct {
			TenantID          string `json:"tenantId"`
			PartyID           string `json:"partyId"`
			PartyName         string `json:"partyName"`
			Revision          int    `json:"revision"`
			Basis             string `json:"basis"`
			EffectiveFrom     string `json:"effectiveFrom"`
			DeactivatedAt     string `json:"deactivatedAt"`
			DeactivationBasis string `json:"deactivationBasis"`
			RegisteredAt      string `json:"registeredAt"`
		} `json:"revisions"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode %s: %v", recorder.Body.Bytes(), err)
	}
	if body.Outcome != "BUSINESS_PARTY_REVISIONS_LISTED" || body.PartyID != "party-1" || len(body.Revisions) != 2 {
		t.Fatalf("body = %+v", body)
	}
	first := body.Revisions[0]
	if first.Revision != 1 || first.TenantID != "tenant-1" || first.PartyID != "party-1" ||
		first.PartyName != "参与方一" || first.Basis != "basis-party-1" ||
		first.EffectiveFrom != partyRevisionRegisteredAt.Format(time.RFC3339Nano) ||
		first.RegisteredAt != partyRevisionRegisteredAt.Format(time.RFC3339Nano) ||
		first.DeactivatedAt != "" || first.DeactivationBasis != "" {
		t.Fatalf("修订 1 = %+v；未停用的笔不该带停用两件", first)
	}
	second := body.Revisions[1]
	if second.Revision != 2 || second.PartyName != "参与方一（更名）" || second.Basis != "basis-party-1-r2" ||
		second.DeactivationBasis != "basis-deact" ||
		second.DeactivatedAt != partyRevisionRegisteredAt.Add(48*time.Hour).Format(time.RFC3339Nano) ||
		second.RegisteredAt != partyRevisionRegisteredAt.Add(time.Hour).Format(time.RFC3339Nano) {
		t.Fatalf("修订 2 = %+v", second)
	}
	// 未停用那一笔的停用两键不得以空值在场：页面看键在不在，空串在场会被读成「停用于空时刻」。
	var raw struct {
		Revisions []map[string]json.RawMessage `json:"revisions"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &raw); err != nil {
		t.Fatalf("decode raw: %v", err)
	}
	for _, key := range []string{"deactivatedAt", "deactivationBasis"} {
		if _, has := raw.Revisions[0][key]; has {
			t.Fatalf("修订 1 带了 %s 键：%s", key, recorder.Body)
		}
	}
}

// Covers: 票 12 裁决「参与方不在册 → 200 + 空数组」（沿票 03 按 ADR-0022：能力在、册在、只是没有这一个身份，
// 续办是查册面不是查路由；判据同 writePartyRegistryAnswer 对`未找到`的处置）；空数组不得编成 null。
func TestBusinessPartyRevisionsEndpointAnswersUnknownPartyAsEmptyArray(t *testing.T) {
	reader := &businessPartyRevisionReaderDouble{
		tenant: catValue(t, domain.NewTenantID, "tenant-1"),
		party:  "party-1",
		rows:   twoPartyRevisions(),
	}
	endpoint := commercialhttp.NewQueryBusinessPartyRevisionsEndpoint(intakeDouble{query: catalogueQuery(t)}, reader)

	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, partyRevisionsRequest("party-none"))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", recorder.Code, recorder.Body)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(recorder.Body.Bytes(), &fields); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if string(fields["revisions"]) != "[]" {
		t.Fatalf("revisions = %s, want []", fields["revisions"])
	}
	if string(fields["outcome"]) != `"BUSINESS_PARTY_REVISIONS_LISTED"` || string(fields["partyId"]) != `"party-none"` {
		t.Fatalf("body = %s", recorder.Body)
	}
}

// Covers: 票 12 完成判据「跨租户不可见」——租户只从 Intake 交出的作用域取（ADR-0003），别家租户的
// 参与方在本租户作用域下与不在册同形（200 + 空数组），不泄露存在性。
func TestBusinessPartyRevisionsEndpointDoesNotSeeForeignTenant(t *testing.T) {
	reader := &businessPartyRevisionReaderDouble{
		tenant: catValue(t, domain.NewTenantID, "tenant-b"),
		party:  "party-1",
		rows:   twoPartyRevisions(),
	}
	endpoint := commercialhttp.NewQueryBusinessPartyRevisionsEndpoint(intakeDouble{query: catalogueQuery(t)}, reader)

	recorder := httptest.NewRecorder()
	request := partyRevisionsRequest("party-1")
	request.Header.Set("X-Reported-Tenant", "tenant-b")
	endpoint.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(recorder.Body.Bytes(), &fields); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if string(fields["revisions"]) != "[]" {
		t.Fatalf("跨租户 revisions = %s, want []", fields["revisions"])
	}
}

// Covers: 方法门由处理器自守（405 + Allow）；路径缺参与方标识是请求构造不出查询（400，重发不会好）；
// 读不回是答案未形成（5xx），不伪装成空历史。
func TestBusinessPartyRevisionsEndpointGuardsMethodPathAndReadFailure(t *testing.T) {
	healthy := commercialhttp.NewQueryBusinessPartyRevisionsEndpoint(
		intakeDouble{query: catalogueQuery(t)},
		&businessPartyRevisionReaderDouble{tenant: catValue(t, domain.NewTenantID, "tenant-1"), party: "party-1"})

	posted := httptest.NewRecorder()
	postRequest := httptest.NewRequest(http.MethodPost, "/commercial-business-parties/party-1/revisions", nil)
	postRequest.SetPathValue("partyId", "party-1")
	healthy.ServeHTTP(posted, postRequest)
	if posted.Code != http.StatusMethodNotAllowed || posted.Header().Get("Allow") != http.MethodGet {
		t.Fatalf("POST status = %d, allow = %q", posted.Code, posted.Header().Get("Allow"))
	}

	missing := httptest.NewRecorder()
	healthy.ServeHTTP(missing, httptest.NewRequest(http.MethodGet, "/commercial-business-parties//revisions", nil))
	if missing.Code != http.StatusBadRequest {
		t.Fatalf("缺参与方标识 status = %d, want 400; body = %s", missing.Code, missing.Body)
	}
	var problem struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(missing.Body.Bytes(), &problem); err != nil || problem.Error.Code != "MALFORMED_REQUEST" {
		t.Fatalf("缺参与方标识 problem = %s (%v)", missing.Body, err)
	}

	failing := commercialhttp.NewQueryBusinessPartyRevisionsEndpoint(
		intakeDouble{query: catalogueQuery(t)},
		&businessPartyRevisionReaderDouble{err: errors.New("库连不上")})
	failed := httptest.NewRecorder()
	failing.ServeHTTP(failed, partyRevisionsRequest("party-1"))
	if failed.Code != http.StatusInternalServerError {
		t.Fatalf("读失败 status = %d, want 500", failed.Code)
	}
	if err := json.Unmarshal(failed.Body.Bytes(), &problem); err != nil || problem.Error.Code != "NO_ANSWER_FORMED" {
		t.Fatalf("读失败 problem = %s (%v)", failed.Body, err)
	}
}
