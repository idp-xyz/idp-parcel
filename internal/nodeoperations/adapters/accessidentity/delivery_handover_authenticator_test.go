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

// 本文件证送站交接单两族的认证适配（票 ps-external-mark-relations/12）：作业事实登记面按交接单的三种事实铸信封、判准入范围；客户面
// 按「送站交接确认」这一项能力铸信封。共享接入身份能力的各格逐格译成 nodeopshttp 的格。核验方、各册与准入范围是替身。夹具全为合成
// （S 级）。

// workFactMinterProbe 记下被要求的作业事实请求，交回预设的信封或错误。
type workFactMinterProbe struct {
	envelope identity.WorkFactEnvelope
	err      error
	asked    []identity.WorkFactRequest
}

func (probe *workFactMinterProbe) MintWorkFact(_ context.Context, _ identity.OperatorCredential, request identity.WorkFactRequest) (identity.WorkFactEnvelope, error) {
	probe.asked = append(probe.asked, request)
	return probe.envelope, probe.err
}

// Covers: 票 12 地盘「作业事实登记的事实类型加开立、归入、关闭」——三种事实各带自己的准入要求（WORK_FACT_REGISTRATION，事实类型）
// 去铸，作业范围与设备照头上出示的；准入不覆盖、读不动与其余各格逐格译成 nodeopshttp 的格；词表外的事实不铸。
func TestTheHandoverWorkFactAuthenticatorAsksForEachFactsAdmission(t *testing.T) {
	for fact, kind := range map[nodeopshttp.DeliveryHandoverFact]identity.WorkFactKind{
		nodeopshttp.DeliveryHandoverFactOpening:    identity.WorkFactDeliveryHandoverNoteOpening,
		nodeopshttp.DeliveryHandoverFactAssignment: identity.WorkFactDeliveryHandoverNoteAssignment,
		nodeopshttp.DeliveryHandoverFactClosure:    identity.WorkFactDeliveryHandoverNoteClosure,
	} {
		probe := &workFactMinterProbe{err: identity.ErrOutsideAdmissionScope}
		authenticator, err := noaccess.NewWorkFactAuthenticator(probe)
		if err != nil {
			t.Fatal(err)
		}
		_, _, err = authenticator.AuthenticateDeliveryHandoverFact(context.Background(), "t", "SYN-DEVICE-01", "NODE", "SYN-NODE-SHA-HUB", fact)
		if !errors.Is(err, nodeopshttp.ErrOutsideAdmissionScope) {
			t.Fatalf("%s：err = %v, want 不在准入范围", fact, err)
		}
		if len(probe.asked) != 1 {
			t.Fatalf("%s：铸了 %d 次", fact, len(probe.asked))
		}
		request := probe.asked[0]
		if request.Admission == nil || *request.Admission != *identity.WorkFactAdmission(kind) ||
			request.DeviceID != "SYN-DEVICE-01" || request.WorkScope.Kind() != identity.WorkScopeNode || request.WorkScope.Reference() != "SYN-NODE-SHA-HUB" {
			t.Fatalf("%s：铸造请求 = %+v", fact, request)
		}
	}
	for grade, want := range map[error]error{
		identity.ErrAccessChannelNotConfigured: nodeopshttp.ErrAccessChannelNotConfigured,
		identity.ErrCredentialRejected:         nodeopshttp.ErrOperatorCredentialRejected,
		identity.ErrOperatorNotGranted:         nodeopshttp.ErrOperatorNotGranted,
		identity.ErrDeviceNotInUse:             nodeopshttp.ErrDeviceNotInUse,
		identity.ErrAdmissionScopeUnavailable:  nodeopshttp.ErrIdentityDependencyUnavailable,
		identity.ErrDeviceRegistryUnavailable:  nodeopshttp.ErrIdentityDependencyUnavailable,
	} {
		authenticator, err := noaccess.NewWorkFactAuthenticator(&workFactMinterProbe{err: grade})
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err := authenticator.AuthenticateDeliveryHandoverFact(context.Background(), "t", "SYN-DEVICE-01", "NODE", "SYN-NODE-SHA-HUB",
			nodeopshttp.DeliveryHandoverFactOpening); !errors.Is(err, want) {
			t.Fatalf("%v：err = %v, want %v", grade, err, want)
		}
	}
	probe := &workFactMinterProbe{}
	authenticator, err := noaccess.NewWorkFactAuthenticator(probe)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := authenticator.AuthenticateDeliveryHandoverFact(context.Background(), "t", "SYN-DEVICE-01", "NODE", "SYN-NODE-SHA-HUB", "NODE_RECEPTION"); err == nil || len(probe.asked) != 0 {
		t.Fatalf("词表外的事实：err = %v，铸了 %d 次", err, len(probe.asked))
	}
	if _, _, err := authenticator.AuthenticateDeliveryHandoverFact(context.Background(), "t", "SYN-DEVICE-01", "ZONE", "SYN-NODE-SHA-HUB", nodeopshttp.DeliveryHandoverFactOpening); !errors.Is(err, nodeopshttp.ErrMalformedRequest) {
		t.Fatalf("作业范围种类认不出：err = %v", err)
	}
}

