package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
)

// ErrUnexpectedConsolidationSave 说明集运库交回了封闭集合以外的写入结果。
var ErrUnexpectedConsolidationSave = errors.New("node operations: unexpected consolidation save outcome")

// ConsolidationOutcome 是集运作业请求的应用处理结果。
type ConsolidationOutcome uint8

const (
	ConsolidationOutcomeInvalid ConsolidationOutcome = iota
	UnitOpened
	UnitExisting
	MemberAdded
	MemberAlreadyContained
	MemberElsewhereContained
	MemberRemoved
	UnitSealedRecorded
	UnitUnsealed
	UnitClosedRecorded
	UnitNotFound
	ConsolidationNotAccepted
	ConsolidationUndecided
	ConsolidationExistingResult
	ConsolidationSourceConflict
)

func (outcome ConsolidationOutcome) String() string {
	switch outcome {
	case UnitOpened:
		return "OPENED"
	case UnitExisting:
		return "EXISTING"
	case MemberAdded:
		return "MEMBER_ADDED"
	case MemberAlreadyContained:
		return "MEMBER_ALREADY_CONTAINED"
	case MemberElsewhereContained:
		return "MEMBER_ELSEWHERE_CONTAINED"
	case MemberRemoved:
		return "MEMBER_REMOVED"
	case UnitSealedRecorded:
		return "SEALED"
	case UnitUnsealed:
		return "UNSEALED"
	case UnitClosedRecorded:
		return "CLOSED"
	case UnitNotFound:
		return "UNIT_NOT_FOUND"
	case ConsolidationNotAccepted:
		return "NOT_ACCEPTED"
	case ConsolidationUndecided:
		return "UNDECIDED"
	case ConsolidationExistingResult:
		return "EXISTING_RESULT"
	case ConsolidationSourceConflict:
		return "SOURCE_CONFLICT"
	default:
		return ""
	}
}

// 六个集运口的输入。每口各一个命令类型而不是共用一个带可选字段的大结构：载具只在开启
// 格有意义、成员只在移入/移出两格有意义，压进一个类型后「这一格该不该填」就只剩注释在
// 说，而注释拦不住调用方。
//
// 四个来源格由 domain.WorkFactSource 一次带入，六口共用同一个词——来源身份兼幂等键，
// 缺它即不受理（AT-NO-043 在集运口上无从判定），执行方、证据与业务发生时间是 UC-NO-003
// 结果契约「作业事实已形成」要求保存的那几项。

type OpenUnitCommand struct {
	TenantID domain.TenantID
	Unit     domain.ConsolidationUnitID
	Asset    domain.CarrierAssetReference
	Source   domain.WorkFactSource
}

type AddMemberCommand struct {
	TenantID domain.TenantID
	Unit     domain.ConsolidationUnitID
	Member   domain.HandlingUnitID
	Source   domain.WorkFactSource
}

type RemoveMemberCommand struct {
	TenantID domain.TenantID
	Unit     domain.ConsolidationUnitID
	Member   domain.HandlingUnitID
	Source   domain.WorkFactSource
}

type SealUnitCommand struct {
	TenantID domain.TenantID
	Unit     domain.ConsolidationUnitID
	Seal     domain.SealReference
	Basis    domain.WorkBasisReference
	Source   domain.WorkFactSource
}

type UnsealUnitCommand struct {
	TenantID domain.TenantID
	Unit     domain.ConsolidationUnitID
	Basis    domain.WorkBasisReference
	Source   domain.WorkFactSource
}

type CloseUnitCommand struct {
	TenantID    domain.TenantID
	Unit        domain.ConsolidationUnitID
	Disposition domain.WorkBasisReference
	Source      domain.WorkFactSource
}

type ConsolidationResult struct {
	outcome    ConsolidationOutcome
	unit       *domain.ConsolidationUnit
	elsewhere  domain.ConsolidationUnitID
	handoffRef string
}

func (result ConsolidationResult) Outcome() ConsolidationOutcome {
	return result.outcome
}

func (result ConsolidationResult) Unit() (*domain.ConsolidationUnit, bool) {
	return result.unit, result.unit != nil
}

// Elsewhere 只在成员已被别的单元包含时给出——违反单父级的恢复动作是先从那里移出。
func (result ConsolidationResult) Elsewhere() domain.ConsolidationUnitID {
	return result.elsewhere
}

