package application

import (
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// TestEveryPendingReasonHasAStringAndAResumePath 堵一条静默链。
//
// 漏补 String() 的取值交回空串，而空串会一路无声地走完：`recordAttempt` 里
// `NewProcessingAttemptReason("")` 报 `ErrBlankValue`，那一支直接返回，于是处理尝试悄悄
// 不落库——`UC-PS-001` 要的「任务同时留下判断与处理尝试」就此失效，一份卡了十轮的委托看
// 起来和刚建单的一样。同一个空串还会进 `judgmentContinuation` 的摘要，让所有漏登记的原因
// 共用同一条续办引用，调用方按引用查回来的是别人的缺口。全程没有任何东西变红。
//
// 它遍历到 judgmentPendingReasonEnd 而不是照着一份手写名单查，因此新增取值当天就被查到，
// 不必有人记得回来加一行。本测试放在包内正是为了够得到那个上界——把上界导出去只为测试可见，
// 会让一个不是原因的东西出现在调用方的取值集合里。
func TestEveryPendingReasonHasAStringAndAResumePath(t *testing.T) {
	t.Parallel()

	if judgmentPendingReasonEnd <= PendingReasonNone+1 {
		t.Fatal("封闭集合是空的；本用例会永远空过")
	}

	seen := make(map[string]JudgmentPendingReason, judgmentPendingReasonEnd)
	for reason := PendingReasonNone + 1; reason < judgmentPendingReasonEnd; reason++ {
		name := reason.String()
		if name == "" {
			t.Errorf("JudgmentPendingReason(%d) 没有 String()；它会让处理尝试静默不落库", uint8(reason))
			continue
		}
		if first, duplicated := seen[name]; duplicated {
			t.Errorf("JudgmentPendingReason(%d) 与 (%d) 同为 %q；两者会共用一条续办引用",
				uint8(reason), uint8(first), name)
		}
		seen[name] = reason

		// 续办路径同样不许落到零值：`WaitingOn` 拿零值当缺席，一条报告缺席的等待态等于
		// 没人会来续办这一轮。
		//
		// 这里逐个列出各等待态而不调 `ResumePath.valid()`，是有意的，别改：**这份枚举
		// 是承重的**。日后再加一个等待态时本用例会变红，而那一红正是要逼人回来逐条确认
		// 每个未决原因映得对不对——加一格几乎必然意味着某些原因该改归属（ADR-0094 就是
		// 这么发现 *NotConfigured 那一族一直压在内部重试上的；ADR-0132 加`等待授权处置`
		// 时逐条复核过：只有 AuthorizedDispositionPending 归它，两格 ControlDisposition*
		// 仍是内部重试）。改成跟着 `valid()` 走，它就永远不会红，而那件必须做的复核也就
		// 永远没人做。
		switch reason.resumePath() {
		case domain.ResumeByCustomerSupplement, domain.ResumeByInternalRetry,
			domain.ResumeByManualReview, domain.ResumeByOperatorRegistration,
			domain.ResumeByAuthorizedDisposition:
		default:
			t.Errorf("JudgmentPendingReason(%d) 的续办路径不是已登记的等待态之一", uint8(reason))
		}
	}

	// `未设`不是原因，不该有名字——它是 recordAttempt 那条空串防线要拦的东西本身。
	if PendingReasonNone.String() != "" {
		t.Fatalf("PendingReasonNone.String() = %q, want empty", PendingReasonNone.String())
	}
}
