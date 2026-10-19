package accessidentity_test

import (
	"context"
	"errors"
	"testing"
	"time"

	identity "go.idp.xyz/idp-parcel/internal/accessidentity"
	noaccess "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/accessidentity"
	nodeopshttp "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/http"
)

const operatorTenant = "SYN-TENANT-01"

var operatorNow = time.Date(2026, 10, 16, 3, 0, 0, 0, time.UTC)

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

func operationsReader(t *testing.T, verifier identity.OperatorCredentialVerifier, register operatorRegister) *noaccess.OperatorOperationsReadAuthenticator {
	t.Helper()
	minter, err := identity.NewOperatorMinter(verifier, register, identity.UnconfiguredAdmissionScope{}, func() time.Time { return operatorNow })
	if err != nil {
		t.Fatal(err)
	}
	built, err := noaccess.NewOperatorOperationsReadAuthenticator(minter)
	if err != nil {
		t.Fatal(err)
	}
	return built
}

// Covers: 票 operator-channel/35——查阅读认证方以主数据与运营查阅读铸信封，认证到操作者绑定的租户。准入范围是未配置那一只：
// 这一面若判了准入，这里就会答不在准入范围。
func TestOperationsReaderAuthenticatesToTheBoundTenantWithoutAnAdmissionCheck(t *testing.T) {
	built := operationsReader(t, operatorVerifier{}, operatorRegister{faces: []identity.CapabilityFace{identity.CapabilityMasterDataAndOperationsRead}})
	tenant, err := built.AuthenticateOperationsRead(context.Background(), "presented.operator.token")
	if err != nil || tenant.String() != operatorTenant {
		t.Fatalf("tenant = %q, err = %v", tenant, err)
	}
}

// Covers: 同上——接入身份能力的格逐格译成 nodeopshttp 的格。只持登记册配置写的操作者在这一面答未授予：两格是册上平列的
// 授予，持写不等于持读。
func TestOperationsReadGradesTranslateIntoNodeOperationsGrades(t *testing.T) {
	reader := operatorRegister{faces: []identity.CapabilityFace{identity.CapabilityMasterDataAndOperationsRead}}
	cases := map[string]struct {
		verifier identity.OperatorCredentialVerifier
		register operatorRegister
		token    string
		want     error
	}{
		"issuer parameters unset": {identity.UnconfiguredOperatorCredentialVerifier{}, reader, "t", nodeopshttp.ErrAccessChannelNotConfigured},
		"token missing":           {operatorVerifier{}, reader, "", nodeopshttp.ErrOperatorCredentialRejected},
		"only a write grant":      {operatorVerifier{}, operatorRegister{faces: []identity.CapabilityFace{identity.CapabilityRegistryConfigurationWrite}}, "t", nodeopshttp.ErrOperatorNotGranted},
		"issuer key set down":     {operatorVerifier{err: identity.ErrCredentialVerifierUnavailable}, reader, "t", nodeopshttp.ErrIdentityDependencyUnavailable},
		"operator register down":  {operatorVerifier{}, operatorRegister{err: errors.New("down")}, "t", nodeopshttp.ErrIdentityDependencyUnavailable},
	}
	grades := []error{nodeopshttp.ErrAccessChannelNotConfigured, nodeopshttp.ErrOperatorCredentialRejected, nodeopshttp.ErrOperatorNotGranted, nodeopshttp.ErrIdentityDependencyUnavailable}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := operationsReader(t, testCase.verifier, testCase.register).AuthenticateOperationsRead(context.Background(), testCase.token)
			for _, grade := range grades {
				if errors.Is(err, grade) != (grade == testCase.want) {
					t.Fatalf("err = %v: errors.Is(%v) = %v", err, grade, errors.Is(err, grade))
				}
			}
		})
	}
}
