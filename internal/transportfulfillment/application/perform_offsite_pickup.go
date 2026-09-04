// Package application 编排 transport-fulfillment 的用例。判断规则在领域，这里只做
// 受理、幂等、逐对象结果提交与发布意图的协调。
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

// ErrUnexpectedPickupSave 说明揽收库交回了封闭集合以外的写入结果。
var ErrUnexpectedPickupSave = errors.New("transport fulfillment: unexpected pickup save outcome")

// PickupOutcome 是一次尝试提交的应用处理结果。逐对象成败在记录里并存（UC-TF-002：
// 任务汇总只能由对象结果派生），这里只回答提交本身的走向。
//
// `来源未受理`与`未决`分格（ADR-0029 按恢复动作分格）：前者是提交自身矛盾——改请求，
// 重来多少次都一样；后者是依赖没答上——等恢复重试同一份。折成一格调用方就不知道该
// 改单还是该重试。
type PickupOutcome uint8

const (
	PickupOutcomeInvalid PickupOutcome = iota
	PickupAttemptRecorded
	PickupExistingResult
	PickupSourceConflict
	PickupNotAccepted
	PickupUndecided
)

func (outcome PickupOutcome) String() string {
	switch outcome {
	case PickupAttemptRecorded:
		return "ATTEMPT_RECORDED"
	case PickupExistingResult:
		return "EXISTING_RESULT"
	case PickupSourceConflict:
		return "SOURCE_CONFLICT"
	case PickupNotAccepted:
		return "SOURCE_NOT_ACCEPTED"
	case PickupUndecided:
		return "PICKUP_UNDECIDED"
	default:
		return ""
	}
}

// PickupUndecidedReason 指名提交停在哪一步等谁。封闭集合：新依赖故障必须补格，不许
// 借用裸字符串标签（同 adopt 侧 IntakeUndecidedReason 的纪律）。
type PickupUndecidedReason uint8

const (
	PickupUndecidedReasonNone PickupUndecidedReason = iota
	PickupStoreUnavailable
	PickupIdentityUnavailable
)

func (reason PickupUndecidedReason) String() string {
	switch reason {
	case PickupStoreUnavailable:
		return "PICKUP_STORE_UNAVAILABLE"
	case PickupIdentityUnavailable:
		return "PICKUP_IDENTITY_UNAVAILABLE"
	default:
		return ""
	}
}

// ObjectPickupSubmission 是执行方对单个载运对象报回的结果。成功必须带控制依据——
// 没有它证明不了运输方取得控制；失败必须带原因依据、不得带控制依据——带了就是把
// 一次失败到场伪装成取得控制（CONTEXT「失败结果不制造实际履约段」）。
type ObjectPickupSubmission struct {
	Object     domain.CarriedObjectReference
	Outcome    domain.AttemptObjectOutcome
	Basis      domain.AttemptResultBasisReference
	Control    domain.TransportControlReference
	OccurredAt time.Time
	// PlannedSegment 是这个对象自己关联的计划履约段，可缺席。**逐对象一个而不是整次到访
	// 一个**：CONTEXT 要求每个对象分别关联自己的计划履约段，共用一个就抹掉了成员差异。
	PlannedSegment string
}

// ObjectSegmentEntry 是一个对象进段那一半的欠账。**逐对象而不是整批一个**（票
// tf-unwired-seven/08 的口径裁定）：一次到访里可能只有部分对象没进去，整批一个引用说不出
// 是哪几个，而 CONTEXT 要求任务汇总只能由对象结果派生。
//
// 只有真有欠账的对象才在列——进去了、没要求进、以及领域正当拒绝的都不在，与单对象入口
// 那边「空串即无欠账」同义。
type ObjectSegmentEntry struct {
	Object domain.CarriedObjectReference
	// ContinuationReference 恒非空：这一格只为登记册读不到或写不进而存在，等它恢复重试
	// 同一份。没有欠账的对象根本不进这张表。
	ContinuationReference string
}

