package domain

import "time"

// AtRegistryPrecision 是本上下文认登记册时刻的那一条规则：本上下文按微秒认登记册上的时刻。凡结论要在入册
// 之后仍然成立的时刻比较——同键内容相等（重放与冲突）和严格先后的构造门——两侧都先经它折到这一精度再判；
// 只改判定，不改存值：构造门照收命令原值，交给登记册的也是原值。
//
// 取微秒，是因为登记册的时刻列是 timestamptz，存不下微秒以下：按纳秒判，入册前「相等」的两份读回后会「不同」，
// 入册前「严格晚于」的一对落库后会「相等」，撞上库面的严格先后约束。
//
// 前提：本上下文登记册的时刻一律落在 timestamptz 列上。哪一册改存 jsonb 快照或改了列型，这条要重看——
// parcel-shipment 受控关闭册的 jsonb 快照就保留纳秒，所以这不是全仓事实。
//
// 同族先例是 accessidentity 的 sameInstant。它的比较在 PG 适配器里「撞键后读回再比」，只截给定的一侧；本上下文
// 的比较在编排与领域里，并发输家比的是读回的赢家对本次命令，哪一侧来自库并不固定，所以两侧对称地折。
//
// 不叫 RecordedInstant：本上下文的 RecordedAt 是登记落库时刻，同形的名字套在业务时间上会被读成登记时刻。
func AtRegistryPrecision(at time.Time) time.Time {
	return at.UTC().Truncate(time.Microsecond)
}

// sameOnRegistry 是同键内容相等一类的判定，规则见 AtRegistryPrecision。
func sameOnRegistry(left, right time.Time) bool {
	return AtRegistryPrecision(left).Equal(AtRegistryPrecision(right))
}

// afterOnRegistry 是严格先后构造门一类的判定，规则见 AtRegistryPrecision：库面的严格先后约束按截后的值判，
// 领域门按截前的值放行的那一对永远存不进去。
func afterOnRegistry(later, earlier time.Time) bool {
	return AtRegistryPrecision(later).After(AtRegistryPrecision(earlier))
}
