package main

import (
	"fmt"
	"io"
	"strings"
	"time"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/application"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
)

// 方向依据记录在导入报告上的读法（票 no-directional-basis/01「偏离标记半边：设计代决」里「呈现」那一条）。移入照常落地、
// 去向与退出码不因缺依据而变——缺依据不是拒收，读不动也不是未决；报告只把记录里该看的那几格写出来，供内勤对纸单。
//
// 逐行只写`缺少`与`读不动`：`未判`跟着身份不成立走，写出身份那一格就够；路由依据`未配置`对身份成立的每一行都成立，
// 逐行写会把真正的已取消、已终局、身份未确认淹掉，所以只在汇总里说一次，身份不成立那几行的`未判`也在汇总里点数。

var basisAspectLabels = map[domain.DirectionalBasisAspect]string{
	domain.IdentityBasisAspect:      "身份",
	domain.ServiceResultBasisAspect: "服务结果",
	domain.RoutingBasisAspect:       "路由依据",
}

var basisReasonLabels = map[domain.DirectionalBasisReason]string{
	domain.ReasonNoIntakeRecord:        "无收寄记录",
	domain.ReasonPendingIdentification: "待识别",
	domain.ReasonIdentityConflict:      "身份冲突",
	domain.ReasonCancelled:             "已取消",
	domain.ReasonFinal:                 "已终局",
}

// directionalBasisTally 汇总一批移入行的方向依据记录，汇总行据此写。
type directionalBasisTally struct {
	members             int
	missing             int
	unreadable          int
	unrecorded          int
	routingUnconfigured int
	unjudged            int
	reasons             map[domain.DirectionalBasisReason]int
}

// count 记一行并交回追加到说明末尾的那一段。只算移入行里有事实可依的两格：新形成（`MEMBER_ADDED`）与同一来源
// 重投（`已有结果`，交回的是册上原样那一份）；其余结果码没有形成或认出移入事实，也就没有记录可读。
func (tally *directionalBasisTally) count(command any, result application.ConsolidationResult) string {
	if _, adding := command.(application.AddMemberCommand); !adding {
		return ""
	}
	if outcome := result.Outcome(); outcome != application.MemberAdded && outcome != application.ConsolidationExistingResult {
		return ""
	}
	tally.members++
	record := result.DirectionalBasis()
	if !record.Recorded() {
		tally.unrecorded++
		return "；方向依据未记录（这条事实落册时还不记方向依据）"
	}
	if _, established := record.Identity().Association(); !established {
		tally.unjudged++
	}
	var missing, unreadable []string
	for _, cell := range record.Cells() {
		switch cell.Answer() {
		case domain.BasisMissing:
			for _, reason := range cell.Reasons() {
				if tally.reasons == nil {
					tally.reasons = map[domain.DirectionalBasisReason]int{}
				}
				tally.reasons[reason]++
				missing = append(missing, basisAspectLabels[cell.Aspect()]+"·"+basisReasonLabels[reason]+referenceOf(record, reason))
			}
		case domain.BasisUnreadable:
			unreadable = append(unreadable, basisAspectLabels[cell.Aspect()])
		case domain.BasisNotConfigured:
			if cell.Aspect() == domain.RoutingBasisAspect {
				tally.routingUnconfigured++
			}
		}
	}
	var detail strings.Builder
	if len(missing) > 0 {
		tally.missing++
		detail.WriteString("；缺少依据：" + strings.Join(missing, "、"))
	}
	if len(unreadable) > 0 {
		tally.unreadable++
		detail.WriteString("；读不动：" + strings.Join(unreadable, "、"))
	}
	// 重投交回的是册上那一份，判断时刻是当时的，与这一次导入不同，写出来免得读成刚判的。
	if result.Outcome() == application.ConsolidationExistingResult {
		detail.WriteString("；方向依据判于 " + record.JudgedAt().Format(time.RFC3339))
	}
	return detail.String()
}

// referenceOf 给服务结果的两种缺少带上 parcel-shipment 的引用，内勤拿它去 PS 那边核。
func referenceOf(record domain.DirectionalBasisRecord, reason domain.DirectionalBasisReason) string {
	switch reason {
	case domain.ReasonCancelled:
		return "（" + record.ServiceResult().Cancellation() + "）"
	case domain.ReasonFinal:
		kind, version := record.ServiceResult().Final()
		return "（" + kind + " " + version + "）"
	default:
		return ""
	}
}

// writeDirectionalBasisTally 在汇总行之后补一行方向依据汇总；批里没有移入行就不写。时间语义写在行里：记录是
// 判断时刻所知，不是装袋那一刻——历史装袋可能早于 PS 的取消，先后归 PS 按业务时间裁。
func writeDirectionalBasisTally(out io.Writer, tally directionalBasisTally) {
	if tally.members == 0 {
		return
	}
	var reasons []string
	for _, reason := range []domain.DirectionalBasisReason{
		domain.ReasonNoIntakeRecord, domain.ReasonPendingIdentification, domain.ReasonIdentityConflict,
		domain.ReasonCancelled, domain.ReasonFinal,
	} {
		if n := tally.reasons[reason]; n > 0 {
			reasons = append(reasons, fmt.Sprintf("%s %d", basisReasonLabels[reason], n))
		}
	}
	line := fmt.Sprintf("方向依据（判断时刻所知，不是装袋那一刻）：移入 %d 行，缺少依据 %d 行", tally.members, tally.missing)
	if len(reasons) > 0 {
		line += "（" + strings.Join(reasons, "，") + "）"
	}
	line += fmt.Sprintf("，读不动 %d 行，未记录 %d 行", tally.unreadable, tally.unrecorded)
	if tally.routingUnconfigured > 0 {
		line += fmt.Sprintf("；路由依据未配置 %d 行（路由指令与集运兼容判断尚未建成，方向相容与否无从判断）", tally.routingUnconfigured)
	}
	if tally.unjudged > 0 {
		line += fmt.Sprintf("；身份不成立 %d 行，服务结果与路由依据未判", tally.unjudged)
	}
	fmt.Fprintln(out, line)
}
