package domain

import "fmt"

// 本文件是客户服务规则册接进服务端规范化的那一格（加册不换号，仍是 PCC-1——ADR-0126 Decision 一；票 admin-write-faces/18）。
// 规则版本的正文是父行三格（挂在哪个商业对象上、责任方、范围）加两张子表（索赔期限按种类成行、最低材料按索赔类型成行，
// 0023，ADR-0104 Decision 二）；受控批文把它写成 declarations 下的 customerServiceRuleBody{serviceProduct | customerContract,
// responsible, scope, claimDeadlines[…], minimumMaterials[…]}，这里键名镜像它。

// CustomerServiceRuleBody 是客户服务规则版本的正文输入面，以领域值对象给出：它就是 NewCustomerServiceRuleVersion 收的那
// 五项，只是不带拥有它们的（已生效）版本——预览与录入发生在发布之前。
//
// Deadlines / Materials 是已过 NewClaimDeadlineRule / NewMinimumMaterialsRule 的行；跨行的门（两项合起来至少一行、每一种期限与
// 每一种索赔类型至多一行）在折成文档前由 validate 答，与发布时同一条规则。适用对象恰一格由 CustomerServiceRuleApplicability
// 自己的两格封闭守着，这里不再摊成「产品、合同、哪一格」。
type CustomerServiceRuleBody struct {
	Applicability CustomerServiceRuleApplicability
	Responsible   PartyID
	Scope         CommercialScopeReference
	Deadlines     []ClaimDeadlineRule
	Materials     []MinimumMaterialsRule
}

// filed 在折成文档前把正文过一遍与发布时相同的门，并交回按键归档的两张表。父行三格立不立得住答
// ErrInvalidCustomerServiceRuleVersion（与 NewCustomerServiceRuleVersion 对同一件事的答复同一格），两张子表的跨行门只在
// filedCustomerServiceRuleItems 一处——预览要在录入之前就把「两项都空」「同一种期限两行」答给操作者，不能等到发布那一刻。
func (body CustomerServiceRuleBody) filed() (map[ClaimDeadlineKind]ClaimDeadlineRule, map[ClaimKindReference]MinimumMaterialsRule, error) {
	if !body.Applicability.valid() || !body.Responsible.valid() || !body.Scope.valid() {
		return nil, nil, ErrInvalidCustomerServiceRuleVersion
	}
	return filedCustomerServiceRuleItems(body.Deadlines, body.Materials)
}

func (body CustomerServiceRuleBody) validate() error {
	_, _, err := body.filed()
	return err
}

// canonicalCustomerServiceRuleBody 镜像批文 customerServiceRuleBody 的键名。适用对象两键恰一在场（缺席省略）；两张子表一律
// 在场，空表写成 `[]`——「这一版对期限无客户差异」是正文说出的真话，与键缺席不是两种东西。期限按种类顺序、材料按索赔类型
// 引用串顺序写出：两者各是那张表的键，因此就是它唯一的自然序——表单里换行序不是换正文，摘要不该跟着变（判据同
// canonicalPreAcceptanceFinancialControlPolicyBody 按判断顺序写出）。
type canonicalCustomerServiceRuleBody struct {
	ServiceProduct   string                          `json:"serviceProduct,omitempty"`
	CustomerContract string                          `json:"customerContract,omitempty"`
	Responsible      string                          `json:"responsible"`
	Scope            string                          `json:"scope"`
	ClaimDeadlines   []canonicalClaimDeadlineRule    `json:"claimDeadlines"`
	MinimumMaterials []canonicalMinimumMaterialsRule `json:"minimumMaterials"`
}

// canonicalClaimDeadlineRule 是一行索赔期限，四格都必在场：行的形状由 NewClaimDeadlineRule 守，文档不给缺格留位置。起算事件
// 与日历是开放引用（customer_service_rule.go 头注：解释权在 visibility-exception），收串不校验存在性。
type canonicalClaimDeadlineRule struct {
	Kind       string `json:"kind"`
	StartEvent string `json:"startEvent"`
	Days       int    `json:"days"`
	Calendar   string `json:"calendar"`
}

