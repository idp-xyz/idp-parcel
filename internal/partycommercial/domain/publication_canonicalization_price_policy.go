package domain

import (
	"fmt"
	"time"
)

// 本文件是价格规则册（PRICE_RULE）接进服务端规范化的那一格（加册不换号，仍是 PCC-1——ADR-0126 Decision 一；票
// admin-write-faces/14）。正文两层：0010 的政策正文（方向 × 方案绑定 × 政策自己的范围与区间，连同发布当时保全的
// planDirection 与 conversion，ADR-0057）与 0022 的口径（税务、体积、可缺的汇率）。口径随正文同笔登记是发布编排的
// 纪律（ports.PricePolicyRow.HasCaliber 的注释），所以口径嵌在正文一格里、不另开一格：「只给口径不给正文」在结构上
// 就写不出来，「先发正文、回头补口径」的两步也没有地方落。

// PricePolicyBody 是价格规则版本的正文输入面，以领域值对象给出：就是 NewCommercialPricePolicy 收的那几项，只是不带
// 拥有它的（已生效）版本——预览与录入发生在发布之前，那时没有已生效版本可挂。
//
// PlanDirection 与 Conversion 是发布当时 parcel-pricing 的答复与当时声明的转换（ADR-0057）：表单如实收、这里如实盖进
// 摘要，不从方案反推——同一价格规则版本改挂另一种方案方向或另一种转换是另一次发布内容（ADR-0057 Decision 四），
// 摘要因此必须把它们算进去。Caliber 可缺：0010 早于 0022，只有正文没有口径的行是合法状态；缺席不折进文档。
type PricePolicyBody struct {
	Direction     PriceDirection
	PricingPlan   PricingPlanReference
	PlanDirection PriceDirection
	Conversion    PlanBindingConversion
	Scope         CommercialScopeReference
	Effective     EffectiveInterval
	Caliber       *PricePolicyCaliberBody
}

// PricePolicyCaliberBody 是口径那一节的输入面：税务与体积必需，汇率可缺——不涉及外币的政策没有汇率口径，nil 就是
// 「没声明」而不是零口径（判据同 application.PricePolicyCaliberDeclaration）。
type PricePolicyCaliberBody struct {
	Tax        TaxCaliber
	Volumetric VolumetricCaliber
	Fx         *FxCaliber
}

// validate 在折成文档前过与发布时相同的门：NewCommercialPricePolicy 的逐格非零与 checkPlanBinding，口径走
// NewPricePolicyCaliber / NewPricePolicyCaliberWithFx 的判据与 ConsistentWithDirection——只少拥有版本那一判。不另造
// 校验：SELL 政策绑 BUY 方案而未声明转换（AT-PC-033）要在预览上就答给操作者，不能等到发布那一刻。
func (body PricePolicyBody) validate() error {
	if !body.Direction.valid() || !body.PricingPlan.valid() || !body.PlanDirection.valid() ||
		!body.Conversion.valid() || !body.Scope.valid() || !body.Effective.valid() {
		return ErrInvalidPricePolicy
	}
	if err := checkPlanBinding(body.Direction, body.PlanDirection, body.Conversion); err != nil {
		return err
	}
	if body.Caliber != nil {
		return body.Caliber.validate(body.Direction)
	}
	return nil
}

// validate 与 NewPricePolicyCaliber / NewPricePolicyCaliberWithFx 同判据：税务与体积各自立得住、汇率在场时立得住
// （指向零值的 Fx 是缺件不是缺席，拒）；体积口径的方向必须与政策方向一致，否则 A 方向的系数会装进 B 方向的政策。
func (caliber PricePolicyCaliberBody) validate(policyDirection PriceDirection) error {
	if !caliber.Tax.valid() || !caliber.Volumetric.valid() || (caliber.Fx != nil && !caliber.Fx.valid()) {
		return ErrInvalidPricePolicyCaliber
	}
	if caliber.Volumetric.direction != policyDirection {
		return ErrPricePolicyCaliberDirectionMismatch
	}
	return nil
}

// canonicalizePricePolicy 是 CanonicalizePublicationContent 里价格规则那一支：正文缺席与立不住的正文各答自己那一格
// （补正文 / 改正文），不折成一个「算不出」。
func canonicalizePricePolicy(content PublicationContent) (CanonicalPublicationContent, error) {
	if content.PricePolicy == nil {
		return CanonicalPublicationContent{}, ErrPublicationContentAbsent
	}
	if err := content.PricePolicy.validate(); err != nil {
		return CanonicalPublicationContent{}, err
	}
	return canonicalDigestOf(canonicalPublicationDocument{
		Canonicalization: publicationCanonicalizationVersion,
		Kind:             content.Kind.String(),
		PricePolicy:      canonicalPricePolicyBodyOf(*content.PricePolicy),
	})
}

