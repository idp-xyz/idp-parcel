package accessidentity_test

import (
	"context"
	"errors"
	"testing"
	"time"

	identity "go.idp.xyz/idp-parcel/internal/accessidentity"
	saaccess "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/accessidentity"
	settlementhttp "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/http"
)

type decisionVerifier struct{}

func (decisionVerifier) VerifyOperatorCredential(_ context.Context, credential identity.OperatorCredential) (identity.OperatorSubject, error) {
	if credential.Token() == "" {
		return identity.OperatorSubject{}, identity.ErrCredentialRejected
	}
	return identity.NewOperatorSubject("https://id.syn.example/dex", "SYN-FINANCE-01")
}

// decisionRegister 是操作者册的替身：主体绑演示租户，逐种持有给定的运营决定授予。
type decisionRegister struct{ kinds []identity.DecisionKind }

func (register decisionRegister) FindOperator(_ context.Context, subject identity.OperatorSubject) (identity.OperatorStanding, bool, error) {
	binding, err := identity.NewOperatorBinding(subject, "SYN-TENANT-01", "SYN-BASIS-BINDING")
	if err != nil {
		return identity.OperatorStanding{}, false, err
	}
	interval, err := identity.NewEffectiveInterval(fundsNow.Add(-time.Hour), time.Time{})
	if err != nil {
		return identity.OperatorStanding{}, false, err
	}
	var grants []identity.RecordedGrant
	for index, kind := range register.kinds {
		grant, err := identity.NewOperationDecisionGrant("SYN-TENANT-01", "SYN-GRANT-D"+string(rune('A'+index)), subject, kind, interval, "SYN-BASIS-GRANT")
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

// decisionAdmission 只放行列出的事实类型，记下被问的要求。
type decisionAdmission struct {
	admitted map[string]bool
	asked    *[]identity.AdmissionRequirement
}

func (scope decisionAdmission) Admits(_ context.Context, _ string, requirement identity.AdmissionRequirement, _ time.Time) (bool, error) {
	*scope.asked = append(*scope.asked, requirement)
	return scope.admitted[requirement.FactKind], nil
}

func decisionAuthenticator(t *testing.T, register decisionRegister, scope identity.AdmissionScope) *saaccess.OperatorDecisionAuthenticator {
	t.Helper()
	minter, err := identity.NewOperatorMinter(decisionVerifier{}, register, scope, func() time.Time { return fundsNow })
	if err != nil {
		t.Fatal(err)
	}
	built, err := saaccess.NewOperatorDecisionAuthenticator(minter)
	if err != nil {
		t.Fatal(err)
	}
	return built
}

// Covers: AT-SA-201（认证半边）——按外部资金事实采用这一种决定铸信封并判准入（能力 OPERATION_DECISION、事实类型取决定种类）；只持
// 监管凭证登记的操作者答无此种授予；准入范围没登答不在准入范围；没令牌答令牌不过。交回的操作者是「发行方#sub」。
func TestFundsAdoptionAuthenticatesItsOwnKindAndAsksTheAdmissionScope(t *testing.T) {
	var asked []identity.AdmissionRequirement
	built := decisionAuthenticator(t, decisionRegister{kinds: []identity.DecisionKind{identity.DecisionExternalFundsFactAdoption}},
		decisionAdmission{admitted: map[string]bool{"EXTERNAL_FUNDS_FACT_ADOPTION": true}, asked: &asked})

	operator, err := built.AuthenticateOperatorDecision(context.Background(), "presented.operator.token", settlementhttp.OperatorDecisionExternalFundsFactAdoption)
	if err != nil || operator.Tenant.String() != "SYN-TENANT-01" || operator.Operator != "https://id.syn.example/dex#SYN-FINANCE-01" {
		t.Fatalf("operator = %+v err = %v", operator, err)
	}
	if len(asked) != 1 || asked[0].Capability != "OPERATION_DECISION" || asked[0].FactKind != "EXTERNAL_FUNDS_FACT_ADOPTION" {
		t.Fatalf("准入要求 = %+v", asked)
	}
	if _, err := built.AuthenticateOperatorDecision(context.Background(), "", settlementhttp.OperatorDecisionExternalFundsFactAdoption); !errors.Is(err, settlementhttp.ErrOperatorCredentialRejected) {
		t.Fatalf("没令牌：err = %v", err)
	}

	credentialOnly := decisionAuthenticator(t, decisionRegister{kinds: []identity.DecisionKind{identity.DecisionRegulatoryCredentialRegistration}},
		decisionAdmission{admitted: map[string]bool{"EXTERNAL_FUNDS_FACT_ADOPTION": true}, asked: &asked})
	if _, err := credentialOnly.AuthenticateOperatorDecision(context.Background(), "presented.operator.token", settlementhttp.OperatorDecisionExternalFundsFactAdoption); !errors.Is(err, settlementhttp.ErrOperatorNotGranted) {
		t.Fatalf("只持凭证登记的去采用资金事实：err = %v", err)
	}
	unadmitted := decisionAuthenticator(t, decisionRegister{kinds: []identity.DecisionKind{identity.DecisionExternalFundsFactAdoption}},
		decisionAdmission{admitted: map[string]bool{}, asked: &asked})
	if _, err := unadmitted.AuthenticateOperatorDecision(context.Background(), "presented.operator.token", settlementhttp.OperatorDecisionExternalFundsFactAdoption); !errors.Is(err, settlementhttp.ErrOutsideAdmissionScope) {
		t.Fatalf("准入没登：err = %v", err)
	}
}
