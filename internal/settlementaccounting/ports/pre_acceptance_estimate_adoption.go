package ports

import (
	"context"
	"errors"
	"time"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// PreAcceptanceEstimateAdoptionEffect 是登记一行受理前估价采用的三种落点。重放是同一户同一份再交一次；冲突是同一户不同内容。
// 两种都不改已落的行——采用册不设修订（ADR-0179 决定五，照 ADR-0158）。
type PreAcceptanceEstimateAdoptionEffect uint8

const (
	PreAcceptanceEstimateAdoptionRegistered PreAcceptanceEstimateAdoptionEffect = iota + 1
	PreAcceptanceEstimateAdoptionReplay
	PreAcceptanceEstimateAdoptionConflict
)

// PreAcceptanceEstimateAdoptionView 是采用册的只读口：按（租户、结算账户）交回那一行；册上没有这一户答 false——估价据此答未配置，
// 不默认形态、不拿零顶。与写口分名：施加编排只该问采用，不该拿到 Save。
type PreAcceptanceEstimateAdoptionView interface {
	FindAdoption(
		ctx context.Context,
		tenant domain.TenantID,
		account domain.SettlementAccountID,
	) (domain.PreAcceptanceEstimateAdoption, bool, error)
}

// PreAcceptanceEstimateAdoptionRegister 是采用册的写口。指向的结算账户还不在账户册上时交回的错误 errors.Is
// domain.ErrCatalogueTargetMissing：本册不代为建账户。
type PreAcceptanceEstimateAdoptionRegister interface {
	SaveAdoption(
		ctx context.Context,
		tenant domain.TenantID,
		adoption domain.PreAcceptanceEstimateAdoption,
		at time.Time,
	) (PreAcceptanceEstimateAdoptionEffect, error)
}

// PreAcceptanceEstimateAdoptionRegisteredIntent 是一行采用落库后要通知下游的最小事实：哪个租户的哪一户。下游是 parcel-shipment
// 停在`等待运营登记`的接受前财务控制——金额那一停等的正是这一行。形态与计价范围不进载荷：重驱时估价照旧向本册读口问，信封里再抄
// 一份就成了第二处权威。
type PreAcceptanceEstimateAdoptionRegisteredIntent struct {
	Tenant       domain.TenantID
	Account      domain.SettlementAccountID
	RegisteredAt time.Time
}

// ErrPreAcceptanceEstimateAdoptionHandoffRejected 表示交接口铸出的信封被框架的确定性校验拒收（账户标识超长时主题过不了上限）。
// 理由与形同 ErrSettlementAccountHandoffRejected：编排整笔不作答、事务回滚，登记口归用法格。
var ErrPreAcceptanceEstimateAdoptionHandoffRejected = errors.New("settlement accounting: pre-acceptance estimate adoption registered envelope rejected by envelope validation")

// PreAcceptanceEstimateAdoptionRegisteredHandoff 把「受理前估价采用已登记」写入 Outbox，与采用行同一事务（ADR-0094 决定四：
// `等待运营登记`那一格必须与它的续办触发同笔落地）。入队失败整项出错让事务回滚。信封被确定性拒收时交回的错误 errors.Is
// ErrPreAcceptanceEstimateAdoptionHandoffRejected；别的失败都是依赖不可用。
type PreAcceptanceEstimateAdoptionRegisteredHandoff interface {
	HandOffPreAcceptanceEstimateAdoptionRegistered(ctx context.Context, intent PreAcceptanceEstimateAdoptionRegisteredIntent) error
}
