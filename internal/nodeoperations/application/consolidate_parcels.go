package application

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
)

// ErrUnexpectedConsolidationSave 说明集运库交回了封闭集合以外的写入结果。
var ErrUnexpectedConsolidationSave = errors.New("node operations: unexpected consolidation save outcome")

var errStoredConsolidationDigestNotComparable = errors.New("node operations: stored consolidation fact digest is not comparable")

// ErrNoDirectionalBasisRecord 说明移入的调用方没交方向依据记录。这是装配缺陷，不是业务答案：真缺依据是记录里
// 一格的取值，照样落册；没判就交进来，落下去的只能是一条读作`未记录`的新事实，而那一格留给的是它出现之前的旧事实。
var ErrNoDirectionalBasisRecord = errors.New("node operations: add member needs the directional basis judged before the write")

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
	basis      domain.DirectionalBasisRecord
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

// DirectionalBasis 交回移入事实所带的方向依据记录：新形成时是与事实同一笔落册的那一份，`已有结果`时是册上
// 原样的那一份，判断时刻也是当时的——本次调用方交进来的那一份不落册、不交回。`已有结果`交回零值时，说的是那条
// 移入事实落册时还不记方向依据（`未记录`），不是依据齐全；其余结果恒为零值。
func (result ConsolidationResult) DirectionalBasis() domain.DirectionalBasisRecord {
	return result.basis
}

// ConsolidateParcelsDeps 是集运编排的全部依赖，缺任一个构造即拒。
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

// NewConsolidateParcelsHandler 在构造期拒缺依赖的装配：留到运行期，缺件要到第一次集运作业时才露出来（ADR-0186）。
func NewConsolidateParcelsHandler(deps ConsolidateParcelsDeps) (*ConsolidateParcelsHandler, error) {
	switch {
	case deps.Store == nil:
		return nil, errors.New("node operations: consolidation needs a unit store")
	case deps.Facts == nil:
		return nil, errors.New("node operations: consolidation needs a fact store")
	case deps.Containment == nil:
		return nil, errors.New("node operations: consolidation needs a containment index")
	case deps.Downstream == nil:
		return nil, errors.New("node operations: consolidation needs a sealed snapshot handoff")
	case deps.Clock == nil:
		return nil, errors.New("node operations: consolidation needs a clock")
	}
	return &ConsolidateParcelsHandler{deps: deps}, nil
}

// Open 开启集运单元实例：同 ID 重复开启返原实例（载具复用创建新实例是新 ID 的事）。
func (handler *ConsolidateParcelsHandler) Open(
	ctx context.Context,
	command OpenUnitCommand,
) (ConsolidationResult, error) {
	_, digest, err := domain.CanonicalizeOpenUnitPayload(command.Source, command.Unit, command.Asset)
	if err != nil {
		return ConsolidationResult{}, err
	}
	gate, proceed, err := handler.admit(ctx, command.TenantID, command.Unit, command.Source, digest,
		unversionedConsolidationDigest(domain.OpenUnitAction, command.Source, command.Unit.String(), command.Asset.String()))
	if err != nil {
		return ConsolidationResult{}, err
	}
	if !proceed {
		return gate.result, nil
	}

	unit, err := domain.OpenConsolidationUnit(command.Unit, command.Asset, command.Source)
	if err != nil {
		return handler.rejected(ctx, gate)
	}
	saved, err := handler.deps.Store.Save(ctx, command.TenantID, unit)
	if err != nil {
		return ConsolidationResult{}, fmt.Errorf("record consolidation unit: %w", err)
	}
	switch saved {
	case ports.ConsolidationSaved:
		return handler.recorded(ctx, gate, ports.ConsolidationFactRecord{
			Action: domain.OpenUnitAction,
			Unit:   command.Unit,
		}, ConsolidationResult{outcome: UnitOpened, unit: unit})
	case ports.ConsolidationAlreadyRecorded:
		// 别的来源先开了同一个实例：这不是来源冲突（两次报的不是同一件事），本次没有
		// 作业发生，因此不留来源事实——重放照样读到同一个已有实例。
		existing, found, err := handler.deps.Store.FindByID(ctx, command.TenantID, command.Unit)
		if err != nil {
			return ConsolidationResult{}, fmt.Errorf("read back consolidation unit: %w", err)
		}
		if !found {
			return ConsolidationResult{}, errors.New("node operations: consolidation unit already recorded but not readable")
		}
		return ConsolidationResult{outcome: UnitExisting, unit: existing}, nil
	default:
		return ConsolidationResult{}, fmt.Errorf("%w: %d", ErrUnexpectedConsolidationSave, saved)
	}
}

