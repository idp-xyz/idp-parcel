package ports

import (
	"context"
	"errors"
)

// TransportScheduleReader 按（租户 + 班次）取回已登记的班次，供引用班次的决定核对「班次在册」。
//
// 单开一个只读口而不直接依赖 TransportScheduleStore，理由同 DispatchTaskReader：那个口带 Save，
// 而核对在册只读班次，依赖它等于声明自己可能建立班次。TransportSchedules 适配器天然满足它。
//
// 键里带租户：他租户登记的同名班次按本租户的键读不回，引用方因此不必、也不该另判租户。
type TransportScheduleReader interface {
	FindByKey(ctx context.Context, key ScheduleKey) (ScheduleRecord, bool, error)
}

// ErrTransportScheduleRecordUnreadable：按键读到了班次那一行，却过不了重建门——库面被绕过构造门写过。TransportSchedules 把这一类
// 包住它交出，与连不上库分开：坏行重读一万次还是坏行（ADR-0029；出发门禁读数据它另记一个停点，补问不为它再问，ADR-0305）。
var ErrTransportScheduleRecordUnreadable = errors.New("transport fulfillment: a stored transport schedule cannot be read back")
