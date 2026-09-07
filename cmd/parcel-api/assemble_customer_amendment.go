package main

import (
	"context"
	"fmt"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	ccpostgres "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/postgres"
	nopostgres "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/postgres"
	pscustoms "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/customscompliance"
	shipmenthttp "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/http"
	psidentity "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/identity"
	psnode "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/nodeoperations"
	pspartycommercial "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/partycommercial"
	pspostgres "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	shipmentapp "go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	pcpostgres "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
)

// amendmentBoundary 是资料修订编排里委托落库那一层的事务壳：Save 切一个事务，不发信封。
//
// 与 supplementBoundary 不同形，是因为两条编排交出意图的位置不同：受控补充编排自己不调任何交接
// 口，「新提交版本已形成」只能挂在 Save 里同事务交出；资料修订编排则在 Save 之后**自己**调
// `Downstream.HandOffSourceDataVersion`（步骤 9），并把交接失败译成`技术未形成`加续办引用、重放时经
// existing 路径重发同一份意图（编排 handOff 头注写明的设计，`AT-PS-031`）。壳若再在 Save 里发一次，
// 同一份意图就会被两处各交一遍——EnqueueOnce 能吞掉第二份，但那是拿幂等去掩装配上的重复。
//
// 于是版本与意图不在同一个事务里：版本落了库而意图没入队时编排交回 SourceDataVersionNotHandedOff，
// 客户重放同一请求身份时重发。要做到同事务，得让编排不再自己调 Downstream——那是改编排不是改装配，
// 记在票 ps-port-remainder/04「一格诚实的缝」，本壳照编排今天的形状接。
type amendmentBoundary struct {
	transactor bentoapp.Transactor
	inner      ports.ShipmentRequestRepository
}

var _ ports.ShipmentRequestRepository = amendmentBoundary{}

func (boundary amendmentBoundary) FindBySourceIdentity(
	ctx context.Context,
	identity domain.SourceIdentity,
) (domain.ShipmentRequest, bool, error) {
	return boundary.inner.FindBySourceIdentity(ctx, identity)
}

// Insert 只切事务：资料修订编排走不到这一口，它在这里是因为端口带着它。
func (boundary amendmentBoundary) Insert(
	ctx context.Context,
	identity domain.SourceIdentity,
	request domain.ShipmentRequest,
) (ports.ShipmentRequestInsertOutcome, error) {
	var outcome ports.ShipmentRequestInsertOutcome
	err := boundary.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		inserted, err := boundary.inner.Insert(txCtx, identity, request)
		if err != nil {
			return err
		}
		outcome = inserted
		return nil
	})
	if err != nil {
		return ports.ShipmentRequestInsertOutcomeInvalid, err
	}
	return outcome, nil
}

func (boundary amendmentBoundary) Save(
	ctx context.Context,
	identity domain.SourceIdentity,
	request domain.ShipmentRequest,
) (ports.ShipmentRequestSaveOutcome, error) {
	var outcome ports.ShipmentRequestSaveOutcome
	err := boundary.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		saved, err := boundary.inner.Save(txCtx, identity, request)
		if err != nil {
			return err
		}
		outcome = saved
		return nil
	})
	if err != nil {
		return ports.ShipmentRequestSaveOutcomeInvalid, err
	}
	return outcome, nil
}

// sourceDataHandoffBoundary 给资料版本意图的每次交出各开一个事务：OutboxSourceDataHandoff 走
// RequireExecutor（意图只与调用方同一事务落库），而编排在 Save 之后、事务之外调它。意图由版本标识
// 认领（EnqueueOnce 先查后插），重放路径上再交一次仍是同一份。
type sourceDataHandoffBoundary struct {
	transactor bentoapp.Transactor
	inner      ports.SourceDataVersionHandoff
}

var _ ports.SourceDataVersionHandoff = sourceDataHandoffBoundary{}

func (boundary sourceDataHandoffBoundary) HandOffSourceDataVersion(
	ctx context.Context,
	intent ports.SourceDataVersionHandoffIntent,
) error {
	return boundary.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		return boundary.inner.HandOffSourceDataVersion(txCtx, intent)
	})
}

