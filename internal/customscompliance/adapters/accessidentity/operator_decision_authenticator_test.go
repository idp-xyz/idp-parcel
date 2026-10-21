package accessidentity_test

import (
	"context"
	"errors"
	"testing"
	"time"

	identity "go.idp.xyz/idp-parcel/internal/accessidentity"
	ccaccess "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/accessidentity"
	customshttp "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/http"
)

// decisionRegister 是操作者册的替身：主体绑演示租户，逐种持有给定的运营决定授予。
type decisionRegister struct{ kinds []identity.DecisionKind }

func (register decisionRegister) FindOperator(_ context.Context, subject identity.OperatorSubject) (identity.OperatorStanding, bool, error) {
	binding, err := identity.NewOperatorBinding(subject, operatorTenant, "SYN-BASIS-BINDING")
	if err != nil {
		return identity.OperatorStanding{}, false, err
	}
	interval, err := identity.NewEffectiveInterval(operatorNow.Add(-time.Hour), time.Time{})
	if err != nil {
		return identity.OperatorStanding{}, false, err
	}
	var grants []identity.RecordedGrant
	for index, kind := range register.kinds {
		grant, err := identity.NewOperationDecisionGrant(operatorTenant, "SYN-GRANT-D"+string(rune('A'+index)), subject, kind, interval, "SYN-BASIS-GRANT")
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

// admission 是准入范围的替身：只放行列出的事实类型，记下被问的要求。
type admission struct {
	admitted map[string]bool
	asked    *[]identity.AdmissionRequirement
}

func (scope admission) Admits(_ context.Context, _ string, requirement identity.AdmissionRequirement, _ time.Time) (bool, error) {
	if scope.asked != nil {
		*scope.asked = append(*scope.asked, requirement)
	}
	return scope.admitted[requirement.FactKind], nil
}

func decisionAuthenticator(t *testing.T, register decisionRegister, scope identity.AdmissionScope) *ccaccess.OperatorDecisionAuthenticator {
	t.Helper()
	minter, err := identity.NewOperatorMinter(operatorVerifier{}, register, scope, func() time.Time { return operatorNow })
	if err != nil {
		t.Fatal(err)
	}
	built, err := ccaccess.NewOperatorDecisionAuthenticator(minter)
	if err != nil {
		t.Fatal(err)
	}
	return built
}

// Covers: AT-CC-445（认证半边）——按本口那一种决定铸信封并判准入（能力 OPERATION_DECISION、事实类型取决定种类）；持提交授权确认的
// 操作者在提交执行口答无此种授予；准入范围没登这一种答不在准入范围。交回的操作者是「发行方#sub」。
func TestCustomsDecisionsAuthenticatePerKindAndAskTheAdmissionScope(t *testing.T) {
	var asked []identity.AdmissionRequirement
	built := decisionAuthenticator(t,
		decisionRegister{kinds: []identity.DecisionKind{identity.DecisionCustomsSubmissionAuthorization, identity.DecisionCustomsCaseEstablishment}},
		admission{admitted: map[string]bool{"CUSTOMS_SUBMISSION_AUTHORIZATION": true}, asked: &asked})

	operator, err := built.AuthenticateOperatorDecision(context.Background(), "presented.operator.token", customshttp.OperatorDecisionSubmissionAuthorization)
	if err != nil || operator.Tenant.String() != operatorTenant || operator.Operator != "https://id.syn.example/dex#SYN-OPERATOR-01" {
		t.Fatalf("operator = %+v err = %v", operator, err)
	}
	if len(asked) != 1 || asked[0].Capability != "OPERATION_DECISION" || asked[0].FactKind != "CUSTOMS_SUBMISSION_AUTHORIZATION" {
		t.Fatalf("准入要求 = %+v", asked)
	}

	if _, err := built.AuthenticateOperatorDecision(context.Background(), "presented.operator.token", customshttp.OperatorDecisionDeclarationSubmission); !errors.Is(err, customshttp.ErrOperatorNotGranted) {
		t.Fatalf("持批准的去提交：err = %v", err)
	}
	if _, err := built.AuthenticateOperatorDecision(context.Background(), "presented.operator.token", customshttp.OperatorDecisionCaseEstablishment); !errors.Is(err, customshttp.ErrOutsideAdmissionScope) {
		t.Fatalf("准入没登建案：err = %v", err)
	}
	if _, err := built.AuthenticateOperatorDecision(context.Background(), "", customshttp.OperatorDecisionCaseEstablishment); !errors.Is(err, customshttp.ErrOperatorCredentialRejected) {
		t.Fatalf("没令牌：err = %v", err)
	}
}
