package commercialhttp_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	commercialhttp "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/http"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// 往来单位识别信息（ADR-0367）的传输层用例：参与方登记口把四格译进命令，形状不对即 MALFORMED_REQUEST、缺不缺与查重交给
// 用例；目录与修订历史两个读口带出识别信息，没登识别信息的行只带显式的 identityLayerRegistered:false、不带外部编码键。

func TestOperatorRegistryIntakeTranslatesBusinessPartyIdentification(t *testing.T) {
	intake := operatorIntake(t)
	command, err := intake.IntakeBusinessPartyRegistration(context.Background(),
		identityRequest("/commercial-business-party-registrations", `{"businessParties":[{
		"partyId":"SYN-PARTY-02","name":"SYN 合成参与方二号","revision":2,"basis":"SYN-BASIS/party-02-r2",
		"effectiveFrom":"2026-09-16T00:00:00Z",
		"registrationCountry":"CN",
		"lifetimeRegistrationNumbers":[{"typeCode":"SYN-XA-LIFETIME","number":"SYN-XA-000009"}],
		"identityCorrectionBasis":"SYN-CORRECTION-01",
		"externalReferences":[{"system":"SYN-ERP","purpose":"SUPPLIER","code":"V-0009"}]
	}]}`))
	if err != nil {
		t.Fatalf("intake：%v", err)
	}
	if command.RegistrationCountry == nil || command.RegistrationCountry.String() != "CN" ||
		len(command.LifetimeNumbers) != 1 || command.LifetimeNumbers[0].Number().String() != "SYN-XA-000009" ||
		command.IdentityCorrectionBasis == nil || command.IdentityCorrectionBasis.String() != "SYN-CORRECTION-01" {
		t.Fatalf("identity fields = %+v %+v %+v", command.RegistrationCountry, command.LifetimeNumbers, command.IdentityCorrectionBasis)
	}
	if len(command.ExternalReferences) != 1 || command.ExternalReferences[0].Purpose() != domain.ExternalSupplierCode ||
		command.ExternalReferences[0].Code() != "V-0009" {
		t.Fatalf("ExternalReferences = %+v", command.ExternalReferences)
	}

	bare, err := intake.IntakeBusinessPartyRegistration(context.Background(),
		identityRequest("/commercial-business-party-registrations", `{"businessParties":[{
		"partyId":"SYN-PARTY-02","name":"SYN 合成参与方二号","revision":1,"basis":"SYN-BASIS/party-02",
		"effectiveFrom":"2026-09-16T00:00:00Z"}]}`))
	if err != nil || bare.RegistrationCountry != nil || bare.LifetimeNumbers != nil || bare.ExternalReferences != nil {
		t.Fatalf("bare = (%+v, %v), want no identification and no error", bare, err)
	}

	malformed := map[string]string{
		"国家小写":    `"registrationCountry":"xa"`,
		"号为空":     `"registrationCountry":"CN","lifetimeRegistrationNumbers":[{"typeCode":"SYN-XA-LIFETIME","number":""}]`,
		"系统带空格":   `"externalReferences":[{"system":"KD ERP","purpose":"CUSTOMER","code":"C-1"}]`,
		"用途不在词表":  `"externalReferences":[{"system":"SYN-ERP","purpose":"VENDOR","code":"C-1"}]`,
		"编码首尾带空白": `"externalReferences":[{"system":"SYN-ERP","purpose":"CUSTOMER","code":" C-1"}]`,
	}
	for name, fields := range malformed {
		t.Run(name, func(t *testing.T) {
			_, err := intake.IntakeBusinessPartyRegistration(context.Background(),
				identityRequest("/commercial-business-party-registrations", `{"businessParties":[{
				"partyId":"SYN-PARTY-02","name":"SYN 合成参与方二号","revision":1,"basis":"SYN-BASIS/party-02",
				"effectiveFrom":"2026-09-16T00:00:00Z",`+fields+`}]}`))
			if !errors.Is(err, commercialhttp.ErrMalformedRequest) {
				t.Fatalf("error = %v, want ErrMalformedRequest", err)
			}
		})
	}
}

