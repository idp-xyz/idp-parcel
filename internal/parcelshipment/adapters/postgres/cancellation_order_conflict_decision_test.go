package postgres_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证取消先后冲突的决定留痕、运营查阅读口、有效取消读与不采用记录作废（ADR-0295）：决定只追加、一份冲突至多被决定
// 一次、两人同时决定只一笔成立败者读回赢家；册上没有这份冲突（或属于另一个租户）什么也不写；未解开的冲突按租户列出并计数，被决定的行从清单里消失而
// 冲突行不变；被认定不成立的取消不是有效取消而取消记录仍在；作废只动依据这份取消的不采用行；写口无事务拒跑（PBC-08）。

var conflictDecidedAt = time.Date(2026, 10, 27, 9, 0, 0, 0, time.UTC)

type conflictDecisionRig struct {
	db            *bentopg.DB
	register      *adapter.CancellationOrderConflicts
	decisions     *adapter.CancellationOrderConflictDecisions
	catalogue     *adapter.CancellationOrderConflictCatalogue
	cancellations *adapter.ParcelCancellations
	finals        *adapter.FinalOutcomes
	replays       *adapter.CancellationBlockedReplays
	pool          *pgxpool.Pool
	countRows     func(*testing.T, string, ...any) int
}

func newConflictDecisionRig(t *testing.T) *conflictDecisionRig {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	register, err := adapter.NewCancellationOrderConflicts(db)
	if err != nil {
		t.Fatalf("构造冲突册：%v", err)
	}
	decisions, err := adapter.NewCancellationOrderConflictDecisions(db)
	if err != nil {
		t.Fatalf("构造决定留痕：%v", err)
	}
	catalogue, err := adapter.NewCancellationOrderConflictCatalogue(db)
	if err != nil {
		t.Fatalf("构造运营查阅读口：%v", err)
	}
	cancellations, err := adapter.NewParcelCancellations(db)
	if err != nil {
		t.Fatalf("构造取消库：%v", err)
	}
	finals, err := adapter.NewFinalOutcomes(db)
	if err != nil {
		t.Fatalf("构造终局库：%v", err)
	}
	replays, err := adapter.NewCancellationBlockedReplays(db)
	if err != nil {
		t.Fatalf("构造回放读口：%v", err)
	}
	return &conflictDecisionRig{
		db: db, register: register, decisions: decisions, catalogue: catalogue,
		cancellations: cancellations, finals: finals, replays: replays, pool: pool,
		countRows: func(t *testing.T, sql string, args ...any) int {
			t.Helper()
			var n int
			if err := pool.QueryRow(t.Context(), sql, args...).Scan(&n); err != nil {
				t.Fatalf("计数 %q：%v", sql, err)
			}
			return n
		},
	}
}

func (rig *conflictDecisionRig) inTx(t *testing.T, fn func(context.Context) error) {
	t.Helper()
	mustWithinTransaction(t, rig.db.Transactor(), t.Context(), fn)
}

// conflictOf 判出一份取消先后冲突：取消 cancellationID 的业务时间是 cancelledAt，边界事实 reference 发生在 boundaryAt（不晚于取消）。
func conflictOf(
	t *testing.T,
	parcel, cancellationID, reference string,
	kind domain.CancellationBoundaryKind,
	cancelledAt, boundaryAt time.Time,
) domain.CancellationOrderConflict {
	t.Helper()
	cancellation, err := domain.DecideParcelCancellation(domain.ParcelCancellationSpec{
		ID:          mustBuild(t, domain.NewParcelCancellationID, cancellationID),
		Parcel:      mustBuild(t, domain.NewDeclaredParcelID, parcel),
		Requester:   mustBuild(t, domain.NewCancellationRequesterReference, "customer-a"),
		Authority:   mustBuild(t, domain.NewCancellationAuthorityReference, "CANCEL-RULE/1"),
		Reason:      mustBuild(t, domain.NewCancellationReasonReference, "CHANGED_MIND"),
		RequestedAt: cancelledAt,
	}, domain.CurrentIntakeFact{})
	if err != nil {
		t.Fatalf("decide cancellation: %v", err)
	}
	conflict, conflicted, err := domain.JudgeCancellationOrder(cancellation, domain.CancellationBoundaryFact{
		Kind:       kind,
		Reference:  mustBuild(t, domain.NewCancellationBoundaryReference, reference),
		OccurredAt: boundaryAt,
	})
	if err != nil || !conflicted {
		t.Fatalf("judge order: conflicted = %v err = %v", conflicted, err)
	}
	return conflict
}

