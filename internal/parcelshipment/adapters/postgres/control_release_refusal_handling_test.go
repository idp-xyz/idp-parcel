package postgres_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证释放未受理册的处理留痕与运营查阅读口（ADR-0293 决定一、二）：处理只追加、一笔占用至多被处理一次、
// 两人同时处理只一笔成立败者读回赢家；册上没有这一行（或属于另一个租户）什么也不写；未处理的行按租户列出并计数，被处理的行
// 从清单里消失而册上原行不变；写口无事务拒跑（PBC-08）。

var refusalHandledAt = time.Date(2026, 10, 27, 9, 0, 0, 0, time.UTC)

type refusalHandlingRig struct {
	db         *bentopg.DB
	register   *adapter.ControlReleaseRefusals
	handlings  *adapter.ControlReleaseRefusalHandlings
	catalogue  *adapter.ControlReleaseRefusalCatalogue
	runInTx    func(*testing.T, func(context.Context) error)
	queryCount func(*testing.T, string, ...any) int
}

func newRefusalHandlingRig(t *testing.T) *refusalHandlingRig {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	register, err := adapter.NewControlReleaseRefusals(db)
	if err != nil {
		t.Fatalf("构造释放未受理册：%v", err)
	}
	handlings, err := adapter.NewControlReleaseRefusalHandlings(db)
	if err != nil {
		t.Fatalf("构造处理留痕：%v", err)
	}
	catalogue, err := adapter.NewControlReleaseRefusalCatalogue(db)
	if err != nil {
		t.Fatalf("构造运营查阅读口：%v", err)
	}
	return &refusalHandlingRig{
		db: db, register: register, handlings: handlings, catalogue: catalogue,
		runInTx: func(t *testing.T, fn func(context.Context) error) {
			t.Helper()
			mustWithinTransaction(t, db.Transactor(), t.Context(), fn)
		},
		queryCount: func(t *testing.T, sql string, args ...any) int {
			t.Helper()
			var n int
			if err := pool.QueryRow(t.Context(), sql, args...).Scan(&n); err != nil {
				t.Fatalf("计数 %q：%v", sql, err)
			}
			return n
		},
	}
}

func (rig *refusalHandlingRig) record(t *testing.T, refusal ports.ControlReleaseRefusal) {
	t.Helper()
	rig.runInTx(t, func(ctx context.Context) error {
		_, err := rig.register.Record(ctx, refusal)
		return err
	})
}

func (rig *refusalHandlingRig) handle(
	t *testing.T,
	handling ports.ControlReleaseRefusalHandling,
) (ports.ControlReleaseRefusalHandlingSaveOutcome, ports.ControlReleaseRefusalHandling) {
	t.Helper()
	var (
		outcome ports.ControlReleaseRefusalHandlingSaveOutcome
		winner  ports.ControlReleaseRefusalHandling
	)
	rig.runInTx(t, func(ctx context.Context) error {
		var err error
		outcome, winner, err = rig.handlings.HandleControlReleaseRefusal(ctx, handling)
		return err
	})
	return outcome, winner
}

func handlingOf(
	t *testing.T,
	refusal ports.ControlReleaseRefusal,
	kind ports.ControlReleaseRefusalHandlingKind,
	handler string,
) ports.ControlReleaseRefusalHandling {
	t.Helper()
	return ports.ControlReleaseRefusalHandling{
		Key:       ports.KeyOfRefusal(refusal),
		Kind:      kind,
		Handler:   mustBuild(t, domain.NewControlReleaseRefusalHandlerReference, handler),
		Basis:     mustBuild(t, domain.NewControlReleaseRefusalHandlingBasis, "SYN-CHECKED-AGAINST-LEDGER"),
		HandledAt: refusalHandledAt,
	}
}

func refusalFor(t *testing.T, tenant, request, version string, recordedAt time.Time, scope domain.OccupationScope) ports.ControlReleaseRefusal {
	t.Helper()
	refusal := refusalOf(t, tenant, version, request+"/"+version, scope)
	refusal.ShipmentRequestID = mustBuild(t, domain.NewShipmentRequestID, request)
	refusal.RecordedAt = recordedAt
	return refusal
}

func syntheticScope(t *testing.T) domain.OccupationScope {
	t.Helper()
	scope, err := domain.NewOccupationScope("SYN-LEGAL-ENTITY-1", "SYN-ACCOUNT-1", "CNY")
	if err != nil {
		t.Fatalf("new occupation scope: %v", err)
	}
	return scope
}

