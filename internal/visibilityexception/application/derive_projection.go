// Package application 编排 visibility-exception 的用例。判断规则在领域，这里只做
// 受理、幂等、归类分派、派生提交与发布意图的协调。
package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

// ErrUnexpectedFactSave 说明事实库交回了封闭集合以外的写入结果。
var ErrUnexpectedFactSave = errors.New("visibility exception: unexpected fact save outcome")

// DeriveProjectionOutcome 是接收源事实并派生投影的应用处理结果。
type DeriveProjectionOutcome uint8

const (
	DeriveProjectionOutcomeInvalid DeriveProjectionOutcome = iota
	ProjectionDerived
	FactExistingResult
	FactSourceConflict
	DeriveUndecided
	FactNotAccepted
)

func (outcome DeriveProjectionOutcome) String() string {
	switch outcome {
	case ProjectionDerived:
		return "PROJECTION_DERIVED"
	case FactExistingResult:
		return "EXISTING_RESULT"
	case FactSourceConflict:
		return "SOURCE_CONFLICT"
	case DeriveUndecided:
		return "UNDECIDED"
	case FactNotAccepted:
		return "NOT_ACCEPTED"
	default:
		return ""
	}
}

// DeriveUndecidedReason 指名派生停在哪一步。
type DeriveUndecidedReason uint8

const (
	DeriveUndecidedReasonNone DeriveUndecidedReason = iota
	FactStoreUnavailable
	MappingViewUnavailable
	ProjectionStoreUnavailable
	ProjectionIdentityUnavailable
)

func (reason DeriveUndecidedReason) String() string {
	switch reason {
	case FactStoreUnavailable:
		return "FACT_STORE_UNAVAILABLE"
	case MappingViewUnavailable:
		return "MAPPING_VIEW_UNAVAILABLE"
	case ProjectionStoreUnavailable:
		return "PROJECTION_STORE_UNAVAILABLE"
	case ProjectionIdentityUnavailable:
		return "PROJECTION_IDENTITY_UNAVAILABLE"
	default:
		return ""
	}
}

// DeriveProjectionCommand 携带一份已被源上下文接受的事实。投影只消费已接受事实——
// 命令的形状就是 AcceptedSourceFactSpec，原始消息与外部状态码构造不出它。租户显式
// 随命令到达（ADR-0003）：事实引用只在租户内唯一，编排不替来源补租户。
type DeriveProjectionCommand struct {
	TenantID domain.TenantID
	Fact     domain.AcceptedSourceFactSpec
}

type DeriveProjectionResult struct {
	outcome       DeriveProjectionOutcome
	projection    domain.TrackingProjection
	hasProjection bool
	reason        DeriveUndecidedReason
	handoffRef    string
}

func (result DeriveProjectionResult) Outcome() DeriveProjectionOutcome {
	return result.outcome
}

// Projection 只在派生成功（或读回已有）时给出。
func (result DeriveProjectionResult) Projection() (domain.TrackingProjection, bool) {
	return result.projection, result.hasProjection
}

func (result DeriveProjectionResult) UndecidedReason() DeriveUndecidedReason {
	return result.reason
}

// HandoffReference 非空说明投影已提交但意图还没交出去，重放会重发同一份。
func (result DeriveProjectionResult) HandoffReference() string {
	return result.handoffRef
}

type DeriveProjectionDeps struct {
	Facts       ports.AcceptedFactStore
	Mapping     ports.MilestoneMappingView
	Projections ports.ProjectionStore
	Identities  ports.ProjectionIdentityFactory
	Downstream  ports.ProjectionHandoff
	Clock       ports.Clock
}

type DeriveProjectionHandler struct {
	deps DeriveProjectionDeps
}

func NewDeriveProjectionHandler(deps DeriveProjectionDeps) *DeriveProjectionHandler {
	return &DeriveProjectionHandler{deps: deps}
}