// PerformOffsitePickupCommand 携带一次实际到场的全部来源：尝试身份与内容、逐对象结果。
// 改约或重派由 RescheduledFrom 指向旧尝试——新到场是新尝试，不覆盖旧的。
type PerformOffsitePickupCommand struct {
	TenantID        domain.TenantID
	SourceID        string
	Task            string
	Attempt         string
	ExecutedBy      string
	Place           string
	PlannedFrom     time.Time
	PlannedTo       time.Time
	ArrivedAt       time.Time
	Evidence        string
	RescheduledFrom string
	Objects         []ObjectPickupSubmission
	// Segment 指名这次到访把取得控制的对象送进哪个实际履约段，**缺席时不立段**。它整次
	// 到访共用——同一次到访取得控制的对象进同一个共同控制范围；逐对象那一半是各自的
	// PlannedSegment。两层分设的理由与形状同 enterFulfillmentSegment 的自注。
	Segment string
}

type PerformOffsitePickupResult struct {
	outcome        PickupOutcome
	reason         PickupUndecidedReason
	record         ports.PickupAttemptRecord
	hasRecord      bool
	continuation   string
	handoff        string
	segments       []ObjectSegmentEntry
	segmentRefusal SegmentEntryRefusal
}

func (result PerformOffsitePickupResult) Outcome() PickupOutcome {
	return result.outcome
}

// UndecidedReason 只在`未决`时非零：它指名要等哪个依赖恢复。`来源未受理`没有它——
// 那一格的恢复动作是改请求，不是等谁。
func (result PerformOffsitePickupResult) UndecidedReason() PickupUndecidedReason {
	return result.reason
}

func (result PerformOffsitePickupResult) Record() (ports.PickupAttemptRecord, bool) {
	return result.record, result.hasRecord
}

func (result PerformOffsitePickupResult) ContinuationReference() string {
	return result.continuation
}

// PickupHandoffReference 非空说明结果已提交但意图还没交出去，重放会重发同一份
// （AT-TF-024：投递失败只重试原发布意图，不回退揽收）。
func (result PerformOffsitePickupResult) PickupHandoffReference() string {
	return result.handoff
}

// SegmentEntries 逐对象交回进段那一半的欠账，空表示没有欠账。**只列真有欠账的对象**，
// 进去了的不在列——与单对象入口「空串即无欠账」同义。
func (result PerformOffsitePickupResult) SegmentEntries() []ObjectSegmentEntry {
	return append([]ObjectSegmentEntry(nil), result.segments...)
}

// SegmentEntryRefusal 非空说明到访已登记、段那一半被领域正当拒绝（今天只有`段已关闭`一格）。
// **整次一格而不是逐对象**：段是整次到访共用的一个，它关了就对这次到访的每个成功对象都关了，
// 逐对象重复同一句话说不出更多东西——与逐对象的欠账（SegmentEntries）恰相反，那边每个对象可以
// 各自成败。
func (result PerformOffsitePickupResult) SegmentEntryRefusal() SegmentEntryRefusal {
	return result.segmentRefusal
}

type PerformOffsitePickupDeps struct {
	Attempts ports.PickupAttemptStore
	// Segments 可缺席：没有段登记册时到访照登不误。派生一侧缺席不该让来源保全停摆。
	Segments ports.ActualFulfillmentSegmentRegistry
	// Judgments 让段首登后同笔铸实际承运商判断的首版（票 tf-segment-lifecycle-closure/02）。可缺席，
	// 判据与形状同 RegisterOffsitePickupDeps.Judgments。
	Judgments  ports.ActualCarrierJudgmentRegistry
	Versions   ports.PickupIdentityFactory
	Downstream ports.OffsitePickupHandoff
	Clock      ports.Clock
}

type PerformOffsitePickupHandler struct {
	deps PerformOffsitePickupDeps
}

func NewPerformOffsitePickupHandler(deps PerformOffsitePickupDeps) *PerformOffsitePickupHandler {
	return &PerformOffsitePickupHandler{deps: deps}
}