// HandoffReference 非空说明封装快照已入册但意图还没交出去，重放会重发同一份。
func (result ConsolidationResult) HandoffReference() string {
	return result.handoffRef
}

type ConsolidateParcelsDeps struct {
	Store       ports.ConsolidationStore
	Facts       ports.ConsolidationFactStore
	Containment ports.ContainmentIndex
	Downstream  ports.SealedSnapshotHandoff
	Clock       ports.Clock
}

type ConsolidateParcelsHandler struct {
	deps ConsolidateParcelsDeps
}

func NewConsolidateParcelsHandler(deps ConsolidateParcelsDeps) *ConsolidateParcelsHandler {
	return &ConsolidateParcelsHandler{deps: deps}
}

// Open 开启集运单元实例：同 ID 重复开启返原实例（载具复用创建新实例是新 ID 的事）。
func (handler *ConsolidateParcelsHandler) Open(
	ctx context.Context,
	command OpenUnitCommand,
) (ConsolidationResult, error) {
	digest := consolidationDigest(domain.OpenUnitAction, command.Source,
		command.Unit.String(), command.Asset.String())
	gate, proceed := handler.admit(ctx, command.TenantID, command.Unit, command.Source, digest)
	if !proceed {
		return gate.result, nil
	}

	unit, err := domain.OpenConsolidationUnit(command.Unit, command.Asset, command.Source)
	if err != nil {
		return handler.rejected(ctx, gate), nil
	}
	saved, err := handler.deps.Store.Save(ctx, command.TenantID, unit)
	if err != nil {
		return ConsolidationResult{outcome: ConsolidationUndecided}, nil
	}
	switch saved {
	case ports.ConsolidationSaved:
		return handler.recorded(ctx, gate, ports.ConsolidationFactRecord{
			Action: domain.OpenUnitAction,
			Unit:   command.Unit,
		}, ConsolidationResult{outcome: UnitOpened, unit: unit}), nil
	case ports.ConsolidationAlreadyRecorded:
		// 别的来源先开了同一个实例：这不是来源冲突（两次报的不是同一件事），本次没有
		// 作业发生，因此不留来源事实——重放照样读到同一个已有实例。
		existing, found, err := handler.deps.Store.FindByID(ctx, command.TenantID, command.Unit)
		if err != nil || !found {
			return ConsolidationResult{outcome: ConsolidationUndecided}, nil
		}
		return ConsolidationResult{outcome: UnitExisting, unit: existing}, nil
	default:
		return ConsolidationResult{}, fmt.Errorf("%w: %d", ErrUnexpectedConsolidationSave, saved)
	}
}

// AddMember 把实物移入单元：先跨单元核对「同一时点最多一个直接物理父级」——已在
// 本单元是幂等重放、在别的未关闭单元里是业务负向带对方标识（恢复动作是先从那里
// 移出，不是重试）；封装/关闭态由领域拦。
func (handler *ConsolidateParcelsHandler) AddMember(
	ctx context.Context,
	command AddMemberCommand,
) (ConsolidationResult, error) {
	digest := consolidationDigest(domain.AddMemberAction, command.Source,
		command.Unit.String(), command.Member.String())
	gate, proceed := handler.admit(ctx, command.TenantID, command.Unit, command.Source, digest)
	if !proceed {
		return gate.result, nil
	}

	unit, found, err := handler.deps.Store.FindByID(ctx, command.TenantID, command.Unit)
	if err != nil {
		return ConsolidationResult{outcome: ConsolidationUndecided}, nil
	}
	if !found {
		return ConsolidationResult{outcome: UnitNotFound}, nil
	}

	parent, contained, err := handler.deps.Containment.CurrentParent(ctx, command.TenantID, command.Member)
	if err != nil {
		return ConsolidationResult{outcome: ConsolidationUndecided}, nil
	}
	if contained {
		if parent == command.Unit {
			return ConsolidationResult{outcome: MemberAlreadyContained, unit: unit}, nil
		}
		return ConsolidationResult{outcome: MemberElsewhereContained, elsewhere: parent}, nil
	}

	if err := unit.AddMember(command.Member); err != nil {
		return handler.rejected(ctx, gate), nil
	}
	if err := handler.deps.Store.Update(ctx, command.TenantID, unit); err != nil {
		return ConsolidationResult{outcome: ConsolidationUndecided}, nil
	}
	return handler.recorded(ctx, gate, ports.ConsolidationFactRecord{
		Action: domain.AddMemberAction,
		Unit:   command.Unit,
		Member: command.Member,
	}, ConsolidationResult{outcome: MemberAdded, unit: unit}), nil
}