func (rig *conflictDecisionRig) recordConflict(
	t *testing.T,
	tenant string,
	conflict domain.CancellationOrderConflict,
	detectedAt time.Time,
) ports.CancellationOrderConflictKey {
	t.Helper()
	rig.inTx(t, func(ctx context.Context) error {
		_, err := rig.register.Record(ctx, ports.CancellationOrderConflictRecord{
			TenantID: psTenant(t, tenant), Conflict: conflict, DetectedAt: detectedAt,
		})
		return err
	})
	return conflictKeyOf(t, tenant, conflict)
}

func conflictKeyOf(t *testing.T, tenant string, conflict domain.CancellationOrderConflict) ports.CancellationOrderConflictKey {
	t.Helper()
	return ports.CancellationOrderConflictKey{
		TenantID:     psTenant(t, tenant),
		Parcel:       conflict.Parcel(),
		BoundaryKind: conflict.Boundary().Kind,
		BoundaryRef:  conflict.Boundary().Reference,
		Cancellation: conflict.Cancellation(),
	}
}

func decisionOf(
	t *testing.T,
	key ports.CancellationOrderConflictKey,
	kind domain.CancellationOrderConflictDecisionKind,
	decider string,
) ports.CancellationOrderConflictDecision {
	t.Helper()
	return ports.CancellationOrderConflictDecision{
		Key:       key,
		Kind:      kind,
		Decider:   mustBuild(t, domain.NewCancellationOrderConflictDeciderReference, decider),
		Basis:     mustBuild(t, domain.NewCancellationOrderConflictDecisionBasis, "SYN-CHECKED-AGAINST-NODE-SCAN"),
		DecidedAt: conflictDecidedAt,
	}
}

func (rig *conflictDecisionRig) decide(
	t *testing.T,
	decision ports.CancellationOrderConflictDecision,
) (ports.CancellationOrderConflictDecisionSaveOutcome, ports.CancellationOrderConflictDecision) {
	t.Helper()
	var (
		outcome ports.CancellationOrderConflictDecisionSaveOutcome
		winner  ports.CancellationOrderConflictDecision
	)
	rig.inTx(t, func(ctx context.Context) error {
		var err error
		outcome, winner, err = rig.decisions.DecideCancellationOrderConflict(ctx, decision)
		return err
	})
	return outcome, winner
}

// Covers: AT-PS-166；ADR-0295 决定一——决定只追加：同一份冲突被决定第二次答`已决定`并读回先到的那一份（不覆盖，即便第二次说的是另一种去向），
// 冲突行一字不变；另一份冲突不受影响；册上没有的冲突与别的租户的冲突答`册上无此份`、什么也不写。
func TestAConflictIsDecidedOnceAndTheSecondCallReadsTheFirst(t *testing.T) {
	rig := newConflictDecisionRig(t)
	first := conflictOf(t, "parcel-1", "PCXL-1", "NODE_INTAKE/v1", domain.NetworkIntakeBoundary, cancelRequestedAt, cancelRequestedAt.Add(-time.Hour))
	other := conflictOf(t, "parcel-2", "PCXL-2", "NODE_INTAKE/v1", domain.NetworkIntakeBoundary, cancelRequestedAt, cancelRequestedAt.Add(-time.Hour))
	firstKey := rig.recordConflict(t, "tenant-1", first, conflictDetectedAt)
	otherKey := rig.recordConflict(t, "tenant-1", other, conflictDetectedAt)

	notUpheld := decisionOf(t, firstKey, domain.CancellationNotUpheld, "https://id.syn.example#alice")
	outcome, winner := rig.decide(t, notUpheld)
	if outcome != ports.CancellationOrderConflictDecisionRecorded || winner != notUpheld {
		t.Fatalf("首次决定 = %d %+v, want 已记下且原样", outcome, winner)
	}
	referred := decisionOf(t, firstKey, domain.BoundaryFactReferred, "https://id.syn.example#bob")
	outcome, winner = rig.decide(t, referred)
	if outcome != ports.CancellationOrderConflictAlreadyDecided || winner != notUpheld {
		t.Fatalf("第二次决定 = %d %+v, want 已决定并读回先到的那一份，不被覆盖", outcome, winner)
	}
	if read, found, err := rig.decisions.FindConflictDecision(t.Context(), firstKey); err != nil || !found || read != notUpheld {
		t.Fatalf("读回 = %+v found=%v err=%v", read, found, err)
	}
	if _, found, err := rig.decisions.FindConflictDecision(t.Context(), otherKey); err != nil || found {
		t.Fatalf("另一份冲突被连带决定了 found=%v err=%v", found, err)
	}

	entries, err := rig.register.ListByParcel(t.Context(), psTenant(t, "tenant-1"), first.Parcel())
	if err != nil || len(entries) != 1 || !entries[0].DetectedAt.Equal(conflictDetectedAt) ||
		!entries[0].CancelledAt.Equal(cancelRequestedAt) {
		t.Fatalf("冲突行被决定改动了：%+v err=%v", entries, err)
	}

	unknown := conflictKeyOf(t, "tenant-1", conflictOf(t, "parcel-9", "PCXL-9", "NODE_INTAKE/v1", domain.NetworkIntakeBoundary, cancelRequestedAt, cancelRequestedAt))
	foreign := firstKey
	foreign.TenantID = psTenant(t, "tenant-2")
	for name, key := range map[string]ports.CancellationOrderConflictKey{"册上没有": unknown, "别的租户": foreign} {
		outcome, _ := rig.decide(t, decisionOf(t, key, domain.CancellationNotUpheld, "https://id.syn.example#alice"))
		if outcome != ports.CancellationOrderConflictNotRegistered {
			t.Fatalf("%s：决定 = %d, want 册上无此份", name, outcome)
		}
	}
	if n := rig.countRows(t, `SELECT count(*) FROM parcel_shipment.cancellation_order_conflict_decision`); n != 1 {
		t.Fatalf("决定行数 = %d, want 1——无此份的决定没有写任何东西", n)
	}

	if _, _, err := rig.decisions.DecideCancellationOrderConflict(t.Context(), notUpheld); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务决定应返回 ErrTransactionRequired，实得：%v", err)
	}
}

