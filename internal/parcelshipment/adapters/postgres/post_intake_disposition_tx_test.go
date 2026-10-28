package postgres_test

import (
	"errors"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	adapter "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// Covers: PBC-08 行为面——收寄后服务处置决定的两个写口（追加决定、交出信封）在无事务上下文必须被拒绝，落库行数为零；信封口对缺项的意图响亮报错、不入队。
func TestThePostIntakeDispositionWritesRefuseToRunOutsideATransaction(t *testing.T) {
	rig := newConflictDecisionRig(t)
	dispositions, err := adapter.NewPostIntakeDispositions(rig.db)
	if err != nil {
		t.Fatal(err)
	}
	store, err := outbox.NewStore(rig.db)
	if err != nil {
		t.Fatal(err)
	}
	handoff, err := adapter.NewOutboxPostIntakeDispositionHandoff(rig.db, store, notUpheldClock{at: time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatal(err)
	}
	disposition, err := domain.DecidePostIntakeDisposition(domain.PostIntakeDispositionSpec{
		Tenant:              mustBuild(t, domain.NewTenantID, "tenant-1"),
		Parcel:              mustBuild(t, domain.NewDeclaredParcelID, "parcel-1"),
		Version:             1,
		Direction:           domain.PostIntakeIntercept,
		Source:              domain.PostIntakeOperatorInitiated,
		ResponsibilityStart: mustBuild(t, domain.NewSourceResultVersion, "intake-result/v1"),
		Decider:             mustBuild(t, domain.NewPostIntakeDispositionDeciderReference, "https://id.syn.example#alice"),
		Basis:               mustBuild(t, domain.NewPostIntakeDispositionBasis, "SYN-BASIS"),
		DecidedAt:           time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := dispositions.AppendPostIntakeDisposition(t.Context(), disposition); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务追加应返回 ErrTransactionRequired，实得：%v", err)
	}
	if err := handoff.HandOffPostIntakeDisposition(t.Context(), ports.PostIntakeDispositionHandoffIntent{Disposition: disposition}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务入队应返回 ErrTransactionRequired，实得：%v", err)
	}
	if err := handoff.HandOffPostIntakeDisposition(t.Context(), ports.PostIntakeDispositionHandoffIntent{}); err == nil {
		t.Error("缺项的意图被入队")
	}
	if n := rig.countRows(t, `SELECT count(*) FROM parcel_shipment.post_intake_disposition_decision`); n != 0 {
		t.Errorf("被拒的写口落了 %d 行决定", n)
	}
}
