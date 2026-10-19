package accessidentity_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	identity "go.idp.xyz/idp-parcel/internal/accessidentity"
	noaccess "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/accessidentity"
	nodeopshttp "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/http"
)

// decisionRegister 是操作者册的替身：主体绑在 operatorTenant 上，持有 kinds 列出的各种运营决定授予。
type decisionRegister struct {
	kinds []identity.DecisionKind
	err   error
}

func (register decisionRegister) FindOperator(_ context.Context, subject identity.OperatorSubject) (identity.OperatorStanding, bool, error) {
	if register.err != nil {
		return identity.OperatorStanding{}, false, register.err
	}
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
		grant, err := identity.NewOperationDecisionGrant(operatorTenant, fmt.Sprintf("SYN-GRANT-%d", index), subject, kind, interval, "SYN-BASIS-GRANT")
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

type decisionAdmission struct {
	admitted bool
	asked    *[]identity.AdmissionRequirement
	err      error
}

func (admission decisionAdmission) Admits(_ context.Context, _ string, requirement identity.AdmissionRequirement, _ time.Time) (bool, error) {
	if admission.asked != nil {
		*admission.asked = append(*admission.asked, requirement)
	}
	return admission.admitted, admission.err
}

func decisionAuthenticator(t *testing.T, verifier identity.OperatorCredentialVerifier, register decisionRegister, admission decisionAdmission) *noaccess.OperatorDecisionAuthenticator {
	t.Helper()
	minter, err := identity.NewOperatorMinter(verifier, register, admission, func() time.Time { return operatorNow })
	if err != nil {
		t.Fatal(err)
	}
	built, err := noaccess.NewOperatorDecisionAuthenticator(minter)
	if err != nil {
		t.Fatal(err)
	}
	return built
}

// Covers: 人工重核口归「运营决定」能力面（ADR-0151 决定一）：持`待识别重核`授予的操作者认证到绑定的租户，准入范围按（运营决定，
// 待识别重核）判（ADR-0151 决定三）；只持别的决定种类答无授予。
func TestARecheckOperatorAuthenticatesForThePendingIdentityRecheckDecision(t *testing.T) {
	var asked []identity.AdmissionRequirement
	granted := decisionRegister{kinds: []identity.DecisionKind{identity.DecisionPendingIdentityRecheck}}
	built := decisionAuthenticator(t, operatorVerifier{}, granted, decisionAdmission{admitted: true, asked: &asked})
	tenant, err := built.AuthenticateOperatorDecision(context.Background(), "presented.operator.token", nodeopshttp.OperatorDecisionPendingIdentityRecheck)
	if err != nil || tenant.String() != operatorTenant {
		t.Fatalf("tenant = %q, err = %v", tenant, err)
	}
	if len(asked) != 1 || asked[0].Capability != "OPERATION_DECISION" || asked[0].FactKind != "PENDING_IDENTITY_RECHECK" {
		t.Fatalf("准入范围被问 %+v, want 运营决定 × 待识别重核", asked)
	}

	other := decisionRegister{kinds: []identity.DecisionKind{identity.DecisionSegmentClosure}}
	if _, err := decisionAuthenticator(t, operatorVerifier{}, other, decisionAdmission{admitted: true}).
		AuthenticateOperatorDecision(context.Background(), "t", nodeopshttp.OperatorDecisionPendingIdentityRecheck); !errors.Is(err, nodeopshttp.ErrOperatorNotGranted) {
		t.Fatalf("只持关段授予：err = %v, want ErrOperatorNotGranted", err)
	}
}

// Covers: 接入身份能力的格逐格译成 nodeopshttp 的格，含运营决定才有的「不在准入范围」。
func TestOperatorDecisionGradesTranslateIntoNodeOperationsGrades(t *testing.T) {
	granted := decisionRegister{kinds: []identity.DecisionKind{identity.DecisionPendingIdentityRecheck}}
	cases := map[string]struct {
		verifier  identity.OperatorCredentialVerifier
		register  decisionRegister
		admission decisionAdmission
		token     string
		want      error
	}{
		"issuer parameters unset": {identity.UnconfiguredOperatorCredentialVerifier{}, granted, decisionAdmission{admitted: true}, "t", nodeopshttp.ErrAccessChannelNotConfigured},
		"token missing":           {operatorVerifier{}, granted, decisionAdmission{admitted: true}, "", nodeopshttp.ErrOperatorCredentialRejected},
		"no grant":                {operatorVerifier{}, decisionRegister{}, decisionAdmission{admitted: true}, "t", nodeopshttp.ErrOperatorNotGranted},
		"outside admission":       {operatorVerifier{}, granted, decisionAdmission{}, "t", nodeopshttp.ErrOutsideAdmissionScope},
		"issuer key set down":     {operatorVerifier{err: identity.ErrCredentialVerifierUnavailable}, granted, decisionAdmission{admitted: true}, "t", nodeopshttp.ErrIdentityDependencyUnavailable},
		"register down":           {operatorVerifier{}, decisionRegister{err: errors.New("down")}, decisionAdmission{admitted: true}, "t", nodeopshttp.ErrIdentityDependencyUnavailable},
		"admission unreadable":    {operatorVerifier{}, granted, decisionAdmission{err: errors.New("down")}, "t", nodeopshttp.ErrIdentityDependencyUnavailable},
	}
	grades := []error{nodeopshttp.ErrAccessChannelNotConfigured, nodeopshttp.ErrOperatorCredentialRejected, nodeopshttp.ErrOperatorNotGranted,
		nodeopshttp.ErrOutsideAdmissionScope, nodeopshttp.ErrIdentityDependencyUnavailable}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := decisionAuthenticator(t, testCase.verifier, testCase.register, testCase.admission).
				AuthenticateOperatorDecision(context.Background(), testCase.token, nodeopshttp.OperatorDecisionPendingIdentityRecheck)
			for _, grade := range grades {
				if errors.Is(err, grade) != (grade == testCase.want) {
					t.Fatalf("err = %v: errors.Is(%v) = %v", err, grade, errors.Is(err, grade))
				}
			}
		})
	}
	if _, err := noaccess.NewOperatorDecisionAuthenticator(nil); err == nil {
		t.Fatal("缺铸造器却构造成功")
	}
}
