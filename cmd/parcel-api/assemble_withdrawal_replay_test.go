package main

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	psidentity "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/identity"
	pspostgres "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	shipmentapp "go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 证撤回编排的重放（ADR-0256 决定一第 2 格「重放须接着办」，UC-PS-005「撤回未决」一行）：
// 撤回的装配事务在编排交回业务答案时照样提交来源保全，所以停在授权未决、读不回或并发抢先的撤回，来源已落库而委托
// 仍是`已提交`。重放同一请求要接着办形成撤回，且只形成一次；委托已越过提交边界时才答已有决定并带那份决定。
// 装配与 buildWithdrawalOrchestration 同形（真来源仓储、真委托仓储、真判断账、真决定标识、事务壳），只有撤回授权与
// 释放口是替身：授权的请求映射是实例半边，生产装配里还没有租户取值，走不到授权放行之后的格；没有占用的委托也不发释放。

// scriptedWithdrawalAuthorizer 是撤回授权替身（隔离合成 `S`）：能拨成答不出，放行时带一份合成的授权引用。
type scriptedWithdrawalAuthorizer struct {
	authority domain.WithdrawalAuthorityReference
	err       error
	calls     atomic.Int32
}

func newScriptedWithdrawalAuthorizer(t *testing.T) *scriptedWithdrawalAuthorizer {
	t.Helper()
	return &scriptedWithdrawalAuthorizer{authority: mustValue(t, domain.NewWithdrawalAuthorityReference, "SYN-WITHDRAW-ROLE-1")}
}

func (double *scriptedWithdrawalAuthorizer) AuthorizeWithdrawal(
	context.Context,
	ports.WithdrawalAuthorizationQuery,
) (ports.WithdrawalAuthorization, error) {
	double.calls.Add(1)
	if double.err != nil {
		return ports.WithdrawalAuthorization{}, double.err
	}
	return ports.WithdrawalAuthorization{Outcome: ports.AuthorizationGranted, Authority: double.authority}, nil
}

// noReleaseExpected 在被调用时让用例失败：这些委托从未形成占用，撤回不该向结算发任何释放。
type noReleaseExpected struct{ t *testing.T }

func (double noReleaseExpected) ReleasePreAcceptanceControl(
	context.Context,
	ports.ControlReleaseRequest,
) (ports.ControlReleaseAnswer, error) {
	double.t.Errorf("没有任何占用的委托，撤回却发了释放")
	return ports.ControlReleaseAnswer{}, errors.New("no release expected")
}

// replayableWithdrawal 装一只与生产同形的撤回编排：requests 可由调用方包一层（用来注入并发抢先），授权换替身。
func replayableWithdrawal(
	t *testing.T,
	db *bentopg.DB,
	authorizer ports.WithdrawalAuthorizer,
	wrapRequests func(ports.ShipmentRequestRepository) ports.ShipmentRequestRepository,
) transactionalWithdrawal {
	t.Helper()
	sources, err := pspostgres.NewSourceSubmissions(db)
	if err != nil {
		t.Fatalf("构造来源仓储：%v", err)
	}
	requestStore, err := pspostgres.NewShipmentRequests(db)
	if err != nil {
		t.Fatalf("构造委托仓储：%v", err)
	}
	var requests ports.ShipmentRequestRepository = requestStore
	if wrapRequests != nil {
		requests = wrapRequests(requests)
	}
	judgments, err := pspostgres.NewAcceptanceJudgments(db)
	if err != nil {
		t.Fatalf("构造判断账：%v", err)
	}
	refusals, err := pspostgres.NewControlReleaseRefusals(db)
	if err != nil {
		t.Fatalf("构造释放未受理册：%v", err)
	}
	identities, err := psidentity.NewAcceptanceDecisions()
	if err != nil {
		t.Fatalf("构造决定标识：%v", err)
	}
	handler := shipmentapp.NewWithdrawShipmentRequestHandler(shipmentapp.WithdrawShipmentRequestDeps{
		Sources:    sources,
		Requests:   requests,
		Authorizer: authorizer,
		Judgments:  judgments,
		Recorder:   judgments,
		Release:    noReleaseExpected{t: t},
		Refusals:   refusals,
		Identities: identities,
		Clock:      systemClock{},
	})
	return transactionalWithdrawal{transactor: db.Transactor(), inner: handler}
}

