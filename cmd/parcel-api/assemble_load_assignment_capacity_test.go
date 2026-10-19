package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
	tfpostgres "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
	tfapp "go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	tfports "go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 本文件在生产端点构造、生产准入桥与生产装配上证票 demo-chain-gaps/21（ADR-0203 决定一、二）：干线 2026-10-21 那一班的重量池
// （5000 KG）上预占 1200 KG 之后，带预占引用的装载分配答 201，超出那份预占剩下的量答 ALLOCATION_EXCEEDED 并带余量；挂了池而
// 不带分配行答 CAPACITY_RESERVATION_REQUIRED；已分配的那一段释放不出来，分配撤回之后释放得出来；同一份预占上两笔并发分配
// 串行判。测试输入是隔离合成，只记 `S`。

const allocationFaceSchedule = "SYN-SCHED-SZX-SIN-DAILY/2026-10-21"

type allocationFaceFixture struct {
	pool        *pgxpool.Pool
	db          *bentopg.DB
	endpoint    http.Handler
	capacity    transactionalCapacityDecisions
	assignments *tfpostgres.LoadAssignments
}

func newAllocationFaceFixture(t *testing.T) allocationFaceFixture {
	t.Helper()
	pool, db := capacityDB(t)
	registerOperationDecisionAdmission(t, db, isolatedSelfAuthority, accessidentity.DecisionLoadAssignment)
	admission, err := buildAdmissionScope(db, isolatedSelfAuthority)
	if err != nil {
		t.Fatal(err)
	}
	minter, err := buildOperatorMinter(acceptingVerifier{},
		decisionRegister{kinds: []accessidentity.DecisionKind{accessidentity.DecisionLoadAssignment}}, admission)
	if err != nil {
		t.Fatal(err)
	}
	decisions, err := buildOperatorDecisionIntakes(minter, unwiredDecisionTargets{}, unwiredDecisionTargets{})
	if err != nil {
		t.Fatal(err)
	}
	operations, err := buildSegmentOperations(db)
	if err != nil {
		t.Fatal(err)
	}
	capacity, err := buildCapacityDecisions(db)
	if err != nil {
		t.Fatal(err)
	}
	assignments, err := tfpostgres.NewLoadAssignments(db)
	if err != nil {
		t.Fatal(err)
	}
	return allocationFaceFixture{
		pool:        pool,
		db:          db,
		endpoint:    tfhttp.NewFormLoadAssignmentEndpoint(decisions.transport, operations.assign),
		capacity:    capacity,
		assignments: assignments,
	}
}

func allocationFaceBody(assignment, allocations string) string {
	return `{"assignment": "` + assignment + `", "schedule": "` + allocationFaceSchedule + `",` +
		` "members": ["SYN-PARCEL-1", "SYN-PARCEL-2"], "version": "SYN-LAV-1", "assignedAt": "2026-10-19T10:00:00Z",` +
		` "allocations": [` + allocations + `]}`
}

func allocationFaceLine(quantity string) string {
	return `{"pool": "` + capacityPool + `", "reservation": "SYN-RES-1", "quantity": ` + quantity + `, "unit": "KG",` +
		` "source": {"kind": "OPERATOR_DECLARATION", "reference": "SYN-DEMAND-DECL-01/v1"}}`
}

func (fixture allocationFaceFixture) post(t *testing.T, body string) (int, map[string]json.RawMessage, string) {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/transport-fulfillment-load-assignment-registrations", strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer presented.operator.token")
	response := httptest.NewRecorder()
	fixture.endpoint.ServeHTTP(response, request)
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(response.Body.Bytes(), &fields)
	return response.Code, fields, response.Body.String()
}

func (fixture allocationFaceFixture) release(t *testing.T, release string, quantity int64) tfapp.OpportunityResult {
	t.Helper()
	result, err := fixture.capacity.Release(t.Context(), tfapp.ReleaseCapacityCommand{
		TenantID: chargeValue(t, tfdomain.NewTenantID, capacityTenant), Pool: capacityPool, Reservation: "SYN-RES-1",
		Release: release, Quantity: quantity, Unit: "KG", SourceKind: "OPERATOR_DECLARATION",
		SourceReference: "SYN-DEMAND-DECL-01/v2", At: capacityReservedAt.Add(3 * time.Hour),
	})
	if err != nil {
		t.Fatalf("释放 %s：%v", release, err)
	}
	return result
}

