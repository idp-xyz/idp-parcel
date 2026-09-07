package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"time"

	pspartycommercial "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/partycommercial"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	pcapplication "go.idp.xyz/idp-parcel/internal/partycommercial/application"
	pcdomain "go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// 本文件把发布批 JSON 折成应用命令。翻译严格且零默认：未知字段拒收（打错字段名不得
// 静默变成「没给」）、集合外取值拒收、缺件交给领域构造门与用例按未决/拒绝处理——
// 这里绝不代填。所有取值集合都是领域封闭集的名称镜像，default 报错不吸收。

type publicationBatchDocument struct {
	Items []publicationItemDocument `json:"items"`
}

type publicationItemDocument struct {
	TenantID          string                `json:"tenantId"`
	Kind              string                `json:"kind"`
	ObjectID          string                `json:"objectId"`
	Version           string                `json:"version"`
	Scope             string                `json:"scope"`
	ContentDigest     string                `json:"contentDigest"`
	EffectiveStartsAt time.Time             `json:"effectiveStartsAt"`
	EffectiveEndsAt   *time.Time            `json:"effectiveEndsAt,omitempty"`
	References        map[string]string     `json:"references,omitempty"`
	Approval          approvalDocument      `json:"approval"`
	RoleStanding      string                `json:"approvalRoleStanding"`
	Declarations      *declarationsDocument `json:"declarations,omitempty"`
}

type approvalDocument struct {
	Reference  string    `json:"reference"`
	Source     string    `json:"source"`
	ApprovedAt time.Time `json:"approvedAt"`
}

type declarationsDocument struct {
	AsOfPolicies            []asOfPolicyDocument             `json:"asOfPolicies,omitempty"`
	AcceptanceContent       *acceptanceContentDocument       `json:"acceptanceContent,omitempty"`
	PendingRoutingBasis     string                           `json:"pendingRoutingBasis,omitempty"`
	PreAcceptanceControl    *preAcceptanceControlDocument    `json:"preAcceptanceControl,omitempty"`
	ContractContent         *contractContentDocument         `json:"contractContent,omitempty"`
	IntakeQualification     *intakeQualificationDocument     `json:"intakeQualification,omitempty"`
	FinalRules              []finalRuleDocument              `json:"finalRules,omitempty"`
	CancellationAuthority   []cancellationRuleDocument       `json:"cancellationAuthority,omitempty"`
	RulePackageBody         *rulePackageBodyDocument         `json:"rulePackageBody,omitempty"`
	SettlementPolicyBody    *settlementPolicyBodyDocument    `json:"settlementPolicyBody,omitempty"`
	CreditPolicyBody        *creditPolicyBodyDocument        `json:"creditPolicyBody,omitempty"`
	SupplierAgreementBody   *supplierAgreementBodyDocument   `json:"supplierAgreementBody,omitempty"`
	PricePolicyBody         *pricePolicyBodyDocument         `json:"pricePolicyBody,omitempty"`
	CustomerServiceRuleBody *customerServiceRuleBodyDocument `json:"customerServiceRuleBody,omitempty"`
	// preAcceptanceFinancialControlPolicyBody 与 preAcceptanceControl 是两层不同的声明：后者挂在客户合同
	// 版本上答「要不要」，前者挂在策略版本上答「控制怎么做」（ADR-0115）。
	PreAcceptanceFinancialControlPolicyBody *preAcceptanceFinancialControlPolicyBodyDocument `json:"preAcceptanceFinancialControlPolicyBody,omitempty"`
}

type asOfPolicyDocument struct {
	Judgment      string `json:"judgment"`
	Semantics     string `json:"semantics"`
	PolicyVersion string `json:"policyVersion"`
}

type acceptanceContentDocument struct {
	ApplicableGroups []string `json:"applicableGroups"`
	ManualReview     string   `json:"manualReview"`
}

type preAcceptanceControlDocument struct {
	Requirement        string `json:"requirement"`
	NotApplicableBasis string `json:"notApplicableBasis,omitempty"`
}

type contractContentDocument struct {
	RulePackage string                   `json:"rulePackage"`
	Bindings    []controlBindingDocument `json:"bindings,omitempty"`
}

type controlBindingDocument struct {
	ChargeScope          string `json:"chargeScope"`
	Policy               string `json:"policy,omitempty"`
	InapplicabilityBasis string `json:"inapplicabilityBasis,omitempty"`
}

type intakeQualificationDocument struct {
	Sources        []string `json:"sources"`
	Qualifications []string `json:"qualifications,omitempty"`
}

type finalRuleDocument struct {
	Outcome   string `json:"outcome"`
	FinalKind string `json:"finalKind"`
}

type cancellationRuleDocument struct {
	Party string `json:"party"`
	Rule  string `json:"rule"`
}

type rulePackageBodyDocument struct {
	ServiceProduct    string             `json:"serviceProduct"`
	Contract          string             `json:"contract"`
	LegalEntity       string             `json:"legalEntity"`
	Scope             string             `json:"scope"`
	EffectiveStartsAt time.Time          `json:"effectiveStartsAt"`
	EffectiveEndsAt   *time.Time         `json:"effectiveEndsAt,omitempty"`
	Rules             []assembledRuleDoc `json:"rules"`
}

type assembledRuleDoc struct {
	Category  string `json:"category"`
	Reference string `json:"reference"`
}

// settlementPolicyBodyDocument 是一份结算政策正文：一种结算方式与它覆盖的六维适用范围。
// 六维一维不少地摆在这里，是因为本上下文禁止借宽泛的客户关系跨维归集——批文里省掉哪一维，
// 都会变成「这一维随便什么值都算命中」。
type settlementPolicyBodyDocument struct {
	Method            string                  `json:"method"`
	LegalEntity       string                  `json:"legalEntity"`
	Counterparty      string                  `json:"counterparty"`
	Contract          contractVersionDocument `json:"contract"`
	ChargeScope       string                  `json:"chargeScope"`
	Currency          string                  `json:"currency"`
	EffectiveStartsAt time.Time               `json:"effectiveStartsAt"`
	EffectiveEndsAt   *time.Time              `json:"effectiveEndsAt,omitempty"`
}

// creditPolicyBodyDocument 是一份信用政策正文（票 party-commercial-context-gaps/03）。
//
// 额度是两个键恰一在场：limitMinor（最小货币单位）或 limitRatioBasisPoints（万分比）。两个都
// 是指针，因为 `0` 是一句合法的商业声明（授予零额度）而不是「没给」——用普通整数就分不出
// 这两件事，而它们要人做的事相反。两个都给或都不给在这里就拒收，不交给库上的 CHECK 去以一条
// 技术错误报出一件领域上早该拒绝的事。
type creditPolicyBodyDocument struct {
	LegalEntity           string     `json:"legalEntity"`
	AuthorityLevel        string     `json:"authorityLevel"`
	ChargeType            string     `json:"chargeType"`
	LimitMinor            *int64     `json:"limitMinor,omitempty"`
	LimitRatioBasisPoints *int64     `json:"limitRatioBasisPoints,omitempty"`
	EffectiveStartsAt     time.Time  `json:"effectiveStartsAt"`
	EffectiveEndsAt       *time.Time `json:"effectiveEndsAt,omitempty"`
}

