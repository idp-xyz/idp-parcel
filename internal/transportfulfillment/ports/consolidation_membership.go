package ports

import (
	"context"
	"errors"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 出发门禁读数展开集运单元要问的那一口（ADR-0225）。另立文件而不并进 departure_gate.go：它问的是 node-operations，不是关务。

// ConsolidationMembershipQuery 问一件载运对象在某一时点是不是集运单元、那一刻直接装着什么。
type ConsolidationMembershipQuery struct {
	TenantID domain.TenantID
	Object   domain.CarriedObjectReference
	At       time.Time
}

// ConsolidationMemberKind 是一件直接成员在本上下文认出的样子。
type ConsolidationMemberKind uint8

const (
	ConsolidationMemberKindInvalid ConsolidationMemberKind = iota
	// ConsolidationMemberParcel：认出了正式包裹，Object 是那件包裹的引用，可以拿去问关务。
	ConsolidationMemberParcel
	// ConsolidationMemberUnit：成员本身是集运单元，Object 是它的引用，要再展开一层。
	ConsolidationMemberUnit
	// ConsolidationMemberIdentityUnresolved：成员的正式包裹身份此刻不成立（待识别、身份冲突、没有收寄记录），没有可问关务的对象。
	ConsolidationMemberIdentityUnresolved
	// ConsolidationMemberIdentityUnreadable：成员的身份那一问读不动。
	ConsolidationMemberIdentityUnreadable
)

// ConsolidationMember 是一件直接成员。Handling 是 node-operations 的作业实物标识原文，四种都带：问不到关务的那几件只能凭它
// 指名；Object 只在包裹与单元两种上有。
type ConsolidationMember struct {
	Kind     ConsolidationMemberKind
	Handling string
	Object   domain.CarriedObjectReference
}

// ConsolidationMembership 是一次问询的答复。Unit 为假即这件对象不是集运单元（Members 恒空），按包裹问关务；为真而 Members 为空，
// 是那一刻这件单元不装任何东西。
type ConsolidationMembership struct {
	Unit    bool
	Members []ConsolidationMember
}

// ConsolidationMembershipView 是 node-operations 集运成员关系的消费侧端口（ADR-0025）。实现只翻译：成员是谁、那一刻在不在、身份
// 认没认出来，都是 node-operations 的判断，本上下文不重判。
//
// error 说这一问没有形成答复——不能读成「不是单元」：那等于把一袋当成一件包裹去问关务，正是 ADR-0225 要治的假干净读数。编排把它
// 记成那一件读不动；答复译不回来的一类包住 ErrConsolidationMembershipUntranslatable 交出，与调用失败分记依据种类。
type ConsolidationMembershipView interface {
	MembersAt(ctx context.Context, query ConsolidationMembershipQuery) (ConsolidationMembership, error)
}

// ErrConsolidationMembershipUntranslatable：node-operations 答了，却译不回本上下文认得的形状（成员标识立不成载运对象引用、身份那一格
// 落在封闭集外），或本上下文的值在那边立不起来。重问也是同一个结果，要查两边的口径，不是等那边恢复。
var ErrConsolidationMembershipUntranslatable = errors.New("transport fulfillment: the consolidation membership answer cannot be translated")