// Covers: ADR-0293 决定二——处理留痕只追加：同一笔占用被处理第二次答`已处理`并读回先到的那一份（不覆盖），册上原行一字不变；另一笔不受影响。
func TestAControlReleaseRefusalIsHandledOnceAndTheSecondCallReadsTheFirst(t *testing.T) {
	rig := newRefusalHandlingRig(t)
	scope := syntheticScope(t)
	first := refusalFor(t, "tenant-1", "request-1", "version-1", refusalRecordedAt, scope)
	other := refusalFor(t, "tenant-1", "request-2", "version-1", refusalRecordedAt.Add(time.Minute), domain.OccupationScope{})
	rig.record(t, first)
	rig.record(t, other)

	released := handlingOf(t, first, ports.ControlReleaseRefusalReleasedAtSettlement, "https://id.syn.example#alice")
	outcome, winner := rig.handle(t, released)
	if outcome != ports.ControlReleaseRefusalHandlingRecorded || winner != released {
		t.Fatalf("首次处理 = %d %+v, want 已记下且原样", outcome, winner)
	}
	marked := handlingOf(t, first, ports.ControlReleaseRefusalMarkedHandled, "https://id.syn.example#bob")
	outcome, winner = rig.handle(t, marked)
	if outcome != ports.ControlReleaseRefusalAlreadyHandled {
		t.Fatalf("第二次处理 = %d, want 已处理——一笔占用至多被处理一次", outcome)
	}
	if winner != released {
		t.Fatalf("败者读回 %+v, want 先到的那一份 %+v", winner, released)
	}
	if n := rig.queryCount(t, `SELECT count(*) FROM parcel_shipment.control_release_refusal_handling`); n != 1 {
		t.Fatalf("处理留痕行数 = %d, want 1", n)
	}

	read, found, err := rig.handlings.FindControlReleaseRefusalHandling(t.Context(), ports.KeyOfRefusal(first))
	if err != nil || !found || read != released {
		t.Fatalf("读回处理 = %+v found=%v err=%v, want %+v", read, found, err, released)
	}
	if _, found, err := rig.handlings.FindControlReleaseRefusalHandling(t.Context(), ports.KeyOfRefusal(other)); err != nil || found {
		t.Fatalf("没处理过的那一笔读到了处理 found=%v err=%v", found, err)
	}

	// 册上原行不改：仍能按委托列出，三段作用域与原因照旧。
	listed, err := rig.register.ListByShipmentRequest(t.Context(), first.TenantID, first.ShipmentRequestID)
	if err != nil || len(listed) != 1 || listed[0] != first {
		t.Fatalf("册上原行 = %+v err=%v, want 不变 %+v", listed, err, first)
	}
}

// Covers: ADR-0293 决定二、ADR-0003——处理按租户把关：另一个租户拿着同一笔占用的标识处理，答`册上无此笔`、什么也不写；本租户的这一笔仍未处理。
func TestAnotherTenantCannotHandleARefusalAndWritesNothing(t *testing.T) {
	rig := newRefusalHandlingRig(t)
	refusal := refusalFor(t, "tenant-1", "request-1", "version-1", refusalRecordedAt, syntheticScope(t))
	rig.record(t, refusal)

	foreign := handlingOf(t, refusal, ports.ControlReleaseRefusalMarkedHandled, "https://id.syn.example#mallory")
	foreign.Key.TenantID = psTenant(t, "tenant-2")
	outcome, winner := rig.handle(t, foreign)
	if outcome != ports.ControlReleaseRefusalNotRegistered || winner != (ports.ControlReleaseRefusalHandling{}) {
		t.Fatalf("另一租户处理 = %d %+v, want 册上无此笔", outcome, winner)
	}
	if n := rig.queryCount(t, `SELECT count(*) FROM parcel_shipment.control_release_refusal_handling`); n != 0 {
		t.Fatalf("处理留痕行数 = %d, want 0——别的租户的处理写进了库", n)
	}
	listed, err := rig.catalogue.ListUnhandledControlReleaseRefusals(t.Context(), psTenant(t, "tenant-1"), 10)
	if err != nil || listed.Count != 1 || len(listed.Refusals) != 1 {
		t.Fatalf("本租户仍应有一笔未处理：%+v err=%v", listed, err)
	}

	// 册上根本没有的一笔同答。
	absent := handlingOf(t, refusalFor(t, "tenant-1", "request-9", "version-1", refusalRecordedAt, syntheticScope(t)),
		ports.ControlReleaseRefusalMarkedHandled, "https://id.syn.example#alice")
	if outcome, _ := rig.handle(t, absent); outcome != ports.ControlReleaseRefusalNotRegistered {
		t.Fatalf("册上没有的一笔 = %d, want 册上无此笔", outcome)
	}
}

