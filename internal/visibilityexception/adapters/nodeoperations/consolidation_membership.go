package nodeoperations

import (
	"context"
	"fmt"
	"time"

	nodomain "go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	noports "go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
	veports "go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
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

// ConsolidationMembershipView 实现 veports.ConsolidationMembershipView：全程追踪按集运单元成员投移动事实时，问节点作业「这一件
// 那一刻是不是一袋、袋里直接装着什么、每件认出是哪件包裹」。只翻译——成员按什么时间算、身份怎么认，都由节点作业答。
type ConsolidationMembershipView struct {
	memberships MembershipAtReader
	identities  IntakeIdentityReader
}

// NewConsolidationMembershipView 两口都必须在：缺了哪一口，每一条带袋的移动事实都只能停在未决，一个装配漏接会长成一串看似依赖
// 故障的重投。
func NewConsolidationMembershipView(memberships MembershipAtReader, identities IntakeIdentityReader) (*ConsolidationMembershipView, error) {
	if memberships == nil || identities == nil {
		return nil, fmt.Errorf("visibility exception nodeoperations adapter: the membership and the intake identity readers are both required")
	}
	return &ConsolidationMembershipView{memberships: memberships, identities: identities}, nil
}

var _ veports.ConsolidationMembershipView = (*ConsolidationMembershipView)(nil)

// MembersAt 问节点作业这件对象在时点那一刻的直接成员。嵌套成员的标识就是那件单元的标识，原串交回；其余成员各问一次身份，由
// 节点作业的 JudgeIdentityBasis 认——与出发门禁读数展开成员、与节点作业给移入记方向依据时同一个读法，一件实物「此刻是哪件包裹」
// 不在本上下文另立判据。
//
// 身份一格缺少（待识别、身份冲突、没有收寄记录）即身份不成立，作为成员的一种交回；收寄册读不动作为错误交回。身份链本身判不下来
// （JudgeIdentityBasis 报错）是节点作业册上的确定性问题，重问也是同一个结果，包住 ErrConsolidationMembershipUntranslatable——出发
// 门禁读数把它与读不动同记一格，是因为读数只需要说「这一件说不准」，这里要决定的却是重投还是收工。
func (view *ConsolidationMembershipView) MembersAt(
	ctx context.Context,
	query veports.ConsolidationMembershipQuery,
) (veports.ConsolidationMembership, error) {
	none := veports.ConsolidationMembership{}
	tenant, err := nodomain.NewTenantID(query.TenantID.String())
	if err != nil {
		return none, fmt.Errorf("%w: tenant: %v", veports.ErrConsolidationMembershipUntranslatable, err)
	}
	membership, err := view.memberships.ReadConsolidationMembershipAt(ctx, tenant, query.Object, query.At)
	if err != nil {
		return none, fmt.Errorf("read consolidation membership: node operations: %w", err)
	}
	if !membership.Unit {
		return none, nil
	}
	answer := veports.ConsolidationMembership{Unit: true, Members: []veports.ConsolidationMember{}}
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
) (veports.ConsolidationMember, error) {
	handling := member.Member.String()
	if handling == "" {
		return veports.ConsolidationMember{}, fmt.Errorf("%w: blank member", veports.ErrConsolidationMembershipUntranslatable)
	}
	if member.NestedUnit {
		return veports.ConsolidationMember{Kind: veports.ConsolidationMemberUnit, Handling: handling, Object: handling}, nil
	}
	observations, err := view.identities.FindIntakeIdentities(ctx, tenant, member.Member)
	if err != nil {
		return veports.ConsolidationMember{}, fmt.Errorf("read intake identity of member %q: node operations: %w", handling, err)
	}
	identity, err := nodomain.JudgeIdentityBasis(observations)
	if err != nil {
		return veports.ConsolidationMember{}, fmt.Errorf("%w: member %q identity: %v",
			veports.ErrConsolidationMembershipUntranslatable, handling, err)
	}
	switch identity.Cell().Answer() {
	case nodomain.BasisEstablished:
		parcel, _ := identity.Association()
		if parcel.String() == "" {
			return veports.ConsolidationMember{}, fmt.Errorf("%w: member %q established without a parcel",
				veports.ErrConsolidationMembershipUntranslatable, handling)
		}
		return veports.ConsolidationMember{Kind: veports.ConsolidationMemberParcel, Handling: handling, Object: parcel.String()}, nil
	case nodomain.BasisMissing:
		return veports.ConsolidationMember{Kind: veports.ConsolidationMemberIdentityUnresolved, Handling: handling}, nil
	default:
		return veports.ConsolidationMember{}, fmt.Errorf("%w: member %q identity answered %q",
			veports.ErrConsolidationMembershipUntranslatable, handling, identity.Cell().Answer())
	}
}
