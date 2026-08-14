package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

var (
	collabDecidedAt  = time.Date(2026, 8, 13, 10, 0, 0, 0, time.UTC)
	collabRecordedAt = time.Date(2026, 8, 13, 10, 5, 0, 0, time.UTC)
	factPerformedAt  = time.Date(2026, 8, 13, 11, 0, 0, 0, time.UTC)
)

func TestAnAcceptanceRoundTripsInAllThreeDecisions(t *testing.T) {
	acceptances, _, transactor, _ := newCollaborationStores(t)
	ctx := t.Context()

	cases := []struct {
		name   string
		record ports.CollaborationAcceptanceRecord
	}{
		{name: "accepted", record: acceptedRecord(t, "tenant-a", "item-1")},
		{name: "declined", record: declinedRecord(t, "tenant-a", "item-2")},
		{name: "partial", record: partialRecord(t, "tenant-a", "item-3")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			inTx(t, transactor, ctx, func(txCtx context.Context) error {
				outcome, err := acceptances.Save(txCtx, tc.record)
				if err != nil {
					return err
				}
				if outcome != ports.AcceptanceSaved {
					t.Fatalf("save outcome = %d", outcome)
				}
				return nil
			})

			found, exists, err := acceptances.FindByKey(ctx, tc.record.Key)
			if err != nil || !exists {
				t.Fatalf("读回失败：err=%v exists=%v", err, exists)
			}
			if found.ContentDigest != tc.record.ContentDigest {
				t.Errorf("digest = %q", found.ContentDigest)
			}
			got := found.Acceptance
			want := tc.record.Acceptance
			if got.Decision() != want.Decision() ||
				got.Node() != want.Node() ||
				got.Authority() != want.Authority() ||
				!got.DecidedAt().Equal(want.DecidedAt()) {
				t.Fatalf("决定头部往返变形：got=%+v", got)
			}
			if len(got.AcceptedUnits()) != len(want.AcceptedUnits()) ||
				len(got.AcceptedActions()) != len(want.AcceptedActions()) {
				t.Fatalf("范围往返变形：units=%d actions=%d",
					len(got.AcceptedUnits()), len(got.AcceptedActions()))
			}
			_, wantBasis := want.Basis()
			_, gotBasis := got.Basis()
			if gotBasis != wantBasis {
				t.Fatal("依据在场性往返变形")
			}
		})
	}
}

func TestASecondAcceptanceWriterGetsAlreadyDecided(t *testing.T) {
	acceptances, _, transactor, _ := newCollaborationStores(t)
	ctx := t.Context()

	first := acceptedRecord(t, "tenant-a", "item-1")
	inTx(t, transactor, ctx, func(txCtx context.Context) error {
		_, err := acceptances.Save(txCtx, first)
		return err
	})

	second := acceptedRecord(t, "tenant-a", "item-1")
	second.ContentDigest = "digest-other-decision"
	var outcome ports.AcceptanceSaveOutcome
	var winner ports.CollaborationAcceptanceRecord
	var winnerFound bool
	inTx(t, transactor, ctx, func(txCtx context.Context) error {
		saved, err := acceptances.Save(txCtx, second)
		if err != nil {
			return err
		}
		outcome = saved
		winner, winnerFound, err = acceptances.FindByKey(txCtx, first.Key)
		return err
	})

	if outcome != ports.AcceptanceAlreadyDecided {
		t.Fatalf("第二份写入结果 = %d，应为 ALREADY_DECIDED", outcome)
	}
	if !winnerFound || winner.ContentDigest != first.ContentDigest {
		t.Fatalf("同事务读回赢家失败：found=%v digest=%q", winnerFound, winner.ContentDigest)
	}
}

