// Package labelfinal 拥有面单渠道服务终局判断各路触发（TF 首次有效收寄到达、面单交易定案 / 后续动作
// 两拍、受控关闭 / 重开决定生效；哪几路由 ADR-0134 定）**共用的处理方核**与那一套消费结论翻译（ADR-0134
// 决定二）。各路只有译码与取事实那一层不同：面单交易那一路（lc/26）不带收寄事实，关闭 / 重开那一路（lc/27）也不带；
// 收寄那一路（lc/25）在核之前多一段「按信封所指版本取回 TF 事实、核有效时间」并把引用折进命令——
// 那一段在 adapters/transportfulfillment 的 JudgeOnCarrierFirstEffectivePickupAdapter（它要认 TF 的键与记录形状，
// 本包不 import 提供方），核与翻译表不另写第二份。
//
// 它放在 parcel-shipment 的适配器层而不是 platform：它认得 psapplication 的封闭结果集合。
package labelfinal

import (
	"context"
	"errors"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/finalconsume"
	psapplication "go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

var (
	// ErrUntranslatableEnvelope 表示信封里的串译不成本上下文的标识——引用坏了，不是等谁。
	ErrUntranslatableEnvelope = errors.New(
		"parcel shipment label final: envelope carries an untranslatable reference")
	// ErrParcelTargetNotFound 表示按（租户 + 包裹）反查不到当前已接受委托。它与交付那一路的同名哨兵
	// 同一格：不猜委托、如实报；生产装配把它登进未决名单（委托可能尚未接受或尚未可见，重投会改变结果）。
	ErrParcelTargetNotFound = errors.New(
		"parcel shipment label final: no current accepted shipment request covers the parcel")
	// ErrJudgmentUndecided 表示判断编排停在自己的未决上——四个读口之一答不出。重投会改变结果，
	// 是未决哨兵，不是毒丸也不是缺陷。
	ErrJudgmentUndecided = errors.New(
		"parcel shipment label final: label service final judgment is undecided")
	// ErrJudgmentNotAccepted 表示命令立不起：信封与反查都过了还立不起，是适配器缺陷，响亮报错不吸收，
	// 不进未决名单——重投同样内容不自愈。
	ErrJudgmentNotAccepted = errors.New(
		"parcel shipment label final: judgment command was not accepted")
	// ErrUnexpectedJudgmentOutcome 表示应用层交回了封闭集合以外的结果。静默入账等于替编排作判断。
	ErrUnexpectedJudgmentOutcome = errors.New(
		"parcel shipment label final: unexpected label service final outcome")
	// ErrFinalUndecided 与 ErrFinalHandoffPending 是 finalconsume 同名哨兵的别名：形成了终局之后
	// 采用路径的两格由 finalconsume 一处回答，本包只暴露口。
	ErrFinalUndecided      = finalconsume.ErrFinalUndecided
	ErrFinalHandoffPending = finalconsume.ErrFinalHandoffPending
)

// LabelServiceFinalJudge 是判断编排的入口；生产装配接 psapplication.JudgeLabelServiceFinalHandler。
type LabelServiceFinalJudge interface {
	Handle(
		ctx context.Context,
		command psapplication.JudgeLabelServiceFinalCommand,
	) (psapplication.LabelServiceFinalResult, error)
}

// ParcelJudgmentCore 是各路共用的核：按（租户 + 包裹）经当前已接受投影反查目标委托 → 折
// JudgeLabelServiceFinalCommand → 判断 → 五值译成消费结论。
//
// 反查不中不猜委托（照交付适配器那条纪律）：载运对象可能是集运单元，今天不替它判。
type ParcelJudgmentCore struct {
	targets psports.CurrentAcceptedParcelTargetView
	judge   LabelServiceFinalJudge
}

func NewParcelJudgmentCore(
	targets psports.CurrentAcceptedParcelTargetView,
	judge LabelServiceFinalJudge,
) (*ParcelJudgmentCore, error) {
	if targets == nil {
		return nil, fmt.Errorf("parcel shipment label final: parcel target view is nil")
	}
	if judge == nil {
		return nil, fmt.Errorf("parcel shipment label final: label service final judge is nil")
	}
	return &ParcelJudgmentCore{targets: targets, judge: judge}, nil
}

// JudgeParcel 判一件包裹并把结果译成消费结论：nil 入账、error 回滚。
//
// pickup 零值即缺席（交易与关闭两路），判断走关闭路径；收寄那一路带它进来。租户串从信封带到
// 反查用同一个，避免信封租户与记录租户各说各话。
func (core *ParcelJudgmentCore) JudgeParcel(
	ctx context.Context,
	tenantRaw string,
	parcelRaw string,
	pickup psdomain.CarrierFirstEffectivePickupSpec,
) error {
	tenant, err := psdomain.NewTenantID(tenantRaw)
	if err != nil {
		return fmt.Errorf("%w: tenant: %v", ErrUntranslatableEnvelope, err)
	}
	parcel, err := psdomain.NewDeclaredParcelID(parcelRaw)
	if err != nil {
		return fmt.Errorf("%w: parcel: %v", ErrUntranslatableEnvelope, err)
	}

	target, found, err := core.targets.FindCurrentAcceptedByParcel(ctx, tenant, parcel)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("%w: parcel %q", ErrParcelTargetNotFound, parcel)
	}

	result, err := core.judge.Handle(ctx, psapplication.JudgeLabelServiceFinalCommand{
		Identity:             target.Identity(),
		ShipmentRequestID:    target.ShipmentRequestID(),
		Parcel:               parcel,
		FirstEffectivePickup: pickup,
	})
	if err != nil {
		return err
	}
	return Consumption(result)
}

// Consumption 把判断编排的五值译成消费门的两格（ADR-0029 按恢复动作分格，lc/25 做法 3 那张表）：
//
//   - LABEL_SERVICE_FINAL_ADOPTED → 判出终局并交了采用路径，采用各格由 finalconsume 一处回答；
//   - LABEL_SERVICE_NOT_FINAL / CANCELLATION_STANDS → 判断到了、不形成，无事可续，入账；
//   - JUDGMENT_UNDECIDED → 四个读口之一答不出，重投会改变结果，回滚待重投；
//   - REQUEST_NOT_ACCEPTED → 命令立不起，是适配器缺陷，响亮报错不吸收；
//   - 集合外 → 静默入账等于替编排作判断，不留 default 兜底。
func Consumption(result psapplication.LabelServiceFinalResult) error {
	switch result.Outcome() {
	case psapplication.LabelServiceFinalAdopted:
		adoption, delegated := result.Adoption()
		if !delegated {
			return fmt.Errorf("%w: adopted without an adoption result", ErrUnexpectedJudgmentOutcome)
		}
		return finalconsume.Consumption(adoption)
	case psapplication.LabelServiceNotFinalOutcome, psapplication.LabelServiceCancellationStandsOutcome:
		return nil
	case psapplication.LabelServiceJudgmentUndecided:
		return fmt.Errorf("%w: %s", ErrJudgmentUndecided, result.UndecidedReason())
	case psapplication.LabelServiceJudgmentNotAccepted:
		return ErrJudgmentNotAccepted
	default:
		return fmt.Errorf("%w: %q", ErrUnexpectedJudgmentOutcome, result.Outcome())
	}
}