// staleOnce 在第一次 Save 之前让另一个连接抢先写这份委托（版本号加一），好让本方的 Save 在真库上撞出
// 版本冲突——并发写入抢先在真库上就是这么发生的，不是替身凭空答一个冲突。
type staleOnce struct {
	ports.ShipmentRequestRepository
	pool     *pgxpool.Pool
	identity domain.SourceIdentity
	spent    atomic.Bool
}

func (store *staleOnce) Save(
	ctx context.Context,
	identity domain.SourceIdentity,
	request domain.ShipmentRequest,
) (ports.ShipmentRequestSaveOutcome, error) {
	if store.spent.CompareAndSwap(false, true) {
		overtakeRequest(ctx, store.pool, store.identity)
	}
	return store.ShipmentRequestRepository.Save(ctx, identity, request)
}

// overtakeRequest 经另一个连接把委托行的版本号加一，等同于一个并发写入者抢先落了库。
func overtakeRequest(ctx context.Context, pool *pgxpool.Pool, identity domain.SourceIdentity) {
	_, err := pool.Exec(ctx,
		`UPDATE parcel_shipment.shipment_request SET revision = revision + 1
		  WHERE tenant_id = $1 AND customer_account_id = $2 AND source = $3 AND source_request_key = $4`,
		identity.TenantID().String(), identity.CustomerAccountID().String(),
		identity.Source().String(), identity.RequestKey().String())
	if err != nil {
		panic("overtake request: " + err.Error())
	}
}

func sourceObservationRows(t *testing.T, db *bentopg.DB, identity domain.SourceIdentity) int {
	t.Helper()
	querier, err := db.ReadExecutor(t.Context())
	if err != nil {
		t.Fatalf("取读执行器：%v", err)
	}
	var count int
	err = querier.QueryRow(t.Context(),
		`SELECT count(*) FROM parcel_shipment.source_submission_observation
		  WHERE tenant_id = $1 AND customer_account_id = $2 AND source = $3 AND source_request_key = $4`,
		identity.TenantID().String(), identity.CustomerAccountID().String(),
		identity.Source().String(), identity.RequestKey().String(),
	).Scan(&count)
	if err != nil {
		t.Fatalf("统计来源观察行数：%v", err)
	}
	return count
}

func loadSeededRequest(t *testing.T, db *bentopg.DB, identity domain.SourceIdentity) domain.ShipmentRequest {
	t.Helper()
	requests, err := pspostgres.NewShipmentRequests(db)
	if err != nil {
		t.Fatalf("构造委托仓储：%v", err)
	}
	request, found, err := requests.FindBySourceIdentity(t.Context(), identity)
	if err != nil || !found {
		t.Fatalf("读回委托：err=%v found=%v", err, found)
	}
	return request
}

func withdrawalReplayDatabase(t *testing.T) (*pgxpool.Pool, *bentopg.DB, domain.SourceIdentity) {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	identity := withdrawalSeedIdentity(t)
	seedSubmittedShipmentRequest(t, db, identity)
	return pool, db, identity
}

