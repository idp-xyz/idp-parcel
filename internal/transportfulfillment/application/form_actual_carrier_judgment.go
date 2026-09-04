package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// CarrierJudgmentOutcome 是「形成实际承运商判断」的结果代数（票 tf-segment-lifecycle-closure/02）。
//
// 各格按恢复动作分（ADR-0029），不按看到的原因分：`已考虑`重放即可；`早于段成立`去看这条证据该归哪一段；
// `段不在册`去查段成没成立；`已封存`说明段已结束、这条证据只能走来源事实更正那条路；`未受理`改输入；
// `未决`等依赖恢复后重试同一份。
type CarrierJudgmentOutcome uint8

const (
	CarrierJudgmentOutcomeInvalid CarrierJudgmentOutcome = iota
	CarrierJudgmentVersionFormed
	CarrierEvidenceAlreadyConsidered
	CarrierEvidencePrecedesSegment
	CarrierJudgmentSegmentNotFound
	CarrierJudgmentSealed
	CarrierJudgmentNotAccepted
	CarrierJudgmentUndecided
)

func (outcome CarrierJudgmentOutcome) String() string {
	switch outcome {
	case CarrierJudgmentVersionFormed:
		return "VERSION_FORMED"
	case CarrierEvidenceAlreadyConsidered:
		return "EVIDENCE_ALREADY_CONSIDERED"
	case CarrierEvidencePrecedesSegment:
		return "EVIDENCE_PRECEDES_SEGMENT"
	case CarrierJudgmentSegmentNotFound:
		return "SEGMENT_NOT_FOUND"
	case CarrierJudgmentSealed:
		return "JUDGMENT_SEALED"
	case CarrierJudgmentNotAccepted:
		return "INPUT_NOT_ACCEPTED"
	case CarrierJudgmentUndecided:
		return "JUDGMENT_UNDECIDED"
	default:
		return ""
	}
}

// CarrierJudgmentUndecidedReason 指名`未决`停在哪一步等谁。
//
// `版本撞序号`单开一格：两条编排各基于同一个当前版算出同一个下一序号，后写的那一方要**读回再来**——与
// 「登记册读不到」不同，它的依赖此刻是好的，重试立刻就能过。
type CarrierJudgmentUndecidedReason uint8

const (
	CarrierJudgmentUndecidedReasonNone CarrierJudgmentUndecidedReason = iota
	CarrierJudgmentRegistryUnavailable
	CarrierIdentityDirectoryUnavailable
	CarrierJudgmentVersionRaced
)

func (reason CarrierJudgmentUndecidedReason) String() string {
	switch reason {
	case CarrierJudgmentRegistryUnavailable:
		return "JUDGMENT_REGISTRY_UNAVAILABLE"
	case CarrierIdentityDirectoryUnavailable:
		return "IDENTITY_DIRECTORY_UNAVAILABLE"
	case CarrierJudgmentVersionRaced:
		return "JUDGMENT_VERSION_RACED"
	default:
		return ""
	}
}

// FormActualCarrierJudgmentCommand 携带一条合格证据与它指名的承运主体。
//
// **承运主体给引用或给名称素材，恰给其一。** 给引用（分支 + 引用）时本编排去 party-commercial 问它登了
// 没有；只给名称素材时不问——名称不是身份，本上下文也不按名称去册上找（CONTEXT：不据名称铸身份）。
// 分支由证据决定而不由本编排推断（ADR-0103 决定三）：证据是自营执行方的作业事实则给运营法人分支，是外部
// 承运方的收寄、凭证、交接或回传则给外部参与方分支。
type FormActualCarrierJudgmentCommand struct {
	TenantID          domain.TenantID
	Segment           string
	Source            domain.CarrierEvidenceSource
	EvidenceReference string
	// OccurredAt 是来源事实的业务时间（源的），不是提交时刻。
	OccurredAt       time.Time
	SubjectKind      domain.CarrierSubjectKind
	SubjectReference string
	NameMaterial     string
}

type FormActualCarrierJudgmentResult struct {
	outcome      CarrierJudgmentOutcome
	reason       CarrierJudgmentUndecidedReason
	continuation string
	record       ports.ActualCarrierJudgmentRecord
	hasRecord    bool
}

func (result FormActualCarrierJudgmentResult) Outcome() CarrierJudgmentOutcome { return result.outcome }

// UndecidedReason 只在`未决`时非零。
func (result FormActualCarrierJudgmentResult) UndecidedReason() CarrierJudgmentUndecidedReason {
	return result.reason
}