// canonicalMinimumMaterialsRule 是一行最低材料：索赔类型引用加它的材料清单。清单按 NewMinimumMaterialsRule 排好的稳定序写出，
// 表单里材料的先后同样不是正文。
type canonicalMinimumMaterialsRule struct {
	ClaimKind string   `json:"claimKind"`
	Materials []string `json:"materials"`
}

// canonicalizeCustomerServiceRule 是 CanonicalizePublicationContent 在本册的那一支：正文缺席答 ErrPublicationContentAbsent，
// 正文过不了门答构造门的原话，过了门折成文档算摘要。
func canonicalizeCustomerServiceRule(content PublicationContent) (CanonicalPublicationContent, error) {
	none := CanonicalPublicationContent{}
	if content.CustomerServiceRule == nil {
		return none, ErrPublicationContentAbsent
	}
	document, err := canonicalCustomerServiceRuleBodyOf(*content.CustomerServiceRule)
	if err != nil {
		return none, err
	}
	return canonicalDigestOf(canonicalPublicationDocument{
		Canonicalization:    publicationCanonicalizationVersion,
		Kind:                content.Kind.String(),
		CustomerServiceRule: document,
	})
}

func canonicalCustomerServiceRuleBodyOf(body CustomerServiceRuleBody) (*canonicalCustomerServiceRuleBody, error) {
	filedDeadlines, filedMaterials, err := body.filed()
	if err != nil {
		return nil, err
	}
	document := &canonicalCustomerServiceRuleBody{
		Responsible:      body.Responsible.String(),
		Scope:            body.Scope.String(),
		ClaimDeadlines:   make([]canonicalClaimDeadlineRule, 0, len(filedDeadlines)),
		MinimumMaterials: make([]canonicalMinimumMaterialsRule, 0, len(filedMaterials)),
	}
	if product, applies := body.Applicability.ServiceProduct(); applies {
		document.ServiceProduct = product.String()
	}
	if contract, applies := body.Applicability.CustomerContract(); applies {
		document.CustomerContract = contract.String()
	}
	for _, deadline := range sortedClaimDeadlines(filedDeadlines) {
		document.ClaimDeadlines = append(document.ClaimDeadlines, canonicalClaimDeadlineRule{
			Kind:       deadline.Kind().String(),
			StartEvent: deadline.StartEvent().String(),
			Days:       deadline.DurationDays(),
			Calendar:   deadline.Calendar().String(),
		})
	}
	for _, rule := range sortedMinimumMaterials(filedMaterials) {
		references := rule.Materials()
		materials := make([]string, 0, len(references))
		for _, material := range references {
			materials = append(materials, material.String())
		}
		document.MinimumMaterials = append(document.MinimumMaterials, canonicalMinimumMaterialsRule{
			ClaimKind: rule.ClaimKind().String(),
			Materials: materials,
		})
	}
	return document, nil
}

// body 把文档里的一节折回领域正文。适用对象两键恰一在场折回两格封闭；每一行过各自的构造门，期限种类按 String() 原词反查；
// 跨行的门留给 validate——快照是数据，正文立不立得住仍由构造门说。
func (document canonicalCustomerServiceRuleBody) body() (CustomerServiceRuleBody, error) {
	none := CustomerServiceRuleBody{}
	applicability, err := customerServiceRuleApplicabilityOf(document.ServiceProduct, document.CustomerContract)
	if err != nil {
		return none, err
	}
	responsible, err := NewPartyID(document.Responsible)
	if err != nil {
		return none, fmt.Errorf("responsible: %w", err)
	}
	scope, err := NewCommercialScopeReference(document.Scope)
	if err != nil {
		return none, fmt.Errorf("scope: %w", err)
	}
	body := CustomerServiceRuleBody{
		Applicability: applicability,
		Responsible:   responsible,
		Scope:         scope,
		Deadlines:     make([]ClaimDeadlineRule, 0, len(document.ClaimDeadlines)),
		Materials:     make([]MinimumMaterialsRule, 0, len(document.MinimumMaterials)),
	}
	for index, row := range document.ClaimDeadlines {
		deadline, err := row.rule()
		if err != nil {
			return none, fmt.Errorf("claimDeadlines[%d]: %w", index, err)
		}
		body.Deadlines = append(body.Deadlines, deadline)
	}
	for index, row := range document.MinimumMaterials {
		rule, err := row.rule()
		if err != nil {
			return none, fmt.Errorf("minimumMaterials[%d]: %w", index, err)
		}
		body.Materials = append(body.Materials, rule)
	}
	if err := body.validate(); err != nil {
		return none, err
	}
	return body, nil
}