// supplierAgreementBodyDocument 是一份供应商商业协议正文（同票）。没有方向键：领域把它钉死为
// BUY，批文里出现 direction 就是未知字段，由 DisallowUnknownFields 拒收。
type supplierAgreementBodyDocument struct {
	Supplier          string     `json:"supplier"`
	LegalEntity       string     `json:"legalEntity"`
	Scope             string     `json:"scope"`
	PurchasePlan      string     `json:"purchasePlan"`
	EffectiveStartsAt time.Time  `json:"effectiveStartsAt"`
	EffectiveEndsAt   *time.Time `json:"effectiveEndsAt,omitempty"`
}

// pricePolicyBodyDocument 是一份商业价格政策正文（票 party-commercial-context-gaps/06）：方向、
// 定价方案绑定、政策自己的适用范围与区间，以及可缺的计价口径。
//
// planDirection 与 conversion 按 ADR-0057 是**发布当时** parcel-pricing 的答复与当时声明的转换。
// 受控登记口采信批文：写批文的人从 parcel-pricing 的价卡目录抄来方案方向，与 contentDigest、
// approval 同一信任级。抄错的后果是 checkPlanBinding 在发布面按错的方向判——这一格在批文口
// 守不住，与 contentDigest 抄错同级；在线发布口那天必须改为向 parcel-pricing 现问（票 06）。
type pricePolicyBodyDocument struct {
	Direction         string                      `json:"direction"`
	PricingPlan       string                      `json:"pricingPlan"`
	PlanDirection     string                      `json:"planDirection"`
	Conversion        string                      `json:"conversion"`
	Scope             string                      `json:"scope"`
	EffectiveStartsAt time.Time                   `json:"effectiveStartsAt"`
	EffectiveEndsAt   *time.Time                  `json:"effectiveEndsAt,omitempty"`
	Caliber           *pricePolicyCaliberDocument `json:"caliber,omitempty"`
}

// pricePolicyCaliberDocument 是价格政策声明的计价口径。没有方向键：体积口径的方向就是政策的
// 方向，批文里再写一遍只会造出「两个方向对不上」这种本不该存在的输入。taxClassification 只在
// 含税/未税时给、volumetricFactor 只在 SELL 时给——反过来给了或漏了都由领域构造门拒。fx 一节
// 可缺：不涉及外币的政策没有汇率口径，缺席是「没声明」而不是零口径。
//
// 没有加点键，且不是漏掉：票 02 裁 (a) 首发显式未决，批文里出现 markup 之类就是未知字段、拒收。
type pricePolicyCaliberDocument struct {
	TaxDisposition    string             `json:"taxDisposition"`
	TaxClassification string             `json:"taxClassification,omitempty"`
	VolumetricFactor  string             `json:"volumetricFactor,omitempty"`
	Fx                *fxCaliberDocument `json:"fx,omitempty"`
}

// fxCaliberDocument 是汇率口径的三格引用：牌价类型（实例半边，租户与其银行的约定）、取值时点的
// 语义引用与时点政策版本。三格缺一不可，由 pcdomain.NewFxCaliber 拒。
type fxCaliberDocument struct {
	QuoteType         string `json:"quoteType"`
	AsOfSemantics     string `json:"asOfSemantics"`
	AsOfPolicyVersion string `json:"asOfPolicyVersion"`
}

// customerServiceRuleBodyDocument 是一份客户服务规则正文（票 party-commercial-context-gaps/05，ADR-0104）：
// 挂在哪个商业对象上、责任方、范围，与首发两项——索赔期限、最低材料。
//
// 适用对象是两个键恰一在场：serviceProduct 或 customerContract。两个都给或都不给在这里就拒收，不交给
// 库上的 CHECK 去以一条技术错误报出一件领域上早该拒绝的事（判据同 creditPolicyBodyDocument 的两格额度）。
// 两项清单可各自缺席，「合起来至少一项」由领域构造门在发布用例里拒——翻译层不代判也不代填。
//
// 只有这两项，且不是漏掉：另四项（追踪披露、异常响应、客户更新、通知义务）按 ADR-0104 不进首发，批文里
// 出现就是未知字段、由 DisallowUnknownFields 拒收；重启条件在该 ADR 的 Consequences。
type customerServiceRuleBodyDocument struct {
	ServiceProduct   string                     `json:"serviceProduct,omitempty"`
	CustomerContract string                     `json:"customerContract,omitempty"`
	Responsible      string                     `json:"responsible"`
	Scope            string                     `json:"scope"`
	ClaimDeadlines   []claimDeadlineDocument    `json:"claimDeadlines,omitempty"`
	MinimumMaterials []minimumMaterialsDocument `json:"minimumMaterials,omitempty"`
}

// claimDeadlineDocument 是一条索赔期限：种类（封闭三值，镜像 pcdomain.ClaimDeadlineKind）× 起算事件引用 ×
// 整数天 × 日历或时区引用。days 用普通整数而不是指针：零与缺席在这里同义——都不是一条算得出东西的
// 期限，由 NewClaimDeadlineRule 拒。任何天数只在批文里出现，仓库不持有取值。
type claimDeadlineDocument struct {
	Kind       string `json:"kind"`
	StartEvent string `json:"startEvent"`
	Days       int    `json:"days"`
	Calendar   string `json:"calendar"`
}

// minimumMaterialsDocument 是一种索赔类型的最低材料清单。空清单由 NewMinimumMaterialsRule 拒——「这一类
// 不要材料」没有任何消费方读得出来，要说也该是不写这一行。
type minimumMaterialsDocument struct {
	ClaimKind string   `json:"claimKind"`
	Materials []string `json:"materials"`
}

// preAcceptanceFinancialControlPolicyBodyDocument 是一份接受前财务控制策略正文（票 party-commercial-context-gaps/07，
// ADR-0115）：共同通过条件与要执行的控制项。
//
// jointPassCondition 必填且首发只认 ALL_CONTROLS_PASS：缺席不折成「全部通过」——CONTEXT 要求策略明确它，
// 没写就是没写。controls 至少一项由领域构造门在发布用例里拒；零项不是「显式无控制」，那一句由客户合同的
// preAcceptanceControl 与 contractContent.bindings 声明，本节说不了它——批文里出现 NO_CONTROL 之类的控制
// 种类就是集外取值、拒收。
type preAcceptanceFinancialControlPolicyBodyDocument struct {
	JointPassCondition string                             `json:"jointPassCondition"`
	Controls           []preAcceptanceControlItemDocument `json:"controls"`
}

