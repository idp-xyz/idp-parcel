package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	adapter "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

var dispositionAt = time.Date(2026, 8, 13, 9, 0, 0, 0, time.UTC)

func TestADispositionRoundTripsInAllThreeKinds(t *testing.T) {
	store, transactor, _ := newDispositionAcceptances(t)
	ctx := t.Context()

	cases := []struct {
		name   string
		record ports.DispositionAcceptanceRecord
	}{
		{name: "accepted", record: acceptedDisposition(t, "tenant-a", "item-1")},
		{name: "partial", record: partialDisposition(t, "tenant-a", "item-2")},
		{name: "declined", record: declinedDisposition(t, "tenant-a", "item-3")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mustWithinDeliveryTransaction(t, transactor, ctx, func(txCtx context.Context) error {
				outcome, err := store.Save(txCtx, tc.record)
				if err != nil {
					return err
				}
				if outcome != ports.DispositionAcceptanceSaved {
					t.Fatalf("save outcome = %d", outcome)
				}
				return nil
			})

			found, exists, err := store.FindByKey(ctx, tc.record.Key)
			if err != nil || !exists {
				t.Fatalf("读回失败：err=%v exists=%v", err, exists)
			}
			got := found.Decision
			want := tc.record.Decision
			if got.Kind() != want.Kind() ||
				got.Basis() != want.Basis() ||
				got.DeclineBasis() != want.DeclineBasis() ||
				len(got.AcceptedObjects()) != len(want.AcceptedObjects()) ||
				!got.DecidedAt().Equal(want.DecidedAt()) {
				t.Fatalf("承接往返变形：got kind=%s objects=%d decline=%q",
					got.Kind(), len(got.AcceptedObjects()), got.DeclineBasis())
			}
			_, wantAuth := want.MovementAuthority()
			_, gotAuth := got.MovementAuthority()
			if gotAuth != wantAuth {
				t.Fatal("移动授权在场性往返变形")
			}
		})
	}
}

func TestASecondDispositionWriterGetsAlreadyRecorded(t *testing.T) {
	store, transactor, _ := newDispositionAcceptances(t)
	ctx := t.Context()

	first := acceptedDisposition(t, "tenant-a", "item-1")
	mustWithinDeliveryTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		_, err := store.Save(txCtx, first)
		return err
	})

	second := acceptedDisposition(t, "tenant-a", "item-1")
	second.ContentDigest = "digest-other-decision"
	var outcome ports.DispositionAcceptanceSaveOutcome
	var winner ports.DispositionAcceptanceRecord
	var winnerFound bool
	mustWithinDeliveryTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		saved, err := store.Save(txCtx, second)
		if err != nil {
			return err
		}
		outcome = saved
		winner, winnerFound, err = store.FindByKey(txCtx, first.Key)
		return err
	})
	if outcome != ports.DispositionAcceptanceAlreadyRecorded {
		t.Fatalf("第二份写入结果 = %d", outcome)
	}
	if !winnerFound || winner.ContentDigest != first.ContentDigest {
		t.Fatalf("同事务读回赢家失败：found=%v digest=%q", winnerFound, winner.ContentDigest)
	}
}

func TestDispositionAcceptancesAreInvisibleAcrossTenants(t *testing.T) {
	store, transactor, _ := newDispositionAcceptances(t)
	ctx := t.Context()

	saved := acceptedDisposition(t, "tenant-a", "item-shared")
	mustWithinDeliveryTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		_, err := store.Save(txCtx, saved)
		return err
	})

	_, exists, err := store.FindByKey(ctx, ports.DispositionAcceptanceKey{
		Tenant: deliveryValue(t, domain.NewTenantID, "tenant-b"),
		Item:   saved.Key.Item,
	})
	if err != nil || exists {
		t.Errorf("另一个租户读到了承接决定：exists=%v err=%v", exists, err)
	}
}

