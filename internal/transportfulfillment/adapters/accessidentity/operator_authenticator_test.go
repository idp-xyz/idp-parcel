package accessidentity_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	identity "go.idp.xyz/idp-parcel/internal/accessidentity"
	tfaccess "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/accessidentity"
	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
)

const operatorTenant = "SYN-TENANT-01"

var operatorNow = time.Date(2026, 9, 25, 3, 0, 0, 0, time.UTC)

type operatorVerifier struct{ err error }

func (verifier operatorVerifier) VerifyOperatorCredential(_ context.Context, credential identity.OperatorCredential) (identity.OperatorSubject, error) {
	if verifier.err != nil {
		return identity.OperatorSubject{}, verifier.err
	}
	if credential.Token() == "" {
		return identity.OperatorSubject{}, identity.ErrCredentialRejected
	}
	return identity.NewOperatorSubject("https://id.syn.example/dex", "SYN-OPERATOR-01")
}

type operatorRegister struct {
	kinds []identity.DecisionKind
	err   error
}

func (register operatorRegister) FindOperator(_ context.Context, subject identity.OperatorSubject) (identity.OperatorStanding, bool, error) {
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

type operatorAdmission struct {
	admitted bool
	err      error
}

func (admission operatorAdmission) Admits(context.Context, string, identity.AdmissionRequirement, time.Time) (bool, error) {
	return admission.admitted, admission.err
}

func authenticator(t *testing.T, verifier identity.OperatorCredentialVerifier, register operatorRegister, admission operatorAdmission) *tfaccess.OperatorAuthenticator {
	t.Helper()
	minter, err := identity.NewOperatorMinter(verifier, register, admission, func() time.Time { return operatorNow })
	if err != nil {
		t.Fatal(err)
	}
	built, err := tfaccess.NewOperatorAuthenticator(minter)
	if err != nil {
		t.Fatal(err)
	}
	return built
}

func TestGrantedOperatorAuthenticatesToTheBoundTenant(t *testing.T) {
	built := authenticator(t, operatorVerifier{}, operatorRegister{kinds: []identity.DecisionKind{identity.DecisionSegmentClosure}}, operatorAdmission{admitted: true})
	tenant, err := built.AuthenticateOperatorDecision(context.Background(), "presented.operator.token", tfhttp.OperatorDecisionSegmentClosure)
	if err != nil || tenant.String() != operatorTenant {
		t.Fatalf("tenant = %q, err = %v", tenant.String(), err)
	}
}

func TestEveryTFDecisionMapsToItsOwnDecisionKind(t *testing.T) {
	decisions := map[tfhttp.OperatorDecision]identity.DecisionKind{
		tfhttp.OperatorDecisionSegmentClosure:                      identity.DecisionSegmentClosure,
		tfhttp.OperatorDecisionDispatchTaskRegistration:            identity.DecisionDispatchTaskRegistration,
		tfhttp.OperatorDecisionLoadAssignment:                      identity.DecisionLoadAssignment,
		tfhttp.OperatorDecisionParticipationTermination:            identity.DecisionParticipationTermination,
		tfhttp.OperatorDecisionEffectiveTimeJudgment:               identity.DecisionEffectiveTimeJudgment,
		tfhttp.OperatorDecisionCarrierFirstEffectivePickupJudgment: identity.DecisionCarrierFirstEffectivePickupJudgment,
	}
	for decision, kind := range decisions {
		for other, otherKind := range decisions {
			built := authenticator(t, operatorVerifier{}, operatorRegister{kinds: []identity.DecisionKind{otherKind}}, operatorAdmission{admitted: true})
			_, err := built.AuthenticateOperatorDecision(context.Background(), "t", decision)
			if (other == decision) != (err == nil) {
				t.Fatalf("decision %s with a grant for %s (%s): err = %v", decision, other, kind, err)
			}
		}
	}
}

func TestIdentityGradesTranslateIntoTransportFulfillmentGrades(t *testing.T) {
	granted := operatorRegister{kinds: []identity.DecisionKind{identity.DecisionSegmentClosure}}
	cases := map[string]struct {
		verifier  identity.OperatorCredentialVerifier
		register  operatorRegister
		admission operatorAdmission
		token     string
		want      error
	}{
		"issuer parameters unset":    {identity.UnconfiguredOperatorCredentialVerifier{}, granted, operatorAdmission{admitted: true}, "t", tfhttp.ErrAccessChannelNotConfigured},
		"token missing":              {operatorVerifier{}, granted, operatorAdmission{admitted: true}, "", tfhttp.ErrOperatorCredentialRejected},
		"no grant":                   {operatorVerifier{}, operatorRegister{}, operatorAdmission{admitted: true}, "t", tfhttp.ErrOperatorNotGranted},
		"outside admission":          {operatorVerifier{}, granted, operatorAdmission{}, "t", tfhttp.ErrOutsideAdmissionScope},
		"issuer key set unreachable": {operatorVerifier{err: identity.ErrCredentialVerifierUnavailable}, granted, operatorAdmission{admitted: true}, "t", tfhttp.ErrIdentityDependencyUnavailable},
		"register down":              {operatorVerifier{}, operatorRegister{err: errors.New("down")}, operatorAdmission{admitted: true}, "t", tfhttp.ErrIdentityDependencyUnavailable},
		"admission unreadable":       {operatorVerifier{}, granted, operatorAdmission{err: errors.New("down")}, "t", tfhttp.ErrIdentityDependencyUnavailable},
	}
	grades := []error{tfhttp.ErrAccessChannelNotConfigured, tfhttp.ErrOperatorCredentialRejected, tfhttp.ErrOperatorNotGranted, tfhttp.ErrOutsideAdmissionScope, tfhttp.ErrIdentityDependencyUnavailable}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := authenticator(t, testCase.verifier, testCase.register, testCase.admission).
				AuthenticateOperatorDecision(context.Background(), testCase.token, tfhttp.OperatorDecisionSegmentClosure)
			for _, grade := range grades {
				if errors.Is(err, grade) != (grade == testCase.want) {
					t.Fatalf("err = %v: errors.Is(%v) = %v", err, grade, errors.Is(err, grade))
				}
			}
		})
	}
}
