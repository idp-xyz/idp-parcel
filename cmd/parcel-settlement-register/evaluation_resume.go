package main

import (
	"fmt"
	"strings"

	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	pspostgres "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	pcpostgres "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	saidentity "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/identity"
	saparcelshipment "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/parcelshipment"
	sapartycommercial "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/partycommercial"
	sapostgres "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
)

// buildEvaluationResume 装运营续办口（ADR-0229 决定六）：续办编排要的两只发起编排（重跑发起停办）与两本请求册的整套
// 写口（起下一代），都接真册与真 Outbox 适配器，与派发进程里收「参数已登记」的那几扇门同一种装法——问的是同一组册、
// 交的是同一种请求信封。生产装配里不放替身：空册答未配置，续办重跑照实再停一次。
func buildEvaluationResume(db *bentopg.DB, outboxStore *outbox.Store) (*application.OperatorResumeEvaluationHandler, error) {
	clock := systemClock{}
	stalls, err := sapostgres.NewEvaluationResumptions(db)
	if err != nil {
		return nil, fmt.Errorf("构造评价续办队列：%w", err)
	}
	shipments, err := pspostgres.NewShipmentRequests(db)
	if err != nil {
		return nil, fmt.Errorf("构造包裹商业解析来源：%w", err)
	}
	resolutions, err := saparcelshipment.NewParcelCommercialResolutions(shipments)
	if err != nil {
		return nil, fmt.Errorf("构造包裹商业解析：%w", err)
	}
	closures, err := pcpostgres.NewCommercialResolutions(db)
	if err != nil {
		return nil, fmt.Errorf("构造商业闭包：%w", err)
	}
	terms, err := sapartycommercial.NewAdoptedSettlementTerms(closures)
	if err != nil {
		return nil, fmt.Errorf("构造结算政策读口：%w", err)
	}
	accounts, err := sapostgres.NewSettlementAccounts(db)
	if err != nil {
		return nil, fmt.Errorf("构造结算账户册：%w", err)
	}
	moments, err := sapostgres.NewSellBillingMomentAdoptions(db)
	if err != nil {
		return nil, fmt.Errorf("构造 SELL 计费时点采用册：%w", err)
	}
	sellRequests, err := sapostgres.NewSellEvaluationRequests(db)
	if err != nil {
		return nil, fmt.Errorf("构造 SELL 评价请求册：%w", err)
	}
	sellIdentities, err := saidentity.NewSellEvaluationRequestIdentities()
	if err != nil {
		return nil, fmt.Errorf("构造 SELL 请求铸造口：%w", err)
	}
	sellHandoff, err := sapostgres.NewOutboxSellEvaluationRequestHandoff(db, outboxStore, clock)
	if err != nil {
		return nil, fmt.Errorf("构造 SELL 请求交接口：%w", err)
	}
	sellMoments, err := application.NewRequestSellEvaluationOnParcelMomentHandler(application.RequestSellEvaluationOnParcelMomentDeps{
		Resolutions: resolutions, Terms: terms, Accounts: accounts, Adoptions: moments,
		Registry: sellRequests, Identity: sellIdentities, Downstream: sellHandoff, Stalls: stalls, Clock: clock,
	})
	if err != nil {
		return nil, fmt.Errorf("构造 SELL 发起编排：%w", err)
	}
	buyRequests, err := sapostgres.NewEvaluationRequests(db)
	if err != nil {
		return nil, fmt.Errorf("构造 BUY 评价请求册：%w", err)
	}
	buyIdentities, err := saidentity.NewEvaluationRequestIdentities()
	if err != nil {
		return nil, fmt.Errorf("构造 BUY 请求铸造口：%w", err)
	}
	buyHandoff, err := sapostgres.NewOutboxEvaluationRequestHandoff(db, outboxStore, clock)
	if err != nil {
		return nil, fmt.Errorf("构造 BUY 请求交接口：%w", err)
	}
	buyRequester, err := application.NewRequestBuyEvaluationHandler(application.RequestBuyEvaluationDeps{
		Registry: buyRequests, Identity: buyIdentities, Downstream: buyHandoff, Clock: clock,
	})
	if err != nil {
		return nil, fmt.Errorf("构造 BUY 请求编排：%w", err)
	}
	triggers, err := sapostgres.NewBuyEvaluationTriggers(db)
	if err != nil {
		return nil, fmt.Errorf("构造 BUY 触发册：%w", err)
	}
	agreementScopes, err := pcpostgres.NewSupplierAgreementScopes(db)
	if err != nil {
		return nil, fmt.Errorf("构造协议范围读口：%w", err)
	}
	scopes, err := sapartycommercial.NewSupplierAgreementScope(agreementScopes)
	if err != nil {
		return nil, fmt.Errorf("构造协议范围：%w", err)
	}
	buyOccurrences, err := application.NewTriggerBuyEvaluationOnOccurrenceFormedHandler(application.TriggerBuyEvaluationOnOccurrenceFormedDeps{
		Triggers: triggers, Agreements: scopes, Requests: buyRequester, Stalls: stalls, Clock: clock,
	})
	if err != nil {
		return nil, fmt.Errorf("构造 BUY 发起编排：%w", err)
	}
	resumer, err := application.NewResumeStalledEvaluationHandler(application.ResumeStalledEvaluationDeps{
		SellStalls: stalls, BuyStalls: stalls, SellMoments: sellMoments, BuyOccurrences: buyOccurrences,
		SellRequests: sellRequests, SellSuccessors: sellRequests, SellRegistry: sellRequests,
		SellIdentity: sellIdentities, SellHandoff: sellHandoff,
		BuyRequests: buyRequests, BuySuccessors: buyRequests, BuyRegistry: buyRequests,
		BuyIdentity: buyIdentities, BuyHandoff: buyHandoff, Clock: clock,
	})
	if err != nil {
		return nil, fmt.Errorf("构造续办编排：%w", err)
	}
	handler, err := application.NewOperatorResumeEvaluationHandler(stalls, resumer)
	if err != nil {
		return nil, fmt.Errorf("构造运营续办口：%w", err)
	}
	return handler, nil
}

