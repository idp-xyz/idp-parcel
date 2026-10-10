package accessidentity_test

import (
	"context"
	"errors"
	"testing"
	"time"

	identity "go.idp.xyz/idp-parcel/internal/accessidentity"
	ccaccess "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/accessidentity"
	customshttp "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/http"
)

var clientNow = time.Date(2026, 10, 9, 2, 0, 0, 0, time.UTC)

type clientVerifier struct{ err error }

func (verifier clientVerifier) VerifyIntegrationClientCredential(
	_ context.Context,
	credential identity.IntegrationClientCredential,
) (identity.VerifiedClientToken, error) {
	if verifier.err != nil {
		return identity.VerifiedClientToken{}, verifier.err
	}
	if credential.Token() == "" {
		return identity.VerifiedClientToken{}, identity.ErrCredentialRejected
	}
	subject, err := identity.NewIntegrationClientSubject("https://id.syn.example/dex", "SYN-CLIENT-01")
	if err != nil {
		return identity.VerifiedClientToken{}, err
	}
	return identity.NewVerifiedClientToken(subject, false), nil
}

type clientRegister struct {
	facts []identity.ExternalFactType
	found bool
	err   error
	at    time.Time
}

func (register clientRegister) FindIntegrationClient(context.Context, identity.IntegrationClientSubject) (identity.IntegrationClientStanding, bool, error) {
	if register.err != nil {
		return identity.IntegrationClientStanding{}, false, register.err
	}
	if !register.found {
		return identity.IntegrationClientStanding{}, false, nil
	}
	subject, err := identity.NewIntegrationClientSubject("https://id.syn.example/dex", "SYN-CLIENT-01")
	if err != nil {
		return identity.IntegrationClientStanding{}, false, err
	}
	reference, err := identity.NewCredentialReference("SYN-CREDENTIAL-REF/broker")
	if err != nil {
		return identity.IntegrationClientStanding{}, false, err
	}
	binding, err := identity.NewIntegrationClientBinding(subject, "SYN-TENANT-01", "SYN-SOURCE/broker", reference, false, "SYN-BASIS")
	if err != nil {
		return identity.IntegrationClientStanding{}, false, err
	}
	start := register.at
	if start.IsZero() {
		start = clientNow
	}
	interval, err := identity.NewEffectiveInterval(start.Add(-time.Hour), start.Add(time.Hour))
	if err != nil {
		return identity.IntegrationClientStanding{}, false, err
	}
	var grants []identity.RecordedClientGrant
	for _, fact := range register.facts {
		grant, err := identity.NewIntegrationClientGrant("SYN-TENANT-01", "SYN-GRANT-"+fact.String(), subject, fact, interval, "SYN-BASIS-GRANT")
		if err != nil {
			return identity.IntegrationClientStanding{}, false, err
		}
		recorded, err := identity.NewRecordedClientGrant(grant, nil)
		if err != nil {
			return identity.IntegrationClientStanding{}, false, err
		}
		grants = append(grants, recorded)
	}
	standing, err := identity.NewIntegrationClientStanding(binding, grants)
	return standing, err == nil, err
}

type admittingScope struct{}

func (admittingScope) Admits(context.Context, string, identity.AdmissionRequirement, time.Time) (bool, error) {
	return true, nil
}

func customsAuthenticator(t *testing.T, verifier identity.IntegrationClientCredentialVerifier, register clientRegister, admission identity.AdmissionScope) *ccaccess.IntegrationClientAuthenticator {
	t.Helper()
	minter, err := identity.NewIntegrationClientMinter(verifier, register, admission, func() time.Time { return clientNow })
	if err != nil {
		t.Fatal(err)
	}
	built, err := ccaccess.NewIntegrationClientAuthenticator(minter)
	if err != nil {
		t.Fatal(err)
	}
	return built
}

func TestCustomsIntegrationClientGrades(t *testing.T) {
	granted := clientRegister{facts: []identity.ExternalFactType{identity.FactCustomsExternalResult}, found: true}
	cases := map[string]struct {
		verifier identity.IntegrationClientCredentialVerifier
		register clientRegister
		want     error
	}{
		"issuer parameters unset": {verifier: identity.UnconfiguredIntegrationClientCredentialVerifier{}, register: granted, want: customshttp.ErrAccessChannelNotConfigured},
		"token rejected":          {verifier: clientVerifier{err: identity.ErrCredentialRejected}, register: granted, want: customshttp.ErrIntegrationClientCredentialRejected},
		"client not registered":   {verifier: clientVerifier{}, register: clientRegister{}, want: customshttp.ErrIntegrationClientNotGranted},
		"grant lacks the fact": {
			verifier: clientVerifier{},
			register: clientRegister{facts: []identity.ExternalFactType{identity.FactRegulatoryCredential}, found: true},
			want:     customshttp.ErrIntegrationClientNotGranted,
		},
		"outside the interval": {
			verifier: clientVerifier{},
			register: clientRegister{facts: []identity.ExternalFactType{identity.FactCustomsExternalResult}, found: true, at: clientNow.Add(48 * time.Hour)},
			want:     customshttp.ErrIntegrationClientNotGranted,
		},
		"register cannot be read": {
			verifier: clientVerifier{},
			register: clientRegister{err: errors.New("connection refused")},
			want:     customshttp.ErrIdentityDependencyUnavailable,
		},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			built := customsAuthenticator(t, testCase.verifier, testCase.register, identity.UnconfiguredAdmissionScope{})
			_, err := built.Authenticate(context.Background(), customshttp.PresentedClientCredential{Token: "presented.client.token"}, customshttp.FactCustomsExternalResult)
			if !errors.Is(err, testCase.want) {
				t.Fatalf("err = %v, want %v", err, testCase.want)
			}
		})
	}
}

func TestGrantedCustomsClientReachesTheBoundTenantWhenAdmissionCoversTheWrite(t *testing.T) {
	built := customsAuthenticator(t, clientVerifier{},
		clientRegister{facts: []identity.ExternalFactType{identity.FactCustomsExternalResult, identity.FactRegulatoryCredential}, found: true},
		admittingScope{})
	for _, fact := range []customshttp.IntegrationClientFact{customshttp.FactCustomsExternalResult, customshttp.FactRegulatoryCredential} {
		tenant, err := built.Authenticate(context.Background(), customshttp.PresentedClientCredential{Token: "presented.client.token"}, fact)
		if err != nil || tenant.String() != "SYN-TENANT-01" {
			t.Fatalf("%s: tenant %q, err %v", fact, tenant.String(), err)
		}
	}
}

func TestGrantedCustomsClientWithoutARegisteredAdmissionIntervalIsOutsideScope(t *testing.T) {
	built := customsAuthenticator(t, clientVerifier{},
		clientRegister{facts: []identity.ExternalFactType{identity.FactCustomsExternalResult}, found: true},
		identity.UnconfiguredAdmissionScope{})
	_, err := built.Authenticate(context.Background(), customshttp.PresentedClientCredential{Token: "presented.client.token"}, customshttp.FactCustomsExternalResult)
	if !errors.Is(err, customshttp.ErrOutsideAdmissionScope) {
		t.Fatalf("err = %v, want ErrOutsideAdmissionScope", err)
	}
}
