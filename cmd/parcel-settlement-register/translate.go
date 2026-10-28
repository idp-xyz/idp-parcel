package main

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/registrationjson"
)

// 本文件只把命令名分派到登记用例；「登记输入 JSON → 应用命令」的译装在
// `internal/settlementaccounting/adapters/registrationjson`——分家理由写在那个包的头注。命令名与退出码是
// 本入口自己的表面，因此留在这里。

// 封闭命令表：资金事实的首版与更正是两个命令类型，不共享入口——更正命令上没有来源 / 付款人 / 种类 / 币种 /
// 发生时刻（从链头照抄），首版命令上没有回指；一条更正无回指时不能退化成首版。结算账户是第三族命令，
// 登记的是固定属性那一行，不走采用编排。资金映射、核销与核销撤销各一条命令，调用已有的 Map、Apply、Reverse：
// 命令只译载荷，分配由交进来的人写全、不在这里推导；撤销只登记核销关系失效，原分配不删。
//
// 用法文本与未知命令的错误文本都从 allCommands 生成，不各抄一遍（parcel-network-register supportedKinds 的教训）。
const (
	commandExternalFundsFact           = "external-funds-fact"
	commandExternalFundsFactCorrection = "external-funds-fact-correction"
	commandFundsMapping                = "funds-mapping"
	commandFundsApplication            = "funds-application"
	commandFundsApplicationReversal    = "funds-application-reversal"
	commandSettlementAccount           = "settlement-account"
	commandSupplierAuditAuthority      = "supplier-audit-authority"
	commandSupplierPayableAccount      = "supplier-payable-account"
	commandClaimAmountRule             = "claim-amount-rule"
	commandChargeConfirmationFacts     = "charge-confirmation-facts"
	commandAmountGrammar               = "amount-grammar"
	commandAllocationForm              = "allocation-form"
	commandAuditEscalationCeiling      = "audit-escalation-ceiling"
	commandBuyEvaluationTrigger        = "buy-evaluation-trigger"
	commandAccountingConnector         = "accounting-connector"
	commandPeriodicFee                 = "periodic-fee"
	commandChargeAttribution           = "charge-attribution"
	commandStatementArrangement        = "statement-arrangement"
	commandSellEvaluationTrigger       = "sell-evaluation-trigger"
	commandSettlementMoment            = "settlement-moment"
	commandChargeConfirmationCondition = "charge-confirmation-condition"
	commandPreAcceptanceEstimate       = "pre-acceptance-estimate-adoption"
	commandSellBillingMomentAdoption   = "sell-billing-moment-adoption"
	commandFeeItemMapping              = "fee-item-mapping"
	commandEvaluationResume            = "evaluation-resume"
)

var allCommands = []string{
	commandExternalFundsFact,
	commandExternalFundsFactCorrection,
	commandFundsMapping,
	commandFundsApplication,
	commandFundsApplicationReversal,
	commandSettlementAccount,
	commandSupplierAuditAuthority,
	commandSupplierPayableAccount,
	commandClaimAmountRule,
	commandChargeConfirmationFacts,
	commandAmountGrammar,
	commandAllocationForm,
	commandAuditEscalationCeiling,
	commandBuyEvaluationTrigger,
	commandAccountingConnector,
	commandPeriodicFee,
	commandChargeAttribution,
	commandStatementArrangement,
	commandSellEvaluationTrigger,
	commandSettlementMoment,
	commandChargeConfirmationCondition,
	commandPreAcceptanceEstimate,
	commandSellBillingMomentAdoption,
	commandFeeItemMapping,
	commandEvaluationResume,
}

// dispatchFunc 是一条命令在事务内的一次调用，交回已经归好退出码的答复。资金事实与结算账户是两族答案，
// 归格留在各自的函数里，这里只负责把译装后的命令送进对应用例。
type dispatchFunc func(ctx context.Context, registrar registrar) (string, int, error)

