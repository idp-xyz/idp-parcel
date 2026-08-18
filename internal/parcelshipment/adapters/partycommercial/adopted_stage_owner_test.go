package partycommercial_test

import (
	"context"
	"errors"
	"testing"
	"time"

	adapter "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/partycommercial"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	pcapplication "go.idp.xyz/idp-parcel/internal/partycommercial/application"
	pcdomain "go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	pcports "go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

type requestFinderDouble struct {
	request psdomain.ShipmentRequest
	found   bool
	err     error
}

func (double *requestFinderDouble) FindBySourceIdentity(
	context.Context,
	psdomain.SourceIdentity,
) (psdomain.ShipmentRequest, bool, error) {
	if double.err != nil {
		return psdomain.ShipmentRequest{}, false, double.err
	}
	return double.request, double.found, nil
}

type viewOnlyResolution struct {
	closure pcdomain.CommercialClosure
	found   bool
	err     error
}

func (view viewOnlyResolution) LoadResolution(
	context.Context,
	pcdomain.TenantID,
	pcdomain.ResolutionID,
) (pcdomain.CommercialClosure, bool, error) {
	if view.err != nil {
		return pcdomain.CommercialClosure{}, false, view.err
	}
	return view.closure, view.found, nil
}

var _ pcports.CommercialResolutionView = viewOnlyResolution{}

// Covers: 编译期只读口可接线——owner 只要 CommercialResolutionView，不持 Save。
func TestResolvedAdoptedStageOwnerWiresAReadOnlyResolutionView(t *testing.T) {
	_, err := adapter.NewResolvedAdoptedStageOwner(&requestFinderDouble{}, viewOnlyResolution{})
	if err != nil {
		t.Fatalf("只读口接线失败：%v", err)
	}
}

// Covers: 已接受委托上的解析标识能从提供方闭包取出采用的接单规则包版本；kind/object/version
// 逐项来自闭包，不拆快照上的 RulePackage 字符串。授权规则未列入必需依据时 found=false。
func TestResolvedOwnerReturnsTheAdoptedRulePackageFromTheClosure(t *testing.T) {
	closure := resolvedClosure(t)
	adopted, ok := closure.AdoptedFor(pcdomain.AcceptanceRulePackageObject)
	if !ok {
		t.Fatal("夹具闭包没有采用接单规则包")
	}
	identity := stageIdentity(t)
	owner, err := adapter.NewResolvedAdoptedStageOwner(
		&requestFinderDouble{
			request: acceptedRequestWithResolution(t, identity, closure.ResolutionID().String(), "not-the-owner/with/slash"),
			found:   true,
		},
		viewOnlyResolution{closure: closure, found: true},
	)
	if err != nil {
		t.Fatalf("构造：%v", err)
	}

	got, found, err := owner.AcceptanceRulePackageFor(context.Background(), identity)
	if err != nil || !found {
		t.Fatalf("found = %v err = %v", found, err)
	}
	if got.Kind() != adopted.Version().Kind() ||
		got.ObjectID() != adopted.Version().ObjectID() ||
		got.Version() != adopted.Version().Version() {
		t.Fatalf("got kind/object/version = %s %s %s, want %s %s %s",
			got.Kind(), got.ObjectID(), got.Version(),
			adopted.Version().Kind(), adopted.Version().ObjectID(), adopted.Version().Version())
	}

	if _, found, err := owner.AuthorizationRuleFor(context.Background(), identity); err != nil || found {
		t.Fatalf("授权规则缺席应 found=false：found = %v err = %v", found, err)
	}
}

// Covers: 提供方还没有这份闭包时答 found=false——SYN-RES-01 未写入解析库就是这一格，
// 不是装配错误，不要种规则包把它变绿。
func TestResolvedOwnerReportsMissingClosureAsNotFound(t *testing.T) {
	identity := stageIdentity(t)
	owner, err := adapter.NewResolvedAdoptedStageOwner(
		&requestFinderDouble{request: acceptedRequestWithResolution(t, identity, "SYN-RES-01", "rules-1/v1"), found: true},
		viewOnlyResolution{},
	)
	if err != nil {
		t.Fatalf("构造：%v", err)
	}
	if _, found, err := owner.AcceptanceRulePackageFor(context.Background(), identity); err != nil || found {
		t.Fatalf("found = %v err = %v, want found=false", found, err)
	}
}

func TestResolvedOwnerDoesNotTreatLookupFailuresAsUnconfigured(t *testing.T) {
	identity := stageIdentity(t)
	tests := []struct {
		name    string
		finder  *requestFinderDouble
		view    viewOnlyResolution
		wantErr error
	}{
		{
			name:   "读委托失败",
			finder: &requestFinderDouble{err: errors.New("shipment store down")},
			view:   viewOnlyResolution{found: true},
		},
		{
			name: "重建失败",
			finder: &requestFinderDouble{
				err: psdomain.ErrInvalidRehydratedShipmentRequest,
			},
			view:    viewOnlyResolution{found: true},
			wantErr: psdomain.ErrInvalidRehydratedShipmentRequest,
		},
		{
			name: "读闭包失败",
			finder: &requestFinderDouble{
				request: acceptedRequestWithResolution(t, identity, "RES-1", "rules-1/v1"),
				found:   true,
			},
			view: viewOnlyResolution{err: errors.New("resolution store down")},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			owner, err := adapter.NewResolvedAdoptedStageOwner(test.finder, test.view)
			if err != nil {
				t.Fatalf("构造：%v", err)
			}
			_, found, err := owner.AcceptanceRulePackageFor(context.Background(), identity)
			if err == nil || found {
				t.Fatalf("found = %v err = %v, want error", found, err)
			}
			if test.wantErr != nil && !errors.Is(err, test.wantErr) {
				t.Fatalf("err = %v, want %v", err, test.wantErr)
			}
		})
	}
}

