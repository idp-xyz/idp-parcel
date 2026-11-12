package domain

import (
	"fmt"
	"sort"
	"strings"
)

// ServiceProductReference 是一个服务产品在本上下文里的引用：参与方与商业的服务产品对象标识，不带版本（ADR-0382 决定一）。
// 售价卡用它声明自己计价的产品，选卡用它带委托申报的那一个。不带版本是因为产品换版（交付条件、规则包引用变了）不改它卖
// 什么价，带了就得每换一版重登一次价卡。本上下文不核它在不在册、不解释它：在不在册由商业解析答。零值即没有产品。
type ServiceProductReference struct{ identifier }

// serviceProductListSeparator 是价卡导入模板里产品列表的分隔符。标识里不许有它：否则同一格分号列表能拆出两种读法，一个产品
// 会被读成两个，或两个被读成一个。构造门守住它，任何一处解码（快照、模板、试算载荷、小包托运的申报）就都守住了。
const serviceProductListSeparator = ";"

func NewServiceProductReference(value string) (ServiceProductReference, error) {
	identifier, err := newIdentifier("service product reference", value)
	if err != nil {
		return ServiceProductReference{}, err
	}
	if strings.Contains(value, serviceProductListSeparator) {
		return ServiceProductReference{}, fmt.Errorf("%w: service product reference %q contains %q", ErrInvalidIdentifier, value, serviceProductListSeparator)
	}
	return ServiceProductReference{identifier: identifier}, nil
}

// Declared 报出这一格有没有产品。选卡收到零值即委托没申报，只看不声明产品的卡（ADR-0382 决定二）。
func (product ServiceProductReference) Declared() bool { return product.valid() }

// valid 比通用标识多一道：不含列表分隔符。读回门、输入快照门都经它，绕过构造门的快照同样拒。
func (product ServiceProductReference) valid() bool {
	return product.identifier.valid() && !strings.Contains(product.value, serviceProductListSeparator)
}

// ServiceProducts 交回卡声明的适用服务产品，按标识升序。空即不声明：这张卡接其余产品，即同一计价范围、方向与目的上时点没有
// 任何在用卡指名的那些产品（PP CONTEXT「适用服务产品」）。
func (plan PricingPlanVersion) ServiceProducts() []ServiceProductReference {
	return append([]ServiceProductReference(nil), plan.serviceProducts...)
}

// WithServiceProducts 交回声明了这组适用服务产品的副本，原有的声明整组换掉。至少一个，同一个写两遍按一个算，按标识排序后
// 保存：两张只差书写次序的卡是同一份内容，摘要不该因此分开。只有 SELL·客户费用的卡能声明（ADR-0382 决定一）：采购方向按
// 供应商协议的商业范围选卡，渠道比价另有来源，那两个方向上没有哪一次选卡会读这一格，收下它只会让摘要多出一处没人读的差异。
// 带产品的卡写 PPC-7（决定四），规范化版本与摘要随之重算。
func (plan PricingPlanVersion) WithServiceProducts(products ...ServiceProductReference) (PricingPlanVersion, error) {
	if !plan.valid() {
		return PricingPlanVersion{}, ErrInvalidPricingPlan
	}
	if plan.direction != PricingDirectionSell || plan.purpose != PricingPurposeCustomerCharge {
		return PricingPlanVersion{}, fmt.Errorf("%w: %s / %s", ErrServiceProductsOnlyOnSellCustomerCharge, plan.direction, plan.purpose)
	}
	if len(products) == 0 {
		return PricingPlanVersion{}, fmt.Errorf("%w: a declaration names at least one service product", ErrInvalidPricingPlan)
	}
	seen := make(map[string]struct{}, len(products))
	declared := make([]ServiceProductReference, 0, len(products))
	for _, product := range products {
		if !product.valid() {
			return PricingPlanVersion{}, fmt.Errorf("%w: %q", ErrInvalidIdentifier, product.value)
		}
		if _, repeated := seen[product.value]; repeated {
			continue
		}
		seen[product.value] = struct{}{}
		declared = append(declared, product)
	}
	sort.Slice(declared, func(left, right int) bool { return declared[left].value < declared[right].value })
	plan.serviceProducts = declared
	plan.canonicalization = plan.requiredCanonicalization()
	plan.contentDigest = calculatePricingPlanContentDigest(plan)
	return plan, nil
}

// serviceProductsHold 是读回门上的那一道：声明只在 SELL·客户费用的卡上，至少一个，按标识严格升序（升序即已去重）。读回不替
// 快照排序：摘要按保存的次序算，次序不对说明快照不是本构建写的，与规则次序不对同一个处置。写了键却一个都没列（非 nil 的空
// 切片，见 serviceProductsFrom）同样拒：缺键才是不声明，空列表是写的人以为声明了什么，读成「接其余产品」就替他改了意思。
func (plan PricingPlanVersion) serviceProductsHold() bool {
	if plan.serviceProducts != nil && len(plan.serviceProducts) == 0 {
		return false
	}
	if len(plan.serviceProducts) == 0 {
		return true
	}
	if plan.direction != PricingDirectionSell || plan.purpose != PricingPurposeCustomerCharge {
		return false
	}
	for index, product := range plan.serviceProducts {
		if !product.valid() {
			return false
		}
		if index > 0 && plan.serviceProducts[index-1].value >= product.value {
			return false
		}
	}
	return true
}

// requiredCanonicalization 取能表达这张卡内容的最低一版（ADR-0267 决定一）：声明了适用服务产品的写 PPC-7，其余照结构件取
// PPC-5 或 PPC-6，字节不动（ADR-0382 决定四）。
func (plan PricingPlanVersion) requiredCanonicalization() string {
	if len(plan.serviceProducts) > 0 {
		return canonicalizationVersionServiceProducts
	}
	return plan.structures.requiredCanonicalization()
}

func serviceProductTexts(products []ServiceProductReference) []string {
	if len(products) == 0 {
		return nil
	}
	texts := make([]string, 0, len(products))
	for _, product := range products {
		texts = append(texts, product.value)
	}
	return texts
}

// serviceProductsFrom 把快照里的产品列表读回。缺键（nil）读作不声明；写了键却是空列表读成非 nil 的空切片，留给
// serviceProductsHold 拒。
func serviceProductsFrom(texts []string) []ServiceProductReference {
	if texts == nil {
		return nil
	}
	products := make([]ServiceProductReference, 0, len(texts))
	for _, text := range texts {
		products = append(products, ServiceProductReference{identifier{value: text}})
	}
	return products
}
