package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// ErrUnexpectedHandoverSave 说明交接登记库交回了封闭集合以外的写入结果。
var ErrUnexpectedHandoverSave = errors.New("transport fulfillment: unexpected transport handover save outcome")

// HandoverRegistrationOutcome 是交接判断登记的应用处理结果。三裁决（已交接/拒收/
// 待确认）都是可登记的判断——完备性由领域把门，编排只分幂等与冲突。
type HandoverRegistrationOutcome uint8

const (
	HandoverRegistrationOutcomeInvalid HandoverRegistrationOutcome = iota
	HandoverRegistered
	HandoverExistingVersion
	HandoverRegistrationConflict
	HandoverCorrected
	HandoverNotAccepted
	HandoverUndecided
)

func (outcome HandoverRegistrationOutcome) String() string {
	switch outcome {
	case HandoverRegistered:
		return "HANDOVER_REGISTERED"
	case HandoverExistingVersion:
		return "EXISTING_VERSION"
	case HandoverRegistrationConflict:
		return "SOURCE_CONFLICT"
	case HandoverCorrected:
		return "HANDOVER_CORRECTED"
	case HandoverNotAccepted:
		return "SOURCE_NOT_ACCEPTED"
	case HandoverUndecided:
		return "HANDOVER_UNDECIDED"
	default:
		return ""
	}
}

// HandoverRegistrationUndecidedReason 指名提交停在哪一步等谁。
type HandoverRegistrationUndecidedReason uint8

const (
	HandoverRegistrationUndecidedReasonNone HandoverRegistrationUndecidedReason = iota
	HandoverRegistryUnavailable
)

func (reason HandoverRegistrationUndecidedReason) String() string {
	switch reason {
	case HandoverRegistryUnavailable:
		return "HANDOVER_REGISTRY_UNAVAILABLE"
	default:
		return ""
	}
}

// RegisterTransportHandoverCommand 携带一次交接判断登记的全部输入。版本由裁决过程
// 指名——它是判断的身份，不是登记时铸的号。
type RegisterTransportHandoverCommand struct {
	TenantID          domain.TenantID
	Object            string
	Scope             string
	ReleasedBy        string
	ReceivedBy        string
	Verdict           domain.HandoverVerdict
	ReleasingEvidence string
	ReceivingEvidence string
	Rule              string
	Basis             string
	Version           string
	JudgedAt          time.Time
	// Segment 指名这次交接把对象送进哪个实际履约段。**缺席时不立段**——实际履约段不等同于
	// 交接范围、计划段、班次或订舱（CONTEXT），段身份由谁铸出至今没有裁决，这里不拿手边
	// 任一引用顶替。缺席不是失败：交接登记是控制事实的保全，它自己成立。
	Segment string
	// PlannedSegment 是该对象自己关联的计划履约段，可缺席（待路由产品此刻还没有计划段）。
	PlannedSegment string
}

// CorrectTransportHandoverCommand 携带更正入口的全部输入：指名被更正的前版，更正走
// 领域版本链（新版回指前身、原判断不动、每格完备性同首次裁决）。
type CorrectTransportHandoverCommand struct {
	TenantID           domain.TenantID
	Object             string
	Scope              string
	PredecessorVersion string
	Verdict            domain.HandoverVerdict
	ReleasingEvidence  string
	ReceivingEvidence  string
	Rule               string
	Basis              string
	NewVersion         string
	CorrectedAt        time.Time
}

type RegisterTransportHandoverResult struct {
	outcome      HandoverRegistrationOutcome
	reason       HandoverRegistrationUndecidedReason
	record       ports.TransportHandoverRecord
	hasRecord    bool
	continuation string
	handoff      string
}

func (result RegisterTransportHandoverResult) Outcome() HandoverRegistrationOutcome {
	return result.outcome
}

// UndecidedReason 只在`未决`时非零。
func (result RegisterTransportHandoverResult) UndecidedReason() HandoverRegistrationUndecidedReason {
	return result.reason
}

func (result RegisterTransportHandoverResult) Record() (ports.TransportHandoverRecord, bool) {
	return result.record, result.hasRecord
}

func (result RegisterTransportHandoverResult) ContinuationReference() string {
	return result.continuation
}

// HandoverHandoffReference 非空说明记录已提交但意图还没交出去，重放会重发同一份。
func (result RegisterTransportHandoverResult) HandoverHandoffReference() string {
	return result.handoff
}

