package tfhttp

import (
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// rfc3339 是本包答复里时刻的唯一写法，命令答复与查阅答复同经它。
//
// 先经 domain.AtRegistryPrecision 折到登记册精度，再按 RFC3339Nano 写：首答的记录取自内存（命令时刻与时钟都带
// 纳秒），重放的记录取自读回（只到微秒）；不先折，同一份记录在首答和重放里会差一截亚微秒尾巴。只到秒又会丢掉
// 重放与`内容冲突`判定实际比到的那部分精度，调用方就拿回显分不出两者。精度只在 domain 定义一处，这里不另写
// 固定小数位；整秒值 RFC3339Nano 连小数点一起省去，与只到秒的写法逐字节相同。
func rfc3339(at time.Time) string {
	return domain.AtRegistryPrecision(at).Format(time.RFC3339Nano)
}

// optionalInstant 把可空时刻转写成串，未发生时交回空串配 omitempty 整键不出现。
// 缺席是真话不是缺陷：不为「未更正」编造一个零时刻——零时刻是合法时间值，用它兼表
// 没发生会让两态在报文上分不开。
func optionalInstant(at *time.Time) string {
	if at == nil {
		return ""
	}
	return rfc3339(*at)
}