// preAcceptanceControlItemDocument 是一项控制：种类（封闭两值，镜像 pcdomain.PreAcceptanceControlKind）×
// 费用范围引用 × 判断顺序 × 失败处置（封闭两值，镜像 pcdomain.ControlFailureDisposition）× 责任引用。
// order 用普通整数而不是指针：零与缺席在这里同义——都不是「排第几」的答案，由 NewPreAcceptanceControlItem 拒。
type preAcceptanceControlItemDocument struct {
	Control        string `json:"control"`
	ChargeScope    string `json:"chargeScope"`
	Order          int    `json:"order"`
	OnFailure      string `json:"onFailure"`
	Responsibility string `json:"responsibility"`
}

// contractVersionDocument 分两段收「本约定属于哪一版客户合同」，不收一个已经拼好的串。
// 串由 pcdomain.NewQualifiedVersionLabel 拼，与闭包解出合同后拿去命中的那个串同出一处
// （ADR-0080）；收现成串等于把分隔符这件事交给写批文的人，改法那天两边静静对不上，而
// 看起来像「这个范围没有结算政策」。
type contractVersionDocument struct {
	ObjectID string `json:"objectId"`
	Version  string `json:"version"`
}

func publishCommandsFromJSON(raw []byte) ([]pcapplication.PublishCommercialAuthorityCommand, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var document publicationBatchDocument
	if err := decoder.Decode(&document); err != nil {
		return nil, fmt.Errorf("发布批不是本入口的形状：%w", err)
	}
	if len(document.Items) == 0 {
		return nil, fmt.Errorf("发布批没有任何项")
	}
	commands := make([]pcapplication.PublishCommercialAuthorityCommand, 0, len(document.Items))
	for index, item := range document.Items {
		command, err := publishCommandFrom(item)
		if err != nil {
			return nil, fmt.Errorf("第 %d 项：%w", index+1, err)
		}
		commands = append(commands, command)
	}
	return commands, nil
}

func publishCommandFrom(item publicationItemDocument) (pcapplication.PublishCommercialAuthorityCommand, error) {
	none := pcapplication.PublishCommercialAuthorityCommand{}
	kind, err := commercialKindFrom(item.Kind)
	if err != nil {
		return none, err
	}
	spec := pcdomain.CommercialVersionSpec{Kind: kind}
	if spec.TenantID, err = pcdomain.NewTenantID(item.TenantID); err != nil {
		return none, err
	}
	if spec.ObjectID, err = pcdomain.NewCommercialObjectID(item.ObjectID); err != nil {
		return none, err
	}
	if spec.Version, err = pcdomain.NewCommercialVersionLabel(item.Version); err != nil {
		return none, err
	}
	if spec.Scope, err = pcdomain.NewCommercialScopeReference(item.Scope); err != nil {
		return none, err
	}
	if spec.ContentDigest, err = pcdomain.NewCommercialContentDigest(item.ContentDigest); err != nil {
		return none, err
	}
	endsAt := time.Time{}
	if item.EffectiveEndsAt != nil {
		endsAt = *item.EffectiveEndsAt
	}
	if spec.Effective, err = pcdomain.NewEffectiveInterval(item.EffectiveStartsAt, endsAt); err != nil {
		return none, err
	}
	if len(item.References) > 0 {
		spec.References = make(map[pcdomain.CommercialObjectKind]pcdomain.CommercialObjectID, len(item.References))
		for kindName, objectID := range item.References {
			referencedKind, err := commercialKindFrom(kindName)
			if err != nil {
				return none, err
			}
			referencedID, err := pcdomain.NewCommercialObjectID(objectID)
			if err != nil {
				return none, err
			}
			spec.References[referencedKind] = referencedID
		}
	}

	approvalReference, err := pcdomain.NewApprovalReference(item.Approval.Reference)
	if err != nil {
		return none, err
	}
	approvalSource, err := pcdomain.NewCommercialSourceReference(item.Approval.Source)
	if err != nil {
		return none, err
	}
	approval, err := pcdomain.NewApprovalBasis(approvalReference, approvalSource, item.Approval.ApprovedAt)
	if err != nil {
		return none, err
	}

	roleStanding, err := roleStandingFrom(item.RoleStanding)
	if err != nil {
		return none, err
	}

	declarations, err := declarationsFrom(item.Declarations)
	if err != nil {
		return none, err
	}

	return pcapplication.PublishCommercialAuthorityCommand{
		Spec:         spec,
		Approval:     approval,
		RoleStanding: roleStanding,
		Declarations: declarations,
	}, nil
}

