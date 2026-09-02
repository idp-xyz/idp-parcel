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
// `等待内部续办`重投：它等的依赖会自行恢复，回滚重跑是对的。
//
// **`等待受控补充`也重投，而这一格是本记录刻意没有改的。** 单看恢复动作它该与人工复核同组
// ——客户补件同样不是本进程重试推得动的。但 ADR-0086 的 Context 判过这一格「回滚重投是
// 对的」，理由是「客户新提交版本会自己回来」；而 ADR-0045 把受控补充的重触发判断划为另一
// 切片，那条前提今天既核不实也证不伪。推翻一条已接受判断要有证据，因此 ADR-0094 Decision 三
// 维持原判，只把它从一个沉默的 default 变成这里一个具名的、写着理由的格。取证见票
// `.scratch/first-tenant-runway/issues/09`；**若那一票判出它不自愈，改的就是这一行**。
//
// `等待人工复核`与`等待运营登记`入账：两者的续办方都不是本进程，重投只会把失败预算烧尽，
// 而预算烧尽之后连那份等待态都随回滚一起蒸发，队列读面从此列不出这份委托（ADR-0086 给
// `等待人工复核`开例外时列的两条理由，对`等待运营登记`逐字成立）。
//
// 不留 default：新增一个等待态时这里要报错，而不是静默继承某一格。那一格决定的是烧不烧失败
// 预算，静默继承等于替编排作判断。
func undecidedDisposition(path domain.ResumePath) error {
	switch path {
	case domain.ResumeByInternalRetry, domain.ResumeByCustomerSupplement:
		return ErrAcceptanceChainUndecided
	case domain.ResumeByManualReview, domain.ResumeByOperatorRegistration:
		// 走到这两格时编排已把等待态 Save 进聚合——没保存成时它交回的是保存那一格自己的
		// 原因，其恢复动作是内部重试，因此仍走上面那一支回滚重投（ADR-0086 Decision 一
		// 那道护栏，ADR-0094 Decision 五把它原样扩用到新格）。
		return nil
	default:
		return fmt.Errorf("%w: %d", ErrUnknownResumePath, path)
	}
}