// AddMember 把实物移入单元：先跨单元核对「同一时点最多一个直接物理父级」——已在
// 本单元是幂等重放、在别的未关闭单元里是业务负向带对方标识（恢复动作是先从那里
// 移出，不是重试）；封装/关闭态由领域拦。
//
// basis 是调用方在写事务之前判好的方向依据记录（DirectionalBasisJudge），只在形成新的移入事实时与事实同一笔
// 落册。它不挡事实：缺依据、读不动都照常 MEMBER_ADDED——UC-NO-003「已经发生的错误分拣保留为事实并形成偏离，
// 不回写为未发生」，偏离的判断归 network-routing，异常归 visibility-exception。
func (handler *ConsolidateParcelsHandler) AddMember(
	ctx context.Context,
	command AddMemberCommand,
	basis domain.DirectionalBasisRecord,
) (ConsolidationResult, error) {
	if !basis.Recorded() {
		return ConsolidationResult{}, ErrNoDirectionalBasisRecord
	}
	_, digest, err := domain.CanonicalizeAddMemberPayload(command.Source, command.Unit, command.Member)
	if err != nil {
		return ConsolidationResult{}, err
	}
	gate, proceed, err := handler.admit(ctx, command.TenantID, command.Unit, command.Source, digest,
		unversionedConsolidationDigest(domain.AddMemberAction, command.Source, command.Unit.String(), command.Member.String()))
	if err != nil {
		return ConsolidationResult{}, err
	}
	if !proceed {
		return gate.result, nil
	}

	unit, found, err := handler.deps.Store.FindByID(ctx, command.TenantID, command.Unit)
	if err != nil {
		return ConsolidationResult{}, fmt.Errorf("look up consolidation unit: %w", err)
	}
	if !found {
		return ConsolidationResult{outcome: UnitNotFound}, nil
	}

	parent, contained, err := handler.deps.Containment.CurrentParent(ctx, command.TenantID, command.Member)
	if err != nil {
		return ConsolidationResult{}, fmt.Errorf("look up current parent: %w", err)
	}
	if contained {
		if parent == command.Unit {
			return ConsolidationResult{outcome: MemberAlreadyContained, unit: unit}, nil
		}
		return ConsolidationResult{outcome: MemberElsewhereContained, elsewhere: parent}, nil
	}

	if err := unit.AddMember(command.Member); err != nil {
		return handler.rejected(ctx, gate)
	}
	if err := handler.deps.Store.Update(ctx, command.TenantID, unit); err != nil {
		return ConsolidationResult{}, fmt.Errorf("update consolidation unit: %w", err)
	}
	return handler.recorded(ctx, gate, ports.ConsolidationFactRecord{
		Action:           domain.AddMemberAction,
		Unit:             command.Unit,
		Member:           command.Member,
		DirectionalBasis: basis,
	}, ConsolidationResult{outcome: MemberAdded, unit: unit, basis: basis})
}

// RemoveMember 把实物移出单元（开放态，领域拦）。
func (handler *ConsolidateParcelsHandler) RemoveMember(
	ctx context.Context,
	command RemoveMemberCommand,
) (ConsolidationResult, error) {
	_, digest, err := domain.CanonicalizeRemoveMemberPayload(command.Source, command.Unit, command.Member)
	if err != nil {
		return ConsolidationResult{}, err
	}
	gate, proceed, err := handler.admit(ctx, command.TenantID, command.Unit, command.Source, digest,
		unversionedConsolidationDigest(domain.RemoveMemberAction, command.Source, command.Unit.String(), command.Member.String()))
	if err != nil {
		return ConsolidationResult{}, err
	}
	if !proceed {
		return gate.result, nil
	}

	unit, found, err := handler.deps.Store.FindByID(ctx, command.TenantID, command.Unit)
	if err != nil {
		return ConsolidationResult{}, fmt.Errorf("look up consolidation unit: %w", err)
	}
	if !found {
		return ConsolidationResult{outcome: UnitNotFound}, nil
	}
	if err := unit.RemoveMember(command.Member); err != nil {
		return handler.rejected(ctx, gate)
	}
	if err := handler.deps.Store.Update(ctx, command.TenantID, unit); err != nil {
		return ConsolidationResult{}, fmt.Errorf("update consolidation unit: %w", err)
	}
	return handler.recorded(ctx, gate, ports.ConsolidationFactRecord{
		Action: domain.RemoveMemberAction,
		Unit:   command.Unit,
		Member: command.Member,
	}, ConsolidationResult{outcome: MemberRemoved, unit: unit})
}

