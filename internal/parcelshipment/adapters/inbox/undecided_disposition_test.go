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

// TestRetriedWaitsRollBack 钉住重投那一组——如今只剩`等待内部续办`一格：它等的依赖会自行
// 恢复，回滚重跑是对的。四格里其余三格的续办方都在进程之外（ADR-0106 Consequences 那句
// 「消费门只剩一个回滚格」）。
func TestRetriedWaitsRollBack(t *testing.T) {
	rolledBack := []domain.ResumePath{
		domain.ResumeByInternalRetry,
	}

	for _, path := range rolledBack {
		if err := undecidedDisposition(path); !errors.Is(err, ErrAcceptanceChainUndecided) {
			t.Errorf("%s 应当整笔回滚重投，实际 err = %v", path.String(), err)
		}
	}
}

// TestCustomerSupplementIsCommittedNowThatItsResumeTriggerLands 钉住`等待受控补充`的入账
// （ADR-0106 Decision 一）。
//
// 它曾与`等待内部续办`同在回滚那一组，且是刻意留下的：ADR-0086 Context 判过「客户新提交版本会
// 自己回来」，ADR-0094 Decision 三因缺证据维持原判并把取证交给票 first-tenant-runway/09。那一票
// 从代码答出三条——受控补充编排不铸信封且无生产调用方、等待态从未落库、旧信封重投即便新版本
// 到达也推不动链——前提两半都不成立，这一格才并回入账那一组。与`等待运营登记`一样单列一条，
// 让读到的人看见它的过渡史：谁再想把它挪回重投，先回答「新提交版本已形成」那封信封去哪了。
func TestCustomerSupplementIsCommittedNowThatItsResumeTriggerLands(t *testing.T) {
	if err := undecidedDisposition(domain.ResumeByCustomerSupplement); err != nil {
		t.Fatalf("续办触发已落地，等待受控补充应按本份投递处理完毕入账，实际 err = %v", err)
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

// TestOperatorRegistrationIsCommittedNowThatItsResumeTriggerLands 钉住第四格的入账（ADR-0094
// Decision 三）。
//
// 它曾反着写（`…RollsBackUntilItsResumeTriggerLands`）：Decision 四要求「第四格必须与它的续办触发
// 同笔落地，否则不许落地」，而落等待态那一段（D5）与「参数已登记」信封（D4）当时都没有，入账只会
// 把 ABANDONED 换成一个更安静的永久停滞。两半如今都在——D5 由两条 as-of 编排先 Save 等待态再交回
// 原因，D4 由 party-commercial 的登记动作同事务发信封、OperatorRegistrationCompletedConsumer 按租户
// 重驱——这一格才并回入账那一组。仍单列一条而不并进 TestWaitsThatRetryCannotMoveAreCommitted，
// 是为了让读到它的人看见这一格是有过渡史的：谁再想把它挪回重投，先回答续办触发去哪了。
func TestOperatorRegistrationIsCommittedNowThatItsResumeTriggerLands(t *testing.T) {
	if err := undecidedDisposition(domain.ResumeByOperatorRegistration); err != nil {
		t.Fatalf("续办触发已落地，等待运营登记应按本份投递处理完毕入账，实际 err = %v", err)
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
