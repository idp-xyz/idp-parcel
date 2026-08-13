// Package ports 定义 parcelpricing 应用层与外界的边界。评价是纯计算，端口只承担
// 请求受理、结果存续与结果交付，不代表任何费用或应收所有权。
package ports

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

type Clock interface {
	Now() time.Time
}

type EvaluationSaveOutcome uint8

const (
	EvaluationSaveOutcomeInvalid EvaluationSaveOutcome = iota
	EvaluationSaved
	EvaluationAlreadyRecorded
)

// EvaluationStore 按评价标识找回并保存评价结果（写入代数同 ADR-0031）。失败评价
// 同样入册——失败是版本化的计算事实，不是丢弃品。
type EvaluationStore interface {
	FindByID(ctx context.Context, id domain.EvaluationID) (domain.PricingEvaluation, bool, error)
	Save(ctx context.Context, evaluation domain.PricingEvaluation) (EvaluationSaveOutcome, error)
}

// EvaluationHandoffIntent 把评价结果交给适用下游（费用采用在 settlement-accounting，
// 本上下文只交结果不造费用）。意图由评价标识认领，重放重发同一份（ADR-0043）。
type EvaluationHandoffIntent struct {
	Evaluation domain.PricingEvaluation
}

// EvaluationHandoff 今天没有实现，唯一实现是测试替身；事务发布仍阻断于 ADR-0017
// 的 Bento/Outbox 闸门。
type EvaluationHandoff interface {
	HandOffEvaluation(ctx context.Context, intent EvaluationHandoffIntent) error
}