// Handle 把一次实际到场推进到逐对象揽收判断：幂等/冲突按内容指纹分界 → 形成履约尝试
// → 逐对象分派（成功→场外揽收+意图；失败→只存尝试结果，不造段不交意图）→ 原子提交
// → 发布意图。意图投递失败不翻结果，重放重发同一份。
func (handler *PerformOffsitePickupHandler) Handle(
	ctx context.Context,
	command PerformOffsitePickupCommand,
) (PerformOffsitePickupResult, error) {
	if strings.TrimSpace(command.TenantID.String()) == "" ||
		strings.TrimSpace(command.SourceID) == "" ||
		len(command.Objects) == 0 {
		return PerformOffsitePickupResult{outcome: PickupNotAccepted}, nil
	}

	key := ports.PickupAttemptKey{TenantID: command.TenantID, SourceID: command.SourceID}
	digest := pickupContentDigest(command)
	existing, found, err := handler.deps.Attempts.FindByKey(ctx, key)
	if err != nil {
		return storeUndecided(command.SourceID), nil
	}
	if found {
		if existing.ContentDigest != digest {
			// 同一来源身份携带不同对象、时间或结果：冲突保留原结果，不用最后消息覆盖
			// （AT-TF-020）。
			return PerformOffsitePickupResult{outcome: PickupSourceConflict}, nil
		}
		// 同一尝试和内容重复回传：返回原结果，不重复建立控制或履约参与（AT-TF-019）。
		return handler.existingResult(ctx, existing), nil
	}

	attempt, err := formAttempt(command)
	if err != nil {
		// 尝试形状立不起来是提交自身的矛盾：改请求，不是等谁（ADR-0029 的分格）。
		return PerformOffsitePickupResult{outcome: PickupNotAccepted}, nil
	}

	record := ports.PickupAttemptRecord{
		Key:           key,
		ContentDigest: digest,
		Attempt:       attempt,
		RecordedAt:    handler.deps.Clock.Now(),
	}
	for _, submission := range command.Objects {
		result, err := domain.FormAttemptObjectResult(
			attempt, submission.Object, submission.Outcome, submission.Basis, submission.OccurredAt)
		if err != nil {
			return PerformOffsitePickupResult{outcome: PickupNotAccepted}, nil
		}
		record.Results = append(record.Results, result)

		if !submission.Outcome.Succeeded() {
			// 失败或拒收对象到此为止：不形成场外揽收，也没有可交给 parcel-shipment 的
			// 东西（步骤 6B）。带了控制依据的失败被拒——那是把失败到场伪装成取得控制。
			if submission.Control.String() != "" {
				return PerformOffsitePickupResult{outcome: PickupNotAccepted}, nil
			}
			continue
		}
		if submission.Control.String() == "" {
			// 成功却没有控制依据：证明不了运输方取得控制，是提交矛盾不是未决。
			return PerformOffsitePickupResult{outcome: PickupNotAccepted}, nil
		}
		pickup, undecided, err := handler.formPickup(ctx, command, submission)
		if err != nil {
			return PerformOffsitePickupResult{}, err
		}
		if undecided {
			return PerformOffsitePickupResult{outcome: PickupUndecided, reason: PickupIdentityUnavailable,
				continuation: pickupContinuation("PICKUP_IDENTITY_UNAVAILABLE", command.SourceID)}, nil
		}
		record.Pickups = append(record.Pickups, pickup)
	}

	return handler.commit(ctx, command, record)
}

func storeUndecided(sourceID string) PerformOffsitePickupResult {
	return PerformOffsitePickupResult{
		outcome:      PickupUndecided,
		reason:       PickupStoreUnavailable,
		continuation: pickupContinuation("PICKUP_STORE_UNAVAILABLE", sourceID),
	}
}

// formAttempt 把来源字符串折成领域尝试。立不起来的输入由领域构造器拒绝，编排不代答。
func formAttempt(command PerformOffsitePickupCommand) (domain.FulfillmentAttempt, error) {
	spec := domain.FulfillmentAttemptSpec{
		TenantID:    command.TenantID,
		PlannedFrom: command.PlannedFrom,
		PlannedTo:   command.PlannedTo,
		ArrivedAt:   command.ArrivedAt,
	}
	var err error
	if spec.Attempt, err = domain.NewAttemptReference(command.Attempt); err != nil {
		return domain.FulfillmentAttempt{}, err
	}
	if spec.Task, err = domain.NewDispatchTaskReference(command.Task); err != nil {
		return domain.FulfillmentAttempt{}, err
	}
	if spec.ExecutedBy, err = domain.NewExecutingPartyReference(command.ExecutedBy); err != nil {
		return domain.FulfillmentAttempt{}, err
	}
	if spec.Place, err = domain.NewAttemptPlaceReference(command.Place); err != nil {
		return domain.FulfillmentAttempt{}, err
	}
	if spec.Evidence, err = domain.NewAttemptEvidenceReference(command.Evidence); err != nil {
		return domain.FulfillmentAttempt{}, err
	}
	if strings.TrimSpace(command.RescheduledFrom) != "" {
		if spec.RescheduledFrom, err = domain.NewAttemptReference(command.RescheduledFrom); err != nil {
			return domain.FulfillmentAttempt{}, err
		}
	}
	for _, submission := range command.Objects {
		spec.Objects = append(spec.Objects, submission.Object)
	}
	return domain.FormFulfillmentAttempt(spec)
}

