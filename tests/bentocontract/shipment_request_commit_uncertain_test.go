package bentocontract

import (
	"context"
	"errors"
	"testing"
	"time"

	bentoapp "go.idp.xyz/idp-bento-go/application"

	psapplication "go.idp.xyz/idp-parcel/internal/parcelshipment/application"
)

// 本文件取 `PBC-07`:`application.ErrCommitUncertain` 不触发无条件自动重放,调用方能够
// 按稳定请求关联查询原结果。
//
// 框架合同（application-transaction-contract 的「提交结果不确定后的恢复」一节）规定:
// 收到不确定错误后,只有具备稳定命令关联与业务幂等证明的消费者才可以恢复,且恢复是
// 「先查原结果,确认不存在才重执行」,绝不是盲目重跑事务。Parcel 的业务幂等证明就是
// 来源保全:四维来源身份是稳定请求关联,同键同摘要重放返原。本文件证的是消费方自己的
// 这两条义务,不重证框架何时产出该错误——那属框架自己的合同。
//
// 注入方式:包一层事务器,真提交落地后谎报「连接已断」。这正是 ErrCommitUncertain 语义里
// 调用方无从分辨的那一格——事务可能成也可能败,而这里故意让它「成了但你不知道」,恢复
// 路径的每一步才都有实底可查。

// uncertainAfterCommit 让下一次提交在真实落地后报不确定。armed 只在测试主协程置位与
// 消费,不跨协程。
type uncertainAfterCommit struct {
	inner bentoapp.Transactor
	armed bool
}

func (transactor *uncertainAfterCommit) WithinTransaction(
	ctx context.Context,
	fn bentoapp.TxFunc,
) error {
	err := transactor.inner.WithinTransaction(ctx, fn)
	if err == nil && transactor.armed {
		transactor.armed = false
		return errors.Join(bentoapp.ErrCommitUncertain, errors.New("pbc-07: connection lost after commit"))
	}
	return err
}

func TestCommitUncertainDoesNotReplayAndTheOriginalResultStaysQueryable(t *testing.T) {
	injector := &uncertainAfterCommit{}
	fixture := newSubmitFlowFixture(t, func(base bentoapp.Transactor) bentoapp.Transactor {
		injector.inner = base
		return injector
	})
	ctx := t.Context()
	identity := contractIdentity(t, "tenant-1", "pbc07-key-uncertain")

	injector.armed = true
	result, err := fixture.handler.Handle(ctx,
		fixture.submitCommand(t, identity, "PBC07-REQ-01", "sha256:pbc07-a", submitFlowSubmittedAt.Add(-time.Minute)))

	// 其一:错误原样透出且可判——调用方要靠 errors.Is 认出这一格,吞掉或改写它,上层
	// 连「不能盲目重试」这个判断都做不出来。
	if !errors.Is(err, bentoapp.ErrCommitUncertain) {
		t.Fatalf("错误 = %v, want 包着 ErrCommitUncertain", err)
	}
	// 结果必须是零值:提交没被确认,任何业务答案都是编造。
	if result.Outcome() != psapplication.OutcomeInvalid {
		t.Fatalf("不确定提交仍交回了业务结果:%s", result.Outcome())
	}
	// 其二:整个 Handle 只开过一次提交事务——不确定不触发无条件自动重放。
	if calls := fixture.submits.calls.Load(); calls != 1 {
		t.Fatalf("提交事务开了 %d 次, want 1——不确定错误不得自动重放", calls)
	}

	// 其三:按稳定请求关联查询原结果。四维来源身份就是稳定关联,查询走生产读口。
	// 这次注入的真相是「已提交」,查询必须看得见原结果——委托行与意图各恰好一份。
	found, exists, err := fixture.requests.FindBySourceIdentity(ctx, identity)
	if err != nil || !exists {
		t.Fatalf("按稳定关联查原结果:exists=%v err=%v", exists, err)
	}
	if found.ShipmentRequestID().String() != "PBC07-REQ-01" {
		t.Fatalf("查回的委托 = %q, want PBC07-REQ-01", found.ShipmentRequestID())
	}
	if rows := fixture.requestRowCount(t, identity); rows != 1 {
		t.Fatalf("委托行数 = %d, want 1", rows)
	}
	if intents := fixture.submittedIntentCount(t); intents != 1 {
		t.Fatalf("意图行数 = %d, want 1", intents)
	}

	// 其四:合法的恢复路径是带着同一份命令重新提交——来源保全把它判成重放,返回原结果,
	// 全程不再打开提交事务,更不产出第二个 EventID。这就是「已证明业务幂等时才能自动
	// 续办」在本仓的形状。
	recovered, err := fixture.handler.Handle(ctx,
		fixture.submitCommand(t, identity, "PBC07-REQ-01", "sha256:pbc07-a", submitFlowSubmittedAt.Add(time.Minute)))
	if err != nil {
		t.Fatalf("恢复提交:%v", err)
	}
	if recovered.Outcome() != psapplication.OutcomeExistingResult {
		t.Fatalf("恢复结果 = %s, want EXISTING_RESULT", recovered.Outcome())
	}
	recoveredID, ok := recovered.ShipmentRequestID()
	if !ok || recoveredID.String() != "PBC07-REQ-01" {
		t.Fatalf("恢复交回 %q, want 原委托 PBC07-REQ-01", recoveredID)
	}
	if calls := fixture.submits.calls.Load(); calls != 1 {
		t.Fatalf("恢复后提交事务计数 = %d, want 仍为 1——重放路径不重跑事务", calls)
	}
	if rows := fixture.requestRowCount(t, identity); rows != 1 {
		t.Fatalf("恢复后委托行数 = %d, want 1", rows)
	}
	if intents := fixture.submittedIntentCount(t); intents != 1 {
		t.Fatalf("恢复后意图行数 = %d, want 1", intents)
	}
}

