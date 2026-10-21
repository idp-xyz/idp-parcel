package ports

import (
	"context"
	"errors"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 实际履约未形成册的存取口（ADR-0231 决定六）。补形成那一处答「这一次不形成」时立或刷新一条，之后形成了或认出已被盖住时了结为已形成，
// 那一版改说不计费时了结为不计费；写它的只有补形成那一处（自动形成与采用、换版之后的补形成共用的那一个后半），读它的是查阅页。

// ErrActualFulfillmentUnformedWriteRefused 是册的写口给出的确定性拒绝：没有环境事务、库上的约束不让写。原样重来一万次都一样，调用方
// 不该把它当成「册此刻读写不动」去重投；别的写入失败不带它。
var ErrActualFulfillmentUnformedWriteRefused = errors.New("transport fulfillment: actual fulfillment unformed write refused")

// ActualFulfillmentUnformedKey 是一件未形成的身份：哪个租户的哪个对象在哪个段里。一条参与只结束一次，同段同对象只有一条替代链。
type ActualFulfillmentUnformedKey struct {
	TenantID domain.TenantID
	Segment  domain.FulfillmentSegmentReference
	Object   domain.CarriedObjectReference
}

// ActualFulfillmentUnformedReason 是「这一次不形成」的那一格，封闭四值，字面与登记用例答复的那一格相同。
type ActualFulfillmentUnformedReason uint8

const (
	ActualFulfillmentUnformedReasonInvalid ActualFulfillmentUnformedReason = iota
	// UnformedNotConfigured：委托那一版没声明实际履约的收费发生条件。续办是租户发一版声明了条件的协议，再给委托换版。
	UnformedNotConfigured
	// UnformedAgreementNotEffective：委托那一版在参与终点未生效（截止点之后，或还没开始）。
	UnformedAgreementNotEffective
	// UnformedAgreementTerminated：委托那一版已终止或退役。
	UnformedAgreementTerminated
	// UnformedProcurementContextMissing：委托提交时没记下协议版本，也没换过版。续办是给委托换版、记下版本。
	UnformedProcurementContextMissing
	// UnformedFormationDefect：补形成那一路上这一件撞上数据缺陷，原样重来不会变；哪几种算数据缺陷只由 application 的 formationDefectOf
	// 认。记进册、同一封门铃里的其余各件照常形成；续办是修好缺陷之后再给委托换版或重新采用，让补形成再问一次。
	UnformedFormationDefect
)

func (reason ActualFulfillmentUnformedReason) String() string {
	switch reason {
	case UnformedNotConfigured:
		return "NOT_CONFIGURED"
	case UnformedAgreementNotEffective:
		return "AGREEMENT_NOT_EFFECTIVE"
	case UnformedAgreementTerminated:
		return "AGREEMENT_TERMINATED"
	case UnformedProcurementContextMissing:
		return "PROCUREMENT_CONTEXT_MISSING"
	case UnformedFormationDefect:
		return "FORMATION_DEFECT"
	default:
		return ""
	}
}

// ActualFulfillmentUnformedState 是册上一条的状态。
type ActualFulfillmentUnformedState uint8

const (
	ActualFulfillmentUnformedStateInvalid ActualFulfillmentUnformedState = iota
	// ActualFulfillmentUnformed：最近一次答复是「这一次不形成」。
	ActualFulfillmentUnformed
	// ActualFulfillmentUnformedFormed：之后某一次形成了，或认出已有一项仍有效的发生项盖住它。
	ActualFulfillmentUnformedFormed
	// ActualFulfillmentUnformedNotCharged：之后某一次作答的那一版协议明说不计费。
	ActualFulfillmentUnformedNotCharged
)

func (state ActualFulfillmentUnformedState) String() string {
	switch state {
	case ActualFulfillmentUnformed:
		return "UNFORMED"
	case ActualFulfillmentUnformedFormed:
		return "FORMED"
	case ActualFulfillmentUnformedNotCharged:
		return "NOT_CHARGED"
	default:
		return ""
	}
}

// ActualFulfillmentUnformedEntry 是一次「这一次不形成」的答复：在哪份委托下按第几版答的、参与哪一刻结束、哪一格、问的是哪一版协议
// （缺采购上下文那一格没问，Agreement 为零值）。缺陷那一格另带 Defect（类别码连同完整错误原文）；它停在登记用例答复之前，Revision
// 为零、Agreement 为零值。
type ActualFulfillmentUnformedEntry struct {
	Key        ActualFulfillmentUnformedKey
	Commission domain.TransportCommissionReference
	Revision   int
	EndedAt    time.Time
	Reason     ActualFulfillmentUnformedReason
	Agreement  domain.SupplierAgreementVersion
	Defect     string
}

// ActualFulfillmentUnformedRecord 是册上一条。时刻都是答复那一刻的登记时刻（派发时钟），只有 EndedAt 是业务时间。
type ActualFulfillmentUnformedRecord struct {
	Entry           ActualFulfillmentUnformedEntry
	FirstAnsweredAt time.Time
	LastAnsweredAt  time.Time
	State           ActualFulfillmentUnformedState
	// Occurrence 只在已形成时给出：盖住它的那一项发生项。
	Occurrence ChargeOccurrenceKey
	// NotChargedBy 只在不计费时给出：作答的那一版协议快照。
	NotChargedBy string
	SettledAt    time.Time
}

// ActualFulfillmentUnformedRegister 是补形成写册的各口，都要环境事务：册上那一行与那一拍的其余写入同笔。
//
//   - RecordUnformed：立一条，或把任一状态的那一条刷新为未形成，各格取这一次；首次答复时刻只在立的那一刻写。已形成的也刷新：调用方
//     只在确认没有一项仍有效的发生项盖住它之后才记未形成，盖住它的那一项此后失效了，它就又是未形成的。
//   - SettleFormed：把一条改成已形成；册上没有这一件就什么也不写——册只记未形成过的那几件，顺利形成的不进册。
//   - SettleNotCharged：同上，改成不计费。
type ActualFulfillmentUnformedRegister interface {
	RecordUnformed(ctx context.Context, entry ActualFulfillmentUnformedEntry, answeredAt time.Time) error
	SettleFormed(ctx context.Context, key ActualFulfillmentUnformedKey, occurrence ChargeOccurrenceKey, at time.Time) error
	SettleNotCharged(ctx context.Context, key ActualFulfillmentUnformedKey, agreement domain.AgreementSnapshotReference, at time.Time) error
}
