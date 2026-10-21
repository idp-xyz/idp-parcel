package domain

import "time"

// versionFold 是版本壳上的收尾折进一份挂在它下面、自带区间的正文之后留下的那一点：壳整版离开新的选择（同起点退役，
// 或壳已不在`已生效`），或壳上最新一条区间更正的终点。价格、结算、信用政策正文与供应商协议正文按它判（ADR-0233
// 决定五），授权授予与合同委派也按它判（ADR-0261 决定一、二）——几处说的是同一条折法，只在这里定义一次。
//
// 只截终点，不拿更正的起点去裁正文的前段：截止只截终点（起点取壳此刻的选用起点），而正文起点可以早于壳的起点（发布
// 允许），按整段取交会把那一段旧版唯一可选的时段截成无依据。开放结束的更正不收窄。改起点、延终点的更正今天没有生产
// 写入方，它们怎样作用到正文上归后继票。
//
// 正文自带的区间不改写：折法只在判「此刻还选不选得中」时多加一道，零值即壳上没有收尾。
type versionFold struct {
	cutAt  time.Time
	closed bool
}

// narrowedBy 折进壳上最新一条区间更正。调用方先核更正指的是不是这一版。
func (fold versionFold) narrowedBy(correction ValidityCorrection) versionFold {
	if end, bounded := correction.corrected.EndsAt(); bounded {
		fold.cutAt = end
	}
	return fold
}

// closedWhole 折进整版离开：退役只管退役登记之后作出的新决定，而问到这里的决定都在其后，所以与业务时点无关地一律
// 排除——同起点重发时旧版的整段时间都由后继接着（ADR-0233 越权风险点 2）。
func (fold versionFold) closedWhole() versionFold {
	fold.closed = true
	return fold
}

// excludes 答 at 是否已被收尾排除在这一版之外。
func (fold versionFold) excludes(at time.Time) bool {
	return fold.closed || (!fold.cutAt.IsZero() && !at.Before(fold.cutAt))
}