type RegisterTransportHandoverDeps struct {
	Handovers ports.TransportHandoverRegistry
	// Segments 可缺席：没有段登记册时交接照登不误。派生一侧缺席不该让来源保全停摆。
	Segments   ports.ActualFulfillmentSegmentRegistry
	Downstream ports.TransportHandoverRegistrationHandoff
	Clock      ports.Clock
}

type RegisterTransportHandoverHandler struct {
	deps RegisterTransportHandoverDeps
}

func NewRegisterTransportHandoverHandler(deps RegisterTransportHandoverDeps) *RegisterTransportHandoverHandler {
	return &RegisterTransportHandoverHandler{deps: deps}
}

// Register 首登一个交接判断版本：受理（三裁决各格完备性由 FormTransportHandover
// 把门）→ 幂等按（租户+对象+范围+版本）分重放/冲突 → 原子提交 → 一份意图交下游
// （NO 控制转移与 NR 重判自分）。
func (handler *RegisterTransportHandoverHandler) Register(
	ctx context.Context,
	command RegisterTransportHandoverCommand,
) (RegisterTransportHandoverResult, error) {
	handover, err := handoverFrom(command)
	if err != nil {
		return RegisterTransportHandoverResult{outcome: HandoverNotAccepted}, nil
	}

	key := ports.TransportHandoverKey{
		TenantID: command.TenantID,
		Object:   handover.Object(),
		Scope:    handover.Scope(),
		Version:  handover.Version(),
	}
	digest := handoverDigest(command)
	existing, found, err := handler.deps.Handovers.FindByKey(ctx, key)
	if err != nil {
		return handoverRegistryUndecided(command.Object), nil
	}
	if found {
		if existing.ContentDigest != digest {
			// 同一判断版本携带不同裁决或证据：冲突保留原判断——改判走更正入口换新版，
			// 不按最后到达顶替。
			return RegisterTransportHandoverResult{outcome: HandoverRegistrationConflict}, nil
		}
		return handler.existingResult(ctx, existing), nil
	}

	result, err := handler.commit(ctx, ports.TransportHandoverRecord{
		Key:           key,
		ContentDigest: digest,
		Handover:      handover,
		RecordedAt:    handler.deps.Clock.Now(),
	}, HandoverRegistered)
	if err != nil || result.outcome != HandoverRegistered {
		return result, err
	}
	handler.establishSegment(ctx, command, handover)
	return result, nil
}

// establishSegment 让这次交接的对象进入实际履约段（CONTEXT 生命周期①）。
//
// 立段是派生的一侧，**它的失败不得回滚交接登记**：接货时间是责任起点锚，一次段登记故障
// 抹不掉一条已经发生的物理事实。三裁决里只有`已交接`转出控制，拒收与待确认立不起段也不
// 报错——那是正当的业务结果，由领域的 TransferOutBasis 把门，这里不重判一遍。
func (handler *RegisterTransportHandoverHandler) establishSegment(
	ctx context.Context,
	command RegisterTransportHandoverCommand,
	handover domain.TransportHandover,
) {
	if handler.deps.Segments == nil || strings.TrimSpace(command.Segment) == "" {
		return
	}
	segment, err := domain.NewFulfillmentSegmentReference(command.Segment)
	if err != nil {
		return
	}
	var planned domain.PlannedSegmentReference
	if strings.TrimSpace(command.PlannedSegment) != "" {
		if planned, err = domain.NewPlannedSegmentReference(command.PlannedSegment); err != nil {
			return
		}
	}
	established, err := domain.EstablishSegmentWithHandover(segment, handover, planned)
	if err != nil {
		return
	}
	_, _ = handler.deps.Segments.Save(ctx, ports.FulfillmentSegmentRecord{
		Key:        ports.FulfillmentSegmentKey{TenantID: command.TenantID, Segment: segment},
		Segment:    established,
		RecordedAt: handler.deps.Clock.Now(),
	})
}

