package domain

import (
	"errors"
	"fmt"
)

// ErrInvalidPricePolicyCaliber 拒绝立不住的价格政策口径声明。
var ErrInvalidPricePolicyCaliber = errors.New("party commercial: invalid price policy caliber")

// ErrVolumetricFactorOnPricePolicy 拒价格政策口径上的体积系数，三个方向同一判法（ADR-0260 决定二）。它 Is
// ErrInvalidPricePolicyCaliber：带着系数的口径就是一份立不住的口径。registrationjson.pricePolicyCaliberDocument、
// http.PricePolicyCaliberPayload 与 canonicalPricePolicyCaliber 认得 volumetricFactor 这个键，只为在那里点名拒它——当
// 未知键整份拒，操作者拿到的是一句「不认识这个字段」，看不出这一格去了价卡；静默丢键，则批文以为登进去的比实际多。
var ErrVolumetricFactorOnPricePolicy = fmt.Errorf(
	"%w: a volumetric factor is a price card term and is not declared on a price policy in any direction (ADR-0260)",
	ErrInvalidPricePolicyCaliber)

// PricePolicyCaliber 是一个价格规则版本声明的计价口径正文：税务口径，以及可缺的汇率口径（CONTEXT
// 「商业价格政策版本」词条：政策可以声明汇率口径）。
//
// 它与 CommercialPricePolicy 分开，不并进那个结构体：后者表达的是**选用时要观察的正文**（方向、
// 方案、范围、区间），口径不参与选用——ADR-0057 Decision 三给发布期邻接答复的理由对口径同样
// 成立。分开之后口径走族 B（按版本点读，不进整册与 ViewRevision），与信用政策、供应商协议正文
// 同一条路（票 party-commercial-context-gaps/03/06）。
//
// 税务一格必需：CONTEXT「必须声明含税、未税或税务不适用」。汇率一格**可缺**——不涉及外币的政策
// 没有汇率口径，缺席是一句合法的商业声明；fxDeclared 让它与「忘了填」分得开，后者只能经
// NewPricePolicyCaliberWithFx 传零值进来，那里拒。
//
// **没有体积系数一格，且不是漏掉。** 体积系数与金额取整是价卡内容、不分方向（ADR-0260 决定二）：
// 同一格若在卡与政策上各有一处合法声明，计价只读卡上那一处，政策上那一处静默不起作用——租户按合同
// 调了政策上的系数而不改卡，计费重量就与预期不符而没有任何地方报错。所以这一格在政策上任何方向都
// 不收（ErrVolumetricFactorOnPricePolicy）；0041 迁移之前按销售方向登下的系数原样留在册上，读面标「不参与计价」
// （ports.PricePolicyRow.HistoricalVolumetricFactor）。
//
// direction 是拥有本口径的政策的方向，不是口径自己的一条声明：发布编排从政策正文抄给它，输入面上没有第二个
// 方向可以与正文对不上；口径行以复合外键钉在同方向的政策正文行上（0022 迁移），绕开编排自己造口径的调用方
// 在库上被拦。
//
// 加点规则不在这里，且不是漏掉：票 02 裁 (a)，理由与重启条件见 FxCaliber 的注释。
type PricePolicyCaliber struct {
	version    CommercialVersion
	direction  PriceDirection
	tax        TaxCaliber
	fx         FxCaliber
	fxDeclared bool
}

// NewPricePolicyCaliber 形成一份不含汇率口径的口径声明。
func NewPricePolicyCaliber(
	version CommercialVersion,
	direction PriceDirection,
	tax TaxCaliber,
) (PricePolicyCaliber, error) {
	if !caliberOwnerUsable(version) || !direction.valid() || !tax.valid() {
		return PricePolicyCaliber{}, ErrInvalidPricePolicyCaliber
	}
	return PricePolicyCaliber{version: version, direction: direction, tax: tax}, nil
}

// NewPricePolicyCaliberWithFx 形成一份含汇率口径的口径声明。零值 fx 在这里拒——要表达「没有
// 汇率口径」走 NewPricePolicyCaliber，两条路让缺席与缺件在构造上就分开。
func NewPricePolicyCaliberWithFx(
	version CommercialVersion,
	direction PriceDirection,
	tax TaxCaliber,
	fx FxCaliber,
) (PricePolicyCaliber, error) {
	caliber, err := NewPricePolicyCaliber(version, direction, tax)
	if err != nil {
		return PricePolicyCaliber{}, err
	}
	if !fx.valid() {
		return PricePolicyCaliber{}, ErrInvalidPricePolicyCaliber
	}
	caliber.fx = fx
	caliber.fxDeclared = true
	return caliber, nil
}

// caliberOwnerUsable 与 NewCommercialPricePolicy 对拥有版本的判据同一条：类别是价格规则且已生效。
func caliberOwnerUsable(version CommercialVersion) bool {
	return version.kind == PriceRuleObject && version.status == CommercialVersionEffective
}

func (caliber PricePolicyCaliber) Version() CommercialVersion {
	return caliber.version
}

// Direction 是拥有本口径的政策的方向（见类型注释）。
func (caliber PricePolicyCaliber) Direction() PriceDirection {
	return caliber.direction
}

func (caliber PricePolicyCaliber) Tax() TaxCaliber {
	return caliber.tax
}

// Fx 的第二个返回值分「没声明汇率口径」与「声明了」。不带它的话，一份零值 FxCaliber 读起来
// 与「牌价类型为空」一样，而前者是合法缺席、后者造不出来。
func (caliber PricePolicyCaliber) Fx() (FxCaliber, bool) {
	return caliber.fx, caliber.fxDeclared
}

func (caliber TaxCaliber) valid() bool {
	switch caliber.disposition {
	case TaxInclusive, TaxExclusive:
		return caliber.classification.valid()
	case TaxNotApplicable:
		return !caliber.classification.valid()
	default:
		return false
	}
}

func (caliber FxCaliber) valid() bool {
	return caliber.quoteType.valid() && caliber.asOfSemantics.valid() && caliber.asOfPolicyVersion.valid()
}
