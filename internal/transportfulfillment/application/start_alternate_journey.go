package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// ErrUnexpectedJourneySave 说明旅程库交回了封闭集合以外的写入结果。
var ErrUnexpectedJourneySave = errors.New("transport fulfillment: unexpected alternate journey save outcome")

// JourneyStartOutcome 是旅程启动提交的应用处理结果。取消/终止入口领域没有——独立
// 旅程结构上无回写口（T9 结构防线），编排不造。
type JourneyStartOutcome uint8

const (
	JourneyStartOutcomeInvalid JourneyStartOutcome = iota
	JourneyStarted
	JourneyExistingResult
	JourneyStartConflict
	JourneyNotAccepted
	JourneyUndecided
)

func (outcome JourneyStartOutcome) String() string {
	switch outcome {
	case JourneyStarted:
		return "JOURNEY_STARTED"
	case JourneyExistingResult:
		return "EXISTING_JOURNEY"
	case JourneyStartConflict:
		return "SOURCE_CONFLICT"
	case JourneyNotAccepted:
		return "SOURCE_NOT_ACCEPTED"
	case JourneyUndecided:
		return "JOURNEY_UNDECIDED"
	default:
		return ""
	}
}

// JourneyUndecidedReason 指名提交停在哪一步等谁。
type JourneyUndecidedReason uint8

const (
	JourneyUndecidedReasonNone JourneyUndecidedReason = iota
	JourneyStoreUnavailable
)

func (reason JourneyUndecidedReason) String() string {
	switch reason {
	case JourneyStoreUnavailable:
		return "JOURNEY_STORE_UNAVAILABLE"
	default:
		return ""
	}
}

// StartAlternateJourneyCommand 携带一次替代/退运旅程启动的全部输入。处置依据必备
// ——PS 处置决定或监管退运决定，来路由 BasisKind 显式标记不得冒充（领域已钉）。
type StartAlternateJourneyCommand struct {
	TenantID  domain.TenantID
	Journey   string
	Purpose   domain.JourneyPurpose
	Original  string
	BasisKind domain.DispositionBasisKind
	Basis     string
	Members   []string
	StartedAt time.Time
}

type StartAlternateJourneyResult struct {
	outcome      JourneyStartOutcome
	reason       JourneyUndecidedReason
	record       ports.AlternateJourneyRecord
	hasRecord    bool
	continuation string
	ccHandoff    string
	veHandoff    string
}

func (result StartAlternateJourneyResult) Outcome() JourneyStartOutcome {
	return result.outcome
}

// UndecidedReason 只在`未决`时非零。
func (result StartAlternateJourneyResult) UndecidedReason() JourneyUndecidedReason {
	return result.reason
}

func (result StartAlternateJourneyResult) Record() (ports.AlternateJourneyRecord, bool) {
	return result.record, result.hasRecord
}

func (result StartAlternateJourneyResult) ContinuationReference() string {
	return result.continuation
}

// DispositionHandoffReference 非空说明监管处置链的意图还没交出去，重放会重发同一份。
func (result StartAlternateJourneyResult) DispositionHandoffReference() string {
	return result.ccHandoff
}

// ExceptionHandoffReference 非空说明异常链的意图还没交出去，重放会重发同一份。
func (result StartAlternateJourneyResult) ExceptionHandoffReference() string {
	return result.veHandoff
}

type StartAlternateJourneyDeps struct {
	Journeys    ports.AlternateJourneyStore
	Disposition ports.DispositionExecutionHandoff
	Exception   ports.ExceptionJourneyHandoff
	Clock       ports.Clock
}

type StartAlternateJourneyHandler struct {
	deps StartAlternateJourneyDeps
}

func NewStartAlternateJourneyHandler(deps StartAlternateJourneyDeps) *StartAlternateJourneyHandler {
	return &StartAlternateJourneyHandler{deps: deps}
}