func TestAnExecutionFactRoundTripsAndKeepsTheFirstWriter(t *testing.T) {
	_, facts, transactor, _ := newCollaborationStores(t)
	ctx := t.Context()

	first := executionRecord(t, "tenant-a", "item-1", "unit-1", domain.UnsealAction, "evidence-1")
	inTx(t, transactor, ctx, func(txCtx context.Context) error {
		outcome, err := facts.Save(txCtx, first)
		if err != nil {
			return err
		}
		if outcome != ports.ExecutionFactSaved {
			t.Fatalf("save outcome = %d", outcome)
		}
		return nil
	})

	found, exists, err := facts.FindByKey(ctx, first.Key)
	if err != nil || !exists {
		t.Fatalf("读回失败：err=%v exists=%v", err, exists)
	}
	if found.Fact.Evidence().String() != "evidence-1" ||
		found.Fact.Action() != domain.UnsealAction ||
		!found.Fact.PerformedAt().Equal(factPerformedAt) {
		t.Fatalf("事实往返变形：%+v", found.Fact)
	}

	second := executionRecord(t, "tenant-a", "item-1", "unit-1", domain.UnsealAction, "evidence-other")
	second.ContentDigest = "digest-other-evidence"
	var outcome ports.ExecutionFactSaveOutcome
	var winner ports.ExecutionFactRecord
	var winnerFound bool
	inTx(t, transactor, ctx, func(txCtx context.Context) error {
		saved, err := facts.Save(txCtx, second)
		if err != nil {
			return err
		}
		outcome = saved
		winner, winnerFound, err = facts.FindByKey(txCtx, first.Key)
		return err
	})
	if outcome != ports.ExecutionFactAlreadyRecorded {
		t.Fatalf("第二份写入结果 = %d", outcome)
	}
	if !winnerFound || winner.ContentDigest != first.ContentDigest {
		t.Fatalf("同事务读回赢家失败：found=%v digest=%q", winnerFound, winner.ContentDigest)
	}
}

func TestCollaborationRecordsAreInvisibleAcrossTenants(t *testing.T) {
	acceptances, facts, transactor, _ := newCollaborationStores(t)
	ctx := t.Context()

	savedAcceptance := acceptedRecord(t, "tenant-a", "item-shared")
	savedFact := executionRecord(t, "tenant-a", "item-shared", "unit-1", domain.PresentAction, "evidence-1")
	inTx(t, transactor, ctx, func(txCtx context.Context) error {
		if _, err := acceptances.Save(txCtx, savedAcceptance); err != nil {
			return err
		}
		_, err := facts.Save(txCtx, savedFact)
		return err
	})

	otherTenant := ref(t, domain.NewTenantID, "tenant-b")
	_, exists, err := acceptances.FindByKey(ctx, ports.CollaborationAcceptanceKey{
		TenantID: otherTenant, Item: savedAcceptance.Key.Item,
	})
	if err != nil || exists {
		t.Errorf("另一个租户读到了承接决定：exists=%v err=%v", exists, err)
	}
	_, exists, err = facts.FindByKey(ctx, ports.ExecutionFactKey{
		TenantID: otherTenant, Item: savedFact.Key.Item, Unit: savedFact.Key.Unit, Action: savedFact.Key.Action,
	})
	if err != nil || exists {
		t.Errorf("另一个租户读到了执行事实：exists=%v err=%v", exists, err)
	}
}

func TestCollaborationWritesRefuseToRunOutsideATransaction(t *testing.T) {
	acceptances, facts, _, _ := newCollaborationStores(t)
	ctx := t.Context()

	acceptance := acceptedRecord(t, "tenant-a", "item-1")
	if _, err := acceptances.Save(ctx, acceptance); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务保存承接应返回 ErrTransactionRequired，实得：%v", err)
	}
	fact := executionRecord(t, "tenant-a", "item-1", "unit-1", domain.UnsealAction, "evidence-1")
	if _, err := facts.Save(ctx, fact); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务保存事实应返回 ErrTransactionRequired，实得：%v", err)
	}

	if _, exists, err := acceptances.FindByKey(ctx, acceptance.Key); err != nil || exists {
		t.Errorf("被拒绝的承接仍然落库：exists=%v err=%v", exists, err)
	}
	if _, exists, err := facts.FindByKey(ctx, fact.Key); err != nil || exists {
		t.Errorf("被拒绝的事实仍然落库：exists=%v err=%v", exists, err)
	}
}

func TestCollaborationRollbackLeavesNothingBehind(t *testing.T) {
	acceptances, facts, transactor, _ := newCollaborationStores(t)
	ctx := t.Context()
	rollback := errors.New("回滚")

	acceptance := acceptedRecord(t, "tenant-a", "item-1")
	fact := executionRecord(t, "tenant-a", "item-1", "unit-1", domain.UnsealAction, "evidence-1")
	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		if _, err := acceptances.Save(txCtx, acceptance); err != nil {
			return err
		}
		if _, err := facts.Save(txCtx, fact); err != nil {
			return err
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatalf("事务应以回滚错误结束，实得：%v", err)
	}

	if _, exists, err := acceptances.FindByKey(ctx, acceptance.Key); err != nil || exists {
		t.Errorf("回滚后承接仍在：exists=%v err=%v", exists, err)
	}
	if _, exists, err := facts.FindByKey(ctx, fact.Key); err != nil || exists {
		t.Errorf("回滚后事实仍在：exists=%v err=%v", exists, err)
	}
}