// Seal 封装并交快照意图：冻结当时成员、封签与依据（领域把门空单元与重复封装）；
// 快照随意图交装载与交接消费，失败不翻封装留续办。封装时刻由来源自带的业务时间
// 决定，不再取处理时的系统时钟。
func (handler *ConsolidateParcelsHandler) Seal(
	ctx context.Context,
	command SealUnitCommand,
) (ConsolidationResult, error) {
	_, digest, err := domain.CanonicalizeSealUnitPayload(command.Source, command.Unit, command.Seal, command.Basis)
	if err != nil {
		return ConsolidationResult{}, err
	}
	gate, proceed, err := handler.admit(ctx, command.TenantID, command.Unit, command.Source, digest,
		unversionedConsolidationDigest(domain.SealUnitAction, command.Source, command.Unit.String(), command.Seal.String(), command.Basis.String()))
	if err != nil {
		return ConsolidationResult{}, err
	}
	if !proceed {
		return gate.result, nil
	}

	unit, found, err := handler.deps.Store.FindByID(ctx, command.TenantID, command.Unit)
	if err != nil {
		return ConsolidationResult{}, fmt.Errorf("look up consolidation unit: %w", err)
	}
	if !found {
		return ConsolidationResult{outcome: UnitNotFound}, nil
	}
	if err := unit.Seal(command.Seal, command.Basis, command.Source); err != nil {
		return handler.rejected(ctx, gate)
	}
	if err := handler.deps.Store.Update(ctx, command.TenantID, unit); err != nil {
		return ConsolidationResult{}, fmt.Errorf("update consolidation unit: %w", err)
	}
	result := ConsolidationResult{outcome: UnitSealedRecorded, unit: unit}
	snapshots := unit.Snapshots()
	result.handoffRef = handler.handOffSnapshot(ctx, command.TenantID, command.Unit, snapshots[len(snapshots)-1])
	return handler.recorded(ctx, gate, ports.ConsolidationFactRecord{
		Action: domain.SealUnitAction,
		Unit:   command.Unit,
		Seal:   command.Seal,
	}, result)
}

// Unseal 受控开封（历史快照原样保留，领域拦未封装）。CONTEXT 要求节点保存开封的授权
// 来源、执行人与时间——它们落在来源事实登记上，不改写任何历史快照。
func (handler *ConsolidateParcelsHandler) Unseal(
	ctx context.Context,
	command UnsealUnitCommand,
) (ConsolidationResult, error) {
	_, digest, err := domain.CanonicalizeUnsealUnitPayload(command.Source, command.Unit, command.Basis)
	if err != nil {
		return ConsolidationResult{}, err
	}
	gate, proceed, err := handler.admit(ctx, command.TenantID, command.Unit, command.Source, digest,
		unversionedConsolidationDigest(domain.UnsealUnitAction, command.Source, command.Unit.String(), command.Basis.String()))
	if err != nil {
		return ConsolidationResult{}, err
	}
	if !proceed {
		return gate.result, nil
	}

	unit, found, err := handler.deps.Store.FindByID(ctx, command.TenantID, command.Unit)
	if err != nil {
		return ConsolidationResult{}, fmt.Errorf("look up consolidation unit: %w", err)
	}
	if !found {
		return ConsolidationResult{outcome: UnitNotFound}, nil
	}
	if err := unit.Unseal(command.Basis, command.Source.OccurredAt()); err != nil {
		return handler.rejected(ctx, gate)
	}
	if err := handler.deps.Store.Update(ctx, command.TenantID, unit); err != nil {
		return ConsolidationResult{}, fmt.Errorf("update consolidation unit: %w", err)
	}
	return handler.recorded(ctx, gate, ports.ConsolidationFactRecord{
		Action: domain.UnsealUnitAction,
		Unit:   command.Unit,
	}, ConsolidationResult{outcome: UnitUnsealed, unit: unit})
}