var partyIdentificationRow = ports.BusinessPartyIdentificationRow{
	HasIdentityLayer:        true,
	RegistrationCountry:     "CN",
	LifetimeNumbers:         []ports.LifetimeRegistrationNumberRow{{TypeCode: "SYN-XA-LIFETIME", Number: "SYN-XA-000009"}},
	HasIdentityCorrection:   true,
	IdentityCorrectionBasis: "SYN-CORRECTION-01",
	ExternalReferences:      []ports.ExternalReferenceRow{{System: "SYN-ERP", Purpose: "CUSTOMER", Code: "C-0009"}},
}

func assertPartyIdentificationKeys(t *testing.T, label string, item map[string]json.RawMessage, want bool) {
	t.Helper()
	assertIdentityLayerKeys(t, label, item, want)
	raw, has := item["externalReferences"]
	if has != want {
		t.Fatalf("%s externalReferences present = %v, want %v", label, has, want)
	}
	if !want {
		return
	}
	var references []struct {
		System  string `json:"system"`
		Purpose string `json:"purpose"`
		Code    string `json:"code"`
	}
	if err := json.Unmarshal(raw, &references); err != nil || len(references) != 1 ||
		references[0].System != "SYN-ERP" || references[0].Purpose != "CUSTOMER" || references[0].Code != "C-0009" {
		t.Fatalf("%s externalReferences = %s (err %v)", label, raw, err)
	}
}

// Covers: 目录与修订历史各一行带识别信息、一行不带。
func TestBusinessPartyBodiesCarryIdentification(t *testing.T) {
	tenant := catValue(t, domain.NewTenantID, "tenant-1")
	reader := &partyIdentityReaderDouble{
		tenant: tenant,
		parties: []ports.BusinessPartyRow{
			{
				TenantID: "tenant-1", PartyID: "party-id", PartyName: "带识别信息", Status: "EFFECTIVE", Revision: 2,
				Basis: "basis-id", EffectiveFrom: identityListedAt, RegisteredAt: identityListedAt,
				BusinessPartyIdentificationRow: partyIdentificationRow,
			},
			{
				TenantID: "tenant-1", PartyID: "party-plain", PartyName: "没登识别信息", Status: "EFFECTIVE", Revision: 1,
				Basis: "basis-plain", EffectiveFrom: identityListedAt, RegisteredAt: identityListedAt,
			},
		},
	}
	recorder := httptest.NewRecorder()
	commercialhttp.NewQueryBusinessPartiesEndpoint(intakeDouble{query: catalogueQuery(t)}, reader).
		ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/commercial-business-parties", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body)
	}
	var catalogue struct {
		Parties []map[string]json.RawMessage `json:"parties"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &catalogue); err != nil || len(catalogue.Parties) != 2 {
		t.Fatalf("decode catalogue %s: %v", recorder.Body, err)
	}
	assertPartyIdentificationKeys(t, "目录行 party-id", catalogue.Parties[0], true)
	assertPartyIdentificationKeys(t, "目录行 party-plain", catalogue.Parties[1], false)

	revisions := &businessPartyRevisionReaderDouble{
		tenant: tenant,
		party:  "party-id",
		rows: []ports.BusinessPartyRevisionRow{
			{TenantID: "tenant-1", PartyID: "party-id", PartyName: "带识别信息", Revision: 1, Basis: "basis-id",
				EffectiveFrom: revisionRegisteredAt, RegisteredAt: revisionRegisteredAt},
			{TenantID: "tenant-1", PartyID: "party-id", PartyName: "带识别信息", Revision: 2, Basis: "basis-id",
				EffectiveFrom: revisionRegisteredAt, RegisteredAt: revisionRegisteredAt,
				BusinessPartyIdentificationRow: partyIdentificationRow},
		},
	}
	recorder = httptest.NewRecorder()
	commercialhttp.NewQueryBusinessPartyRevisionsEndpoint(intakeDouble{query: catalogueQuery(t)}, revisions).
		ServeHTTP(recorder, partyRevisionsRequest("party-id"))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body)
	}
	var history struct {
		Revisions []map[string]json.RawMessage `json:"revisions"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &history); err != nil || len(history.Revisions) != 2 {
		t.Fatalf("decode revisions %s: %v", recorder.Body, err)
	}
	assertPartyIdentificationKeys(t, "修订 1", history.Revisions[0], false)
	assertPartyIdentificationKeys(t, "修订 2", history.Revisions[1], true)
}
