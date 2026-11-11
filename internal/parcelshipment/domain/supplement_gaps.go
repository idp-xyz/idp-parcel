package domain

// SupplementGap 是受理任务停在等待受控补充时缺的一项（ADR-0376 决定一）：哪一判组、哪一件包裹（委托级缺口为空）、什么原因
// （提交条件码，或别组交来的原因）。Decide 停进`等待受控补充`时从本轮的`未定`结果里摘下来记在任务上——校验结果本身只在形成
// 决定时随决定保存，不记下来，客户与运营就只看得到「在等补充」，看不到缺的是哪一条、哪几箱。
type SupplementGap struct {
	group  AcceptanceCheckGroup
	parcel DeclaredParcelID
	reason CheckReason
}

// NewSupplementGap 供重建用；Decide 自己从校验结果里摘。
func NewSupplementGap(group AcceptanceCheckGroup, parcel DeclaredParcelID, reason CheckReason) (SupplementGap, error) {
	if !group.valid() || !reason.valid() {
		return SupplementGap{}, ErrInvalidAcceptanceCheck
	}
	return SupplementGap{group: group, parcel: parcel, reason: reason}, nil
}

func (gap SupplementGap) Group() AcceptanceCheckGroup { return gap.group }

// Parcel 的第二个返回值为假即委托级的缺口。
func (gap SupplementGap) Parcel() (DeclaredParcelID, bool) { return gap.parcel, gap.parcel.valid() }

func (gap SupplementGap) Reason() CheckReason { return gap.reason }

// supplementGapsOf 摘出续办方是客户的那几条`未定`。
func supplementGapsOf(checks []AcceptanceCheck) []SupplementGap {
	var gaps []SupplementGap
	for _, check := range checks {
		if check.outcome == CheckUndetermined && check.resumePath == ResumeByCustomerSupplement {
			gaps = append(gaps, SupplementGap{group: check.group, parcel: check.parcelID, reason: check.reason})
		}
	}
	return gaps
}

// SupplementGaps 交回任务停在等待受控补充时缺的各项；不在等补充时为空。
func (task AcceptanceDecisionTask) SupplementGaps() []SupplementGap {
	return append([]SupplementGap(nil), task.supplementGaps...)
}
