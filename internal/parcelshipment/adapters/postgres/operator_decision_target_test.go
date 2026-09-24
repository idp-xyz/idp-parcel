package postgres_test

import (
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

func TestOperatorDecisionTargetIsFoundTenantWideAndNowhereElse(t *testing.T) {
	views, repository, transactor := newShipmentRequestViews(t)
	ctx := t.Context()
	mustInsert(t, transactor, ctx, repository,
		scopedSubmittedRequest(t, "tenant-1", "customer-7", "target-k1", "REQ-T1", submittedAtFixture))

	target, found, err := views.FindOperatorDecisionTarget(ctx,
		mustBuild(t, domain.NewTenantID, "tenant-1"), mustBuild(t, domain.NewShipmentRequestID, "REQ-T1"))
	if err != nil || !found {
		t.Fatalf("target in its own tenant: found=%v err=%v", found, err)
	}
	if target.Identity.CustomerAccountID().String() != "customer-7" || target.Identity.RequestKey().String() != "target-k1" ||
		target.CurrentVersion.String() == "" {
		t.Fatalf("target = identity %+v, version %q", target.Identity, target.CurrentVersion.String())
	}

	if _, found, err := views.FindOperatorDecisionTarget(ctx,
		mustBuild(t, domain.NewTenantID, "tenant-2"), mustBuild(t, domain.NewShipmentRequestID, "REQ-T1")); err != nil || found {
		t.Fatalf("another tenant sees the request: found=%v err=%v", found, err)
	}
	if _, found, err := views.FindOperatorDecisionTarget(ctx,
		mustBuild(t, domain.NewTenantID, "tenant-1"), mustBuild(t, domain.NewShipmentRequestID, "REQ-NONE")); err != nil || found {
		t.Fatalf("unknown request: found=%v err=%v", found, err)
	}
}
