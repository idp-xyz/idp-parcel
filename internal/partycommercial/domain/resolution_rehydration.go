package domain

import (
	"errors"
	"fmt"
)

// ErrInvalidRehydratedResolution 是解析重建入口因快照数据本身而拒绝时给出的理由。
// 与解析当时的领域错误分开：后者说「此刻解不出来」，前者说「这份已经固定过的东西
// 不可能是本上下文写出来的」（ADR-0028）。
var ErrInvalidRehydratedResolution = errors.New("party commercial: invalid rehydrated commercial resolution")

// RehydrateAdoptedBasisSpec 是闭包里一项已采用依据在库里的样子。版本身份始终固定；
// 服务产品（ADR-0050）、结算政策（ADR-0044）、信用依据（ADR-0127）与客户服务规则的继承底座（ADR-0268）
// 快照在场时一并重建。
//
// 价格政策不在其中：`NewCommercialPricePolicy` 要方案方向与跨向转换两个入参才立得起来，
// 而 `CommercialPricePolicy` 并不留存它们——照本形状重建就得跳过那道绑定校验，或者在
// 快照里再存一份只为过校验的输入。两条路都要先决定「已固定的价格政策还要不要重验绑定」，
// 那是一道决定，不是一段代码，留给它自己的票。
//
// 信用依据只收额度，不收整份 CreditBasis：CreditBasis 是 ResolveCreditPolicy 的产物，产物
// 没有构造函数（判断的输入有构造函数、产物没有）；出处就是本项采用的版本，快照里再存一份
// 就是给同一件事留两个可以互相打架的记录。
type RehydrateAdoptedBasisSpec struct {
	Kind                CommercialObjectKind
	Version             CommercialVersion
	ServiceProduct      ServiceProduct
	HasServiceProduct   bool
	SettlementPolicy    SettlementPolicy
	HasSettlementPolicy bool
	CreditLimit         CreditLimit
	HasCreditBasis      bool
	// ProductBaseFixed 说快照里这一项有没有底座一格（ADR-0268 决定一）；ProductBase 只在 HasProductBase 时有意义。
	// 三格分开收：固定了一版、固定了没有、没有这一格（ADR-0268 之前的快照，决定三），后两格不可互相顶替。
	ProductBase      CommercialVersion
	HasProductBase   bool
	ProductBaseFixed bool
}

// RehydrateCommercialClosureSpec 是一次已固定解析在库里的样子。字段一律当数据收下，
// 不重算解析标识（ADR-0028）。
type RehydrateCommercialClosureSpec struct {
	Outcome      ResolutionOutcome
	ResolutionID ResolutionID
	Key          ClosureResolutionKey
	Anchor       SelectionAnchor
	ViewRevision AuthorityViewRevision
	Adopted      []RehydrateAdoptedBasisSpec
}

