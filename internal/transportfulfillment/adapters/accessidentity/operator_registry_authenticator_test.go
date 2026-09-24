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

type registryRegister struct{ faces []identity.CapabilityFace }

func (register registryRegister) FindOperator(_ context.Context, subject identity.OperatorSubject) (identity.OperatorStanding, bool, error) {
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

func registryAuthenticator(t *testing.T, register registryRegister) *tfaccess.OperatorRegistryAuthenticator {
	t.Helper()
	// 准入替身答「不在范围」：登记写面不带准入要求，这一格若被问到，下面的首条断言就会红。
	minter, err := identity.NewOperatorMinter(operatorVerifier{}, register, operatorAdmission{admitted: false}, func() time.Time { return operatorNow })
	if err != nil {
		t.Fatal(err)
	}
	built, err := tfaccess.NewOperatorRegistryAuthenticator(minter)
	if err != nil {
		t.Fatal(err)
	}
	return built
}

// Covers: ADR-0100 决定四与票 operator-channel/04——登记写面以登记册配置写能力面铸信封、不判准入范围；只持查阅授予
// 的操作者答未授予。
func TestRegistryWriteAuthenticatesWithoutAdmission(t *testing.T) {
	granted := registryAuthenticator(t, registryRegister{faces: []identity.CapabilityFace{identity.CapabilityRegistryConfigurationWrite}})
	tenant, err := granted.AuthenticateRegistryWrite(context.Background(), "presented.operator.token")
	if err != nil || tenant.String() != operatorTenant {
		t.Fatalf("tenant = %q, err = %v", tenant.String(), err)
	}
	reader := registryAuthenticator(t, registryRegister{faces: []identity.CapabilityFace{identity.CapabilityMasterDataAndOperationsRead}})
	if _, err := reader.AuthenticateRegistryWrite(context.Background(), "presented.operator.token"); !errors.Is(err, tfhttp.ErrOperatorNotGranted) {
		t.Fatalf("reader: err = %v, want ErrOperatorNotGranted", err)
	}
	if _, err := granted.AuthenticateRegistryWrite(context.Background(), ""); !errors.Is(err, tfhttp.ErrOperatorCredentialRejected) {
		t.Fatalf("no token: err = %v, want ErrOperatorCredentialRejected", err)
	}
}