// uncertainOverRollback 让下一次提交在真实回滚后报不确定——ErrCommitUncertain 语义的
// 另半格：「败了但你不知道」。业务回调成功、事务被人为回滚，调用方拿到的与落地方向
// 同一个错误，无从分辨——恢复纪律必须在两个方向上都成立才算闭合。
type uncertainOverRollback struct {
	inner bentoapp.Transactor
	armed bool
}

var errForcedRollbackBehindUncertainty = errors.New("pbc-07: forced rollback behind uncertainty")

func (transactor *uncertainOverRollback) WithinTransaction(
	ctx context.Context,
	fn bentoapp.TxFunc,
) error {
	if !transactor.armed {
		return transactor.inner.WithinTransaction(ctx, fn)
	}
	transactor.armed = false
	err := transactor.inner.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := fn(txCtx); err != nil {
			return err
		}
		return errForcedRollbackBehindUncertainty
	})
	if errors.Is(err, errForcedRollbackBehindUncertainty) {
		return errors.Join(bentoapp.ErrCommitUncertain, errors.New("pbc-07: connection lost before commit acknowledgement"))
	}
	return err
}

// TestCommitUncertainOverARollbackAnswersTruthfullyByStableCorrelation 证回滚方向的
// 恢复纪律：按稳定关联查询答的是真相（委托不存在、半个都没长出来），且重发同一命令
// 不盲目重跑提交事务。
//
// 这个方向上来源保全已经独立提交（简报「事务边界」第一段），重发同命令因此走重放
// 判定：答`已有结果`且不附委托——「只有已经建单时才附已有委托结果」（简报「输入与
// 幂等」）——提交事务一次都不再开。把保全后中断的提交续成委托属「已证明业务幂等时
// 才能自动续办」的续办机制，本文件只证当前口径如实、不越界替它作答。
func TestCommitUncertainOverARollbackAnswersTruthfullyByStableCorrelation(t *testing.T) {
	injector := &uncertainOverRollback{}
	fixture := newSubmitFlowFixture(t, func(base bentoapp.Transactor) bentoapp.Transactor {
		injector.inner = base
		return injector
	})
	ctx := t.Context()
	identity := contractIdentity(t, "tenant-1", "pbc07-key-rolled-back")

	injector.armed = true
	result, err := fixture.handler.Handle(ctx,
		fixture.submitCommand(t, identity, "PBC07-REQ-02", "sha256:pbc07-b", submitFlowSubmittedAt.Add(-time.Minute)))
	if !errors.Is(err, bentoapp.ErrCommitUncertain) {
		t.Fatalf("错误 = %v, want 包着 ErrCommitUncertain", err)
	}
	if result.Outcome() != psapplication.OutcomeInvalid {
		t.Fatalf("不确定提交仍交回了业务结果:%s", result.Outcome())
	}
	if calls := fixture.submits.calls.Load(); calls != 1 {
		t.Fatalf("提交事务开了 %d 次, want 1——不确定错误不得自动重放", calls)
	}

	// 按稳定关联查询:这个方向的真相是「没提交」,查询必须答查无——半份委托都不该在。
	if _, exists, err := fixture.requests.FindBySourceIdentity(ctx, identity); err != nil || exists {
		t.Fatalf("回滚方向查原结果:exists=%v err=%v, want 查无", exists, err)
	}
	if rows := fixture.requestRowCount(t, identity); rows != 0 {
		t.Fatalf("委托行数 = %d, want 0——回滚方向不得留下半份委托", rows)
	}
	if intents := fixture.submittedIntentCount(t); intents != 0 {
		t.Fatalf("意图行数 = %d, want 0——回滚方向不得留下无主意图", intents)
	}

	// 重发同一命令:来源保全已独立提交,重放判定接手——答`已有结果`、不附委托、
	// 不再开提交事务。
	replayed, err := fixture.handler.Handle(ctx,
		fixture.submitCommand(t, identity, "PBC07-REQ-02", "sha256:pbc07-b", submitFlowSubmittedAt.Add(time.Minute)))
	if err != nil {
		t.Fatalf("重发同一命令:%v", err)
	}
	if replayed.Outcome() != psapplication.OutcomeExistingResult {
		t.Fatalf("重发结果 = %s, want EXISTING_RESULT（保全已提交）", replayed.Outcome())
	}
	if _, attached := replayed.ShipmentRequestID(); attached {
		t.Fatal("重发附带了委托标识——委托从未建立,附上即编造")
	}
	if calls := fixture.submits.calls.Load(); calls != 1 {
		t.Fatalf("重发后提交事务计数 = %d, want 仍为 1——重放判定不重跑事务", calls)
	}
	if rows := fixture.requestRowCount(t, identity); rows != 0 {
		t.Fatalf("重发后委托行数 = %d, want 0", rows)
	}
	if intents := fixture.submittedIntentCount(t); intents != 0 {
		t.Fatalf("重发后意图行数 = %d, want 0", intents)
	}
}
