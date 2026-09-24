package accessidentity_test

import (
	"context"
	"errors"
	"testing"
	"time"

	identity "go.idp.xyz/idp-parcel/internal/accessidentity"
	pcaccess "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/accessidentity"
	commercialhttp "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/http"
)

const operatorTenant = "SYN-TENANT-01"

var operatorNow = time.Date(2026, 9, 25, 3, 0, 0, 0, time.UTC)

type operatorVerifier struct{ err error }

func (verifier operatorVerifier) VerifyOperatorCredential(_ context.Context, credential identity.OperatorCredential) (identity.OperatorSubject, error) {
	if verifier.err != nil {
		return identity.OperatorSubject{}, verifier.err
	}
	if credential.Token() == "" {
		return identity.OperatorSubject{}, identity.ErrCredentialRejected
	}
	return identity.NewOperatorSubject("https://id.syn.example/dex", "SYN-OPERATOR-01")
}

type operatorRegister struct {
	faces []identity.CapabilityFace
	err   error
}

func (register operatorRegister) FindOperator(_ context.Context, subject identity.OperatorSubject) (identity.OperatorStanding, bool, error) {
	if register.err != nil {
		return identity.OperatorStanding{}, false, register.err
	}
	binding, err := identity.NewOperatorBinding(subject, operatorTenant, "SYN-BASIS-BINDING")
	if err != nil {
		return identity.OperatorStanding{}, false, err
	}
	interval, err := identity.NewEffectiveInterval(operatorNow.Add(-time.Hour), time.Time{})
	if err != nil {
		return identity.OperatorStanding{}, false, err
	}
	var grants []identity.RecordedGrant
	for index, face := range register.faces {
		grant, err := identity.NewOperatorGrant(operatorTenant, "SYN-GRANT-"+string(rune('A'+index)), subject, face, interval, "SYN-BASIS-GRANT")
		if err != nil {
			return identity.OperatorStanding{}, false, err
		}
		recorded, err := identity.NewRecordedGrant(grant, nil)
		if err != nil {
			return identity.OperatorStanding{}, false, err
		}
		grants = append(grants, recorded)
	}
	standing, err := identity.NewOperatorStanding(binding, grants)
	return standing, err == nil, err
}

func authenticator(t *testing.T, verifier identity.OperatorCredentialVerifier, register operatorRegister) *pcaccess.OperatorRegistryAuthenticator {
	t.Helper()
	minter, err := identity.NewOperatorMinter(verifier, register, identity.UnconfiguredAdmissionScope{}, func() time.Time { return operatorNow })
	if err != nil {
		t.Fatal(err)
	}
	built, err := pcaccess.NewOperatorRegistryAuthenticator(minter)
	if err != nil {
		t.Fatal(err)
	}
	return built
}

func TestRegistryWriterAuthenticatesToTheBoundTenantWithoutAnAdmissionCheck(t *testing.T) {
	// 准入范围是未配置那一只：登记写面若判了准入，这里就会答不在准入范围。
	built := authenticator(t, operatorVerifier{}, operatorRegister{faces: []identity.CapabilityFace{identity.CapabilityRegistryConfigurationWrite}})
	tenant, err := built.AuthenticateRegistryWrite(context.Background(), "presented.operator.token")
	if err != nil || tenant.String() != operatorTenant {
		t.Fatalf("tenant = %q, err = %v", tenant.String(), err)
	}
}

func TestRegistryGradesTranslateIntoCommercialGrades(t *testing.T) {
	writer := operatorRegister{faces: []identity.CapabilityFace{identity.CapabilityRegistryConfigurationWrite}}
	cases := map[string]struct {
		verifier identity.OperatorCredentialVerifier
		register operatorRegister
		token    string
		want     error
	}{
		"issuer parameters unset": {identity.UnconfiguredOperatorCredentialVerifier{}, writer, "t", commercialhttp.ErrAccessChannelNotConfigured},
		"token missing":           {operatorVerifier{}, writer, "", commercialhttp.ErrOperatorCredentialRejected},
		"only a read grant":       {operatorVerifier{}, operatorRegister{faces: []identity.CapabilityFace{identity.CapabilityMasterDataAndOperationsRead}}, "t", commercialhttp.ErrOperatorNotGranted},
		"issuer key set down":     {operatorVerifier{err: identity.ErrCredentialVerifierUnavailable}, writer, "t", commercialhttp.ErrIdentityDependencyUnavailable},
		"operator register down":  {operatorVerifier{}, operatorRegister{err: errors.New("down")}, "t", commercialhttp.ErrIdentityDependencyUnavailable},
	}
	grades := []error{commercialhttp.ErrAccessChannelNotConfigured, commercialhttp.ErrOperatorCredentialRejected, commercialhttp.ErrOperatorNotGranted, commercialhttp.ErrIdentityDependencyUnavailable}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := authenticator(t, testCase.verifier, testCase.register).AuthenticateRegistryWrite(context.Background(), testCase.token)
			for _, grade := range grades {
				if errors.Is(err, grade) != (grade == testCase.want) {
					t.Fatalf("err = %v: errors.Is(%v) = %v", err, grade, errors.Is(err, grade))
				}
			}
		})
	}
}