// Covers: AT-PS-166；ADR-0295 决定一——六路并发决定同一份冲突，恰一笔成立，其余读回赢家。
func TestSixConcurrentDecisionsOfOneConflictRecordExactlyOne(t *testing.T) {
	rig := newConflictDecisionRig(t)
	conflict := conflictOf(t, "parcel-1", "PCXL-1", "NODE_INTAKE/v1", domain.NetworkIntakeBoundary, cancelRequestedAt, cancelRequestedAt.Add(-time.Hour))
	key := rig.recordConflict(t, "tenant-1", conflict, conflictDetectedAt)

	const callers = 6
	var (
		wg        sync.WaitGroup
		mu        sync.Mutex
		recorded  int
		already   int
		failures  []error
		decisions = make([]ports.CancellationOrderConflictDecision, callers)
	)
	for index := range callers {
		kind := domain.CancellationNotUpheld
		if index%2 == 1 {
			kind = domain.BoundaryFactReferred
		}
		decisions[index] = decisionOf(t, key, kind, "https://id.syn.example#caller-"+string(rune('a'+index)))
	}
	for index := range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := rig.db.Transactor().WithinTransaction(t.Context(), func(ctx context.Context) error {
				outcome, _, err := rig.decisions.DecideCancellationOrderConflict(ctx, decisions[index])
				if err != nil {
					return err
				}
				mu.Lock()
				defer mu.Unlock()
				switch outcome {
				case ports.CancellationOrderConflictDecisionRecorded:
					recorded++
				case ports.CancellationOrderConflictAlreadyDecided:
					already++
				}
				return nil
			})
			if err != nil {
				mu.Lock()
				failures = append(failures, err)
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if len(failures) != 0 || recorded != 1 || already != callers-1 {
		t.Fatalf("recorded=%d already=%d failures=%v, want 恰一笔成立其余读回赢家", recorded, already, failures)
	}
}

// Covers: AT-PS-167；ADR-0295 决定一——运营查阅按租户列未解开的冲突并计数：最早发现的在前；被决定的行从清单里消失而册上冲突行不变；别的租户的
// 冲突不出现；页大小截断时计数仍是全部未解开的行数；页大小非正数拒。
func TestTheCatalogueListsTheTenantsUnresolvedConflictsOldestFirst(t *testing.T) {
	rig := newConflictDecisionRig(t)
	keys := make([]ports.CancellationOrderConflictKey, 3)
	for index := range keys {
		parcel := "parcel-" + string(rune('1'+index))
		keys[index] = rig.recordConflict(t, "tenant-1",
			conflictOf(t, parcel, "PCXL-"+parcel, "NODE_INTAKE/v1", domain.NetworkIntakeBoundary, cancelRequestedAt, cancelRequestedAt.Add(-time.Hour)),
			conflictDetectedAt.Add(time.Duration(2-index)*time.Minute))
	}
	rig.recordConflict(t, "tenant-2",
		conflictOf(t, "parcel-x", "PCXL-x", "NODE_INTAKE/v1", domain.NetworkIntakeBoundary, cancelRequestedAt, cancelRequestedAt.Add(-time.Hour)), conflictDetectedAt)

	listed, err := rig.catalogue.ListUnresolvedCancellationOrderConflicts(t.Context(), psTenant(t, "tenant-1"), 10)
	if err != nil || listed.Count != 3 || len(listed.Conflicts) != 3 {
		t.Fatalf("清单 = %+v err=%v, want 三行", listed, err)
	}
	if got := listed.Conflicts[0].Parcel.String(); got != "parcel-3" {
		t.Fatalf("第一行 = %s, want 最早发现的 parcel-3", got)
	}

	rig.decide(t, decisionOf(t, keys[2], domain.BoundaryFactReferred, "https://id.syn.example#alice"))
	listed, err = rig.catalogue.ListUnresolvedCancellationOrderConflicts(t.Context(), psTenant(t, "tenant-1"), 10)
	if err != nil || listed.Count != 2 || len(listed.Conflicts) != 2 || listed.Conflicts[0].Parcel.String() != "parcel-2" {
		t.Fatalf("决定后清单 = %+v err=%v, want 被决定的行消失", listed, err)
	}
	if n := rig.countRows(t, `SELECT count(*) FROM parcel_shipment.cancellation_order_conflict WHERE tenant_id = 'tenant-1'`); n != 3 {
		t.Fatalf("冲突行数 = %d, want 3——决定不改册", n)
	}

	truncated, err := rig.catalogue.ListUnresolvedCancellationOrderConflicts(t.Context(), psTenant(t, "tenant-1"), 1)
	if err != nil || truncated.Count != 2 || len(truncated.Conflicts) != 1 {
		t.Fatalf("截断清单 = %+v err=%v, want 计数 2、清单 1 行", truncated, err)
	}
	empty, err := rig.catalogue.ListUnresolvedCancellationOrderConflicts(t.Context(), psTenant(t, "tenant-3"), 10)
	if err != nil || empty.Count != 0 || empty.Conflicts == nil || len(empty.Conflicts) != 0 {
		t.Fatalf("空租户清单 = %+v err=%v", empty, err)
	}
	if _, err := rig.catalogue.ListUnresolvedCancellationOrderConflicts(t.Context(), psTenant(t, "tenant-1"), 0); err == nil {
		t.Fatal("页大小 0 没有被拒")
	}
}

// Covers: AT-PS-167；ADR-0295 决定一——未解开的读法按冲突行的五维逐维对账：与已决定的那一行只差一维（别的包裹、别的边界种类、别的边界那一代、别的取消、
// 别的租户）的冲突行仍在清单里。尤其同一包裹同一边界那一代、两份取消：（取消 X，边界 R）被决定之后，同一来源再被另一份取消 Y 挡住产生（Y，R），
// 不能因 X 已决定就把它当成已解开而藏掉——藏掉它，这件被挡住的来源就没有人再看见。
func TestTheCatalogueKeepsAConflictThatDiffersFromADecidedOneInASingleDimension(t *testing.T) {
	rig := newConflictDecisionRig(t)
	at, before := cancelRequestedAt, cancelRequestedAt.Add(-time.Hour)
	decidedKey := rig.recordConflict(t, "tenant-1",
		conflictOf(t, "parcel-1", "PCXL-X", "NODE_INTAKE/v1", domain.NetworkIntakeBoundary, at, before), conflictDetectedAt)
	variants := map[string]domain.CancellationOrderConflict{
		"别的包裹":    conflictOf(t, "parcel-2", "PCXL-X", "NODE_INTAKE/v1", domain.NetworkIntakeBoundary, at, before),
		"别的边界种类":  conflictOf(t, "parcel-1", "PCXL-X", "NODE_INTAKE/v1", domain.CarrierFirstEffectivePickupBoundary, at, before),
		"别的边界那一代": conflictOf(t, "parcel-1", "PCXL-X", "NODE_INTAKE/v2", domain.NetworkIntakeBoundary, at, before),
		"别的取消":    conflictOf(t, "parcel-1", "PCXL-Y", "NODE_INTAKE/v1", domain.NetworkIntakeBoundary, at, before),
	}
	for _, conflict := range variants {
		rig.recordConflict(t, "tenant-1", conflict, conflictDetectedAt.Add(time.Minute))
	}
	// 别的租户上与已决定那一行五维同形的冲突行：它自己没有被决定。
	rig.recordConflict(t, "tenant-3",
		conflictOf(t, "parcel-1", "PCXL-X", "NODE_INTAKE/v1", domain.NetworkIntakeBoundary, at, before), conflictDetectedAt)
	rig.decide(t, decisionOf(t, decidedKey, domain.CancellationNotUpheld, "https://id.syn.example#alice"))

	listed, err := rig.catalogue.ListUnresolvedCancellationOrderConflicts(t.Context(), psTenant(t, "tenant-1"), 10)
	if err != nil || listed.Count != len(variants) || len(listed.Conflicts) != len(variants) {
		t.Fatalf("tenant-1 清单 = %+v err=%v, want 只差一维的 %d 行都在", listed, err, len(variants))
	}
	for name, conflict := range variants {
		found := false
		for _, entry := range listed.Conflicts {
			found = found || (entry.Parcel == conflict.Parcel() && entry.Cancellation == conflict.Cancellation() &&
				entry.Boundary.Kind == conflict.Boundary().Kind && entry.Boundary.Reference == conflict.Boundary().Reference)
		}
		if !found {
			t.Errorf("%s：这一行与已决定的只差一维，却被藏出了清单", name)
		}
	}
	other, err := rig.catalogue.ListUnresolvedCancellationOrderConflicts(t.Context(), psTenant(t, "tenant-3"), 10)
	if err != nil || other.Count != 1 || len(other.Conflicts) != 1 {
		t.Fatalf("tenant-3 清单 = %+v err=%v, want 别的租户的决定不藏本租户的冲突行", other, err)
	}
}

func (rig *conflictDecisionRig) saveCancellation(t *testing.T, tenant, requestKey, parcel, cancellationID string, requestedAt time.Time) {
	t.Helper()
	key := cancellationKey(t, tenant, requestKey, parcel)
	cancellation, err := domain.DecideParcelCancellation(domain.ParcelCancellationSpec{
		ID:          mustBuild(t, domain.NewParcelCancellationID, cancellationID),
		Parcel:      key.Parcel,
		Requester:   mustBuild(t, domain.NewCancellationRequesterReference, "customer-a"),
		Authority:   mustBuild(t, domain.NewCancellationAuthorityReference, "cancel-rule-3"),
		Reason:      mustBuild(t, domain.NewCancellationReasonReference, "order-withdrawn"),
		RequestedAt: requestedAt,
	}, domain.CurrentIntakeFact{})
	if err != nil {
		t.Fatalf("形成取消决定：%v", err)
	}
	rig.inTx(t, func(ctx context.Context) error {
		_, err := rig.cancellations.Save(ctx, ports.CancellationRecord{
			Key: key, ContentDigest: "digest-" + cancellationID, Kind: ports.RecordParcelCancelled,
			Cancellation: cancellation, DecidedAt: adoptionRecordAt,
		})
		return err
	})
}

// Covers: AT-PS-168；ADR-0295 决定三——被认定不成立的取消不是有效取消（有效取消读排除它，取消记录仍在表里作历史），同包裹后一份有效取消补上；
// 交来源方更正的决定不动取消；别的租户同一包裹号的取消不受影响。
func TestACancellationDecidedNotUpheldIsNoLongerTheEffectiveCancellation(t *testing.T) {
	rig := newConflictDecisionRig(t)
	rig.saveCancellation(t, "tenant-1", "req-1", "parcel-1", "PCXL-1", cancelRequestedAt)
	rig.saveCancellation(t, "tenant-1", "req-2", "parcel-1", "PCXL-2", cancelRequestedAt.Add(time.Hour))
	rig.saveCancellation(t, "tenant-2", "req-1", "parcel-1", "PCXL-1", cancelRequestedAt)
	tenant, parcel := psTenant(t, "tenant-1"), mustBuild(t, domain.NewDeclaredParcelID, "parcel-1")

	effective := func(tenant domain.TenantID) string {
		t.Helper()
		cancellation, found, err := rig.cancellations.FindCancellation(t.Context(), tenant, parcel)
		if err != nil {
			t.Fatalf("读有效取消：%v", err)
		}
		if !found {
			return ""
		}
		return cancellation.ID().String()
	}
	if got := effective(tenant); got != "PCXL-1" {
		t.Fatalf("决定前有效取消 = %q, want 最早的 PCXL-1", got)
	}

	referredKey := rig.recordConflict(t, "tenant-1",
		conflictOf(t, "parcel-1", "PCXL-1", "NODE_INTAKE/v1", domain.NetworkIntakeBoundary, cancelRequestedAt, cancelRequestedAt.Add(-time.Hour)), conflictDetectedAt)
	rig.decide(t, decisionOf(t, referredKey, domain.BoundaryFactReferred, "https://id.syn.example#alice"))
	if got := effective(tenant); got != "PCXL-1" {
		t.Fatalf("交来源方更正之后有效取消 = %q, want 取消保持有效 PCXL-1", got)
	}

	otherKey := rig.recordConflict(t, "tenant-1",
		conflictOf(t, "parcel-1", "PCXL-1", "OFFSITE_PICKUP/v1", domain.NetworkIntakeBoundary, cancelRequestedAt, cancelRequestedAt.Add(-time.Minute)), conflictDetectedAt)
	rig.decide(t, decisionOf(t, otherKey, domain.CancellationNotUpheld, "https://id.syn.example#alice"))
	if got := effective(tenant); got != "PCXL-2" {
		t.Fatalf("认定不成立之后有效取消 = %q, want 补上的后一份 PCXL-2", got)
	}
	if got := effective(psTenant(t, "tenant-2")); got != "PCXL-1" {
		t.Fatalf("别的租户的有效取消 = %q, want 不受影响的 PCXL-1", got)
	}
	if n := rig.countRows(t, `SELECT count(*) FROM parcel_shipment.parcel_cancellation WHERE tenant_id = 'tenant-1' AND kind = 'PARCEL_CANCELLED'`); n != 2 {
		t.Fatalf("取消记录行数 = %d, want 2——取消作历史仍在", n)
	}
}

func refusalRecordOf(t *testing.T, tenant, parcel, version, basis string) ports.FinalOutcomeRecord {
	t.Helper()
	return ports.FinalOutcomeRecord{
		Key:           finalKey(t, tenant, parcel, domain.EffectiveDeliveryOutcome, version),
		ContentDigest: "digest-" + version,
		RefusalBasis:  mustBuild(t, domain.NewCheckReason, basis),
		AdoptedAt:     adoptionRecordAt,
	}
}

func (rig *conflictDecisionRig) saveFinal(t *testing.T, record ports.FinalOutcomeRecord) {
	t.Helper()
	var outcome ports.FinalOutcomeSaveOutcome
	rig.inTx(t, func(ctx context.Context) error {
		var err error
		outcome, err = rig.finals.Save(ctx, record)
		return err
	})
	if outcome != ports.FinalOutcomeSaved {
		t.Fatalf("保存终局采用 = %d", outcome)
	}
}

// Covers: AT-PS-169；ADR-0295 决定三——作废只动依据这份取消的不采用行：依据另一份取消的、依据已有终局的、已形成的终局、别的租户同一包裹号的行一概不碰；
// 作废之后同一来源版本可以重新落一份记录；无事务拒跑（PBC-08）。
func TestVoidingRemovesOnlyTheRefusalsGroundedOnThatCancellation(t *testing.T) {
	rig := newConflictDecisionRig(t)
	rig.saveFinal(t, refusalRecordOf(t, "tenant-1", "parcel-1", "v1", "PARCEL_CANCELLATION/PCXL-1"))
	rig.saveFinal(t, refusalRecordOf(t, "tenant-1", "parcel-1", "v2", "PARCEL_CANCELLATION/PCXL-2"))
	rig.saveFinal(t, refusalRecordOf(t, "tenant-1", "parcel-1", "v3", "FINAL_ALREADY_FORMED/FIN-0001"))
	rig.saveFinal(t, refusalRecordOf(t, "tenant-2", "parcel-1", "v1", "PARCEL_CANCELLATION/PCXL-1"))
	rig.saveFinal(t, firstFinalRecord(t, "tenant-1", "parcel-9", "v1", "digest-final"))

	var voided int
	rig.inTx(t, func(ctx context.Context) error {
		var err error
		voided, err = rig.finals.VoidCancellationRefusedFinals(ctx, psTenant(t, "tenant-1"),
			mustBuild(t, domain.NewDeclaredParcelID, "parcel-1"), mustBuild(t, domain.NewParcelCancellationID, "PCXL-1"))
		return err
	})
	if voided != 1 {
		t.Fatalf("作废 %d 行, want 1", voided)
	}
	for _, expectation := range []struct {
		tenant, parcel, version string
		present                 bool
	}{
		{"tenant-1", "parcel-1", "v1", false},
		{"tenant-1", "parcel-1", "v2", true},
		{"tenant-1", "parcel-1", "v3", true},
		{"tenant-2", "parcel-1", "v1", true},
		{"tenant-1", "parcel-9", "v1", true},
	} {
		_, found, err := rig.finals.FindByKey(t.Context(), finalKey(t, expectation.tenant, expectation.parcel, domain.EffectiveDeliveryOutcome, expectation.version))
		if err != nil || found != expectation.present {
			t.Errorf("%s/%s/%s found=%v err=%v, want present=%v", expectation.tenant, expectation.parcel, expectation.version, found, err, expectation.present)
		}
	}
	rig.saveFinal(t, refusalRecordOf(t, "tenant-1", "parcel-1", "v1", "FINAL_ALREADY_FORMED/FIN-0002"))

	if _, err := rig.finals.VoidCancellationRefusedFinals(t.Context(), psTenant(t, "tenant-1"),
		mustBuild(t, domain.NewDeclaredParcelID, "parcel-1"), mustBuild(t, domain.NewParcelCancellationID, "PCXL-2")); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务作废应返回 ErrTransactionRequired，实得：%v", err)
	}
}