// Covers: 票 demo-chain-gaps/21（AT-TF-171、AT-TF-173、AT-TF-175、AT-TF-180）——演示那一步在真库上走通：预占 1200 KG 之后，
// 带预占引用分 1000 KG 答 201 并回显分配量；另一分配要 201 KG 答 200 ALLOCATION_EXCEEDED、余量 200、那份预占已分配 1000；
// 不带分配行答 200 CAPACITY_RESERVATION_REQUIRED；释放 201 KG 撞守恒；第一个分配撤回之后，整份 1200 KG 释放得出来。
func TestTheWiredLoadAssignmentAllocatesAgainstAReservationOnARealDatabase(t *testing.T) {
	fixture := newAllocationFaceFixture(t)
	result, err := fixture.capacity.Reserve(t.Context(), capacityReserveCommand(t, "SYN-RES-1", 1200))
	wantCapacityOutcome(t, "预占", result, err, tfapp.CapacityReserved)

	code, fields, body := fixture.post(t, allocationFaceBody("SYN-LA-1", allocationFaceLine("1000")))
	if code != http.StatusCreated || string(fields["outcome"]) != `"LOAD_ASSIGNMENT_FORMED"` ||
		!strings.Contains(string(fields["allocations"]), `"quantity":"1000"`) {
		t.Fatalf("带预占引用的分配 = %d %s", code, body)
	}

	code, fields, body = fixture.post(t, allocationFaceBody("SYN-LA-2", allocationFaceLine("201")))
	if code != http.StatusOK || string(fields["outcome"]) != `"ALLOCATION_EXCEEDED"` || string(fields["availableQuantity"]) != `"200"` ||
		!strings.Contains(string(fields["reservation"]), `"allocated":"1000"`) {
		t.Fatalf("超出余量 = %d %s", code, body)
	}

	code, fields, body = fixture.post(t, allocationFaceBody("SYN-LA-3", ""))
	if code != http.StatusOK || string(fields["outcome"]) != `"CAPACITY_RESERVATION_REQUIRED"` ||
		!strings.Contains(string(fields["poolsWithoutReservation"]), capacityPool) {
		t.Fatalf("不带分配行 = %d %s", code, body)
	}

	if released := fixture.release(t, "SYN-REL-1", 201); released.Outcome() != tfapp.ReservationOverdrawnOutcome {
		t.Fatalf("释放进已分配的那一段：%q, want RESERVATION_OVERDRAWN", released.Outcome())
	}

	key := tfports.LoadAssignmentKey{
		TenantID:   chargeValue(t, tfdomain.NewTenantID, capacityTenant),
		Assignment: chargeValue(t, tfdomain.NewLoadAssignmentReference, "SYN-LA-1"),
		Version:    chargeValue(t, tfdomain.NewLoadAssignmentVersion, "SYN-LAV-1"),
	}
	formed, exists, err := fixture.assignments.FindByKey(t.Context(), key)
	if err != nil || !exists {
		t.Fatalf("读回 SYN-LA-1：exists=%v err=%v", exists, err)
	}
	// 撤回入口还没接（tf-unwired-seven/05）：经领域撤回、生产登记册落下撤回版，模拟入口接上之后的那一笔。
	withdrawn, err := formed.Assignment.Withdraw(chargeValue(t, tfdomain.NewLoadAssignmentVersion, "SYN-LAV-2"), capacityReservedAt.Add(2*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if err := fixture.db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		withdrawalKey := key
		withdrawalKey.Version = withdrawn.Version()
		_, saveErr := fixture.assignments.Save(txCtx, tfports.LoadAssignmentRecord{
			Key: withdrawalKey, Assignment: withdrawn, RecordedAt: capacityReservedAt.Add(2 * time.Hour),
		})
		return saveErr
	}); err != nil {
		t.Fatalf("落撤回版：%v", err)
	}
	if released := fixture.release(t, "SYN-REL-1", 1200); released.Outcome() != tfapp.CapacityReleased {
		t.Fatalf("撤回之后释放整份：%q, want CAPACITY_RELEASED", released.Outcome())
	}
}

// pausingAssignments 是生产装载分配册外面一层：第一次登记之后先跑 afterSave——此刻握着各池行的锁、分配量还没提交。
type pausingAssignments struct {
	tfports.LoadAssignmentRegistry
	afterSave func()
}

func (assignments pausingAssignments) Save(ctx context.Context, record tfports.LoadAssignmentRecord) (tfports.LoadAssignmentSaveOutcome, error) {
	saved, err := assignments.LoadAssignmentRegistry.Save(ctx, record)
	if assignments.afterSave != nil {
		assignments.afterSave()
	}
	return saved, err
}