// ContinuationReference 只在`未决`时非空。
func (result FormActualCarrierJudgmentResult) ContinuationReference() string {
	return result.continuation
}

// Record 在形成了新版本或答`已考虑`时带回判断的当前记录。
func (result FormActualCarrierJudgmentResult) Record() (ports.ActualCarrierJudgmentRecord, bool) {
	return result.record, result.hasRecord
}

type FormActualCarrierJudgmentDeps struct {
	// Segments 答两件：段成没成立、段关没关——封存不是判断上存的一格，是段的关闭状态派生出来的。
	Segments  ports.ActualFulfillmentSegmentRegistry
	Judgments ports.ActualCarrierJudgmentRegistry
	// Identities 可缺席；缺席时凡给了引用的命令一律`未决`——装配缺件不是「未登记」这个业务答案。
	Identities ports.CarrierIdentityDirectory
	Clock      ports.Clock
}

type FormActualCarrierJudgmentHandler struct {
	deps FormActualCarrierJudgmentDeps
}

func NewFormActualCarrierJudgmentHandler(deps FormActualCarrierJudgmentDeps) *FormActualCarrierJudgmentHandler {
	return &FormActualCarrierJudgmentHandler{deps: deps}
}

// Form 收一条合格证据并形成实际承运商判断的新版本（CONTEXT「实际承运商判断」规则与生命周期节）：
// 受理 → 段在册且未关闭 → 查承运主体身份 → 读回判断（不在册则补开首版）→ 领域转换门 → 只追加那一版。
//
// **同一份证据的两种再来。** 引用已在依据里而身份状态未变，是重放，答`已考虑`不追加；引用已在依据里、
// 当时只有名称素材而此刻身份已在册，走 RecogniseCarrierIdentity——那正是「PC 登记之后凭同一份证据形成
// 新版本」，业务时间不动、形成时间是现在，未登记期间原样留在上一版里。
//
// **封存先于一切读写。** 段已关闭的判断不再收新证据（CONTEXT 生命周期末条），连身份也不去问：问了也不用，
// 而一次不该发生的查询日后会被当成「这条证据确实核过身份」的证据。
func (handler *FormActualCarrierJudgmentHandler) Form(
	ctx context.Context,
	command FormActualCarrierJudgmentCommand,
) (FormActualCarrierJudgmentResult, error) {
	segmentKey, evidenceReference, accepted := carrierJudgmentTargetFrom(command)
	if !accepted {
		return FormActualCarrierJudgmentResult{outcome: CarrierJudgmentNotAccepted}, nil
	}

	segment, found, err := handler.deps.Segments.FindByKey(ctx, segmentKey)
	if err != nil {
		return carrierJudgmentUndecided(command, CarrierJudgmentRegistryUnavailable), nil
	}
	if !found {
		return FormActualCarrierJudgmentResult{outcome: CarrierJudgmentSegmentNotFound}, nil
	}
	if segment.Segment.Closed() {
		return FormActualCarrierJudgmentResult{outcome: CarrierJudgmentSealed}, nil
	}

	subject, material, outcome := handler.resolveSubject(ctx, command)
	if outcome != CarrierJudgmentOutcomeInvalid {
		return handler.undecidedOrRefused(command, outcome), nil
	}
	evidence, err := domain.NewCarrierEvidence(domain.CarrierEvidenceSpec{
		Source:     command.Source,
		Reference:  evidenceReference,
		OccurredAt: command.OccurredAt,
		Subject:    subject,
		Material:   material,
	})
	if err != nil {
		return FormActualCarrierJudgmentResult{outcome: CarrierJudgmentNotAccepted}, nil
	}

	key := ports.ActualCarrierJudgmentKey{TenantID: segmentKey.TenantID, Segment: segmentKey.Segment}
	record, outcome := handler.loadOrOpenJudgment(ctx, key, segment.Segment)
	if outcome != CarrierJudgmentOutcomeInvalid {
		return carrierJudgmentUndecided(command, CarrierJudgmentRegistryUnavailable), nil
	}

	next, outcome := handler.transition(record.Judgment, evidence)
	switch outcome {
	case CarrierEvidenceAlreadyConsidered:
		return FormActualCarrierJudgmentResult{outcome: outcome, record: record, hasRecord: true}, nil
	case CarrierJudgmentOutcomeInvalid:
	default:
		return FormActualCarrierJudgmentResult{outcome: outcome}, nil
	}

	now := handler.deps.Clock.Now()
	appended, err := handler.deps.Judgments.AppendVersion(ctx, key, next.Current(), now)
	if err != nil {
		return carrierJudgmentUndecided(command, CarrierJudgmentRegistryUnavailable), nil
	}
	switch appended {
	case ports.JudgmentVersionAppended:
		return FormActualCarrierJudgmentResult{
			outcome:   CarrierJudgmentVersionFormed,
			record:    ports.ActualCarrierJudgmentRecord{Key: key, Judgment: next, RecordedAt: now},
			hasRecord: true,
		}, nil
	case ports.JudgmentVersionAlreadyRecorded:
		// 另一次追加先落了同一序号：本次算出的版本建在一个已经过时的当前版上，读回再来。
		return carrierJudgmentUndecided(command, CarrierJudgmentVersionRaced), nil
	default:
		return carrierJudgmentUndecided(command, CarrierJudgmentRegistryUnavailable), nil
	}
}