// canonicalPricePolicyBody 镜像批文 pricePolicyBodyDocument 的键名：区间上界可缺、口径节可缺。时刻一律 UTC RFC 3339
// 纳秒，与其余各册同一格式。
type canonicalPricePolicyBody struct {
	Direction         string                       `json:"direction"`
	PricingPlan       string                       `json:"pricingPlan"`
	PlanDirection     string                       `json:"planDirection"`
	Conversion        string                       `json:"conversion"`
	Scope             string                       `json:"scope"`
	EffectiveStartsAt string                       `json:"effectiveStartsAt"`
	EffectiveEndsAt   string                       `json:"effectiveEndsAt,omitempty"`
	Caliber           *canonicalPricePolicyCaliber `json:"caliber,omitempty"`
}

// canonicalPricePolicyCaliber 镜像批文 pricePolicyCaliberDocument：没有方向键——体积口径的方向就是政策的方向，文档里再写
// 一遍只会造出「两个方向对不上」这种本不该存在的输入。三处可缺的键各是口径说出的真话（不适用因而没有分类、采购方向
// 因而没有系数、不涉外币因而没有汇率），缺席即键不在场，不补空串。
type canonicalPricePolicyCaliber struct {
	TaxDisposition    string              `json:"taxDisposition"`
	TaxClassification string              `json:"taxClassification,omitempty"`
	VolumetricFactor  string              `json:"volumetricFactor,omitempty"`
	Fx                *canonicalFxCaliber `json:"fx,omitempty"`
}

// canonicalFxCaliber 镜像批文 fxCaliberDocument：三格缺一不可，由 NewFxCaliber 拒。
type canonicalFxCaliber struct {
	QuoteType         string `json:"quoteType"`
	AsOfSemantics     string `json:"asOfSemantics"`
	AsOfPolicyVersion string `json:"asOfPolicyVersion"`
}

func canonicalPricePolicyBodyOf(body PricePolicyBody) *canonicalPricePolicyBody {
	document := &canonicalPricePolicyBody{
		Direction:         body.Direction.String(),
		PricingPlan:       body.PricingPlan.String(),
		PlanDirection:     body.PlanDirection.String(),
		Conversion:        body.Conversion.String(),
		Scope:             body.Scope.String(),
		EffectiveStartsAt: canonicalTime(body.Effective.StartsAt()),
	}
	if endsAt, bounded := body.Effective.EndsAt(); bounded {
		document.EffectiveEndsAt = canonicalTime(endsAt)
	}
	if body.Caliber != nil {
		document.Caliber = canonicalPricePolicyCaliberOf(*body.Caliber)
	}
	return document
}

func canonicalPricePolicyCaliberOf(caliber PricePolicyCaliberBody) *canonicalPricePolicyCaliber {
	document := &canonicalPricePolicyCaliber{TaxDisposition: caliber.Tax.Disposition().String()}
	if classification, applies := caliber.Tax.Classification(); applies {
		document.TaxClassification = classification.String()
	}
	if factor, declared := caliber.Volumetric.Factor(); declared {
		document.VolumetricFactor = factor.String()
	}
	if caliber.Fx != nil {
		document.Fx = &canonicalFxCaliber{
			QuoteType:         caliber.Fx.QuoteType().String(),
			AsOfSemantics:     caliber.Fx.AsOfSemantics().String(),
			AsOfPolicyVersion: caliber.Fx.AsOfPolicyVersion().String(),
		}
	}
	return document
}

// body 把文档里的一节折回领域正文。每一格过构造门，封闭集按 String() 原词反查；折回后再过一遍 validate——快照是
// 数据，正文立不立得住仍由构造门说（判据同客户合同那一节）。
func (document canonicalPricePolicyBody) body() (PricePolicyBody, error) {
	direction, known := PriceDirectionNamed(document.Direction)
	if !known {
		return PricePolicyBody{}, fmt.Errorf("direction: %w: %q", ErrInvalidPricePolicy, document.Direction)
	}
	pricingPlan, err := NewPricingPlanReference(document.PricingPlan)
	if err != nil {
		return PricePolicyBody{}, fmt.Errorf("pricingPlan: %w", err)
	}
	planDirection, known := PriceDirectionNamed(document.PlanDirection)
	if !known {
		return PricePolicyBody{}, fmt.Errorf("planDirection: %w: %q", ErrInvalidPricePolicy, document.PlanDirection)
	}
	conversion, known := PlanBindingConversionNamed(document.Conversion)
	if !known {
		return PricePolicyBody{}, fmt.Errorf("conversion: %w: %q", ErrInvalidPricePolicy, document.Conversion)
	}
	scope, err := NewCommercialScopeReference(document.Scope)
	if err != nil {
		return PricePolicyBody{}, fmt.Errorf("scope: %w", err)
	}
	startsAt, err := time.Parse(time.RFC3339Nano, document.EffectiveStartsAt)
	if err != nil {
		return PricePolicyBody{}, fmt.Errorf("effectiveStartsAt: %w", err)
	}
	var endsAt time.Time
	if document.EffectiveEndsAt != "" {
		if endsAt, err = time.Parse(time.RFC3339Nano, document.EffectiveEndsAt); err != nil {
			return PricePolicyBody{}, fmt.Errorf("effectiveEndsAt: %w", err)
		}
	}
	effective, err := NewEffectiveInterval(startsAt, endsAt)
	if err != nil {
		return PricePolicyBody{}, err
	}
	body := PricePolicyBody{
		Direction:     direction,
		PricingPlan:   pricingPlan,
		PlanDirection: planDirection,
		Conversion:    conversion,
		Scope:         scope,
		Effective:     effective,
	}
	if document.Caliber != nil {
		caliber, err := document.Caliber.body(direction)
		if err != nil {
			return PricePolicyBody{}, fmt.Errorf("caliber: %w", err)
		}
		body.Caliber = &caliber
	}
	if err := body.validate(); err != nil {
		return PricePolicyBody{}, err
	}
	return body, nil
}

