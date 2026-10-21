package ports

import (
	"context"
	"errors"
	"time"

	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
)

// 本文件是全程追踪认集运单元时要问的那一口：一件对象那一刻若是一袋，事实不投到袋上（CONTEXT「集运、装载或同批关系不合并包裹身份
// 和客户轨迹」）；移动事实另投到袋里的成员身上。是不是单元、成员是谁只有 node-operations 的集运成员关系册说得出，正式包裹身份只有
// 它的收寄册说得出，本上下文只按引用问，不重判。

// ConsolidationMembershipQuery 问一件对象在某一时点是不是集运单元、那一刻直接装着什么。Object 是源上下文交来的对象引用原文：
// 本上下文此刻还不知道它是包裹还是单元，所以不先立成追踪包裹引用。
type ConsolidationMembershipQuery struct {
	TenantID domain.TenantID
	Object   string
	At       time.Time
}

// ConsolidationMemberKind 是一件直接成员在本上下文认出的样子。
type ConsolidationMemberKind uint8

const (
	ConsolidationMemberKindInvalid ConsolidationMemberKind = iota
	// ConsolidationMemberParcel：认出了正式包裹，Object 是那件包裹的引用原文。它与某件单元同串时仍按单元认（与出发门禁读数同一
	// 判据），所以调用方照样先问它是不是单元。
	ConsolidationMemberParcel
	// ConsolidationMemberUnit：成员本身是集运单元，Object 是它的引用原文，要再展开一层。
	ConsolidationMemberUnit
	// ConsolidationMemberIdentityUnresolved：成员的正式包裹身份此刻不成立（待识别、身份冲突、没有收寄记录），没有可投的追踪包裹。
	ConsolidationMemberIdentityUnresolved
)

// ConsolidationMember 是一件直接成员。Handling 是 node-operations 的作业实物标识原文，三种都带；Object 只在包裹与单元两种上有。
type ConsolidationMember struct {
	Kind     ConsolidationMemberKind
	Handling string
	Object   string
}

// ConsolidationMembership 是一次问询的答复。Unit 为假即这件对象不是集运单元（Members 恒空）；为真而 Members 为空，是那一刻这件
// 单元不装任何东西。
type ConsolidationMembership struct {
	Unit    bool
	Members []ConsolidationMember
}

// ConsolidationMembershipView 是 node-operations 集运成员关系与收寄身份的消费侧端口（ADR-0025）。
//
// error 说这一问没有形成答复，不能读成「不是单元」：那等于把一袋当成一件包裹投，正是本口要治的那一格。成员关系或收寄身份那一问
// 调用失败原样交回；节点作业答了、却立不成本口的形状（成员标识不是合法引用、身份链本身判不下来、身份那一格落在封闭集外）包住
// ErrConsolidationMembershipUntranslatable 交回——重问还是同一个结果。
//
// IsUnitAt 只答 MembersAt 答复里的 Unit 那一格，不认成员身份。只要知道「这一件是不是单元」的一路用它：成员身份读不动或判不下来，
// 与这一路要的答复无关，不该让它停在未决或记发布失败。两问的判据是节点作业同一本单元册，同串的包裹号都按单元认。
type ConsolidationMembershipView interface {
	MembersAt(ctx context.Context, query ConsolidationMembershipQuery) (ConsolidationMembership, error)
	IsUnitAt(ctx context.Context, query ConsolidationMembershipQuery) (bool, error)
}

// ErrConsolidationMembershipUntranslatable 见 ConsolidationMembershipView。确定性：要查两边的口径或节点作业的册，不是等那边恢复。
var ErrConsolidationMembershipUntranslatable = errors.New(
	"visibility exception: the consolidation membership answer cannot be translated")
