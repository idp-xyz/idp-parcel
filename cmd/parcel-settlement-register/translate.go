package main

import (
	"context"
	"fmt"
	"strings"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/registrationjson"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
)

// 本文件只把命令名分派到采用用例；「登记输入 JSON → 应用命令」的译装在
// `internal/settlementaccounting/adapters/registrationjson`——分家理由写在那个包的头注。命令名与退出码是
// 本入口自己的表面，因此留在这里。

// 封闭命令表：两条对齐采用编排的两条方法——首版 AdoptFact 与更正 CorrectFact。两条是两个命令类型，不共享
// 入口：更正命令上没有来源 / 付款人 / 种类 / 币种 / 发生时刻（从链头照抄），首版命令上没有回指；一条更正无
// 回指时不能退化成首版。映射（Map）、核销（Apply）与撤销（Reverse）三格今天**没有**命令：本票只开采用这一口
// （ADR-0137 决定四保留的那一口），核销面归 UC-SA-005 另一段的票。
//
// 用法文本与未知命令的错误文本都从 allCommands 生成，不各抄一遍（parcel-network-register supportedKinds 的教训）。
const (
	commandExternalFundsFact           = "external-funds-fact"
	commandExternalFundsFactCorrection = "external-funds-fact-correction"
)

var allCommands = []string{commandExternalFundsFact, commandExternalFundsFactCorrection}

// answer 是一个用例交回的封闭结果在本入口的译法。本口今天只有一族（采用编排的 FundsOutcome），接口仍留着：
// 退出码把各族按恢复动作归进同一张四格表，但格与格之间不互译。
type answer interface {
	exit(command string) (string, int)
}

type dispatchFunc func(ctx context.Context, registrar registrar) (answer, error)

// fundsCall 是采用编排的一次调用，由 adopted 包成 dispatchFunc。
type fundsCall func(ctx context.Context, registrar registrar) (application.FundsResult, error)

func adopted(call fundsCall) dispatchFunc {
	return func(ctx context.Context, registrar registrar) (answer, error) {
		result, err := call(ctx, registrar)
		return fundsResult{result: result}, err
	}
}

// commandFor 按命令译装输入，交回一个在事务内执行的调用。译装失败当场拒，不进事务——用法错误与「登记与否
// 未知」是两个退出码，让它进了事务就分不开了。
func commandFor(command string, raw []byte) (dispatchFunc, error) {
	switch command {
	case commandExternalFundsFact:
		translated, err := registrationjson.ExternalFundsFactFromJSON(raw)
		if err != nil {
			return nil, err
		}
		return adopted(func(ctx context.Context, registrar registrar) (application.FundsResult, error) {
			return registrar.funds.AdoptFact(ctx, translated)
		}), nil
	case commandExternalFundsFactCorrection:
		translated, err := registrationjson.ExternalFundsFactCorrectionFromJSON(raw)
		if err != nil {
			return nil, err
		}
		return adopted(func(ctx context.Context, registrar registrar) (application.FundsResult, error) {
			return registrar.funds.CorrectFact(ctx, translated)
		}), nil
	default:
		return nil, fmt.Errorf("未知登记命令 %q（支持 %s）", command, strings.Join(allCommands, " / "))
	}
}
