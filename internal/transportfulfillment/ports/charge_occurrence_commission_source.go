package ports

import "context"

// 运输收费发生项登记读委托一侧的窄只读口（票 demo-chain-gaps/28，ADR-0207 文末补记）。订舱与取消两种原因的事实是订舱、
// 它所属的委托与承运答复；实际履约段已采用委托时，采购上下文从那份委托的快照取（ADR-0204 决定二末条）。
//
// 每口只有读：登记发生项不改委托、订舱、答复与采用连线，依赖带写口的那几个存取口等于声明自己可能改它们（FailedAttemptSource
// 头注同一条理由）。各口的形与对应存取口的读方法一字不差，postgres 那几个适配器天然满足，不需要第二个实现。另立文件、不进
// ports.go，理由同 SegmentCommissionAdoptionStore 的文件头。

// SegmentCommissionAdoptionView 按段读采用连线：段已采用委托时，发生项的采购上下文取那份委托的快照。
type SegmentCommissionAdoptionView interface {
	FindBySegment(ctx context.Context, key SegmentCommissionAdoptionKey) (SegmentCommissionAdoptionRecord, bool, error)
}

// TransportCommissionView 按键读运输委托：服务提供方、协议快照、采购责任法人与成员都在它上。
type TransportCommissionView interface {
	FindByKey(ctx context.Context, key TransportCommissionKey) (TransportCommissionRecord, bool, error)
}

// BookingView 按键读订舱申请：订舱与取消发生项的事实。
type BookingView interface {
	FindByKey(ctx context.Context, key BookingKey) (BookingRecord, bool, error)
}

// BookingAnswerView 按订舱键读当前那一份承运答复：有定局交定局，没有定局交候补（BookingAnswerStore.FindByKey 同一读法）。
// 订舱费的依据是已接受的那一份，取消费也只认接受过的订舱。
type BookingAnswerView interface {
	FindByKey(ctx context.Context, key BookingKey) (BookingAnswerRecord, bool, error)
}
