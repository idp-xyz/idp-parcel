package accessidentity_test

import (
	"context"
	"errors"
	"testing"
	"time"

	identity "go.idp.xyz/idp-parcel/internal/accessidentity"
	saaccess "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/accessidentity"
	settlementhttp "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/http"
)

var fundsNow = time.Date(2026, 10, 9, 2, 0, 0, 0, time.UTC)

type fundsVerifier struct{ err error }

func (verifier fundsVerifier) VerifyIntegrationClientCredential(
	_ context.Context,
	credential identity.IntegrationClientCredential,
) (identity.VerifiedClientToken, error) {
	if verifier.err != nil {
		return identity.VerifiedClientToken{}, verifier.err
	}
	subject, err := identity.NewIntegrationClientSubject("https://id.syn.example/dex", "SYN-CLIENT-01")
	if err != nil {
		return identity.VerifiedClientToken{}, err
	}
	return identity.NewVerifiedClientToken(subject, false), nil
}

type fundsRegister struct {
	fact  identity.ExternalFactType
	found bool
}

func (register fundsRegister) FindIntegrationClient(context.Context, identity.IntegrationClientSubject) (identity.IntegrationClientStanding, bool, error) {
	if !register.found {
		return identity.IntegrationClientStanding{}, false, nil
	}
	subject, err := identity.NewIntegrationClientSubject("https://id.syn.example/dex", "SYN-CLIENT-01")
	if err != nil {
		return identity.IntegrationClientStanding{}, false, err
	}
	reference, err := identity.NewCredentialReference("SYN-CREDENTIAL-REF/bank")
	if err != nil {
		return identity.IntegrationClientStanding{}, false, err
	}
	binding, err := identity.NewIntegrationClientBinding(subject, "SYN-TENANT-01", "SYN-SOURCE/bank", reference, false, "SYN-BASIS")
	if err != nil {
		return identity.IntegrationClientStanding{}, false, err
	}
	interval, err := identity.NewEffectiveInterval(fundsNow.Add(-time.Hour), fundsNow.Add(time.Hour))
	if err != nil {
		return identity.IntegrationClientStanding{}, false, err
	}
	grant, err := identity.NewIntegrationClientGrant("SYN-TENANT-01", "SYN-GRANT-FUNDS", subject, register.fact, interval, "SYN-BASIS-GRANT")
	if err != nil {
		return identity.IntegrationClientStanding{}, false, err
	}
	recorded, err := identity.NewRecordedClientGrant(grant, nil)
	if err != nil {
		return identity.IntegrationClientStanding{}, false, err
	}
	standing, err := identity.NewIntegrationClientStanding(binding, []identity.RecordedClientGrant{recorded})
	return standing, err == nil, err
}

func fundsAuthenticatorBuilt(t *testing.T, register fundsRegister, admission identity.AdmissionScope) *saaccess.IntegrationClientAuthenticator {
	t.Helper()
	minter, err := identity.NewIntegrationClientMinter(fundsVerifier{}, register, admission, func() time.Time { return fundsNow })
	if err != nil {
		t.Fatal(err)
	}
	built, err := saaccess.NewIntegrationClientAuthenticator(minter)
	if err != nil {
		t.Fatal(err)
	}
	return built
}

func TestExternalFundsClientWithoutTheGrantIsNotGranted(t *testing.T) {
	built := fundsAuthenticatorBuilt(t, fundsRegister{fact: identity.FactCustomsExternalResult, found: true}, identity.UnconfiguredAdmissionScope{})
	_, err := built.AuthenticateExternalFunds(context.Background(), settlementhttp.PresentedClientCredential{Token: "presented.client.token"})
	if !errors.Is(err, settlementhttp.ErrIntegrationClientNotGranted) {
		t.Fatalf("err = %v, want ErrIntegrationClientNotGranted", err)
	}
}

func TestExternalFundsClientWithoutARegisteredAdmissionIntervalIsOutsideScope(t *testing.T) {
	built := fundsAuthenticatorBuilt(t, fundsRegister{fact: identity.FactExternalFunds, found: true}, identity.UnconfiguredAdmissionScope{})
	_, err := built.AuthenticateExternalFunds(context.Background(), settlementhttp.PresentedClientCredential{Token: "presented.client.token"})
	if !errors.Is(err, settlementhttp.ErrOutsideAdmissionScope) {
		t.Fatalf("err = %v, want ErrOutsideAdmissionScope", err)
	}
}

func TestRejectedExternalFundsTokenStaysCredentialRejected(t *testing.T) {
	minter, err := identity.NewIntegrationClientMinter(fundsVerifier{err: identity.ErrCredentialRejected}, fundsRegister{}, identity.UnconfiguredAdmissionScope{}, func() time.Time { return fundsNow })
	if err != nil {
		t.Fatal(err)
	}
	built, err := saaccess.NewIntegrationClientAuthenticator(minter)
	if err != nil {
		t.Fatal(err)
	}
	_, err = built.AuthenticateExternalFunds(context.Background(), settlementhttp.PresentedClientCredential{Token: "presented.client.token"})
	if !errors.Is(err, settlementhttp.ErrIntegrationClientCredentialRejected) {
		t.Fatalf("err = %v, want ErrIntegrationClientCredentialRejected", err)
	}
}
