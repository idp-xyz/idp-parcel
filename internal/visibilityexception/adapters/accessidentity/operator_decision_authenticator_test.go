package accessidentity_test

import (
	"context"
	"errors"
	"testing"
	"time"

	identity "go.idp.xyz/idp-parcel/internal/accessidentity"
	veaccess "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/accessidentity"
	visibilityhttp "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/http"
)

// decisionRegister 是一本只登了「运营决定」授予的操作者册：每种决定一行授予。
type decisionRegister struct {
	kinds []identity.DecisionKind
}

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
		grant, err := identity.NewOperationDecisionGrant(operatorTenant, "SYN-DECISION-"+string(rune('A'+index)), subject, kind, interval, "SYN-BASIS-GRANT")
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

type admittingEverything struct{}

func (admittingEverything) Admits(context.Context, string, identity.AdmissionRequirement, time.Time) (bool, error) {
	return true, nil
}

func decisionAuthenticator(t *testing.T, register decisionRegister, admission identity.AdmissionScope) *veaccess.OperatorDecisionAuthenticator {
	t.Helper()
	minter, err := identity.NewOperatorMinter(operatorVerifier{}, register, admission, func() time.Time { return operatorNow })
	if err != nil {
		t.Fatal(err)
	}
	built, err := veaccess.NewOperatorDecisionAuthenticator(minter)
	if err != nil {
		t.Fatal(err)
	}
	return built
}

// Covers: 异常案件三种决定的认证——持有接单授予的操作者按接单认证，交回册上绑定的租户与「发行方#sub」形的操作者；同一
// 个人拿接单的授予认证关闭，答未授予（授予粒度等于权力粒度）；运营决定带准入要求，准入范围没登时答不在准入范围。
func TestACaseDecisionNeedsItsOwnGrantAndAnAdmittedWrite(t *testing.T) {
	onlyTakeUp := decisionRegister{kinds: []identity.DecisionKind{identity.DecisionExceptionCaseTakeUp}}

	granted := decisionAuthenticator(t, onlyTakeUp, admittingEverything{})
	operator, err := granted.AuthenticateOperatorDecision(context.Background(), "presented.operator.token", visibilityhttp.OperatorDecisionExceptionCaseTakeUp)
	if err != nil || operator.Tenant.String() != operatorTenant || operator.Operator != "https://id.syn.example/dex#SYN-OPERATOR-01" {
		t.Fatalf("operator = %+v, err = %v", operator, err)
	}
	if _, err := granted.AuthenticateOperatorDecision(context.Background(), "presented.operator.token", visibilityhttp.OperatorDecisionExceptionCaseClosure); !errors.Is(err, visibilityhttp.ErrOperatorNotGranted) {
		t.Fatalf("err = %v, want ErrOperatorNotGranted——能接单不等于能关闭", err)
	}

	outside := decisionAuthenticator(t, onlyTakeUp, identity.UnconfiguredAdmissionScope{})
	if _, err := outside.AuthenticateOperatorDecision(context.Background(), "presented.operator.token", visibilityhttp.OperatorDecisionExceptionCaseTakeUp); !errors.Is(err, visibilityhttp.ErrOutsideAdmissionScope) {
		t.Fatalf("err = %v, want ErrOutsideAdmissionScope", err)
	}
}
