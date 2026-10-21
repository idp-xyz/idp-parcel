// Package nodeoperations 是 transport-fulfillment 消费 node-operations 的适配器（ADR-0025 消费方侧）：出发门禁读数展开集运单元时问
// 「这一件此刻是不是一袋、那一刻袋里直接装着什么、每件认出是哪件包裹」（ADR-0225）。只翻译不判断——成员按什么时间算、身份怎么
// 认，都由节点作业答；本包只把答复译成本上下文的载运对象引用。
package nodeoperations

import (
	"context"
	"fmt"
	"time"

	nodomain "go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	noports "go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	tfports "go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// MembershipAtReader 是节点作业集运成员关系按时点的那一问；真实装配交给 nodeoperations/adapters/postgres.ConsolidationMembershipsAt。
type MembershipAtReader interface {
	ReadConsolidationMembershipAt(
		ctx context.Context,
		tenant nodomain.TenantID,
		object string,
		at time.Time,
	) (noports.ConsolidationMembershipAt, error)
}

// IntakeIdentityReader 是节点作业按作业实物读收寄册的那一问；真实装配交给 nodeoperations/adapters/postgres.IntakeIdentities。
type IntakeIdentityReader interface {
	FindIntakeIdentities(
		ctx context.Context,
		tenant nodomain.TenantID,
		unit nodomain.HandlingUnitID,
	) ([]nodomain.IntakeIdentityObservation, error)
}

// ConsolidationMembershipView 实现 tfports.ConsolidationMembershipView。
type ConsolidationMembershipView struct {
	memberships MembershipAtReader
	identities  IntakeIdentityReader
}

// NewConsolidationMembershipView 两口都必须在：缺了哪一口，每一条带袋的出发都只能记读不动，一个装配漏接会长成一串看似依赖故障的
// 读数。
func NewConsolidationMembershipView(memberships MembershipAtReader, identities IntakeIdentityReader) (*ConsolidationMembershipView, error) {
	if memberships == nil || identities == nil {
		return nil, fmt.Errorf("transport fulfillment nodeoperations adapter: the membership and the intake identity readers are both required")
	}
	return &ConsolidationMembershipView{memberships: memberships, identities: identities}, nil
}

var _ tfports.ConsolidationMembershipView = (*ConsolidationMembershipView)(nil)

// MembersAt 问节点作业这件对象在出发那一刻的直接成员。时点按本上下文的登记精度交过去：出发登记与出发前问询拿同一时点问到的成员
// 必须相同，不能随调用方带进来的纳秒漂。
//
// 嵌套成员的标识就是那件单元的标识，原串译成载运对象引用；其余成员各问一次身份，由节点作业的 JudgeIdentityBasis 认——与它给移入
// 记方向依据时的身份一格同一个读法。成立即那件包裹；缺少（待识别、身份冲突、没有收寄记录）即身份不成立；读不动或身份链说不准（
// JudgeIdentityBasis 报错）即身份读不动，与节点作业自己的方向依据判断同一待遇。
func (view *ConsolidationMembershipView) MembersAt(
	ctx context.Context,
	query tfports.ConsolidationMembershipQuery,
) (tfports.ConsolidationMembership, error) {
	none := tfports.ConsolidationMembership{}
	tenant, err := nodomain.NewTenantID(query.TenantID.String())
	if err != nil {
		return none, fmt.Errorf("%w: tenant: %v", tfports.ErrConsolidationMembershipUntranslatable, err)
	}
	membership, err := view.memberships.ReadConsolidationMembershipAt(ctx, tenant, query.Object.String(),
		tfdomain.AtRegistryPrecision(query.At))
	if err != nil {
		return none, fmt.Errorf("read consolidation membership: node operations: %w", err)
	}
	if !membership.Unit {
		return tfports.ConsolidationMembership{}, nil
	}
	answer := tfports.ConsolidationMembership{Unit: true, Members: []tfports.ConsolidationMember{}}
	for _, member := range membership.Members {
		translated, err := view.member(ctx, tenant, member)
		if err != nil {
			return none, err
		}
		answer.Members = append(answer.Members, translated)
	}
	return answer, nil
}

func (view *ConsolidationMembershipView) member(
	ctx context.Context,
	tenant nodomain.TenantID,
	member noports.ConsolidationMemberAt,
) (tfports.ConsolidationMember, error) {
	handling := member.Member.String()
	if member.NestedUnit {
		object, err := carriedObject(handling)
		if err != nil {
			return tfports.ConsolidationMember{}, err
		}
		return tfports.ConsolidationMember{Kind: tfports.ConsolidationMemberUnit, Handling: handling, Object: object}, nil
	}
	unreadable := tfports.ConsolidationMember{Kind: tfports.ConsolidationMemberIdentityUnreadable, Handling: handling}
	observations, err := view.identities.FindIntakeIdentities(ctx, tenant, member.Member)
	if err != nil {
		return unreadable, nil
	}
	identity, err := nodomain.JudgeIdentityBasis(observations)
	if err != nil {
		return unreadable, nil
	}
	switch identity.Cell().Answer() {
	case nodomain.BasisEstablished:
		parcel, _ := identity.Association()
		object, err := carriedObject(parcel.String())
		if err != nil {
			return tfports.ConsolidationMember{}, err
		}
		return tfports.ConsolidationMember{Kind: tfports.ConsolidationMemberParcel, Handling: handling, Object: object}, nil
	case nodomain.BasisMissing:
		return tfports.ConsolidationMember{Kind: tfports.ConsolidationMemberIdentityUnresolved, Handling: handling}, nil
	case nodomain.BasisUnreadable:
		return unreadable, nil
	default:
		return tfports.ConsolidationMember{}, fmt.Errorf("%w: member %q identity answered %q",
			tfports.ErrConsolidationMembershipUntranslatable, handling, identity.Cell().Answer())
	}
}

func carriedObject(raw string) (tfdomain.CarriedObjectReference, error) {
	object, err := tfdomain.NewCarriedObjectReference(raw)
	if err != nil {
		return tfdomain.CarriedObjectReference{}, fmt.Errorf("%w: member %q: %v", tfports.ErrConsolidationMembershipUntranslatable, raw, err)
	}
	return object, nil
}