// Correct 对已登记的判断落更正版本：读回前版 → 领域 Correct（新版回指前身、完备性
// 同首次裁决、同版本号拒）→ 以新版本键登记 → 意图重新交付下游。
func (handler *RegisterTransportHandoverHandler) Correct(
	ctx context.Context,
	command CorrectTransportHandoverCommand,
) (RegisterTransportHandoverResult, error) {
	predecessorKey, err := correctionKey(command)
	if err != nil {
		return RegisterTransportHandoverResult{outcome: HandoverNotAccepted}, nil
	}
	existing, found, err := handler.deps.Handovers.FindByKey(ctx, predecessorKey)
	if err != nil {
		return handoverRegistryUndecided(command.Object), nil
	}
	if !found {
		// 没有可更正的判断：更正不出无中生有的交接。
		return RegisterTransportHandoverResult{outcome: HandoverNotAccepted}, nil
	}

	correction, err := correctionFrom(command)
	if err != nil {
		return RegisterTransportHandoverResult{outcome: HandoverNotAccepted}, nil
	}
	corrected, err := existing.Handover.Correct(correction)
	if err != nil {
		return RegisterTransportHandoverResult{outcome: HandoverNotAccepted}, nil
	}

	return handler.commit(ctx, ports.TransportHandoverRecord{
		Key: ports.TransportHandoverKey{
			TenantID: command.TenantID,
			Object:   corrected.Object(),
			Scope:    corrected.Scope(),
			Version:  corrected.Version(),
		},
		ContentDigest: correctionDigest(command),
		Handover:      corrected,
		RecordedAt:    handler.deps.Clock.Now(),
	}, HandoverCorrected)
}

// commit 提交记录并交发布意图；并发下另一方先提交时读回赢家。
func (handler *RegisterTransportHandoverHandler) commit(
	ctx context.Context,
	record ports.TransportHandoverRecord,
	outcome HandoverRegistrationOutcome,
) (RegisterTransportHandoverResult, error) {
	saved, err := handler.deps.Handovers.Save(ctx, record)
	if err != nil {
		return handoverRegistryUndecided(record.Key.Object.String()), nil
	}
	switch saved {
	case ports.HandoverSaved:
		result := RegisterTransportHandoverResult{outcome: outcome, record: record, hasRecord: true}
		result.handoff = handler.handOff(ctx, record)
		return result, nil
	case ports.HandoverAlreadyRegistered:
		winner, found, err := handler.deps.Handovers.FindByKey(ctx, record.Key)
		if err != nil || !found {
			return handoverRegistryUndecided(record.Key.Object.String()), nil
		}
		return handler.existingResult(ctx, winner), nil
	default:
		return RegisterTransportHandoverResult{}, fmt.Errorf("%w: %d", ErrUnexpectedHandoverSave, saved)
	}
}

func handoverFrom(command RegisterTransportHandoverCommand) (domain.TransportHandover, error) {
	spec := domain.TransportHandoverSpec{
		TenantID: command.TenantID,
		Verdict:  command.Verdict,
		JudgedAt: command.JudgedAt,
	}
	var err error
	if spec.Object, err = domain.NewCarriedObjectReference(command.Object); err != nil {
		return domain.TransportHandover{}, err
	}
	if spec.Scope, err = domain.NewHandoverScopeReference(command.Scope); err != nil {
		return domain.TransportHandover{}, err
	}
	if spec.ReleasedBy, err = domain.NewHandoverPartyReference(command.ReleasedBy); err != nil {
		return domain.TransportHandover{}, err
	}
	if spec.ReceivedBy, err = domain.NewHandoverPartyReference(command.ReceivedBy); err != nil {
		return domain.TransportHandover{}, err
	}
	if spec.Version, err = domain.NewHandoverResultVersion(command.Version); err != nil {
		return domain.TransportHandover{}, err
	}
	if strings.TrimSpace(command.ReleasingEvidence) != "" {
		if spec.ReleasingEvidence, err = domain.NewHandoverEvidenceReference(command.ReleasingEvidence); err != nil {
			return domain.TransportHandover{}, err
		}
	}
	if strings.TrimSpace(command.ReceivingEvidence) != "" {
		if spec.ReceivingEvidence, err = domain.NewHandoverEvidenceReference(command.ReceivingEvidence); err != nil {
			return domain.TransportHandover{}, err
		}
	}
	if strings.TrimSpace(command.Rule) != "" {
		if spec.Rule, err = domain.NewHandoverRuleReference(command.Rule); err != nil {
			return domain.TransportHandover{}, err
		}
	}
	if strings.TrimSpace(command.Basis) != "" {
		if spec.Basis, err = domain.NewHandoverBasisReference(command.Basis); err != nil {
			return domain.TransportHandover{}, err
		}
	}
	return domain.FormTransportHandover(spec)
}

