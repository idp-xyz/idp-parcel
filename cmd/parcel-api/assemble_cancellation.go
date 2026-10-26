package main

import (
	"context"
	"fmt"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	shipmenthttp "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/http"
	psidentity "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/identity"
	psparty "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/partycommercial"
	pspostgres "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	pstf "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/transportfulfillment"
	shipmentapp "go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	pcpostgres "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	tfpostgres "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
)

// transactionalCancellation 把取消编排包进一笔事务：取消库的写口按框架合同走
// RequireExecutor，无事务即拒，事务边界因此归装配点，编排自己不开也不提交。编排交回业务答案（含未决、
// 拒绝、待处置与已有结果）时事务提交；发布意图交不出不翻决定——编排吞成重发引用后
// 照常作答，重放重发同一份；编排返回错误时整笔回滚。
type transactionalCancellation struct {
	transactor bentoapp.Transactor
	inner      *shipmentapp.CancelParcelHandler
}

var _ shipmenthttp.CancellationHandler = transactionalCancellation{}

func (cancellation transactionalCancellation) Handle(
	ctx context.Context,
	command shipmentapp.CancelParcelCommand,
) (shipmentapp.CancelParcelResult, error) {
	var result shipmentapp.CancelParcelResult
	err := cancellation.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		handled, handleErr := cancellation.inner.Handle(txCtx, command)
		if handleErr != nil {
			return handleErr
		}
		result = handled
		return nil
	})
	if err != nil {
		return shipmentapp.CancelParcelResult{}, err
	}
	return result, nil
}

// buildCancellationOrchestration 装配首方取消口（`/v1/shipment-requests/parcel-cancellations`，UC-PS-006 接受后取消）的
// 真编排（ADR-0310 取舍 C；ADR-0313；票 first-party-api/11）。
//
// 授权缝接真，规则仍是租户取值：委托仓储、收寄采认读口、当前有效终局读口、取消决定库（写口与按包裹的已成立取消读口同一只）、
// 面单渠道服务的首次有效收寄读口（TF 登记册之上的消费侧适配器，ADR-0025）、PCXL 取消标识签发、发布意图的 Outbox 交付、生产时钟
// 与取消边界锁——提交边界上的边界事实都读真的，任一样在册都使取消不成立（PS CONTEXT）。
//
// 授权缝走 ServiceStageRulesAdapter 整条真翻译链：取消授权目录由 ScopedCancellationContent 读（账户的商业范围里、委托接受那一刻
// 在效的、登记了取消目录的授权规则，ADR-0313），请求方格由 CustomerSelfCancellationParty 给（首方直客 1:1，请求方就是客户账户
// 本人）。「谁能在哪个阶段取消」不写死：账户没登解析键、该时点没有登记取消目录的授权规则、目录里没有客户这一行，前两处如实停在
// AUTHORITY_UNCONFIGURED，最后一处是一份真的拒绝——恢复动作是租户登记，不是修授权服务，更不是默认任何角色可取消或不可取消
// （UC-PS-006 明禁两个方向的默认）。「收寄前」这条边界不在目录里，是提交边界上的收寄 / 终局读口判的。
// 收寄资格 / 终局两个内容读口不装：本编排用不到，传 nil 即答「未配置」。
func buildCancellationOrchestration(db *bentopg.DB) (shipmenthttp.CancellationHandler, error) {
	authority, err := buildCancellationAuthority(db)
	if err != nil {
		return nil, err
	}
	return cancellationOrchestrationWith(db, authority)
}

// buildCancellationAuthority 装取消授权那一口的真翻译链，单独成函数是为了让装配用例拿同一条链对着真库说话。
func buildCancellationAuthority(db *bentopg.DB) (psports.CancellationAuthorityView, error) {
	requests, err := pspostgres.NewShipmentRequests(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: shipment requests: %w", err)
	}
	keys, err := pspostgres.NewCommercialResolutionKeyStore(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: resolution key store: %w", err)
	}
	publications, err := pcpostgres.NewCommercialPublications(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: commercial publications: %w", err)
	}
	scopeView, err := pcpostgres.NewCommercialAuthority(publications)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: commercial authority: %w", err)
	}
	declarations, err := pcpostgres.NewStageContentDeclarations(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: stage content declarations: %w", err)
	}
	content, err := psparty.NewScopedCancellationContent(requests, keys, scopeView, declarations)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: scoped cancellation content: %w", err)
	}
	return psparty.NewServiceStageRulesAdapter(
		nil, nil, content, psparty.CustomerSelfCancellationParty{}, nil), nil
}

// cancellationOrchestrationWith 是取消编排除授权以外的全部装配。授权单独作参数，是因为它是唯一一条
// 等租户登记的缝：真库用例要越过授权、走到提交边界上那几道读，只能在这里换一个已登记的授权答复，
// 其余缝照生产原样。
func cancellationOrchestrationWith(
	db *bentopg.DB,
	authority psports.CancellationAuthorityView,
) (shipmenthttp.CancellationHandler, error) {
	requests, err := pspostgres.NewShipmentRequests(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: shipment requests: %w", err)
	}
	adoptions, err := pspostgres.NewIntakeAdoptions(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: intake adoptions: %w", err)
	}
	cancellations, err := pspostgres.NewParcelCancellations(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: parcel cancellations: %w", err)
	}
	finals, err := pspostgres.NewFinalOutcomes(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: final outcomes: %w", err)
	}
	pickupRegistry, err := tfpostgres.NewCarrierFirstEffectivePickups(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: carrier first effective pickups: %w", err)
	}
	pickups, err := pstf.NewCarrierPickupBoundaryAdapter(pickupRegistry)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: carrier pickup boundary: %w", err)
	}
	identities, err := psidentity.NewParcelCancellations()
	if err != nil {
		return nil, fmt.Errorf("parcel-api: cancellation identities: %w", err)
	}
	store, err := outbox.NewStore(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: outbox store: %w", err)
	}
	clock := systemClock{}
	downstream, err := pspostgres.NewOutboxParcelCancellationHandoff(db, store, clock)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: parcel cancellation handoff: %w", err)
	}
	boundaryLocks, err := pspostgres.NewCancellationBoundaryLocks(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: cancellation boundary locks: %w", err)
	}

	handler := shipmentapp.NewCancelParcelHandler(shipmentapp.CancelParcelDeps{
		Requests:       requests,
		Authority:      authority,
		Adoptions:      adoptions,
		Finals:         finals,
		Cancellations:  cancellations,
		CarrierPickups: pickups,
		Store:          cancellations,
		Identities:     identities,
		Downstream:     downstream,
		Clock:          clock,
		BoundaryLock:   boundaryLocks,
	})
	return transactionalCancellation{transactor: db.Transactor(), inner: handler}, nil
}
