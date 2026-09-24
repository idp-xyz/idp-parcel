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

// 票 legal-entity-profile/02 的传输层用例：隔离写 Intake 把身份层三格译进命令（形状不对即 MALFORMED_REQUEST，缺不缺
// 交给用例判）；目录与修订历史两个读口的答复带出身份层四格，历史修订只带显式的 identityLayerRegistered:false。

func TestIsolatedIntakeTranslatesTheLegalEntityIdentityLayer(t *testing.T) {
	intake := isolatedIdentityIntakeForTest(t)
	command, err := intake.IntakeLegalEntityRegistration(context.Background(), legalEntityRequest(`{"legalEntities":[{
		"legalEntityId":"SYN-LE-02",
		"partyId":"SYN-PARTY-02",
		"revision":2,
		"basis":"SYN-BASIS/le-02-r2",
		"effectiveFrom":"2026-09-16T00:00:00Z",
		"registrationCountry":"XA",
		"lifetimeRegistrationNumbers":[{"typeCode":"SYN-XA-LIFETIME","number":"SYN-XA-000009"}],
		"identityCorrectionBasis":"SYN-CORRECTION-01"
	}]}`))
	if err != nil {
		t.Fatalf("intake：%v", err)
	}
	if command.RegistrationCountry == nil || command.RegistrationCountry.String() != "XA" {
		t.Fatalf("RegistrationCountry = %v, want XA", command.RegistrationCountry)
	}
	if len(command.LifetimeNumbers) != 1 ||
		command.LifetimeNumbers[0].TypeCode().String() != "SYN-XA-LIFETIME" ||
		command.LifetimeNumbers[0].Number().String() != "SYN-XA-000009" {
		t.Fatalf("LifetimeNumbers = %+v", command.LifetimeNumbers)
	}
	if command.IdentityCorrectionBasis == nil || command.IdentityCorrectionBasis.String() != "SYN-CORRECTION-01" {
		t.Fatalf("IdentityCorrectionBasis = %v", command.IdentityCorrectionBasis)
	}

	// 三键都缺是本格落地之前的形状：照旧译得出命令，缺不缺由用例判。
	bare, err := intake.IntakeLegalEntityRegistration(context.Background(), legalEntityRequest(`{"legalEntities":[{
		"legalEntityId":"SYN-LE-02","partyId":"SYN-PARTY-02","revision":1,"basis":"SYN-BASIS/le-02","effectiveFrom":"2026-09-16T00:00:00Z"
	}]}`))
	if err != nil || bare.RegistrationCountry != nil || bare.LifetimeNumbers != nil || bare.IdentityCorrectionBasis != nil {
		t.Fatalf("bare = (%+v, %v), want no identity fields and no error", bare, err)
	}

	malformed := map[string]string{
		"国家小写":  `"registrationCountry":"xa"`,
		"类型为空":  `"registrationCountry":"XA","lifetimeRegistrationNumbers":[{"typeCode":"","number":"SYN-XA-000001"}]`,
		"号为空":   `"registrationCountry":"XA","lifetimeRegistrationNumbers":[{"typeCode":"SYN-XA-LIFETIME","number":""}]`,
		"更正依据空": `"identityCorrectionBasis":""`,
	}
	for name, fields := range malformed {
		t.Run(name, func(t *testing.T) {
			_, err := intake.IntakeLegalEntityRegistration(context.Background(), legalEntityRequest(`{"legalEntities":[{
				"legalEntityId":"SYN-LE-02","partyId":"SYN-PARTY-02","revision":1,"basis":"SYN-BASIS/le-02",
				"effectiveFrom":"2026-09-16T00:00:00Z",`+fields+`}]}`))
			if !errors.Is(err, commercialhttp.ErrMalformedRequest) {
				t.Fatalf("error = %v, want ErrMalformedRequest", err)
			}
		})
	}
}

var identityLayerRow = struct {
	country    string
	numbers    []ports.LifetimeRegistrationNumberRow
	correction string
}{
	country:    "XA",
	numbers:    []ports.LifetimeRegistrationNumberRow{{TypeCode: "SYN-XA-LIFETIME", Number: "SYN-XA-000009"}},
	correction: "SYN-CORRECTION-01",
}

