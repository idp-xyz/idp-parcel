package accessidentity_test

import (
	"context"
	"errors"
	"testing"
	"time"

	identity "go.idp.xyz/idp-parcel/internal/accessidentity"
	tfaccess "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/accessidentity"
	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
)

var carrierNow = time.Date(2026, 10, 19, 2, 0, 0, 0, time.UTC)

type carrierVerifier struct{ err error }

func (verifier carrierVerifier) VerifyIntegrationClientCredential(
	_ context.Context,
	credential identity.IntegrationClientCredential,
) (identity.VerifiedClientToken, error) {
	if verifier.err != nil {
		return identity.VerifiedClientToken{}, verifier.err
	}
	if credential.Token() == "" {
		return identity.VerifiedClientToken{}, identity.ErrCredentialRejected
	}
	subject, err := identity.NewIntegrationClientSubject("https://id.syn.example/dex", "SYN-CLIENT-CARRIER-01")
	if err != nil {
		return identity.VerifiedClientToken{}, err
	}
	return identity.NewVerifiedClientToken(subject, false), nil
}

type carrierRegister struct {
	facts []identity.ExternalFactType
	found bool
	err   error
}

func (register carrierRegister) FindIntegrationClient(context.Context, identity.IntegrationClientSubject) (identity.IntegrationClientStanding, bool, error) {
	if register.err != nil {
		return identity.IntegrationClientStanding{}, false, register.err
	}
	if !register.found {
		return identity.IntegrationClientStanding{}, false, nil
	}
	subject, err := identity.NewIntegrationClientSubject("https://id.syn.example/dex", "SYN-CLIENT-CARRIER-01")
	if err != nil {
		return identity.IntegrationClientStanding{}, false, err
	}
	reference, err := identity.NewCredentialReference("SYN-CREDENTIAL-REF/carrier")
	if err != nil {
		return identity.IntegrationClientStanding{}, false, err
	}
	binding, err := identity.NewIntegrationClientBinding(subject, "SYN-TENANT-01", "SYN-PARTY-CARRIER-TRUNK-01", reference, false, "SYN-BASIS")
	if err != nil {
		return identity.IntegrationClientStanding{}, false, err
	}
	interval, err := identity.NewEffectiveInterval(carrierNow.Add(-time.Hour), time.Time{})
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

// carrierAdmission 记下被问到的准入坐标。
type carrierAdmission struct {
	admitted bool
	err      error
	asked    *identity.AdmissionRequirement
}

func (admission carrierAdmission) Admits(_ context.Context, _ string, requirement identity.AdmissionRequirement, _ time.Time) (bool, error) {
	if admission.asked != nil {
		*admission.asked = requirement
	}
	return admission.admitted, admission.err
}

func carrierAuthenticator(t *testing.T, verifier identity.IntegrationClientCredentialVerifier, register carrierRegister, admission carrierAdmission) *tfaccess.IntegrationClientAuthenticator {
	t.Helper()
	minter, err := identity.NewIntegrationClientMinter(verifier, register, admission, func() time.Time { return carrierNow })
	if err != nil {
		t.Fatal(err)
	}
	built, err := tfaccess.NewIntegrationClientAuthenticator(minter)
	if err != nil {
		t.Fatal(err)
	}
	return built
}

// Covers: ADR-0202 决定二的接入面——承运答复按承运订舱答复这一类事实铸信封、按外部事实能力判准入；认出的身份带册上绑定
// 的租户与来源身份（不从载荷来）。持有别的事实类型授予的客户端答不了订舱。
func TestACarrierClientAuthenticatesWithItsBoundSource(t *testing.T) {
	var asked identity.AdmissionRequirement
	built := carrierAuthenticator(t, carrierVerifier{},
		carrierRegister{found: true, facts: []identity.ExternalFactType{identity.FactCarrierBookingAnswer}},
		carrierAdmission{admitted: true, asked: &asked})
	answerer, err := built.AuthenticateCarrierBookingAnswer(context.Background(), tfhttp.PresentedClientCredential{Token: "presented.client.token"})
	if err != nil || answerer.Tenant.String() != "SYN-TENANT-01" || answerer.Source != "SYN-PARTY-CARRIER-TRUNK-01" {
		t.Fatalf("身份 = %+v, err = %v", answerer, err)
	}
	if asked.Capability != identity.ExternalFactAdmissionCapability || asked.FactKind != "CARRIER_BOOKING_ANSWER" {
		t.Fatalf("准入坐标 = %+v", asked)
	}

	trackingOnly := carrierAuthenticator(t, carrierVerifier{},
		carrierRegister{found: true, facts: []identity.ExternalFactType{identity.FactCarrierTracking}}, carrierAdmission{admitted: true})
	if _, err := trackingOnly.AuthenticateCarrierBookingAnswer(context.Background(), tfhttp.PresentedClientCredential{Token: "t"}); !errors.Is(err, tfhttp.ErrIntegrationClientNotGranted) {
		t.Fatalf("只持承运轨迹授予：err = %v", err)
	}
}

func TestCarrierClientGradesTranslateIntoTransportFulfillmentGrades(t *testing.T) {
	granted := carrierRegister{found: true, facts: []identity.ExternalFactType{identity.FactCarrierBookingAnswer}}
	cases := map[string]struct {
		verifier  identity.IntegrationClientCredentialVerifier
		register  carrierRegister
		admission carrierAdmission
		token     string
		want      error
	}{
		"issuer parameters unset":    {identity.UnconfiguredIntegrationClientCredentialVerifier{}, granted, carrierAdmission{admitted: true}, "t", tfhttp.ErrAccessChannelNotConfigured},
		"token missing":              {carrierVerifier{}, granted, carrierAdmission{admitted: true}, "", tfhttp.ErrIntegrationClientCredentialRejected},
		"client not registered":      {carrierVerifier{}, carrierRegister{}, carrierAdmission{admitted: true}, "t", tfhttp.ErrIntegrationClientNotGranted},
		"outside admission":          {carrierVerifier{}, granted, carrierAdmission{}, "t", tfhttp.ErrOutsideAdmissionScope},
		"issuer key set unreachable": {carrierVerifier{err: identity.ErrCredentialVerifierUnavailable}, granted, carrierAdmission{admitted: true}, "t", tfhttp.ErrIdentityDependencyUnavailable},
		"register down":              {carrierVerifier{}, carrierRegister{err: errors.New("down")}, carrierAdmission{admitted: true}, "t", tfhttp.ErrIdentityDependencyUnavailable},
		"admission unreadable":       {carrierVerifier{}, granted, carrierAdmission{err: errors.New("down")}, "t", tfhttp.ErrIdentityDependencyUnavailable},
	}
	grades := []error{
		tfhttp.ErrAccessChannelNotConfigured, tfhttp.ErrIntegrationClientCredentialRejected, tfhttp.ErrIntegrationClientNotGranted,
		tfhttp.ErrOutsideAdmissionScope, tfhttp.ErrIdentityDependencyUnavailable,
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := carrierAuthenticator(t, testCase.verifier, testCase.register, testCase.admission).
				AuthenticateCarrierBookingAnswer(context.Background(), tfhttp.PresentedClientCredential{Token: testCase.token})
			for _, grade := range grades {
				if errors.Is(err, grade) != (grade == testCase.want) {
					t.Fatalf("err = %v: errors.Is(%v) = %v", err, grade, errors.Is(err, grade))
				}
			}
		})
	}
}
