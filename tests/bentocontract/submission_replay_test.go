package bentocontract

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentoapp "go.idp.xyz/idp-bento-go/application"

	pspostgres "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// 本文件取 `PBC-04` 的可取证面与 `PBC-07`。
//
// 装配方式：真实的来源保全仓储与委托仓储（PostgreSQL 16）+ 应用编排
// SubmitShipmentRequestHandler + Parcel 自有端口的确定性取值（归属权威、身份工厂、
// 时钟——简报明写这类替身替的是 Parcel 的端口而非框架合同，不属「本地框架替身」）。
// 每条命令在一个 Transactor 事务里执行。
//
// 边界（如实记录）：`PBC-04` 的「不能创建第二个 EventID」这半今天**不可取证**——
// `委托已提交`事件的生产发射器（EventID 随业务结果保存、重复请求复用原值）在
// internal/ 下不存在，取证对象缺席。本文件证到「并发重复不能创建第二份委托、同键
// 重放返回原结果、同键异摘要冲突」，EventID 半边随发射器落地后补证。
type submissionRig struct {
	handler    *application.SubmitShipmentRequestHandler
	transactor bentoapp.Transactor
	pool       *pgxpool.Pool
}

func newSubmissionRig(t *testing.T) submissionRig {
	t.Helper()

	db, pool := parcelDB(t)
	sources, err := pspostgres.NewSourceSubmissions(db)
	if err != nil {
		t.Fatalf("构造来源保全仓储：%v", err)
	}
	requests, err := pspostgres.NewShipmentRequests(db)
	if err != nil {
		t.Fatalf("构造委托仓储：%v", err)
	}
	handler := application.NewSubmitShipmentRequestHandler(
		sources,
		requests,
		fixedOwnership{decision: contractOwnershipDecision(t)},
		&sequenceIdentities{},
		fixedClock{at: contractInstant},
	)
	return submissionRig{handler: handler, transactor: db.Transactor(), pool: pool}
}

// submit 在一个事务里执行一条提交命令——事务提交后结果才作数（Transactional 的语义）。
func (rig submissionRig) submit(
	ctx context.Context,
	command application.SubmitShipmentRequestCommand,
) (application.SubmitShipmentRequestResult, error) {
	return bentoapp.Transactional(ctx, rig.transactor,
		func(txCtx context.Context) (application.SubmitShipmentRequestResult, error) {
			return rig.handler.Handle(txCtx, command)
		})
}

func (rig submissionRig) countRows(t *testing.T, table string, scope contractScope, key string) int {
	t.Helper()
	var count int
	err := rig.pool.QueryRow(context.Background(),
		`SELECT count(*) FROM `+table+
			` WHERE tenant_id = $1 AND customer_account_id = $2 AND source = 'portal' AND source_request_key = $3`,
		scope.tenant, scope.customer, key,
	).Scan(&count)
	if err != nil {
		t.Fatalf("数 %s 行：%v", table, err)
	}
	return count
}

func contractSubmitCommand(
	t *testing.T,
	scope contractScope,
	key, digest, requestID string,
) application.SubmitShipmentRequestCommand {
	t.Helper()
	return application.SubmitShipmentRequestCommand{
		Identity:          contractIdentity(t, scope, key),
		PayloadDigest:     mustBuild(t, domain.NewPayloadDigest, digest),
		OccurredAt:        contractInstant.Add(-time.Hour),
		ReceivedAt:        contractInstant.Add(-time.Hour + time.Second),
		BatchID:           mustBuild(t, domain.NewSubmissionBatchID, requestID+"-batch"),
		ShipmentRequestID: mustBuild(t, domain.NewShipmentRequestID, requestID),
		DeclaredParcelIDs: []domain.DeclaredParcelID{
			mustBuild(t, domain.NewDeclaredParcelID, requestID+"-p1"),
			mustBuild(t, domain.NewDeclaredParcelID, requestID+"-p2"),
		},
		AdmissionScope:   contractAdmissionScope(t),
		ExpectedRevision: mustBuild(t, domain.NewProductionOwnershipRevision, "contract-rev-1"),
	}
}