// commandFor 按命令译装输入，交回一个在事务内执行的调用。译装失败当场拒，不进事务——用法错误与「登记与否
// 未知」是两个退出码，让它进了事务就分不开了。
func commandFor(command string, raw []byte) (dispatchFunc, error) {
	switch command {
	case commandExternalFundsFact:
		translated, err := registrationjson.ExternalFundsFactFromJSON(raw)
		if err != nil {
			return nil, err
		}
		return func(ctx context.Context, registrar registrar) (string, int, error) {
			result, err := registrar.funds.AdoptFact(ctx, translated)
			if err != nil {
				return "", 0, err
			}
			message, code := fundsAnswer(commandExternalFundsFact, result.Outcome(), result.UndecidedReason(),
				result.FundsHandoffReference(), subjectOf(result))
			return message, code, nil
		}, nil
	case commandExternalFundsFactCorrection:
		translated, err := registrationjson.ExternalFundsFactCorrectionFromJSON(raw)
		if err != nil {
			return nil, err
		}
		return func(ctx context.Context, registrar registrar) (string, int, error) {
			result, err := registrar.funds.CorrectFact(ctx, translated)
			if err != nil {
				return "", 0, err
			}
			message, code := fundsAnswer(commandExternalFundsFactCorrection, result.Outcome(), result.UndecidedReason(),
				result.FundsHandoffReference(), correctionSubjectOf(result))
			return message, code, nil
		}, nil
	case commandFundsMapping:
		translated, err := registrationjson.FundsMappingFromJSON(raw)
		if err != nil {
			return nil, err
		}
		return func(ctx context.Context, registrar registrar) (string, int, error) {
			result, err := registrar.funds.Map(ctx, translated)
			if err != nil {
				return "", 0, err
			}
			message, code := fundsAnswer(commandFundsMapping, result.Outcome(), result.UndecidedReason(),
				result.FundsHandoffReference(), mappingSubjectOf(result))
			return message, code, nil
		}, nil
	case commandFundsApplication:
		translated, err := registrationjson.FundsApplicationFromJSON(raw)
		if err != nil {
			return nil, err
		}
		return func(ctx context.Context, registrar registrar) (string, int, error) {
			result, err := registrar.funds.Apply(ctx, translated)
			if err != nil {
				return "", 0, err
			}
			message, code := fundsAnswer(commandFundsApplication, result.Outcome(), result.UndecidedReason(),
				result.FundsHandoffReference(), applicationSubjectOf(result))
			return message, code, nil
		}, nil
	case commandFundsApplicationReversal:
		translated, err := registrationjson.FundsApplicationReversalFromJSON(raw)
		if err != nil {
			return nil, err
		}
		return func(ctx context.Context, registrar registrar) (string, int, error) {
			result, err := registrar.funds.Reverse(ctx, translated)
			if err != nil {
				return "", 0, err
			}
			message, code := fundsAnswer(commandFundsApplicationReversal, result.Outcome(), result.UndecidedReason(),
				result.FundsHandoffReference(), applicationSubjectOf(result))
			return message, code, nil
		}, nil
	case commandSettlementAccount:
		translated, err := registrationjson.SettlementAccountFromJSON(raw)
		if err != nil {
			return nil, err
		}
		return func(ctx context.Context, registrar registrar) (string, int, error) {
			effect, err := registrar.accounts.Register(ctx, translated)
			if err != nil {
				return "", 0, err
			}
			message, code := accountAnswer(commandSettlementAccount, effect, translated.Account.ID().String())
			return message, code, nil
		}, nil
	case commandPreAcceptanceEstimate:
		translated, err := registrationjson.PreAcceptanceEstimateAdoptionFromJSON(raw)
		if err != nil {
			return nil, err
		}
		return func(ctx context.Context, registrar registrar) (string, int, error) {
			effect, err := registrar.adoptions.Register(ctx, translated)
			if err != nil {
				return "", 0, err
			}
			message, code := adoptionAnswer(commandPreAcceptanceEstimate, effect, translated.Adoption.Account().String())
			return message, code, nil
		}, nil
	case commandSellBillingMomentAdoption:
		translated, err := registrationjson.SellBillingMomentAdoptionFromJSON(raw)
		if err != nil {
			return nil, err
		}
		return func(ctx context.Context, registrar registrar) (string, int, error) {
			effect, err := registrar.sellMoments.Register(ctx, translated)
			if err != nil {
				return "", 0, err
			}
			message, code := catalogueAnswer(commandSellBillingMomentAdoption, effect, translated.Adoption.Account().String())
			return message, code, nil
		}, nil
	case commandFeeItemMapping:
		tenant, mapping, err := registrationjson.FeeItemMappingFromJSON(raw)
		if err != nil {
			return nil, err
		}
		return func(ctx context.Context, registrar registrar) (string, int, error) {
			effect, err := registrar.feeMappings.RegisterFeeItemMapping(ctx, tenant, mapping)
			if err != nil {
				return "", 0, err
			}
			message, code := catalogueAnswer(commandFeeItemMapping, effect, mapping.ChargeCode().String())
			return message, code, nil
		}, nil
	case commandSupplierAuditAuthority:
		tenant, registration, err := registrationjson.SupplierAuditAuthorityFromJSON(raw)
		if err != nil {
			return nil, err
		}
		return func(ctx context.Context, registrar registrar) (string, int, error) {
			effect, err := registrar.catalogues.RegisterSupplierAuditAuthority(ctx, tenant, registration)
			if err != nil {
				return "", 0, err
			}
			message, code := catalogueAnswer(commandSupplierAuditAuthority, effect, registration.Auditor().String())
			return message, code, nil
		}, nil
	case commandSupplierPayableAccount:
		tenant, registration, err := registrationjson.SupplierPayableAccountFromJSON(raw)
		if err != nil {
			return nil, err
		}
		return func(ctx context.Context, registrar registrar) (string, int, error) {
			effect, err := registrar.catalogues.RegisterSupplierPayableAccount(ctx, tenant, registration)
			if err != nil {
				return "", 0, err
			}
			message, code := catalogueAnswer(commandSupplierPayableAccount, effect, registration.Account().String())
			return message, code, nil
		}, nil
	case commandClaimAmountRule:
		tenant, registration, err := registrationjson.ClaimAmountRuleFromJSON(raw)
		if err != nil {
			return nil, err
		}
		return func(ctx context.Context, registrar registrar) (string, int, error) {
			effect, err := registrar.catalogues.RegisterClaimAmountRule(ctx, tenant, registration)
			if err != nil {
				return "", 0, err
			}
			message, code := catalogueAnswer(commandClaimAmountRule, effect, registration.Rule().String())
			return message, code, nil
		}, nil
	case commandAmountGrammar:
		tenant, registration, err := registrationjson.AmountGrammarFromJSON(raw)
		if err != nil {
			return nil, err
		}
		return func(ctx context.Context, registrar registrar) (string, int, error) {
			effect, err := registrar.grammars.RegisterAmountGrammar(ctx, tenant, registration)
			if err != nil {
				return "", 0, err
			}
			message, code := catalogueAnswer(commandAmountGrammar, effect, registration.Ref())
			return message, code, nil
		}, nil
	case commandAuditEscalationCeiling:
		tenant, registration, err := registrationjson.AuditEscalationCeilingFromJSON(raw)
		if err != nil {
			return nil, err
		}
		return func(ctx context.Context, registrar registrar) (string, int, error) {
			effect, err := registrar.ceilings.RegisterAuditEscalationCeiling(ctx, tenant, registration)
			if err != nil {
				return "", 0, err
			}
			message, code := catalogueAnswer(commandAuditEscalationCeiling, effect, registration.Supplier().String())
			return message, code, nil
		}, nil
	case commandAccountingConnector:
		tenant, registration, err := registrationjson.AccountingConnectorFromJSON(raw)
		if err != nil {
			return nil, err
		}
		return func(ctx context.Context, registrar registrar) (string, int, error) {
			effect, err := registrar.connectors.RegisterAccountingConnector(ctx, tenant, registration)
			if err != nil {
				return "", 0, err
			}
			message, code := catalogueAnswer(commandAccountingConnector, effect, registration.Counterparty().String())
			return message, code, nil
		}, nil
	case commandBuyEvaluationTrigger:
		tenant, registration, err := registrationjson.BuyEvaluationTriggerFromJSON(raw)
		if err != nil {
			return nil, err
		}
		return func(ctx context.Context, registrar registrar) (string, int, error) {
			effect, err := registrar.triggers.RegisterBuyEvaluationTrigger(ctx, tenant, registration)
			if err != nil {
				return "", 0, err
			}
			message, code := catalogueAnswer(commandBuyEvaluationTrigger, effect, registration.Reason().String())
			return message, code, nil
		}, nil
	case commandAllocationForm:
		tenant, registration, err := registrationjson.AllocationFormFromJSON(raw)
		if err != nil {
			return nil, err
		}
		return func(ctx context.Context, registrar registrar) (string, int, error) {
			effect, err := registrar.forms.RegisterAllocationForm(ctx, tenant, registration)
			if err != nil {
				return "", 0, err
			}
			message, code := catalogueAnswer(commandAllocationForm, effect, registration.Rule().String())
			return message, code, nil
		}, nil
	case commandChargeConfirmationFacts:
		tenant, registration, err := registrationjson.ChargeConfirmationFactsFromJSON(raw)
		if err != nil {
			return nil, err
		}
		return func(ctx context.Context, registrar registrar) (string, int, error) {
			effect, err := registrar.catalogues.RegisterChargeConfirmationFacts(ctx, tenant, registration)
			if err != nil {
				return "", 0, err
			}
			message, code := catalogueAnswer(commandChargeConfirmationFacts, effect, registration.Charge().String())
			return message, code, nil
		}, nil
	case commandChargeAttribution:
		tenant, registration, err := registrationjson.ChargeAttributionFromJSON(raw)
		if err != nil {
			return nil, err
		}
		return func(ctx context.Context, registrar registrar) (string, int, error) {
			effect, err := registrar.attributions.RegisterChargeAttribution(ctx, tenant, registration)
			if err != nil {
				return "", 0, err
			}
			message, code := catalogueAnswer(commandChargeAttribution, effect, registration.FeeItem().String())
			return message, code, nil
		}, nil
	case commandStatementArrangement:
		tenant, arrangement, err := registrationjson.StatementArrangementFromJSON(raw)
		if err != nil {
			return nil, err
		}
		return func(ctx context.Context, registrar registrar) (string, int, error) {
			effect, err := registrar.arrangements.RegisterStatementArrangement(ctx, tenant, arrangement)
			if err != nil {
				return "", 0, err
			}
			message, code := catalogueAnswer(commandStatementArrangement, effect, arrangement.Account().String())
			return message, code, nil
		}, nil
	case commandPeriodicFee:
		tenant, registration, err := registrationjson.PeriodicFeeFromJSON(raw)
		if err != nil {
			return nil, err
		}
		return func(ctx context.Context, registrar registrar) (string, int, error) {
			effect, err := registrar.fees.RegisterPeriodicFee(ctx, tenant, registration)
			if err != nil {
				return "", 0, err
			}
			message, code := catalogueAnswer(commandPeriodicFee, effect, registration.Rule().String())
			return message, code, nil
		}, nil
	case commandSettlementMoment:
		tenant, moment, err := registrationjson.SettlementMomentFromJSON(raw)
		if err != nil {
			return nil, err
		}
		return func(ctx context.Context, registrar registrar) (string, int, error) {
			effect, err := registrar.moments.RegisterSettlementMoment(ctx, tenant, moment)
			if err != nil {
				return "", 0, err
			}
			message, code := catalogueAnswer(commandSettlementMoment, effect, moment.String())
			return message, code, nil
		}, nil
	case commandChargeConfirmationCondition:
		tenant, feeItem, kind, err := registrationjson.ChargeConfirmationConditionFromJSON(raw)
		if err != nil {
			return nil, err
		}
		return func(ctx context.Context, registrar registrar) (string, int, error) {
			effect, err := registrar.conditions.RegisterChargeConfirmationCondition(ctx, tenant, feeItem, kind)
			if err != nil {
				return "", 0, err
			}
			message, code := catalogueAnswer(commandChargeConfirmationCondition, effect, feeItem.String())
			return message, code, nil
		}, nil
	case commandSellEvaluationTrigger:
		tenant, registration, err := registrationjson.SellEvaluationTriggerFromJSON(raw)
		if err != nil {
			return nil, err
		}
		return func(ctx context.Context, registrar registrar) (string, int, error) {
			effect, err := registrar.sellTriggers.RegisterSellEvaluationTrigger(ctx, tenant, registration)
			if err != nil {
				return "", 0, err
			}
			message, code := catalogueAnswer(commandSellEvaluationTrigger, effect, registration.Reason().String())
			return message, code, nil
		}, nil
	case commandEvaluationResume:
		resume, err := registrationjson.EvaluationResumeFromJSON(raw)
		if err != nil {
			return nil, err
		}
		return func(ctx context.Context, registrar registrar) (string, int, error) {
			result, err := registrar.resumes.Handle(ctx, resume)
			if err != nil {
				return "", 0, err
			}
			message, code := resumeAnswer(commandEvaluationResume, result)
			if code == exitUndecided {
				// 交回错误让 execute 回滚这一笔：已续办的那几件随之撤回，同一个续办引用重跑时再办，不留半份。
				return message, code, fmt.Errorf("%w: %s", errEvaluationResumeUndecided, message)
			}
			return message, code, nil
		}, nil
	default:
		return nil, fmt.Errorf("未知登记命令 %q（支持 %s）", command, strings.Join(allCommands, " / "))
	}
}

// errEvaluationResumeUndecided 让运营续办在某一件未决时整笔回滚；execute 把它译成未决的退出码。
var errEvaluationResumeUndecided = errors.New("evaluation resume undecided")