// resolveSubject 按命令给的引用或素材定出这条依据指名的承运主体。第三个返回值非零表示这一步已经有了答案。
//
// 引用查得到 → 身份引用；查不到 → 引用作素材保留（命令另给了名称就用名称）——不据名称铸身份，也不把
// 未登记的引用直接当身份。读口读不通或未装 → `未决`，不折成未登记（ADR-0029）。
func (handler *FormActualCarrierJudgmentHandler) resolveSubject(
	ctx context.Context,
	command FormActualCarrierJudgmentCommand,
) (domain.CarrierSubject, string, CarrierJudgmentOutcome) {
	material := strings.TrimSpace(command.NameMaterial)
	if strings.TrimSpace(command.SubjectReference) == "" {
		if material == "" {
			return domain.CarrierSubject{}, "", CarrierJudgmentNotAccepted
		}
		return domain.CarrierSubject{}, material, CarrierJudgmentOutcomeInvalid
	}
	claimed, err := domain.NewCarrierSubject(command.SubjectKind, command.SubjectReference)
	if err != nil {
		return domain.CarrierSubject{}, "", CarrierJudgmentNotAccepted
	}
	if handler.deps.Identities == nil {
		return domain.CarrierSubject{}, "", CarrierJudgmentUndecided
	}
	registered, err := handler.deps.Identities.IdentityRegistered(ctx, command.TenantID, claimed)
	if err != nil {
		return domain.CarrierSubject{}, "", CarrierJudgmentUndecided
	}
	if registered {
		return claimed, "", CarrierJudgmentOutcomeInvalid
	}
	if material == "" {
		material = claimed.Reference()
	}
	return domain.CarrierSubject{}, material, CarrierJudgmentOutcomeInvalid
}

// undecidedOrRefused 把 resolveSubject 的两种提前答案各归各格：`未决`带身份读口的理由与续办引用，`未受理`不带。
func (handler *FormActualCarrierJudgmentHandler) undecidedOrRefused(
	command FormActualCarrierJudgmentCommand,
	outcome CarrierJudgmentOutcome,
) FormActualCarrierJudgmentResult {
	if outcome == CarrierJudgmentUndecided {
		return carrierJudgmentUndecided(command, CarrierIdentityDirectoryUnavailable)
	}
	return FormActualCarrierJudgmentResult{outcome: outcome}
}

// loadOrOpenJudgment 读回判断；段在册而判断不在册时补开首版。
//
// 补开不是掩盖挂点的欠账——那一笔的续办引用已经报出去了；这里只是不让一条合格证据因为首版还没落而等在
// 门外。补开的首版形成时间如实是现在，业务时间是段成立时刻：知识史与事实史各归各的。
func (handler *FormActualCarrierJudgmentHandler) loadOrOpenJudgment(
	ctx context.Context,
	key ports.ActualCarrierJudgmentKey,
	segment domain.ActualFulfillmentSegment,
) (ports.ActualCarrierJudgmentRecord, CarrierJudgmentOutcome) {
	record, found, err := handler.deps.Judgments.FindByKey(ctx, key)
	if err != nil {
		return ports.ActualCarrierJudgmentRecord{}, CarrierJudgmentUndecided
	}
	if found {
		return record, CarrierJudgmentOutcomeInvalid
	}
	now := handler.deps.Clock.Now()
	opened, err := domain.OpenActualCarrierJudgment(domain.OpenActualCarrierJudgmentSpec{
		TenantID:      key.TenantID,
		Segment:       key.Segment,
		EstablishedAt: segmentEstablishedAt(segment),
		FormedAt:      now,
	})
	if err != nil {
		return ports.ActualCarrierJudgmentRecord{}, CarrierJudgmentUndecided
	}
	record = ports.ActualCarrierJudgmentRecord{Key: key, Judgment: opened, RecordedAt: now}
	outcome, err := handler.deps.Judgments.Open(ctx, record)
	if err != nil {
		return ports.ActualCarrierJudgmentRecord{}, CarrierJudgmentUndecided
	}
	switch outcome {
	case ports.JudgmentOpened:
		return record, CarrierJudgmentOutcomeInvalid
	case ports.JudgmentAlreadyOpened:
		// 并发下另一方先开了：读回赢家。
		winner, found, err := handler.deps.Judgments.FindByKey(ctx, key)
		if err != nil || !found {
			return ports.ActualCarrierJudgmentRecord{}, CarrierJudgmentUndecided
		}
		return winner, CarrierJudgmentOutcomeInvalid
	default:
		return ports.ActualCarrierJudgmentRecord{}, CarrierJudgmentUndecided
	}
}

