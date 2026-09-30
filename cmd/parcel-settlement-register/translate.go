package main

import (
	"context"
	"fmt"
	"strings"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/registrationjson"
)

// 本文件只把命令名分派到登记用例；「登记输入 JSON → 应用命令」的译装在
// `internal/settlementaccounting/adapters/registrationjson`——分家理由写在那个包的头注。命令名与退出码是
// 本入口自己的表面，因此留在这里。

// 封闭命令表：资金事实的首版与更正是两个命令类型，不共享入口——更正命令上没有来源 / 付款人 / 种类 / 币种 /
// 发生时刻（从链头照抄），首版命令上没有回指；一条更正无回指时不能退化成首版。结算账户是第三族命令，
// 登记的是固定属性那一行，不走采用编排。映射（Map）、核销（Apply）与撤销（Reverse）没有命令：
// 核销面归 UC-SA-005 另一段的票。
//
// 用法文本与未知命令的错误文本都从 allCommands 生成，不各抄一遍（parcel-network-register supportedKinds 的教训）。
const (
	commandExternalFundsFact           = "external-funds-fact"
	commandExternalFundsFactCorrection = "external-funds-fact-correction"
	commandSettlementAccount           = "settlement-account"
	commandSupplierAuditAuthority      = "supplier-audit-authority"
	commandSupplierPayableAccount      = "supplier-payable-account"
	commandClaimAmountRule             = "claim-amount-rule"
	commandChargeConfirmationFacts     = "charge-confirmation-facts"
	commandAmountGrammar               = "amount-grammar"
	commandAllocationForm              = "allocation-form"
	commandAuditEscalationCeiling      = "audit-escalation-ceiling"
)

var allCommands = []string{
	commandExternalFundsFact,
	commandExternalFundsFactCorrection,
	commandSettlementAccount,
	commandSupplierAuditAuthority,
	commandSupplierPayableAccount,
	commandClaimAmountRule,
	commandChargeConfirmationFacts,
	commandAmountGrammar,
	commandAllocationForm,
	commandAuditEscalationCeiling,
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
				result.FundsHandoffReference(), subjectOf(result))
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
	default:
		return nil, fmt.Errorf("未知登记命令 %q（支持 %s）", command, strings.Join(allCommands, " / "))
	}
}