func intakeReplaySpec(t *testing.T, parcel, version, corrects string) domain.IntakeSourceSpec {
	t.Helper()
	spec := domain.IntakeSourceSpec{
		Kind:       domain.NodeIntakeSource,
		Object:     mustBuild(t, domain.NewSourceObjectReference, "HU-1"),
		Parcel:     mustBuild(t, domain.NewDeclaredParcelID, parcel),
		Place:      mustBuild(t, domain.NewIntakePlaceReference, "NODE-1"),
		Control:    mustBuild(t, domain.NewIntakeControlReference, "NODE-INTAKE/EVID-1"),
		Version:    mustBuild(t, domain.NewSourceResultVersion, version),
		OccurredAt: cancelRequestedAt.Add(-time.Hour),
	}
	if corrects != "" {
		spec.Corrects = mustBuild(t, domain.NewSourceResultVersion, corrects)
	}
	return spec
}

func finalReplaySpec(t *testing.T, parcel, version string) domain.ResponsibilityOutcomeSpec {
	t.Helper()
	return domain.ResponsibilityOutcomeSpec{
		Kind:       domain.EffectiveDeliveryOutcome,
		Parcel:     mustBuild(t, domain.NewDeclaredParcelID, parcel),
		Decision:   mustBuild(t, domain.NewResponsibilityDecisionReference, "DELIVERY-JUDGMENT/"+parcel+"/ATT-1"),
		Execution:  mustBuild(t, domain.NewExecutionEvidenceReference, "POD-1"),
		Version:    mustBuild(t, domain.NewResponsibilityOutcomeVersion, version),
		OccurredAt: cancelRequestedAt.Add(2 * time.Hour),
	}
}

