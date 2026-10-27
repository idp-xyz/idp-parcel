package transportfulfillment

import (
	"context"
	"errors"
	"fmt"
	"time"

	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/veconsume"
	veapplication "go.idp.xyz/idp-parcel/internal/visibilityexception/application"
	vedomain "go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	veports "go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

var (
	// ErrConsolidationMembershipNotVisible 表示问节点作业「这一件那一刻是不是一袋、袋里装着什么、成员是哪件包裹」，或「这件实物曾被
	// 装进哪几件单元」读不动。说不出是不是袋就不能当包裹投，也不能当空袋入账；说不出曾随过哪几袋就不能认定它没有要补的移动；读得动
	// 之后重投，从头再问一遍，已派生过的答已有结果。续办格。
	ErrConsolidationMembershipNotVisible = errors.New(
		"visibility exception transportfulfillment adapter: the consolidation membership cannot be read")
	// ErrConsolidationMembershipInconsistent 表示节点作业答了，答复却立不成成员：译不回来、成员的身份链判不下来、成员种类落在
	// 封闭集合外、成员引用立不成追踪包裹、曾装它的单元标识立不成对象引用。重问还是同一个结果，要查节点作业的册或两边的口径。禁止进
	// WithUndecidedSentinels。
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
	membership, err := expansion.members.MembersAt(ctx, membershipQuery(expansion.tenant, object, expansion.at))
	if err != nil {
		return membershipFailure(object, err)
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

// deriveUnlessConsolidationUnit 先问命令上的对象在事实发生那一刻是不是集运单元：是就入账不派生，否则照常交给派生编排。单元不是
// 追踪包裹，不为它立投影（CONTEXT）；也不在这里投到成员上——交接与交付对成员意味着控制随单元转移，场外揽收同样是取得控制，单元
// 控制规则立起来之前运输履约不为成员形成参与或控制判断（ADR-0263 决定四），投影先投给成员，追踪上就有一份运输履约自己都没认的成员
// 控制；外部承运轨迹说的是单元本身，封签不符时不得据原成员快照推出成员已经到达（同一决定），成员也等那条规则。
//
// 时点取命令的业务发生时间，与派生进投影的那一格同一个。判据与 trackedParcelsAt 同一本单元册（同串的包裹号按单元认），失败分格也
// 同一处（membershipFailure）；只是问 IsUnitAt、不认成员身份：此刻不投到成员上，成员身份那一侧的故障与这一问的答复无关。
func deriveUnlessConsolidationUnit(
	ctx context.Context,
	members veports.ConsolidationMembershipView,
	derive ProjectionHandler,
	command veapplication.DeriveProjectionCommand,
) error {
	unit, err := members.IsUnitAt(ctx, membershipQuery(command.TenantID, command.Fact.Parcel, command.Fact.OccurredAt))
	if err != nil {
		return membershipFailure(command.Fact.Parcel, err)
	}
	if unit {
		return nil
	}
	result, err := derive.Handle(ctx, command)
	if err != nil {
		return err
	}
	return veconsume.Consumption(result)
}

func membershipQuery(
	tenant vedomain.TenantID,
	object vedomain.TrackedParcelReference,
	at time.Time,
) veports.ConsolidationMembershipQuery {
	return veports.ConsolidationMembershipQuery{TenantID: tenant, Object: object.String(), At: at}
}

// membershipFailure 把节点作业那一问的失败分成两格：答了却立不成本口形状是确定性的，其余都是读不动、续办。
func membershipFailure(object vedomain.TrackedParcelReference, err error) error {
	if errors.Is(err, veports.ErrConsolidationMembershipUntranslatable) {
		return fmt.Errorf("%w: object %q: %v", ErrConsolidationMembershipInconsistent, object, err)
	}
	return fmt.Errorf("%w: object %q: %v", ErrConsolidationMembershipNotVisible, object, err)
}
