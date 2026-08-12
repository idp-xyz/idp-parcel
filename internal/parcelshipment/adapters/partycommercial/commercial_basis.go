package partycommercial

import (
	"context"
	"fmt"

	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	pcapplication "go.idp.xyz/idp-parcel/internal/partycommercial/application"
	pcdomain "go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	pcports "go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// 本适配器自有的停摆原因引用。前两类带 PC- 前缀，因为它们译的是提供方答复里的状态；
// 键那条没有前缀——形不成解析键是消费方自己的实例参数缺席，冠 PC 会把账记错方向。
const (
	reasonKeyNotConfigured         = "COMMERCIAL_RESOLUTION_KEY_NOT_CONFIGURED"
	reasonContentNotConfigured     = "PC-ACCEPTANCE_CONTENT_NOT_CONFIGURED"
	reasonContentUnreadable        = "PC-ACCEPTANCE_CONTENT_UNREADABLE"
	reasonAsOfPoliciesUnreadable   = "PC-ASOF_POLICIES_UNREADABLE"
	reasonPendingRoutingUnreadable = "PC-PENDING_ROUTING_UNREADABLE"
)

// ResolutionKeySource 把一次消费方查询折成提供方的闭包解析键。
//
// 范围、法人候选、锚点策略与必需依据种类全是实例半边（试点参数）：没有租户时谁也说不出
// 这份委托该在哪个商业范围下解析。nil 或第二个返回值为 false 都是「显式未配置」，本适配器
// 据以交回`解析未决`——停下，不代拟一个范围。
type ResolutionKeySource interface {
	FormResolutionKey(ctx context.Context, query psports.CommercialBasisQuery) (pcdomain.ClosureResolutionKey, bool, error)
}

// CommercialBasisAdapter 把 parcel-shipment 的商业依据端口接到 party-commercial 的应用
// 编排上，实现 psports.CommercialBasisResolver 的三个阶段。
//
// ADR-0027 已裁：后续阶段的协作者由提供方以用例与端口提供（FormJudgmentAsOf、
// AsOfPolicyDeclaration、AcceptanceContentDeclaration），适配器直接调用，不再另定接口把
// 提供方模型转手一遍。Keys 与 Values 是消费方自己的实例半边，接口留在本包（ADR-0025）。
type CommercialBasisAdapter struct {
	resolve      *pcapplication.ResolveCommercialBasisHandler
	revalidate   *pcapplication.ValidateCommercialBasisHandler
	judgments    *pcapplication.FormJudgmentAsOfHandler
	asOfPolicies pcports.AsOfPolicyDeclaration
	contents     pcports.AcceptanceContentDeclaration
	keys         ResolutionKeySource
	values       AsOfValueSource
}

// CommercialBasisAdapterDeps 收拢七个协作方；≥5 个输入按本仓分界用结构体。
type CommercialBasisAdapterDeps struct {
	Resolve      *pcapplication.ResolveCommercialBasisHandler
	Revalidate   *pcapplication.ValidateCommercialBasisHandler
	Judgments    *pcapplication.FormJudgmentAsOfHandler
	AsOfPolicies pcports.AsOfPolicyDeclaration
	Contents     pcports.AcceptanceContentDeclaration
	Keys         ResolutionKeySource
	Values       AsOfValueSource
}

// NewCommercialBasisAdapter 装配三个阶段。Keys 与 Values 允许为 nil：今天没有任何租户
// 登记过范围映射或时点语义，nil 是「显式未配置」的诚实表达，届时解析停在`解析未决`、
// 时点停在`未配置`——那正是首发要停下的地方，不是要绕过的地方。
func NewCommercialBasisAdapter(deps CommercialBasisAdapterDeps) *CommercialBasisAdapter {
	return &CommercialBasisAdapter{
		resolve:      deps.Resolve,
		revalidate:   deps.Revalidate,
		judgments:    deps.Judgments,
		asOfPolicies: deps.AsOfPolicies,
		contents:     deps.Contents,
		keys:         deps.Keys,
		values:       deps.Values,
	}
}

var _ psports.CommercialBasisResolver = (*CommercialBasisAdapter)(nil)

// ResolveCommercialBasis 执行第一阶段：把消费方查询折成解析键（实例半边），交提供方在
// 同一权威视图下解析闭包，再把闭包译回本上下文的三值结果与快照。
func (adapter *CommercialBasisAdapter) ResolveCommercialBasis(
	ctx context.Context,
	query psports.CommercialBasisQuery,
) (psports.CommercialBasisResolution, error) {
	if adapter.keys == nil {
		return undeterminedResolution(reasonKeyNotConfigured)
	}
	key, formed, err := adapter.keys.FormResolutionKey(ctx, query)
	if err != nil {
		return psports.CommercialBasisResolution{}, fmt.Errorf("form resolution key: %w", err)
	}
	if !formed {
		return undeterminedResolution(reasonKeyNotConfigured)
	}

	answer, err := adapter.resolve.Handle(ctx, pcapplication.ResolveCommercialBasisCommand{Key: key})
	if err != nil {
		return psports.CommercialBasisResolution{}, fmt.Errorf("resolve commercial basis: %w", err)
	}
	return adapter.resolutionOf(ctx, answer.Closure())
}

// RevalidateCommercialBasis 执行第三阶段：按原解析标识请提供方重解一次，把重校验特有的
// 结果代数译回本上下文。身份与标识照第二阶段的办法原样译过去，立不立得起来由提供方短路
// 作答。
func (adapter *CommercialBasisAdapter) RevalidateCommercialBasis(
	ctx context.Context,
	query psports.CommercialRevalidationQuery,
) (psports.CommercialRevalidation, error) {
	answer, err := adapter.revalidate.Handle(ctx, pcapplication.ValidateCommercialBasisCommand{
		Caller:     callerFor(query.Identity),
		Resolution: resolutionFor(query.Resolution),
	})
	if err != nil {
		return psports.CommercialRevalidation{}, fmt.Errorf("revalidate commercial basis: %w", err)
	}

	closure := answer.Closure()
	switch closure.Outcome() {
	case pcdomain.UniquelyResolved:
		resolution, err := adapter.adoptedResolution(ctx, closure)
		if err != nil {
			return psports.CommercialRevalidation{}, err
		}
		if resolution.Applicability != psdomain.CommerciallyApplicable {
			// 提供方确认了原解析仍然唯一，但内容/时点声明读不回或未配置，快照建不起来。
			// 原解析既没被确认可用也没被推翻，只能未决——判`仍然成立`会让调用方在一份
			// 拿不到声明的依据上提交决定。
			return psports.CommercialRevalidation{
				Outcome: psports.CommercialRevalidationUndetermined,
				Reason:  resolution.Reason,
			}, nil
		}
		return psports.CommercialRevalidation{
			Outcome:    psports.CommercialBasisStillValid,
			Resolution: resolution,
		}, nil
	case pcdomain.ResolutionStale:
		reason, err := checkReasonFor(closure)
		if err != nil {
			return psports.CommercialRevalidation{}, err
		}
		// `已失效`不带解析：交回一份，调用方会以为可以继续用它（端口注释）。
		return psports.CommercialRevalidation{Outcome: psports.CommercialBasisSuperseded, Reason: reason}, nil
	case pcdomain.ResolutionPending:
		reason, err := checkReasonFor(closure)
		if err != nil {
			return psports.CommercialRevalidation{}, err
		}
		return psports.CommercialRevalidation{Outcome: psports.CommercialRevalidationUndetermined, Reason: reason}, nil
	case pcdomain.BasisNotResolved:
		return psports.CommercialRevalidation{Outcome: psports.CommercialRevalidationBasisNotResolved}, nil
	case pcdomain.InputNotAccepted:
		return psports.CommercialRevalidation{Outcome: psports.CommercialRevalidationInputNotAccepted}, nil
	case pcdomain.NoApplicableBasis, pcdomain.ApplicabilityConflict:
		// 重校验从不产这两个：原解析不唯一时提供方原样交回原结果，而消费方记下的采用
		// 标识只可能指向一次唯一解析。走到这里是阶段契约被打破。
		return psports.CommercialRevalidation{}, fmt.Errorf("%w: revalidation closure outcome %q",
			ErrUntranslatableAnswer, closure.Outcome())
	default:
		return psports.CommercialRevalidation{}, fmt.Errorf("%w: revalidation closure outcome %d",
			ErrUntranslatableAnswer, closure.Outcome())
	}
}

// resolutionOf 是第一阶段闭包到消费方三值结果的全函数。`适用冲突`与`解析未决`都落
// `无法判定`：三值代数里只有它既阻断新决定又不冒充商业拒绝——`无适用依据`才是权威说了
// 「这个范围没有适用对象」，那一格由消费方按自身规则形成拒绝（AT-PC-020）。
func (adapter *CommercialBasisAdapter) resolutionOf(
	ctx context.Context,
	closure pcdomain.CommercialClosure,
) (psports.CommercialBasisResolution, error) {
	switch closure.Outcome() {
	case pcdomain.UniquelyResolved:
		return adapter.adoptedResolution(ctx, closure)
	case pcdomain.NoApplicableBasis:
		reason, err := checkReasonFor(closure)
		if err != nil {
			return psports.CommercialBasisResolution{}, err
		}
		return psports.CommercialBasisResolution{
			Applicability: psdomain.CommerciallyNotApplicable,
			Reason:        reason,
		}, nil
	case pcdomain.ApplicabilityConflict, pcdomain.ResolutionPending, pcdomain.InputNotAccepted:
		reason, err := checkReasonFor(closure)
		if err != nil {
			return psports.CommercialBasisResolution{}, err
		}
		return psports.CommercialBasisResolution{
			Applicability: psdomain.CommercialApplicabilityUndetermined,
			Reason:        reason,
		}, nil
	case pcdomain.ResolutionStale, pcdomain.BasisNotResolved:
		// 第一阶段从不产这两个：`已失效`与`依据未解析`都以「已有原解析」为前提。
		return psports.CommercialBasisResolution{}, fmt.Errorf("%w: first-phase closure outcome %q",
			ErrUntranslatableAnswer, closure.Outcome())
	default:
		return psports.CommercialBasisResolution{}, fmt.Errorf("%w: first-phase closure outcome %d",
			ErrUntranslatableAnswer, closure.Outcome())
	}
}

// adoptedResolution 把一份唯一闭包译成携带快照的`适用`结果。快照四类内容各有其源：
// 标识/规则包/视图修订取自闭包本身，时点声明与内容声明按已选对象经提供方端口读取
// （ADR-0042「在唯一选出后取用」），待路由许可按已选服务产品读取。
//
// 声明读不回或未配置时交回`无法判定`而不是造一份缺声明的快照：空适用组等于无条件接受，
// 正是提供方 DeclareAcceptanceRuleContent 拒绝表达的东西，适配器不得在翻译里把它造出来。
func (adapter *CommercialBasisAdapter) adoptedResolution(
	ctx context.Context,
	closure pcdomain.CommercialClosure,
) (psports.CommercialBasisResolution, error) {
	adopted, ok := closure.AdoptedFor(pcdomain.AcceptanceRulePackageObject)
	if !ok {
		// 唯一闭包却没采用规则包：解析键没把接单规则包列为必需依据。没有规则包就没有
		// 校验组与时点声明，接受语言整个立不起来——这是装配缺陷，不是一种未决。
		return psports.CommercialBasisResolution{}, fmt.Errorf("%w: adopted closure carries no acceptance rule package",
			ErrUntranslatableAnswer)
	}
	viewRevision, ok := closure.ViewRevision()
	if !ok {
		return psports.CommercialBasisResolution{}, fmt.Errorf("%w: adopted closure carries no view revision",
			ErrUntranslatableAnswer)
	}
	tenant := closure.ResolutionKey().TenantID

	content, found, err := adapter.contents.LoadAcceptanceRuleContent(ctx, tenant, adopted.Version())
	if err != nil {
		return undeterminedResolution(reasonContentUnreadable)
	}
	if !found {
		return undeterminedResolution(reasonContentNotConfigured)
	}
	applicable, err := applicableGroupsFor(content)
	if err != nil {
		return psports.CommercialBasisResolution{}, err
	}
	review, err := reviewPolicyFor(content.ManualReview())
	if err != nil {
		return psports.CommercialBasisResolution{}, err
	}

	policies, err := adapter.asOfPolicies.LoadAsOfPolicies(ctx, tenant, adopted.Version())
	if err != nil {
		return undeterminedResolution(reasonAsOfPoliciesUnreadable)
	}
	declared, err := declaredAsOfFor(policies)
	if err != nil {
		return psports.CommercialBasisResolution{}, err
	}

	allowance, resolution, stalled, err := adapter.pendingRoutingFor(ctx, tenant, closure)
	if err != nil || stalled {
		return resolution, err
	}

	snapshot, err := snapshotFor(closure, adopted, viewRevision, snapshotDeclarations{
		DeclaredAsOf:   declared,
		Applicable:     applicable,
		ManualReview:   review,
		PendingRouting: allowance,
	})
	if err != nil {
		return psports.CommercialBasisResolution{}, err
	}
	return psports.CommercialBasisResolution{
		Snapshot:      snapshot,
		Applicability: psdomain.CommerciallyApplicable,
	}, nil
}

// pendingRoutingFor 读取服务产品的待路由许可。found=false 是提供方作过的回答——产品没有
// 声明许可，即`未许可`（零值），`不可达`因此拒绝整份版本；读不回则谁也没回答过，交回未决，
// 不拿一次读取失败冒充「产品说了不许」。闭包没采用服务产品时同样保持零值：没有产品就没有
// 许可可言。
func (adapter *CommercialBasisAdapter) pendingRoutingFor(
	ctx context.Context,
	tenant pcdomain.TenantID,
	closure pcdomain.CommercialClosure,
) (psdomain.PendingRoutingAllowance, psports.CommercialBasisResolution, bool, error) {
	product, ok := closure.AdoptedFor(pcdomain.ServiceProductObject)
	if !ok {
		return psdomain.PendingRoutingAllowance{}, psports.CommercialBasisResolution{}, false, nil
	}
	permission, found, err := adapter.contents.LoadPendingRoutingPermission(ctx, tenant, product.Version())
	if err != nil {
		resolution, err := undeterminedResolution(reasonPendingRoutingUnreadable)
		return psdomain.PendingRoutingAllowance{}, resolution, true, err
	}
	if !found || !permission.Allowed() {
		return psdomain.PendingRoutingAllowance{}, psports.CommercialBasisResolution{}, false, nil
	}
	basis, err := psdomain.NewPendingRoutingBasis(permission.Basis().String())
	if err != nil {
		return psdomain.PendingRoutingAllowance{}, psports.CommercialBasisResolution{}, false,
			fmt.Errorf("%w: pending routing basis: %v", ErrUntranslatableAnswer, err)
	}
	allowance, err := psdomain.NewPendingRoutingAllowance(basis)
	if err != nil {
		return psdomain.PendingRoutingAllowance{}, psports.CommercialBasisResolution{}, false,
			fmt.Errorf("%w: pending routing allowance: %v", ErrUntranslatableAnswer, err)
	}
	return allowance, psports.CommercialBasisResolution{}, false, nil
}

// snapshotDeclarations 收拢快照里由提供方声明而非闭包本体携带的四项。
type snapshotDeclarations struct {
	DeclaredAsOf   []psdomain.DeclaredAsOf
	Applicable     psdomain.ApplicableCheckGroups
	ManualReview   psdomain.ManualReviewPolicy
	PendingRouting psdomain.PendingRoutingAllowance
}

func snapshotFor(
	closure pcdomain.CommercialClosure,
	adopted pcdomain.AdoptedBasis,
	viewRevision pcdomain.AuthorityViewRevision,
	declarations snapshotDeclarations,
) (psdomain.CommercialBasisSnapshot, error) {
	resolutionID, err := psdomain.NewCommercialResolutionID(closure.ResolutionID().String())
	if err != nil {
		return psdomain.CommercialBasisSnapshot{}, fmt.Errorf("%w: resolution ID: %v", ErrUntranslatableAnswer, err)
	}
	// 引用格式取「对象/版本」两段：与消费方既有测试语汇一致，且两段各自来自提供方的
	// 不可覆盖身份，不携带任何版本正文。
	rulePackage, err := psdomain.NewRulePackageReference(
		adopted.Version().ObjectID().String() + "/" + adopted.Version().Version().String())
	if err != nil {
		return psdomain.CommercialBasisSnapshot{}, fmt.Errorf("%w: rule package reference: %v", ErrUntranslatableAnswer, err)
	}
	revision, err := psdomain.NewCommercialViewRevision(viewRevision.String())
	if err != nil {
		return psdomain.CommercialBasisSnapshot{}, fmt.Errorf("%w: view revision: %v", ErrUntranslatableAnswer, err)
	}
	snapshot, err := psdomain.NewCommercialBasisSnapshot(psdomain.CommercialBasisSnapshotSpec{
		ResolutionID:   resolutionID,
		RulePackage:    rulePackage,
		ViewRevision:   revision,
		DeclaredAsOf:   declarations.DeclaredAsOf,
		Applicable:     declarations.Applicable,
		ManualReview:   declarations.ManualReview,
		PendingRouting: declarations.PendingRouting,
	})
	if err != nil {
		return psdomain.CommercialBasisSnapshot{}, fmt.Errorf("%w: commercial basis snapshot: %v", ErrUntranslatableAnswer, err)
	}
	return snapshot, nil
}

// applicableGroupsFor 逐格翻译规则包声明的适用校验组。两边的封闭集合逐名对应；不用数值
// 强转，两边的编号从此可以各自演化。
func applicableGroupsFor(content pcdomain.AcceptanceRuleContent) (psdomain.ApplicableCheckGroups, error) {
	declared := content.ApplicableGroups()
	groups := make([]psdomain.AcceptanceCheckGroup, 0, len(declared))
	for _, group := range declared {
		translated, err := checkGroupFor(group)
		if err != nil {
			return psdomain.ApplicableCheckGroups{}, err
		}
		groups = append(groups, translated)
	}
	applicable, err := psdomain.NewApplicableCheckGroups(groups...)
	if err != nil {
		return psdomain.ApplicableCheckGroups{}, fmt.Errorf("%w: applicable check groups: %v", ErrUntranslatableAnswer, err)
	}
	return applicable, nil
}

func checkGroupFor(group pcdomain.AcceptanceCheckGroupType) (psdomain.AcceptanceCheckGroup, error) {
	switch group {
	case pcdomain.CustomerRelationshipCheckGroup:
		return psdomain.CustomerRelationshipCheck, nil
	case pcdomain.LegalEntityAndContractCheckGroup:
		return psdomain.LegalEntityAndContractCheck, nil
	case pcdomain.ProductAndServiceCheckGroup:
		return psdomain.ProductAndServiceCheck, nil
	case pcdomain.MemberBaselineCheckGroup:
		return psdomain.MemberBaselineCheck, nil
	case pcdomain.RequiredDocumentCheckGroup:
		return psdomain.RequiredDocumentCheck, nil
	case pcdomain.PreAcceptanceFinancialControlCheckGroup:
		return psdomain.PreAcceptanceFinancialControlCheck, nil
	case pcdomain.NetworkReachabilityCheckGroup:
		return psdomain.NetworkReachabilityCheck, nil
	default:
		return psdomain.AcceptanceCheckGroupInvalid, fmt.Errorf("%w: check group %d", ErrUntranslatableAnswer, group)
	}
}

func reviewPolicyFor(directive pcdomain.ManualReviewDirective) (psdomain.ManualReviewPolicy, error) {
	switch directive {
	case pcdomain.ManualReviewUndeclared:
		return psdomain.ManualReviewNotDeclaredByRules, nil
	case pcdomain.ManualReviewNotRequired:
		return psdomain.ManualReviewNotRequiredByRules, nil
	case pcdomain.ManualReviewRequired:
		return psdomain.ManualReviewRequiredByRules, nil
	default:
		return psdomain.ManualReviewNotDeclaredByRules, fmt.Errorf("%w: manual review directive %d",
			ErrUntranslatableAnswer, directive)
	}
}

func declaredAsOfFor(policies []pcdomain.AsOfPolicy) ([]psdomain.DeclaredAsOf, error) {
	declared := make([]psdomain.DeclaredAsOf, 0, len(policies))
	for _, policy := range policies {
		kind, err := consumerJudgmentKindFor(policy.Judgment())
		if err != nil {
			return nil, err
		}
		semantics, err := psdomain.NewAsOfSemanticsReference(policy.Semantics().String())
		if err != nil {
			return nil, fmt.Errorf("%w: as-of semantics: %v", ErrUntranslatableAnswer, err)
		}
		version, err := psdomain.NewAsOfPolicyVersion(policy.PolicyVersion().String())
		if err != nil {
			return nil, fmt.Errorf("%w: as-of policy version: %v", ErrUntranslatableAnswer, err)
		}
		translated, err := psdomain.NewDeclaredAsOf(kind, semantics, version)
		if err != nil {
			return nil, fmt.Errorf("%w: declared as-of: %v", ErrUntranslatableAnswer, err)
		}
		declared = append(declared, translated)
	}
	return declared, nil
}

// checkReasonFor 把提供方的原因引用折成消费方的稳定引用：有具名原因用原因，没有用结果
// 名。前缀 PC- 标明出处，本上下文原样记录、不重新解释（端口注释）。
func checkReasonFor(closure pcdomain.CommercialClosure) (psdomain.CheckReason, error) {
	label := closure.Reason().String()
	if label == "" {
		label = closure.Outcome().String()
	}
	reason, err := psdomain.NewCheckReason("PC-" + label)
	if err != nil {
		return psdomain.CheckReason{}, fmt.Errorf("%w: check reason: %v", ErrUntranslatableAnswer, err)
	}
	return reason, nil
}

func undeterminedResolution(label string) (psports.CommercialBasisResolution, error) {
	reason, err := psdomain.NewCheckReason(label)
	if err != nil {
		return psports.CommercialBasisResolution{}, fmt.Errorf("%w: check reason: %v", ErrUntranslatableAnswer, err)
	}
	return psports.CommercialBasisResolution{
		Applicability: psdomain.CommercialApplicabilityUndetermined,
		Reason:        reason,
	}, nil
}