// Covers: ADR-0256 决定一第 2 格、UC-PS-005「撤回未决」一行——授权答不出的那一轮，装配事务提交了来源保全，委托仍是
// `已提交`；授权恢复后客户重放同一请求，撤回成立（FORMED），委托在真库上读回为`已撤回`并带这一次的撤回决定；再重放
// 答已有决定、带同一份撤回，委托不再被写（版本号不动）、不再问授权。
func TestAWithdrawalReplayedAfterAnUndecidedAuthorizationIsFormedOnceOnARealDatabase(t *testing.T) {
	_, db, identity := withdrawalReplayDatabase(t)
	authorizer := newScriptedWithdrawalAuthorizer(t)
	withdrawal := replayableWithdrawal(t, db, authorizer, nil)
	command := withdrawalCommand(t, identity)

	authorizer.err = errors.New("authority unreachable")
	stopped, err := withdrawal.Handle(t.Context(), command)
	if err != nil {
		t.Fatalf("停下的那一轮：%v", err)
	}
	if stopped.Outcome() != shipmentapp.WithdrawalUndecided || stopped.PendingReason() != shipmentapp.WithdrawalAuthorityUnavailable {
		t.Fatalf("outcome = %v reason = %v, want UNDECIDED / WITHDRAWAL_AUTHORITY_UNAVAILABLE", stopped.Outcome(), stopped.PendingReason())
	}
	if rows := preservedSourceRows(t, db, command.WithdrawalIdentity); rows != 1 {
		t.Fatalf("来源保全行数 = %d, want 1——停点之后保全仍在，重放才认得出", rows)
	}
	if state := loadSeededRequest(t, db, identity).State(); state != domain.ShipmentRequestSubmitted {
		t.Fatalf("state = %v, want 已提交", state)
	}

	authorizer.err = nil
	replayed, err := withdrawal.Handle(t.Context(), command)
	if err != nil {
		t.Fatalf("重放：%v", err)
	}
	if got := replayed.Outcome(); got != shipmentapp.WithdrawalFormed {
		t.Fatalf("重放 outcome = %v（原因 %v）, want FORMED——保全之后没形成的撤回，重放要接着办", got, replayed.PendingReason())
	}
	formed, present := replayed.Withdrawal()
	if !present {
		t.Fatal("重放形成了撤回，却没带撤回记录")
	}
	stored := loadSeededRequest(t, db, identity)
	storedRecord, held := stored.Withdrawal()
	if stored.State() != domain.ShipmentRequestWithdrawn || !held || storedRecord.DecisionID() != formed.DecisionID() ||
		storedRecord.Authority() != formed.Authority() {
		t.Fatalf("读回 state = %v 撤回 = %+v（held=%v）, want 已撤回与这一次形成的 %+v", stored.State(), storedRecord, held, formed)
	}
	if rows := preservedSourceRows(t, db, command.WithdrawalIdentity); rows != 1 {
		t.Fatalf("重放之后来源保全行数 = %d, want 仍是 1", rows)
	}
	if rows := sourceObservationRows(t, db, command.WithdrawalIdentity); rows != 1 {
		t.Fatalf("来源观察行数 = %d, want 1——重放追加一次观察", rows)
	}

	asked := authorizer.calls.Load()
	for round := 0; round < 2; round++ {
		again, err := withdrawal.Handle(t.Context(), command)
		if err != nil {
			t.Fatalf("再重放 %d：%v", round, err)
		}
		if got := again.Outcome(); got != shipmentapp.WithdrawalDecisionAlreadyFormed {
			t.Fatalf("再重放 %d outcome = %v, want DECISION_ALREADY_FORMED", round, got)
		}
		record, present := again.Withdrawal()
		if !present || record.DecisionID() != formed.DecisionID() {
			t.Fatalf("再重放 %d 撤回 = %+v（present=%v）, want 原撤回 %+v", round, record, present, formed)
		}
	}
	if authorizer.calls.Load() != asked {
		t.Fatal("已形成撤回的请求重放，编排又问了一次授权")
	}
	if after := loadSeededRequest(t, db, identity); after.Revision() != stored.Revision() {
		t.Fatalf("版本号 %d, want 仍是 %d——重复重放写了第二次", after.Revision(), stored.Revision())
	}
}

