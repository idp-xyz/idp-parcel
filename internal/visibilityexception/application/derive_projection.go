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

// ConflictSignalDisposition 说明一处无法裁决的冲突在信号那一步落到了哪里。三格的恢复
// 动作各不相同：已进分诊什么都不必做；规则未配置等租户登记 `PAR-VIS-04`；被搁置的要
// 重放本轮（规则视图调不通，或分诊那一侧本轮停在未决）。
type ConflictSignalDisposition uint8

const (
	ConflictSignalDispositionInvalid ConflictSignalDisposition = iota
	ConflictSignalRaised
	ConflictSignalRuleNotConfigured
	ConflictSignalDeferred
)

func (disposition ConflictSignalDisposition) String() string {
	switch disposition {
	case ConflictSignalRaised:
		return "SIGNAL_RAISED"
	case ConflictSignalRuleNotConfigured:
		return "SIGNAL_RULE_NOT_CONFIGURED"
	case ConflictSignalDeferred:
		return "SIGNAL_DEFERRED"
	default:
		return ""
	}
}

// FactConflict 是本轮派生里一处按业务时间无法裁决的替代链分叉：裁决判断（保留着全部
// 各方）、据它形成的异常信号（规则未配置时缺席）与信号的去向。投影本身不因它改变——
// 各方都留在场按信息待确认表达，这里记的是「冲突这件事被交到哪里了」。
type FactConflict struct {
	judgment    domain.ConflictJudgment
	signal      domain.ExceptionSignal
	hasSignal   bool
	disposition ConflictSignalDisposition
}

func (conflict FactConflict) Judgment() domain.ConflictJudgment {
	return conflict.judgment
}

// Signal 只在冲突信号规则已登记、信号已形成时给出。
func (conflict FactConflict) Signal() (domain.ExceptionSignal, bool) {
	return conflict.signal, conflict.hasSignal
}

func (conflict FactConflict) Disposition() ConflictSignalDisposition {
	return conflict.disposition
}

