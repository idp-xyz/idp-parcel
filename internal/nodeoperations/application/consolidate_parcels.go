package application

import (
	"context"
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
	default:
		return ""
	}
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
	tenant domain.TenantID,
	id domain.ConsolidationUnitID,
	asset domain.CarrierAssetReference,
) (ConsolidationResult, error) {
	if strings.TrimSpace(tenant.String()) == "" {
		return ConsolidationResult{outcome: ConsolidationNotAccepted}, nil
	}
	unit, err := domain.OpenConsolidationUnit(id, asset)
	if err != nil {
		return ConsolidationResult{outcome: ConsolidationNotAccepted}, nil
	}
	saved, err := handler.deps.Store.Save(ctx, tenant, unit)
	if err != nil {
		return ConsolidationResult{outcome: ConsolidationUndecided}, nil
	}
	switch saved {
	case ports.ConsolidationSaved:
		return ConsolidationResult{outcome: UnitOpened, unit: unit}, nil
	case ports.ConsolidationAlreadyRecorded:
		existing, found, err := handler.deps.Store.FindByID(ctx, tenant, id)
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
	tenant domain.TenantID,
	id domain.ConsolidationUnitID,
	member domain.HandlingUnitID,
) (ConsolidationResult, error) {
	unit, found, err := handler.deps.Store.FindByID(ctx, tenant, id)
	if err != nil {
		return ConsolidationResult{outcome: ConsolidationUndecided}, nil
	}
	if !found {
		return ConsolidationResult{outcome: UnitNotFound}, nil
	}

	parent, contained, err := handler.deps.Containment.CurrentParent(ctx, tenant, member)
	if err != nil {
		return ConsolidationResult{outcome: ConsolidationUndecided}, nil
	}
	if contained {
		if parent == id {
			return ConsolidationResult{outcome: MemberAlreadyContained, unit: unit}, nil
		}
		return ConsolidationResult{outcome: MemberElsewhereContained, elsewhere: parent}, nil
	}

	if err := unit.AddMember(member); err != nil {
		return ConsolidationResult{outcome: ConsolidationNotAccepted}, nil
	}
	if err := handler.deps.Store.Update(ctx, tenant, unit); err != nil {
		return ConsolidationResult{outcome: ConsolidationUndecided}, nil
	}
	return ConsolidationResult{outcome: MemberAdded, unit: unit}, nil
}

// RemoveMember 把实物移出单元（开放态，领域拦）。
func (handler *ConsolidateParcelsHandler) RemoveMember(
	ctx context.Context,
	tenant domain.TenantID,
	id domain.ConsolidationUnitID,
	member domain.HandlingUnitID,
) (ConsolidationResult, error) {
	unit, found, err := handler.deps.Store.FindByID(ctx, tenant, id)
	if err != nil {
		return ConsolidationResult{outcome: ConsolidationUndecided}, nil
	}
	if !found {
		return ConsolidationResult{outcome: UnitNotFound}, nil
	}
	if err := unit.RemoveMember(member); err != nil {
		return ConsolidationResult{outcome: ConsolidationNotAccepted}, nil
	}
	if err := handler.deps.Store.Update(ctx, tenant, unit); err != nil {
		return ConsolidationResult{outcome: ConsolidationUndecided}, nil
	}
	return ConsolidationResult{outcome: MemberRemoved, unit: unit}, nil
}

// Seal 封装并交快照意图：冻结当时成员、封签与依据（领域把门空单元与重复封装）；
// 快照随意图交装载与交接消费，失败不翻封装留续办。
func (handler *ConsolidateParcelsHandler) Seal(
	ctx context.Context,
	tenant domain.TenantID,
	id domain.ConsolidationUnitID,
	seal domain.SealReference,
	basis domain.WorkBasisReference,
) (ConsolidationResult, error) {
	unit, found, err := handler.deps.Store.FindByID(ctx, tenant, id)
	if err != nil {
		return ConsolidationResult{outcome: ConsolidationUndecided}, nil
	}
	if !found {
		return ConsolidationResult{outcome: UnitNotFound}, nil
	}
	if err := unit.Seal(seal, basis, handler.deps.Clock.Now()); err != nil {
		return ConsolidationResult{outcome: ConsolidationNotAccepted}, nil
	}
	if err := handler.deps.Store.Update(ctx, tenant, unit); err != nil {
		return ConsolidationResult{outcome: ConsolidationUndecided}, nil
	}
	result := ConsolidationResult{outcome: UnitSealedRecorded, unit: unit}
	snapshots := unit.Snapshots()
	result.handoffRef = handler.handOffSnapshot(ctx, tenant, id, snapshots[len(snapshots)-1])
	return result, nil
}

// Unseal 开封（历史快照原样保留，领域拦未封装）。
func (handler *ConsolidateParcelsHandler) Unseal(
	ctx context.Context,
	tenant domain.TenantID,
	id domain.ConsolidationUnitID,
	basis domain.WorkBasisReference,
) (ConsolidationResult, error) {
	unit, found, err := handler.deps.Store.FindByID(ctx, tenant, id)
	if err != nil {
		return ConsolidationResult{outcome: ConsolidationUndecided}, nil
	}
	if !found {
		return ConsolidationResult{outcome: UnitNotFound}, nil
	}
	if err := unit.Unseal(basis, handler.deps.Clock.Now()); err != nil {
		return ConsolidationResult{outcome: ConsolidationNotAccepted}, nil
	}
	if err := handler.deps.Store.Update(ctx, tenant, unit); err != nil {
		return ConsolidationResult{outcome: ConsolidationUndecided}, nil
	}
	return ConsolidationResult{outcome: UnitUnsealed, unit: unit}, nil
}

// Close 永久终局关闭（成员未清空且无处置转移依据由领域拦；已关闭重复关闭按领域
// 错误未受理——终局不重演）。
func (handler *ConsolidateParcelsHandler) Close(
	ctx context.Context,
	tenant domain.TenantID,
	id domain.ConsolidationUnitID,
	disposition domain.WorkBasisReference,
	at time.Time,
) (ConsolidationResult, error) {
	unit, found, err := handler.deps.Store.FindByID(ctx, tenant, id)
	if err != nil {
		return ConsolidationResult{outcome: ConsolidationUndecided}, nil
	}
	if !found {
		return ConsolidationResult{outcome: UnitNotFound}, nil
	}
	if err := unit.Close(disposition, at); err != nil {
		return ConsolidationResult{outcome: ConsolidationNotAccepted}, nil
	}
	if err := handler.deps.Store.Update(ctx, tenant, unit); err != nil {
		return ConsolidationResult{outcome: ConsolidationUndecided}, nil
	}
	return ConsolidationResult{outcome: UnitClosedRecorded, unit: unit}, nil
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