func TestResolvedOwnerRejectsAClosureFromAnotherTenant(t *testing.T) {
	closure := resolvedClosure(t)
	identity := stageIdentity(t)
	owner, err := adapter.NewResolvedAdoptedStageOwner(
		&requestFinderDouble{
			request: acceptedRequestWithResolution(t, identity, closure.ResolutionID().String(), "rules-1/v1"),
			found:   true,
		},
		viewOnlyResolution{closure: foreignTenantClosure(t), found: true},
	)
	if err != nil {
		t.Fatalf("构造：%v", err)
	}
	_, found, err := owner.AcceptanceRulePackageFor(context.Background(), identity)
	if !errors.Is(err, adapter.ErrAdoptedResolutionTenantMismatch) || found {
		t.Fatalf("found = %v err = %v, want ErrAdoptedResolutionTenantMismatch", found, err)
	}
}

func TestResolvedOwnerRejectsAClosureWithoutAnAcceptanceRulePackage(t *testing.T) {
	closure := resolvedClosureWithoutRulePackage(t)
	if _, ok := closure.AdoptedFor(pcdomain.AcceptanceRulePackageObject); ok {
		t.Fatal("夹具闭包不该采用接单规则包")
	}
	identity := stageIdentity(t)
	owner, err := adapter.NewResolvedAdoptedStageOwner(
		&requestFinderDouble{
			request: acceptedRequestWithResolution(t, identity, closure.ResolutionID().String(), "rules-1/v1"),
			found:   true,
		},
		viewOnlyResolution{closure: closure, found: true},
	)
	if err != nil {
		t.Fatalf("构造：%v", err)
	}
	_, found, err := owner.AcceptanceRulePackageFor(context.Background(), identity)
	if !errors.Is(err, adapter.ErrUntranslatableAnswer) || found {
		t.Fatalf("found = %v err = %v, want ErrUntranslatableAnswer", found, err)
	}
}

func TestResolvedOwnerTreatsUnfixedAdoptionAsNotFound(t *testing.T) {
	identity := stageIdentity(t)
	tests := []struct {
		name   string
		finder *requestFinderDouble
	}{
		{"委托找不到", &requestFinderDouble{}},
		{"尚未已接受", &requestFinderDouble{request: submittedRequest(t, identity), found: true}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			owner, err := adapter.NewResolvedAdoptedStageOwner(test.finder, viewOnlyResolution{found: true})
			if err != nil {
				t.Fatalf("构造：%v", err)
			}
			if _, found, err := owner.AcceptanceRulePackageFor(context.Background(), identity); err != nil || found {
				t.Fatalf("found = %v err = %v, want found=false", found, err)
			}
		})
	}
}

