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
	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
)

// 本文件对参与方身份两个目录端点（票 admin-remainder-mechanism-batch/01）证传输面：
// 方法门、行体逐字段转写、可缺席字段（名称、停用两件、终止三件）如实缺席、
// 空目录答空数组、读失败答 5xx。

type partyIdentityReaderDouble struct {
	tenant        domain.TenantID
	parties       []ports.BusinessPartyRow
	entities      []ports.GroupLegalEntityRow
	relationships []ports.PartyRelationshipRow
	// facets 非空时随业务参与方与参与方关系两册的答复带出（ADR-0378 决定六点名的两册）。
	facets cataloguepage.Facets
	err    error
}

func (double *partyIdentityReaderDouble) ListBusinessParties(
	_ context.Context, tenant domain.TenantID, _ int, _ cataloguepage.Query,
) (cataloguepage.Result[ports.BusinessPartyRow], error) {
	page, err := tenantPage(double.err, tenant == double.tenant, double.parties)
	page.Facets = double.facets
	return page, err
}

func (double *partyIdentityReaderDouble) ListGroupLegalEntities(
	_ context.Context, tenant domain.TenantID, _ int, _ cataloguepage.Query,
) (cataloguepage.Result[ports.GroupLegalEntityRow], error) {
	return tenantPage(double.err, tenant == double.tenant, double.entities)
}

func (double *partyIdentityReaderDouble) ListPartyRelationships(
	_ context.Context, tenant domain.TenantID, _ int, _ cataloguepage.Query,
) (cataloguepage.Result[ports.PartyRelationshipRow], error) {
	page, err := tenantPage(double.err, tenant == double.tenant, double.relationships)
	page.Facets = double.facets
	return page, err
}

// 两册的修订历史读口随 PartyIdentityCatalogueReader 嵌入进来；本文件的用例不经它们，替身只补形状。
// 各自的替身与用例在 query_legal_entity_revisions_test.go 与 query_business_party_revisions_test.go。
func (double *partyIdentityReaderDouble) ListLegalEntityRevisions(
	context.Context, domain.TenantID, domain.LegalEntityReference,
) ([]ports.LegalEntityRevisionRow, error) {
	return nil, double.err
}

func (double *partyIdentityReaderDouble) ListBusinessPartyRevisions(
	context.Context, domain.TenantID, domain.PartyID,
) ([]ports.BusinessPartyRevisionRow, error) {
	return nil, double.err
}

var identityListedAt = time.Date(2026, 8, 28, 9, 0, 0, 0, time.UTC)

