package accessidentity_test

import (
	"context"
	"errors"
	"testing"

	identity "go.idp.xyz/idp-parcel/internal/accessidentity"
	customshttp "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/http"
)

// Covers: AT-CC-447（认证半边）——监管凭证登记按自己那一种决定铸信封并判准入；只持外部资金事实采用的操作者答无此种授予，持关务
// 三种决定的也一样——凭证登记分开授予。
func TestCredentialRegistrationAuthenticatesItsOwnKind(t *testing.T) {
	var asked []identity.AdmissionRequirement
	scope := admission{admitted: map[string]bool{"REGULATORY_CREDENTIAL_REGISTRATION": true}, asked: &asked}
	granted := decisionAuthenticator(t, decisionRegister{kinds: []identity.DecisionKind{identity.DecisionRegulatoryCredentialRegistration}}, scope)
	operator, err := granted.AuthenticateOperatorDecision(context.Background(), "presented.operator.token", customshttp.OperatorDecisionRegulatoryCredentialRegistration)
	if err != nil || operator.Tenant.String() != operatorTenant || operator.Operator != "https://id.syn.example/dex#SYN-OPERATOR-01" {
		t.Fatalf("operator = %+v err = %v", operator, err)
	}
	if len(asked) != 1 || asked[0].Capability != "OPERATION_DECISION" || asked[0].FactKind != "REGULATORY_CREDENTIAL_REGISTRATION" {
		t.Fatalf("准入要求 = %+v", asked)
	}

	for _, kinds := range [][]identity.DecisionKind{
		{identity.DecisionExternalFundsFactAdoption},
		{identity.DecisionCustomsCaseEstablishment, identity.DecisionCustomsSubmissionAuthorization, identity.DecisionCustomsDeclarationSubmission},
	} {
		other := decisionAuthenticator(t, decisionRegister{kinds: kinds}, scope)
		if _, err := other.AuthenticateOperatorDecision(context.Background(), "presented.operator.token",
			customshttp.OperatorDecisionRegulatoryCredentialRegistration); !errors.Is(err, customshttp.ErrOperatorNotGranted) {
			t.Fatalf("持 %v 去登凭证：err = %v", kinds, err)
		}
	}
}