func submittedRequest(t *testing.T, identity psdomain.SourceIdentity) psdomain.ShipmentRequest {
	t.Helper()
	at := time.Date(2026, 8, 5, 10, 0, 0, 0, time.UTC)
	versionID := value(t, psdomain.NewSubmissionVersionID, "version-1")
	fingerprint, err := psdomain.NewSourceSubmissionFingerprint(
		identity,
		value(t, psdomain.NewPayloadDigest, "sha256:a"),
		at,
		at.Add(time.Second),
	)
	if err != nil {
		t.Fatalf("fingerprint: %v", err)
	}
	request, err := psdomain.RehydrateShipmentRequest(psdomain.RehydrateShipmentRequestSpec{
		Revision:          1,
		ShipmentRequestID: value(t, psdomain.NewShipmentRequestID, "request-1"),
		BatchID:           value(t, psdomain.NewSubmissionBatchID, "batch-1"),
		State:             psdomain.ShipmentRequestSubmitted,
		SubmittedAt:       at,
		CurrentVersion: psdomain.RehydrateSubmissionVersionSpec{
			VersionID:         versionID,
			SourceSubmission:  fingerprint,
			DeclaredParcelIDs: []psdomain.DeclaredParcelID{value(t, psdomain.NewDeclaredParcelID, "parcel-1")},
			EstablishedAt:     at,
		},
		AcceptanceTask: psdomain.RehydrateAcceptanceTaskSpec{
			TaskID:              value(t, psdomain.NewAcceptanceDecisionTaskID, "task-1"),
			SubmissionVersionID: versionID,
			EstablishedAt:       at,
			State:               psdomain.AcceptanceTaskRunning,
		},
	})
	if err != nil {
		t.Fatalf("rehydrate submitted: %v", err)
	}
	return request
}

func acceptedRequestWithResolution(
	t *testing.T,
	identity psdomain.SourceIdentity,
	resolutionID, rulePackage string,
) psdomain.ShipmentRequest {
	t.Helper()
	at := time.Date(2026, 8, 7, 13, 0, 0, 0, time.UTC)
	versionID := value(t, psdomain.NewSubmissionVersionID, "version-1")
	parcel := value(t, psdomain.NewDeclaredParcelID, "parcel-1")
	fingerprint, err := psdomain.NewSourceSubmissionFingerprint(
		identity,
		value(t, psdomain.NewPayloadDigest, "sha256:a"),
		at.Add(-time.Hour),
		at.Add(-time.Hour+time.Second),
	)
	if err != nil {
		t.Fatalf("fingerprint: %v", err)
	}
	applicable, err := psdomain.NewApplicableCheckGroups(
		psdomain.CustomerRelationshipCheck,
		psdomain.LegalEntityAndContractCheck,
		psdomain.ProductAndServiceCheck,
		psdomain.MemberBaselineCheck,
		psdomain.RequiredDocumentCheck,
		psdomain.PreAcceptanceFinancialControlCheck,
		psdomain.NetworkReachabilityCheck,
	)
	if err != nil {
		t.Fatalf("applicable groups: %v", err)
	}
	basis, err := psdomain.NewCommercialBasisSnapshot(psdomain.CommercialBasisSnapshotSpec{
		ResolutionID: value(t, psdomain.NewCommercialResolutionID, resolutionID),
		RulePackage:  value(t, psdomain.NewRulePackageReference, rulePackage),
		ViewRevision: value(t, psdomain.NewCommercialViewRevision, "VIEW-1"),
		Applicable:   applicable,
		ManualReview: psdomain.ManualReviewNotRequiredByRules,
	})
	if err != nil {
		t.Fatalf("basis: %v", err)
	}
	request, err := psdomain.RehydrateShipmentRequest(psdomain.RehydrateShipmentRequestSpec{
		Revision:          2,
		ShipmentRequestID: value(t, psdomain.NewShipmentRequestID, "request-1"),
		BatchID:           value(t, psdomain.NewSubmissionBatchID, "batch-1"),
		State:             psdomain.ShipmentRequestAccepted,
		SubmittedAt:       at.Add(-time.Hour),
		CurrentVersion: psdomain.RehydrateSubmissionVersionSpec{
			VersionID:         versionID,
			SourceSubmission:  fingerprint,
			DeclaredParcelIDs: []psdomain.DeclaredParcelID{parcel},
			EstablishedAt:     at.Add(-time.Hour),
		},
		AcceptanceTask: psdomain.RehydrateAcceptanceTaskSpec{
			TaskID:              value(t, psdomain.NewAcceptanceDecisionTaskID, "task-1"),
			SubmissionVersionID: versionID,
			EstablishedAt:       at.Add(-time.Hour),
			State:               psdomain.AcceptanceTaskComplete,
		},
		DecisionFormed: true,
		Decision: psdomain.RehydrateAcceptanceDecisionSpec{
			DecisionID:   value(t, psdomain.NewAcceptanceDecisionID, "decision-1"),
			Accepted:     true,
			Checks:       passingChecksFor(t, "parcel-1"),
			Basis:        basis,
			ManualReview: psdomain.ManualReviewNotRequired,
			DecidedAt:    at,
		},
		Baseline: psdomain.RehydrateAcceptanceBaselineSpec{
			DeclaredParcelIDs: []psdomain.DeclaredParcelID{parcel},
			SubmissionVersion: versionID,
			FixedAt:           at,
		},
		Commitment: psdomain.RehydrateExpectedCommitmentSpec{
			Basis:    basis,
			FormedAt: at,
		},
	})
	if err != nil {
		t.Fatalf("rehydrate accepted: %v", err)
	}
	return request
}

