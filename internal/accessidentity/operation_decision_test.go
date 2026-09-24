package accessidentity_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
)

func decisionStanding(t *testing.T, kinds ...accessidentity.DecisionKind) accessidentity.OperatorStanding {
	t.Helper()
	subject := operatorSubject(t)
	binding, err := accessidentity.NewOperatorBinding(subject, operatorTenant, "SYN-BASIS-BINDING")
	if err != nil {
		t.Fatal(err)
	}
	interval, err := accessidentity.NewEffectiveInterval(mintAt.Add(-24*time.Hour), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	var grants []accessidentity.RecordedGrant
	for index, kind := range kinds {
		grant, err := accessidentity.NewOperationDecisionGrant(operatorTenant, "SYN-DECISION-GRANT-"+string(rune('A'+index)), subject, kind, interval, "SYN-BASIS-GRANT")
		if err != nil {
			t.Fatal(err)
		}
		recorded, err := accessidentity.NewRecordedGrant(grant, nil)
		if err != nil {
			t.Fatal(err)
		}
		grants = append(grants, recorded)
	}
	standing, err := accessidentity.NewOperatorStanding(binding, grants)
	if err != nil {
		t.Fatal(err)
	}
	return standing
}

func TestOperationDecisionFaceIsGrantedPerDecisionKind(t *testing.T) {
	face, err := accessidentity.ParseCapabilityFace("OPERATION_DECISION")
	if err != nil || face != accessidentity.CapabilityOperationDecision {
		t.Fatalf("ParseCapabilityFace = (%q, %v)", face, err)
	}
	interval, err := accessidentity.NewEffectiveInterval(mintAt.Add(-time.Hour), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := accessidentity.NewOperatorGrant(operatorTenant, "SYN-G", operatorSubject(t), accessidentity.CapabilityOperationDecision, interval, "SYN-B"); !errors.Is(err, accessidentity.ErrDecisionKindRequired) {
		t.Fatalf("decision face granted without a decision kind: err = %v", err)
	}
	if _, err := accessidentity.ParseDecisionKind("APPROVE_EVERYTHING"); !errors.Is(err, accessidentity.ErrDecisionKindUnknown) {
		t.Fatalf("unknown decision kind: err = %v", err)
	}
	for _, raw := range []string{
		"MANUAL_REVIEW_COMPLETION", "ACTIVE_REJECTION", "AUTHORIZED_DISPOSITION", "CONTROLLED_CLOSURE", "CONTROLLED_REOPENING",
		"SEGMENT_CLOSURE", "DISPATCH_TASK_REGISTRATION", "LOAD_ASSIGNMENT", "PARTICIPATION_TERMINATION",
		"EFFECTIVE_TIME_JUDGMENT", "CARRIER_FIRST_EFFECTIVE_PICKUP_JUDGMENT",
	} {
		if kind, err := accessidentity.ParseDecisionKind(raw); err != nil || kind.String() != raw {
			t.Fatalf("decision kind %s of ADR-0151 decision one: (%q, %v)", raw, kind, err)
		}
	}

	standing := decisionStanding(t, accessidentity.DecisionManualReviewCompletion)
	grant := standing.Grants()[0].Grant()
	if grant.Face() != accessidentity.CapabilityOperationDecision || grant.DecisionKind() != accessidentity.DecisionManualReviewCompletion {
		t.Fatalf("grant = (%s, %s)", grant.Face(), grant.DecisionKind())
	}
	if !standing.HoldsDecisionAt(accessidentity.DecisionManualReviewCompletion, mintAt) {
		t.Fatal("operator does not hold the granted decision kind")
	}
	if standing.HoldsDecisionAt(accessidentity.DecisionActiveRejection, mintAt) {
		t.Fatal("a grant for one decision kind reads as another")
	}
	if standing.HoldsAt(accessidentity.CapabilityOperationDecision, mintAt) {
		t.Fatal("the decision face is held as a whole; it must be asked per decision kind")
	}
}

func TestOperatorMintedForOneDecisionKindCannotActOnAnother(t *testing.T) {
	minter := newMinter(t, verifierFake{subject: operatorSubject(t)}, registryFake{standing: decisionStanding(t, accessidentity.DecisionManualReviewCompletion), found: true})
	request := func(kind accessidentity.DecisionKind) accessidentity.OperatorRequest {
		return accessidentity.OperatorRequest{TenantID: operatorTenant, Face: accessidentity.CapabilityOperationDecision, DecisionKind: kind}
	}

	envelope, err := minter.MintOperator(context.Background(), accessidentity.NewOperatorCredential("presented.operator.token"), request(accessidentity.DecisionManualReviewCompletion))
	if err != nil {
		t.Fatalf("granted decision kind: %v", err)
	}
	if !envelope.HoldsDecision(accessidentity.DecisionManualReviewCompletion) || envelope.HoldsDecision(accessidentity.DecisionActiveRejection) || envelope.Holds(accessidentity.CapabilityOperationDecision) {
		t.Fatal("envelope misreports the decision kinds it carries")
	}
	for _, kind := range []accessidentity.DecisionKind{accessidentity.DecisionActiveRejection, ""} {
		_, err := minter.MintOperator(context.Background(), accessidentity.NewOperatorCredential("presented.operator.token"), request(kind))
		assertGrade(t, err, "not granted")
	}
}
