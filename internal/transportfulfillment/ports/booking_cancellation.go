package ports

import "context"

// BookingCancellationStore 是取消订舱对订舱册的写口：只落取消这一格（ADR-0204 决定三）。
//
// 另立一口而不在 BookingStore 上加 Replace：BookingStore 在 ports.go 里，在途的别的会话有它的替身，加方法会让那些替身
// 编不过（tf-chain-plan「跨票通则」X4）；取消也只有取消编排要，提交与答复两条路用不着这把写口。
type BookingCancellationStore interface {
	// Replace 只写取消时刻，且只作用于尚未取消的那一行：订舱不在册与已被另一笔先取消都答 false，由编排读回再答。申请量、
	// 班次与委托只在提交时写，这个口改不了它们。
	Replace(ctx context.Context, record BookingRecord) (bool, error)
}