// Handle 启动一条替代/退运旅程：受理（目的+原旅程+处置依据由 FormAlternateJourney
// 把门）→ 幂等按（租户+原旅程+目的+处置依据）——同一处置不开两条旅程 → 原子提交 →
// 意图分链：监管来路交 CC 处置执行事实源+VE 异常链，非监管只交 VE。
func (handler *StartAlternateJourneyHandler) Handle(
	ctx context.Context,
	command StartAlternateJourneyCommand,
) (StartAlternateJourneyResult, error) {
	journey, err := journeyFrom(command)
	if err != nil {
		return StartAlternateJourneyResult{outcome: JourneyNotAccepted}, nil
	}

	key := ports.AlternateJourneyKey{
		TenantID: command.TenantID,
		Original: journey.OriginalJourney(),
		Purpose:  journey.Purpose(),
		Basis:    journey.Basis(),
	}
	digest := journeyDigest(command)
	existing, found, err := handler.deps.Journeys.FindByKey(ctx, key)
	if err != nil {
		return journeyStoreUndecided(command.Journey), nil
	}
	if found {
		if existing.ContentDigest != digest {
			// 同一处置携带不同旅程身份或成员：冲突保留原旅程——同一处置决定不开两条
			// 替代旅程。
			return StartAlternateJourneyResult{outcome: JourneyStartConflict}, nil
		}
		return handler.existingResult(ctx, existing), nil
	}

	record := ports.AlternateJourneyRecord{
		Key:           key,
		ContentDigest: digest,
		Journey:       journey,
		RecordedAt:    handler.deps.Clock.Now(),
	}
	saved, err := handler.deps.Journeys.Save(ctx, record)
	if err != nil {
		return journeyStoreUndecided(command.Journey), nil
	}
	switch saved {
	case ports.AlternateJourneySaved:
		result := StartAlternateJourneyResult{outcome: JourneyStarted, record: record, hasRecord: true}
		result.ccHandoff, result.veHandoff = handler.handOff(ctx, record)
		return result, nil
	case ports.AlternateJourneyAlreadyStarted:
		winner, found, err := handler.deps.Journeys.FindByKey(ctx, key)
		if err != nil || !found {
			return journeyStoreUndecided(command.Journey), nil
		}
		return handler.existingResult(ctx, winner), nil
	default:
		return StartAlternateJourneyResult{}, fmt.Errorf("%w: %d", ErrUnexpectedJourneySave, saved)
	}
}

func journeyFrom(command StartAlternateJourneyCommand) (domain.AlternateJourney, error) {
	spec := domain.AlternateJourneySpec{
		TenantID:  command.TenantID,
		Purpose:   command.Purpose,
		BasisKind: command.BasisKind,
		StartedAt: command.StartedAt,
	}
	var err error
	if spec.Journey, err = domain.NewJourneyReference(command.Journey); err != nil {
		return domain.AlternateJourney{}, err
	}
	if spec.OriginalJourney, err = domain.NewJourneyReference(command.Original); err != nil {
		return domain.AlternateJourney{}, err
	}
	if spec.Basis, err = domain.NewDispositionBasisReference(command.Basis); err != nil {
		return domain.AlternateJourney{}, err
	}
	for _, raw := range command.Members {
		member, err := domain.NewCarriedObjectReference(raw)
		if err != nil {
			return domain.AlternateJourney{}, err
		}
		spec.Members = append(spec.Members, member)
	}
	return domain.FormAlternateJourney(spec)
}

func journeyStoreUndecided(journey string) StartAlternateJourneyResult {
	return StartAlternateJourneyResult{
		outcome:      JourneyUndecided,
		reason:       JourneyStoreUnavailable,
		continuation: journeyContinuation("JOURNEY_STORE_UNAVAILABLE", journey),
	}
}

// existingResult 按已有旅程作答并重发意图（分链规则同首登）。
func (handler *StartAlternateJourneyHandler) existingResult(
	ctx context.Context,
	record ports.AlternateJourneyRecord,
) StartAlternateJourneyResult {
	result := StartAlternateJourneyResult{
		outcome:   JourneyExistingResult,
		record:    record,
		hasRecord: true,
	}
	result.ccHandoff, result.veHandoff = handler.handOff(ctx, record)
	return result
}

// handOff 按来路分链：监管处置的旅程交 CC 处置执行事实源+VE 异常链；非监管只交 VE
// ——普通退运不得混进监管范围的续办与核对（AT-TF-080 分流）。投递失败不翻结果。
func (handler *StartAlternateJourneyHandler) handOff(
	ctx context.Context,
	record ports.AlternateJourneyRecord,
) (ccHandoff string, veHandoff string) {
	intent := ports.AlternateJourneyIntent{Record: record}
	if record.Journey.RegulatoryOrigin() {
		if err := handler.deps.Disposition.HandOffDispositionExecution(ctx, intent); err != nil {
			ccHandoff = journeyContinuation("DISPOSITION_EXECUTION_HANDOFF", record.Key.TenantID.String(), record.Journey.Journey().String())
		}
	}
	if err := handler.deps.Exception.HandOffExceptionJourney(ctx, intent); err != nil {
		veHandoff = journeyContinuation("EXCEPTION_JOURNEY_HANDOFF", record.Key.TenantID.String(), record.Journey.Journey().String())
	}
	return ccHandoff, veHandoff
}

func journeyContinuation(parts ...string) string {
	digest := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return "CONT-" + hex.EncodeToString(digest[:8])
}

// journeyDigest 是同一处置键下的内容比对锚：旅程身份、成员与开始时间任一不同即是
// 另一条旅程。成员先排序——提交顺序不构成不同的旅程。
func journeyDigest(command StartAlternateJourneyCommand) string {
	members := append([]string(nil), command.Members...)
	sort.Strings(members)
	digest := sha256.Sum256([]byte(strings.Join(append([]string{
		command.Journey,
		fmt.Sprintf("%d", command.BasisKind),
		command.StartedAt.UTC().Format(time.RFC3339Nano),
	}, members...), "\x00")))
	return hex.EncodeToString(digest[:])
}
