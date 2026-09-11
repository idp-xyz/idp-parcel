package main

import (
	"context"
	"fmt"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	pspostgres "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	pcpostgres "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	visibilityhttp "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/http"
	veidentity "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/identity"
	vepartycommercial "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/partycommercial"
	vepostgres "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/postgres"
	visibilityapp "go.idp.xyz/idp-parcel/internal/visibilityexception/application"
	veports "go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

// transactionalClaims 把索赔编排的入口各包进一笔事务，理由随 transactionalCancellation：
// 索赔库的写口（UPSERT 与只增历史）按框架合同无事务即拒，事务边界归装配点。编排交回
// 业务答案（含未决与已有项）时事务提交；返回错误时整笔回滚。
//
// HTTP 端点只见 ClaimReceiver（受理）：资格审核与责任结论是另外两个判断，由内部授权
// 角色走各自入口，客户提交面连形状都看不见（CONTEXT 三判分步）。ScreenClaim 在这里
// 仍然包装，是给装配测试钉资格缝两态（未登记租户答待登记、已登记租户按册答）用的
// ——审核面今天没有 HTTP 端点，但它审的是生产装配的同一个处理器，不另造一份。
type transactionalClaims struct {
	transactor bentoapp.Transactor
	inner      *visibilityapp.HandleClaimHandler
}

var _ visibilityhttp.ClaimReceiver = transactionalClaims{}

func (claims transactionalClaims) ReceiveClaim(
	ctx context.Context,
	command visibilityapp.ReceiveClaimCommand,
) (visibilityapp.HandleClaimResult, error) {
	var result visibilityapp.HandleClaimResult
	err := claims.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		handled, handleErr := claims.inner.ReceiveClaim(txCtx, command)
		if handleErr != nil {
			return handleErr
		}
		result = handled
		return nil
	})
	if err != nil {
		return visibilityapp.HandleClaimResult{}, err
	}
	return result, nil
}

func (claims transactionalClaims) ScreenClaim(
	ctx context.Context,
	command visibilityapp.ScreenClaimCommand,
) (visibilityapp.HandleClaimResult, error) {
	var result visibilityapp.HandleClaimResult
	err := claims.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		handled, handleErr := claims.inner.ScreenClaim(txCtx, command)
		if handleErr != nil {
			return handleErr
		}
		result = handled
		return nil
	})
	if err != nil {
		return visibilityapp.HandleClaimResult{}, err
	}
	return result, nil
}

