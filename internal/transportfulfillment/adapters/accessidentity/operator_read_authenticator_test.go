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

func operationsReader(t *testing.T, register registryRegister) *tfaccess.OperatorOperationsReadAuthenticator {
	t.Helper()
	// 准入替身答「不在范围」：查阅读不带准入要求，这一格若被问到，下面的首条断言就会红。
	minter, err := identity.NewOperatorMinter(operatorVerifier{}, register, operatorAdmission{admitted: false}, func() time.Time { return operatorNow })
	if err != nil {
		t.Fatal(err)
	}
	built, err := tfaccess.NewOperatorOperationsReadAuthenticator(minter)
	if err != nil {
		t.Fatal(err)
	}
	return built
}

// Covers: 票 operator-channel/35——查阅读以主数据与运营查阅读能力面铸信封、不判准入范围；只持登记册配置写的操作者答
// 未授予，不带令牌答令牌不过。
func TestOperationsReadAuthenticatesWithoutAdmission(t *testing.T) {
	granted := operationsReader(t, registryRegister{faces: []identity.CapabilityFace{identity.CapabilityMasterDataAndOperationsRead}})
	tenant, err := granted.AuthenticateOperationsRead(context.Background(), "presented.operator.token")
	if err != nil || tenant.String() != operatorTenant {
		t.Fatalf("tenant = %q, err = %v", tenant.String(), err)
	}
	writer := operationsReader(t, registryRegister{faces: []identity.CapabilityFace{identity.CapabilityRegistryConfigurationWrite}})
	if _, err := writer.AuthenticateOperationsRead(context.Background(), "presented.operator.token"); !errors.Is(err, tfhttp.ErrOperatorNotGranted) {
		t.Fatalf("writer: err = %v, want ErrOperatorNotGranted", err)
	}
	if _, err := granted.AuthenticateOperationsRead(context.Background(), ""); !errors.Is(err, tfhttp.ErrOperatorCredentialRejected) {
		t.Fatalf("no token: err = %v, want ErrOperatorCredentialRejected", err)
	}
}