// Covers: 票 admin-remainder-mechanism-batch/01 的补格裁定——身份本体这一口要能把生命
// 周期三格都透出来，尤其是`已停用`：那类参与方既不在法人册也不在关系册，另两口一行都
// 不给，本口不给就等于管理台看不见它。停用两件只在已停用时在场，不拿空串兼表没停用。
func TestBusinessPartiesEndpointTranscribesTheLifecycleCells(t *testing.T) {
	reader := &partyIdentityReaderDouble{
		tenant: catValue(t, domain.NewTenantID, "tenant-1"),
		parties: []ports.BusinessPartyRow{
			{
				TenantID:      "tenant-1",
				PartyID:       "party-effective",
				PartyName:     "已生效参与方",
				Status:        "EFFECTIVE",
				Revision:      1,
				Basis:         "basis-effective",
				EffectiveFrom: identityListedAt,
				RegisteredAt:  identityListedAt,
			},
			{
				TenantID:          "tenant-1",
				PartyID:           "party-retired",
				PartyName:         "已停用参与方",
				Status:            "DEACTIVATED",
				Revision:          2,
				Basis:             "basis-retired",
				EffectiveFrom:     identityListedAt,
				DeactivatedAt:     identityListedAt.Add(time.Hour),
				DeactivationBasis: "basis-deact",
				HasDeactivation:   true,
				RegisteredAt:      identityListedAt,
			},
		},
	}
	endpoint := commercialhttp.NewQueryBusinessPartiesEndpoint(
		intakeDouble{query: catalogueQuery(t)}, reader)

	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/commercial-business-parties", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body)
	}

	var body struct {
		Outcome string `json:"outcome"`
		Parties []struct {
			PartyID           string `json:"partyId"`
			PartyName         string `json:"partyName"`
			Status            string `json:"status"`
			Revision          int    `json:"revision"`
			Basis             string `json:"basis"`
			DeactivatedAt     string `json:"deactivatedAt"`
			DeactivationBasis string `json:"deactivationBasis"`
		} `json:"parties"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode %s: %v", recorder.Body.Bytes(), err)
	}
	if body.Outcome != "BUSINESS_PARTIES_LISTED" || len(body.Parties) != 2 {
		t.Fatalf("body = %+v", body)
	}
	if got := body.Parties[0]; got.PartyID != "party-effective" || got.Status != "EFFECTIVE" ||
		got.PartyName != "已生效参与方" || got.DeactivatedAt != "" || got.DeactivationBasis != "" {
		t.Fatalf("已生效那一行 = %+v；未停用的行不该带停用两件", got)
	}
	if got := body.Parties[1]; got.Status != "DEACTIVATED" || got.Revision != 2 ||
		got.DeactivationBasis != "basis-deact" ||
		got.DeactivatedAt != identityListedAt.Add(time.Hour).Format(time.RFC3339Nano) {
		t.Fatalf("已停用那一行 = %+v", got)
	}
}

// Covers: ADR-0378 决定三、五——行上的类别与关系段数逐字段转写，类别一格总是数组（读面没给也不编成 null）；读端口
// 答了分面就原样带出 facets，没答就没有这一格（不认识它的调用方不受影响）。
func TestBusinessPartiesEndpointCarriesCategoriesCountsAndFacets(t *testing.T) {
	tenant := catValue(t, domain.NewTenantID, "tenant-1")
	reader := &partyIdentityReaderDouble{
		tenant: tenant,
		parties: []ports.BusinessPartyRow{
			{
				TenantID: "tenant-1", PartyID: "party-ours", PartyName: "我方公司", Status: "EFFECTIVE", Revision: 1,
				Basis: "basis-ours", EffectiveFrom: identityListedAt, RegisteredAt: identityListedAt,
				Categories: []string{"ACCOUNT_HOLDER", "OUR_COMPANY"},
				RelationshipCounts: ports.PartyRelationshipCounts{
					AsHolder:       ports.RelationshipTally{Live: 1, Candidate: 0},
					AsCounterparty: ports.RelationshipTally{Live: 6, Candidate: 3},
				},
			},
			{
				TenantID: "tenant-1", PartyID: "party-bare", PartyName: "没带类别的行", Status: "EFFECTIVE", Revision: 1,
				Basis: "basis-bare", EffectiveFrom: identityListedAt, RegisteredAt: identityListedAt,
			},
		},
		facets: cataloguepage.Facets{
			"status":     {{Value: "REGISTERED", Count: 0}, {Value: "EFFECTIVE", Count: 2}},
			"categories": {{Value: "ACCOUNT_HOLDER", Count: 1}, {Value: "OUR_COMPANY", Count: 1}},
		},
	}
	recorder := httptest.NewRecorder()
	commercialhttp.NewQueryBusinessPartiesEndpoint(intakeDouble{query: catalogueQuery(t)}, reader).
		ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/commercial-business-parties", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(recorder.Body.Bytes(), &fields); err != nil {
		t.Fatalf("decode %s: %v", recorder.Body.Bytes(), err)
	}
	var parties []map[string]json.RawMessage
	if err := json.Unmarshal(fields["parties"], &parties); err != nil || len(parties) != 2 {
		t.Fatalf("parties = %s (%v)", fields["parties"], err)
	}
	if got := string(parties[0]["categories"]); got != `["ACCOUNT_HOLDER","OUR_COMPANY"]` {
		t.Fatalf("categories = %s", got)
	}
	if got, want := string(parties[0]["relationshipCounts"]),
		`{"asHolder":{"live":1,"candidate":0},"asCounterparty":{"live":6,"candidate":3}}`; got != want {
		t.Fatalf("relationshipCounts = %s, want %s", got, want)
	}
	if got := string(parties[1]["categories"]); got != `[]` {
		t.Fatalf("读面没给类别的行 categories = %s, want []（数组，不是 null）", got)
	}
	if got, want := string(fields["facets"]),
		`{"categories":[{"value":"ACCOUNT_HOLDER","count":1},{"value":"OUR_COMPANY","count":1}],`+
			`"status":[{"value":"REGISTERED","count":0},{"value":"EFFECTIVE","count":2}]}`; got != want {
		t.Fatalf("facets = %s, want %s", got, want)
	}

	// 参与方关系那一册同样把 facets 带出来。
	relations := httptest.NewRecorder()
	commercialhttp.NewQueryPartyRelationshipsEndpoint(intakeDouble{query: catalogueQuery(t)}, reader).
		ServeHTTP(relations, httptest.NewRequest(http.MethodGet, "/commercial-party-relationships", nil))
	var relationFields map[string]json.RawMessage
	if err := json.Unmarshal(relations.Body.Bytes(), &relationFields); err != nil || relationFields["facets"] == nil {
		t.Fatalf("参与方关系的答复没有 facets：%s (%v)", relations.Body.Bytes(), err)
	}

	// 读端口没答分面：答复里没有 facets 这一格。集团法人那一册没点名分面，替身答了它也不往外带。
	for name, endpoint := range map[string]http.Handler{
		"业务参与方（没答分面）": commercialhttp.NewQueryBusinessPartiesEndpoint(
			intakeDouble{query: catalogueQuery(t)}, &partyIdentityReaderDouble{tenant: tenant}),
		"集团法人": commercialhttp.NewQueryGroupLegalEntitiesEndpoint(intakeDouble{query: catalogueQuery(t)}, reader),
	} {
		plain := httptest.NewRecorder()
		endpoint.ServeHTTP(plain, httptest.NewRequest(http.MethodGet, "/", nil))
		var plainFields map[string]json.RawMessage
		if err := json.Unmarshal(plain.Body.Bytes(), &plainFields); err != nil {
			t.Fatalf("%s decode %s: %v", name, plain.Body.Bytes(), err)
		}
		if _, present := plainFields["facets"]; present {
			t.Fatalf("%s 的答复带了 facets：%s", name, plain.Body.Bytes())
		}
	}
}

// Covers: 空册是答案不是错误（ADR-0077 Decision 四），读不回才是 5xx；非 GET 拒在方法门。
func TestBusinessPartiesEndpointAnswersEmptyAndFailureApart(t *testing.T) {
	empty := commercialhttp.NewQueryBusinessPartiesEndpoint(
		intakeDouble{query: catalogueQuery(t)},
		&partyIdentityReaderDouble{tenant: catValue(t, domain.NewTenantID, "tenant-1")})
	recorder := httptest.NewRecorder()
	empty.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/commercial-business-parties", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("空册 status = %d", recorder.Code)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(recorder.Body.Bytes(), &fields); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if string(fields["parties"]) != "[]" {
		t.Fatalf("parties = %s, want []", fields["parties"])
	}

	failing := commercialhttp.NewQueryBusinessPartiesEndpoint(
		intakeDouble{query: catalogueQuery(t)},
		&partyIdentityReaderDouble{err: errors.New("库连不上")})
	failed := httptest.NewRecorder()
	failing.ServeHTTP(failed, httptest.NewRequest(http.MethodGet, "/commercial-business-parties", nil))
	if failed.Code != http.StatusInternalServerError {
		t.Fatalf("读失败 status = %d, want 500", failed.Code)
	}

	posted := httptest.NewRecorder()
	empty.ServeHTTP(posted, httptest.NewRequest(http.MethodPost, "/commercial-business-parties", nil))
	if posted.Code != http.StatusMethodNotAllowed || posted.Header().Get("Allow") != http.MethodGet {
		t.Fatalf("POST status = %d, allow = %q", posted.Code, posted.Header().Get("Allow"))
	}
}

func TestGroupLegalEntitiesEndpointTranscribesRows(t *testing.T) {
	reader := &partyIdentityReaderDouble{
		tenant: catValue(t, domain.NewTenantID, "tenant-1"),
		entities: []ports.GroupLegalEntityRow{
			{
				TenantID:      "tenant-1",
				LegalEntityID: "le-1",
				PartyID:       "party-le",
				PartyName:     "运营法人参与方",
				HasPartyName:  true,
				Status:        "EFFECTIVE",
				Revision:      1,
				Basis:         "basis-le",
				EffectiveFrom: identityListedAt,
				RegisteredAt:  identityListedAt,
			},
			{
				TenantID:          "tenant-1",
				LegalEntityID:     "le-2",
				PartyID:           "party-gone",
				Status:            "DEACTIVATED",
				Revision:          2,
				Basis:             "basis-le2",
				EffectiveFrom:     identityListedAt,
				DeactivatedAt:     identityListedAt.Add(time.Hour),
				DeactivationBasis: "basis-deact",
				HasDeactivation:   true,
				RegisteredAt:      identityListedAt,
			},
		},
	}
	endpoint := commercialhttp.NewQueryGroupLegalEntitiesEndpoint(
		intakeDouble{query: catalogueQuery(t)}, reader)

	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/commercial-group-legal-entities", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body)
	}

	var body struct {
		Outcome  string `json:"outcome"`
		Entities []struct {
			LegalEntityID     string `json:"legalEntityId"`
			Kind              string `json:"kind"`
			PartyID           string `json:"partyId"`
			PartyName         string `json:"partyName"`
			PartyNameKnown    bool   `json:"partyNameKnown"`
			Status            string `json:"status"`
			Revision          int    `json:"revision"`
			DeactivatedAt     string `json:"deactivatedAt"`
			DeactivationBasis string `json:"deactivationBasis"`
		} `json:"entities"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Outcome != "GROUP_LEGAL_ENTITIES_LISTED" || len(body.Entities) != 2 {
		t.Fatalf("body = %+v", body)
	}
	live := body.Entities[0]
	if live.Kind != "RESPONSIBLE_LEGAL_ENTITY" || !live.PartyNameKnown ||
		live.PartyName != "运营法人参与方" || live.Status != "EFFECTIVE" || live.DeactivatedAt != "" {
		t.Fatalf("live entity = %+v", live)
	}
	// 悬空参与方：名称如实缺席（布尔 false 且无占位文本）；停用两件在场。
	gone := body.Entities[1]
	if gone.PartyNameKnown || gone.PartyName != "" || gone.Status != "DEACTIVATED" ||
		gone.DeactivatedAt == "" || gone.DeactivationBasis != "basis-deact" {
		t.Fatalf("deactivated entity = %+v", gone)
	}
}