type DeriveProjectionResult struct {
	outcome       DeriveProjectionOutcome
	projection    domain.TrackingProjection
	hasProjection bool
	conflicts     []FactConflict
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

// Conflicts 给出本轮派生时按业务时间无法裁决的各处分叉（副本）。可裁决的分叉不在这里
// ——它们不是异常。
func (result DeriveProjectionResult) Conflicts() []FactConflict {
	return append([]FactConflict(nil), result.conflicts...)
}

func (result DeriveProjectionResult) UndecidedReason() DeriveUndecidedReason {
	return result.reason
}

// HandoffReference 非空说明投影已提交但意图还没交出去，重放会重发同一份。
func (result DeriveProjectionResult) HandoffReference() string {
	return result.handoffRef
}

// SignalRaiser 是 `UC-VE-004` 的入口：无法裁决的冲突形成的信号从这里进分诊。RaiseSignalHandler
// 满足它；接口立在本包，是为了让派生编排不必持有另一个编排的具体类型，替身也好换。
type SignalRaiser interface {
	Handle(ctx context.Context, command RaiseSignalCommand) (RaiseSignalResult, error)
}

type DeriveProjectionDeps struct {
	Facts         ports.AcceptedFactStore
	Mapping       ports.MilestoneMappingView
	Projections   ports.ProjectionStore
	Identities    ports.ProjectionIdentityFactory
	ConflictRules ports.ConflictSignalRuleView
	Signals       SignalRaiser
	Downstream    ports.ProjectionHandoff
	Clock         ports.Clock
}

type DeriveProjectionHandler struct {
	deps DeriveProjectionDeps
}

func NewDeriveProjectionHandler(deps DeriveProjectionDeps) *DeriveProjectionHandler {
	return &DeriveProjectionHandler{deps: deps}
}

// Handle 把一份已接受事实推进到新的投影版本：受理（三时间与来源封闭构造期拦）→
// 幂等/冲突按内容指纹分界 → 事实落库（只增）→ 逐事实归类（映射未配置即如实未归类，
// 不阻断投影）→ 派生新版本（有当前投影则指回）→ 提交 → 替代链分叉按业务时间裁决，
// 裁不了的形成适用异常信号进分诊（`UC-VE-004`）→ 发布意图。源事实全程只读，投影不使
// 任何源事实失效，裁决与信号也不使——它们只是判断与提示（CONTEXT）。
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
	// 被替代条目留档不参与派生（CONTEXT 硬句）：标准里程碑只由当前有效即未被替代的
	// 条目派生——留在事实库是为可追溯，不进条目是为不同时呈现两个互斥结果。过滤在
	// 编排内做，事实库仍交回全量；替代链分叉时 CurrentlyEffective 把各后继都留在场，
	// 投影据此按信息待确认表达，不择一。
	facts := make([]domain.AcceptedSourceFact, 0, len(records))
	for _, record := range records {
		facts = append(facts, record.Fact)
	}
	effective := domain.CurrentlyEffective(facts)
	entries := make([]domain.MilestoneClassification, 0, len(effective))
	for _, candidate := range effective {
		classification, err := handler.classify(ctx, candidate)
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

	conflicts, err := handler.judgeForks(ctx, command.TenantID, effective)
	if err != nil {
		return DeriveProjectionResult{}, err
	}

	result := DeriveProjectionResult{
		outcome:       ProjectionDerived,
		projection:    projection,
		hasProjection: true,
		conflicts:     conflicts,
	}
	if err := handler.deps.Downstream.HandOffProjection(ctx, ports.ProjectionHandoffIntent{
		TenantID:   command.TenantID,
		Projection: projection,
	}); err != nil {
		result.handoffRef = "CONT-" + shortDigest("PROJECTION_HANDOFF", projection.Version().String())
	}
	return result, nil
}

// judgeForks 对当前有效集里的每处替代链分叉按业务时间裁决（`AT-VE-043` 的裁决半边：
// 「多个有效事实冲突时，依据……业务发生时间……形成版本化投影判断」）。裁得出全序的不是
// 异常，不进结果；同刻裁不了的按 CONTEXT「冲突仍无法裁决时……形成适用异常信号」立信号
// 进分诊。其余裁决维度（因果、权威范围）各有自己的函数，这里不混判也不新写。
//
// 信号那一步失败不推翻派生：各方已经留在投影里按信息待确认表达，冲突本身没有丢；丢的
// 只是「这次没把它交进分诊」，记成搁置让重放补上——分诊那一侧对同对象同类型的再命中
// 本就按同一发作期记，重放不会造出第二个信号。
func (handler *DeriveProjectionHandler) judgeForks(
	ctx context.Context,
	tenant domain.TenantID,
	effective []domain.AcceptedSourceFact,
) ([]FactConflict, error) {
	var conflicts []FactConflict
	for _, fork := range domain.SupersessionForks(effective) {
		judgment, err := domain.ResolveByBusinessTime(fork)
		if err != nil {
			return nil, fmt.Errorf("resolve forked supersession: %w", err)
		}
		if judgment.Resolved() {
			continue
		}
		conflict, err := handler.raiseConflict(ctx, tenant, judgment)
		if err != nil {
			return nil, err
		}
		conflicts = append(conflicts, conflict)
	}
	return conflicts, nil
}

// raiseConflict 依据一份未裁决的判断形成信号并交进分诊。信号的类型、规则版本与可信度
// 三样全来自已登记的冲突信号规则（`PAR-VIS-04`）——未配置即如实记「无适用信号」，不替
// 租户拟一种异常类型。
func (handler *DeriveProjectionHandler) raiseConflict(
	ctx context.Context,
	tenant domain.TenantID,
	judgment domain.ConflictJudgment,
) (FactConflict, error) {
	rule, configured, err := handler.deps.ConflictRules.ConflictSignalRule(ctx)
	if err != nil {
		return FactConflict{judgment: judgment, disposition: ConflictSignalDeferred}, nil
	}
	if !configured {
		return FactConflict{judgment: judgment, disposition: ConflictSignalRuleNotConfigured}, nil
	}

	now := handler.deps.Clock.Now()
	signal, err := domain.RaiseConflictSignal(rule.Kind, judgment, now)
	if err != nil {
		// 判断已核过未裁决且带保留事实，走到这里只剩规则答复缺类型——端口坏答复，上抛。
		return FactConflict{}, fmt.Errorf("raise conflict signal: %w", err)
	}
	conflict := FactConflict{judgment: judgment, signal: signal, hasSignal: true}

	raised, err := handler.deps.Signals.Handle(ctx, RaiseSignalCommand{
		TenantID:   tenant,
		Kind:       signal.Kind(),
		Parcel:     signal.Parcel(),
		Rule:       rule.Rule,
		Confidence: rule.Confidence,
		HitAt:      signal.RaisedAt(),
	})
	if err != nil {
		return FactConflict{}, fmt.Errorf("hand conflict signal to triage: %w", err)
	}
	switch raised.Outcome() {
	case SignalEpisodeOpened, SignalHitRecorded, SignalEpisodeReopened:
		conflict.disposition = ConflictSignalRaised
	default:
		// 分诊那一侧停在未决或未受理：信号形成了但没进去，留给重放。
		conflict.disposition = ConflictSignalDeferred
	}
	return conflict, nil
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

// factContentDigest 取事实的内容指纹，幂等与冲突的分界线。前身引用与类型同为内容维：
// 同一事实重投带不同前身，说的已是另一份替代关系，该当成冲突而不是重放；首登无前身
// 以空串入指纹，与任何前身都分得开。接收时间刻意不进指纹——同内容迟到重投是重放。
func factContentDigest(fact domain.AcceptedSourceFact) string {
	supersedes, _ := fact.Supersedes()
	return shortDigest(
		fact.Parcel().String(),
		fact.Kind().String(),
		supersedes.String(),
		fact.OccurredAt().UTC().Format(time.RFC3339Nano),
		fact.EffectiveAt().UTC().Format(time.RFC3339Nano),
	)
}

func shortDigest(parts ...string) string {
	digest := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(digest[:8])
}
