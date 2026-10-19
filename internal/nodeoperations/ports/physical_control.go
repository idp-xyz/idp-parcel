package ports

import (
	"context"
	"errors"
	"time"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
)

// 本文件是节点控制按权威运输交接推进那几问（ADR-0214，UC-NO-005）：读回一版交接、认交接对象是哪件作业实物、读写节点控制转移册。
// 与 ReceptionStore 分立：收寄编排不转出控制，扩它的接口就要连带改所有只为收寄而写的替身。

// ErrAuthoritativeHandoverUnreadable 说明交接登记册此刻读不动：重投可能就过。它与「读回了却译不过来」分开——后者是提供方的
// 记录坏了或本上下文的缺陷，重投不会变。
var ErrAuthoritativeHandoverUnreadable = errors.New("node operations: authoritative transport handover is unreadable")

// AuthoritativeHandover 是按键读回的一版权威运输交接，用本上下文的语言（ADR-0025：提供方类型不进消费方 ports）。
//
// HandedOver 只在`已交接`时为真，TransferOut 只随它在场——拒收与待确认给不出转出引用。JudgedAt 是交接判断的业务时间，已折到
// 节点控制转移册的存储精度：转出前比较的与读回时折叠比较的是同一个时刻。Superseded 说这一版已被同链的后一版更正：晚到的前版
// 不再是这个对象的交接结果。
//
// ReleasingNode 与 ReceivingNode 是这一版声明的交出与接收节点（ADR-0213 决定一），零值即没有声明——缺席是一种答案，本上下文不补。
// 接收节点决定转入在哪个节点立控制；交出节点只用来与被转出那一份控制的节点比，不是转出的前提。更正承继前版的声明，同一条更正链上
// 各版的节点相同。
type AuthoritativeHandover struct {
	Reference     domain.AuthoritativeHandoverReference
	HandedOver    bool
	TransferOut   domain.TransferOutReference
	JudgedAt      time.Time
	Superseded    bool
	ReleasingNode domain.NodeReference
	ReceivingNode domain.NodeReference
}

// AuthoritativeHandoverSource 按（租户，交接键）读回一版权威运输交接；没有交回 false。读不动的错误包 ErrAuthoritativeHandoverUnreadable。
type AuthoritativeHandoverSource interface {
	FindAuthoritativeHandover(
		ctx context.Context,
		tenant domain.TenantID,
		reference domain.AuthoritativeHandoverReference,
	) (AuthoritativeHandover, bool, error)
}

// HandoverObjectKind 是交接对象在本上下文册上认出的种类。
type HandoverObjectKind uint8

const (
	HandoverObjectKindInvalid HandoverObjectKind = iota
	// HandoverObjectParcel：对象按正式包裹身份认。零件也在这一格——对象没对上任何一件作业实物，仍是按包裹认的答案。
	HandoverObjectParcel
	// HandoverObjectConsolidationUnit：对象是本租户的集运单元。与包裹号同串时也在这一格（ADR-0214 决定二）。
	HandoverObjectConsolidationUnit
)

func (kind HandoverObjectKind) String() string {
	switch kind {
	case HandoverObjectParcel:
		return "PARCEL"
	case HandoverObjectConsolidationUnit:
		return "CONSOLIDATION_UNIT"
	default:
		return ""
	}
}

// HandoverObjectResolution 是交接对象认出的结果。Units 只在按包裹认时有意义：此刻关联到这件包裹的各件作业实物，去重、定序。
type HandoverObjectResolution struct {
	Kind  HandoverObjectKind
	Units []domain.HandlingUnitID
}

// HandoverObjectResolver 认交接对象是本租户的集运单元，还是按正式包裹身份经本上下文册上的当前关联对上哪几件作业实物
// （ADR-0214 决定二）。当前关联只读各条判断链的当前有效版本：被重核、冲突或处置接续过的那一版上的关联不再算。读不动作为错误返回。
type HandoverObjectResolver interface {
	ResolveHandoverObject(
		ctx context.Context,
		tenant domain.TenantID,
		handover domain.AuthoritativeHandoverReference,
	) (HandoverObjectResolution, error)
}