func declarationsFrom(document *declarationsDocument) (pcapplication.CommercialDeclarations, error) {
	var declarations pcapplication.CommercialDeclarations
	if document == nil {
		return declarations, nil
	}

	for _, policy := range document.AsOfPolicies {
		judgment, err := judgmentTypeFrom(policy.Judgment)
		if err != nil {
			return declarations, err
		}
		semantics, err := pcdomain.NewAsOfSemanticsReference(policy.Semantics)
		if err != nil {
			return declarations, err
		}
		version, err := pcdomain.NewAsOfPolicyVersion(policy.PolicyVersion)
		if err != nil {
			return declarations, err
		}
		declared, err := pcdomain.NewAsOfPolicy(judgment, semantics, version)
		if err != nil {
			return declarations, err
		}
		declarations.AsOfPolicies = append(declarations.AsOfPolicies, declared)
	}

	if document.AcceptanceContent != nil {
		groups := make([]pcdomain.AcceptanceCheckGroupType, 0, len(document.AcceptanceContent.ApplicableGroups))
		for _, name := range document.AcceptanceContent.ApplicableGroups {
			group, err := checkGroupFrom(name)
			if err != nil {
				return declarations, err
			}
			groups = append(groups, group)
		}
		review, err := manualReviewFrom(document.AcceptanceContent.ManualReview)
		if err != nil {
			return declarations, err
		}
		declarations.AcceptanceContent = &pcapplication.AcceptanceContentDeclaration{
			ApplicableGroups: groups,
			ManualReview:     review,
		}
	}

	if document.PendingRoutingBasis != "" {
		basis, err := pcdomain.NewPendingRoutingBasisReference(document.PendingRoutingBasis)
		if err != nil {
			return declarations, err
		}
		declarations.PendingRoutingBasis = &basis
	}

	if document.PreAcceptanceControl != nil {
		requirement, err := controlRequirementFrom(document.PreAcceptanceControl.Requirement)
		if err != nil {
			return declarations, err
		}
		instruction := &pcapplication.PreAcceptanceControlInstruction{Requirement: requirement}
		if document.PreAcceptanceControl.NotApplicableBasis != "" {
			basis, err := pcdomain.NewControlNotApplicableBasis(document.PreAcceptanceControl.NotApplicableBasis)
			if err != nil {
				return declarations, err
			}
			instruction.Basis = basis
		}
		declarations.PreAcceptanceControl = instruction
	}

	if document.ContractContent != nil {
		rulePackage, err := pcdomain.NewCommercialObjectID(document.ContractContent.RulePackage)
		if err != nil {
			return declarations, err
		}
		bindings := make([]pcdomain.FinancialControlBinding, 0, len(document.ContractContent.Bindings))
		for _, binding := range document.ContractContent.Bindings {
			declared, err := controlBindingFrom(binding)
			if err != nil {
				return declarations, err
			}
			bindings = append(bindings, declared)
		}
		declarations.ContractContent = &pcapplication.ContractContentDeclaration{
			RulePackage: rulePackage,
			Bindings:    bindings,
		}
	}

	if document.IntakeQualification != nil {
		sources := make([]pcdomain.DeclaredIntakeSource, 0, len(document.IntakeQualification.Sources))
		for _, name := range document.IntakeQualification.Sources {
			source, err := intakeSourceFrom(name)
			if err != nil {
				return declarations, err
			}
			sources = append(sources, source)
		}
		qualifications := make([]pcdomain.RuleReference, 0, len(document.IntakeQualification.Qualifications))
		for _, name := range document.IntakeQualification.Qualifications {
			qualification, err := pcdomain.NewRuleReference(name)
			if err != nil {
				return declarations, err
			}
			qualifications = append(qualifications, qualification)
		}
		declarations.IntakeQualification = &pcapplication.IntakeQualificationDeclaration{
			Sources:        sources,
			Qualifications: qualifications,
		}
	}

	for _, rule := range document.FinalRules {
		outcome, err := responsibilityOutcomeFrom(rule.Outcome)
		if err != nil {
			return declarations, err
		}
		finalKind, err := pcdomain.NewRuleReference(rule.FinalKind)
		if err != nil {
			return declarations, err
		}
		declarations.FinalRules = append(declarations.FinalRules, pcdomain.FinalizationDeclaration{
			Outcome:   outcome,
			FinalKind: finalKind,
		})
	}

	for _, rule := range document.CancellationAuthority {
		party, err := cancellationPartyFrom(rule.Party)
		if err != nil {
			return declarations, err
		}
		reference, err := pcdomain.NewRuleReference(rule.Rule)
		if err != nil {
			return declarations, err
		}
		declarations.CancellationAuthority = append(declarations.CancellationAuthority,
			pcdomain.CancellationAuthorityDeclaration{Party: party, Rule: reference})
	}

	if document.RulePackageBody != nil {
		body, err := rulePackageBodyFrom(*document.RulePackageBody)
		if err != nil {
			return declarations, err
		}
		declarations.RulePackageBody = body
	}

	if document.SettlementPolicyBody != nil {
		body, err := settlementPolicyBodyFrom(*document.SettlementPolicyBody)
		if err != nil {
			return declarations, err
		}
		declarations.SettlementPolicyBody = body
	}

	if document.CreditPolicyBody != nil {
		body, err := creditPolicyBodyFrom(*document.CreditPolicyBody)
		if err != nil {
			return declarations, err
		}
		declarations.CreditPolicyBody = body
	}

	if document.SupplierAgreementBody != nil {
		body, err := supplierAgreementBodyFrom(*document.SupplierAgreementBody)
		if err != nil {
			return declarations, err
		}
		declarations.SupplierAgreementBody = body
	}

	if document.PricePolicyBody != nil {
		body, err := pricePolicyBodyFrom(*document.PricePolicyBody)
		if err != nil {
			return declarations, err
		}
		declarations.PricePolicyBody = body
	}

	if document.CustomerServiceRuleBody != nil {
		body, err := customerServiceRuleBodyFrom(*document.CustomerServiceRuleBody)
		if err != nil {
			return declarations, err
		}
		declarations.CustomerServiceRuleBody = body
	}

	if document.PreAcceptanceFinancialControlPolicyBody != nil {
		body, err := preAcceptanceFinancialControlPolicyBodyFrom(*document.PreAcceptanceFinancialControlPolicyBody)
		if err != nil {
			return declarations, err
		}
		declarations.PreAcceptanceFinancialControlPolicyBody = body
	}

	return declarations, nil
}

func preAcceptanceFinancialControlPolicyBodyFrom(
	document preAcceptanceFinancialControlPolicyBodyDocument,
) (*pcapplication.PreAcceptanceFinancialControlPolicyBodyDeclaration, error) {
	jointPass, err := jointPassConditionFrom(document.JointPassCondition)
	if err != nil {
		return nil, err
	}
	items := make([]pcdomain.PreAcceptanceControlItem, 0, len(document.Controls))
	for _, control := range document.Controls {
		kind, err := preAcceptanceControlKindFrom(control.Control)
		if err != nil {
			return nil, err
		}
		scope, err := pcdomain.NewChargeScopeReference(control.ChargeScope)
		if err != nil {
			return nil, err
		}
		disposition, err := controlFailureDispositionFrom(control.OnFailure)
		if err != nil {
			return nil, err
		}
		responsibility, err := pcdomain.NewControlResponsibilityReference(control.Responsibility)
		if err != nil {
			return nil, err
		}
		item, err := pcdomain.NewPreAcceptanceControlItem(kind, scope, control.Order, disposition, responsibility)
		if err != nil {
			return nil, fmt.Errorf("控制项 %s@%s（顺序 %d）：%w", control.Control, control.ChargeScope, control.Order, err)
		}
		items = append(items, item)
	}
	return &pcapplication.PreAcceptanceFinancialControlPolicyBodyDeclaration{
		JointPass: jointPass,
		Items:     items,
	}, nil
}

func customerServiceRuleBodyFrom(
	document customerServiceRuleBodyDocument,
) (*pcapplication.CustomerServiceRuleBodyDeclaration, error) {
	applicability, err := customerServiceRuleApplicabilityFrom(document.ServiceProduct, document.CustomerContract)
	if err != nil {
		return nil, err
	}
	responsible, err := pcdomain.NewPartyID(document.Responsible)
	if err != nil {
		return nil, err
	}
	scope, err := pcdomain.NewCommercialScopeReference(document.Scope)
	if err != nil {
		return nil, err
	}
	deadlines := make([]pcdomain.ClaimDeadlineRule, 0, len(document.ClaimDeadlines))
	for _, deadline := range document.ClaimDeadlines {
		kind, err := claimDeadlineKindFrom(deadline.Kind)
		if err != nil {
			return nil, err
		}
		startEvent, err := pcdomain.NewDeadlineStartEventReference(deadline.StartEvent)
		if err != nil {
			return nil, err
		}
		calendar, err := pcdomain.NewBusinessCalendarReference(deadline.Calendar)
		if err != nil {
			return nil, err
		}
		rule, err := pcdomain.NewClaimDeadlineRule(kind, startEvent, deadline.Days, calendar)
		if err != nil {
			return nil, fmt.Errorf("期限 %s（%d 天）：%w", deadline.Kind, deadline.Days, err)
		}
		deadlines = append(deadlines, rule)
	}
	materials := make([]pcdomain.MinimumMaterialsRule, 0, len(document.MinimumMaterials))
	for _, entry := range document.MinimumMaterials {
		claimKind, err := pcdomain.NewClaimKindReference(entry.ClaimKind)
		if err != nil {
			return nil, err
		}
		references := make([]pcdomain.MaterialRequirementReference, 0, len(entry.Materials))
		for _, material := range entry.Materials {
			reference, err := pcdomain.NewMaterialRequirementReference(material)
			if err != nil {
				return nil, err
			}
			references = append(references, reference)
		}
		rule, err := pcdomain.NewMinimumMaterialsRule(claimKind, references)
		if err != nil {
			return nil, fmt.Errorf("索赔类型 %q 的最低材料：%w", entry.ClaimKind, err)
		}
		materials = append(materials, rule)
	}
	return &pcapplication.CustomerServiceRuleBodyDeclaration{
		Applicability: applicability,
		Responsible:   responsible,
		Scope:         scope,
		Deadlines:     deadlines,
		Materials:     materials,
	}, nil
}

