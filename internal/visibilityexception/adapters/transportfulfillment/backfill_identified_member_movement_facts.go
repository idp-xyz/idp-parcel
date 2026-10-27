package transportfulfillment

import (
	"context"
	"errors"
	"fmt"

	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	veinbox "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/inbox"
	vedomain "go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	veports "go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

var (
	// ErrFormedIntakeNotVisible 表示按信封键还读不回那份节点收寄：册读不动，或按键还没有。可见性滞后是续办，重投会改变结果；
	// 不当毒丸拒收。
	ErrFormedIntakeNotVisible = errors.New(
		"visibility exception transportfulfillment adapter: the formed node intake cannot be read")
	// ErrFormedIntakeInconsistent 表示读回的收寄指着另一个键、租户对不上、关联立不成追踪包裹引用：节点作业的收寄册已破，重读还是
	// 同一个结果。禁止进 WithUndecidedSentinels。
	ErrFormedIntakeInconsistent = errors.New(
		"visibility exception transportfulfillment adapter: the formed node intake disagrees with its key")
)

// BackfillIdentifiedMemberAdapter 是节点收寄形成信封上移动事实补派生的处理方（ADR-0299）：一件作业实物识别成功——收寄首次形成、
// 待识别经重核形成、身份冲突经处置形成，信封同一类——就回头找它随袋走过的移动事实，凡事实发生那一刻它正是袋里（逐层向上到段里
// 在控的那一件）的成员，就在它新认出的包裹上补一份。
//
// 要补，是因为移动事实那一路按事实那一刻的成员展开，当时身份不成立的成员没有可投的追踪包裹，入账不派生（trackedParcelsAt）；
// 而识别成功不会让那封移动事实信封重投，没有任何东西回头补，这件包裹的追踪上就永久缺这一条出发、移动或到达。
//
// 反向寻找分两步，让正向展开与反向寻找永远是同一条成员口径：先向节点作业问这件实物曾被装进哪几件单元、那些单元又曾被装进哪几件
// 单元（候选，不问哪一刻），再对每件候选单元走 movementBackfill——按它有参与的段、按段声明的在册事实、事实发生那一刻它在控的，
// 用 trackedParcelsAt 展开它那一刻的成员，展开到的恰是这件包裹的才补。候选只放宽、展开才收紧，所以袋拆了之后才识别成功、成员早
// 已移出、嵌套的袋，都由同一处判断，不在这里再算一遍时间区间。
//
// 补的是这一封信指的这件包裹，不补袋里别的成员：同袋别的成员各有各的识别信封，或早已投过。只补投影，不补信号、不重问关务，同
// ADR-0224 决定五。判据与幂等键就是移动事实那一路的（deriveMovementFactOnto），补过的再被正常路径派生、或正常路径派生过的再被补、
// 信封重投，都只答已有结果。
//
// 一封信指的包裹以信封那一刻的收寄为准，展开却取此刻的收寄身份：实物的身份后来被纠正成另一件包裹时，这封旧信展开不到它原先认出的
// 那一件，什么也不补，新认出的那一件由它自己的信封补；先前已补到旧包裹上的那一份不撤，投影不使任何源事实失效（ADR-0299）。
type BackfillIdentifiedMemberAdapter struct {
	identified veports.IdentifiedMemberView
	backfill   movementBackfill
}

func NewBackfillIdentifiedMemberAdapter(
	identified veports.IdentifiedMemberView,
	segments ObjectSegmentFinder,
	facts SegmentMovementFactLister,
	members veports.ConsolidationMembershipView,
	derive MovementProjectionHandler,
) (*BackfillIdentifiedMemberAdapter, error) {
	if identified == nil {
		return nil, fmt.Errorf("visibility exception transportfulfillment adapter: identified member view is nil")
	}
	if segments == nil {
		return nil, fmt.Errorf("visibility exception transportfulfillment adapter: segment finder is nil")
	}
	if facts == nil {
		return nil, fmt.Errorf("visibility exception transportfulfillment adapter: movement fact lister is nil")
	}
	if members == nil {
		return nil, fmt.Errorf("visibility exception transportfulfillment adapter: consolidation membership view is nil")
	}
	if derive == nil {
		return nil, fmt.Errorf("visibility exception transportfulfillment adapter: projection handler is nil")
	}
	return &BackfillIdentifiedMemberAdapter{
		identified: identified,
		backfill:   movementBackfill{segments: segments, facts: facts, members: members, derive: derive},
	}, nil
}

var _ veinbox.FormedNodeIntakeHandler = (*BackfillIdentifiedMemberAdapter)(nil)

// HandleFormedNodeIntake 按信封键回读收寄；它没有认出包裹（待识别、未形成、形成了却没有关联）就入账收工。认出了，就找这件实物
// 随过的每一件单元（含嵌套向上），在每一件单元上补派生到这件包裹。
//
// 一封信一笔事务，失败格与别的补派生同口径：任一处读不动或派生未决即整封回滚重投；册上的确定性问题记发布失败，不当未决。绝大多数
// 收寄形成的实物从没被装进过任何袋，那一封只多一次反向读，其余一概不读。
func (adapter *BackfillIdentifiedMemberAdapter) HandleFormedNodeIntake(ctx context.Context, formed veinbox.FormedNodeIntake) error {
	veTenant, err := vedomain.NewTenantID(formed.TenantID)
	if err != nil {
		return fmt.Errorf("%w: tenant: %v", ErrMovementFactUntranslatableAnswer, err)
	}
	tfTenant, err := tfdomain.NewTenantID(formed.TenantID)
	if err != nil {
		return fmt.Errorf("%w: tenant: %v", ErrMovementFactUntranslatableAnswer, err)
	}
	identity, err := adapter.identified.FormedIntakeIdentity(ctx, veTenant, formed.SourceID)
	if err != nil {
		if errors.Is(err, veports.ErrFormedIntakeUntranslatable) {
			return fmt.Errorf("%w: source %q: %v", ErrFormedIntakeInconsistent, formed.SourceID, err)
		}
		return fmt.Errorf("%w: source %q: %v", ErrFormedIntakeNotVisible, formed.SourceID, err)
	}
	if !identity.Identified {
		return nil
	}
	units, err := adapter.unitsAbove(ctx, veTenant, identity.Handling)
	if err != nil {
		return err
	}
	for _, unit := range units {
		object, err := tfdomain.NewCarriedObjectReference(unit)
		if err != nil {
			return fmt.Errorf("%w: unit %q holding %q: %v", ErrConsolidationMembershipInconsistent, unit, identity.Handling, err)
		}
		if err := adapter.backfill.backfillObject(ctx, tfTenant, object, identity.Parcel.String()); err != nil {
			return err
		}
	}
	return nil
}

// unitsAbove 由这件实物逐层向上列出它曾被装进的每一件单元，宽度优先、每件只问一次：嵌套的袋同样可以被别的袋装过，成环的册也不会
// 绕不出来。次序取读口交回的次序，只影响读的先后，不影响结论。
func (adapter *BackfillIdentifiedMemberAdapter) unitsAbove(
	ctx context.Context,
	tenant vedomain.TenantID,
	handling string,
) ([]string, error) {
	seen := map[string]bool{handling: true}
	queue := []string{handling}
	var units []string
	for len(queue) > 0 {
		member := queue[0]
		queue = queue[1:]
		holders, err := adapter.identified.UnitsEverHolding(ctx, tenant, member)
		if err != nil {
			if errors.Is(err, veports.ErrConsolidationMembershipUntranslatable) {
				return nil, fmt.Errorf("%w: member %q: %v", ErrConsolidationMembershipInconsistent, member, err)
			}
			return nil, fmt.Errorf("%w: member %q: %v", ErrConsolidationMembershipNotVisible, member, err)
		}
		for _, holder := range holders {
			if seen[holder] {
				continue
			}
			seen[holder] = true
			units = append(units, holder)
			queue = append(queue, holder)
		}
	}
	return units, nil
}
