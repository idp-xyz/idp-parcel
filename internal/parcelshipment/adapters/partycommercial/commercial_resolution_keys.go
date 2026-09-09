package partycommercial

import (
	"context"
	"fmt"
	"time"

	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	pcdomain "go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// ResolutionKeyRow 是登记面在持久化层的行形状：全是字符串与时刻，不带任何 party-commercial
// 领域类型。
//
// 这条缝是两条架构门禁的交集逼出来的，不是分层洁癖：跨上下文翻译只能落在
// adapters/partycommercial（TestBusinessModulesDoNotReachIntoEachOther），而数据库驱动只能
// 在持久化适配器里碰（TestBusinessPackagesDoNotTouchTheDriverDirectly）。一个既拼 SQL 又造
// `ClosureResolutionKey` 的类型两头都违规，只能把行搬运与词汇翻译分开：SQL 归
// adapters/postgres，翻译归这里，中间过这个只有基本类型的行。
type ResolutionKeyRow struct {
	TenantID          string
	CustomerAccountID string
	Scope             string
	LegalEntity       string
	AnchorPolicy      string
	AnchorAt          time.Time
	RequiredBases     []string
	// 结算三维只在必需依据含结算政策时在场，空串表示缺席（ADR-0080）。合同维不在其中，
	// 它是闭包解出来的结论——理由见 ResolutionKeyRegistration。
	SettlementCounterparty string
	SettlementChargeScope  string
	SettlementCurrency     string
	// 信用二维只在必需依据含信用政策时在场，空串表示缺席（ADR-0127）。
	CreditLevel      string
	CreditChargeType string
}

// ResolutionKeySaveOutcome 是一次登记在持久化面的落点（ADR-0031 同款）：`已登记`是
// 重放，`内容冲突`是同一（租户+客户）被登记成另一套键参数——换范围或换锚点属于换一套
// 解析口径，要以显式新决定处理，不静默覆盖。
type ResolutionKeySaveOutcome uint8

const (
	ResolutionKeySaveOutcomeInvalid ResolutionKeySaveOutcome = iota
	ResolutionKeySaved
	ResolutionKeyAlreadyRegistered
	ResolutionKeyContentConflict
)

func (outcome ResolutionKeySaveOutcome) String() string {
	switch outcome {
	case ResolutionKeySaved:
		return "SAVED"
	case ResolutionKeyAlreadyRegistered:
		return "ALREADY_REGISTERED"
	case ResolutionKeyContentConflict:
		return "CONTENT_CONFLICT"
	default:
		return ""
	}
}

// ResolutionKeyStore 是登记行的持久化半边，由 adapters/postgres 实现。查无行交回
// (zero, false, nil)——显式未配置，与读取失败（error）分格。
type ResolutionKeyStore interface {
	RegisterResolutionKey(ctx context.Context, row ResolutionKeyRow) (ResolutionKeySaveOutcome, error)
	FindResolutionKey(ctx context.Context, tenant, customer string) (ResolutionKeyRow, bool, error)
}

// CommercialResolutionKeys 实现本包 ResolutionKeySource 的登记面半边
// （syn-wall-door-audit 票 03 件 2）：把（租户+货主客户账户）映射到形成闭包解析键的
// 四项实例参数——商业范围、责任法人候选、选择锚点与必需依据种类。
//
// 无行即显式未配置：FormResolutionKey 交回 formed=false，商业依据适配器据以停在
// `COMMERCIAL_RESOLUTION_KEY_NOT_CONFIGURED`。本适配器不代拟任何一项，尤其不拿系统
// 当前时间顶锚点（AT-PC-018）；锚点时刻与策略版本都来自登记行。
type CommercialResolutionKeys struct {
	store ResolutionKeyStore
}

func NewCommercialResolutionKeys(store ResolutionKeyStore) (*CommercialResolutionKeys, error) {
	if store == nil {
		return nil, fmt.Errorf("parcel shipment party commercial: resolution key store is nil")
	}
	return &CommercialResolutionKeys{store: store}, nil
}

var _ ResolutionKeySource = (*CommercialResolutionKeys)(nil)

// ResolutionKeyRegistration 是一行登记的输入。全部字段都是显式实例参数：锚点时刻
// 是登记下来的值，不是任何时钟；必需依据种类由登记方逐项指名。
//
// 结算三维逐维列出，而不是收一个 `pcdomain.SettlementSelector`：那个类型带合同维，而本
// 登记面**不得承载合同维**（ADR-0080——合同是闭包解出的结论，登记它就是消费方在指定该
// 选中哪个商业版本）。拆成三个字段之后，那一维在这一层根本无从表达，而不是靠一句校验
// 拦着。
type ResolutionKeyRegistration struct {
	TenantID          psdomain.TenantID
	CustomerAccountID psdomain.CustomerAccountID
	Scope             pcdomain.CommercialScopeReference
	LegalEntity       pcdomain.LegalEntityReference
	AnchorPolicy      pcdomain.AnchorPolicyVersion
	AnchorAt          time.Time
	RequiredBases     []pcdomain.CommercialObjectKind
	// 三维只在必需依据含结算政策时给出，否则必须全缺（ADR-0044 的「含则必填、不含则必缺」）。
	SettlementCounterparty pcdomain.CounterpartyReference
	SettlementChargeScope  pcdomain.ChargeScopeReference
	SettlementCurrency     pcdomain.CurrencyCode
	// 信用二维只在必需依据含信用政策时给出，否则必须全缺（ADR-0127 决定二：请求信用依据必填、
	// 其余请求必缺）。逐维列出而不收一个 `pcdomain.CreditSelector`，只是与上面结算三维同形：
	// 信用政策没有合同那样由闭包解出的一维，两维都是租户登记的实例参数，这里不是被迫拆的。
	// 法人与时点不在其中——键上已有法人候选与锚点，重复携带就允许两者不一致。
	CreditLevel      pcdomain.AuthorityLevel
	CreditChargeType pcdomain.ChargeTypeReference
}

// Register 登记一行解析键参数。集合与默认值的判读全在触库前做完——库内 CHECK 是同一
// 判据的第二道镜像，不是唯一一道。
func (adapter *CommercialResolutionKeys) Register(
	ctx context.Context,
	registration ResolutionKeyRegistration,
) (ResolutionKeySaveOutcome, error) {
	if err := registration.validate(); err != nil {
		return ResolutionKeySaveOutcomeInvalid, fmt.Errorf("register resolution key: %w", err)
	}
	bases := make([]string, 0, len(registration.RequiredBases))
	for _, kind := range registration.RequiredBases {
		bases = append(bases, kind.String())
	}
	return adapter.store.RegisterResolutionKey(ctx, ResolutionKeyRow{
		TenantID:               registration.TenantID.String(),
		CustomerAccountID:      registration.CustomerAccountID.String(),
		Scope:                  registration.Scope.String(),
		LegalEntity:            registration.LegalEntity.String(),
		AnchorPolicy:           registration.AnchorPolicy.String(),
		AnchorAt:               registration.AnchorAt.UTC(),
		RequiredBases:          bases,
		SettlementCounterparty: registration.SettlementCounterparty.String(),
		SettlementChargeScope:  registration.SettlementChargeScope.String(),
		SettlementCurrency:     registration.SettlementCurrency.String(),
		CreditLevel:            registration.CreditLevel.String(),
		CreditChargeType:       registration.CreditChargeType.String(),
	})
}

func (registration ResolutionKeyRegistration) validate() error {
	if registration.TenantID.String() == "" ||
		registration.CustomerAccountID.String() == "" ||
		registration.Scope.String() == "" ||
		registration.LegalEntity.String() == "" ||
		registration.AnchorPolicy.String() == "" {
		return fmt.Errorf("每一项键参数都必须显式给出")
	}
	if registration.AnchorAt.IsZero() {
		return fmt.Errorf("锚点时刻必须显式登记，不得留零值待人补默认")
	}
	if len(registration.RequiredBases) == 0 {
		return fmt.Errorf("必需依据种类至少一项")
	}
	seen := make(map[pcdomain.CommercialObjectKind]bool, len(registration.RequiredBases))
	for _, kind := range registration.RequiredBases {
		if kind.String() == "" {
			return fmt.Errorf("必需依据种类含集合外取值")
		}
		// 价格规则要求键携带价格方向（ADR-0034 含则必填），本登记面没有那一维——放行
		// 等于登记一个永远立不起来的键。结算政策已随 ADR-0080 放行（见下方三维校验），
		// 信用政策随 ADR-0127 承载两维（见 validateCredit）。库内 CHECK 同拦。
		if kind == pcdomain.PriceRuleObject {
			return fmt.Errorf("%s 需要键携带额外选择维度，本登记面不承载", kind)
		}
		if seen[kind] {
			return fmt.Errorf("必需依据种类重复：%s", kind)
		}
		seen[kind] = true
	}
	if err := registration.validateSettlement(seen[pcdomain.SettlementPolicyObject],
		seen[pcdomain.CustomerContractObject]); err != nil {
		return err
	}
	return registration.validateCredit(seen[pcdomain.CreditPolicyObject])
}

// validateSettlement 是 ADR-0044「含则必填、不含则必缺」加 ADR-0080「要结算就要合同」在
// 登记面的一道。库内 `..._settlement_paired` 是同一判据的第二道镜像，不是唯一一道。
func (registration ResolutionKeyRegistration) validateSettlement(needsSettlement, needsContract bool) error {
	given := 0
	for _, dimension := range []string{
		registration.SettlementCounterparty.String(),
		registration.SettlementChargeScope.String(),
		registration.SettlementCurrency.String(),
	} {
		if dimension != "" {
			given++
		}
	}
	if !needsSettlement {
		if given > 0 {
			return fmt.Errorf("不要结算依据的登记不得携带结算维度")
		}
		return nil
	}
	if given < 3 {
		return fmt.Errorf("要结算依据就必须登记结算相对方、费用范围与币种三维")
	}
	// 合同维由闭包解出的合同来填；不请求合同就永远没人填得上（ADR-0080）。
	if !needsContract {
		return fmt.Errorf("要结算依据就必须一并要客户合同——结算政策按哪一版合同选，由本闭包解出")
	}
	return nil
}

// validateCredit 是 ADR-0127 决定二「请求信用依据必填、其余请求必缺」在登记面的一道。库内
// `..._credit_paired` 是同一判据的第二道镜像，不是唯一一道。
//
// 与 validateSettlement 只差一条：这里没有「要信用就要合同」。信用政策不引用同一闭包正在解的
// 别的成员，两维都由登记方给全，没有要等闭包填的一格——所以两维齐了就是齐了。
func (registration ResolutionKeyRegistration) validateCredit(needsCredit bool) error {
	given := 0
	for _, dimension := range []string{
		registration.CreditLevel.String(),
		registration.CreditChargeType.String(),
	} {
		if dimension != "" {
			given++
		}
	}
	if !needsCredit {
		if given > 0 {
			return fmt.Errorf("不要信用依据的登记不得携带信用维度")
		}
		return nil
	}
	if given < 2 {
		return fmt.Errorf("要信用依据就必须登记商业权限等级与费用类型两维")
	}
	return nil
}

// FormResolutionKey 把一次消费方查询折成闭包解析键。
//
// 目的钉死在`接受控制`：商业依据适配器只服务接受流（其 adoptedResolution 注释），
// 计价目的的键要求价格方向等另一套维度，届时按新决定另开登记面。
//
// 查无行交回 (zero, false, nil)——显式未配置，与读取失败（error）分格：前者等租户
// 登记，后者等依赖恢复，压成一格调用方就不知道该催人还是该重试。
func (adapter *CommercialResolutionKeys) FormResolutionKey(
	ctx context.Context,
	query psports.CommercialBasisQuery,
) (pcdomain.ClosureResolutionKey, bool, error) {
	none := pcdomain.ClosureResolutionKey{}
	tenant := query.Identity.TenantID().String()
	customer := query.Identity.CustomerAccountID().String()
	if tenant == "" || customer == "" {
		return none, false, fmt.Errorf("form resolution key: tenant and customer account are required")
	}

	row, found, err := adapter.store.FindResolutionKey(ctx, tenant, customer)
	if err != nil {
		return none, false, fmt.Errorf("form resolution key: %w", err)
	}
	if !found {
		return none, false, nil
	}

	key := pcdomain.ClosureResolutionKey{Purpose: pcdomain.AcceptanceControlPurpose}
	if key.TenantID, err = pcdomain.NewTenantID(row.TenantID); err != nil {
		return none, false, fmt.Errorf("form resolution key: %w", err)
	}
	if key.CustomerAccountID, err = pcdomain.NewCustomerAccountID(row.CustomerAccountID); err != nil {
		return none, false, fmt.Errorf("form resolution key: %w", err)
	}
	if key.Scope, err = pcdomain.NewCommercialScopeReference(row.Scope); err != nil {
		return none, false, fmt.Errorf("form resolution key: %w", err)
	}
	if key.LegalEntityCandidate, err = pcdomain.NewLegalEntityReference(row.LegalEntity); err != nil {
		return none, false, fmt.Errorf("form resolution key: %w", err)
	}
	anchorPolicy, err := pcdomain.NewAnchorPolicyVersion(row.AnchorPolicy)
	if err != nil {
		return none, false, fmt.Errorf("form resolution key: %w", err)
	}
	if key.Anchor, err = pcdomain.NewSelectionAnchor(row.AnchorAt, anchorPolicy); err != nil {
		return none, false, fmt.Errorf("form resolution key: %w", err)
	}
	key.RequiredBases = make([]pcdomain.CommercialObjectKind, 0, len(row.RequiredBases))
	for _, name := range row.RequiredBases {
		kind, err := commercialKindFrom(name)
		if err != nil {
			// 库内 CHECK 与领域封闭集是同一集合的两份镜像，出现集外取值说明两份已经
			// 分叉，那要人来看，不能就地当成没登记。
			return none, false, fmt.Errorf("form resolution key: %w", err)
		}
		key.RequiredBases = append(key.RequiredBases, kind)
	}
	if key.Settlement, err = settlementSelectorFromRow(row); err != nil {
		return none, false, fmt.Errorf("form resolution key: %w", err)
	}
	if key.Credit, err = creditSelectorFromRow(row); err != nil {
		return none, false, fmt.Errorf("form resolution key: %w", err)
	}
	return key, true, nil
}

// settlementSelectorFromRow 把登记行上的三维折成闭包键上的选择器。三维全缺时交回零值，
// 那是「本次不要结算依据」的正常形状。
//
// 合同维一律不填：它由 ResolveCommercialClosure 解出客户合同之后补上（ADR-0080）。这里
// 若顺手塞一个，闭包键的最小身份当场就不成立——那道校验挡的正是本函数这一手。
func settlementSelectorFromRow(row ResolutionKeyRow) (pcdomain.SettlementSelector, error) {
	none := pcdomain.SettlementSelector{}
	if row.SettlementCounterparty == "" && row.SettlementChargeScope == "" && row.SettlementCurrency == "" {
		return none, nil
	}
	var selector pcdomain.SettlementSelector
	var err error
	if selector.Counterparty, err = pcdomain.NewCounterpartyReference(row.SettlementCounterparty); err != nil {
		return none, err
	}
	if selector.ChargeScope, err = pcdomain.NewChargeScopeReference(row.SettlementChargeScope); err != nil {
		return none, err
	}
	if selector.Currency, err = pcdomain.NewCurrencyCode(row.SettlementCurrency); err != nil {
		return none, err
	}
	return selector, nil
}

// creditSelectorFromRow 把登记行上的两维折成闭包键上的信用选择器。两维全缺时交回零值，那是
// 「本次不要信用依据」的正常形状（ADR-0127 决定二）。
//
// 两维都走非空引用值的构造门，没有 commercialKindFrom 那样的集外判读：商业权限等级是租户的
// 版本化业务授权，party-commercial 不预设它有哪几档，费用类型同理——库内 `..._credit_not_blank`
// 与这里各拦一道「空串冒充在场」，别的它们无从判。与结算不同，这里没有一维要留给闭包填。
func creditSelectorFromRow(row ResolutionKeyRow) (pcdomain.CreditSelector, error) {
	none := pcdomain.CreditSelector{}
	if row.CreditLevel == "" && row.CreditChargeType == "" {
		return none, nil
	}
	var selector pcdomain.CreditSelector
	var err error
	if selector.Level, err = pcdomain.NewAuthorityLevel(row.CreditLevel); err != nil {
		return none, err
	}
	if selector.ChargeType, err = pcdomain.NewChargeTypeReference(row.CreditChargeType); err != nil {
		return none, err
	}
	return selector, nil
}

func commercialKindFrom(name string) (pcdomain.CommercialObjectKind, error) {
	for _, kind := range []pcdomain.CommercialObjectKind{
		pcdomain.ServiceProductObject,
		pcdomain.CustomerContractObject,
		pcdomain.SupplierAgreementObject,
		pcdomain.AcceptanceRulePackageObject,
		pcdomain.PreAcceptanceFinancialControlPolicyObject,
		pcdomain.PriceRuleObject,
		pcdomain.SettlementPolicyObject,
		pcdomain.CreditPolicyObject,
		pcdomain.AuthorizationRuleObject,
	} {
		if kind.String() == name {
			return kind, nil
		}
	}
	return pcdomain.CommercialObjectKindInvalid, fmt.Errorf("unknown commercial object kind %q", name)
}