// Covers: AT-PS-170；ADR-0295 决定四——被取消挡住的来源把回放原料与挡住它的那一行同笔落库：收寄采用的冲突行带来源规格（含自报更正的那一版），
// 终局采用的不采用行带责任结果规格；读回逐项经领域构造器复验、与写入逐字相同；同一来源重投不翻倍；原料指着别的取消、挂在已形成的终局上、两件都缺
// 一律拒；回滚后原料与行一起消失。
func TestABlockedSourceCarriesItsReplayMaterialWithTheRecordThatBlockedIt(t *testing.T) {
	rig := newConflictDecisionRig(t)
	tenant := psTenant(t, "tenant-1")
	parcel := mustBuild(t, domain.NewDeclaredParcelID, "parcel-1")
	cancellation := mustBuild(t, domain.NewParcelCancellationID, "PCXL-1")

	intake := intakeReplaySpec(t, "parcel-1", "v2", "v1")
	conflict := conflictOf(t, "parcel-1", "PCXL-1", "NODE_INTAKE/v2", domain.NetworkIntakeBoundary, cancelRequestedAt, intake.OccurredAt)
	record := func(replay *ports.CancellationBlockedReplay) error {
		return rig.db.Transactor().WithinTransaction(t.Context(), func(ctx context.Context) error {
			_, err := rig.register.Record(ctx, ports.CancellationOrderConflictRecord{
				TenantID: tenant, Conflict: conflict, DetectedAt: conflictDetectedAt, Replay: replay,
			})
			return err
		})
	}
	if err := record(&ports.CancellationBlockedReplay{Cancellation: cancellation, Intake: &intake}); err != nil {
		t.Fatalf("记冲突与原料：%v", err)
	}
	if err := record(&ports.CancellationBlockedReplay{Cancellation: cancellation, Intake: &intake}); err != nil {
		t.Fatalf("重投：%v", err)
	}

	final := finalReplaySpec(t, "parcel-1", "DLV-v1")
	refusal := refusalRecordOf(t, "tenant-1", "parcel-1", "DLV-v1", "PARCEL_CANCELLATION/PCXL-1")
	refusal.CancellationReplay = &ports.CancellationBlockedReplay{Cancellation: cancellation, Final: &final}
	rig.saveFinal(t, refusal)

	listed, err := rig.replays.ListCancellationBlockedReplays(t.Context(), tenant, parcel, cancellation)
	if err != nil || len(listed) != 2 {
		t.Fatalf("读回 %d 份 err=%v, want 两份（重投不翻倍）", len(listed), err)
	}
	var gotIntake *domain.IntakeSourceSpec
	var gotFinal *domain.ResponsibilityOutcomeSpec
	for _, replay := range listed {
		if replay.Cancellation != cancellation {
			t.Fatalf("原料的取消 = %s", replay.Cancellation)
		}
		gotIntake, gotFinal = firstNonNil(gotIntake, replay.Intake), firstNonNilFinal(gotFinal, replay.Final)
	}
	if gotIntake == nil || *gotIntake != intake {
		t.Fatalf("收寄来源规格往返变形：%+v want %+v", gotIntake, intake)
	}
	if gotFinal == nil || *gotFinal != final {
		t.Fatalf("责任结果规格往返变形：%+v want %+v", gotFinal, final)
	}
	if other, err := rig.replays.ListCancellationBlockedReplays(t.Context(), psTenant(t, "tenant-2"), parcel, cancellation); err != nil || len(other) != 0 {
		t.Fatalf("别的租户读到 %d 份（err=%v）", len(other), err)
	}

	wrongCancellation := mustBuild(t, domain.NewParcelCancellationID, "PCXL-9")
	if err := record(&ports.CancellationBlockedReplay{Cancellation: wrongCancellation, Intake: &intake}); err == nil {
		t.Error("原料指着别的取消，没有被拒")
	}
	if err := record(&ports.CancellationBlockedReplay{Cancellation: cancellation}); err == nil {
		t.Error("原料两件都缺，没有被拒")
	}
	finalized := firstFinalRecord(t, "tenant-1", "parcel-7", "v1", "digest-final")
	finalized.CancellationReplay = &ports.CancellationBlockedReplay{Cancellation: cancellation, Final: &final}
	if err := rig.db.Transactor().WithinTransaction(t.Context(), func(ctx context.Context) error {
		_, err := rig.finals.Save(ctx, finalized)
		return err
	}); err == nil {
		t.Error("原料挂在已形成的终局上，没有被拒")
	}

	rolledBack := errors.New("回滚")
	laterFinal := finalReplaySpec(t, "parcel-1", "DLV-v2")
	laterRefusal := refusalRecordOf(t, "tenant-1", "parcel-1", "DLV-v2", "PARCEL_CANCELLATION/PCXL-1")
	laterRefusal.CancellationReplay = &ports.CancellationBlockedReplay{Cancellation: cancellation, Final: &laterFinal}
	if err := rig.db.Transactor().WithinTransaction(t.Context(), func(ctx context.Context) error {
		if _, err := rig.finals.Save(ctx, laterRefusal); err != nil {
			return err
		}
		return rolledBack
	}); !errors.Is(err, rolledBack) {
		t.Fatalf("事务应以回滚错误结束，实得：%v", err)
	}
	after, err := rig.replays.ListCancellationBlockedReplays(t.Context(), tenant, parcel, cancellation)
	if err != nil || len(after) != 2 {
		t.Fatalf("回滚后原料 %d 份 err=%v, want 仍是两份", len(after), err)
	}
}