// Covers: ADR-0256 决定一第 2 格——并发写入抢先的那一轮：撤回的 Save 在真库上撞出版本冲突，答未决 STALE_SHIPMENT_REQUEST_REVISION，
// 装配事务照常提交（来源保全落下）。重放重读到新的一份，撤回成立；抢先者只动了版本号，没有做出决定。
func TestAWithdrawalReplayedAfterAStaleRevisionIsFormedOnARealDatabase(t *testing.T) {
	pool, db, identity := withdrawalReplayDatabase(t)
	authorizer := newScriptedWithdrawalAuthorizer(t)
	withdrawal := replayableWithdrawal(t, db, authorizer, func(inner ports.ShipmentRequestRepository) ports.ShipmentRequestRepository {
		return &staleOnce{ShipmentRequestRepository: inner, pool: pool, identity: identity}
	})
	command := withdrawalCommand(t, identity)

	stopped, err := withdrawal.Handle(t.Context(), command)
	if err != nil {
		t.Fatalf("被抢先的那一轮：%v", err)
	}
	if stopped.Outcome() != shipmentapp.WithdrawalUndecided || stopped.PendingReason() != shipmentapp.StaleShipmentRequestRevision {
		t.Fatalf("outcome = %v reason = %v, want UNDECIDED / STALE_SHIPMENT_REQUEST_REVISION", stopped.Outcome(), stopped.PendingReason())
	}
	if rows := preservedSourceRows(t, db, command.WithdrawalIdentity); rows != 1 {
		t.Fatalf("来源保全行数 = %d, want 1——版本冲突时事务照常提交", rows)
	}
	if state := loadSeededRequest(t, db, identity).State(); state != domain.ShipmentRequestSubmitted {
		t.Fatalf("state = %v, want 已提交", state)
	}

	replayed, err := withdrawal.Handle(t.Context(), command)
	if err != nil {
		t.Fatalf("重放：%v", err)
	}
	if got := replayed.Outcome(); got != shipmentapp.WithdrawalFormed {
		t.Fatalf("重放 outcome = %v（原因 %v）, want FORMED", got, replayed.PendingReason())
	}
	if state := loadSeededRequest(t, db, identity).State(); state != domain.ShipmentRequestWithdrawn {
		t.Fatalf("state = %v, want 已撤回", state)
	}
}

// Covers: 同一场景的相反方向——重放不被吞成接着办：同来源身份不同内容仍答来源冲突，不形成撤回，也不问授权；
// 原请求的内容摘要不变。
func TestAWithdrawalWithDifferentContentAfterAStoppedRoundStillConflictsOnARealDatabase(t *testing.T) {
	_, db, identity := withdrawalReplayDatabase(t)
	authorizer := newScriptedWithdrawalAuthorizer(t)
	withdrawal := replayableWithdrawal(t, db, authorizer, nil)
	command := withdrawalCommand(t, identity)

	authorizer.err = errors.New("authority unreachable")
	if _, err := withdrawal.Handle(t.Context(), command); err != nil {
		t.Fatalf("停下的那一轮：%v", err)
	}
	authorizer.err = nil
	asked := authorizer.calls.Load()

	conflicting := command
	conflicting.PayloadDigest = mustValue(t, domain.NewPayloadDigest, "syn-withdraw-digest-2")
	result, err := withdrawal.Handle(t.Context(), conflicting)
	if err != nil {
		t.Fatalf("同身份不同内容：%v", err)
	}
	if got := result.Outcome(); got != shipmentapp.WithdrawalSourceConflict {
		t.Fatalf("outcome = %v, want SOURCE_CONFLICT", got)
	}
	if authorizer.calls.Load() != asked {
		t.Fatal("来源冲突的请求还去问了授权")
	}
	if state := loadSeededRequest(t, db, identity).State(); state != domain.ShipmentRequestSubmitted {
		t.Fatalf("state = %v, want 仍是已提交——冲突不形成撤回", state)
	}
	if rows := sourceObservationRows(t, db, command.WithdrawalIdentity); rows != 0 {
		t.Fatalf("来源观察行数 = %d, want 0——冲突不追加为原请求的观察", rows)
	}
}

