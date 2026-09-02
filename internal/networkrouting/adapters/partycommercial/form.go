package partycommercial

import (
	"errors"
	"fmt"

	nrdomain "go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	pcdomain "go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

var (
	// ErrUntranslatableAnswer 是提供方交回了本适配器翻译表没有落点的取值。全函数要求
	// default 报错不吸收（ADR-0025 / ADR-0050）：静默归入要求或不要求，就是把新形态
	// 做成了默认值。
	ErrUntranslatableAnswer = errors.New("network routing partycommercial adapter: untranslatable commercial answer")
	// ErrServiceProductUnavailable 是闭包里服务产品不可观察：没采用、或只登了版本没登
	// 产品。消费方必须把缺席当依赖不可用（ADR-0050 第三条），绝不能读成要求或不要求。
	ErrServiceProductUnavailable = errors.New("network routing partycommercial adapter: service product is not observable")
)

// eligibilityFromClosure 从已唯一解析的闭包翻译网络资格。键取闭包里已选出的那份产品
// （ADR-0050 第五条），不另开产品查询键。
func eligibilityFromClosure(closure pcdomain.CommercialClosure) (nrdomain.NetworkEligibility, error) {
	if closure.Outcome() != pcdomain.UniquelyResolved {
		return nrdomain.NetworkEligibility{}, fmt.Errorf("%w: closure outcome %q", ErrServiceProductUnavailable, closure.Outcome())
	}
	adopted, ok := closure.AdoptedFor(pcdomain.ServiceProductObject)
	if !ok {
		return nrdomain.NetworkEligibility{}, fmt.Errorf("%w: closure did not adopt a service product", ErrServiceProductUnavailable)
	}
	product, ok := adopted.ServiceProduct()
	if !ok {
		return nrdomain.NetworkEligibility{}, fmt.Errorf("%w: adopted product has no observable form", ErrServiceProductUnavailable)
	}
	return translateForm(product.Form())
}

func translateForm(form pcdomain.ServiceProductForm) (nrdomain.NetworkEligibility, error) {
	switch form {
	case pcdomain.NetworkServiceForm:
		return nrdomain.NewNetworkEligibility(nrdomain.NetworkJudgmentRequired, nrdomain.EligibilityBasisReference{})
	case pcdomain.LabelChannelServiceForm:
		// 面单渠道服务落`不要求`而不是`不可达`：NR CONTEXT「仅提供面单渠道服务时，不得虚构
		// 运营企业不控制的端到端网络路由」说的是这个问题不该问，不是问过了答案是否定的。
		//
		// 依据取形态自身的取值，本适配器不另铸一个字面量。EligibilityBasisReference 指名的是
		// **商业事实**，由消费侧写一个字符串就是替商业侧铸事实；而「已采用产品的服务形态是
		// 面单渠道服务」正是刚从闭包里读到的那条事实，复核时按依据维度答得出。
		basis, err := nrdomain.NewEligibilityBasisReference(form.String())
		if err != nil {
			return nrdomain.NetworkEligibility{}, fmt.Errorf("%w: %w", ErrUntranslatableAnswer, err)
		}
		return nrdomain.NewNetworkEligibility(nrdomain.NetworkJudgmentNotRequired, basis)
	default:
		return nrdomain.NetworkEligibility{}, fmt.Errorf("%w: service product form %q", ErrUntranslatableAnswer, form)
	}
}
