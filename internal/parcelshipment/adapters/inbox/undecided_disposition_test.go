package psinbox

import (
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// 本文件穷尽钉住未决的折法（ADR-0094 Decision 一与三）：**按恢复动作折，不按未决原因的
// 具体取值折**。
//
// 折法抽成对 `ResumePath` 的纯全函数，是因为 `AdvanceAcceptanceChainResult` 的取值只有
// 编排自己造得出（同包消费者测试的头注记着这条），替身交不回一个带原因的结果。把判断留在
// 一个纯函数上，这四格才穷尽得了；接线那一层保持薄，由 cmd/parcel-dispatch 的装配用例按
// 真编排证。

// TestRetriedWaitsRollBack 钉住重投那一组。
//
// `等待内部续办`重投是因为它等的依赖会自行恢复。**`等待受控补充`也在这一组，而它是刻意留下
// 的**：单看恢复动作它该与人工复核同组，但 ADR-0086 判过这一格且 ADR-0045 把受控补充的重触发
// 判断划为另一切片，那条前提今天核不实也证不伪，ADR-0094 Decision 三因此维持原判。取证见票
// `first-tenant-runway/09`——**那一票若判出它不自愈，本用例要跟着改，而不是反过来**。
func TestRetriedWaitsRollBack(t *testing.T) {
	rolledBack := []domain.ResumePath{
		domain.ResumeByInternalRetry,
		domain.ResumeByCustomerSupplement,
	}

	for _, path := range rolledBack {
		if err := undecidedDisposition(path); !errors.Is(err, ErrAcceptanceChainUndecided) {
			t.Errorf("%s 应当整笔回滚重投，实际 err = %v", path.String(), err)
		}
	}
}

// TestWaitsThatRetryCannotMoveAreCommitted 钉住入账那一组。
//
// 续办方是授权复核角色，**不是本进程重试推得动的**。按 ADR-0086 给`等待人工复核`开的那个形状，
// 它按「本份投递处理完毕」提交入账：等待态与处理尝试因此留在库里，而不是随回滚蒸发。
func TestWaitsThatRetryCannotMoveAreCommitted(t *testing.T) {
	cannotBeRetried := []domain.ResumePath{
		domain.ResumeByManualReview,
	}

	for _, path := range cannotBeRetried {
		if err := undecidedDisposition(path); err != nil {
			t.Errorf("%s 重投推不动，应按本份投递处理完毕入账，实际 err = %v", path.String(), err)
		}
	}
}

// TestOperatorRegistrationRollsBackUntilItsResumeTriggerLands 钉住那个**过渡态**。
//
// `等待运营登记`按恢复动作本该与人工复核同组入账，但 ADR-0094 Decision 四写的是「第四格必须与
// 它的续办触发同笔落地，否则不许落地」——续办信封（D4）与落等待态那一段（D5）今天都没有，此刻
// 入账只会把 ABANDONED 换成一个更安静的永久停滞。所以在 D4/D5 落地前它暂按旧行为回滚重投
// （票 first-tenant-runway/07，MCP-3 裁）。**D4/D5 那一笔落地时本用例要反过来**：把它并进
// 上面那一组。它单独成一条而不是塞进 TestRetriedWaitsRollBack，是为了让那一笔的人一眼看见
// 该动哪一行。
func TestOperatorRegistrationRollsBackUntilItsResumeTriggerLands(t *testing.T) {
	if err := undecidedDisposition(domain.ResumeByOperatorRegistration); !errors.Is(err, ErrAcceptanceChainUndecided) {
		t.Fatalf("D4/D5 未落地前，等待运营登记应暂按旧行为回滚重投，实际 err = %v", err)
	}
}

// TestUnknownResumePathIsLoud 钉住不留 default 那一条。
//
// 一个立不起来的续办路径意味着编排交回了集合外的东西，或者有人加了第五格却没回来看这里。
// 静默折成任何一格都等于替编排作判断：折成重投会烧穿失败预算，折成入账会让这一封悄悄消失。
func TestUnknownResumePathIsLoud(t *testing.T) {
	err := undecidedDisposition(domain.ResumePathInvalid)

	if err == nil {
		t.Fatal("立不起来的续办路径被静默入账了——那一封就此消失，没有任何东西会再驱动它")
	}
	if errors.Is(err, ErrAcceptanceChainUndecided) {
		t.Fatal("它挂上了未决哨兵，会被登记成「等依赖」而无休止重投；这不是依赖问题")
	}
}
