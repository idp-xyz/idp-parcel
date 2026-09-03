package domain

import "errors"

var (
	// ErrInvalidPricePolicyCaliber 拒绝立不住的价格政策口径声明。
	ErrInvalidPricePolicyCaliber = errors.New("party commercial: invalid price policy caliber")
	// ErrPricePolicyCaliberDirectionMismatch 是装配错误：口径里的体积方向与政策自己的方向不一致。
	// 它不是 ErrInvalidPricePolicyCaliber——口径自身形状合法，错的是它挂在了另一个方向的政策上。
	ErrPricePolicyCaliberDirectionMismatch = errors.New("party commercial: price policy caliber direction does not match the policy direction")
)

// PricePolicyCaliber 是一个价格规则版本声明的计价口径正文：税务口径、体积口径，以及可缺的
// 汇率口径（CONTEXT「它还声明计价所需的商业口径」）。
//
// 它与 CommercialPricePolicy 分开，不并进那个结构体：后者表达的是**选用时要观察的正文**（方向、
// 方案、范围、区间），口径不参与选用——ADR-0057 Decision 三给发布期邻接答复的理由对口径同样
// 成立。分开之后口径走族 B（按版本点读，不进整册与 ViewRevision），与信用政策、供应商协议正文
// 同一条路（票 party-commercial-context-gaps/03/06）。
//
// 税务与体积两格必需：CONTEXT「必须声明含税、未税或税务不适用」，体积口径按方向的必需与禁止
// 已在 VolumetricCaliber 里。汇率一格**可缺**——不涉及外币的政策没有汇率口径，缺席是一句合法的
// 商业声明；fxDeclared 让它与「忘了填」分得开，后者只能经 NewPricePolicyCaliberWithFx 传零值
// 进来，那里拒。
//
// 加点规则不在这里，且不是漏掉：票 02 裁 (a)，理由与重启条件见 FxCaliber 的注释。
type PricePolicyCaliber struct {
	version    CommercialVersion
	tax        TaxCaliber
	volumetric VolumetricCaliber
	fx         FxCaliber
	fxDeclared bool
}

// NewPricePolicyCaliber 形成一份不含汇率口径的口径声明。
func NewPricePolicyCaliber(
	version CommercialVersion,
	tax TaxCaliber,
	volumetric VolumetricCaliber,
) (PricePolicyCaliber, error) {
	if !caliberOwnerUsable(version) || !tax.valid() || !volumetric.valid() {
		return PricePolicyCaliber{}, ErrInvalidPricePolicyCaliber
	}
	return PricePolicyCaliber{version: version, tax: tax, volumetric: volumetric}, nil
}

// NewPricePolicyCaliberWithFx 形成一份含汇率口径的口径声明。零值 fx 在这里拒——要表达「没有
// 汇率口径」走 NewPricePolicyCaliber，两条路让缺席与缺件在构造上就分开。
func NewPricePolicyCaliberWithFx(
	version CommercialVersion,
	tax TaxCaliber,
	volumetric VolumetricCaliber,
	fx FxCaliber,
) (PricePolicyCaliber, error) {
	caliber, err := NewPricePolicyCaliber(version, tax, volumetric)
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

func (caliber PricePolicyCaliber) Tax() TaxCaliber {
	return caliber.tax
}

func (caliber PricePolicyCaliber) Volumetric() VolumetricCaliber {
	return caliber.volumetric
}

// Fx 的第二个返回值分「没声明汇率口径」与「声明了」。不带它的话，一份零值 FxCaliber 读起来
// 与「牌价类型为空」一样，而前者是合法缺席、后者造不出来。
func (caliber PricePolicyCaliber) Fx() (FxCaliber, bool) {
	return caliber.fx, caliber.fxDeclared
}

// ConsistentWithDirection 核口径里的体积方向与政策方向一致。口径与政策正文是两张表、两条
// 写入，但必须同一次发布登记；这道核在发布编排里走，装载读回后消费方再走一遍（判据同
// ConsistentAcceptanceRulePackage：两处各写各的就会把 A 方向的系数装进 B 方向的政策）。
func (caliber PricePolicyCaliber) ConsistentWithDirection(policyDirection PriceDirection) error {
	if caliber.volumetric.direction != policyDirection {
		return ErrPricePolicyCaliberDirectionMismatch
	}
	return nil
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

func (caliber VolumetricCaliber) valid() bool {
	switch caliber.direction {
	case SellDirection:
		return caliber.factor.valid()
	case BuyDirection, InternalDirection:
		return !caliber.factor.valid()
	default:
		return false
	}
}

func (caliber FxCaliber) valid() bool {
	return caliber.quoteType.valid() && caliber.asOfSemantics.valid() && caliber.asOfPolicyVersion.valid()
}