// customerServiceRuleApplicabilityFrom 只认恰一格在场。两格都给时不挑一格读——同一个标识串作产品与作
// 合同是两件事，这里把它变成一次响亮的拒收（判据同 creditLimitFrom）。
func customerServiceRuleApplicabilityFrom(product, contract string) (pcdomain.CustomerServiceRuleApplicability, error) {
	switch {
	case product != "" && contract == "":
		id, err := pcdomain.NewCommercialObjectID(product)
		if err != nil {
			return pcdomain.CustomerServiceRuleApplicability{}, err
		}
		return pcdomain.CustomerServiceRuleAppliesToServiceProduct(id), nil
	case product == "" && contract != "":
		id, err := pcdomain.NewCommercialObjectID(contract)
		if err != nil {
			return pcdomain.CustomerServiceRuleApplicability{}, err
		}
		return pcdomain.CustomerServiceRuleAppliesToCustomerContract(id), nil
	default:
		return pcdomain.CustomerServiceRuleApplicability{},
			fmt.Errorf("客户服务规则必须恰好给出 serviceProduct 或 customerContract 之一")
	}
}

func pricePolicyBodyFrom(document pricePolicyBodyDocument) (*pcapplication.PricePolicyBodyDeclaration, error) {
	direction, err := priceDirectionFrom(document.Direction)
	if err != nil {
		return nil, err
	}
	pricingPlan, err := pcdomain.NewPricingPlanReference(document.PricingPlan)
	if err != nil {
		return nil, err
	}
	planDirection, err := priceDirectionFrom(document.PlanDirection)
	if err != nil {
		return nil, err
	}
	conversion, err := planBindingConversionFrom(document.Conversion)
	if err != nil {
		return nil, err
	}
	scope, err := pcdomain.NewCommercialScopeReference(document.Scope)
	if err != nil {
		return nil, err
	}
	endsAt := time.Time{}
	if document.EffectiveEndsAt != nil {
		endsAt = *document.EffectiveEndsAt
	}
	interval, err := pcdomain.NewEffectiveInterval(document.EffectiveStartsAt, endsAt)
	if err != nil {
		return nil, err
	}
	body := &pcapplication.PricePolicyBodyDeclaration{
		Direction:     direction,
		PricingPlan:   pricingPlan,
		PlanDirection: planDirection,
		Conversion:    conversion,
		Scope:         scope,
		Effective:     interval,
	}
	if document.Caliber != nil {
		caliber, err := pricePolicyCaliberFrom(direction, *document.Caliber)
		if err != nil {
			return nil, err
		}
		body.Caliber = caliber
	}
	return body, nil
}

// pricePolicyCaliberFrom 按政策方向装体积口径——方向不从批文读，所以批文口造不出方向不一致的口径；
// 用例里那道 ConsistentWithDirection 守的是绕开本翻译的调用方。
func pricePolicyCaliberFrom(
	direction pcdomain.PriceDirection,
	document pricePolicyCaliberDocument,
) (*pcapplication.PricePolicyCaliberDeclaration, error) {
	disposition, err := taxDispositionFrom(document.TaxDisposition)
	if err != nil {
		return nil, err
	}
	classification := pcdomain.TaxClassificationReference{}
	if document.TaxClassification != "" {
		if classification, err = pcdomain.NewTaxClassificationReference(document.TaxClassification); err != nil {
			return nil, err
		}
	}
	tax, err := pcdomain.NewTaxCaliber(disposition, classification)
	if err != nil {
		return nil, fmt.Errorf("税务口径 %q 与分类 %q：%w", document.TaxDisposition, document.TaxClassification, err)
	}
	factor := pcdomain.VolumetricFactorReference{}
	if document.VolumetricFactor != "" {
		if factor, err = pcdomain.NewVolumetricFactorReference(document.VolumetricFactor); err != nil {
			return nil, err
		}
	}
	volumetric, err := pcdomain.NewVolumetricCaliber(direction, factor)
	if err != nil {
		return nil, fmt.Errorf("方向 %s 的体积口径（系数 %q）：%w", direction, document.VolumetricFactor, err)
	}
	caliber := &pcapplication.PricePolicyCaliberDeclaration{Tax: tax, Volumetric: volumetric}
	if document.Fx != nil {
		quoteType, err := pcdomain.NewFxQuoteTypeReference(document.Fx.QuoteType)
		if err != nil {
			return nil, err
		}
		semantics, err := pcdomain.NewAsOfSemanticsReference(document.Fx.AsOfSemantics)
		if err != nil {
			return nil, err
		}
		policyVersion, err := pcdomain.NewAsOfPolicyVersion(document.Fx.AsOfPolicyVersion)
		if err != nil {
			return nil, err
		}
		fx, err := pcdomain.NewFxCaliber(quoteType, semantics, policyVersion)
		if err != nil {
			return nil, err
		}
		caliber.Fx = &fx
	}
	return caliber, nil
}

func creditPolicyBodyFrom(document creditPolicyBodyDocument) (*pcapplication.CreditPolicyBodyDeclaration, error) {
	legalEntity, err := pcdomain.NewLegalEntityReference(document.LegalEntity)
	if err != nil {
		return nil, err
	}
	level, err := pcdomain.NewAuthorityLevel(document.AuthorityLevel)
	if err != nil {
		return nil, err
	}
	chargeType, err := pcdomain.NewChargeTypeReference(document.ChargeType)
	if err != nil {
		return nil, err
	}
	limit, err := creditLimitFrom(document.LimitMinor, document.LimitRatioBasisPoints)
	if err != nil {
		return nil, err
	}
	endsAt := time.Time{}
	if document.EffectiveEndsAt != nil {
		endsAt = *document.EffectiveEndsAt
	}
	interval, err := pcdomain.NewEffectiveInterval(document.EffectiveStartsAt, endsAt)
	if err != nil {
		return nil, err
	}
	return &pcapplication.CreditPolicyBodyDeclaration{
		LegalEntity: legalEntity,
		Level:       level,
		ChargeType:  chargeType,
		Limit:       limit,
		Effective:   interval,
	}, nil
}

