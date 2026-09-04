package ports

import (
	"context"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// SourceConnectorBindingOutcome 是来源连接器绑定登记的写入结果代数。行只增不改：同（租户、
// 序列、绑定版本）第二份按内容比对译成幂等重放或冲突，原行不被顶替——改声明就登新版本。
type SourceConnectorBindingOutcome uint8

const (
	SourceConnectorBindingOutcomeInvalid SourceConnectorBindingOutcome = iota
	// SourceConnectorBindingRegistered：新绑定版本已入册。
	SourceConnectorBindingRegistered
	// SourceConnectorBindingAlreadyRegistered：同键同内容已在册，幂等重放。
	SourceConnectorBindingAlreadyRegistered
	// SourceConnectorBindingConflict：同键在册而内容不同；原行保持原样，改声明请登新版本。
	SourceConnectorBindingConflict
)

func (outcome SourceConnectorBindingOutcome) String() string {
	switch outcome {
	case SourceConnectorBindingRegistered:
		return "REGISTERED"
	case SourceConnectorBindingAlreadyRegistered:
		return "ALREADY_REGISTERED"
	case SourceConnectorBindingConflict:
		return "CONFLICT"
	default:
		return ""
	}
}

// SourceConnectorBindingRegister 是来源连接器绑定的登记写口。绑定是实例半边的格（ADR-0099
// 决定六）：出厂零行，每一行都由租户经受控口登进来。
type SourceConnectorBindingRegister interface {
	Register(ctx context.Context, binding domain.SourceConnectorBinding) (SourceConnectorBindingOutcome, error)
}

// SourceConnectorBindingLoader 读一条序列当前在用的绑定：同（租户、序列）多版本时取最近登记的
// 那一版（同刻按版本号字典序），判定确定、无可维护的指针——与在用序列版本的派生同一形状。
// 没有绑定答 false 不答 error：那是「先去登记绑定」这一格答案。
type SourceConnectorBindingLoader interface {
	LoadCurrentBinding(ctx context.Context, tenant domain.TenantID, seriesID string) (domain.SourceConnectorBinding, bool, error)
}