func customerHandoverAuthenticator(t *testing.T, verifier customerVerifier, register customerRegister, admission customerAdmission) *noaccess.CustomerHandoverConclusionAuthenticator {
	t.Helper()
	minter, err := identity.NewCustomerClientMinter(verifier, register, admission, func() time.Time { return customerNow })
	if err != nil {
		t.Fatal(err)
	}
	authenticator, err := noaccess.NewCustomerHandoverConclusionAuthenticator(minter)
	if err != nil {
		t.Fatal(err)
	}
	return authenticator
}

// Covers: `AT-NO-073`（认证半边）与票 12 完成判据「信封各格」的客户面——持「送站交接确认」授予、落在准入范围内的客户端，交回的客户
// 身份整组取自册上的绑定；各格逐格译成 nodeopshttp 的格，只持预报两格的客户端答未授予；缺接入请求标识译成坏载荷。
func TestTheCustomerHandoverAuthenticatorTranslatesTheEnvelopeAndEveryGrade(t *testing.T) {
	granted := customerRegister{capabilities: []identity.CustomerCapability{identity.CustomerCapabilityDeliveryHandoverConfirmation}}
	authenticate := func(authenticator *noaccess.CustomerHandoverConclusionAuthenticator, id func() (string, error)) (nodeopshttp.CustomerIdentity, error) {
		return authenticator.AuthenticateDeliveryHandoverConclusion(context.Background(), nodeopshttp.PresentedCustomerCredential{Token: "t"}, id)
	}
	customer, err := authenticate(customerHandoverAuthenticator(t, customerVerifier{}, granted, customerAdmission{admitted: true}),
		func() (string, error) { return " SYN-ACCESS-REQUEST-HN-0001 ", nil })
	if err != nil {
		t.Fatal(err)
	}
	if customer.Tenant.String() != customerTenant || customer.CustomerAccount.String() != "SYN-ACCOUNT-01" ||
		customer.Channel != customerIssuer+"#SYN-CUSTOMER-CLIENT-01" || customer.RequestKey != "SYN-ACCESS-REQUEST-HN-0001" {
		t.Fatalf("客户身份 = %+v", customer)
	}
	preAdviceOnly := customerRegister{capabilities: []identity.CustomerCapability{
		identity.CustomerCapabilityDeliveryPreAdviceSubmission, identity.CustomerCapabilityDeliveryPreAdviceWithdrawal,
	}}
	for name, testCase := range map[string]struct {
		verifier  customerVerifier
		register  customerRegister
		admission customerAdmission
		id        func() (string, error)
		want      error
	}{
		"issuer unset":         {customerVerifier{err: identity.ErrAccessChannelNotConfigured}, granted, customerAdmission{admitted: true}, nil, nodeopshttp.ErrAccessChannelNotConfigured},
		"token rejected":       {customerVerifier{err: identity.ErrCredentialRejected}, granted, customerAdmission{admitted: true}, nil, nodeopshttp.ErrCustomerClientCredentialRejected},
		"only pre-advice":      {customerVerifier{}, preAdviceOnly, customerAdmission{admitted: true}, nil, nodeopshttp.ErrCustomerClientNotGranted},
		"outside admission":    {customerVerifier{}, granted, customerAdmission{}, nil, nodeopshttp.ErrOutsideAdmissionScope},
		"admission unreadable": {customerVerifier{}, granted, customerAdmission{err: errors.New("unreachable")}, nil, nodeopshttp.ErrIdentityDependencyUnavailable},
		"no access request id": {customerVerifier{}, granted, customerAdmission{admitted: true}, func() (string, error) { return " ", nil }, nodeopshttp.ErrMalformedRequest},
	} {
		t.Run(name, func(t *testing.T) {
			id := testCase.id
			if id == nil {
				id = func() (string, error) { return "SYN-ACCESS-REQUEST-HN-0001", nil }
			}
			if _, err := authenticate(customerHandoverAuthenticator(t, testCase.verifier, testCase.register, testCase.admission), id); !errors.Is(err, testCase.want) {
				t.Fatalf("err = %v, want %v", err, testCase.want)
			}
		})
	}
	if _, err := noaccess.NewCustomerHandoverConclusionAuthenticator(nil); err == nil {
		t.Fatal("缺铸造器也建起来了")
	}
}