// creditLimitFrom 只认恰一格在场。两格都给时不挑一格读——那正是「一个数加一列标记」那种表形
// 会静默犯的错，这里把它变成一次响亮的拒收。
func creditLimitFrom(limitMinor, limitBps *int64) (pcdomain.CreditLimit, error) {
	switch {
	case limitMinor != nil && limitBps == nil:
		return pcdomain.NewCreditAmountLimit(*limitMinor)
	case limitMinor == nil && limitBps != nil:
		return pcdomain.NewCreditRatioLimit(*limitBps)
	default:
		return pcdomain.CreditLimit{}, fmt.Errorf("信用额度必须恰好给出 limitMinor 或 limitRatioBasisPoints 之一")
	}
}

func supplierAgreementBodyFrom(
	document supplierAgreementBodyDocument,
) (*pcapplication.SupplierAgreementBodyDeclaration, error) {
	supplier, err := pcdomain.NewPartyID(document.Supplier)
	if err != nil {
		return nil, err
	}
	legalEntity, err := pcdomain.NewLegalEntityReference(document.LegalEntity)
	if err != nil {
		return nil, err
	}
	scope, err := pcdomain.NewCommercialScopeReference(document.Scope)
	if err != nil {
		return nil, err
	}
	purchasePlan, err := pcdomain.NewPricingPlanReference(document.PurchasePlan)
	if err != nil {
		return nil, err
	}
	endsAt := time.Time{}
	if document.EffectiveEndsAt != nil {
		endsAt = *document.EffectiveEndsAt
	}
	interval, err := pcdomain.NewEffectiveInterval(document.EffectiveStartsAt, endsAt)
	if err != nil {
		return nil, err
	}
	return &pcapplication.SupplierAgreementBodyDeclaration{
		Supplier:     supplier,
		LegalEntity:  legalEntity,
		Scope:        scope,
		PurchasePlan: purchasePlan,
		Effective:    interval,
	}, nil
}

func rulePackageBodyFrom(document rulePackageBodyDocument) (*pcapplication.RulePackageBodyDeclaration, error) {
	serviceProduct, err := pcdomain.NewCommercialObjectID(document.ServiceProduct)
	if err != nil {
		return nil, err
	}
	contract, err := pcdomain.NewCommercialObjectID(document.Contract)
	if err != nil {
		return nil, err
	}
	legalEntity, err := pcdomain.NewLegalEntityReference(document.LegalEntity)
	if err != nil {
		return nil, err
	}
	scope, err := pcdomain.NewCommercialScopeReference(document.Scope)
	if err != nil {
		return nil, err
	}
	endsAt := time.Time{}
	if document.EffectiveEndsAt != nil {
		endsAt = *document.EffectiveEndsAt
	}
	interval, err := pcdomain.NewEffectiveInterval(document.EffectiveStartsAt, endsAt)
	if err != nil {
		return nil, err
	}
	applicability, err := pcdomain.NewRulePackageApplicability(serviceProduct, contract, legalEntity, scope, interval)
	if err != nil {
		return nil, err
	}
	rules := make([]pcdomain.AssembledRule, 0, len(document.Rules))
	for _, rule := range document.Rules {
		category, err := ruleCategoryFrom(rule.Category)
		if err != nil {
			return nil, err
		}
		reference, err := pcdomain.NewRuleReference(rule.Reference)
		if err != nil {
			return nil, err
		}
		assembled, err := pcdomain.NewAssembledRule(category, reference)
		if err != nil {
			return nil, err
		}
		rules = append(rules, assembled)
	}
	return &pcapplication.RulePackageBodyDeclaration{Applicability: applicability, Rules: rules}, nil
}

func settlementPolicyBodyFrom(
	document settlementPolicyBodyDocument,
) (*pcapplication.SettlementPolicyBodyDeclaration, error) {
	method, err := settlementMethodFrom(document.Method)
	if err != nil {
		return nil, err
	}
	legalEntity, err := pcdomain.NewLegalEntityReference(document.LegalEntity)
	if err != nil {
		return nil, err
	}
	counterparty, err := pcdomain.NewCounterpartyReference(document.Counterparty)
	if err != nil {
		return nil, err
	}
	contractObject, err := pcdomain.NewCommercialObjectID(document.Contract.ObjectID)
	if err != nil {
		return nil, err
	}
	contractVersion, err := pcdomain.NewCommercialVersionLabel(document.Contract.Version)
	if err != nil {
		return nil, err
	}
	contract, err := pcdomain.NewQualifiedVersionLabel(contractObject, contractVersion)
	if err != nil {
		return nil, err
	}
	chargeScope, err := pcdomain.NewChargeScopeReference(document.ChargeScope)
	if err != nil {
		return nil, err
	}
	currency, err := pcdomain.NewCurrencyCode(document.Currency)
	if err != nil {
		return nil, err
	}
	endsAt := time.Time{}
	if document.EffectiveEndsAt != nil {
		endsAt = *document.EffectiveEndsAt
	}
	interval, err := pcdomain.NewEffectiveInterval(document.EffectiveStartsAt, endsAt)
	if err != nil {
		return nil, err
	}
	applicability, err := pcdomain.NewSettlementApplicability(
		legalEntity, counterparty, contract, chargeScope, currency, interval)
	if err != nil {
		return nil, err
	}
	return &pcapplication.SettlementPolicyBodyDeclaration{Method: method, Applicability: applicability}, nil
}

// ---- 解析键登记的 JSON 形状 ----

type resolutionKeyDocument struct {
	TenantID          string    `json:"tenantId"`
	CustomerAccountID string    `json:"customerAccountId"`
	Scope             string    `json:"scope"`
	LegalEntity       string    `json:"legalEntity"`
	AnchorPolicy      string    `json:"anchorPolicyVersion"`
	AnchorAt          time.Time `json:"anchorAt"`
	RequiredBases     []string  `json:"requiredBases"`
	// Settlement 只在必需依据含 SETTLEMENT_POLICY 时给出，且只有三维——合同维由闭包解出
	// 的客户合同来填，登记面结构上就没有它（ADR-0080）。
	Settlement *settlementSelectorDocument `json:"settlement,omitempty"`
}

type settlementSelectorDocument struct {
	Counterparty string `json:"counterparty"`
	ChargeScope  string `json:"chargeScope"`
	Currency     string `json:"currency"`
}