// Covers: ADR-0293 决定一——运营查阅按租户列未处理的行并计数：被处理的行从清单消失、他租户的行看不见；清单按登记先后、受页大小截断而计数不截断；
// 没存作用域的存量行如实列出（作用域缺席）。
func TestTheCatalogueListsOnlyThisTenantsUnhandledRefusalsWithTheirCount(t *testing.T) {
	rig := newRefusalHandlingRig(t)
	scope := syntheticScope(t)
	oldest := refusalFor(t, "tenant-1", "request-1", "version-1", refusalRecordedAt, domain.OccupationScope{})
	middle := refusalFor(t, "tenant-1", "request-2", "version-1", refusalRecordedAt.Add(time.Minute), scope)
	newest := refusalFor(t, "tenant-1", "request-3", "version-2", refusalRecordedAt.Add(2*time.Minute), scope)
	foreign := refusalFor(t, "tenant-2", "request-1", "version-1", refusalRecordedAt, scope)
	for _, refusal := range []ports.ControlReleaseRefusal{newest, foreign, oldest, middle} {
		rig.record(t, refusal)
	}

	tenant := psTenant(t, "tenant-1")
	listed, err := rig.catalogue.ListUnhandledControlReleaseRefusals(t.Context(), tenant, 10)
	if err != nil {
		t.Fatalf("查阅：%v", err)
	}
	if listed.Count != 3 || len(listed.Refusals) != 3 {
		t.Fatalf("查阅 = %d 行（计数 %d）, want 3 行 3 笔——他租户那一笔不该出现", len(listed.Refusals), listed.Count)
	}
	if listed.Refusals[0] != oldest || listed.Refusals[1] != middle || listed.Refusals[2] != newest {
		t.Fatalf("清单没有按登记先后原样给出：%+v", listed.Refusals)
	}
	if listed.Refusals[0].Scope.Recorded() {
		t.Fatalf("存量行的作用域应缺席：%+v", listed.Refusals[0].Scope)
	}

	truncated, err := rig.catalogue.ListUnhandledControlReleaseRefusals(t.Context(), tenant, 2)
	if err != nil || len(truncated.Refusals) != 2 || truncated.Count != 3 {
		t.Fatalf("页大小 2 = %d 行（计数 %d）err=%v, want 2 行、计数仍为 3", len(truncated.Refusals), truncated.Count, err)
	}
	if truncated.Refusals[0] != oldest || truncated.Refusals[1] != middle {
		t.Fatalf("截断后应留最早的两笔：%+v", truncated.Refusals)
	}

	if outcome, _ := rig.handle(t, handlingOf(t, middle, ports.ControlReleaseRefusalMarkedHandled, "https://id.syn.example#alice")); outcome !=
		ports.ControlReleaseRefusalHandlingRecorded {
		t.Fatalf("处理 = %d, want 已记下", outcome)
	}
	after, err := rig.catalogue.ListUnhandledControlReleaseRefusals(t.Context(), tenant, 10)
	if err != nil || after.Count != 2 || len(after.Refusals) != 2 || after.Refusals[0] != oldest || after.Refusals[1] != newest {
		t.Fatalf("处理之后 = %+v err=%v, want 只剩 oldest 与 newest", after, err)
	}
	other, err := rig.catalogue.ListUnhandledControlReleaseRefusals(t.Context(), psTenant(t, "tenant-2"), 10)
	if err != nil || other.Count != 1 || len(other.Refusals) != 1 || other.Refusals[0] != foreign {
		t.Fatalf("租户 2 = %+v err=%v, want 只有它自己那一笔", other, err)
	}
	empty, err := rig.catalogue.ListUnhandledControlReleaseRefusals(t.Context(), psTenant(t, "tenant-3"), 10)
	if err != nil || empty.Count != 0 || empty.Refusals == nil || len(empty.Refusals) != 0 {
		t.Fatalf("没有行的租户 = %+v err=%v, want 空清单（非 nil）与零计数", empty, err)
	}
	if _, err := rig.catalogue.ListUnhandledControlReleaseRefusals(t.Context(), tenant, 0); err == nil {
		t.Fatal("页大小 0 没有被拒——不设上限的清单会把整本册读出来")
	}
}

