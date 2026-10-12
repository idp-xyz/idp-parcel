package accessidentity_test

import (
	"context"
	"errors"
	"testing"
	"time"

	identity "go.idp.xyz/idp-parcel/internal/accessidentity"
	ppaccess "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/accessidentity"
	pricinghttp "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/http"
)

func operationsReader(t *testing.T, verifier identity.OperatorCredentialVerifier, register operatorRegister) *ppaccess.OperatorOperationsReadAuthenticator {
	t.Helper()
	minter, err := identity.NewOperatorMinter(verifier, register, identity.UnconfiguredAdmissionScope{}, func() time.Time { return operatorNow })
	if err != nil {
		t.Fatal(err)
	}
	built, err := ppaccess.NewOperatorOperationsReadAuthenticator(minter)
	if err != nil {
		t.Fatal(err)
	}
	return built
}

// Covers: ADR-0152 决定六与票 operator-channel/06——查阅读面认证方以主数据与运营查阅读铸信封，认证到操作者绑定的租户。
// 准入范围是未配置那一只：这一面若判了准入，这里就会答不在准入范围。
func TestOperationsReaderAuthenticatesToTheBoundTenantWithoutAnAdmissionCheck(t *testing.T) {
	built := operationsReader(t, operatorVerifier{}, operatorRegister{faces: []identity.CapabilityFace{identity.CapabilityMasterDataAndOperationsRead}})
	tenant, err := built.AuthenticateOperationsRead(context.Background(), "presented.operator.token")
	if err != nil || tenant.String() != operatorTenant {
		t.Fatalf("tenant = %q, err = %v", tenant, err)
	}
}

// Covers: 同上——接入身份能力的格逐格译成 pricinghttp 的格。只持登记册配置写的操作者在这一面答未授予：两格是册上平列的
// 授予，持写不等于持读。
func TestOperationsReadGradesTranslateIntoPricingGrades(t *testing.T) {
	reader := operatorRegister{faces: []identity.CapabilityFace{identity.CapabilityMasterDataAndOperationsRead}}
	cases := map[string]struct {
		verifier identity.OperatorCredentialVerifier
		register operatorRegister
		token    string
		want     error
	}{
		"issuer parameters unset": {identity.UnconfiguredOperatorCredentialVerifier{}, reader, "t", pricinghttp.ErrAccessChannelNotConfigured},
		"token missing":           {operatorVerifier{}, reader, "", pricinghttp.ErrOperatorCredentialRejected},
		"only a write grant":      {operatorVerifier{}, operatorRegister{faces: []identity.CapabilityFace{identity.CapabilityRegistryConfigurationWrite}}, "t", pricinghttp.ErrOperatorNotGranted},
		"issuer key set down":     {operatorVerifier{err: identity.ErrCredentialVerifierUnavailable}, reader, "t", pricinghttp.ErrIdentityDependencyUnavailable},
		"operator register down":  {operatorVerifier{}, operatorRegister{err: errors.New("down")}, "t", pricinghttp.ErrIdentityDependencyUnavailable},
	}
	grades := []error{pricinghttp.ErrAccessChannelNotConfigured, pricinghttp.ErrOperatorCredentialRejected, pricinghttp.ErrOperatorNotGranted, pricinghttp.ErrIdentityDependencyUnavailable}
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
