package accessidentity_test

import (
	"context"
	"errors"
	"testing"

	identity "go.idp.xyz/idp-parcel/internal/accessidentity"
	whaccess "go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/accessidentity"
	warehousehttp "go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/http"
)

// receiptMinterProbe 记下铸造被问到的作业事实请求。
type receiptMinterProbe struct {
	err   error
	asked []identity.WorkFactRequest
}

func (probe *receiptMinterProbe) MintWorkFact(_ context.Context, _ identity.OperatorCredential, request identity.WorkFactRequest) (identity.WorkFactEnvelope, error) {
	probe.asked = append(probe.asked, request)
	return identity.WorkFactEnvelope{}, probe.err
}

// Covers: 票 10——收货确认在铸造侧问的准入是能力词 WORK_FACT_REGISTRATION、事实类型 RECEIPT_CONFIRMATION，
// 作业范围种类 WAREHOUSE，引用等于仓。
func TestReceiptConfirmationAsksForWorkFactAdmissionAtTheWarehouse(t *testing.T) {
	probe := &receiptMinterProbe{err: identity.ErrOutsideAdmissionScope}
	authenticator, err := whaccess.NewReceiptConfirmationAuthenticator(probe)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, err = authenticator.AuthenticateReceiptConfirmation(context.Background(), "t", "SYN-DEVICE-WH", "WAREHOUSE", "SYN-WH-01"); !errors.Is(err, warehousehttp.ErrOutsideAdmissionScope) {
		t.Fatalf("err = %v, want 不在准入范围", err)
	}
	if len(probe.asked) != 1 {
		t.Fatalf("铸了 %d 次", len(probe.asked))
	}
	request := probe.asked[0]
	if request.Admission == nil ||
		request.Admission.Capability != identity.CapabilityWorkFactRegistration.String() ||
		request.Admission.Capability != "WORK_FACT_REGISTRATION" ||
		request.Admission.FactKind != identity.WorkFactReceiptConfirmation.String() ||
		request.Admission.FactKind != "RECEIPT_CONFIRMATION" ||
		request.DeviceID != "SYN-DEVICE-WH" ||
		request.WorkScope.Kind() != identity.WorkScopeWarehouse ||
		request.WorkScope.Reference() != "SYN-WH-01" {
		t.Fatalf("铸造请求 = %+v", request)
	}
	for grade, want := range map[error]error{
		identity.ErrAccessChannelNotConfigured: warehousehttp.ErrAccessChannelNotConfigured,
		identity.ErrCredentialRejected:         warehousehttp.ErrOperatorCredentialRejected,
		identity.ErrOperatorNotGranted:         warehousehttp.ErrOperatorNotGranted,
		identity.ErrDeviceNotInUse:             warehousehttp.ErrDeviceNotInUse,
		identity.ErrAdmissionScopeUnavailable:  warehousehttp.ErrIdentityDependencyUnavailable,
		identity.ErrDeviceRegistryUnavailable:  warehousehttp.ErrIdentityDependencyUnavailable,
	} {
		graded, err := whaccess.NewReceiptConfirmationAuthenticator(&receiptMinterProbe{err: grade})
		if err != nil {
			t.Fatal(err)
		}
		if _, _, _, err := graded.AuthenticateReceiptConfirmation(context.Background(), "t", "SYN-DEVICE-WH", "WAREHOUSE", "SYN-WH-01"); !errors.Is(err, want) {
			t.Fatalf("%v：err = %v, want %v", grade, err, want)
		}
	}
	if _, err := whaccess.NewReceiptConfirmationAuthenticator(nil); err == nil {
		t.Fatal("缺铸造器也建起来了")
	}
}