// TestReplayWithSameDigestReturnsTheOriginalResult 证同键同摘要返回原结果：重放不建
// 第二份委托、不保全第二份来源，即使 occurredAt/receivedAt 不同也仍是重放、只追加观察。
func TestReplayWithSameDigestReturnsTheOriginalResult(t *testing.T) {
	rig := newSubmissionRig(t)
	ctx := context.Background()
	scope := contractScope{tenant: "tenant-a", customer: "customer-a"}

	first, err := rig.submit(ctx, contractSubmitCommand(t, scope, "replay-key", "digest-1", "replay-request"))
	if err != nil {
		t.Fatalf("首次提交：%v", err)
	}
	if first.Outcome() != application.OutcomeSubmitted {
		t.Fatalf("首次结果 = %s, want SUBMITTED", first.Outcome())
	}
	firstID, ok := first.ShipmentRequestID()
	if !ok {
		t.Fatal("首次提交没有交回委托标识")
	}

	replayCommand := contractSubmitCommand(t, scope, "replay-key", "digest-1", "replay-request")
	// 观察时间不同不改变「这是重放」的判定。
	replayCommand.OccurredAt = contractInstant.Add(-30 * time.Minute)
	replayCommand.ReceivedAt = contractInstant.Add(-30*time.Minute + time.Second)
	replay, err := rig.submit(ctx, replayCommand)
	if err != nil {
		t.Fatalf("重放：%v", err)
	}
	if replay.Outcome() != application.OutcomeExistingResult {
		t.Fatalf("重放结果 = %s, want EXISTING_RESULT", replay.Outcome())
	}
	replayID, ok := replay.ShipmentRequestID()
	if !ok || replayID != firstID {
		t.Fatalf("重放交回 %q，原结果是 %q", replayID, firstID)
	}

	if count := rig.countRows(t, "parcel_shipment.shipment_request", scope, "replay-key"); count != 1 {
		t.Fatalf("委托行数 = %d，重放建出了第二份", count)
	}
	if count := rig.countRows(t, "parcel_shipment.source_submission", scope, "replay-key"); count != 1 {
		t.Fatalf("来源行数 = %d，重放改写了保全", count)
	}
	if count := rig.countRows(t, "parcel_shipment.source_submission_observation", scope, "replay-key"); count != 1 {
		t.Fatalf("观察行数 = %d，重放该恰好追加一条观察", count)
	}
}

// TestReplayWithDifferentDigestConflicts 证同键不同摘要是接入冲突：原内容保留、不覆盖、
// 不再建单。
func TestReplayWithDifferentDigestConflicts(t *testing.T) {
	rig := newSubmissionRig(t)
	ctx := context.Background()
	scope := contractScope{tenant: "tenant-a", customer: "customer-a"}

	first, err := rig.submit(ctx, contractSubmitCommand(t, scope, "conflict-key", "digest-1", "conflict-request"))
	if err != nil || first.Outcome() != application.OutcomeSubmitted {
		t.Fatalf("首次提交 = %s, %v", first.Outcome(), err)
	}

	conflict, err := rig.submit(ctx, contractSubmitCommand(t, scope, "conflict-key", "digest-2", "conflict-request-2"))
	if err != nil {
		t.Fatalf("冲突提交：%v", err)
	}
	if conflict.Outcome() != application.OutcomeIngressConflict {
		t.Fatalf("冲突结果 = %s, want INGRESS_CONFLICT", conflict.Outcome())
	}

	var digest string
	err = rig.pool.QueryRow(ctx,
		`SELECT payload_digest FROM parcel_shipment.source_submission
		  WHERE tenant_id = $1 AND customer_account_id = $2 AND source = 'portal' AND source_request_key = $3`,
		scope.tenant, scope.customer, "conflict-key").Scan(&digest)
	if err != nil {
		t.Fatalf("读回保全摘要：%v", err)
	}
	if digest != "digest-1" {
		t.Fatalf("保全摘要 = %q，冲突覆盖了原内容", digest)
	}
	if count := rig.countRows(t, "parcel_shipment.shipment_request", scope, "conflict-key"); count != 1 {
		t.Fatalf("委托行数 = %d，冲突建出了第二份", count)
	}
}

