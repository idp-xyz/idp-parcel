// Package ports 定义 pilotgovernance 应用层与外界的边界。治理记录只存脱敏引用，
// 端口同样不携带任何真实客户、线路或阈值。
package ports

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/pilotgovernance/domain"
)

type Clock interface {
	Now() time.Time
}

// CandidateSetStore 保存已固定的候选版本组。评审决定必须引用在场的组——引用不存在
// 的组，决定的范围身份就是悬空的。
type CandidateSetStore interface {
	FindByID(ctx context.Context, id domain.CandidateVersionSetID) (domain.CandidateVersionSet, bool, error)
	Save(ctx context.Context, set domain.CandidateVersionSet) error
}

// ReviewKey 是评审决定的幂等键：同一评审目标对同一候选版本组只决定一次；范围扩大或
// 版本变更生成新的候选组，自然换键。
type ReviewKey struct {
	Objective  string
	Candidates domain.CandidateVersionSetID
}

type ReviewSaveOutcome uint8

const (
	ReviewSaveOutcomeInvalid ReviewSaveOutcome = iota
	ReviewSaved
	ReviewAlreadyRecorded
)

// ReviewDecisionStore 按幂等键找回并保存评审决定（不可覆盖：写入代数同 ADR-0031）。
type ReviewDecisionStore interface {
	FindByKey(ctx context.Context, key ReviewKey) (domain.StageReviewDecision, bool, error)
	Save(ctx context.Context, key ReviewKey, decision domain.StageReviewDecision) (ReviewSaveOutcome, error)
}

// AuthorityIntervalStore 保存生产权威区间。ListCurrent 交回全部既有区间供冲突预检
// ——重叠必须在准入前被抓住，双写后人工对账是被点名的错误结果。
type AuthorityIntervalStore interface {
	ListCurrent(ctx context.Context) ([]domain.AuthorityInterval, error)
	Append(ctx context.Context, interval domain.AuthorityInterval) error
}
