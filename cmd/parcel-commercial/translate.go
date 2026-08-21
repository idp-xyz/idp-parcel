package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"time"

	pspostgres "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
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
	TenantID          string                    `json:"tenantId"`
	Kind              string                    `json:"kind"`
	ObjectID          string                    `json:"objectId"`
	Version           string                    `json:"version"`
	Scope             string                    `json:"scope"`
	ContentDigest     string                    `json:"contentDigest"`
	EffectiveStartsAt time.Time                 `json:"effectiveStartsAt"`
	EffectiveEndsAt   *time.Time                `json:"effectiveEndsAt,omitempty"`
	References        map[string]string         `json:"references,omitempty"`
	Approval          approvalDocument          `json:"approval"`
	RoleStanding      string                    `json:"approvalRoleStanding"`
	Declarations      *declarationsDocument     `json:"declarations,omitempty"`
}

type approvalDocument struct {
	Reference  string    `json:"reference"`
	Source     string    `json:"source"`
	ApprovedAt time.Time `json:"approvedAt"`
}

type declarationsDocument struct {
	AsOfPolicies          []asOfPolicyDocument          `json:"asOfPolicies,omitempty"`
	AcceptanceContent     *acceptanceContentDocument    `json:"acceptanceContent,omitempty"`
	PendingRoutingBasis   string                        `json:"pendingRoutingBasis,omitempty"`
	PreAcceptanceControl  *preAcceptanceControlDocument `json:"preAcceptanceControl,omitempty"`
	ContractContent       *contractContentDocument      `json:"contractContent,omitempty"`
	IntakeQualification   *intakeQualificationDocument  `json:"intakeQualification,omitempty"`
	FinalRules            []finalRuleDocument           `json:"finalRules,omitempty"`
	CancellationAuthority []cancellationRuleDocument    `json:"cancellationAuthority,omitempty"`
	RulePackageBody       *rulePackageBodyDocument      `json:"rulePackageBody,omitempty"`
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
	RulePackage string                  `json:"rulePackage"`
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

	return declarations, nil
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

// ---- 解析键登记的 JSON 形状 ----

type resolutionKeyDocument struct {
	TenantID          string    `json:"tenantId"`
	CustomerAccountID string    `json:"customerAccountId"`
	Scope             string    `json:"scope"`
	LegalEntity       string    `json:"legalEntity"`
	AnchorPolicy      string    `json:"anchorPolicyVersion"`
	AnchorAt          time.Time `json:"anchorAt"`
	RequiredBases     []string  `json:"requiredBases"`
}

func keyRegistrationFromJSON(raw []byte) (pspostgres.ResolutionKeyRegistration, error) {
	none := pspostgres.ResolutionKeyRegistration{}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var document resolutionKeyDocument
	if err := decoder.Decode(&document); err != nil {
		return none, fmt.Errorf("解析键登记不是本入口的形状：%w", err)
	}

	registration := pspostgres.ResolutionKeyRegistration{AnchorAt: document.AnchorAt}
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
	} {
		if kind.String() == name {
			return kind, nil
		}
	}
	return pcdomain.CommercialObjectKindInvalid, fmt.Errorf("集合外的商业对象类别 %q", name)
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