// formPickup 为一个取得控制的对象形成场外揽收。版本逐对象签发；版本厂答不上是依赖
// 未决（等恢复重试），其余入参此前都已过构造器——再失败是编排合同被打破，以错误上浮。
func (handler *PerformOffsitePickupHandler) formPickup(
	ctx context.Context,
	command PerformOffsitePickupCommand,
	submission ObjectPickupSubmission,
) (domain.OffsitePickup, bool, error) {
	version, err := handler.deps.Versions.NextPickupResultVersion(ctx)
	if err != nil {
		return domain.OffsitePickup{}, true, nil
	}
	task, err := domain.NewPickupTaskReference(command.Task)
	if err != nil {
		return domain.OffsitePickup{}, false, fmt.Errorf("pickup task reference: %w", err)
	}
	attempt, err := domain.NewAttemptReference(command.Attempt)
	if err != nil {
		return domain.OffsitePickup{}, false, fmt.Errorf("attempt reference: %w", err)
	}
	place, err := domain.NewPickupPlaceReference(command.Place)
	if err != nil {
		return domain.OffsitePickup{}, false, fmt.Errorf("pickup place reference: %w", err)
	}
	executedBy, err := domain.NewExecutingPartyReference(command.ExecutedBy)
	if err != nil {
		return domain.OffsitePickup{}, false, fmt.Errorf("executing party reference: %w", err)
	}
	pickup, err := domain.FormOffsitePickup(domain.OffsitePickupSpec{
		TenantID:   command.TenantID,
		Object:     submission.Object,
		Task:       task,
		Attempt:    attempt,
		Place:      place,
		Control:    submission.Control,
		ExecutedBy: executedBy,
		Version:    version,
		OccurredAt: submission.OccurredAt,
	})
	if err != nil {
		return domain.OffsitePickup{}, false, fmt.Errorf("form offsite pickup: %w", err)
	}
	return pickup, false, nil
}

// commit 提交记录并交发布意图；并发下另一方先提交时读回赢家。
func (handler *PerformOffsitePickupHandler) commit(
	ctx context.Context,
	command PerformOffsitePickupCommand,
	record ports.PickupAttemptRecord,
) (PerformOffsitePickupResult, error) {
	saved, err := handler.deps.Attempts.Save(ctx, record)
	if err != nil {
		return storeUndecided(record.Key.SourceID), nil
	}
	switch saved {
	case ports.PickupSaved:
		result := PerformOffsitePickupResult{outcome: PickupAttemptRecorded, record: record, hasRecord: true}
		result.handoff = handler.handOff(ctx, record)
		result.segments, result.segmentRefusal = handler.enterSegments(ctx, command, record)
		return result, nil
	case ports.PickupAlreadyRecorded:
		winner, found, err := handler.deps.Attempts.FindByKey(ctx, record.Key)
		if err != nil || !found {
			return storeUndecided(record.Key.SourceID), nil
		}
		return handler.existingResult(ctx, winner), nil
	default:
		return PerformOffsitePickupResult{}, fmt.Errorf("%w: %d", ErrUnexpectedPickupSave, saved)
	}
}

