package bentocontract

import (
	"errors"
	"sync"
	"testing"
	"time"

	psadapter "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	psapplication "go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// 本文件取 `PBC-04`:同键同摘要返回原结果,同键不同摘要冲突,并发重复不能创建第二份
// 委托或第二个 EventID。
//
// 三个用例都跑真实的 UC-PS-001 编排与真实 PostgreSQL 适配器（装配见 submit_flow_fixture）,
// 幂等与冲突的判定全在生产代码里——测试只投喂命令、数库里的行。「第二个 EventID」按事件
// 类型全量数而不只按预期 ID 数:意外身份下多出来的那份也逃不掉。

func TestSameKeySameDigestReplaysTheOriginalResult(t *testing.T) {
	fixture := newSubmitFlowFixture(t, nil)
	ctx := t.Context()
	identity := contractIdentity(t, "tenant-1", "pbc04-key-replay")

	first, err := fixture.handler.Handle(ctx,
		fixture.submitCommand(t, identity, "PBC04-REQ-01", "sha256:pbc04-a", submitFlowSubmittedAt.Add(-time.Minute)))
	if err != nil {
		t.Fatalf("首次提交:%v", err)
	}
	if first.Outcome() != psapplication.OutcomeSubmitted {
		t.Fatalf("首次提交结果 = %s, want SUBMITTED", first.Outcome())
	}
	originalID, ok := first.ShipmentRequestID()
	if !ok {
		t.Fatal("首次提交没有交回委托标识")
	}

	// 重放:同键同摘要,occurredAt/receivedAt 不同,连客户自报的委托编号都换了——
	// 原结果照样赢:「即使 occurredAt/receivedAt 不同也仍是重放」(简报「输入与幂等」),
	// 交回的必须是原委托,不是重试请求里的新编号。
	replay, err := fixture.handler.Handle(ctx,
		fixture.submitCommand(t, identity, "PBC04-REQ-01-RETRY", "sha256:pbc04-a", submitFlowSubmittedAt.Add(time.Minute)))
	if err != nil {
		t.Fatalf("重放:%v", err)
	}
	if replay.Outcome() != psapplication.OutcomeExistingResult {
		t.Fatalf("重放结果 = %s, want EXISTING_RESULT", replay.Outcome())
	}
	replayedID, ok := replay.ShipmentRequestID()
	if !ok || replayedID != originalID {
		t.Fatalf("重放交回 %q, want 原委托 %q", replayedID, originalID)
	}

	if rows := fixture.requestRowCount(t, identity); rows != 1 {
		t.Fatalf("委托行数 = %d, want 1", rows)
	}
	if intents := fixture.submittedIntentCount(t); intents != 1 {
		t.Fatalf("意图行数 = %d, want 1——重放不得造第二个 EventID", intents)
	}
	if observations := fixture.observationCount(t, identity); observations != 1 {
		t.Fatalf("观察行数 = %d, want 1——重放只追加本次观察", observations)
	}
}

func TestSameKeyDifferentDigestConflicts(t *testing.T) {
	fixture := newSubmitFlowFixture(t, nil)
	ctx := t.Context()
	identity := contractIdentity(t, "tenant-1", "pbc04-key-conflict")

	first, err := fixture.handler.Handle(ctx,
		fixture.submitCommand(t, identity, "PBC04-REQ-02", "sha256:pbc04-a", submitFlowSubmittedAt.Add(-time.Minute)))
	if err != nil || first.Outcome() != psapplication.OutcomeSubmitted {
		t.Fatalf("首次提交 = %s, err=%v", first.Outcome(), err)
	}

	conflict, err := fixture.handler.Handle(ctx,
		fixture.submitCommand(t, identity, "PBC04-REQ-02", "sha256:pbc04-b", submitFlowSubmittedAt.Add(time.Minute)))
	if err != nil {
		t.Fatalf("冲突提交:%v", err)
	}
	if conflict.Outcome() != psapplication.OutcomeIngressConflict {
		t.Fatalf("冲突结果 = %s, want INGRESS_CONFLICT", conflict.Outcome())
	}
	if _, leaked := conflict.ShipmentRequestID(); leaked {
		t.Fatal("冲突答案不该附带委托标识——冲突方拿不到原内容的任何续办入口")
	}

	if rows := fixture.requestRowCount(t, identity); rows != 1 {
		t.Fatalf("委托行数 = %d, want 1——冲突不建第二份", rows)
	}
	if intents := fixture.submittedIntentCount(t); intents != 1 {
		t.Fatalf("意图行数 = %d, want 1——冲突不出第二个 EventID", intents)
	}
	if observations := fixture.observationCount(t, identity); observations != 0 {
		t.Fatalf("观察行数 = %d, want 0——冲突不是又一次合法观察", observations)
	}
}

// TestConcurrentDuplicatesCreateOneRequestAndOneEventID 放八个同命令协程同时进管线。
//
// 允许的结局是封闭的:恰好一个`已提交`;其余要么当场拿到原结果,要么撞在保全竞态上拿到
// ErrAlreadyPreserved——那是「重试即可读到已保全记录」的可重试错误（来源保全适配器的
// 注释即此义）,重试后必须全部收敛到原结果。库里自始至终一行委托、一份意图。
func TestConcurrentDuplicatesCreateOneRequestAndOneEventID(t *testing.T) {
	fixture := newSubmitFlowFixture(t, nil)
	ctx := t.Context()
	identity := contractIdentity(t, "tenant-1", "pbc04-key-concurrent")

	const workers = 8
	outcomes := make([]psapplication.SubmitShipmentRequestResult, workers)
	failures := make([]error, workers)

	// 命令在主协程造好:夹具助手失败走 t.Fatal,而 t.Fatal 只能在测试主协程调。
	// 各协程收到的是同一份只读值的拷贝。
	command := fixture.submitCommand(t, identity, "PBC04-REQ-CC", "sha256:pbc04-cc", submitFlowSubmittedAt.Add(-time.Minute))

	var wait sync.WaitGroup
	start := make(chan struct{})
	for worker := range workers {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			outcomes[worker], failures[worker] = fixture.handler.Handle(ctx, command)
		}()
	}
	close(start)
	wait.Wait()

	submitted := 0
	var originalID domain.ShipmentRequestID
	for worker := range workers {
		if failures[worker] != nil {
			if !errors.Is(failures[worker], psadapter.ErrAlreadyPreserved) {
				t.Fatalf("协程 %d 的错误不在封闭集合里:%v", worker, failures[worker])
			}
			continue
		}
		switch outcomes[worker].Outcome() {
		case psapplication.OutcomeSubmitted:
			submitted++
			originalID, _ = outcomes[worker].ShipmentRequestID()
		case psapplication.OutcomeExistingResult:
			// 原结果可能还没建完单(先到者仍在途),此时没有委托标识可附;有则必须是原委托,
			// 下面统一核对。
		default:
			t.Fatalf("协程 %d 的结果不在封闭集合里:%s", worker, outcomes[worker].Outcome())
		}
	}
	if submitted != 1 {
		t.Fatalf("`已提交`出现 %d 次, want 恰好 1", submitted)
	}

	// 撞上保全竞态的协程按适配器注释的口径重试:全部收敛到原结果,一个新委托都长不出来。
	for worker := range workers {
		if failures[worker] == nil {
			continue
		}
		retried, err := fixture.handler.Handle(ctx,
			fixture.submitCommand(t, identity, "PBC04-REQ-CC", "sha256:pbc04-cc", submitFlowSubmittedAt.Add(time.Minute)))
		if err != nil {
			t.Fatalf("协程 %d 重试:%v", worker, err)
		}
		if retried.Outcome() != psapplication.OutcomeExistingResult {
			t.Fatalf("协程 %d 重试结果 = %s, want EXISTING_RESULT", worker, retried.Outcome())
		}
	}

	// 全体交回过的委托标识必须指同一份。
	for worker := range workers {
		if failures[worker] != nil {
			continue
		}
		if returned, has := outcomes[worker].ShipmentRequestID(); has && returned != originalID {
			t.Fatalf("协程 %d 交回 %q, 与原委托 %q 不是同一份", worker, returned, originalID)
		}
	}

	if rows := fixture.requestRowCount(t, identity); rows != 1 {
		t.Fatalf("委托行数 = %d, want 1——并发重复不能创建第二份委托", rows)
	}
	if intents := fixture.submittedIntentCount(t); intents != 1 {
		t.Fatalf("意图行数 = %d, want 1——并发重复不能创建第二个 EventID", intents)
	}
}
