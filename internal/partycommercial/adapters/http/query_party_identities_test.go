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

// 本文件对参与方身份两个目录端点（票 admin-remainder-mechanism-batch/01）证传输面：
// 方法门、行体逐字段转写、可缺席字段（名称、停用两件、终止三件）如实缺席、
// 空目录答空数组、读失败答 5xx。

type partyIdentityReaderDouble struct {
	tenant        domain.TenantID
	entities      []ports.GroupLegalEntityRow
	relationships []ports.PartyRelationshipRow
	err           error
}

func (double *partyIdentityReaderDouble) ListGroupLegalEntities(
	_ context.Context, tenant domain.TenantID, _ int,
) ([]ports.GroupLegalEntityRow, error) {
	if double.err != nil {
		return nil, double.err
	}
	if tenant != double.tenant {
		return nil, nil
	}
	return double.entities, nil
}

func (double *partyIdentityReaderDouble) ListPartyRelationships(
	_ context.Context, tenant domain.TenantID, _ int,
) ([]ports.PartyRelationshipRow, error) {
	if double.err != nil {
		return nil, double.err
	}
	if tenant != double.tenant {
		return nil, nil
	}
	return double.relationships, nil
}

var identityListedAt = time.Date(2026, 8, 28, 9, 0, 0, 0, time.UTC)

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