func passingChecksFor(t *testing.T, parcel string) []psdomain.AcceptanceCheck {
	t.Helper()
	groups := []psdomain.AcceptanceCheckGroup{
		psdomain.CustomerRelationshipCheck,
		psdomain.LegalEntityAndContractCheck,
		psdomain.ProductAndServiceCheck,
		psdomain.MemberBaselineCheck,
		psdomain.RequiredDocumentCheck,
		psdomain.PreAcceptanceFinancialControlCheck,
		psdomain.NetworkReachabilityCheck,
	}
	checks := make([]psdomain.AcceptanceCheck, 0, len(groups))
	for _, group := range groups {
		member := psdomain.DeclaredParcelID{}
		if group == psdomain.NetworkReachabilityCheck {
			member = value(t, psdomain.NewDeclaredParcelID, parcel)
		}
		check, err := psdomain.NewAcceptanceCheck(group, member, psdomain.CheckPassed, psdomain.CheckReason{})
		if err != nil {
			t.Fatalf("check %s: %v", group, err)
		}
		checks = append(checks, check)
	}
	return checks
}

func resolvedClosureWithoutRulePackage(t *testing.T) pcdomain.CommercialClosure {
	t.Helper()
	registry := pcdomain.NewCommercialRegistry()
	effectiveIn(t, registry, pcdomain.CustomerContractObject, "contract-1", "v1", "sha256:c1", "scope-a")
	anchor, err := pcdomain.NewSelectionAnchor(anchorAt, value(t, pcdomain.NewAnchorPolicyVersion, "anchor-policy-v1"))
	if err != nil {
		t.Fatalf("new selection anchor: %v", err)
	}
	resolved, err := pcapplication.NewResolveCommercialBasisHandler(
		&authorityDouble{registry: registry}, &resolutionStoreDouble{}, fixedClock{at: judgedAt},
	).Handle(context.Background(), pcapplication.ResolveCommercialBasisCommand{
		Key: pcdomain.ClosureResolutionKey{
			TenantID:             value(t, pcdomain.NewTenantID, "tenant-1"),
			CustomerAccountID:    value(t, pcdomain.NewCustomerAccountID, "customer-1"),
			LegalEntityCandidate: value(t, pcdomain.NewLegalEntityReference, "legal-1"),
			Scope:                value(t, pcdomain.NewCommercialScopeReference, "scope-a"),
			Purpose:              pcdomain.AcceptanceControlPurpose,
			Anchor:               anchor,
			RequiredBases:        []pcdomain.CommercialObjectKind{pcdomain.CustomerContractObject},
		},
	})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if resolved.Closure().Outcome() != pcdomain.UniquelyResolved {
		t.Fatalf("outcome = %q, want UNIQUELY_RESOLVED", resolved.Closure().Outcome())
	}
	return resolved.Closure()
}