// transition 走领域的转换门。第二个返回值非零表示这一步已经有了答案（`已考虑` / `早于段成立` / `未受理`）。
func (handler *FormActualCarrierJudgmentHandler) transition(
	judgment domain.ActualCarrierJudgment,
	evidence domain.CarrierEvidence,
) (domain.ActualCarrierJudgment, CarrierJudgmentOutcome) {
	now := handler.deps.Clock.Now()
	for _, basis := range judgment.Current().Bases() {
		if basis.Reference() != evidence.Reference() {
			continue
		}
		_, wasRegistered := basis.Subject()
		subject, nowRegistered := evidence.Subject()
		if wasRegistered || !nowRegistered {
			return judgment, CarrierEvidenceAlreadyConsidered
		}
		recognised, err := judgment.RecogniseCarrierIdentity(evidence.Reference(), subject, now)
		if err != nil {
			return domain.ActualCarrierJudgment{}, CarrierJudgmentNotAccepted
		}
		return recognised, CarrierJudgmentOutcomeInvalid
	}
	considered, err := judgment.Consider(evidence, now)
	switch {
	case errors.Is(err, domain.ErrCarrierEvidencePrecedesSegment):
		return domain.ActualCarrierJudgment{}, CarrierEvidencePrecedesSegment
	case errors.Is(err, domain.ErrCarrierEvidenceAlreadyConsidered):
		return judgment, CarrierEvidenceAlreadyConsidered
	case err != nil:
		return domain.ActualCarrierJudgment{}, CarrierJudgmentNotAccepted
	}
	return considered, CarrierJudgmentOutcomeInvalid
}

func carrierJudgmentTargetFrom(
	command FormActualCarrierJudgmentCommand,
) (ports.FulfillmentSegmentKey, domain.CarrierEvidenceReference, bool) {
	if strings.TrimSpace(command.TenantID.String()) == "" || command.OccurredAt.IsZero() {
		return ports.FulfillmentSegmentKey{}, domain.CarrierEvidenceReference{}, false
	}
	segment, err := domain.NewFulfillmentSegmentReference(command.Segment)
	if err != nil {
		return ports.FulfillmentSegmentKey{}, domain.CarrierEvidenceReference{}, false
	}
	reference, err := domain.NewCarrierEvidenceReference(command.EvidenceReference)
	if err != nil {
		return ports.FulfillmentSegmentKey{}, domain.CarrierEvidenceReference{}, false
	}
	// 来源必须在封闭四格内；集外的一律不受理，不让它以「未知来源」之名混进依据（ADR-0103 决定四）。
	if _, err := domain.ParseCarrierEvidenceSource(command.Source.String()); err != nil {
		return ports.FulfillmentSegmentKey{}, domain.CarrierEvidenceReference{}, false
	}
	return ports.FulfillmentSegmentKey{TenantID: command.TenantID, Segment: segment}, reference, true
}

func carrierJudgmentUndecided(
	command FormActualCarrierJudgmentCommand,
	reason CarrierJudgmentUndecidedReason,
) FormActualCarrierJudgmentResult {
	digest := sha256.Sum256([]byte(strings.Join([]string{
		reason.String(),
		command.TenantID.String(),
		command.Segment,
		command.EvidenceReference,
	}, "\x00")))
	return FormActualCarrierJudgmentResult{
		outcome:      CarrierJudgmentUndecided,
		reason:       reason,
		continuation: "CONT-" + hex.EncodeToString(digest[:8]),
	}
}