// rejectOnRealDatabase 经领域转移与生产委托仓储把一份已落库的`已提交`委托主动拒绝并保存（拒绝授权是别的编排的事，
// 被测的是之后撤回的重放读到什么）。
func rejectOnRealDatabase(t *testing.T, db *bentopg.DB, identity domain.SourceIdentity) {
	t.Helper()
	requests, err := pspostgres.NewShipmentRequests(db)
	if err != nil {
		t.Fatalf("构造委托仓储：%v", err)
	}
	rejected, err := loadSeededRequest(t, db, identity).RejectByAuthority(domain.ActiveRejectionSpec{
		DecisionID: mustValue(t, domain.NewAcceptanceDecisionID, "SYN-DECISION-REJECTED-1"),
		Authority:  mustValue(t, domain.NewRejectionAuthorityReference, "SYN-REJECT-ROLE-1"),
		Decider:    mustValue(t, domain.NewDeciderReference, "SYN-OPERATOR-1"),
		Reason:     mustValue(t, domain.NewRejectionReasonReference, "SYN-REJECT-REASON-1"),
		Evidence:   mustValue(t, domain.NewRejectionEvidenceReference, "SYN-EVIDENCE-1"),
		DecidedAt:  withdrawalSeedAt.Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("主动拒绝：%v", err)
	}
	var saved ports.ShipmentRequestSaveOutcome
	if err := db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		var saveErr error
		saved, saveErr = requests.Save(txCtx, identity, rejected)
		return saveErr
	}); err != nil {
		t.Fatalf("落已拒绝委托：%v", err)
	}
	if saved != ports.ShipmentRequestSaved {
		t.Fatalf("save outcome = %v, want SAVED", saved)
	}
}

// Covers: AT-PS-071、AT-PS-072、AT-PS-068 与 ADR-0256 决定一第 2 格——上一轮停下之后委托越过了提交边界（接受、拒绝先成立，或
// 本请求自己的撤回已落）：真库上读回的是已决定的那一份，重放答已有决定并带那份决定——接受与拒绝带接受决定，已撤回带原
// 撤回；不再问授权、不再写。
func TestAWithdrawalReplayedOverADecidedRequestAnswersTheExistingDecisionOnARealDatabase(t *testing.T) {
	cases := []struct {
		name    string
		decide  func(*testing.T, *bentopg.DB, domain.SourceIdentity)
		state   domain.ShipmentRequestState
		decided string
	}{
		{
			name: "已接受",
			decide: func(t *testing.T, db *bentopg.DB, identity domain.SourceIdentity) {
				requests, err := pspostgres.NewShipmentRequests(db)
				if err != nil {
					t.Fatalf("构造委托仓储：%v", err)
				}
				acceptAndSave(t, db, requests, shipmentapp.SubmitShipmentRequestCommand{
					Identity:          identity,
					DeclaredParcelIDs: []domain.DeclaredParcelID{mustValue(t, domain.NewDeclaredParcelID, "SYN-PARCEL-1")},
				}, loadSeededRequest(t, db, identity))
			},
			state:   domain.ShipmentRequestAccepted,
			decided: "SYN-DECISION-1",
		},
		{
			name:    "已拒绝",
			decide:  rejectOnRealDatabase,
			state:   domain.ShipmentRequestRejected,
			decided: "SYN-DECISION-REJECTED-1",
		},
		{
			name: "已撤回",
			decide: func(t *testing.T, db *bentopg.DB, identity domain.SourceIdentity) {
				withdrawOnRealDatabase(t, db, identity)
			},
			state:   domain.ShipmentRequestWithdrawn,
			decided: "SYN-DECISION-WITHDRAWN-1",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, db, identity := withdrawalReplayDatabase(t)
			authorizer := newScriptedWithdrawalAuthorizer(t)
			withdrawal := replayableWithdrawal(t, db, authorizer, nil)
			command := withdrawalCommand(t, identity)

			authorizer.err = errors.New("authority unreachable")
			if _, err := withdrawal.Handle(t.Context(), command); err != nil {
				t.Fatalf("停下的那一轮：%v", err)
			}
			tc.decide(t, db, identity)
			before := loadSeededRequest(t, db, identity)
			if before.State() != tc.state {
				t.Fatalf("种下的状态 = %v, want %v", before.State(), tc.state)
			}
			asked := authorizer.calls.Load()

			result, err := withdrawal.Handle(t.Context(), command)
			if err != nil {
				t.Fatalf("重放：%v", err)
			}
			if got := result.Outcome(); got != shipmentapp.WithdrawalDecisionAlreadyFormed {
				t.Fatalf("outcome = %v（原因 %v）, want DECISION_ALREADY_FORMED", got, result.PendingReason())
			}
			if result.State() != tc.state {
				t.Fatalf("state = %v, want %v", result.State(), tc.state)
			}
			if tc.state == domain.ShipmentRequestWithdrawn {
				record, present := result.Withdrawal()
				if !present || record.DecisionID().String() != tc.decided {
					t.Fatalf("撤回 = %+v（present=%v）, want 原撤回 %q", record, present, tc.decided)
				}
			} else {
				decision, present := result.AcceptanceDecision()
				if !present || decision.DecisionID().String() != tc.decided {
					t.Fatalf("决定 = %+v（present=%v）, want 已有决定 %q", decision, present, tc.decided)
				}
			}
			if authorizer.calls.Load() != asked {
				t.Fatal("已决定的委托重放，编排又问了授权")
			}
			if after := loadSeededRequest(t, db, identity); after.Revision() != before.Revision() {
				t.Fatalf("版本号 %d, want 仍是 %d——重放写了委托", after.Revision(), before.Revision())
			}
		})
	}
}