func foreignTenantClosure(t *testing.T) pcdomain.CommercialClosure {
	t.Helper()
	registry := pcdomain.NewCommercialRegistry()
	effectiveInOtherTenant(t, registry, pcdomain.CustomerContractObject, "contract-1", "v1", "sha256:c1", "scope-a")
	effectiveInOtherTenant(t, registry, pcdomain.AcceptanceRulePackageObject, "rules-1", "v1", "sha256:r1", "scope-a")
	anchor, err := pcdomain.NewSelectionAnchor(anchorAt, value(t, pcdomain.NewAnchorPolicyVersion, "anchor-policy-v1"))
	if err != nil {
		t.Fatalf("new selection anchor: %v", err)
	}
	resolved, err := pcapplication.NewResolveCommercialBasisHandler(
		&authorityDouble{registry: registry}, &resolutionStoreDouble{}, fixedClock{at: judgedAt},
	).Handle(context.Background(), pcapplication.ResolveCommercialBasisCommand{
		Key: pcdomain.ClosureResolutionKey{
			TenantID:             value(t, pcdomain.NewTenantID, "tenant-b"),
			CustomerAccountID:    value(t, pcdomain.NewCustomerAccountID, "customer-1"),
			LegalEntityCandidate: value(t, pcdomain.NewLegalEntityReference, "legal-1"),
			Scope:                value(t, pcdomain.NewCommercialScopeReference, "scope-a"),
			Purpose:              pcdomain.AcceptanceControlPurpose,
			Anchor:               anchor,
			RequiredBases: []pcdomain.CommercialObjectKind{
				pcdomain.CustomerContractObject, pcdomain.AcceptanceRulePackageObject,
			},
		},
	})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if resolved.Closure().Outcome() != pcdomain.UniquelyResolved {
		t.Fatalf("outcome = %q, want UNIQUELY_RESOLVED", resolved.Closure().Outcome())
	}
	return resolved.Closure()
}

func effectiveInOtherTenant(
	t *testing.T,
	registry *pcdomain.CommercialRegistry,
	kind pcdomain.CommercialObjectKind,
	objectID, version, digest, scope string,
) pcdomain.CommercialVersion {
	t.Helper()
	interval, err := pcdomain.NewEffectiveInterval(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Time{})
	if err != nil {
		t.Fatalf("new effective interval: %v", err)
	}
	draft, err := pcdomain.NewCommercialDraft(pcdomain.CommercialVersionSpec{
		TenantID:      value(t, pcdomain.NewTenantID, "tenant-b"),
		Kind:          kind,
		ObjectID:      value(t, pcdomain.NewCommercialObjectID, objectID),
		Version:       value(t, pcdomain.NewCommercialVersionLabel, version),
		Scope:         value(t, pcdomain.NewCommercialScopeReference, scope),
		ContentDigest: value(t, pcdomain.NewCommercialContentDigest, digest),
		Effective:     interval,
	})
	if err != nil {
		t.Fatalf("new draft: %v", err)
	}
	approvedAt := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	basis, err := pcdomain.NewApprovalBasis(
		value(t, pcdomain.NewApprovalReference, "approval-"+objectID),
		value(t, pcdomain.NewCommercialSourceReference, "source-"+objectID),
		approvedAt,
	)
	if err != nil {
		t.Fatalf("new approval basis: %v", err)
	}
	published, err := draft.Publish(basis, pcdomain.ApprovalRoleConfirmed, approvedAt, nil)
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	live, err := published.TakeEffect(approvedAt)
	if err != nil {
		t.Fatalf("take effect: %v", err)
	}
	if _, err := registry.Register(live); err != nil {
		t.Fatalf("register: %v", err)
	}
	return live
}