// TestConcurrentDuplicatesCannotCreateASecondRequest 证并发重复不能创建第二份委托。
//
// 四条同键同摘要的命令各在自己的事务里并发执行。允许的答案有三种：`已提交`（至多一个）、
// `已有结果`（后到者）、以及来源保全的并发竞态错误 ErrAlreadyPreserved（后到者撞在先到者
// 提交之前，事务如实回滚）——适配器注释明写此时「调用方重试即可读到已保全记录并走重放
// 那一支」，所以出错的一律重试一次并要求收敛到原结果。
func TestConcurrentDuplicatesCannotCreateASecondRequest(t *testing.T) {
	rig := newSubmissionRig(t)
	ctx := context.Background()
	scope := contractScope{tenant: "tenant-a", customer: "customer-a"}

	const workers = 4
	results := make([]application.SubmitShipmentRequestResult, workers)
	failures := make([]error, workers)
	var wg sync.WaitGroup
	for index := 0; index < workers; index++ {
		wg.Add(1)
		go func(slot int) {
			defer wg.Done()
			results[slot], failures[slot] = rig.submit(ctx,
				contractSubmitCommand(t, scope, "race-key", "digest-1", "race-request"))
		}(index)
	}
	wg.Wait()

	submitted := 0
	var submittedID domain.ShipmentRequestID
	for slot := 0; slot < workers; slot++ {
		if failures[slot] != nil {
			if !errors.Is(failures[slot], pspostgres.ErrAlreadyPreserved) {
				t.Fatalf("并发提交第 %d 路失败于竞态之外：%v", slot, failures[slot])
			}
			continue
		}
		switch results[slot].Outcome() {
		case application.OutcomeSubmitted:
			submitted++
			submittedID, _ = results[slot].ShipmentRequestID()
		case application.OutcomeExistingResult:
		default:
			t.Fatalf("并发提交第 %d 路答 %s，不在允许集合内", slot, results[slot].Outcome())
		}
	}
	if submitted > 1 {
		t.Fatalf("`已提交`出现 %d 次，建出了多份委托", submitted)
	}

	if count := rig.countRows(t, "parcel_shipment.shipment_request", scope, "race-key"); count > 1 {
		t.Fatalf("委托行数 = %d，并发重复建出了第二份", count)
	}
	if count := rig.countRows(t, "parcel_shipment.source_submission", scope, "race-key"); count != 1 {
		t.Fatalf("来源行数 = %d", count)
	}

	// 竞态败者重试：全部收敛到同一份原结果。
	final, err := rig.submit(ctx, contractSubmitCommand(t, scope, "race-key", "digest-1", "race-request"))
	if err != nil {
		t.Fatalf("竞态后的重试：%v", err)
	}
	if final.Outcome() != application.OutcomeExistingResult {
		t.Fatalf("重试结果 = %s, want EXISTING_RESULT", final.Outcome())
	}
	finalID, ok := final.ShipmentRequestID()
	if !ok {
		t.Fatal("重试没有交回委托标识")
	}
	if submitted == 1 && finalID != submittedID {
		t.Fatalf("重试交回 %q，赢家建的是 %q", finalID, submittedID)
	}
	if count := rig.countRows(t, "parcel_shipment.shipment_request", scope, "race-key"); count != 1 {
		t.Fatalf("重试后委托行数 = %d", count)
	}
}