// RemoveMember 把实物移出单元（开放态，领域拦）。
func (handler *ConsolidateParcelsHandler) RemoveMember(
	ctx context.Context,
	command RemoveMemberCommand,
) (ConsolidationResult, error) {
	digest := consolidationDigest(domain.RemoveMemberAction, command.Source,
		command.Unit.String(), command.Member.String())
	gate, proceed := handler.admit(ctx, command.TenantID, command.Unit, command.Source, digest)
	if !proceed {
		return gate.result, nil
	}

	unit, found, err := handler.deps.Store.FindByID(ctx, command.TenantID, command.Unit)
	if err != nil {
		return ConsolidationResult{outcome: ConsolidationUndecided}, nil
	}
	if !found {
		return ConsolidationResult{outcome: UnitNotFound}, nil
	}
	if err := unit.RemoveMember(command.Member); err != nil {
		return handler.rejected(ctx, gate), nil
	}
	if err := handler.deps.Store.Update(ctx, command.TenantID, unit); err != nil {
		return ConsolidationResult{outcome: ConsolidationUndecided}, nil
	}
	return handler.recorded(ctx, gate, ports.ConsolidationFactRecord{
		Action: domain.RemoveMemberAction,
		Unit:   command.Unit,
		Member: command.Member,
	}, ConsolidationResult{outcome: MemberRemoved, unit: unit}), nil
}

// Seal 封装并交快照意图：冻结当时成员、封签与依据（领域把门空单元与重复封装）；
// 快照随意图交装载与交接消费，失败不翻封装留续办。封装时刻由来源自带的业务时间
// 决定，不再取处理时的系统时钟。
func (handler *ConsolidateParcelsHandler) Seal(
	ctx context.Context,
	command SealUnitCommand,
) (ConsolidationResult, error) {
	digest := consolidationDigest(domain.SealUnitAction, command.Source,
		command.Unit.String(), command.Seal.String(), command.Basis.String())
	gate, proceed := handler.admit(ctx, command.TenantID, command.Unit, command.Source, digest)
	if !proceed {
		return gate.result, nil
	}

	unit, found, err := handler.deps.Store.FindByID(ctx, command.TenantID, command.Unit)
	if err != nil {
		return ConsolidationResult{outcome: ConsolidationUndecided}, nil
	}
	if !found {
		return ConsolidationResult{outcome: UnitNotFound}, nil
	}
	if err := unit.Seal(command.Seal, command.Basis, command.Source); err != nil {
		return handler.rejected(ctx, gate), nil
	}
	if err := handler.deps.Store.Update(ctx, command.TenantID, unit); err != nil {
		return ConsolidationResult{outcome: ConsolidationUndecided}, nil
	}
	result := ConsolidationResult{outcome: UnitSealedRecorded, unit: unit}
	snapshots := unit.Snapshots()
	result.handoffRef = handler.handOffSnapshot(ctx, command.TenantID, command.Unit, snapshots[len(snapshots)-1])
	return handler.recorded(ctx, gate, ports.ConsolidationFactRecord{
		Action: domain.SealUnitAction,
		Unit:   command.Unit,
		Seal:   command.Seal,
	}, result), nil
}

// Unseal 受控开封（历史快照原样保留，领域拦未封装）。CONTEXT 要求节点保存开封的授权
// 来源、执行人与时间——它们落在来源事实登记上，不改写任何历史快照。
func (handler *ConsolidateParcelsHandler) Unseal(
	ctx context.Context,
	command UnsealUnitCommand,
) (ConsolidationResult, error) {
	digest := consolidationDigest(domain.UnsealUnitAction, command.Source,
		command.Unit.String(), command.Basis.String())
	gate, proceed := handler.admit(ctx, command.TenantID, command.Unit, command.Source, digest)
	if !proceed {
		return gate.result, nil
	}

	unit, found, err := handler.deps.Store.FindByID(ctx, command.TenantID, command.Unit)
	if err != nil {
		return ConsolidationResult{outcome: ConsolidationUndecided}, nil
	}
	if !found {
		return ConsolidationResult{outcome: UnitNotFound}, nil
	}
	if err := unit.Unseal(command.Basis, command.Source.OccurredAt()); err != nil {
		return handler.rejected(ctx, gate), nil
	}
	if err := handler.deps.Store.Update(ctx, command.TenantID, unit); err != nil {
		return ConsolidationResult{outcome: ConsolidationUndecided}, nil
	}
	return handler.recorded(ctx, gate, ports.ConsolidationFactRecord{
		Action: domain.UnsealUnitAction,
		Unit:   command.Unit,
	}, ConsolidationResult{outcome: UnitUnsealed, unit: unit}), nil
}