// enterSegments 让这次到访取得控制的对象逐个进入同一个实际履约段（CONTEXT 生命周期①②）。
//
// **失败对象走不到这里**：形不成 OffsitePickup 就不在 record.Pickups 里，CONTEXT「客户不在、
// 货物未备好、包装不合格或其他失败结果不制造实际履约段」由那道构造门守着，本编排不重判一遍。
//
// 第一个对象立段、其余加入，而这条编排**不记「段立了没有」**——那道门每次都问登记册，编排
// 自己记住就是一次竞态（理由在 enterFulfillmentSegment）。
//
// 一个对象没进去不影响后面的：登记册故障可以只落在某一次加入上，逐个走完再逐个报，正是
// 本票与两条单对象入口的全部差别。
func (handler *PerformOffsitePickupHandler) enterSegments(
	ctx context.Context,
	command PerformOffsitePickupCommand,
	record ports.PickupAttemptRecord,
) ([]ObjectSegmentEntry, SegmentEntryRefusal) {
	planned := make(map[domain.CarriedObjectReference]string, len(command.Objects))
	for _, submission := range command.Objects {
		planned[submission.Object] = submission.PlannedSegment
	}

	var entries []ObjectSegmentEntry
	refusal := SegmentEntryRefusalNone
	for _, pickup := range record.Pickups {
		entry := enterFulfillmentSegment(
			ctx, handler.deps.Segments, handler.deps.Judgments, handler.deps.Clock,
			command.TenantID, command.Segment, planned[pickup.Object()],
			segmentEntryDoors{
				object: pickup.Object(),
				establish: func(
					segment domain.FulfillmentSegmentReference,
					plannedSegment domain.PlannedSegmentReference,
				) (domain.ActualFulfillmentSegment, error) {
					return domain.EstablishSegmentWithPickup(segment, pickup, plannedSegment)
				},
				join: func(
					existing domain.ActualFulfillmentSegment,
					plannedSegment domain.PlannedSegmentReference,
				) (domain.ActualFulfillmentSegment, error) {
					return existing.JoinWithPickup(pickup, plannedSegment)
				},
			},
		)
		if entry.continuation != "" {
			entries = append(entries, ObjectSegmentEntry{
				Object:                pickup.Object(),
				ContinuationReference: entry.continuation,
			})
		}
		if entry.refusal != SegmentEntryRefusalNone {
			refusal = entry.refusal
		}
	}
	return entries, refusal
}

// existingResult 按已有记录作答并重发同一份意图（AT-TF-019/024）。
func (handler *PerformOffsitePickupHandler) existingResult(
	ctx context.Context,
	record ports.PickupAttemptRecord,
) PerformOffsitePickupResult {
	return PerformOffsitePickupResult{
		outcome:   PickupExistingResult,
		record:    record,
		hasRecord: true,
		handoff:   handler.handOff(ctx, record),
	}
}

// handOff 交发布意图。只有形成了场外揽收的记录才交——全失败的尝试没有 parcel-shipment
// 能采用的东西；投递失败不翻结果，留续办引用重放时重发同一份（步骤 8）。
func (handler *PerformOffsitePickupHandler) handOff(
	ctx context.Context,
	record ports.PickupAttemptRecord,
) string {
	if len(record.Pickups) == 0 {
		return ""
	}
	if err := handler.deps.Downstream.HandOffOffsitePickup(ctx, ports.OffsitePickupHandoffIntent{Record: record}); err == nil {
		return ""
	}
	return pickupContinuation("OFFSITE_PICKUP_HANDOFF", record.Key.TenantID.String(), record.Key.SourceID)
}

func pickupContinuation(parts ...string) string {
	digest := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return "CONT-" + hex.EncodeToString(digest[:8])
}

// pickupContentDigest 是同一来源身份的内容比对锚：尝试身份、任务、到场时间与逐对象
// 结果（对象、走向、依据、控制、时间）任一不同即是另一份内容。对象行先排序——提交
// 顺序不构成不同的内容。
func pickupContentDigest(command PerformOffsitePickupCommand) string {
	lines := make([]string, 0, len(command.Objects))
	for _, submission := range command.Objects {
		lines = append(lines, strings.Join([]string{
			submission.Object.String(),
			fmt.Sprintf("%d", submission.Outcome),
			submission.Basis.String(),
			submission.Control.String(),
			submission.OccurredAt.UTC().Format(time.RFC3339Nano),
		}, "\x1f"))
	}
	sort.Strings(lines)
	digest := sha256.Sum256([]byte(strings.Join(append([]string{
		command.Attempt,
		command.Task,
		command.ArrivedAt.UTC().Format(time.RFC3339Nano),
	}, lines...), "\x00")))
	return hex.EncodeToString(digest[:])
}