// buildCustomerAmendmentOrchestration 装配 `/shipment-requests/source-data-amendments` 的真编排
// （UC-PS-002；票 ps-port-remainder/04 把入口接进生产，形照 ADR-0106 决定四）。来源保全走
// preservationBoundary（与首次提交、受控补充同一层壳），委托落库走 amendmentBoundary，资料版本意图
// 经 sourceDataHandoffBoundary 携 OutboxSourceDataHandoff 入队，事件类型
// `parcel-shipment.source-data-version.formed`。
//
// 授权那一口今天接的仍是**未配置**答复（party-commercial 授权那半尚未立，票 ps-port-remainder/03）：
// 编排会如实停在授权未决（`SourceDataAmendmentAuthorityRulesNotConfigured`）——那正是接上这条入口的
// 意义：停点从「没有入口」变成「有入口、说得出停在哪」。PC 半边落地时在这里换成真适配器，端点、壳与
// 编排都不动。客户渠道的采信身份属 `PAR-INT-01` / `BD-PS-009`：端点表那一行以 `UnconfiguredIntake{}`
// 起步，本函数不带任何默认身份。
//
// 允许矩阵那一口已接真（票 ps-port-remainder/02 余段，ADR-0120）：经 buildSourceDataAmendmentAllowance
// 回指接受时固定的接单规则包版本、读 party-commercial 的资料修订允许声明、按（资料组 × 阶段 × 意图）
// 查格译三值。闭包不在或声明无父行时答`未声明`，编排停在待复核——与此前的未配置答复在停点上同形，
// 差别在从此登了声明就按声明答。
//
// 「资料修订阶段」判断（ADR-0118）读五个口：本上下文自有的收寄采用、面单交易、包裹终局三本登记册
// 接真库；关务三格与装袋一格经消费侧适配器接 customs-compliance 与 node-operations 各自按包裹键的
// 读面（票 ps-port-remainder/05：CC 的 ParcelDeclarationFactsView、NO 的 ParcelContainmentView）——
// 读面读的是同一个库里那两个上下文自己的表，适配器只翻译不判断。此前这两口接的是一律答`不知道`的
// 未接答复，授权过了之后编排必然停在`判不出阶段`；接真之后阶段按事实判出，停点后移到矩阵那一格。
func buildCustomerAmendmentOrchestration(db *bentopg.DB) (shipmenthttp.AmendmentHandler, error) {
	customs, consolidation, err := buildAmendmentStageFactViews(db)
	if err != nil {
		return nil, err
	}
	rules, err := buildSourceDataAmendmentAllowance(db)
	if err != nil {
		return nil, err
	}
	return assembleCustomerAmendmentOrchestration(
		db,
		pspartycommercial.UnconfiguredSourceDataAmendmentAuthorizer{},
		rules,
		customs,
		consolidation,
	)
}

// buildSourceDataAmendmentAllowance 装资料修订允许矩阵那一口的真读法：委托仓储 + PC 解析库回指采用的接单
// 规则包（ResolvedAdoptedStageOwner，ADR-0062），PC 阶段内容读口取声明，消费适配器译三值。回指走的是
// 与 parcel-dispatch 装收寄资格 / 终局规则同一条路径——采用哪一版是接受时固定闭包的事，不在查询上。
// 单独成函数是为了让装配用例能拿同一只真读法套记录壳（阶段这一维只在查询上可见），而不必复制这段接线。
func buildSourceDataAmendmentAllowance(db *bentopg.DB) (ports.SourceDataRuleDeclaration, error) {
	requests, err := pspostgres.NewShipmentRequests(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: shipment requests: %w", err)
	}
	resolutions, err := pcpostgres.NewCommercialResolutions(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: commercial resolutions: %w", err)
	}
	owners, err := pspartycommercial.NewResolvedAdoptedStageOwner(requests, resolutions)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: adopted stage owner: %w", err)
	}
	declarations, err := pcpostgres.NewStageContentDeclarations(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: stage content declarations: %w", err)
	}
	rules, err := pspartycommercial.NewDeclaredSourceDataAmendmentAllowance(owners, declarations)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: source data amendment allowance: %w", err)
	}
	return rules, nil
}