func correctionKey(command CorrectTransportHandoverCommand) (ports.TransportHandoverKey, error) {
	if strings.TrimSpace(command.TenantID.String()) == "" {
		return ports.TransportHandoverKey{}, errors.New("blank tenant")
	}
	object, err := domain.NewCarriedObjectReference(command.Object)
	if err != nil {
		return ports.TransportHandoverKey{}, err
	}
	scope, err := domain.NewHandoverScopeReference(command.Scope)
	if err != nil {
		return ports.TransportHandoverKey{}, err
	}
	version, err := domain.NewHandoverResultVersion(command.PredecessorVersion)
	if err != nil {
		return ports.TransportHandoverKey{}, err
	}
	return ports.TransportHandoverKey{TenantID: command.TenantID, Object: object, Scope: scope, Version: version}, nil
}

func correctionFrom(command CorrectTransportHandoverCommand) (domain.HandoverCorrection, error) {
	correction := domain.HandoverCorrection{
		Verdict:     command.Verdict,
		CorrectedAt: command.CorrectedAt,
	}
	var err error
	if correction.Version, err = domain.NewHandoverResultVersion(command.NewVersion); err != nil {
		return domain.HandoverCorrection{}, err
	}
	if strings.TrimSpace(command.ReleasingEvidence) != "" {
		if correction.ReleasingEvidence, err = domain.NewHandoverEvidenceReference(command.ReleasingEvidence); err != nil {
			return domain.HandoverCorrection{}, err
		}
	}
	if strings.TrimSpace(command.ReceivingEvidence) != "" {
		if correction.ReceivingEvidence, err = domain.NewHandoverEvidenceReference(command.ReceivingEvidence); err != nil {
			return domain.HandoverCorrection{}, err
		}
	}
	if strings.TrimSpace(command.Rule) != "" {
		if correction.Rule, err = domain.NewHandoverRuleReference(command.Rule); err != nil {
			return domain.HandoverCorrection{}, err
		}
	}
	if strings.TrimSpace(command.Basis) != "" {
		if correction.Basis, err = domain.NewHandoverBasisReference(command.Basis); err != nil {
			return domain.HandoverCorrection{}, err
		}
	}
	return correction, nil
}

func handoverRegistryUndecided(object string) RegisterTransportHandoverResult {
	return RegisterTransportHandoverResult{
		outcome:      HandoverUndecided,
		reason:       HandoverRegistryUnavailable,
		continuation: handoverRegistrationContinuation("HANDOVER_REGISTRY_UNAVAILABLE", object),
	}
}

// existingResult 按已有登记作答并重发同一份意图。
func (handler *RegisterTransportHandoverHandler) existingResult(
	ctx context.Context,
	record ports.TransportHandoverRecord,
) RegisterTransportHandoverResult {
	return RegisterTransportHandoverResult{
		outcome:   HandoverExistingVersion,
		record:    record,
		hasRecord: true,
		handoff:   handler.handOff(ctx, record),
	}
}

// handOff 交一份意图，消费方自分（NO 控制转移只认已交接的 TransferOutBasis，NR 以
// 交接证据触发重判）。投递失败不翻结果，重放重发同一份。
func (handler *RegisterTransportHandoverHandler) handOff(
	ctx context.Context,
	record ports.TransportHandoverRecord,
) string {
	if err := handler.deps.Downstream.HandOffTransportHandover(ctx, ports.TransportHandoverRegistrationIntent{Record: record}); err == nil {
		return ""
	}
	return handoverRegistrationContinuation("TRANSPORT_HANDOVER_HANDOFF", record.Key.TenantID.String(), record.Key.Object.String())
}

func handoverRegistrationContinuation(parts ...string) string {
	digest := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return "CONT-" + hex.EncodeToString(digest[:8])
}

// handoverDigest 是同一判断版本的内容比对锚：裁决、双方、证据、规则、依据与业务时间
// 任一不同即是另一份内容。
func handoverDigest(command RegisterTransportHandoverCommand) string {
	digest := sha256.Sum256([]byte(strings.Join([]string{
		fmt.Sprintf("%d", command.Verdict),
		command.ReleasedBy,
		command.ReceivedBy,
		command.ReleasingEvidence,
		command.ReceivingEvidence,
		command.Rule,
		command.Basis,
		command.JudgedAt.UTC().Format(time.RFC3339Nano),
	}, "\x00")))
	return hex.EncodeToString(digest[:])
}

func correctionDigest(command CorrectTransportHandoverCommand) string {
	digest := sha256.Sum256([]byte(strings.Join([]string{
		fmt.Sprintf("%d", command.Verdict),
		command.PredecessorVersion,
		command.ReleasingEvidence,
		command.ReceivingEvidence,
		command.Rule,
		command.Basis,
		command.CorrectedAt.UTC().Format(time.RFC3339Nano),
	}, "\x00")))
	return hex.EncodeToString(digest[:])
}
