package ports

import "context"

// TransportScheduleReader 按（租户 + 班次）取回已登记的班次，供引用班次的决定核对「班次在册」。
//
// 单开一个只读口而不直接依赖 TransportScheduleStore，理由同 DispatchTaskReader：那个口带 Save，
// 而核对在册只读班次，依赖它等于声明自己可能建立班次。TransportSchedules 适配器天然满足它。
//
// 键里带租户：他租户登记的同名班次按本租户的键读不回，引用方因此不必、也不该另判租户。
type TransportScheduleReader interface {
	FindByKey(ctx context.Context, key ScheduleKey) (ScheduleRecord, bool, error)
}
