package ports

import (
	"context"
	"errors"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// ConsolidationUnitView 是小包托运认「运输履约交来的载运对象是不是集运单元」的消费侧端口（ADR-0025）。
//
// 运输履约的载运对象引用不带种类，可能指正式包裹，也可能指集运单元。集运单元不是包裹（CONTEXT「包裹」）：它没有委托，不会有
// 已接受委托，也不形成采用或终局。是不是单元只有 node-operations 的集运单元册说得出，本上下文只按引用问，不另立串形或前缀规则；
// 包裹号与某件单元同串时按单元认，与节点作业认权威运输交接的对象、出发门禁读数展开成员同一判据（ADR-0214 决定二、ADR-0225
// 决定四）。
//
// parcel 是源上下文交来、尚未认出种类的对象引用原文，译成本上下文的包裹号只为能被问；at 是来源事实的业务发生时间，不是消费这一刻。
//
// error 说这一问没有形成答复，不能读成「不是单元」：那等于拿一袋去反查委托，正是本口要治的那一格。节点作业那一问调用失败原样
// 交回；节点作业答了、却立不成本口的形状（租户译不过去、答复自相矛盾）包住 ErrConsolidationUnitAnswerUntranslatable 交回——
// 重问还是同一个结果。
type ConsolidationUnitView interface {
	IsConsolidationUnitAt(
		ctx context.Context,
		tenant domain.TenantID,
		parcel domain.DeclaredParcelID,
		at time.Time,
	) (bool, error)
}

// ErrConsolidationUnitAnswerUntranslatable 见 ConsolidationUnitView。确定性：要查两边的口径或节点作业的册，不是等那边恢复。
var ErrConsolidationUnitAnswerUntranslatable = errors.New(
	"parcel shipment: the consolidation unit answer cannot be translated")
