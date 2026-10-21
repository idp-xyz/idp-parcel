package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	bentoapp "go.idp.xyz/idp-bento-go/application"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/application"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// 价卡收尾的两种登记（ADR-0234）：price-card-cut 把旧版截止在后继的起点，price-card-retirement 让旧版整段让位给起点不晚于
// 它的后继。输入是一份收尾文档（租户、定价方案、旧版、后继、依据），与在线两口的载荷同形、多 tenant 一格；截止点与退役时刻
// 都由服务端定，文档里出现即拒。
const (
	kindPriceCardCut        = "price-card-cut"
	kindPriceCardRetirement = "price-card-retirement"
)

// priceCardCloser 把收尾用例收窄成本工具消费的形状，测试用替身顶上。
type priceCardCloser interface {
	Cut(ctx context.Context, command application.CutPriceCardVersionCommand) (application.PriceCardClosureResult, error)
	RetireAtCoveringSuccessor(ctx context.Context, command application.RetirePriceCardVersionCommand) (application.PriceCardClosureResult, error)
}

var _ priceCardCloser = (*application.ClosePriceCardVersionHandler)(nil)

// closureDocument 是收尾文档的线格式。
type closureDocument struct {
	Tenant           string `json:"tenant"`
	PlanID           string `json:"planId"`
	Version          string `json:"version"`
	SuccessorVersion string `json:"successorVersion"`
	Reference        string `json:"reference"`
}

// parseClosureDocument 只做形状翻译，封闭解码：多出来的键（截止点、退役时刻一类）不是被忽略，而是答用法错——忽略了，
// 登记方会以为自己定了截止点。各格成不成立由用例判。
func parseClosureDocument(raw []byte) (application.PriceCardClosureTarget, string, error) {
	var document closureDocument
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&document); err != nil {
		return application.PriceCardClosureTarget{}, "", fmt.Errorf("收尾文档不是合法 JSON 或带了未知键：%w", err)
	}
	if decoder.More() {
		return application.PriceCardClosureTarget{}, "", fmt.Errorf("收尾文档有尾随内容")
	}
	tenant, err := domain.NewTenantID(document.Tenant)
	if err != nil {
		return application.PriceCardClosureTarget{}, "", fmt.Errorf("收尾文档 tenant：%w", err)
	}
	return application.PriceCardClosureTarget{
		Tenant: tenant, PlanID: document.PlanID, Version: document.Version, Successor: document.SuccessorVersion,
	}, document.Reference, nil
}

// isClosureKind 答这一种登记是不是价卡收尾。
func isClosureKind(kind string) bool {
	return kind == kindPriceCardCut || kind == kindPriceCardRetirement
}

// executeClosure 把一份收尾文档推进到登记册答案：在事务内交收尾用例（锁、读、写与交信封同一笔），再把答案译成退出码。
func executeClosure(
	ctx context.Context,
	kind string,
	raw []byte,
	closer priceCardCloser,
	transactor bentoapp.Transactor,
) (string, int) {
	target, reference, err := parseClosureDocument(raw)
	if err != nil {
		return fmt.Sprintf("%s: %v", kind, err), exitUsage
	}
	var result application.PriceCardClosureResult
	err = transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		var handleErr error
		switch kind {
		case kindPriceCardCut:
			result, handleErr = closer.Cut(txCtx, application.CutPriceCardVersionCommand{PriceCardClosureTarget: target, Reference: reference})
		case kindPriceCardRetirement:
			result, handleErr = closer.RetireAtCoveringSuccessor(txCtx, application.RetirePriceCardVersionCommand{PriceCardClosureTarget: target, Reference: reference})
		default:
			handleErr = fmt.Errorf("unknown closure kind %q", kind)
		}
		return handleErr
	})
	if err != nil {
		return fmt.Sprintf("%s: 未决：%v", kind, err), exitUndecided
	}
	return describeClosure(kind, result), closureExitCode(result.Outcome())
}

// describeClosure 把答案与落下（或重放认出）的那一次收尾写成一行：截止写截到哪一刻、让位给谁，退役写何时起、让位给谁，
// 未受理写原因。
func describeClosure(kind string, result application.PriceCardClosureResult) string {
	message := kind + ": " + result.Outcome().String()
	if cut, held := result.Cut(); held {
		message += fmt.Sprintf("（截到 %s，让位给 %s）", cut.SelectionEndsAt().Format("2006-01-02T15:04:05Z07:00"), cut.Successor().Version())
	}
	if retirement, held := result.Retirement(); held {
		message += fmt.Sprintf("（%s 起退役，让位给 %s）", retirement.RetiredAt().Format("2006-01-02T15:04:05Z07:00"), retirement.Successor().Version())
	}
	if cause := result.Cause(); cause != nil {
		message += "：" + cause.Error()
	}
	return message
}

// closureExitCode：未受理是登记册的治理答案——什么都没写、也不是本工具用错了，操作者拿原因去改请求或先登后继。
func closureExitCode(outcome application.PriceCardClosureOutcome) int {
	switch outcome {
	case application.PriceCardClosureRecorded, application.PriceCardClosureAlreadyOnRegister:
		return exitRegistered
	case application.PriceCardClosureNotAccepted:
		return exitGovernance
	default:
		return exitUndecided
	}
}
