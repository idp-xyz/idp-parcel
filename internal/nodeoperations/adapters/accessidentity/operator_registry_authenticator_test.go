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

func registryWriter(t *testing.T, verifier identity.OperatorCredentialVerifier, register operatorRegister) *noaccess.OperatorRegistryAuthenticator {
	t.Helper()
	minter, err := identity.NewOperatorMinter(verifier, register, identity.UnconfiguredAdmissionScope{}, func() time.Time { return operatorNow })
	if err != nil {
		t.Fatal(err)
	}
	built, err := noaccess.NewOperatorRegistryAuthenticator(minter)
	if err != nil {
		t.Fatal(err)
	}
	return built
}

// Covers: ADR-0100 决定四与 ADR-0085——节点作业的登记写面以登记册配置写能力面铸信封、认证到操作者绑定的租户、不判准入范围
// （准入范围是未配置那一只，判了就会答不在准入范围）；只持查阅授予答未授予，没令牌答令牌不过，发行方未配置答未配置，操作者册
// 读不动答身份依赖读不动。
func TestRegistryWriterAuthenticatesWithoutAnAdmissionCheck(t *testing.T) {
	granted := registryWriter(t, operatorVerifier{}, operatorRegister{faces: []identity.CapabilityFace{identity.CapabilityRegistryConfigurationWrite}})
	tenant, err := granted.AuthenticateRegistryWrite(context.Background(), "presented.operator.token")
	if err != nil || tenant.String() != operatorTenant {
		t.Fatalf("tenant = %q, err = %v", tenant.String(), err)
	}

	for name, testCase := range map[string]struct {
		authenticator *noaccess.OperatorRegistryAuthenticator
		token         string
		want          error
	}{
		"只持查阅授予": {registryWriter(t, operatorVerifier{}, operatorRegister{faces: []identity.CapabilityFace{identity.CapabilityMasterDataAndOperationsRead}}),
			"presented.operator.token", nodeopshttp.ErrOperatorNotGranted},
		"没令牌":    {granted, "", nodeopshttp.ErrOperatorCredentialRejected},
		"发行方未配置": {registryWriter(t, identity.UnconfiguredOperatorCredentialVerifier{}, operatorRegister{}), "presented.operator.token", nodeopshttp.ErrAccessChannelNotConfigured},
		"操作者册读不动": {registryWriter(t, operatorVerifier{}, operatorRegister{err: errors.New("register unreachable")}),
			"presented.operator.token", nodeopshttp.ErrIdentityDependencyUnavailable},
	} {
		if _, err := testCase.authenticator.AuthenticateRegistryWrite(context.Background(), testCase.token); !errors.Is(err, testCase.want) {
			t.Fatalf("%s: err = %v, want %v", name, err, testCase.want)
		}
	}
	if _, err := noaccess.NewOperatorRegistryAuthenticator(nil); err == nil {
		t.Fatal("缺铸造器的认证方构造得出来")
	}
}
