package accessidentity_test

import (
	"context"
	"errors"
	"testing"
	"time"

	identity "go.idp.xyz/idp-parcel/internal/accessidentity"
	nraccess "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/accessidentity"
	networkhttp "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/http"
)

var decisionNow = time.Date(2026, 10, 26, 9, 0, 0, 0, time.UTC)

type decisionVerifier struct{}

func (decisionVerifier) VerifyOperatorCredential(_ context.Context, credential identity.OperatorCredential) (identity.OperatorSubject, error) {
	if credential.Token() == "" {
		return identity.OperatorSubject{}, identity.ErrCredentialRejected
	}
	return identity.NewOperatorSubject("https://id.syn.example/dex", "SYN-NETOPS-01")
}

// decisionRegister 是操作者册的替身：主体绑演示租户，逐种持有给定的运营决定授予。
type decisionRegister struct{ kinds []identity.DecisionKind }

func (register decisionRegister) FindOperator(_ context.Context, subject identity.OperatorSubject) (identity.OperatorStanding, bool, error) {
	binding, err := identity.NewOperatorBinding(subject, "SYN-TENANT-01", "SYN-BASIS-BINDING")
	if err != nil {
		return identity.OperatorStanding{}, false, err
	}
	interval, err := identity.NewEffectiveInterval(decisionNow.Add(-time.Hour), time.Time{})
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

func decisionAuthenticatorOver(t *testing.T, register decisionRegister, scope identity.AdmissionScope) *nraccess.OperatorDecisionAuthenticator {
	t.Helper()
	minter, err := identity.NewOperatorMinter(decisionVerifier{}, register, scope, func() time.Time { return decisionNow })
	if err != nil {
		t.Fatal(err)
	}
	built, err := nraccess.NewOperatorDecisionAuthenticator(minter)
	if err != nil {
		t.Fatal(err)
	}
	return built
}

// Covers: AT-NR-098（ADR-0284 决定一）——按「授权改路」这一种铸信封并判准入（能力 OPERATION_DECISION、事实类型取决定种类）；只持别的种类答
// 无此种授予，不默认放行；准入范围没登答不在准入范围；没令牌答令牌不过。交回的操作者是「发行方#sub」。
func TestTheRerouteDecisionAuthenticatesItsOwnKindAndAsksTheAdmissionScope(t *testing.T) {
	var asked []identity.AdmissionRequirement
	granted := decisionAuthenticatorOver(t, decisionRegister{kinds: []identity.DecisionKind{identity.DecisionAuthorizedReroute}},
		decisionAdmission{admitted: map[string]bool{"AUTHORIZED_REROUTE": true}, asked: &asked})
	operator, err := granted.AuthenticateOperatorDecision(context.Background(), "presented.token", networkhttp.OperatorDecisionAuthorizedReroute)
	if err != nil {
		t.Fatalf("authenticate: %v", err)
	}
	if operator.Tenant.String() != "SYN-TENANT-01" || operator.Operator != "https://id.syn.example/dex#SYN-NETOPS-01" {
		t.Fatalf("operator = %+v", operator)
	}
	if len(asked) != 1 || asked[0].Capability != "OPERATION_DECISION" || asked[0].FactKind != "AUTHORIZED_REROUTE" {
		t.Fatalf("准入所问 = %+v", asked)
	}

	others := decisionAuthenticatorOver(t, decisionRegister{kinds: []identity.DecisionKind{
		identity.DecisionTransportCommissionSubmission, identity.DecisionCustomsCaseEstablishment,
	}}, decisionAdmission{admitted: map[string]bool{"AUTHORIZED_REROUTE": true}, asked: &asked})
	if _, err := others.AuthenticateOperatorDecision(context.Background(), "presented.token", networkhttp.OperatorDecisionAuthorizedReroute); !errors.Is(err, networkhttp.ErrOperatorNotGranted) {
		t.Fatalf("只持别的种类：err = %v, want ErrOperatorNotGranted", err)
	}

	outside := decisionAuthenticatorOver(t, decisionRegister{kinds: []identity.DecisionKind{identity.DecisionAuthorizedReroute}},
		decisionAdmission{admitted: map[string]bool{}, asked: &asked})
	if _, err := outside.AuthenticateOperatorDecision(context.Background(), "presented.token", networkhttp.OperatorDecisionAuthorizedReroute); !errors.Is(err, networkhttp.ErrOutsideAdmissionScope) {
		t.Fatalf("准入范围没登：err = %v, want ErrOutsideAdmissionScope", err)
	}
	if _, err := granted.AuthenticateOperatorDecision(context.Background(), "", networkhttp.OperatorDecisionAuthorizedReroute); !errors.Is(err, networkhttp.ErrOperatorCredentialRejected) {
		t.Fatalf("没令牌：err = %v, want ErrOperatorCredentialRejected", err)
	}
	if _, err := granted.AuthenticateOperatorDecision(context.Background(), "presented.token", networkhttp.OperatorDecision("UNKNOWN_KIND")); err == nil {
		t.Fatal("不认识的决定种类被放行了")
	}
}

func TestTheRerouteDecisionAuthenticatorNeedsAMinter(t *testing.T) {
	if _, err := nraccess.NewOperatorDecisionAuthenticator(nil); err == nil {
		t.Fatal("nil 铸造器应在构造期被拒")
	}
}