func TestCollaborationCheckConstraintsRejectImpossibleRows(t *testing.T) {
	_, _, _, pool := newCollaborationStores(t)
	ctx := t.Context()

	if _, err := pool.Exec(ctx,
		`INSERT INTO node_operations.collaboration_acceptance
			(tenant_id, item_ref, node_ref, decision, authority, basis,
			 accepted_units, accepted_actions, decided_at, content_digest, recorded_at)
		 VALUES ('tenant-a', 'item-bad-1', 'node-1', 'DECLINED', 'auth-1', 'basis-1',
		         '["unit-1"]', '[]', now(), 'digest-1', now())`); err == nil {
		t.Fatal("一行「拒接却带着范围」溜进了承接库")
	}

	if _, err := pool.Exec(ctx,
		`INSERT INTO node_operations.collaboration_acceptance
			(tenant_id, item_ref, node_ref, decision, authority, basis,
			 accepted_units, accepted_actions, decided_at, content_digest, recorded_at)
		 VALUES ('tenant-a', 'item-bad-2', 'node-1', 'ACCEPTED', 'auth-1', NULL,
		         '[]', '["UNSEAL"]', now(), 'digest-1', now())`); err == nil {
		t.Fatal("一行「接受却没有对象范围」溜进了承接库")
	}

	if _, err := pool.Exec(ctx,
		`INSERT INTO node_operations.collaboration_acceptance
			(tenant_id, item_ref, node_ref, decision, authority, basis,
			 accepted_units, accepted_actions, decided_at, content_digest, recorded_at)
		 VALUES ('tenant-a', 'item-bad-3', 'node-1', 'ACCEPTED', 'auth-1', 'basis-1',
		         '["unit-1"]', '["UNSEAL"]', now(), 'digest-1', now())`); err == nil {
		t.Fatal("一行「全量接受却带着依据」溜进了承接库")
	}

	if _, err := pool.Exec(ctx,
		`INSERT INTO node_operations.collaboration_acceptance
			(tenant_id, item_ref, node_ref, decision, authority, basis,
			 accepted_units, accepted_actions, decided_at, content_digest, recorded_at)
		 VALUES ('tenant-a', 'item-bad-4', 'node-1', 'ACCEPTED', 'auth-1', NULL,
		         NULL, '["UNSEAL"]', now(), 'digest-1', now())`); err == nil {
		t.Fatal("一行「接受且对象范围为 NULL」按 jsonb_array_length 三值缝溜进了承接库")
	}

	if _, err := pool.Exec(ctx,
		`INSERT INTO node_operations.collaboration_acceptance
			(tenant_id, item_ref, node_ref, decision, authority, basis,
			 accepted_units, accepted_actions, decided_at, content_digest, recorded_at)
		 VALUES ('tenant-a', 'item-bad-5', 'node-1', 'DECLINED', 'auth-1', NULL,
		         '[]', '[]', now(), 'digest-1', now())`); err == nil {
		t.Fatal("一行「拒接却没有依据」溜进了承接库")
	}

	if _, err := pool.Exec(ctx,
		`INSERT INTO node_operations.execution_fact
			(tenant_id, item_ref, unit_id, action, node_ref, evidence,
			 performed_at, content_digest, recorded_at)
		 VALUES ('tenant-a', 'item-1', 'unit-1', 'RELEASE', 'node-1', 'evidence-1',
		         now(), 'digest-1', now())`); err == nil {
		t.Fatal("一行「放行动作」溜进了执行事实库")
	}
}

func newCollaborationStores(t *testing.T) (*adapter.CollaborationAcceptances, *adapter.ExecutionFacts, bentoapp.Transactor, *pgxpool.Pool) {
	t.Helper()

	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	acceptances, err := adapter.NewCollaborationAcceptances(db)
	if err != nil {
		t.Fatalf("构造承接库：%v", err)
	}
	facts, err := adapter.NewExecutionFacts(db)
	if err != nil {
		t.Fatalf("构造执行事实库：%v", err)
	}
	return acceptances, facts, db.Transactor(), pool
}