func TestPartyRelationshipsEndpointTranscribesRows(t *testing.T) {
	reader := &partyIdentityReaderDouble{
		tenant: catValue(t, domain.NewTenantID, "tenant-1"),
		relationships: []ports.PartyRelationshipRow{
			{
				TenantID:            "tenant-1",
				RelationshipID:      "rel-1",
				Revision:            1,
				HolderID:            "party-cust",
				HolderName:          "货主客户参与方",
				HasHolderName:       true,
				CounterpartyID:      "party-le",
				CounterpartyName:    "运营法人参与方",
				HasCounterpartyName: true,
				Role:                "CUSTOMER",
				Scope:               "scope-1",
				Basis:               "contract-1",
				Status:              "EFFECTIVE",
				EffectiveStartsAt:   identityListedAt,
				RegisteredAt:        identityListedAt,
			},
			{
				TenantID:          "tenant-1",
				RelationshipID:    "rel-2",
				Revision:          1,
				HolderID:          "party-agent",
				CounterpartyID:    "party-le",
				Role:              "CARRIER_AGENT",
				Scope:             "scope-1",
				Basis:             "agreement-1",
				Status:            "CANDIDATE",
				EffectiveStartsAt: identityListedAt,
				RegisteredAt:      identityListedAt,
			},
		},
	}
	endpoint := commercialhttp.NewQueryPartyRelationshipsEndpoint(
		intakeDouble{query: catalogueQuery(t)}, reader)

	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/commercial-party-relationships", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body)
	}

	var body struct {
		Outcome       string `json:"outcome"`
		Relationships []struct {
			RelationshipID        string `json:"relationshipId"`
			HolderID              string `json:"holderId"`
			HolderName            string `json:"holderName"`
			HolderNameKnown       bool   `json:"holderNameKnown"`
			CounterpartyID        string `json:"counterpartyId"`
			CounterpartyNameKnown bool   `json:"counterpartyNameKnown"`
			Role                  string `json:"role"`
			Status                string `json:"status"`
			EffectiveEndsAt       string `json:"effectiveEndsAt"`
			EndedAt               string `json:"endedAt"`
		} `json:"relationships"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Outcome != "PARTY_RELATIONSHIPS_LISTED" || len(body.Relationships) != 2 {
		t.Fatalf("body = %+v", body)
	}
	effective := body.Relationships[0]
	if effective.Role != "CUSTOMER" || effective.Status != "EFFECTIVE" ||
		!effective.HolderNameKnown || effective.HolderName != "货主客户参与方" ||
		effective.EffectiveEndsAt != "" || effective.EndedAt != "" {
		t.Fatalf("effective row = %+v", effective)
	}
	candidate := body.Relationships[1]
	if candidate.Status != "CANDIDATE" || candidate.HolderNameKnown || candidate.CounterpartyNameKnown {
		t.Fatalf("candidate row = %+v", candidate)
	}
}

func TestPartyIdentityEndpointsAnswerEmptyCataloguesAsContent(t *testing.T) {
	reader := &partyIdentityReaderDouble{tenant: catValue(t, domain.NewTenantID, "tenant-1")}

	for _, endpoint := range []http.Handler{
		commercialhttp.NewQueryGroupLegalEntitiesEndpoint(intakeDouble{query: catalogueQuery(t)}, reader),
		commercialhttp.NewQueryPartyRelationshipsEndpoint(intakeDouble{query: catalogueQuery(t)}, reader),
	} {
		recorder := httptest.NewRecorder()
		endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
		if recorder.Code != http.StatusOK {
			t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body)
		}
		var body map[string]json.RawMessage
		if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		for _, field := range []string{"entities", "relationships"} {
			if raw, has := body[field]; has && string(raw) == "null" {
				t.Fatalf("%s = null，空册要答空数组", field)
			}
		}
	}
}

func TestPartyIdentityEndpointsGuardMethodAndReadFailures(t *testing.T) {
	failing := &partyIdentityReaderDouble{err: errors.New("db down")}

	entities := commercialhttp.NewQueryGroupLegalEntitiesEndpoint(
		intakeDouble{query: catalogueQuery(t)}, failing)
	recorder := httptest.NewRecorder()
	entities.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/", nil))
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST status = %d", recorder.Code)
	}

	recorder = httptest.NewRecorder()
	entities.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("read failure status = %d（读不回不是空册）", recorder.Code)
	}

	relationships := commercialhttp.NewQueryPartyRelationshipsEndpoint(
		intakeDouble{query: catalogueQuery(t)}, failing)
	recorder = httptest.NewRecorder()
	relationships.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("read failure status = %d", recorder.Code)
	}
}