// RehydrateCommercialClosure 从库里读到的产物重建一份唯一已解析的闭包。本票只固定
// 这一次解析，其它结局没有可回指的标识，重建门直接拒。
func RehydrateCommercialClosure(spec RehydrateCommercialClosureSpec) (CommercialClosure, error) {
	if spec.Outcome != UniquelyResolved {
		return CommercialClosure{}, rehydratedResolutionRefusal("本票只固定唯一已解析")
	}
	if !spec.ResolutionID.valid() || !spec.Key.minimumIdentityEstablished() ||
		!spec.Anchor.valid() || !spec.ViewRevision.valid() {
		return CommercialClosure{}, rehydratedResolutionRefusal("标识、解析键、锚点或视图修订缺失")
	}
	if spec.Key.Anchor.At() != spec.Anchor.At() ||
		spec.Key.Anchor.PolicyVersion() != spec.Anchor.PolicyVersion() {
		return CommercialClosure{}, rehydratedResolutionRefusal("闭包锚点与解析键锚点不一致")
	}
	if len(spec.Adopted) == 0 {
		return CommercialClosure{}, rehydratedResolutionRefusal("唯一解析没有采用依据")
	}

	adopted := make([]AdoptedBasis, 0, len(spec.Adopted))
	for _, item := range spec.Adopted {
		if item.Kind == CommercialObjectKindInvalid ||
			item.Version.status == CommercialVersionStatusInvalid ||
			item.Version.status == CommercialVersionDraft {
			return CommercialClosure{}, rehydratedResolutionRefusal("采用依据缺席或仍是草稿")
		}
		basis := AdoptedBasis{kind: item.Kind, version: item.Version}
		if item.HasServiceProduct {
			if item.Kind != ServiceProductObject ||
				item.ServiceProduct.version.objectID != item.Version.objectID ||
				item.ServiceProduct.version.version != item.Version.version ||
				!item.ServiceProduct.form.valid() {
				return CommercialClosure{}, rehydratedResolutionRefusal("服务产品快照与采用版本对不上")
			}
			basis.serviceProduct = item.ServiceProduct
			basis.hasServiceProduct = true
		}
		if item.HasSettlementPolicy {
			// 与服务产品同纪律：政策必须挂在结算政策这一格上，且正文所指的版本就是本项
			// 采用的那一版。允许两者不一致，闭包就会声称「采用了 A 版，方式按 B 版」——
			// 而调用方正是凭这份方式决定冻不冻款。
			if item.Kind != SettlementPolicyObject ||
				item.SettlementPolicy.version.objectID != item.Version.objectID ||
				item.SettlementPolicy.version.version != item.Version.version ||
				!item.SettlementPolicy.method.valid() {
				return CommercialClosure{}, rehydratedResolutionRefusal("结算政策快照与采用版本对不上")
			}
			basis.settlementPolicy = item.SettlementPolicy
			basis.hasSettlementPolicy = true
		}
		if item.HasCreditBasis {
			// 额度必须挂在信用政策这一格、且已声明。零值额度过不了 NewCreditPolicy，快照里出现它
			// 只能是绕开构造门写进去的；照收就会让「授予零信用」与「没有额度」读成同一个样子。
			if item.Kind != CreditPolicyObject || !item.CreditLimit.valid() {
				return CommercialClosure{}, rehydratedResolutionRefusal("信用依据快照与采用版本对不上")
			}
			basis.creditBasis = CreditBasis{
				policyVersion: item.Version,
				limit:         item.CreditLimit,
				applicable:    true,
			}
			basis.hasCreditBasis = true
		}
		if item.ProductBaseFixed || item.HasProductBase {
			if err := admitsFixedProductBase(item); err != nil {
				return CommercialClosure{}, err
			}
			basis.productBase = item.ProductBase
			basis.hasProductBase = item.HasProductBase
			basis.productBaseFixed = true
		}
		adopted = append(adopted, basis)
	}

	return CommercialClosure{
		outcome:      UniquelyResolved,
		resolutionID: spec.ResolutionID,
		key:          spec.Key,
		anchor:       spec.Anchor,
		viewRevision: spec.ViewRevision,
		adopted:      adopted,
	}, nil
}

// admitsFixedProductBase 拒本上下文写不出的底座形状：底座只随挂合同的那一版客户服务规则固定，它自己是同租户、
// 同范围、壳上不指名客户合同的客户服务规则（与 customerServiceRuleTiers 的产品层同一判据）。照收一份挂合同的底座，
// 层次读口就会把别户的条款拼进这一户；照收「在场却未固定」，读的人分不出它是不是 ADR-0268 之前的那一种。
func admitsFixedProductBase(item RehydrateAdoptedBasisSpec) error {
	if item.HasProductBase && !item.ProductBaseFixed {
		return rehydratedResolutionRefusal("继承底座在场却没标已固定")
	}
	if item.Kind != CustomerServiceRuleObject || item.Version.kind != CustomerServiceRuleObject {
		return rehydratedResolutionRefusal("继承底座挂在不是客户服务规则的一格上")
	}
	if _, contractTier := item.Version.ReferenceTo(CustomerContractObject); !contractTier {
		return rehydratedResolutionRefusal("继承底座挂在产品层那一版上")
	}
	if !item.HasProductBase {
		return nil
	}
	base := item.ProductBase
	if base.kind != CustomerServiceRuleObject ||
		base.status == CommercialVersionStatusInvalid || base.status == CommercialVersionDraft ||
		base.tenant != item.Version.tenant || base.scope != item.Version.scope {
		return rehydratedResolutionRefusal("继承底座不是同租户同范围的已发布客户服务规则")
	}
	if _, namesContract := base.ReferenceTo(CustomerContractObject); namesContract {
		return rehydratedResolutionRefusal("继承底座的壳指名了客户合同")
	}
	return nil
}

func rehydratedResolutionRefusal(reason string) error {
	return fmt.Errorf("%w：%s", ErrInvalidRehydratedResolution, reason)
}
