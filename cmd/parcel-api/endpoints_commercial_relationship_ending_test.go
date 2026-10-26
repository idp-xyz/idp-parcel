package main

import (
	"context"
	"net/http"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	commercialhttp "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/http"
	pcpostgres "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	commercialapp "go.idp.xyz/idp-parcel/internal/partycommercial/application"
	commercialdomain "go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

func init() {
	registerUnwired(commercialRelationshipEndingWiring{relationshipEnding: unwiredPartyRelationshipEnding{}})
}

// unwiredPartyRelationshipEnding 只给装配测试用，答法同 unwired_orchestration.go 里的各占位。
type unwiredPartyRelationshipEnding struct{}

func (unwiredPartyRelationshipEnding) Revoke(
	context.Context, commercialapp.RevokePartyRelationshipCommand,
) (commercialapp.PartyRegistryResult, error) {
	return commercialapp.PartyRegistryResult{}, errOrchestrationNotWired
}

func (unwiredPartyRelationshipEnding) Expire(
	context.Context, commercialapp.ExpirePartyRelationshipCommand,
) (commercialapp.PartyRegistryResult, error) {
	return commercialapp.PartyRegistryResult{}, errOrchestrationNotWired
}

func (unwiredPartyRelationshipEnding) Supersede(
	context.Context, commercialapp.SupersedePartyRelationshipCommand,
) (commercialapp.PartyRegistryResult, error) {
	return commercialapp.PartyRegistryResult{}, errOrchestrationNotWired
}

// Covers: 票 17 完成判据「……HTTP……各层用例」的进程装配半边——三个结束口经操作者渠道走到真库：生产铸造器、PC 认证适配器、
// 操作者 Intake 与生产编排（含事务壳）连起来，只有令牌校验与操作者册是替身。登记口立起三段关系之后，撤销、到期、替代各答
// 201 ENDED，同一笔重放答 200 ALREADY_REGISTERED；册上读回终止状态与接替关系；不带令牌答 401 且没落行。测试输入是隔离
// 合成，只记 `S`。
func TestTheOperatorChannelEndsPartyRelationshipsAgainstARealDatabase(t *testing.T) {
	db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	registration, err := buildCommercialRegistrationOrchestration(db)
	if err != nil {
		t.Fatalf("装配商业登记编排：%v", err)
	}
	ending, err := buildCommercialRelationshipEnding(db)
	if err != nil {
		t.Fatalf("装配关系结束编排：%v", err)
	}
	registries := operatorRegistriesForRegistryWriter(t)
	parties := commercialhttp.NewRegisterBusinessPartyEndpoint(registries.commercial, registration.partyIdentity)
	relationships := commercialhttp.NewRegisterPartyRelationshipEndpoint(registries.commercial, registration.partyIdentity)
	revocations := commercialhttp.NewRevokePartyRelationshipEndpoint(registries.commercial, ending)
	expirations := commercialhttp.NewExpirePartyRelationshipEndpoint(registries.commercial, ending)
	supersessions := commercialhttp.NewSupersedePartyRelationshipEndpoint(registries.commercial, ending)

	party := func(id string) string {
		return `{"businessParties":[{"partyId":"` + id + `","name":"SYN 合成参与方 ` + id + `","revision":1,` +
			`"basis":"SYN-BASIS/` + id + `","effectiveFrom":"2026-01-01T00:00:00Z"}]}`
	}
	relationship := func(id, endsAt string) string {
		end := ""
		if endsAt != "" {
			end = `"effectiveEndsAt":"` + endsAt + `",`
		}
		return `{"relationships":[{"relationshipId":"` + id + `","revision":1,"holder":"SYN-PARTY-PS17-01","counterparty":"SYN-PARTY-PS17-02",` +
			`"role":"CARRIER_AGENT","scope":"SYN-SCOPE/ps17","basis":"SYN-BASIS/` + id + `","effectiveStartsAt":"2026-02-01T00:00:00Z",` + end +
			`"approval":{"reference":"SYN-APPROVAL/` + id + `","approvedAt":"2026-02-01T00:00:00Z"}}]}`
	}
	const (
		revocation   = `{"revocations":[{"relationshipId":"SYN-REL-PS17-REVOKE","revision":2,"basis":"SYN-REVOCATION/ps17","at":"2026-05-01T00:00:00Z"}]}`
		expiration   = `{"expirations":[{"relationshipId":"SYN-REL-PS17-EXPIRE","revision":2,"basis":"SYN-TERM-ENDED/ps17","at":"2026-04-01T00:00:00Z"}]}`
		supersession = `{"supersessions":[{"relationshipId":"SYN-REL-PS17-SUPERSEDE","revision":2,"successor":"SYN-PARTY-PS17-03",` +
			`"at":"2026-05-01T00:00:00Z","successorRelationshipId":"SYN-REL-PS17-SUPERSEDE-NEXT","basis":"SYN-HANDOVER/ps17",` +
			`"approval":{"reference":"SYN-APPROVAL/handover-ps17","approvedAt":"2026-05-01T00:00:00Z"}}]}`
	)

	anonymous := postAsOperator(revocations, "/commercial-party-relationship-revocations", "", revocation)
	if anonymous.Code != http.StatusUnauthorized || problemCode(t, anonymous) != "OPERATOR_CREDENTIAL_REJECTED" {
		t.Fatalf("不带令牌答 %d %s，want 401 OPERATOR_CREDENTIAL_REJECTED", anonymous.Code, anonymous.Body.String())
	}

	for _, step := range []struct {
		label    string
		endpoint http.Handler
		target   string
		body     string
		status   int
		outcome  string
	}{
		{"持有方", parties, "/commercial-business-party-registrations", party("SYN-PARTY-PS17-01"), http.StatusCreated, "REGISTERED"},
		{"相对方", parties, "/commercial-business-party-registrations", party("SYN-PARTY-PS17-02"), http.StatusCreated, "REGISTERED"},
		{"接替方", parties, "/commercial-business-party-registrations", party("SYN-PARTY-PS17-03"), http.StatusCreated, "REGISTERED"},
		{"要撤销的关系", relationships, "/commercial-party-relationship-registrations", relationship("SYN-REL-PS17-REVOKE", ""), http.StatusCreated, "REGISTERED"},
		{"要到期的关系", relationships, "/commercial-party-relationship-registrations", relationship("SYN-REL-PS17-EXPIRE", "2026-04-01T00:00:00Z"), http.StatusCreated, "REGISTERED"},
		{"要替代的关系", relationships, "/commercial-party-relationship-registrations", relationship("SYN-REL-PS17-SUPERSEDE", ""), http.StatusCreated, "REGISTERED"},
		{"撤销", revocations, "/commercial-party-relationship-revocations", revocation, http.StatusCreated, "ENDED"},
		{"到期", expirations, "/commercial-party-relationship-expirations", expiration, http.StatusCreated, "ENDED"},
		{"替代", supersessions, "/commercial-party-relationship-supersessions", supersession, http.StatusCreated, "ENDED"},
		{"重放撤销", revocations, "/commercial-party-relationship-revocations", revocation, http.StatusOK, "ALREADY_REGISTERED"},
		{"重放替代", supersessions, "/commercial-party-relationship-supersessions", supersession, http.StatusOK, "ALREADY_REGISTERED"},
	} {
		response := postAsOperator(step.endpoint, step.target, presentedOperatorToken, step.body)
		if response.Code != step.status || registrationOutcome(t, response) != step.outcome {
			t.Fatalf("%s：答 %d %s，want %d %s", step.label, response.Code, response.Body.String(), step.status, step.outcome)
		}
	}

	identities, err := pcpostgres.NewPartyIdentityRegistrations(db)
	if err != nil {
		t.Fatalf("身份登记册：%v", err)
	}
	tenant := pcSynthetic(t, commercialdomain.NewTenantID, "SYN-TENANT-01")
	for id, want := range map[string]commercialdomain.RelationshipStatus{
		"SYN-REL-PS17-REVOKE":         commercialdomain.RelationshipRevoked,
		"SYN-REL-PS17-EXPIRE":         commercialdomain.RelationshipExpired,
		"SYN-REL-PS17-SUPERSEDE":      commercialdomain.RelationshipSuperseded,
		"SYN-REL-PS17-SUPERSEDE-NEXT": commercialdomain.RelationshipEffective,
	} {
		latest, found, err := identities.LoadLatestRelationship(t.Context(), tenant, pcSynthetic(t, commercialdomain.NewRelationshipID, id))
		if err != nil || !found || latest.Relationship().Status() != want {
			t.Fatalf("%s 读回 = (%v, %v, %v), want %s", id, latest.Relationship().Status(), found, err, want)
		}
	}
	revoked, _, err := identities.LoadLatestRelationship(t.Context(), tenant, pcSynthetic(t, commercialdomain.NewRelationshipID, "SYN-REL-PS17-REVOKE"))
	if err != nil {
		t.Fatalf("读回撤销行：%v", err)
	}
	if endedAt, _ := revoked.Relationship().EndedAt(); !endedAt.Equal(time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("撤销时刻读回 %v", endedAt)
	}
}