// ControlTransitionRecord 是节点控制转移册上越过提交边界的一行：转移本身与本上下文的记录时刻。
type ControlTransitionRecord struct {
	Transition domain.ControlTransition
	RecordedAt time.Time
}

type ControlTransitionSaveOutcome uint8

const (
	ControlTransitionSaveOutcomeInvalid ControlTransitionSaveOutcome = iota
	ControlTransitionSaved
	ControlTransitionAlreadyRecorded
)

// PhysicalControlRegister 读写节点控制转移册（ADR-0214 决定三），并答一件作业实物此刻在身的各份控制。
type PhysicalControlRegister interface {
	// FindTransition 交回这一版交接在这个方向上已落的那一行；没有交回 false。
	FindTransition(
		ctx context.Context,
		tenant domain.TenantID,
		handover domain.AuthoritativeHandoverReference,
		direction domain.ControlTransitionDirection,
	) (ControlTransitionRecord, bool, error)
	// FindAdvancedInHandoverChain 交回同一条更正链（同一对象与范围，任一版）上在这个方向推进了的那一行；没有交回 false。一条链上
	// 至多推进一次，读到不止一行作为错误返回，不任取一行。
	FindAdvancedInHandoverChain(
		ctx context.Context,
		tenant domain.TenantID,
		handover domain.AuthoritativeHandoverReference,
		direction domain.ControlTransitionDirection,
	) (ControlTransitionRecord, bool, error)
	// FindActiveControls 交回这件作业实物此刻在身的各份控制：收寄成立的与交接转入成立的都算，已折进转出、按控制依据去重；没有交回
	// 空切片。
	//
	// 读之前按（租户，作业实物）取锁，与 ReceptionStore.FindCurrentJudgmentsAtNode 是同一把：转出与同一件实物的收寄串在一起，
	// 也与同一件实物上的另一次转出串在一起。要在转移落册所在的事务里调。
	FindActiveControls(
		ctx context.Context,
		tenant domain.TenantID,
		unit domain.HandlingUnitID,
	) ([]domain.PhysicalControl, error)
	// FindUnitControls 交回这件作业实物在本上下文有过的各份控制，在身的与已转出的都算，同样折进转出、按控制依据去重；没有交回空
	// 切片。转入要看最近一次转出落在什么时刻，只读在身的看不到（ADR-0214 决定七落地补记）。取锁与事务要求同 FindActiveControls：
	// 同一把实物锁，转入与同一件实物的收寄、转出串在一起。
	FindUnitControls(
		ctx context.Context,
		tenant domain.TenantID,
		unit domain.HandlingUnitID,
	) ([]domain.PhysicalControl, error)
	// Save 落一行。同一（租户，交接键，方向）已在册答 ControlTransitionAlreadyRecorded，不覆盖先到者。
	Save(ctx context.Context, record ControlTransitionRecord) (ControlTransitionSaveOutcome, error)
}

// PhysicalControlTransitionCatalogueRow 是节点控制转移册上列的一行（registry=physical-control），照行转写。Direction、Outcome、Reason
// 与 ControlKind 是封闭词原样透出；控制几格只在这一行带控制时在场，转出两格只在推进了的转出与转出一侧的待重算上在场。
// MismatchedReleasingNode 是推进了的转出上的信息格：交接声明的交出节点与被转出那一份控制的节点不符时，记声明的那一个。
type PhysicalControlTransitionCatalogueRow struct {
	HandoverObject          string
	HandoverScope           string
	HandoverVersion         string
	Direction               string
	Outcome                 string
	Reason                  string
	Unit                    string
	Node                    string
	ControlKind             string
	ControlBasis            string
	ControlEstablishedAt    *time.Time
	TransferReference       string
	TransferredAt           *time.Time
	MismatchedReleasingNode string
	OccurredAt              time.Time
	RecordedAt              time.Time
}