// buildAmendmentStageFactViews 装两只跨上下文阶段事实适配器：各自套在提供方的 postgres 读面上。
// 单独成函数是为了让装配用例也能接真读面（「接真后停点从判不出阶段变成按事实答」那一格要证的正是
// 这两只而不是替身），而不必复制这段接线。
func buildAmendmentStageFactViews(db *bentopg.DB) (ports.CustomsStageView, ports.ConsolidationStageView, error) {
	declarationFacts, err := ccpostgres.NewParcelDeclarationFactsView(db)
	if err != nil {
		return nil, nil, fmt.Errorf("parcel-api: parcel declaration facts view: %w", err)
	}
	customs, err := pscustoms.NewCustomsStageView(declarationFacts)
	if err != nil {
		return nil, nil, fmt.Errorf("parcel-api: customs stage view: %w", err)
	}
	containment, err := nopostgres.NewParcelContainmentView(db)
	if err != nil {
		return nil, nil, fmt.Errorf("parcel-api: parcel containment view: %w", err)
	}
	consolidation, err := psnode.NewConsolidationStageView(containment)
	if err != nil {
		return nil, nil, fmt.Errorf("parcel-api: consolidation stage view: %w", err)
	}
	return customs, consolidation, nil
}

// assembleCustomerAmendmentOrchestration 是 buildCustomerAmendmentOrchestration 的形状半边：两个提供方口与
// 两个邻接上下文读口由调用方给，生产给未配置的授权答复、真矩阵读法与真读面，装配用例给放行的授权替身以证
// 「授权过了之后阶段按真读面判出、矩阵按真声明答」那几个更深的格——生产装配自己走不到它们（授权先停）。
func assembleCustomerAmendmentOrchestration(
	db *bentopg.DB,
	authorizer ports.SourceDataAmendmentAuthorizer,
	rules ports.SourceDataRuleDeclaration,
	customs ports.CustomsStageView,
	consolidation ports.ConsolidationStageView,
) (shipmenthttp.AmendmentHandler, error) {
	sources, err := pspostgres.NewSourceSubmissions(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: source submissions: %w", err)
	}
	requests, err := pspostgres.NewShipmentRequests(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: shipment requests: %w", err)
	}
	identities, err := psidentity.NewSourceDataVersions()
	if err != nil {
		return nil, fmt.Errorf("parcel-api: source data version identities: %w", err)
	}
	store, err := outbox.NewStore(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: outbox store: %w", err)
	}
	clock := systemClock{}
	handoff, err := pspostgres.NewOutboxSourceDataHandoff(db, store, clock)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: source data version handoff: %w", err)
	}
	adoptions, err := pspostgres.NewIntakeAdoptions(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: intake adoptions: %w", err)
	}
	labelTransactions, err := pspostgres.NewLabelTransactions(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: label transactions: %w", err)
	}
	finals, err := pspostgres.NewFinalOutcomes(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: final outcomes: %w", err)
	}
	return shipmentapp.NewAmendCustomerSourceDataHandler(shipmentapp.AmendCustomerSourceDataDeps{
		Sources:    preservationBoundary{transactor: db.Transactor(), inner: sources},
		Requests:   amendmentBoundary{transactor: db.Transactor(), inner: requests},
		Authorizer: authorizer,
		Rules:      rules,
		Identities: identities,
		Downstream: sourceDataHandoffBoundary{transactor: db.Transactor(), inner: handoff},
		Stage: shipmentapp.AmendmentStageDeps{
			ResponsibilityStarts: adoptions,
			LabelTransactions:    labelTransactions,
			Finals:               finals,
			Customs:              customs,
			Consolidation:        consolidation,
		},
		Clock: clock,
	}), nil
}