func acceptedRecord(t *testing.T, tenant, item string) ports.CollaborationAcceptanceRecord {
	t.Helper()
	spec := collabSpec(t, tenant, item, domain.CollaborationAccepted)
	acceptance, err := domain.DecideCollaborationAcceptance(spec)
	if err != nil {
		t.Fatalf("构造接受：%v", err)
	}
	return ports.CollaborationAcceptanceRecord{
		Key:           ports.CollaborationAcceptanceKey{TenantID: spec.TenantID, Item: spec.Item},
		ContentDigest: "digest-" + item + "-accepted",
		Acceptance:    acceptance,
		RecordedAt:    collabRecordedAt,
	}
}

func declinedRecord(t *testing.T, tenant, item string) ports.CollaborationAcceptanceRecord {
	t.Helper()
	spec := collabSpec(t, tenant, item, domain.CollaborationDeclined)
	acceptance, err := domain.DecideCollaborationAcceptance(spec)
	if err != nil {
		t.Fatalf("构造拒接：%v", err)
	}
	return ports.CollaborationAcceptanceRecord{
		Key:           ports.CollaborationAcceptanceKey{TenantID: spec.TenantID, Item: spec.Item},
		ContentDigest: "digest-" + item + "-declined",
		Acceptance:    acceptance,
		RecordedAt:    collabRecordedAt,
	}
}

func partialRecord(t *testing.T, tenant, item string) ports.CollaborationAcceptanceRecord {
	t.Helper()
	spec := collabSpec(t, tenant, item, domain.CollaborationPartiallyAccepted)
	acceptance, err := domain.DecideCollaborationAcceptance(spec)
	if err != nil {
		t.Fatalf("构造部分承接：%v", err)
	}
	return ports.CollaborationAcceptanceRecord{
		Key:           ports.CollaborationAcceptanceKey{TenantID: spec.TenantID, Item: spec.Item},
		ContentDigest: "digest-" + item + "-partial",
		Acceptance:    acceptance,
		RecordedAt:    collabRecordedAt,
	}
}

func collabSpec(t *testing.T, tenant, item string, decision domain.AcceptanceDecisionKind) domain.CollaborationAcceptanceSpec {
	t.Helper()
	spec := domain.CollaborationAcceptanceSpec{
		TenantID:  ref(t, domain.NewTenantID, tenant),
		Node:      ref(t, domain.NewNodeReference, "node-1"),
		Item:      ref(t, domain.NewCollaborationItemReference, item),
		Decision:  decision,
		Authority: ref(t, domain.NewAcceptanceAuthorityReference, "node-authority-1"),
		DecidedAt: collabDecidedAt,
	}
	if decision != domain.CollaborationDeclined {
		spec.AcceptedUnits = []domain.HandlingUnitID{
			ref(t, domain.NewHandlingUnitID, "unit-1"),
			ref(t, domain.NewHandlingUnitID, "unit-2"),
		}
		spec.AcceptedActions = []domain.CollaborationActionKind{domain.UnsealAction, domain.PresentAction}
	}
	if decision != domain.CollaborationAccepted {
		spec.Basis = ref(t, domain.NewAcceptanceBasisReference, "basis-"+decision.String())
	}
	return spec
}

func executionRecord(
	t *testing.T,
	tenant, item, unit string,
	action domain.CollaborationActionKind,
	evidence string,
) ports.ExecutionFactRecord {
	t.Helper()
	acceptance, err := domain.DecideCollaborationAcceptance(collabSpec(t, tenant, item, domain.CollaborationAccepted))
	if err != nil {
		t.Fatalf("构造承接以登记事实：%v", err)
	}
	fact, err := domain.RecordExecutionFact(
		acceptance,
		ref(t, domain.NewHandlingUnitID, unit),
		action,
		ref(t, domain.NewExecutionEvidenceReference, evidence),
		factPerformedAt,
	)
	if err != nil {
		t.Fatalf("构造执行事实：%v", err)
	}
	return ports.ExecutionFactRecord{
		Key: ports.ExecutionFactKey{
			TenantID: acceptance.TenantID(),
			Item:     acceptance.Item(),
			Unit:     fact.Unit(),
			Action:   action,
		},
		ContentDigest: "digest-" + evidence,
		Fact:          fact,
		RecordedAt:    collabRecordedAt,
	}
}
