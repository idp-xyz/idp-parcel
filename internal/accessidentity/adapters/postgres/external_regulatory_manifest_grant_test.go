package postgres_test

import (
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
)

// Covers: 迁移 access_identity 0042 的册面（UC-CC-012）——`外部监管舱单`这一类外部事实的授予登得进集成客户端册、读回仍是
// 这一类，不因此持有监管结果；先放开的承运订舱答复与外部资金事实仍登得进去，证约束重建时没有漏抄已有的类型。
func TestTheExternalRegulatoryManifestFactIsGrantedAndReadBack(t *testing.T) {
	register := newClientRegister(t)
	subject := clientSubjectOf(t, syntheticIssuer, "SYN-CLIENT-CARRIER-MANIFEST")
	spec := defaultClientBinding("SYN-TENANT-01")
	spec.source = "SYN-PARTY-CARRIER-TRUNK-01"
	expectClientOutcome(t, "bind", register.bind(t, clientBindingOf(t, subject, spec)), accessidentity.IntegrationClientRegistrationRecorded)
	startsAt := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for grantID, fact := range map[string]accessidentity.ExternalFactType{
		"SYN-CLIENT-GRANT-MANIFEST":        accessidentity.FactExternalRegulatoryManifest,
		"SYN-CLIENT-GRANT-MANIFEST-ANSWER": accessidentity.FactCarrierBookingAnswer,
		"SYN-CLIENT-GRANT-MANIFEST-FUNDS":  accessidentity.FactExternalFunds,
	} {
		expectClientOutcome(t, string(fact)+" grant",
			register.grant(t, clientGrantOf(t, "SYN-TENANT-01", grantID, subject, fact, startsAt, time.Time{})),
			accessidentity.IntegrationClientRegistrationRecorded)
	}

	standing := register.standing(t, subject)
	at := time.Date(2026, 11, 13, 0, 0, 0, 0, time.UTC)
	if standing.Binding().SourceIdentity() != "SYN-PARTY-CARRIER-TRUNK-01" ||
		!standing.HoldsAt(accessidentity.FactExternalRegulatoryManifest, at) ||
		!standing.HoldsAt(accessidentity.FactCarrierBookingAnswer, at) ||
		!standing.HoldsAt(accessidentity.FactExternalFunds, at) ||
		standing.HoldsAt(accessidentity.FactCustomsExternalResult, at) {
		t.Fatalf("事实类型或来源身份没有原样读回：%+v", standing.Grants())
	}
}