func assertIdentityLayerKeys(t *testing.T, label string, item map[string]json.RawMessage, wantRegistered bool) {
	t.Helper()
	var registered bool
	if err := json.Unmarshal(item["identityLayerRegistered"], &registered); err != nil || registered != wantRegistered {
		t.Fatalf("%s identityLayerRegistered = %s (err %v), want %v", label, item["identityLayerRegistered"], err, wantRegistered)
	}
	for _, key := range []string{"registrationCountry", "lifetimeRegistrationNumbers", "identityCorrectionBasis"} {
		if _, has := item[key]; has != wantRegistered {
			t.Fatalf("%s key %s present = %v, want %v", label, key, has, wantRegistered)
		}
	}
	if !wantRegistered {
		return
	}
	var country, correction string
	var numbers []struct {
		TypeCode string `json:"typeCode"`
		Number   string `json:"number"`
	}
	_ = json.Unmarshal(item["registrationCountry"], &country)
	_ = json.Unmarshal(item["identityCorrectionBasis"], &correction)
	_ = json.Unmarshal(item["lifetimeRegistrationNumbers"], &numbers)
	if country != "XA" || correction != "SYN-CORRECTION-01" || len(numbers) != 1 ||
		numbers[0].TypeCode != "SYN-XA-LIFETIME" || numbers[0].Number != "SYN-XA-000009" {
		t.Fatalf("%s identity layer = country %q, correction %q, numbers %+v", label, country, correction, numbers)
	}
}

// Covers: 票 legal-entity-profile/02 完成判据「端点用例：答复带两格」——目录与修订历史各一行带身份层与更正依据、
// 一行是历史修订。
func TestLegalEntityBodiesCarryTheIdentityLayer(t *testing.T) {
	tenant := catValue(t, domain.NewTenantID, "tenant-1")
	reader := &partyIdentityReaderDouble{
		tenant: tenant,
		entities: []ports.GroupLegalEntityRow{
			{
				TenantID: "tenant-1", LegalEntityID: "le-1", PartyID: "party-le", Status: "EFFECTIVE",
				Revision: 2, Basis: "basis-le", EffectiveFrom: identityListedAt, RegisteredAt: identityListedAt,
				HasIdentityLayer: true, RegistrationCountry: identityLayerRow.country, LifetimeNumbers: identityLayerRow.numbers,
				HasIdentityCorrection: true, IdentityCorrectionBasis: identityLayerRow.correction,
			},
			{
				TenantID: "tenant-1", LegalEntityID: "le-legacy", PartyID: "party-le", Status: "EFFECTIVE",
				Revision: 1, Basis: "basis-legacy", EffectiveFrom: identityListedAt, RegisteredAt: identityListedAt,
			},
		},
	}
	recorder := httptest.NewRecorder()
	commercialhttp.NewQueryGroupLegalEntitiesEndpoint(intakeDouble{query: catalogueQuery(t)}, reader).
		ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/commercial-group-legal-entities", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body)
	}
	var catalogue struct {
		Entities []map[string]json.RawMessage `json:"entities"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &catalogue); err != nil || len(catalogue.Entities) != 2 {
		t.Fatalf("decode catalogue %s: %v", recorder.Body, err)
	}
	assertIdentityLayerKeys(t, "目录行 le-1", catalogue.Entities[0], true)
	assertIdentityLayerKeys(t, "目录行 le-legacy", catalogue.Entities[1], false)

	revisions := &legalEntityRevisionReaderDouble{
		tenant: tenant,
		entity: "le-1",
		rows: []ports.LegalEntityRevisionRow{
			{
				TenantID: "tenant-1", LegalEntityID: "le-1", PartyID: "party-le", Revision: 1, Basis: "basis-le",
				EffectiveFrom: revisionRegisteredAt, RegisteredAt: revisionRegisteredAt,
			},
			{
				TenantID: "tenant-1", LegalEntityID: "le-1", PartyID: "party-le", Revision: 2, Basis: "basis-le",
				EffectiveFrom: revisionRegisteredAt, RegisteredAt: revisionRegisteredAt,
				HasIdentityLayer: true, RegistrationCountry: identityLayerRow.country, LifetimeNumbers: identityLayerRow.numbers,
				HasIdentityCorrection: true, IdentityCorrectionBasis: identityLayerRow.correction,
			},
		},
	}
	recorder = httptest.NewRecorder()
	commercialhttp.NewQueryLegalEntityRevisionsEndpoint(intakeDouble{query: catalogueQuery(t)}, revisions).
		ServeHTTP(recorder, revisionsRequest("le-1"))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body)
	}
	var history struct {
		Revisions []map[string]json.RawMessage `json:"revisions"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &history); err != nil || len(history.Revisions) != 2 {
		t.Fatalf("decode revisions %s: %v", recorder.Body, err)
	}
	assertIdentityLayerKeys(t, "修订 1（历史形状）", history.Revisions[0], false)
	assertIdentityLayerKeys(t, "修订 2", history.Revisions[1], true)
}
