package postgres_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	adapter "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/outboxintent"
)

// 本文件对真实 PostgreSQL 16 证「取消被认定不成立」意图（ADR-0295 决定四）：信封与决定留痕同一事务（一起成立或一起消失）、一份冲突至多一封（重放
// 不翻倍，同包裹另一份冲突另一封）、只为「认定取消不成立」入队、无事务拒跑（PBC-08）、缺项响亮报错。事件类型字面在这里再写一遍而不导入提供方常量。

const cancellationNotUpheldEventType = "parcel-shipment.parcel-cancellation.not-upheld"

var cancellationNotUpheldNow = conflictDecidedAt.Add(time.Hour)

type notUpheldClock struct{ at time.Time }

func (clock notUpheldClock) Now() time.Time { return clock.at }

type notUpheldFixture struct {
	rig        *conflictDecisionRig
	handoff    *adapter.OutboxCancellationNotUpheldHandoff
	store      *outbox.Store
	transactor bentoapp.Transactor
}

func newNotUpheldFixture(t *testing.T) *notUpheldFixture {
	t.Helper()
	rig := newConflictDecisionRig(t)
	store, err := outbox.NewStore(rig.db)
	if err != nil {
		t.Fatalf("构造 Outbox Store：%v", err)
	}
	handoff, err := adapter.NewOutboxCancellationNotUpheldHandoff(rig.db, store, notUpheldClock{at: cancellationNotUpheldNow})
	if err != nil {
		t.Fatalf("构造续办交接：%v", err)
	}
	return &notUpheldFixture{rig: rig, handoff: handoff, store: store, transactor: rig.db.Transactor()}
}

func (fixture *notUpheldFixture) countIntents(t *testing.T, key ports.CancellationOrderConflictKey) int {
	t.Helper()
	eventID := string(outboxintent.FingerprintEventID("cancellation-not-upheld",
		key.TenantID.String(), key.Parcel.String(), key.BoundaryKind.String(), key.BoundaryRef.String(), key.Cancellation.String()))
	return fixture.rig.countRows(t,
		`SELECT count(*) FROM `+migrate.SchemaBento+`.outbox WHERE event_id = $1 AND event_type = $2`,
		eventID, cancellationNotUpheldEventType)
}

func (fixture *notUpheldFixture) conflict(t *testing.T, parcel, reference string) ports.CancellationOrderConflictKey {
	t.Helper()
	return fixture.rig.recordConflict(t, "tenant-1",
		conflictOf(t, parcel, "PCXL-1", reference, domain.NetworkIntakeBoundary, cancelRequestedAt, cancelRequestedAt.Add(-time.Hour)), conflictDetectedAt)
}

// Covers: AT-PS-170；ADR-0094 决定四、ADR-0295 决定四——决定留痕与续办信封同一事务成立；载荷指名租户、包裹、取消与冲突的边界那一代。
func TestTheNotUpheldEnvelopeCommitsWithTheDecisionAndNamesTheConflict(t *testing.T) {
	fixture := newNotUpheldFixture(t)
	key := fixture.conflict(t, "parcel-1", "NODE_INTAKE/v1")
	decision := decisionOf(t, key, domain.CancellationNotUpheld, "https://id.syn.example#alice")

	var outcome ports.CancellationOrderConflictDecisionSaveOutcome
	if err := fixture.transactor.WithinTransaction(t.Context(), func(ctx context.Context) error {
		var err error
		if outcome, _, err = fixture.rig.decisions.DecideCancellationOrderConflict(ctx, decision); err != nil {
			return err
		}
		return fixture.handoff.HandOffCancellationNotUpheld(ctx, ports.CancellationNotUpheldHandoffIntent{Decision: decision})
	}); err != nil {
		t.Fatalf("事务内写入失败：%v", err)
	}
	if outcome != ports.CancellationOrderConflictDecisionRecorded {
		t.Fatalf("留痕 = %d, want 已记下", outcome)
	}

	if count := fixture.countIntents(t, key); count != 1 {
		t.Fatalf("outbox 行数 = %d, want 1", count)
	}
	if _, found, err := fixture.rig.decisions.FindConflictDecision(t.Context(), key); err != nil || !found {
		t.Fatalf("决定没落库 found=%v err=%v", found, err)
	}
	envelope := claimEnvelopeOfType(t, fixture.store, cancellationNotUpheldNow.Add(time.Hour), cancellationNotUpheldEventType)
	var payload struct {
		TenantID     string `json:"tenantId"`
		Parcel       string `json:"parcel"`
		Cancellation string `json:"cancellation"`
		BoundaryKind string `json:"boundaryKind"`
		BoundaryRef  string `json:"boundaryRef"`
	}
	if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
		t.Fatalf("解载荷：%v", err)
	}
	if payload.TenantID != "tenant-1" || payload.Parcel != "parcel-1" || payload.Cancellation != "PCXL-1" ||
		payload.BoundaryKind != "NETWORK_INTAKE" || payload.BoundaryRef != "NODE_INTAKE/v1" {
		t.Fatalf("载荷 = %+v", payload)
	}
	if !envelope.OccurredAt.Equal(decision.DecidedAt) || envelope.Subject != "parcel-1" || envelope.PartitionKey != "tenant-1/parcel-1/PCXL-1/NETWORK_INTAKE/NODE_INTAKE/v1" {
		t.Fatalf("occurredAt = %s subject = %q partition = %q, want 决定成立的时刻、包裹与按冲突五维的分区", envelope.OccurredAt, envelope.Subject, envelope.PartitionKey)
	}
}