func keyRegistrationFromJSON(raw []byte) (pspartycommercial.ResolutionKeyRegistration, error) {
	none := pspartycommercial.ResolutionKeyRegistration{}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var document resolutionKeyDocument
	if err := decoder.Decode(&document); err != nil {
		return none, fmt.Errorf("解析键登记不是本入口的形状：%w", err)
	}

	registration := pspartycommercial.ResolutionKeyRegistration{AnchorAt: document.AnchorAt}
	var err error
	if registration.TenantID, err = psdomain.NewTenantID(document.TenantID); err != nil {
		return none, err
	}
	if registration.CustomerAccountID, err = psdomain.NewCustomerAccountID(document.CustomerAccountID); err != nil {
		return none, err
	}
	if registration.Scope, err = pcdomain.NewCommercialScopeReference(document.Scope); err != nil {
		return none, err
	}
	if registration.LegalEntity, err = pcdomain.NewLegalEntityReference(document.LegalEntity); err != nil {
		return none, err
	}
	if registration.AnchorPolicy, err = pcdomain.NewAnchorPolicyVersion(document.AnchorPolicy); err != nil {
		return none, err
	}
	for _, name := range document.RequiredBases {
		kind, err := commercialKindFrom(name)
		if err != nil {
			return none, err
		}
		registration.RequiredBases = append(registration.RequiredBases, kind)
	}
	// 三维逐维过构造门。缺席整节即三维全缺，登记面据此判「本次不要结算依据」；给了节却
	// 少一维在这里就响亮失败，不折成缺席——那会让一次打错字段名变成一句「没登记结算」。
	if document.Settlement != nil {
		if registration.SettlementCounterparty, err = pcdomain.NewCounterpartyReference(
			document.Settlement.Counterparty); err != nil {
			return none, err
		}
		if registration.SettlementChargeScope, err = pcdomain.NewChargeScopeReference(
			document.Settlement.ChargeScope); err != nil {
			return none, err
		}
		if registration.SettlementCurrency, err = pcdomain.NewCurrencyCode(
			document.Settlement.Currency); err != nil {
			return none, err
		}
	}
	return registration, nil
}

// ---- 封闭集名称镜像：default 一律报错不吸收 ----

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
		pcdomain.CustomerServiceRuleObject,
	} {
		if kind.String() == name {
			return kind, nil
		}
	}
	return pcdomain.CommercialObjectKindInvalid, fmt.Errorf("集合外的商业对象类别 %q", name)
}

// claimDeadlineKindFrom 只认三个取值，逐字对应 visibility-exception CONTEXT「三个独立期限」。default
// 报错不吸收——把打错的种类折进某一格，等于替租户改了它想登记的是哪一种期限。
func claimDeadlineKindFrom(name string) (pcdomain.ClaimDeadlineKind, error) {
	for _, kind := range []pcdomain.ClaimDeadlineKind{
		pcdomain.FirstClaimDeadline,
		pcdomain.MaterialSupplementDeadline,
		pcdomain.ConclusionReviewDeadline,
	} {
		if kind.String() == name {
			return kind, nil
		}
	}
	return pcdomain.ClaimDeadlineKindInvalid, fmt.Errorf("集合外的索赔期限种类 %q", name)
}

// preAcceptanceControlKindFrom 只认两个取值。集合里没有「无控制」不是漏掉：那一句由客户合同声明并带依据
// （ADR-0115 Decision 一），批文里写 NO_CONTROL 就是集外取值。
func preAcceptanceControlKindFrom(name string) (pcdomain.PreAcceptanceControlKind, error) {
	for _, kind := range []pcdomain.PreAcceptanceControlKind{
		pcdomain.PrepaidFreezeControl,
		pcdomain.CreditCheckControl,
	} {
		if kind.String() == name {
			return kind, nil
		}
	}
	return pcdomain.PreAcceptanceControlKindInvalid, fmt.Errorf("集合外的控制种类 %q", name)
}

// controlFailureDispositionFrom 只认两个取值，逐字对应 UC-PS-001「按策略拒绝或进入授权处置」。default 报错
// 不吸收——把打错的处置折进某一格，等于替租户改了失败时委托的去向。
func controlFailureDispositionFrom(name string) (pcdomain.ControlFailureDisposition, error) {
	for _, disposition := range []pcdomain.ControlFailureDisposition{
		pcdomain.RejectOnControlFailure,
		pcdomain.AuthorizedDispositionOnControlFailure,
	} {
		if disposition.String() == name {
			return disposition, nil
		}
	}
	return pcdomain.ControlFailureDispositionInvalid, fmt.Errorf("集合外的失败处置 %q", name)
}

// jointPassConditionFrom 首发只认一个取值。缺席不折成它：CONTEXT 要求策略明确共同通过条件，批文口这一层
// 先要求写出来；「任一通过」今天不在集合里（pn-02-w03 禁推导、无消费形状），写了就是集外取值。
func jointPassConditionFrom(name string) (pcdomain.JointPassCondition, error) {
	if name == pcdomain.AllControlsPass.String() {
		return pcdomain.AllControlsPass, nil
	}
	return pcdomain.JointPassConditionInvalid, fmt.Errorf("集合外的共同通过条件 %q", name)
}

func roleStandingFrom(name string) (pcdomain.ApprovalRoleStanding, error) {
	switch name {
	case pcdomain.ApprovalRoleConfirmed.String():
		return pcdomain.ApprovalRoleConfirmed, nil
	case pcdomain.ApprovalRoleUnconfirmed.String():
		return pcdomain.ApprovalRoleUnconfirmed, nil
	default:
		// 缺席不折成未确认：打错字段值的批准会以未决收场没错，但那让人去查批准流程；
		// 这里直接点名是输入坏了。
		return pcdomain.ApprovalRoleStandingInvalid, fmt.Errorf("集合外的批准角色确认 %q", name)
	}
}

func judgmentTypeFrom(name string) (pcdomain.JudgmentType, error) {
	switch name {
	case pcdomain.NetworkReachabilityJudgment.String():
		return pcdomain.NetworkReachabilityJudgment, nil
	case pcdomain.PreAcceptanceFinancialControlJudgment.String():
		return pcdomain.PreAcceptanceFinancialControlJudgment, nil
	default:
		return pcdomain.JudgmentTypeInvalid, fmt.Errorf("集合外的判断类型 %q", name)
	}
}

func checkGroupFrom(name string) (pcdomain.AcceptanceCheckGroupType, error) {
	for _, group := range []pcdomain.AcceptanceCheckGroupType{
		pcdomain.CustomerRelationshipCheckGroup,
		pcdomain.LegalEntityAndContractCheckGroup,
		pcdomain.ProductAndServiceCheckGroup,
		pcdomain.MemberBaselineCheckGroup,
		pcdomain.RequiredDocumentCheckGroup,
		pcdomain.PreAcceptanceFinancialControlCheckGroup,
		pcdomain.NetworkReachabilityCheckGroup,
	} {
		if group.String() == name {
			return group, nil
		}
	}
	return pcdomain.AcceptanceCheckGroupTypeInvalid, fmt.Errorf("集合外的校验组 %q", name)
}