// Close 永久终局关闭（成员未清空且无处置转移依据由领域拦；已关闭重复关闭按领域
// 错误未受理——终局不重演）。
func (handler *ConsolidateParcelsHandler) Close(
	ctx context.Context,
	command CloseUnitCommand,
) (ConsolidationResult, error) {
	_, digest, err := domain.CanonicalizeCloseUnitPayload(command.Source, command.Unit, command.Disposition)
	if err != nil {
		return ConsolidationResult{}, err
	}
	gate, proceed, err := handler.admit(ctx, command.TenantID, command.Unit, command.Source, digest,
		unversionedConsolidationDigest(domain.CloseUnitAction, command.Source, command.Unit.String(), command.Disposition.String()))
	if err != nil {
		return ConsolidationResult{}, err
	}
	if !proceed {
		return gate.result, nil
	}

	unit, found, err := handler.deps.Store.FindByID(ctx, command.TenantID, command.Unit)
	if err != nil {
		return ConsolidationResult{}, fmt.Errorf("look up consolidation unit: %w", err)
	}
	if !found {
		return ConsolidationResult{outcome: UnitNotFound}, nil
	}
	if err := unit.Close(command.Disposition, command.Source.OccurredAt()); err != nil {
		return handler.rejected(ctx, gate)
	}
	if err := handler.deps.Store.Update(ctx, command.TenantID, unit); err != nil {
		return ConsolidationResult{}, fmt.Errorf("update consolidation unit: %w", err)
	}
	return handler.recorded(ctx, gate, ports.ConsolidationFactRecord{
		Action: domain.CloseUnitAction,
		Unit:   command.Unit,
	}, ConsolidationResult{outcome: UnitClosedRecorded, unit: unit})
}

// admissionGate 带着一次受理判断的全部上下文，好让四条出口（放行、已有结果、冲突、
// 未受理复查）共用同一份来源身份与内容指纹，不各自重算一遍。
type admissionGate struct {
	tenant      domain.TenantID
	key         ports.ConsolidationFactKey
	digest      string
	unversioned string
	source      domain.WorkFactSource
	result      ConsolidationResult
}

// admit 是六口共用的受理闸：先要求来源表达完整，再按来源身份分流。
//
// 三条出口对应 AT-NO-043 的两向加一条不受理：同一来源身份携带同一内容是重放，答已有
// 结果；同一身份携带不同内容是冲突，两份都保留、什么都不改；来源身份缺席即不受理——
// 不给业务时间兜底填系统时钟，那正是 ADR-0023 禁止的服务端代铸。
//
// 登记读不动与已存摘要形状认不出交回 error，不答一格：本方册坏了就是没形成答案（ADR-0022），答 2xx 的格设备会
// 出队，这一次作业在服务端从此没有记录（ADR-0186 决定三的同形）。下面几处本方册读写同此。
func (handler *ConsolidateParcelsHandler) admit(
	ctx context.Context,
	tenant domain.TenantID,
	unit domain.ConsolidationUnitID,
	source domain.WorkFactSource,
	digest string,
	unversioned string,
) (admissionGate, bool, error) {
	gate := admissionGate{tenant: tenant, digest: digest, unversioned: unversioned, source: source}
	if strings.TrimSpace(tenant.String()) == "" ||
		strings.TrimSpace(unit.String()) == "" ||
		!isSourceExpressed(source) {
		gate.result = ConsolidationResult{outcome: ConsolidationNotAccepted}
		return gate, false, nil
	}
	gate.key = ports.ConsolidationFactKey{TenantID: tenant, SourceID: source.SourceID()}

	existing, found, err := handler.deps.Facts.FindByKey(ctx, gate.key)
	if err != nil {
		return gate, false, fmt.Errorf("look up consolidation fact: %w", err)
	}
	if !found {
		return gate, true, nil
	}
	switch domain.CompareStoredDigest(existing.ContentDigest, digest, unversioned) {
	case domain.SamePayload:
		gate.result, err = handler.existingResult(ctx, existing)
		return gate, false, err
	case domain.DifferentPayload:
		gate.result = ConsolidationResult{outcome: ConsolidationSourceConflict}
		return gate, false, nil
	default:
		return gate, false, errStoredConsolidationDigestNotComparable
	}
}

