package domain

import "errors"

// ErrNegativeConfirmedUnsettled 说明按账求出的已确认未结为负。那是账不平，
// 推进失败，不把负数截成 0——截成 0 会把可用余额凭空撑大。
var ErrNegativeConfirmedUnsettled = errors.New(
	"settlement accounting: confirmed unsettled recomputed negative")

// ErrNotReceivableAccount 是映射目标写成了结算账户，账户册上却没有这个应收账户。
// 映射不能留下：留下就会有一笔客户收款对不上作用域（ADR-0182 决定一）。
var ErrNotReceivableAccount = errors.New(
	"settlement accounting: mapping target is not a receivable settlement account")

// PostedAndUnsettled 按 ADR-0181 决定二求两列。
//
// postedNet 是已映射客户方向收付净额 P：确认到账为正，资金退回为负。
// writeoffNet 是指向该作用域客户应收、且尚未撤销的核销分配净额 W。
// confirmed 是当前有效的已确认客户运营应收。
// 入账余额 = P − W。已确认未结 = confirmed − W。未分配就是 P − W，不另加一项。
func PostedAndUnsettled(postedNet, writeoffNet, confirmed int64) (posted, unsettled int64, err error) {
	posted = postedNet - writeoffNet
	unsettled = confirmed - writeoffNet
	if unsettled < 0 {
		return 0, 0, ErrNegativeConfirmedUnsettled
	}
	return posted, unsettled, nil
}