// buildClaimsOrchestration 装配 `/claims` 的真编排（UC-VE-007；接线票
// `.scratch/parcel-api-remaining-endpoint-wiring/issues/04`）。
//
// 七条缝全部接真：索赔库、追偿库、追偿标识签发、责任结论的 Outbox 意图交付、生产
// 时钟、资格规则目录与材料证据。资格缝走多租户读适配器（租户从查询来，票
// ve-claims-read-seams/01）：未登记租户答「声明不在场」的业务格，编排停在
// ELIGIBILITY_CATALOGUE_NOT_CONFIGURED——那是待登记的实例参数，不再是接线未决。
// 材料证据缝读收讫减撤销的现存集（归集面由票 ve-claims-read-seams/02 建起，登记走
// 受控 CLI `parcel-ve-register`）：零行是「查过了，一件都没收到」的有效事实，差集
// 成立、限期补充可以推进——「无从查起」那一格随显式未配置桩退役。
//
// 资格缝的首次索赔期限与最低材料两维由消费侧适配器叠在 VE 自己的册上、从 PC 客户服务
// 规则正文读（票 ve-claims-read-seams/03）；按哪一版读，由目标包裹所属委托接受时固定的商业
// 解析回指决定（ADR-0136），装配见 buildClaimEligibilityRules。
func buildClaimsOrchestration(db *bentopg.DB) (transactionalClaims, error) {
	claimStore, err := vepostgres.NewClaims(db)
	if err != nil {
		return transactionalClaims{}, fmt.Errorf("parcel-api: claim store: %w", err)
	}
	clock := systemClock{}
	eligibility, err := buildClaimEligibilityRules(db)
	if err != nil {
		return transactionalClaims{}, err
	}
	evidence, err := vepostgres.NewClaimMaterialReceipts(db)
	if err != nil {
		return transactionalClaims{}, fmt.Errorf("parcel-api: claim material receipts: %w", err)
	}
	recoveries, err := vepostgres.NewRecoveries(db)
	if err != nil {
		return transactionalClaims{}, fmt.Errorf("parcel-api: recovery store: %w", err)
	}
	identities, err := veidentity.NewRecoveryMatters()
	if err != nil {
		return transactionalClaims{}, fmt.Errorf("parcel-api: recovery identities: %w", err)
	}
	store, err := outbox.NewStore(db)
	if err != nil {
		return transactionalClaims{}, fmt.Errorf("parcel-api: outbox store: %w", err)
	}
	settlement, err := vepostgres.NewOutboxLiabilityHandoff(db, store, clock)
	if err != nil {
		return transactionalClaims{}, fmt.Errorf("parcel-api: liability handoff: %w", err)
	}
	handler := visibilityapp.NewHandleClaimHandler(visibilityapp.HandleClaimDeps{
		Claims:      claimStore,
		Eligibility: eligibility,
		Evidence:    evidence,
		Recoveries:  recoveries,
		Identities:  identities,
		Settlement:  settlement,
		Clock:       clock,
	})
	return transactionalClaims{transactor: db.Transactor(), inner: handler}, nil
}

// buildClaimEligibilityRules 装配资格规则读面：VE 自己的册（合同责任范围 + 申请人授权目录，多租户
// 形状）在下，PC 客户服务规则正文的两维叠在上（ADR-0104 Consequences；适配器
// vepartycommercial.ClaimServiceRules）。
//
// 两维按哪一版规则读，由目标包裹所属委托接受时固定的商业解析回指决定（ADR-0136 决定一 / 二）：
// 回指由 PS 的委托仓储按（租户，包裹身份）答（它兼实现 PS 的回指读口），闭包由 PC 的解析库按
// （租户，回指）交回，正文经点读口。这里没有解析编排、没有时钟、没有键——VE 不自己解析商业依据，
// 选用时点已固定在闭包里；不在任何一处拿系统时间或默认范围顶一个键，那会把「租户还没登记」变成
// 一次有依据的解析。实例半边落在 PS 的解析键登记面（那一行的必需依据要含客户服务规则与客户合同，
// ADR-0136 决定三 / 四），VE 侧没有键登记面。机制接上而登记面尚未让闭包形成时，PS 那一头对每个
// 对象都答「没有」、两维停在未登记——与本票之前键来源留 nil 的可观察行为一字不变（ADR-0136
// Consequences）。四个协作方缺一即装配失败（ADR-0079 决定八），没有可选的一半。
func buildClaimEligibilityRules(db *bentopg.DB) (veports.EligibilityRuleView, error) {
	own, err := vepostgres.NewMultiTenantClaimEligibilityRules(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: claim eligibility rules: %w", err)
	}
	references, err := pspostgres.NewShipmentRequests(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: shipment requests: %w", err)
	}
	closures, err := pcpostgres.NewCommercialResolutions(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: commercial resolutions: %w", err)
	}
	contents, err := pcpostgres.NewCustomerServiceRuleContents(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: customer service rule contents: %w", err)
	}
	eligibility, err := vepartycommercial.NewClaimServiceRules(vepartycommercial.ClaimServiceRulesDeps{
		Rules:      own,
		References: references,
		Closures:   closures,
		Contents:   contents,
	})
	if err != nil {
		return nil, fmt.Errorf("parcel-api: claim service rules: %w", err)
	}
	return eligibility, nil
}