// body 把口径一节折回领域值对象。体积口径的方向取政策方向——文档里没有方向键，所以从快照折不出方向不一致的口径；
// 分类与系数两格的在场规则由 NewTaxCaliber / NewVolumetricCaliber 判，与库上 CHECK 同形。
func (document canonicalPricePolicyCaliber) body(direction PriceDirection) (PricePolicyCaliberBody, error) {
	disposition, known := TaxDispositionNamed(document.TaxDisposition)
	if !known {
		return PricePolicyCaliberBody{}, fmt.Errorf("taxDisposition: %w: %q", ErrInvalidTaxCaliber, document.TaxDisposition)
	}
	var classification TaxClassificationReference
	if document.TaxClassification != "" {
		var err error
		if classification, err = NewTaxClassificationReference(document.TaxClassification); err != nil {
			return PricePolicyCaliberBody{}, fmt.Errorf("taxClassification: %w", err)
		}
	}
	tax, err := NewTaxCaliber(disposition, classification)
	if err != nil {
		return PricePolicyCaliberBody{}, fmt.Errorf("taxDisposition %q with classification %q: %w", document.TaxDisposition, document.TaxClassification, err)
	}
	var factor VolumetricFactorReference
	if document.VolumetricFactor != "" {
		if factor, err = NewVolumetricFactorReference(document.VolumetricFactor); err != nil {
			return PricePolicyCaliberBody{}, fmt.Errorf("volumetricFactor: %w", err)
		}
	}
	volumetric, err := NewVolumetricCaliber(direction, factor)
	if err != nil {
		return PricePolicyCaliberBody{}, fmt.Errorf("volumetricFactor %q under direction %s: %w", document.VolumetricFactor, direction, err)
	}
	caliber := PricePolicyCaliberBody{Tax: tax, Volumetric: volumetric}
	if document.Fx != nil {
		quoteType, err := NewFxQuoteTypeReference(document.Fx.QuoteType)
		if err != nil {
			return PricePolicyCaliberBody{}, fmt.Errorf("fx.quoteType: %w", err)
		}
		semantics, err := NewAsOfSemanticsReference(document.Fx.AsOfSemantics)
		if err != nil {
			return PricePolicyCaliberBody{}, fmt.Errorf("fx.asOfSemantics: %w", err)
		}
		policyVersion, err := NewAsOfPolicyVersion(document.Fx.AsOfPolicyVersion)
		if err != nil {
			return PricePolicyCaliberBody{}, fmt.Errorf("fx.asOfPolicyVersion: %w", err)
		}
		fx, err := NewFxCaliber(quoteType, semantics, policyVersion)
		if err != nil {
			return PricePolicyCaliberBody{}, fmt.Errorf("fx: %w", err)
		}
		caliber.Fx = &fx
	}
	return caliber, nil
}

// PriceDirectionNamed 按 String() 的原词反查价格方向：规范化文档里的 direction / planDirection、运营操作者面载荷里的
// 同两格都是那一个词，名单在 String() 一处，这里只是反查（判据同 CommercialObjectKindNamed）。集合外含空串答 false。
func PriceDirectionNamed(name string) (PriceDirection, bool) {
	for direction := BuyDirection; direction.valid(); direction++ {
		if direction.String() == name {
			return direction, true
		}
	}
	return PriceDirectionInvalid, false
}

// PlanBindingConversionNamed 按 String() 原词反查方案绑定转换。空串答 false 而不折成 NONE：SELL 政策绑 BUY 方案而没写
// 转换该由 checkPlanBinding 报出`适用冲突`，代填 NONE 会把「没写」与「明说不转换」混在一起（判据同受控批文的
// planBindingConversionFrom）。
func PlanBindingConversionNamed(name string) (PlanBindingConversion, bool) {
	for conversion := PlanBindingConversionNone; conversion.valid(); conversion++ {
		if conversion.String() == name {
			return conversion, true
		}
	}
	return PlanBindingConversionNone, false
}

// TaxDispositionNamed 按 String() 原词反查税务口径。零值哨兵不落进「不适用」，与领域同判据：缺席与已判定为不适用
// 要人做的事不同，前者去补声明。
func TaxDispositionNamed(name string) (TaxDisposition, bool) {
	for disposition := TaxInclusive; disposition <= TaxNotApplicable; disposition++ {
		if disposition.String() == name {
			return disposition, true
		}
	}
	return TaxDispositionInvalid, false
}