func firstNonNil(current, candidate *domain.IntakeSourceSpec) *domain.IntakeSourceSpec {
	if current != nil {
		return current
	}
	return candidate
}

func firstNonNilFinal(current, candidate *domain.ResponsibilityOutcomeSpec) *domain.ResponsibilityOutcomeSpec {
	if current != nil {
		return current
	}
	return candidate
}

// Covers: AT-PS-166；库内 CHECK 与外键——决定不挂在册上没有的冲突上，去向取封闭两值，决定人与依据不得为空白。
func TestTheDecisionTableRejectsWhatTheRegisterCannotHold(t *testing.T) {
	rig := newConflictDecisionRig(t)
	rig.recordConflict(t, "tenant-1",
		conflictOf(t, "parcel-1", "PCXL-1", "NODE_INTAKE/v1", domain.NetworkIntakeBoundary, cancelRequestedAt, cancelRequestedAt.Add(-time.Hour)), conflictDetectedAt)
	insert := func(parcel, kind, decider, basis string) error {
		_, err := rig.pool.Exec(t.Context(),
			`INSERT INTO parcel_shipment.cancellation_order_conflict_decision
				(tenant_id, parcel_id, boundary_kind, boundary_ref, cancellation_id,
				 decision_kind, decided_by, decision_basis, decided_at)
			 VALUES ('tenant-1', $1, 'NETWORK_INTAKE', 'NODE_INTAKE/v1', 'PCXL-1', $2, $3, $4, now())`,
			parcel, kind, decider, basis)
		return err
	}
	if err := insert("parcel-none", "CANCELLATION_NOT_UPHELD", "alice", "basis"); err == nil {
		t.Error("决定挂在册上没有的冲突上，外键没有拦")
	}
	if err := insert("parcel-1", "DELETE_THE_CANCELLATION", "alice", "basis"); err == nil {
		t.Error("去向取了封闭集合之外的词，CHECK 没有拦")
	}
	if err := insert("parcel-1", "CANCELLATION_NOT_UPHELD", " ", "basis"); err == nil {
		t.Error("决定人空白，CHECK 没有拦")
	}
	if err := insert("parcel-1", "CANCELLATION_NOT_UPHELD", "alice", " "); err == nil {
		t.Error("依据空白，CHECK 没有拦")
	}
	if err := insert("parcel-1", "CANCELLATION_NOT_UPHELD", "alice", "basis"); err != nil {
		t.Errorf("合法决定被拒：%v", err)
	}
}
