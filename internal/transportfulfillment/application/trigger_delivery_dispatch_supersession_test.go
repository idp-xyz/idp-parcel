package application_test

import (
	"context"
	"testing"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// blindTaskReader 扮演「查在册那一刻还看不见」：按键读任务一律答不在册。
type blindTaskReader struct{}

func (blindTaskReader) FindByKey(context.Context, ports.DispatchTaskKey) (ports.DispatchTaskRecord, bool, error) {
	return ports.DispatchTaskRecord{}, false, nil
}

// Covers: 票 tf-face-algebra/09 裁定「查在册之后、落库之前另一笔先提交了这个键」那一道；AT-TF-295 的替身那一半（ADR-0230 决定一）。
// 执行器那几路由形成锁排成一前一后，在真库上走不到这一道了；不经执行器写任务的那一种仍可能在查在册之后先落下同一个键。这里让查在册
// 看不见、任务口撞键：执行器读回赢家，核得过答`已在册`交回那一项，不另写。
func TestATaskLandingBetweenTheCheckAndTheWriteIsReadBackAsTheWinner(t *testing.T) {
	fixture := newDispatchTriggerFixture(t)
	fixture.enterByHandover(t, "parcel-1", "FINAL_DELIVERY", handoverJudgedTime)
	first, err := fixture.handler().Trigger(t.Context(), triggerCommand(t, "parcel-1"))
	if err != nil || first.Outcome() != application.DeliveryDispatchTaskFormed {
		t.Fatalf("先落下的那一笔 = %s err=%v", first.Outcome(), err)
	}
	winner, _ := first.Record()

	fixture.deps.Tasks = blindTaskReader{}
	result, err := fixture.handler().Trigger(t.Context(), triggerCommand(t, "parcel-1"))
	again, _ := result.Record()
	if err != nil || result.Outcome() != application.DeliveryDispatchTaskAlreadyFormed || again.Key != winner.Key {
		t.Fatalf("撞键那一笔 = %s 交回 %v err=%v, want DISPATCH_TASK_ALREADY_FORMED 交回 %v", result.Outcome(), again.Key, err, winner.Key)
	}
	if fixture.places.calls != 2 || len(fixture.tasks.records) != 1 {
		t.Fatalf("问地点 %d 次、册上 %d 项, want 第二次越过查在册问了所有者、在任务口撞键，册上仍 1 项", fixture.places.calls, len(fixture.tasks.records))
	}
}