func TestDispositionWritesRefuseToRunOutsideATransaction(t *testing.T) {
	store, _, _ := newDispositionAcceptances(t)
	ctx := t.Context()

	record := acceptedDisposition(t, "tenant-a", "item-1")
	if _, err := store.Save(ctx, record); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务保存应返回 ErrTransactionRequired，实得：%v", err)
	}
	if _, exists, err := store.FindByKey(ctx, record.Key); err != nil || exists {
		t.Errorf("被拒绝的写入仍然落库：exists=%v err=%v", exists, err)
	}
}

func TestDispositionRollbackLeavesNothingBehind(t *testing.T) {
	store, transactor, _ := newDispositionAcceptances(t)
	ctx := t.Context()
	rollback := errors.New("回滚")
	record := acceptedDisposition(t, "tenant-a", "item-1")

	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		if _, err := store.Save(txCtx, record); err != nil {
			return err
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatalf("事务应以回滚错误结束，实得：%v", err)
	}
	if _, exists, err := store.FindByKey(ctx, record.Key); err != nil || exists {
		t.Errorf("回滚后承接决定仍在：exists=%v err=%v", exists, err)
	}
}

func TestDispositionCheckConstraintsRejectImpossibleRows(t *testing.T) {
	_, _, pool := newDispositionAcceptances(t)
	ctx := t.Context()

	if _, err := pool.Exec(ctx,
		`INSERT INTO transport_fulfillment.disposition_acceptance
			(tenant_id, item_ref, basis, kind, accepted_objects, decline_basis,
			 movement_authority, decided_at, content_digest)
		 VALUES ('tenant-a', 'item-bad-1', 'DISPOSITION/1', 'ACCEPTED', '[]', NULL,
		         'MOVE-AUTH/1', now(), 'd')`); err == nil {
		t.Fatal("一行「接受却没有对象」溜进了承接库")
	}

	if _, err := pool.Exec(ctx,
		`INSERT INTO transport_fulfillment.disposition_acceptance
			(tenant_id, item_ref, basis, kind, accepted_objects, decline_basis,
			 movement_authority, decided_at, content_digest)
		 VALUES ('tenant-a', 'item-bad-2', 'DISPOSITION/1', 'ACCEPTED', '["parcel-1"]', 'why',
		         'MOVE-AUTH/1', now(), 'd')`); err == nil {
		t.Fatal("一行「全量接受却带着拒因」溜进了承接库")
	}

	if _, err := pool.Exec(ctx,
		`INSERT INTO transport_fulfillment.disposition_acceptance
			(tenant_id, item_ref, basis, kind, accepted_objects, decline_basis,
			 movement_authority, decided_at, content_digest)
		 VALUES ('tenant-a', 'item-bad-3', 'DISPOSITION/1', 'DECLINED', '["parcel-1"]', 'why',
		         NULL, now(), 'd')`); err == nil {
		t.Fatal("一行「拒接却带着对象」溜进了承接库")
	}

	if _, err := pool.Exec(ctx,
		`INSERT INTO transport_fulfillment.disposition_acceptance
			(tenant_id, item_ref, basis, kind, accepted_objects, decline_basis,
			 movement_authority, decided_at, content_digest)
		 VALUES ('tenant-a', 'item-bad-4', 'DISPOSITION/1', 'DECLINED', '[]', 'why',
		         'MOVE-AUTH/1', now(), 'd')`); err == nil {
		t.Fatal("一行「拒接却带着移动授权」溜进了承接库")
	}

	if _, err := pool.Exec(ctx,
		`INSERT INTO transport_fulfillment.disposition_acceptance
			(tenant_id, item_ref, basis, kind, accepted_objects, decline_basis,
			 movement_authority, decided_at, content_digest)
		 VALUES ('tenant-a', 'item-bad-5', 'DISPOSITION/1', 'ACCEPTED', '["parcel-1"]', NULL,
		         NULL, now(), 'd')`); err == nil {
		t.Fatal("一行「接受却没有移动授权」溜进了承接库")
	}

	if _, err := pool.Exec(ctx,
		`INSERT INTO transport_fulfillment.disposition_acceptance
			(tenant_id, item_ref, basis, kind, accepted_objects, decline_basis,
			 movement_authority, decided_at, content_digest)
		 VALUES ('tenant-a', 'item-bad-6', 'DISPOSITION/1', 'PARTIALLY_ACCEPTED', '["parcel-1"]', NULL,
		         'MOVE-AUTH/1', now(), 'd')`); err == nil {
		t.Fatal("一行「部分承接却没有拒因」溜进了承接库")
	}

	if _, err := pool.Exec(ctx,
		`INSERT INTO transport_fulfillment.disposition_acceptance
			(tenant_id, item_ref, basis, kind, accepted_objects, decline_basis,
			 movement_authority, decided_at, content_digest)
		 VALUES ('tenant-a', 'item-bad-7', 'DISPOSITION/1', 'ACCEPTED', NULL, NULL,
		         'MOVE-AUTH/1', now(), 'd')`); err == nil {
		t.Fatal("一行「对象列为 NULL」按 jsonb 三值缝溜进了承接库")
	}
}