// Close 永久终局关闭（成员未清空且无处置转移依据由领域拦；已关闭重复关闭按领域
// 错误未受理——终局不重演）。
func (handler *ConsolidateParcelsHandler) Close(
	ctx context.Context,
	command CloseUnitCommand,
) (ConsolidationResult, error) {
	digest := consolidationDigest(domain.CloseUnitAction, command.Source,
		command.Unit.String(), command.Disposition.String())
	gate, proceed := handler.admit(ctx, command.TenantID, command.Unit, command.Source, digest)
	if !proceed {
		return gate.result, nil
	}

	unit, found, err := handler.deps.Store.FindByID(ctx, command.TenantID, command.Unit)
	if err != nil {
		return ConsolidationResult{outcome: ConsolidationUndecided}, nil
	}
	if !found {
		return ConsolidationResult{outcome: UnitNotFound}, nil
	}
	if err := unit.Close(command.Disposition, command.Source.OccurredAt()); err != nil {
		return handler.rejected(ctx, gate), nil
	}
	if err := handler.deps.Store.Update(ctx, command.TenantID, unit); err != nil {
		return ConsolidationResult{outcome: ConsolidationUndecided}, nil
	}
	return handler.recorded(ctx, gate, ports.ConsolidationFactRecord{
		Action: domain.CloseUnitAction,
		Unit:   command.Unit,
	}, ConsolidationResult{outcome: UnitClosedRecorded, unit: unit}), nil
}

// admissionGate 带着一次受理判断的全部上下文，好让四条出口（放行、已有结果、冲突、
// 未受理复查）共用同一份来源身份与内容指纹，不各自重算一遍。
type admissionGate struct {
	tenant domain.TenantID
	key    ports.ConsolidationFactKey
	digest string
	source domain.WorkFactSource
	result ConsolidationResult
}

// admit 是六口共用的受理闸：先要求来源表达完整，再按来源身份分流。
//
// 三条出口对应 AT-NO-043 的两向加一条不受理：同一来源身份携带同一内容是重放，答已有
// 结果；同一身份携带不同内容是冲突，两份都保留、什么都不改；来源身份缺席即不受理——
// 不给业务时间兜底填系统时钟，那正是 ADR-0023 禁止的服务端代铸。
func (handler *ConsolidateParcelsHandler) admit(
	ctx context.Context,
	tenant domain.TenantID,
	unit domain.ConsolidationUnitID,
	source domain.WorkFactSource,
	digest string,
) (admissionGate, bool) {
	gate := admissionGate{tenant: tenant, digest: digest, source: source}
	if strings.TrimSpace(tenant.String()) == "" ||
		strings.TrimSpace(unit.String()) == "" ||
		!isSourceExpressed(source) {
		gate.result = ConsolidationResult{outcome: ConsolidationNotAccepted}
		return gate, false
	}
	gate.key = ports.ConsolidationFactKey{TenantID: tenant, SourceID: source.SourceID()}

	existing, found, err := handler.deps.Facts.FindByKey(ctx, gate.key)
	if err != nil {
		gate.result = ConsolidationResult{outcome: ConsolidationUndecided}
		return gate, false
	}
	if !found {
		return gate, true
	}
	if existing.ContentDigest != digest {
		gate.result = ConsolidationResult{outcome: ConsolidationSourceConflict}
		return gate, false
	}
	gate.result = handler.existingResult(ctx, tenant, existing.Unit)
	return gate, false
}

