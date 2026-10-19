package domain

import "time"

// AtRegistryPrecision 是本上下文认登记册时刻的那一条规则：本上下文按微秒认登记册上的时刻。凡结论要在入册之后仍然
// 成立的时刻判断——同键内容相等（重放与冲突）、严格先后与判零的构造门——两侧都先经它折到这一精度再判；只改判定，
// 不改存值：构造门照收命令原值，交给登记册的也是原值。
//
// 取微秒，是因为本上下文登记册的时刻列是 timestamptz，存不下微秒以下：按纳秒判，入册前「相等」的两份读回后会
// 「不同」，入册前「严格晚于」的一对落库后会「相等」而撞上库面的严格先后约束，入册前「非零」的时刻读回会成零时刻、
// 整行重建不出来。前提是这些时刻一律落在 timestamptz 列上；哪一册改存 jsonb 快照或改了列型，这条要重看。
//
// 这是本上下文自己的决定，不放 platform 共用：精度随各上下文的持久化形态而变。同族先例是
// transportfulfillment/domain 的 AtRegistryPrecision（同名同实现，同样两侧对称地折）与 accessidentity/adapters/postgres
// 的 sameInstant（撞键读回后只截命令一侧）。
func AtRegistryPrecision(at time.Time) time.Time {
	return at.UTC().Truncate(time.Microsecond)
}