// customerServiceRuleApplicabilityOf 把文档里并存的两键折回两格封闭的适用对象：恰一在场才立得住，两空或两满是文档与领域
// 分叉——同一个标识串作产品与作合同是两件事，不挑一格读（判据同 creditLimitOf）。
func customerServiceRuleApplicabilityOf(product, contract string) (CustomerServiceRuleApplicability, error) {
	switch {
	case product != "" && contract == "":
		id, err := NewCommercialObjectID(product)
		if err != nil {
			return CustomerServiceRuleApplicability{}, fmt.Errorf("serviceProduct: %w", err)
		}
		return CustomerServiceRuleAppliesToServiceProduct(id), nil
	case product == "" && contract != "":
		id, err := NewCommercialObjectID(contract)
		if err != nil {
			return CustomerServiceRuleApplicability{}, fmt.Errorf("customerContract: %w", err)
		}
		return CustomerServiceRuleAppliesToCustomerContract(id), nil
	default:
		return CustomerServiceRuleApplicability{}, fmt.Errorf("applicability: %w: serviceProduct 与 customerContract 须恰一在场",
			ErrInvalidCustomerServiceRuleVersion)
	}
}

func (row canonicalClaimDeadlineRule) rule() (ClaimDeadlineRule, error) {
	kind, known := ClaimDeadlineKindNamed(row.Kind)
	if !known {
		return ClaimDeadlineRule{}, fmt.Errorf("kind: %w: %q", ErrInvalidClaimDeadlineRule, row.Kind)
	}
	startEvent, err := NewDeadlineStartEventReference(row.StartEvent)
	if err != nil {
		return ClaimDeadlineRule{}, fmt.Errorf("startEvent: %w", err)
	}
	calendar, err := NewBusinessCalendarReference(row.Calendar)
	if err != nil {
		return ClaimDeadlineRule{}, fmt.Errorf("calendar: %w", err)
	}
	return NewClaimDeadlineRule(kind, startEvent, row.Days, calendar)
}

func (row canonicalMinimumMaterialsRule) rule() (MinimumMaterialsRule, error) {
	claimKind, err := NewClaimKindReference(row.ClaimKind)
	if err != nil {
		return MinimumMaterialsRule{}, fmt.Errorf("claimKind: %w", err)
	}
	materials := make([]MaterialRequirementReference, 0, len(row.Materials))
	for index, raw := range row.Materials {
		material, err := NewMaterialRequirementReference(raw)
		if err != nil {
			return MinimumMaterialsRule{}, fmt.Errorf("materials[%d]: %w", index, err)
		}
		materials = append(materials, material)
	}
	return NewMinimumMaterialsRule(claimKind, materials)
}

// ClaimDeadlineKindNamed 按 String() 的原词反查索赔期限种类：规范化文档里的、运营操作者面载荷里的、词表读口给出的都是那一个词，
// 名单只在 String() 一处，这里只是反查（判据同 CommercialObjectKindNamed）。集合外含空串答 false——三种期限彼此独立
// （visibility-exception CONTEXT），打错的种类折进某一格等于替租户把一种期限顶成另一种。
func ClaimDeadlineKindNamed(name string) (ClaimDeadlineKind, bool) {
	return closedCodeNamed(ClaimDeadlineKind.valid, ClaimDeadlineKind.String, name)
}