// Handle 把一份已接受事实推进到新的投影版本：受理（三时间与来源封闭构造期拦）→
// 幂等/冲突按内容指纹分界 → 事实落库（只增）→ 逐事实归类（映射未配置即如实未归类，
// 不阻断投影）→ 派生新版本（有当前投影则指回）→ 提交与发布意图。源事实全程只读，
// 投影不使任何源事实失效。
func (handler *DeriveProjectionHandler) Handle(
	ctx context.Context,
	command DeriveProjectionCommand,
) (DeriveProjectionResult, error) {
	fact, err := domain.NewAcceptedSourceFact(command.Fact)
	if err != nil || command.TenantID.String() == "" {
		return DeriveProjectionResult{outcome: FactNotAccepted}, nil
	}

	key := ports.FactKey{
		Tenant:  command.TenantID,
		Source:  fact.Source(),
		Fact:    fact.Fact(),
		Version: fact.Version(),
	}
	digest := factContentDigest(fact)
	existing, found, err := handler.deps.Facts.FindByKey(ctx, key)
	if err != nil {
		return DeriveProjectionResult{outcome: DeriveUndecided, reason: FactStoreUnavailable}, nil
	}
	if found {
		if existing.ContentDigest != digest {
			// 同一来源版本携带不同内容：冲突保留原事实，不按最后到达覆盖。
			return DeriveProjectionResult{outcome: FactSourceConflict}, nil
		}
		// 重放：按当前投影作答，不重复派生。
		current, hasCurrent, err := handler.deps.Projections.FindCurrent(ctx, command.TenantID, fact.Parcel())
		if err != nil || !hasCurrent {
			return DeriveProjectionResult{outcome: FactExistingResult}, nil
		}
		return DeriveProjectionResult{
			outcome:       FactExistingResult,
			projection:    current,
			hasProjection: true,
		}, nil
	}

	saved, err := handler.deps.Facts.Save(ctx, ports.FactRecord{
		Key:           key,
		ContentDigest: digest,
		Fact:          fact,
	})
	if err != nil {
		return DeriveProjectionResult{outcome: DeriveUndecided, reason: FactStoreUnavailable}, nil
	}
	if saved != ports.FactSaved && saved != ports.FactAlreadyRecorded {
		return DeriveProjectionResult{}, fmt.Errorf("%w: %d", ErrUnexpectedFactSave, saved)
	}

	records, err := handler.deps.Facts.FindByParcel(ctx, command.TenantID, fact.Parcel())
	if err != nil {
		return DeriveProjectionResult{outcome: DeriveUndecided, reason: FactStoreUnavailable}, nil
	}
	entries := make([]domain.MilestoneClassification, 0, len(records))
	for _, record := range records {
		classification, err := handler.classify(ctx, record.Fact)
		if err != nil {
			return DeriveProjectionResult{outcome: DeriveUndecided, reason: MappingViewUnavailable}, nil
		}
		entries = append(entries, classification)
	}

	version, err := handler.deps.Identities.NextProjectionVersionID(ctx)
	if err != nil {
		return DeriveProjectionResult{outcome: DeriveUndecided, reason: ProjectionIdentityUnavailable}, nil
	}
	current, hasCurrent, err := handler.deps.Projections.FindCurrent(ctx, command.TenantID, fact.Parcel())
	if err != nil {
		return DeriveProjectionResult{outcome: DeriveUndecided, reason: ProjectionStoreUnavailable}, nil
	}
	var projection domain.TrackingProjection
	if hasCurrent {
		projection, err = current.Rederive(version, entries, handler.deps.Clock.Now())
	} else {
		projection, err = domain.DeriveTrackingProjection(version, fact.Parcel(), entries, handler.deps.Clock.Now())
	}
	if err != nil {
		return DeriveProjectionResult{}, fmt.Errorf("derive tracking projection: %w", err)
	}
	if err := handler.deps.Projections.Save(ctx, command.TenantID, projection); err != nil {
		return DeriveProjectionResult{outcome: DeriveUndecided, reason: ProjectionStoreUnavailable}, nil
	}

	result := DeriveProjectionResult{
		outcome:       ProjectionDerived,
		projection:    projection,
		hasProjection: true,
	}
	if err := handler.deps.Downstream.HandOffProjection(ctx, ports.ProjectionHandoffIntent{
		TenantID:   command.TenantID,
		Projection: projection,
	}); err != nil {
		result.handoffRef = "CONT-" + shortDigest("PROJECTION_HANDOFF", projection.Version().String())
	}
	return result, nil
}

// classify 逐事实归类：映射目录未配置即如实未归类（无法可靠映射不强行映射——那不是
// 未决，投影照常派生）；依赖调不通才是未决。
func (handler *DeriveProjectionHandler) classify(
	ctx context.Context,
	fact domain.AcceptedSourceFact,
) (domain.MilestoneClassification, error) {
	answer, configured, err := handler.deps.Mapping.ClassifyFact(ctx, fact)
	if err != nil {
		return domain.MilestoneClassification{}, err
	}
	if !configured {
		fallback, err := domain.NewMappingVersionReference("MAPPING_NOT_CONFIGURED")
		if err != nil {
			return domain.MilestoneClassification{}, err
		}
		return domain.LeaveUnclassified(fact, fallback)
	}
	if !answer.Classified {
		return domain.LeaveUnclassified(fact, answer.Mapping)
	}
	return domain.ClassifyMilestone(fact, answer.Milestone, answer.Mapping)
}

func factContentDigest(fact domain.AcceptedSourceFact) string {
	return shortDigest(
		fact.Parcel().String(),
		fact.OccurredAt().UTC().Format(time.RFC3339Nano),
		fact.EffectiveAt().UTC().Format(time.RFC3339Nano),
	)
}

func shortDigest(parts ...string) string {
	digest := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(digest[:8])
}
