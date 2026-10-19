package ports

import "context"

// TransportCommissionStartAdvancement 是采用对委托册的第二把写口：把已开始委托的开始前移到更早的一条控制事实（ADR-0204
// 决定二）。
//
// 不借 TransportCommissionStore.Replace：Replace 只写尚未开始也未取消的那一行，这是取消与首次开始共用的并发闸；前移要的
// 闸正相反——只写已开始、且在册的开始晚于这一次的那一行。两种条件塞进一个口，调用方就得靠记录的形状去猜它走哪一条 WHERE。
// 另立文件、不进 ports.go，理由同 SegmentCommissionAdoptionStore 的文件头。
type TransportCommissionStartAdvancement interface {
	// AdvanceStart 只写开始依据与时刻（连同落册时刻，与 Replace 同一做法），且只作用于已开始、未取消、在册开始晚于 record 那一
	// 次开始的行。委托不在册、尚未开始、或在册的开始已经不晚于这一次（另一段先把它前移了），都答 false，由编排读回再判。
	AdvanceStart(ctx context.Context, record TransportCommissionRecord) (bool, error)
}