func newDispositionAcceptances(t *testing.T) (*adapter.DispositionAcceptances, bentoapp.Transactor, *pgxpool.Pool) {
	t.Helper()

	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	store, err := adapter.NewDispositionAcceptances(db)
	if err != nil {
		t.Fatalf("构造承接库：%v", err)
	}
	return store, db.Transactor(), pool
}

func acceptedDisposition(t *testing.T, tenant, item string) ports.DispositionAcceptanceRecord {
	t.Helper()
	return dispositionRecord(t, tenant, item, domain.DispositionAccepted, []string{"parcel-1", "parcel-2"}, "", "MOVE-AUTH/1")
}

func partialDisposition(t *testing.T, tenant, item string) ports.DispositionAcceptanceRecord {
	t.Helper()
	return dispositionRecord(t, tenant, item, domain.DispositionPartiallyAccepted, []string{"parcel-1"}, "unaccepted-range", "MOVE-AUTH/1")
}

func declinedDisposition(t *testing.T, tenant, item string) ports.DispositionAcceptanceRecord {
	t.Helper()
	return dispositionRecord(t, tenant, item, domain.DispositionDeclined, nil, "declined-all", "")
}

func dispositionRecord(
	t *testing.T,
	tenant, item string,
	kind domain.DispositionAcceptanceKind,
	objects []string,
	decline, authority string,
) ports.DispositionAcceptanceRecord {
	t.Helper()
	tenantID := deliveryValue(t, domain.NewTenantID, tenant)
	itemRef := deliveryValue(t, domain.NewCollaborationItemReference, item)
	accepted := make([]domain.CarriedObjectReference, 0, len(objects))
	for _, raw := range objects {
		accepted = append(accepted, deliveryValue(t, domain.NewCarriedObjectReference, raw))
	}
	spec := domain.RegulatoryTransportDispositionSpec{
		Tenant:          tenantID,
		Item:            itemRef,
		Basis:           deliveryValue(t, domain.NewDispositionBasisReference, "DISPOSITION/decision-1"),
		Kind:            kind,
		AcceptedObjects: accepted,
		DeclineBasis:    decline,
		DecidedAt:       dispositionAt,
	}
	if authority != "" {
		spec.MovementAuthority = deliveryValue(t, domain.NewMovementAuthorityReference, authority)
	}
	decision, err := domain.FormRegulatoryTransportDisposition(spec)
	if err != nil {
		t.Fatalf("构造承接决定：%v", err)
	}
	return ports.DispositionAcceptanceRecord{
		Key:           ports.DispositionAcceptanceKey{Tenant: tenantID, Item: itemRef},
		ContentDigest: "digest-" + item + "-" + kind.String(),
		Decision:      decision,
	}
}