// recorded 在作业真的发生之后留下来源事实。并发下同一来源身份的另一半先提交时，
// 读回它并按已有结果作答——本次的领域动作已被领域自己的三相拦在前面。
func (handler *ConsolidateParcelsHandler) recorded(
	ctx context.Context,
	gate admissionGate,
	record ports.ConsolidationFactRecord,
	result ConsolidationResult,
) ConsolidationResult {
	record.Key = gate.key
	record.ContentDigest = gate.digest
	record.PerformedBy = gate.source.PerformedBy()
	record.Evidence = gate.source.Evidence()
	record.OccurredAt = gate.source.OccurredAt()
	record.RecordedAt = handler.deps.Clock.Now()

	saved, err := handler.deps.Facts.Save(ctx, record)
	if err != nil {
		return ConsolidationResult{outcome: ConsolidationUndecided}
	}
	if saved == ports.ConsolidationFactAlreadyRecorded {
		return handler.existingResult(ctx, gate.tenant, record.Unit)
	}
	return result
}

// rejected 复查登记再答未受理。领域拒绝有两种来路：这一步在当前三相下本就做不得，
// 或者同一来源身份的前一次已经把它做完了（重放跑赢了 admit 那次读）。后者的真话是
// 「已有结果」——两者都答未受理会让现场按三相去找一处并不存在的状态错。
func (handler *ConsolidateParcelsHandler) rejected(
	ctx context.Context,
	gate admissionGate,
) ConsolidationResult {
	existing, found, err := handler.deps.Facts.FindByKey(ctx, gate.key)
	if err == nil && found && existing.ContentDigest == gate.digest {
		return handler.existingResult(ctx, gate.tenant, existing.Unit)
	}
	return ConsolidationResult{outcome: ConsolidationNotAccepted}
}

// existingResult 按已有来源事实作答：结果格是`已有结果`，单元照当前状态读回。
func (handler *ConsolidateParcelsHandler) existingResult(
	ctx context.Context,
	tenant domain.TenantID,
	id domain.ConsolidationUnitID,
) ConsolidationResult {
	unit, found, err := handler.deps.Store.FindByID(ctx, tenant, id)
	if err != nil {
		return ConsolidationResult{outcome: ConsolidationUndecided}
	}
	if !found {
		return ConsolidationResult{outcome: ConsolidationExistingResult}
	}
	return ConsolidationResult{outcome: ConsolidationExistingResult, unit: unit}
}

// isSourceExpressed 判来源四格是否齐备。领域构造器已经守着同一条，这里再判一次是为了
// 让零值 WorkFactSource（调用方压根没填）在触及领域之前就落进`未受理`。
func isSourceExpressed(source domain.WorkFactSource) bool {
	return strings.TrimSpace(source.SourceID()) != "" &&
		strings.TrimSpace(source.PerformedBy().String()) != "" &&
		strings.TrimSpace(source.Evidence().String()) != "" &&
		!source.OccurredAt().IsZero()
}

// handOffSnapshot 交封装快照意图。失败不翻封装，留续办引用重发同一份。
func (handler *ConsolidateParcelsHandler) handOffSnapshot(
	ctx context.Context,
	tenant domain.TenantID,
	id domain.ConsolidationUnitID,
	snapshot domain.SealedSnapshot,
) string {
	if err := handler.deps.Downstream.HandOffSnapshot(ctx, ports.SealedSnapshotHandoffIntent{
		TenantID: tenant,
		Unit:     id,
		Snapshot: snapshot,
	}); err == nil {
		return ""
	}
	return "CONT-SNAPSHOT/" + id.String() + "/" + snapshot.Seal().String()
}

// consolidationDigest 是同一来源身份的内容比对锚：来源身份之外的一切都算内容——动作、
// 对象、这一格特有的参数（载具/成员/封签/依据）、执行方、证据与业务发生时间，任一不同
// 即是另一份内容，按 AT-NO-043 形成冲突而不是覆盖先到者。
func consolidationDigest(
	action domain.ConsolidationActionKind,
	source domain.WorkFactSource,
	parts ...string,
) string {
	fields := append([]string{
		action.String(),
		source.PerformedBy().String(),
		source.Evidence().String(),
		source.OccurredAt().UTC().Format(time.RFC3339Nano),
	}, parts...)
	digest := sha256.Sum256([]byte(strings.Join(fields, "\x00")))
	return hex.EncodeToString(digest[:])
}
