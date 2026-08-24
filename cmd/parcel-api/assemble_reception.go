package main

import (
	"context"
	"errors"
	"fmt"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	nodeopshttp "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/http"
	noidentity "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/identity"
	nopostgres "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/postgres"
	nodeopsapp "go.idp.xyz/idp-parcel/internal/nodeoperations/application"
	nodomain "go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	noports "go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
)

// errParcelIdentityViewNotConfigured 见 unconfiguredParcelIdentityView 的注释。
var errParcelIdentityViewNotConfigured = errors.New("parcel-api: parcel identity view is not configured")

// unconfiguredParcelIdentityView 是身份核对缝的「显式未配置」：PS 侧的外部标识关联
// 模型还不存在（缺口记录在 `.scratch/ps-external-mark-relations`），这条缝今天没有
// 可接的提供方机制。端口合同本身写着「依赖调不通作为错误返回」，据此它对每次核对
// 如实报错，编排停在`收寄待确认`并携续办引用。
//
// 绝不交回空候选顶替：零候选是「查过了，查无此标识」的业务答案（走`待识别`、真实
// 建立收寄与控制），拿它顶「没查」会把一件没核对过身份的实物记成已核对。明确拒收与
// 仅扫描两支不经身份核对，不受本缝影响。PS 侧模型落地后这里换真跨上下文适配器。
type unconfiguredParcelIdentityView struct{}

var _ noports.ParcelIdentityView = unconfiguredParcelIdentityView{}

func (unconfiguredParcelIdentityView) ResolveParcelIdentity(
	context.Context,
	nodomain.TenantID,
	noports.ExternalMarkObservation,
) ([]nodomain.ParcelAssociationReference, error) {
	return nil, errParcelIdentityViewNotConfigured
}

// transactionalReception 把收寄编排包进一笔事务，理由随 transactionalSubmission：收寄
// 库的写口按框架合同无事务即拒（收寄判断落库与同一步的意图发布必须同生共死），事务
// 边界归装配点。意图交付失败不翻结果——编排把它吞成续办引用后照常交回业务答案，事务
// 因此照常提交，重放重发同一份（AT-NO-027 的语义靠的正是「结果已提交而意图没跟上」
// 这个可观察状态）；编排返回错误时整笔回滚，半截写入不落库。
type transactionalReception struct {
	transactor bentoapp.Transactor
	inner      *nodeopsapp.ReceiveDeliveredUnitHandler
}

var _ nodeopshttp.ReceptionHandler = transactionalReception{}

func (reception transactionalReception) Handle(
	ctx context.Context,
	command nodeopsapp.ReceiveDeliveredUnitCommand,
) (nodeopsapp.ReceiveDeliveredUnitResult, error) {
	var result nodeopsapp.ReceiveDeliveredUnitResult
	err := reception.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		handled, handleErr := reception.inner.Handle(txCtx, command)
		if handleErr != nil {
			return handleErr
		}
		result = handled
		return nil
	})
	if err != nil {
		return nodeopsapp.ReceiveDeliveredUnitResult{}, err
	}
	return result, nil
}

// buildReceptionOrchestration 装配 `/node-operations/receptions` 的真编排（UC-NO-002；
// 接线票 `.scratch/parcel-api-remaining-endpoint-wiring/issues/01`）。
//
// 机制半边接真：收寄库、收寄结果版本签发、Outbox 意图交付走 NO 真库口。身份核对缝
// 是唯一的「显式未配置」，理由与形状见 unconfiguredParcelIdentityView——它不是实例
// 半边（不等租户参数），是提供方机制未建，恢复动作在 PS 侧建模，不在登记参数。
func buildReceptionOrchestration(db *bentopg.DB) (nodeopshttp.ReceptionHandler, error) {
	receptions, err := nopostgres.NewReceptions(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: receptions: %w", err)
	}
	versions, err := noidentity.NewIntakeResultVersions()
	if err != nil {
		return nil, fmt.Errorf("parcel-api: intake result versions: %w", err)
	}
	store, err := outbox.NewStore(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: outbox store: %w", err)
	}
	clock := systemClock{}
	downstream, err := nopostgres.NewOutboxNodeIntakeHandoff(db, store, clock)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: node intake handoff: %w", err)
	}
	handler := nodeopsapp.NewReceiveDeliveredUnitHandler(nodeopsapp.ReceiveDeliveredUnitDeps{
		Identity:   unconfiguredParcelIdentityView{},
		Receptions: receptions,
		Versions:   versions,
		Downstream: downstream,
		Clock:      clock,
	})
	return transactionalReception{transactor: db.Transactor(), inner: handler}, nil
}