// Covers: AT-PS-170；ADR-0094 决定四的另一半——一起消失：回滚之后决定与信封都不在，那份冲突仍是未解开。
func TestARolledBackDecisionLeavesNeitherTheRowNorTheEnvelope(t *testing.T) {
	fixture := newNotUpheldFixture(t)
	key := fixture.conflict(t, "parcel-1", "NODE_INTAKE/v1")
	decision := decisionOf(t, key, domain.CancellationNotUpheld, "https://id.syn.example#alice")
	rollback := errors.New("回滚")

	if err := fixture.transactor.WithinTransaction(t.Context(), func(ctx context.Context) error {
		if _, _, err := fixture.rig.decisions.DecideCancellationOrderConflict(ctx, decision); err != nil {
			return err
		}
		if err := fixture.handoff.HandOffCancellationNotUpheld(ctx, ports.CancellationNotUpheldHandoffIntent{Decision: decision}); err != nil {
			return err
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatalf("事务应以回滚错误结束，实得：%v", err)
	}
	if _, found, err := fixture.rig.decisions.FindConflictDecision(t.Context(), key); err != nil || found {
		t.Fatalf("回滚后决定仍在 found=%v err=%v", found, err)
	}
	if count := fixture.countIntents(t, key); count != 0 {
		t.Fatalf("回滚后 outbox 行数 = %d, want 0", count)
	}
}

// Covers: AT-PS-170；ADR-0043——重放重发同一份；同一包裹上另一份冲突的决定是另一封，不被当成重放吞掉。
func TestTheNotUpheldEnvelopeIsOnePerConflict(t *testing.T) {
	fixture := newNotUpheldFixture(t)
	first := fixture.conflict(t, "parcel-1", "NODE_INTAKE/v1")
	second := fixture.conflict(t, "parcel-1", "NODE_INTAKE/v2")
	send := func(key ports.CancellationOrderConflictKey) {
		if err := fixture.transactor.WithinTransaction(t.Context(), func(ctx context.Context) error {
			return fixture.handoff.HandOffCancellationNotUpheld(ctx, ports.CancellationNotUpheldHandoffIntent{
				Decision: decisionOf(t, key, domain.CancellationNotUpheld, "https://id.syn.example#alice")})
		}); err != nil {
			t.Fatalf("入队：%v", err)
		}
	}
	send(first)
	send(first)
	send(second)
	if count := fixture.countIntents(t, first); count != 1 {
		t.Fatalf("同一份冲突的信封 = %d 封, want 1——重发的必须是同一份", count)
	}
	if count := fixture.countIntents(t, second); count != 1 {
		t.Fatalf("另一份冲突的信封 = %d 封, want 1——不该被当成第一份的重放吞掉", count)
	}
}

// Covers: PBC-08 行为面——写口在无事务上下文必须被拒绝，落库行数为零；缺项与不是「认定取消不成立」的意图都响亮报错、不入信封。
func TestTheNotUpheldEnvelopeRefusesToRunOutsideATransactionAndRefusesWhatItCannotName(t *testing.T) {
	fixture := newNotUpheldFixture(t)
	key := fixture.conflict(t, "parcel-1", "NODE_INTAKE/v1")
	good := decisionOf(t, key, domain.CancellationNotUpheld, "https://id.syn.example#alice")

	if err := fixture.handoff.HandOffCancellationNotUpheld(t.Context(), ports.CancellationNotUpheldHandoffIntent{Decision: good}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("无事务入队应返回 ErrTransactionRequired，实得：%v", err)
	}
	referred := decisionOf(t, key, domain.BoundaryFactReferred, "https://id.syn.example#alice")
	noCancellation := good
	noCancellation.Key.Cancellation = domain.ParcelCancellationID{}
	for name, decision := range map[string]ports.CancellationOrderConflictDecision{
		"交来源方更正不续办": referred,
		"缺取消":       noCancellation,
		"全缺":        {},
	} {
		if err := fixture.transactor.WithinTransaction(t.Context(), func(ctx context.Context) error {
			return fixture.handoff.HandOffCancellationNotUpheld(ctx, ports.CancellationNotUpheldHandoffIntent{Decision: decision})
		}); err == nil {
			t.Errorf("%s：必须响亮报错", name)
		}
	}
	if count := fixture.countIntents(t, key); count != 0 {
		t.Fatalf("被拒绝的意图仍入了 %d 封", count)
	}
}