// recorded 在作业真的发生之后留下来源事实。并发下同一来源身份的另一半先提交时，读回它：同一内容按已有结果作答，
// 另一份内容答来源冲突。本次单元那一步（Store.Update）已在同一事务里写下，并发两份异内容时它不被撤回——这是
// 单元写口没有版本校验的既有缺口（票 no-consolidation-fact-provenance/02）。
func (handler *ConsolidateParcelsHandler) recorded(
	ctx context.Context,
	gate admissionGate,
	record ports.ConsolidationFactRecord,
	result ConsolidationResult,
) (ConsolidationResult, error) {
	record.Key = gate.key
	record.ContentDigest = gate.digest
	record.PerformedBy = gate.source.PerformedBy()
	record.Evidence = gate.source.Evidence()
	record.OccurredAt = gate.source.OccurredAt()
	record.RecordedAt = handler.deps.Clock.Now()

	saved, err := handler.deps.Facts.Save(ctx, record)
	if err != nil {
		return ConsolidationResult{}, fmt.Errorf("record consolidation fact: %w", err)
	}
	if saved == ports.ConsolidationFactAlreadyRecorded {
		// 交回的方向依据记录得是先到那一份落在册上的，不是本次刚判的：读回赢家再答。赢家是同一来源的另一份内容时，
		// 答来源冲突而不是已有结果——与受理闸同一条代数。
		winner, found, err := handler.deps.Facts.FindByKey(ctx, gate.key)
		if err != nil {
			return ConsolidationResult{}, fmt.Errorf("read back consolidation fact winner: %w", err)
		}
		if !found {
			return ConsolidationResult{}, errors.New("node operations: consolidation fact already recorded but not readable")
		}
		switch domain.CompareStoredDigest(winner.ContentDigest, gate.digest, gate.unversioned) {
		case domain.SamePayload:
			return handler.existingResult(ctx, winner)
		case domain.DifferentPayload:
			return ConsolidationResult{outcome: ConsolidationSourceConflict}, nil
		default:
			return ConsolidationResult{}, errStoredConsolidationDigestNotComparable
		}
	}
	return result, nil
}

// rejected 复查登记再答未受理。领域拒绝有两种来路：这一步在当前三相下本就做不得，
// 或者同一来源身份的前一次已经把它做完了（重放跑赢了 admit 那次读）。后者的真话是
// 「已有结果」——两者都答未受理会让现场按三相去找一处并不存在的状态错。复查读不动时分不出这两种来路，
// 因此交回 error 而不是先答未受理。
func (handler *ConsolidateParcelsHandler) rejected(
	ctx context.Context,
	gate admissionGate,
) (ConsolidationResult, error) {
	existing, found, err := handler.deps.Facts.FindByKey(ctx, gate.key)
	if err != nil {
		return ConsolidationResult{}, fmt.Errorf("recheck consolidation fact: %w", err)
	}
	if found &&
		domain.CompareStoredDigest(existing.ContentDigest, gate.digest, gate.unversioned) == domain.SamePayload {
		return handler.existingResult(ctx, existing)
	}
	return ConsolidationResult{outcome: ConsolidationNotAccepted}, nil
}

// existingResult 按已有来源事实作答：结果格是`已有结果`，单元照当前状态读回，方向依据记录照册上原样交回。
func (handler *ConsolidateParcelsHandler) existingResult(
	ctx context.Context,
	existing ports.ConsolidationFactRecord,
) (ConsolidationResult, error) {
	unit, found, err := handler.deps.Store.FindByID(ctx, existing.Key.TenantID, existing.Unit)
	if err != nil {
		return ConsolidationResult{}, fmt.Errorf("read back consolidation unit: %w", err)
	}
	result := ConsolidationResult{outcome: ConsolidationExistingResult, basis: existing.DirectionalBasis}
	if found {
		result.unit = unit
	}
	return result, nil
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
