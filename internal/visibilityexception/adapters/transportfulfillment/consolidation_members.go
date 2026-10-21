package transportfulfillment

import (
	"context"
	"errors"
	"fmt"
	"time"

	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	vedomain "go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	veports "go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

var (
	// ErrConsolidationMembershipNotVisible 表示问节点作业「这一件那一刻是不是一袋、袋里装着什么、成员是哪件包裹」读不动。说不出
	// 是不是袋就不能当包裹投，也不能当空袋入账；读得动之后重投，展开从头再走一遍，已派生过的答已有结果。续办格。
	ErrConsolidationMembershipNotVisible = errors.New(
		"visibility exception transportfulfillment adapter: the consolidation membership cannot be read")
	// ErrConsolidationMembershipInconsistent 表示节点作业答了，答复却立不成成员：译不回来、成员的身份链判不下来、成员种类落在
	// 封闭集合外、成员引用立不成追踪包裹。重问还是同一个结果，要查节点作业的册或两边的口径。禁止进 WithUndecidedSentinels。
	ErrConsolidationMembershipInconsistent = errors.New(
		"visibility exception transportfulfillment adapter: the consolidation membership answer is inconsistent")
)

// trackedParcelsAt 把事实那一刻在控的对象展开成可投的追踪包裹。不是集运单元的那一件就是一件包裹；是单元的，投到它那一刻的直接
// 成员身上，嵌套的再展开一层，直到包裹为止——袋本身不是追踪包裹，集运关系不合并包裹身份和客户轨迹（CONTEXT）。
//
// 成员取事实发生那一刻，不取消费这一刻：离线补登或迟到消费时袋可能已经拆了，与出发门禁读数展开成员同一口径（ADR-0225 决定二）。
// 每个引用——段里的与成员认出的——都先问一次是不是单元：包裹号与某件单元同串时按单元认（ADR-0225 决定四），先访问到的一侧不替
// 后到的一侧定种类。每个引用只问一次、只投一次：同一件既直接在段里又在某一袋里、几袋同时指名它、成员关系册成了环，都收成一份。
// 读数要给环记一格缘由，投影只要成员集合，环上那一边不多出任何成员，跳过即可。
//
// 身份不成立的成员（待识别、身份冲突、没有收寄记录）没有可投的追踪包裹，入账不派生、不挡其余各件：整封停在未决等它识别，袋里其余
// 成员的轨迹就都跟着停住，而识别不会回头让这封信重投。先全部展开再派生，一处读不动就一份也不派生。
func trackedParcelsAt(
	ctx context.Context,
	members veports.ConsolidationMembershipView,
	tenant tfdomain.TenantID,
	objects []tfdomain.CarriedObjectReference,
	at time.Time,
) ([]vedomain.TrackedParcelReference, error) {
	veTenant, err := vedomain.NewTenantID(tenant.String())
	if err != nil {
		return nil, fmt.Errorf("%w: tenant: %v", ErrMovementFactUntranslatableAnswer, err)
	}
	expansion := &memberExpansion{members: members, tenant: veTenant, at: at, visited: map[string]bool{}}
	for _, object := range objects {
		parcel, err := vedomain.NewTrackedParcelReference(object.String())
		if err != nil {
			return nil, fmt.Errorf("%w: carried object: %v", ErrMovementFactUntranslatableAnswer, err)
		}
		if err := expansion.walk(ctx, parcel); err != nil {
			return nil, err
		}
	}
	return expansion.parcels, nil
}

type memberExpansion struct {
	members veports.ConsolidationMembershipView
	tenant  vedomain.TenantID
	at      time.Time
	visited map[string]bool
	parcels []vedomain.TrackedParcelReference
}

// walk 收的引用先立成追踪包裹引用再问：它若不是单元就要按这个引用投，立不成的不该先去问节点作业。
func (expansion *memberExpansion) walk(ctx context.Context, object vedomain.TrackedParcelReference) error {
	if expansion.visited[object.String()] {
		return nil
	}
	expansion.visited[object.String()] = true
	membership, err := expansion.members.MembersAt(ctx, veports.ConsolidationMembershipQuery{
		TenantID: expansion.tenant, Object: object.String(), At: expansion.at,
	})
	if errors.Is(err, veports.ErrConsolidationMembershipUntranslatable) {
		return fmt.Errorf("%w: object %q: %v", ErrConsolidationMembershipInconsistent, object, err)
	}
	if err != nil {
		return fmt.Errorf("%w: object %q: %v", ErrConsolidationMembershipNotVisible, object, err)
	}
	if !membership.Unit {
		expansion.parcels = append(expansion.parcels, object)
		return nil
	}
	for _, member := range membership.Members {
		switch member.Kind {
		case veports.ConsolidationMemberParcel, veports.ConsolidationMemberUnit:
			next, err := vedomain.NewTrackedParcelReference(member.Object)
			if err != nil {
				return fmt.Errorf("%w: unit %q member %q: %v", ErrConsolidationMembershipInconsistent, object, member.Handling, err)
			}
			if err := expansion.walk(ctx, next); err != nil {
				return err
			}
		case veports.ConsolidationMemberIdentityUnresolved:
			// 身份不成立：没有可投的追踪包裹，见 trackedParcelsAt。
		default:
			return fmt.Errorf("%w: unit %q member %q kind %d", ErrConsolidationMembershipInconsistent, object, member.Handling, member.Kind)
		}
	}
	return nil
}