// Covers: ADR-0293 决定二「并发：两人同时处理同一行，只一笔成立」——多路同时处理同一笔占用，恰一路答`已记下`，其余读回的赢家是同一份。
func TestConcurrentHandlersOfOneRefusalLeaveExactlyOneWinner(t *testing.T) {
	rig := newRefusalHandlingRig(t)
	refusal := refusalFor(t, "tenant-1", "request-1", "version-1", refusalRecordedAt, syntheticScope(t))
	rig.record(t, refusal)

	const handlers = 6
	type verdict struct {
		outcome ports.ControlReleaseRefusalHandlingSaveOutcome
		winner  ports.ControlReleaseRefusalHandling
		err     error
	}
	verdicts := make([]verdict, handlers)
	var group sync.WaitGroup
	start := make(chan struct{})
	for index := range handlers {
		group.Add(1)
		go func() {
			defer group.Done()
			<-start
			handling := handlingOf(t, refusal, ports.ControlReleaseRefusalMarkedHandled, "https://id.syn.example#handler-"+string(rune('a'+index)))
			verdicts[index].err = rig.db.Transactor().WithinTransaction(t.Context(), func(ctx context.Context) error {
				var err error
				verdicts[index].outcome, verdicts[index].winner, err = rig.handlings.HandleControlReleaseRefusal(ctx, handling)
				return err
			})
		}()
	}
	close(start)
	group.Wait()

	recorded := 0
	var winner ports.ControlReleaseRefusalHandling
	for _, got := range verdicts {
		if got.err != nil {
			t.Fatalf("并发处理出错：%v", got.err)
		}
		if got.outcome == ports.ControlReleaseRefusalHandlingRecorded {
			recorded++
			winner = got.winner
		}
	}
	if recorded != 1 {
		t.Fatalf("成立了 %d 笔, want 恰好 1", recorded)
	}
	for _, got := range verdicts {
		if got.winner != winner {
			t.Fatalf("败者读回 %+v, want 赢家 %+v", got.winner, winner)
		}
	}
	if n := rig.queryCount(t, `SELECT count(*) FROM parcel_shipment.control_release_refusal_handling`); n != 1 {
		t.Fatalf("处理留痕行数 = %d, want 1", n)
	}
}

// Covers: PBC-08 行为面负向证据——处理写口在无事务上下文必须被 RequireExecutor 拒绝；拒绝先于任何入参解读，传零值就够。
func TestHandlingAControlReleaseRefusalRefusesToRunOutsideATransaction(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	handlings, err := adapter.NewControlReleaseRefusalHandlings(db)
	if err != nil {
		t.Fatalf("构造处理留痕：%v", err)
	}
	if _, _, err := handlings.HandleControlReleaseRefusal(t.Context(), ports.ControlReleaseRefusalHandling{}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务处理应返回 ErrTransactionRequired，实得：%v", err)
	}
}

// Covers: ADR-0293 决定二——库内 CHECK 与外键钉住处理留痕的形状：做法是封闭两值、处理人与依据必填、挂在册上有的占用上。
func TestTheHandlingTableRefusesAMalformedRow(t *testing.T) {
	rig := newRefusalHandlingRig(t)
	refusal := refusalFor(t, "tenant-1", "request-1", "version-1", refusalRecordedAt, syntheticScope(t))
	rig.record(t, refusal)

	insert := func(kind, handler, basis, request string) error {
		return rig.db.Transactor().WithinTransaction(t.Context(), func(ctx context.Context) error {
			executor, err := rig.db.RequireExecutor(ctx)
			if err != nil {
				return err
			}
			_, err = executor.Exec(ctx,
				`INSERT INTO parcel_shipment.control_release_refusal_handling
					(tenant_id, shipment_request_id, submission_version, control_result_id,
					 handling_kind, handled_by, handling_basis, handled_at)
				 VALUES ('tenant-1', $4, 'version-1', $4 || '/version-1', $1, $2, $3, now())`,
				kind, handler, basis, request)
			return err
		})
	}
	if err := insert("DELETED", "https://id.syn.example#alice", "SYN-BASIS", "request-1"); err == nil {
		t.Error("做法不在封闭集里的一行进了处理留痕")
	}
	if err := insert("MARKED_HANDLED", " ", "SYN-BASIS", "request-1"); err == nil {
		t.Error("没有处理人的一行进了处理留痕")
	}
	if err := insert("MARKED_HANDLED", "https://id.syn.example#alice", " ", "request-1"); err == nil {
		t.Error("没有依据的一行进了处理留痕")
	}
	if err := insert("MARKED_HANDLED", "https://id.syn.example#alice", "SYN-BASIS", "request-unregistered"); err == nil {
		t.Error("册上没有的占用挂上了处理留痕")
	}
	if err := insert("MARKED_HANDLED", "https://id.syn.example#alice", "SYN-BASIS", "request-1"); err != nil {
		t.Errorf("形状合法的一行被拒了：%v", err)
	}
}
