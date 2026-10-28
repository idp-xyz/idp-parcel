package postgres_test

import (
	"errors"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// Covers: PBC-08 行为面——待人工处理项的两个写口（落项、登记结果）在无事务上下文必须被拒绝，落库行数为零（ADR-0341）。
func TestThePostIntakeManualItemWritesRefuseToRunOutsideATransaction(t *testing.T) {
	rig := newConflictDecisionRig(t)
	items, err := adapter.NewPostIntakeManualItems(rig.db)
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC)
	disposition, err := domain.DecidePostIntakeDisposition(domain.PostIntakeDispositionSpec{
		Tenant:              mustBuild(t, domain.NewTenantID, "tenant-1"),
		Parcel:              mustBuild(t, domain.NewDeclaredParcelID, "parcel-1"),
		Version:             1,
		Direction:           domain.PostIntakeIntercept,
		Source:              domain.PostIntakeOperatorInitiated,
		ResponsibilityStart: mustBuild(t, domain.NewSourceResultVersion, "intake-result/v1"),
		Decider:             mustBuild(t, domain.NewPostIntakeDispositionDeciderReference, "https://id.syn.example#alice"),
		Basis:               mustBuild(t, domain.NewPostIntakeDispositionBasis, "SYN-BASIS"),
		DecidedAt:           at,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := items.RecordPostIntakeManualItems(t.Context(), disposition); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务落项应返回 ErrTransactionRequired，实得：%v", err)
	}
	result, err := domain.FormManualItemResult(domain.ManualItemHandled,
		mustBuild(t, domain.NewManualItemNote, "done"), mustBuild(t, domain.NewManualItemHandlerReference, "https://id.syn.example#bob"), at)
	if err != nil {
		t.Fatal(err)
	}
	key := domain.ManualItemKey{
		Tenant: disposition.Tenant(), Parcel: disposition.Parcel(), DecisionVersion: 1,
		Owner: domain.DispositionOwnerNodeOperations, Action: domain.DispositionActionHoldAtControl,
	}
	if _, _, _, err := items.RegisterPostIntakeManualItemResult(t.Context(), key, result); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务登记应返回 ErrTransactionRequired，实得：%v", err)
	}
	if n := rig.countRows(t, `SELECT count(*) FROM parcel_shipment.post_intake_disposition_manual_item`); n != 0 {
		t.Errorf("被拒的写口落了 %d 行项", n)
	}
}