func manualReviewFrom(name string) (pcdomain.ManualReviewDirective, error) {
	switch name {
	case pcdomain.ManualReviewRequired.String():
		return pcdomain.ManualReviewRequired, nil
	case pcdomain.ManualReviewNotRequired.String():
		return pcdomain.ManualReviewNotRequired, nil
	default:
		return pcdomain.ManualReviewUndeclared, fmt.Errorf("集合外的人工复核指令 %q", name)
	}
}

func controlRequirementFrom(name string) (pcdomain.PreAcceptanceControlRequirement, error) {
	switch name {
	case pcdomain.PreAcceptanceControlRequired.String():
		return pcdomain.PreAcceptanceControlRequired, nil
	case pcdomain.PreAcceptanceControlNotApplicable.String():
		return pcdomain.PreAcceptanceControlNotApplicable, nil
	default:
		return pcdomain.PreAcceptanceControlUndeclared, fmt.Errorf("集合外的控制要求 %q", name)
	}
}

func controlBindingFrom(document controlBindingDocument) (pcdomain.FinancialControlBinding, error) {
	scope, err := pcdomain.NewChargeScopeReference(document.ChargeScope)
	if err != nil {
		return pcdomain.FinancialControlBinding{}, err
	}
	switch {
	case document.Policy != "" && document.InapplicabilityBasis == "":
		policy, err := pcdomain.NewCommercialObjectID(document.Policy)
		if err != nil {
			return pcdomain.FinancialControlBinding{}, err
		}
		return pcdomain.NewAppliedFinancialControl(scope, policy)
	case document.Policy == "" && document.InapplicabilityBasis != "":
		basis, err := pcdomain.NewInapplicabilityBasis(document.InapplicabilityBasis)
		if err != nil {
			return pcdomain.FinancialControlBinding{}, err
		}
		return pcdomain.NewInapplicableFinancialControl(scope, basis)
	default:
		return pcdomain.FinancialControlBinding{}, fmt.Errorf(
			"费用范围 %q 的约定必须恰好指名一份策略或一条不适用依据", document.ChargeScope)
	}
}

func intakeSourceFrom(name string) (pcdomain.DeclaredIntakeSource, error) {
	switch name {
	case pcdomain.DeclaredNodeIntake.String():
		return pcdomain.DeclaredNodeIntake, nil
	case pcdomain.DeclaredOffsitePickup.String():
		return pcdomain.DeclaredOffsitePickup, nil
	default:
		return pcdomain.DeclaredIntakeSourceInvalid, fmt.Errorf("集合外的收寄来源 %q", name)
	}
}

func responsibilityOutcomeFrom(name string) (pcdomain.DeclaredResponsibilityOutcome, error) {
	for _, outcome := range []pcdomain.DeclaredResponsibilityOutcome{
		pcdomain.DeclaredEffectiveDelivery,
		pcdomain.DeclaredReturnCompleted,
		pcdomain.DeclaredServiceTerminated,
		pcdomain.DeclaredRegulatoryDisposition,
	} {
		if outcome.String() == name {
			return outcome, nil
		}
	}
	return pcdomain.DeclaredResponsibilityOutcomeInvalid, fmt.Errorf("集合外的责任结果 %q", name)
}

func cancellationPartyFrom(name string) (pcdomain.DeclaredCancellationParty, error) {
	switch name {
	case pcdomain.DeclaredCustomerCancellation.String():
		return pcdomain.DeclaredCustomerCancellation, nil
	case pcdomain.DeclaredOperationsCancellation.String():
		return pcdomain.DeclaredOperationsCancellation, nil
	default:
		return pcdomain.DeclaredCancellationPartyInvalid, fmt.Errorf("集合外的取消请求方 %q", name)
	}
}

func priceDirectionFrom(name string) (pcdomain.PriceDirection, error) {
	for _, direction := range []pcdomain.PriceDirection{
		pcdomain.BuyDirection,
		pcdomain.SellDirection,
		pcdomain.InternalDirection,
	} {
		if direction.String() == name {
			return direction, nil
		}
	}
	return pcdomain.PriceDirectionInvalid, fmt.Errorf("集合外的价格方向 %q", name)
}

// planBindingConversionFrom 只认两个取值。缺席不折成 NONE：SELL 政策绑 BUY 价卡而没写转换，该由
// NewCommercialPricePolicy 报出`适用冲突`（AT-PC-033），代填 NONE 会把它报成同一件事，但把
// 「没写」与「明说不转换」混在一起——批文口这一层先要求写出来。
func planBindingConversionFrom(name string) (pcdomain.PlanBindingConversion, error) {
	switch name {
	case pcdomain.PlanBindingConversionNone.String():
		return pcdomain.PlanBindingConversionNone, nil
	case pcdomain.PlanBindingFrozenBuyEvaluation.String():
		return pcdomain.PlanBindingFrozenBuyEvaluation, nil
	default:
		return pcdomain.PlanBindingConversionNone, fmt.Errorf("集合外的方案绑定转换 %q", name)
	}
}

// taxDispositionFrom 只认三个取值。零值哨兵不落进「不适用」，与领域同判据：缺席与已判定为
// 不适用要人做的事不同，前者去补声明。
func taxDispositionFrom(name string) (pcdomain.TaxDisposition, error) {
	for _, disposition := range []pcdomain.TaxDisposition{
		pcdomain.TaxInclusive,
		pcdomain.TaxExclusive,
		pcdomain.TaxNotApplicable,
	} {
		if disposition.String() == name {
			return disposition, nil
		}
	}
	return pcdomain.TaxDispositionInvalid, fmt.Errorf("集合外的税务口径 %q", name)
}

// settlementMethodFrom 只认两个取值。第三个取值——某种客户级默认——正是本上下文明禁的：
// 未命中的范围必须报出`无适用依据`，而不是回落到某种通行做法，进程口这一层也不许开这个口。
func settlementMethodFrom(name string) (pcdomain.SettlementMethod, error) {
	switch name {
	case pcdomain.PrepaidMethod.String():
		return pcdomain.PrepaidMethod, nil
	case pcdomain.TermsMethod.String():
		return pcdomain.TermsMethod, nil
	default:
		return pcdomain.SettlementMethodInvalid, fmt.Errorf("集合外的结算方式 %q", name)
	}
}

func ruleCategoryFrom(name string) (pcdomain.RuleCategory, error) {
	for _, category := range []pcdomain.RuleCategory{
		pcdomain.MinimumIngressIdentityRules,
		pcdomain.ShipmentInvariantRules,
		pcdomain.ProductAndContractDocumentRules,
		pcdomain.RegulatorySourceDocumentRules,
		pcdomain.CrossFieldConditionRules,
	} {
		if category.String() == name {
			return category, nil
		}
	}
	return pcdomain.RuleCategoryInvalid, fmt.Errorf("集合外的规则分类 %q", name)
}