// Covers: 「各自重复重放只形成一次」在并发下也成立——停下之后几个重放同时到达：委托行的版本闸门只放一个，其余要么读到已成立的撤回
// （已有决定），要么撞版本冲突答未决（客户再重放读到已有决定）；库里只有一份撤回，版本号只动一次。
func TestConcurrentReplaysOfAStoppedWithdrawalFormItOnlyOnceOnARealDatabase(t *testing.T) {
	_, db, identity := withdrawalReplayDatabase(t)
	authorizer := newScriptedWithdrawalAuthorizer(t)
	withdrawal := replayableWithdrawal(t, db, authorizer, nil)
	command := withdrawalCommand(t, identity)

	authorizer.err = errors.New("authority unreachable")
	if _, err := withdrawal.Handle(t.Context(), command); err != nil {
		t.Fatalf("停下的那一轮：%v", err)
	}
	authorizer.err = nil
	base := loadSeededRequest(t, db, identity)

	const replays = 4
	results := make([]shipmentapp.WithdrawShipmentRequestResult, replays)
	errs := make([]error, replays)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for index := range replays {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			results[index], errs[index] = withdrawal.Handle(context.Background(), command)
		}()
	}
	close(start)
	wg.Wait()

	formed := 0
	for index, result := range results {
		if errs[index] != nil {
			t.Fatalf("并发重放 %d：%v", index, errs[index])
		}
		switch result.Outcome() {
		case shipmentapp.WithdrawalFormed:
			formed++
		case shipmentapp.WithdrawalDecisionAlreadyFormed:
		case shipmentapp.WithdrawalUndecided:
			if result.PendingReason() != shipmentapp.StaleShipmentRequestRevision {
				t.Fatalf("并发重放 %d 未决原因 = %v, want STALE_SHIPMENT_REQUEST_REVISION", index, result.PendingReason())
			}
		default:
			t.Fatalf("并发重放 %d outcome = %v", index, result.Outcome())
		}
	}
	if formed != 1 {
		t.Fatalf("形成撤回的重放 = %d 个, want 恰好 1", formed)
	}
	stored := loadSeededRequest(t, db, identity)
	if stored.State() != domain.ShipmentRequestWithdrawn || stored.Revision() != base.Revision()+1 {
		t.Fatalf("state = %v 版本号 = %d, want 已撤回且版本号只动一次（%d）", stored.State(), stored.Revision(), base.Revision()+1)
	}
}