// resumeAnswer 把一次运营续办的结果译成答复与退出码，逐件列出续办了什么。归格看恢复动作：办完（含一件都没有、凭这个
// 引用都续办过）→ 0；命令立不住 → 1；某一件停在依赖答不出上 → 3，整笔已回滚，同一个引用重跑。
func resumeAnswer(command string, result application.OperatorResumeEvaluationResult) (string, int) {
	switch result.Outcome {
	case application.OperatorResumeEvaluationDone:
		if len(result.Items) == 0 {
			return command + ": 没有在等的停办（没有停着的，或凭这个续办引用都续办过了）", exitRegistered
		}
		lines := make([]string, 0, len(result.Items))
		for _, item := range result.Items {
			line := item.Item.Kind.String() + " " + item.Item.Cause.String() + " → " + item.Result.Outcome().String()
			if id := item.Result.SellRequest().String(); id != "" {
				line += "（" + id + "）"
			}
			if id := item.Result.BuyRequest().String(); id != "" {
				line += "（" + id + "）"
			}
			if cause, stalled := item.Result.StopCause(); stalled {
				line += "（仍停在 " + cause.String() + "）"
			}
			lines = append(lines, line)
		}
		return command + ": 已续办 " + fmt.Sprint(len(result.Items)) + " 件：" + strings.Join(lines, "；"), exitRegistered
	case application.OperatorResumeEvaluationNotAccepted:
		return command + ": 未受理（方向与点名对不上，或没有续办引用）", exitUsage
	case application.OperatorResumeEvaluationUndecided:
		return command + ": 未决（某一件停在依赖答不出上，整笔已回滚，同一个续办引用重跑）", exitUndecided
	default:
		return fmt.Sprintf("%s: 未知应用结果 %d", command, result.Outcome), exitUndecided
	}
}