// Covers: 票 demo-chain-gaps/21（AT-TF-175 的并发半边）——同一份预占（1200 KG）上两笔并发分配各 700 KG：A 落了分配量、还没提交；
// B 经生产装配形成分配，卡在池那一行的锁上，A 提交后读到含 A 的分配关系，答 ALLOCATION_EXCEEDED、余量 500。读池不锁时，
// B 会按没有 A 的预占放行，两笔合计 1400 KG 超出预占。
func TestConcurrentAllocationsOnOneReservationHoldTheConservation(t *testing.T) {
	fixture := newAllocationFaceFixture(t)
	result, err := fixture.capacity.Reserve(t.Context(), capacityReserveCommand(t, "SYN-RES-1", 1200))
	wantCapacityOutcome(t, "预占", result, err, tfapp.CapacityReserved)
	operations, err := buildSegmentOperations(fixture.db)
	if err != nil {
		t.Fatal(err)
	}
	schedules, err := tfpostgres.NewTransportSchedules(fixture.db)
	if err != nil {
		t.Fatal(err)
	}
	pools, err := tfpostgres.NewCapacityPools(fixture.db)
	if err != nil {
		t.Fatal(err)
	}
	command := func(assignment string) tfapp.FormLoadAssignmentCommand {
		return tfapp.FormLoadAssignmentCommand{
			TenantID: chargeValue(t, tfdomain.NewTenantID, capacityTenant), Assignment: assignment, Schedule: allocationFaceSchedule,
			Members: []string{"SYN-PARCEL-" + assignment}, Version: "SYN-LAV-1", AssignedAt: capacityReservedAt.Add(time.Hour),
			Allocations: []tfapp.LoadAssignmentAllocationLine{{
				Pool: capacityPool, Reservation: "SYN-RES-1", Quantity: 700, Unit: "KG",
				SourceKind: "OPERATOR_DECLARATION", SourceReference: "SYN-DEMAND-DECL-01/v1",
			}},
		}
	}
	var (
		second     tfapp.FormLoadAssignmentResult
		secondErr  error
		secondDone chan struct{}
		waited     bool
	)
	paused := pausingAssignments{LoadAssignmentRegistry: fixture.assignments}
	paused.afterSave = func() {
		if secondDone != nil {
			return
		}
		secondDone = make(chan struct{})
		go func(done chan struct{}) {
			defer close(done)
			second, secondErr = operations.assign.Form(t.Context(), command("SYN-LA-B"))
		}(secondDone)
		waited = awaitPoolLockWaiter(t, fixture.pool, secondDone)
	}
	first := transactionalLoadAssigner{
		transactor: fixture.db.Transactor(),
		inner: tfapp.NewFormLoadAssignmentHandler(tfapp.FormLoadAssignmentDeps{
			Assignments: paused, Schedules: schedules, Pools: pools, Clock: systemClock{},
		}),
	}
	formed, err := first.Form(t.Context(), command("SYN-LA-A"))
	if secondDone == nil {
		t.Fatalf("A 没走到登记那一步：%q %v", formed.Outcome(), err)
	}
	awaitDone(t, secondDone)
	if err != nil || formed.Outcome() != tfapp.LoadAssignmentFormed {
		t.Fatalf("A：%q %v", formed.Outcome(), err)
	}
	if !waited {
		t.Fatal("B 没在池那一行的锁上等就走完了")
	}
	if secondErr != nil || second.Outcome() != tfapp.LoadAssignmentAllocationExceeded || second.AvailableQuantity() != 500 {
		t.Fatalf("B：%q 余量 %d %v, want ALLOCATION_EXCEEDED 500", second.Outcome(), second.AvailableQuantity(), secondErr)
	}
	var rows int
	if err := fixture.pool.QueryRow(t.Context(), `SELECT count(*) FROM transport_fulfillment.load_assignment_allocation`).Scan(&rows); err != nil || rows != 1 {
		t.Fatalf("分配行 %d（%v），want 只有 A 那一行", rows, err)
	}
}

// Covers: 装配点在装载分配答`未决`时回滚再交回那一份答复——池上一份预占读回过不了重建门（已释放列与明细对不上，库面不钉
// 这一条）：带预占引用的分配答 200 LOAD_ASSIGNMENT_UNDECIDED 带续办引用，不是 500，库上一个版本也没有。
func TestTheWiredLoadAssignmentIsUndecidedWhenThePoolDoesNotRehydrate(t *testing.T) {
	fixture := newAllocationFaceFixture(t)
	if _, err := fixture.pool.Exec(t.Context(),
		`INSERT INTO transport_fulfillment.capacity_reservation
			(tenant_id, pool_id, reservation_id, quantity, valid_until, released, consumed, source_kind, source_ref, reserved_at)
		 VALUES ($1, $2, 'SYN-RES-1', 1200, $3, 5, 0, 'OPERATOR_DECLARATION', 'D', $4)`,
		capacityTenant, capacityPool, capacityValidUntil, capacityReservedAt); err != nil {
		t.Fatalf("插探针行：%v", err)
	}
	code, fields, body := fixture.post(t, allocationFaceBody("SYN-LA-1", allocationFaceLine("10")))
	if code != http.StatusOK || string(fields["outcome"]) != `"LOAD_ASSIGNMENT_UNDECIDED"` || fields["continuationReference"] == nil {
		t.Fatalf("读不回的池 = %d %s", code, body)
	}
	var versions int
	if err := fixture.pool.QueryRow(t.Context(), `SELECT count(*) FROM transport_fulfillment.load_assignment`).Scan(&versions); err != nil || versions != 0 {
		t.Fatalf("版本 %d 行（%v），want 0", versions, err)
	}
}
