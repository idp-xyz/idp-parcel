package psinbox

import (
	"errors"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// ErrUnknownResumePath 表示编排交回的未决带着一个立不起来的续办路径。它不进未决名单，也不
// 静默入账：两种静默各自都会造成相反的坏结果，而它们都比一次响亮的失败难查。
var ErrUnknownResumePath = errors.New("parcel shipment inbox: undecided carries an unusable resume path")

// undecidedDisposition 把一轮未决按**恢复动作**折成消费门认的两格：交回 nil 即本份投递处理
// 完毕（提交入账），交回哨兵即整笔回滚等重投（ADR-0094 Decision 一）。
//
// 按 `ResumePath` 折而不按 `JudgmentPendingReason` 的具体取值折，是这条决定的要害。恢复动作
// 是领域语言，映射由 `JudgmentPendingReason.resumePath()` 那个全函数单一持有；消费门若自己
// 认原因名字，就是把同一份知识写第二遍，而两份必然漂开——漂开的症状是同一个原因在两个门下
// 一个重投一个入账。
//
// `等待内部续办`重投：它等的依赖会自行恢复，回滚重跑是对的。各格里如今只有它在这一组。
//
// `等待人工复核`、`等待运营登记`、`等待受控补充`与`等待授权处置`入账：它们的续办方都不是本进程，
// 重投只会把失败预算烧尽，而预算烧尽之后连那份等待态都随回滚一起蒸发，队列读面从此列不出这份
// 委托（ADR-0086 给`等待人工复核`开例外时列的两条理由，对其余几格逐字成立）。
//
// 不留 default：新增一个等待态时这里要报错，而不是静默继承某一格。那一格决定的是烧不烧失败
// 预算，静默继承等于替编排作判断。
func undecidedDisposition(path domain.ResumePath) error {
	switch path {
	case domain.ResumeByInternalRetry:
		return ErrAcceptanceChainUndecided
	case domain.ResumeByManualReview, domain.ResumeByOperatorRegistration, domain.ResumeByCustomerSupplement,
		domain.ResumeByAuthorizedDisposition:
		// 走到这几格时编排已把等待态 Save 进聚合——没保存成时它交回的是保存那一格自己的
		// 原因（`等待运营登记`那一支是 OperatorRegistrationWaitNotSaved，其余几格是
		// DecisionNotRecorded / 换代冲突），其恢复动作是内部重试，因此仍走上面那一支回滚重投
		// （ADR-0086 Decision 一那道护栏，ADR-0094 Decision 五、ADR-0106 Decision 二与
		// ADR-0132 Decision 二各扩用一次）。
		//
		// `等待授权处置`的续办触发是处置命令自身，不是一封信封（ADR-0132 决定二）：`拒绝`在命令
		// 事务里形成决定，`交客户补充`把等待态转到`等待受控补充`、之后由「新提交版本已形成」信封
		// 续办。「触发与格同笔落地」由此满足——触发是那道命令，随本票同笔到位。
		//
		// 其余几格里两格有过渡史，都是「续办触发与入账同笔落地，否则不许落地」这一句拦出来的：
		// `等待运营登记`曾暂按回滚重投，直到「参数已登记」信封（party-commercial 同事务发出）与
		// OperatorRegistrationCompletedConsumer 到位；`等待受控补充`曾与内部续办同组，ADR-0094
		// Decision 三因缺证据维持 ADR-0086 的原判，票 first-tenant-runway/09 从代码答出
		// 「新提交版本不会自己回来、回来了也不是在途那封信封受益」之后，ADR-0106 把它并回入账，
		// 续办由「新提交版本已形成」信封（SubmissionVersionFormedConsumer）驱动。
		return nil
	default:
		return fmt.Errorf("%w: %d", ErrUnknownResumePath, path)
	}
}