// TestCommitUncertaintyResolvesByStableCorrelationQuery 取 `PBC-07`：
// application.ErrCommitUncertain 不触发无条件自动重放——恢复动作是按稳定请求关联
// （完整来源身份）查询原结果，两个真实方向都收敛且都不产生第二份委托。
//
// 编排的第一步 FindPreserved 正是那次按稳定关联的查询：提交其实已落库时，重发同一命令
// 走的是重放判定（读已保全记录作答），业务写入不会重做；提交其实已回滚时，查询答「无
// 结果」，重发才真正建单。「无条件自动重放」指的是不查先写、把同一事务盲目重跑——那在
// 这两个方向上分别会造成重复副作用与假阳性，本用例以行数与结果一致性证明查询路径两个
// 方向都不需要它。
func TestCommitUncertaintyResolvesByStableCorrelationQuery(t *testing.T) {
	ctx := context.Background()
	scope := contractScope{tenant: "tenant-a", customer: "customer-a"}

	t.Run("提交已落库而结果不可知", func(t *testing.T) {
		rig := newSubmissionRig(t)
		uncertain := &uncertainCommitTransactor{inner: rig.transactor, commitLands: true}
		command := contractSubmitCommand(t, scope, "uncertain-key", "digest-1", "uncertain-request")

		_, err := bentoapp.Transactional(ctx, uncertain,
			func(txCtx context.Context) (application.SubmitShipmentRequestResult, error) {
				return rig.handler.Handle(txCtx, command)
			})
		if !errors.Is(err, bentoapp.ErrCommitUncertain) {
			t.Fatalf("首次提交 err = %v, want ErrCommitUncertain", err)
		}

		// 恢复：重发同一命令。编排先按稳定关联查询，已落库的结果被原样答回，
		// 业务写入不重做。
		recovered, err := rig.submit(ctx, command)
		if err != nil {
			t.Fatalf("按稳定关联恢复：%v", err)
		}
		if recovered.Outcome() != application.OutcomeExistingResult {
			t.Fatalf("恢复结果 = %s, want EXISTING_RESULT（提交其实已发生）", recovered.Outcome())
		}
		if _, ok := recovered.ShipmentRequestID(); !ok {
			t.Fatal("恢复没有交回已落库的委托标识")
		}
		if count := rig.countRows(t, "parcel_shipment.shipment_request", scope, "uncertain-key"); count != 1 {
			t.Fatalf("委托行数 = %d，恢复重做了业务写入", count)
		}
	})

	t.Run("提交其实已回滚而结果不可知", func(t *testing.T) {
		rig := newSubmissionRig(t)
		uncertain := &uncertainCommitTransactor{inner: rig.transactor, commitLands: false}
		command := contractSubmitCommand(t, scope, "uncertain-key-2", "digest-1", "uncertain-request-2")

		_, err := bentoapp.Transactional(ctx, uncertain,
			func(txCtx context.Context) (application.SubmitShipmentRequestResult, error) {
				return rig.handler.Handle(txCtx, command)
			})
		if !errors.Is(err, bentoapp.ErrCommitUncertain) {
			t.Fatalf("首次提交 err = %v, want ErrCommitUncertain", err)
		}
		if count := rig.countRows(t, "parcel_shipment.shipment_request", scope, "uncertain-key-2"); count != 0 {
			t.Fatalf("回滚方向的前提坏了：库里已有 %d 行", count)
		}

		recovered, err := rig.submit(ctx, command)
		if err != nil {
			t.Fatalf("按稳定关联恢复：%v", err)
		}
		if recovered.Outcome() != application.OutcomeSubmitted {
			t.Fatalf("恢复结果 = %s, want SUBMITTED（提交其实没发生）", recovered.Outcome())
		}
		if count := rig.countRows(t, "parcel_shipment.shipment_request", scope, "uncertain-key-2"); count != 1 {
			t.Fatalf("恢复后委托行数 = %d", count)
		}
	})
}

// uncertainCommitTransactor 在首个事务上制造「提交结果不可知」：commitLands 为真时事务
// 照常提交、为假时强制回滚，两个方向都对调用方报 ErrCommitUncertain——这正是该哨兵的
// 含义：两种历史都可能，调用方不得凭错误本身断定任何一边。后续事务原样放行。
type uncertainCommitTransactor struct {
	inner       bentoapp.Transactor
	commitLands bool
	fired       atomic.Bool
}

var errUncertainRollback = errors.New("bentocontract: forced rollback behind uncertainty")

func (transactor *uncertainCommitTransactor) WithinTransaction(
	ctx context.Context,
	fn bentoapp.TxFunc,
) error {
	if transactor.fired.Swap(true) {
		return transactor.inner.WithinTransaction(ctx, fn)
	}
	if transactor.commitLands {
		if err := transactor.inner.WithinTransaction(ctx, fn); err != nil {
			return err
		}
		return bentoapp.ErrCommitUncertain
	}
	err := transactor.inner.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := fn(txCtx); err != nil {
			return err
		}
		return errUncertainRollback
	})
	if errors.Is(err, errUncertainRollback) {
		return bentoapp.ErrCommitUncertain
	}
	return err
}

// fixedOwnership 恒答同一份归属决定（Parcel 自有端口的确定性取值）。
type fixedOwnership struct {
	decision domain.ProductionOwnershipDecision
}

func (ownership fixedOwnership) DecideProductionOwnership(
	_ context.Context,
	_ domain.AdmissionScope,
) (domain.ProductionOwnershipDecision, error) {
	return ownership.decision, nil
}

// sequenceIdentities 按进程内计数签发内部身份（Parcel 自有端口的确定性取值）。
type sequenceIdentities struct {
	counter atomic.Uint64
}

func (identities *sequenceIdentities) NextSubmissionVersionID(
	_ context.Context,
) (domain.SubmissionVersionID, error) {
	return domain.NewSubmissionVersionID(fmt.Sprintf("contract-ver-%d", identities.counter.Add(1)))
}

func (identities *sequenceIdentities) NextAcceptanceDecisionTaskID(
	_ context.Context,
) (domain.AcceptanceDecisionTaskID, error) {
	return domain.NewAcceptanceDecisionTaskID(fmt.Sprintf("contract-task-%d", identities.counter.Add(1)))
}

// fixedClock 恒答同一时刻。
type fixedClock struct {
	at time.Time
}

func (clock fixedClock) Now() time.Time {
	return clock.at
}
