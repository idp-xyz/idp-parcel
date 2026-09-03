package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/application"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// 夹具时间线：批准 → 生效边界 → 处理时刻。发布时间取处理时钟，因此边界已开的版本
// 在同一次处理内走完发布与生效两步。
var (
	pubApprovedAt = time.Date(2026, 8, 1, 9, 0, 0, 0, time.UTC)
	pubStartsAt   = time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC)
	pubNow        = time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
)

// publicationRegistryDouble 记录发布用例向登记册写了什么。未脚本化的落点按「已保存」
// 作答，让多数测试只关心自己那一格。
type publicationRegistryDouble struct {
	loaded  *domain.CommercialRegistry
	loadErr error

	versionOutcome ports.PublicationSaveOutcome
	versionErr     error
	savedVersions  []domain.CommercialVersion

	declarationOutcome ports.DeclarationSaveOutcome
	declarationErr     error
	declarationLog     []string
	savedAsOf          []domain.AsOfDeclaration
	savedContract      []domain.CustomerContract
	savedIntake        []domain.IntakeQualificationContent
	savedSettlement    []domain.SettlementPolicy

	// settlementOutcome 单列一格而不是复用 declarationOutcome：结算政策册交回的是另一族
	// 落点，本替身要能演出「政策册答了它自己那族的某一格」，用例才看得见应用层把它折成了
	// 哪一格。信用政策册与供应商协议册同理各占一格。
	settlementOutcome ports.SettlementPolicySaveOutcome

	savedCredit   []domain.CreditPolicy
	creditOutcome ports.CreditPolicySaveOutcome

	savedSupplier   []domain.SupplierAgreement
	supplierOutcome ports.SupplierAgreementSaveOutcome

	savedPrice          []domain.CommercialPricePolicy
	savedPlanDirections []domain.PriceDirection
	savedConversions    []domain.PlanBindingConversion
	priceOutcome        ports.PricePolicySaveOutcome

	savedCaliber   []domain.PricePolicyCaliber
	caliberOutcome ports.PricePolicyCaliberSaveOutcome
}

func (double *publicationRegistryDouble) SavePricePolicyCaliber(
	_ context.Context,
	caliber domain.PricePolicyCaliber,
) (ports.PricePolicyCaliberSaveOutcome, error) {
	if double.declarationErr != nil {
		return ports.PricePolicyCaliberSaveOutcomeInvalid, double.declarationErr
	}
	double.savedCaliber = append(double.savedCaliber, caliber)
	double.declarationLog = append(double.declarationLog, "price-policy-caliber")
	if double.caliberOutcome == ports.PricePolicyCaliberSaveOutcomeInvalid {
		return ports.PricePolicyCaliberSaved, nil
	}
	return double.caliberOutcome, nil
}

func (double *publicationRegistryDouble) SaveCreditPolicy(
	_ context.Context,
	policy domain.CreditPolicy,
) (ports.CreditPolicySaveOutcome, error) {
	if double.declarationErr != nil {
		return ports.CreditPolicySaveOutcomeInvalid, double.declarationErr
	}
	double.savedCredit = append(double.savedCredit, policy)
	double.declarationLog = append(double.declarationLog, "credit-policy-body")
	if double.creditOutcome == ports.CreditPolicySaveOutcomeInvalid {
		return ports.CreditPolicySaved, nil
	}
	return double.creditOutcome, nil
}

func (double *publicationRegistryDouble) SaveSupplierAgreement(
	_ context.Context,
	agreement domain.SupplierAgreement,
) (ports.SupplierAgreementSaveOutcome, error) {
	if double.declarationErr != nil {
		return ports.SupplierAgreementSaveOutcomeInvalid, double.declarationErr
	}
	double.savedSupplier = append(double.savedSupplier, agreement)
	double.declarationLog = append(double.declarationLog, "supplier-agreement-body")
	if double.supplierOutcome == ports.SupplierAgreementSaveOutcomeInvalid {
		return ports.SupplierAgreementSaved, nil
	}
	return double.supplierOutcome, nil
}

func (double *publicationRegistryDouble) declarationAnswer(name string) (ports.DeclarationSaveOutcome, error) {
	if double.declarationErr != nil {
		return ports.DeclarationSaveOutcomeInvalid, double.declarationErr
	}
	double.declarationLog = append(double.declarationLog, name)
	if double.declarationOutcome == ports.DeclarationSaveOutcomeInvalid {
		return ports.DeclarationSaved, nil
	}
	return double.declarationOutcome, nil
}

func (double *publicationRegistryDouble) SaveAsOfPolicies(
	_ context.Context,
	declaration domain.AsOfDeclaration,
) (ports.DeclarationSaveOutcome, error) {
	double.savedAsOf = append(double.savedAsOf, declaration)
	return double.declarationAnswer("as-of")
}

func (double *publicationRegistryDouble) SaveAcceptanceRuleContent(
	_ context.Context,
	_ domain.AcceptanceRuleContent,
) (ports.DeclarationSaveOutcome, error) {
	return double.declarationAnswer("acceptance-content")
}

func (double *publicationRegistryDouble) SavePendingRoutingPermission(
	_ context.Context,
	_ domain.PendingRoutingPermission,
) (ports.DeclarationSaveOutcome, error) {
	return double.declarationAnswer("pending-routing")
}

func (double *publicationRegistryDouble) SavePreAcceptanceControl(
	_ context.Context,
	_ domain.PreAcceptanceControlDeclaration,
) (ports.DeclarationSaveOutcome, error) {
	return double.declarationAnswer("pre-acceptance-control")
}

func (double *publicationRegistryDouble) SaveCustomerContractContent(
	_ context.Context,
	contract domain.CustomerContract,
) (ports.DeclarationSaveOutcome, error) {
	double.savedContract = append(double.savedContract, contract)
	return double.declarationAnswer("contract-content")
}

func (double *publicationRegistryDouble) SaveIntakeQualification(
	_ context.Context,
	content domain.IntakeQualificationContent,
) (ports.DeclarationSaveOutcome, error) {
	double.savedIntake = append(double.savedIntake, content)
	return double.declarationAnswer("intake-qualification")
}

func (double *publicationRegistryDouble) SaveFinalRule(
	_ context.Context,
	_ domain.FinalRuleContent,
) (ports.DeclarationSaveOutcome, error) {
	return double.declarationAnswer("final-rule")
}

func (double *publicationRegistryDouble) SaveCancellationAuthority(
	_ context.Context,
	_ domain.CancellationAuthorityContent,
) (ports.DeclarationSaveOutcome, error) {
	return double.declarationAnswer("cancellation-authority")
}

func (double *publicationRegistryDouble) SaveAcceptanceRulePackage(
	_ context.Context,
	_ domain.AcceptanceRulePackage,
) (ports.DeclarationSaveOutcome, error) {
	return double.declarationAnswer("rule-package-body")
}

func (double *publicationRegistryDouble) LoadForScope(
	_ context.Context,
	_ domain.TenantID,
	_ domain.CommercialScopeReference,
) (*domain.CommercialRegistry, error) {
	if double.loadErr != nil {
		return nil, double.loadErr
	}
	if double.loaded == nil {
		return domain.NewCommercialRegistry(), nil
	}
	return double.loaded, nil
}

func (double *publicationRegistryDouble) SaveVersion(
	_ context.Context,
	version domain.CommercialVersion,
) (ports.PublicationSaveOutcome, error) {
	if double.versionErr != nil {
		return ports.PublicationSaveOutcomeInvalid, double.versionErr
	}
	double.savedVersions = append(double.savedVersions, version)
	if double.versionOutcome == ports.PublicationSaveOutcomeInvalid {
		return ports.PublicationSaved, nil
	}
	return double.versionOutcome, nil
}

func (double *publicationRegistryDouble) SaveServiceProduct(
	_ context.Context,
	_ domain.ServiceProduct,
) (ports.ServiceProductSaveOutcome, error) {
	return ports.ServiceProductSaveOutcomeInvalid, errors.New("发布用例不该触碰服务形态册")
}

func (double *publicationRegistryDouble) SaveValidityCorrection(
	_ context.Context,
	_ domain.ValidityCorrection,
) (ports.ValidityCorrectionSaveOutcome, error) {
	return ports.ValidityCorrectionSaveOutcomeInvalid, errors.New("发布用例不该触碰更正册")
}

func (double *publicationRegistryDouble) SavePricePolicy(
	_ context.Context,
	policy domain.CommercialPricePolicy,
	planDirection domain.PriceDirection,
	conversion domain.PlanBindingConversion,
) (ports.PricePolicySaveOutcome, error) {
	if double.declarationErr != nil {
		return ports.PricePolicySaveOutcomeInvalid, double.declarationErr
	}
	double.savedPrice = append(double.savedPrice, policy)
	double.savedPlanDirections = append(double.savedPlanDirections, planDirection)
	double.savedConversions = append(double.savedConversions, conversion)
	double.declarationLog = append(double.declarationLog, "price-policy-body")
	if double.priceOutcome == ports.PricePolicySaveOutcomeInvalid {
		return ports.PricePolicySaved, nil
	}
	return double.priceOutcome, nil
}

func (double *publicationRegistryDouble) SaveSettlementPolicy(
	_ context.Context,
	policy domain.SettlementPolicy,
) (ports.SettlementPolicySaveOutcome, error) {
	if double.declarationErr != nil {
		return ports.SettlementPolicySaveOutcomeInvalid, double.declarationErr
	}
	double.savedSettlement = append(double.savedSettlement, policy)
	double.declarationLog = append(double.declarationLog, "settlement-policy-body")
	if double.settlementOutcome == ports.SettlementPolicySaveOutcomeInvalid {
		return ports.SettlementPolicySaved, nil
	}
	return double.settlementOutcome, nil
}

func publishSpec(t *testing.T, kind domain.CommercialObjectKind, objectID, label string) domain.CommercialVersionSpec {
	t.Helper()
	interval, err := domain.NewEffectiveInterval(pubStartsAt, time.Time{})
	if err != nil {
		t.Fatalf("有效区间：%v", err)
	}
	return domain.CommercialVersionSpec{
		TenantID:      pcValue(t, domain.NewTenantID, "tenant-1"),
		Kind:          kind,
		ObjectID:      pcValue(t, domain.NewCommercialObjectID, objectID),
		Version:       pcValue(t, domain.NewCommercialVersionLabel, label),
		Scope:         pcValue(t, domain.NewCommercialScopeReference, "scope-1"),
		ContentDigest: pcValue(t, domain.NewCommercialContentDigest, "sha256:"+objectID+"-"+label),
		Effective:     interval,
	}
}

func publishApproval(t *testing.T, objectID string) domain.ApprovalBasis {
	t.Helper()
	approval, err := domain.NewApprovalBasis(
		pcValue(t, domain.NewApprovalReference, "approval-"+objectID),
		pcValue(t, domain.NewCommercialSourceReference, "source-"+objectID),
		pubApprovedAt,
	)
	if err != nil {
		t.Fatalf("批准依据：%v", err)
	}
	return approval
}

func pcValue[T any](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	value, err := construct(raw)
	if err != nil {
		t.Fatalf("construct %q: %v", raw, err)
	}
	return value
}

// registeredEffective 把一份同规格版本按已生效走完生命周期后放进整册，演「库里已有
// 这份发布」。走真实 Publish/TakeEffect 而不是重建：折叠比对的就是发布固定下来的内容。
func registeredEffective(t *testing.T, registry *domain.CommercialRegistry, spec domain.CommercialVersionSpec, approval domain.ApprovalBasis) domain.CommercialVersion {
	t.Helper()
	draft, err := domain.NewCommercialDraft(spec)
	if err != nil {
		t.Fatalf("草稿：%v", err)
	}
	published, err := draft.Publish(approval, domain.ApprovalRoleConfirmed, pubNow.Add(-time.Hour), nil)
	if err != nil {
		t.Fatalf("发布：%v", err)
	}
	live, err := published.TakeEffect(pubNow.Add(-time.Hour))
	if err != nil {
		t.Fatalf("取效：%v", err)
	}
	if _, err := registry.Register(live); err != nil {
		t.Fatalf("入册：%v", err)
	}
	return live
}

// Covers: UC-PC-001 步骤 5–6 与结果语义「已发布」——发布携带批准责任与有效区间；
// 生效边界已开的版本在同一次处理内取效后入册（只增仓储没有事后翻状态的口，
// 停在`已发布`的行永远进不了解析，见处理器注释）。
func TestPublishingAnOpenIntervalVersionTakesEffectAndSaves(t *testing.T) {
	registry := &publicationRegistryDouble{}
	handler := application.NewPublishCommercialAuthorityHandler(registry, fixedClock{at: pubNow})

	result, err := handler.Handle(context.Background(), application.PublishCommercialAuthorityCommand{
		Spec:         publishSpec(t, domain.AcceptanceRulePackageObject, "rules-1", "v1"),
		Approval:     publishApproval(t, "rules-1"),
		RoleStanding: domain.ApprovalRoleConfirmed,
	})
	if err != nil {
		t.Fatalf("Handle：%v", err)
	}

	if result.Outcome() != application.CommercialVersionPublishedEffective {
		t.Fatalf("outcome = %q, want PUBLISHED_EFFECTIVE", result.Outcome())
	}
	version, ok := result.Version()
	if !ok {
		t.Fatal("成功发布没有交回版本")
	}
	if version.Status() != domain.CommercialVersionEffective {
		t.Fatalf("status = %q, want EFFECTIVE", version.Status())
	}
	if publishedAt, _ := version.PublishedAt(); !publishedAt.Equal(pubNow) {
		t.Fatalf("publishedAt = %v, want 处理时刻 %v", publishedAt, pubNow)
	}
	if effectiveAt, _ := version.EffectiveAt(); !effectiveAt.Equal(pubNow) {
		t.Fatalf("effectiveAt = %v, want 处理时刻 %v", effectiveAt, pubNow)
	}
	if approval, approved := version.ApprovalBasis(); !approved ||
		approval.Reference().String() != "approval-rules-1" {
		t.Fatalf("批准责任没有随版本固定：%#v", approval)
	}

	if len(registry.savedVersions) != 1 {
		t.Fatalf("saved = %d, want 1", len(registry.savedVersions))
	}
	if registry.savedVersions[0].Status() != domain.CommercialVersionEffective {
		t.Fatalf("入册状态 = %q, want EFFECTIVE", registry.savedVersions[0].Status())
	}
}

// Covers: UC-PC-001 结果语义「已计划生效」——版本已发布但尚未到达生效边界，如实以
// `已发布`入册，不提前取效；提前用于生产解析由消费侧 AppliesAt 挡住。
func TestAFutureIntervalStaysPlannedEffective(t *testing.T) {
	registry := &publicationRegistryDouble{}
	handler := application.NewPublishCommercialAuthorityHandler(registry, fixedClock{at: pubNow})

	spec := publishSpec(t, domain.AcceptanceRulePackageObject, "rules-future", "v1")
	future, err := domain.NewEffectiveInterval(pubNow.Add(30*24*time.Hour), time.Time{})
	if err != nil {
		t.Fatalf("未来区间：%v", err)
	}
	spec.Effective = future

	result, err := handler.Handle(context.Background(), application.PublishCommercialAuthorityCommand{
		Spec:         spec,
		Approval:     publishApproval(t, "rules-future"),
		RoleStanding: domain.ApprovalRoleConfirmed,
	})
	if err != nil {
		t.Fatalf("Handle：%v", err)
	}
	if result.Outcome() != application.CommercialVersionPlannedEffective {
		t.Fatalf("outcome = %q, want PLANNED_EFFECTIVE", result.Outcome())
	}
	version, _ := result.Version()
	if version.Status() != domain.CommercialVersionPublished {
		t.Fatalf("status = %q, want PUBLISHED（未到界不得取效）", version.Status())
	}
	if len(registry.savedVersions) != 1 || registry.savedVersions[0].Status() != domain.CommercialVersionPublished {
		t.Fatalf("入册的不是已发布状态：%#v", registry.savedVersions)
	}
}

// Covers: AT-PC-010「导入成功但批准角色未确认 → 保留来源，发布保持未决」与
// AT-PC-005「合同引用尚未发布的规则包 → 合同发布未决，不建立悬空生产引用」。
// 两格的恢复动作不同，原因必须随结果交回；两格都不写库。
func TestUnconfirmedRoleAndUnpublishedReferenceStayPending(t *testing.T) {
	t.Run("批准角色未确认", func(t *testing.T) {
		registry := &publicationRegistryDouble{}
		handler := application.NewPublishCommercialAuthorityHandler(registry, fixedClock{at: pubNow})

		result, err := handler.Handle(context.Background(), application.PublishCommercialAuthorityCommand{
			Spec:         publishSpec(t, domain.AcceptanceRulePackageObject, "rules-1", "v1"),
			Approval:     publishApproval(t, "rules-1"),
			RoleStanding: domain.ApprovalRoleUnconfirmed,
		})
		if err != nil {
			t.Fatalf("Handle：%v", err)
		}
		if result.Outcome() != application.CommercialPublicationPending {
			t.Fatalf("outcome = %q, want PENDING", result.Outcome())
		}
		if !errors.Is(result.PendingCause(), domain.ErrApprovalRoleNotConfirmed) {
			t.Fatalf("cause = %v, want ErrApprovalRoleNotConfirmed", result.PendingCause())
		}
		if len(registry.savedVersions) != 0 {
			t.Fatal("未决的发布写了库")
		}
	})

	t.Run("指名引用未发布", func(t *testing.T) {
		registry := &publicationRegistryDouble{}
		handler := application.NewPublishCommercialAuthorityHandler(registry, fixedClock{at: pubNow})

		spec := publishSpec(t, domain.CustomerContractObject, "contract-1", "v1")
		spec.References = map[domain.CommercialObjectKind]domain.CommercialObjectID{
			domain.AcceptanceRulePackageObject: pcValue(t, domain.NewCommercialObjectID, "rules-nowhere"),
		}
		result, err := handler.Handle(context.Background(), application.PublishCommercialAuthorityCommand{
			Spec:         spec,
			Approval:     publishApproval(t, "contract-1"),
			RoleStanding: domain.ApprovalRoleConfirmed,
		})
		if err != nil {
			t.Fatalf("Handle：%v", err)
		}
		if result.Outcome() != application.CommercialPublicationPending {
			t.Fatalf("outcome = %q, want PENDING", result.Outcome())
		}
		if !errors.Is(result.PendingCause(), domain.ErrNamedReferenceNotPublished) {
			t.Fatalf("cause = %v, want ErrNamedReferenceNotPublished", result.PendingCause())
		}
		if len(registry.savedVersions) != 0 {
			t.Fatal("悬空引用的发布写了库")
		}
	})

	t.Run("被引对象已在册则放行", func(t *testing.T) {
		loaded := domain.NewCommercialRegistry()
		registeredEffective(t, loaded,
			publishSpec(t, domain.AcceptanceRulePackageObject, "rules-1", "v1"),
			publishApproval(t, "rules-1"))
		registry := &publicationRegistryDouble{loaded: loaded}
		handler := application.NewPublishCommercialAuthorityHandler(registry, fixedClock{at: pubNow})

		spec := publishSpec(t, domain.CustomerContractObject, "contract-1", "v1")
		spec.References = map[domain.CommercialObjectKind]domain.CommercialObjectID{
			domain.AcceptanceRulePackageObject: pcValue(t, domain.NewCommercialObjectID, "rules-1"),
		}
		result, err := handler.Handle(context.Background(), application.PublishCommercialAuthorityCommand{
			Spec:         spec,
			Approval:     publishApproval(t, "contract-1"),
			RoleStanding: domain.ApprovalRoleConfirmed,
		})
		if err != nil {
			t.Fatalf("Handle：%v", err)
		}
		if result.Outcome() != application.CommercialVersionPublishedEffective {
			t.Fatalf("outcome = %q, want PUBLISHED_EFFECTIVE", result.Outcome())
		}
	})
}

// Covers: AT-PC-002「同一来源版本与摘要重复导入 → 返回原结果，不创建第二版本」。
// 整册与持久化面各自判重放，以持久化面为准。
func TestRepublishingTheSameContentIsAReplay(t *testing.T) {
	loaded := domain.NewCommercialRegistry()
	spec := publishSpec(t, domain.AcceptanceRulePackageObject, "rules-1", "v1")
	approval := publishApproval(t, "rules-1")
	registeredEffective(t, loaded, spec, approval)
	registry := &publicationRegistryDouble{loaded: loaded, versionOutcome: ports.PublicationAlreadyRegistered}
	handler := application.NewPublishCommercialAuthorityHandler(registry, fixedClock{at: pubNow})

	result, err := handler.Handle(context.Background(), application.PublishCommercialAuthorityCommand{
		Spec:         spec,
		Approval:     approval,
		RoleStanding: domain.ApprovalRoleConfirmed,
	})
	if err != nil {
		t.Fatalf("Handle：%v", err)
	}
	if result.Outcome() != application.CommercialPublicationReplayed {
		t.Fatalf("outcome = %q, want REPLAYED", result.Outcome())
	}
}

// Covers: AT-PC-003「同一来源身份携带不同正文 → 形成来源冲突，不覆盖」。冲突在折叠层
// 就被整册挡下，不再触碰持久化面。
func TestRepublishingDifferentContentIsAConflict(t *testing.T) {
	loaded := domain.NewCommercialRegistry()
	spec := publishSpec(t, domain.AcceptanceRulePackageObject, "rules-1", "v1")
	registeredEffective(t, loaded, spec, publishApproval(t, "rules-1"))
	registry := &publicationRegistryDouble{loaded: loaded}
	handler := application.NewPublishCommercialAuthorityHandler(registry, fixedClock{at: pubNow})

	changed := spec
	changed.ContentDigest = pcValue(t, domain.NewCommercialContentDigest, "sha256:another-body")
	result, err := handler.Handle(context.Background(), application.PublishCommercialAuthorityCommand{
		Spec:         changed,
		Approval:     publishApproval(t, "rules-1"),
		RoleStanding: domain.ApprovalRoleConfirmed,
	})
	if err != nil {
		t.Fatalf("Handle：%v", err)
	}
	if result.Outcome() != application.CommercialPublicationConflicted {
		t.Fatalf("outcome = %q, want CONTENT_CONFLICT", result.Outcome())
	}
	if len(registry.savedVersions) != 0 {
		t.Fatal("冲突的发布触碰了持久化面")
	}
}

// Covers: AT-PC-011「发布批逐项独立成败」——后一项冲突不得把同批前一项已落库的发布
// 撤走。本处理器一次只发一个对象、批由调用方逐项各起事务推进（Handle 注释），因此
// 这条性质靠的是「项与项之间没有共同命运」这个结构，而不是任何回滚编排。
//
// 本条守的是处理器这一半：冲突项不入册、也不动前项已交给持久化面的东西，且两次调用
// 之间处理器不留共同状态。其余用例都只发一个对象，这些都照不出来。
//
// **事务边界那一半本条守不住**：这里用的是登记册替身，没有事务，所以「有人把
// cmd/parcel-commercial 那个逐项各起事务的循环整个包进一个事务」这类回归照不出来。
// 那要一条对真库跑 runPublish 的用例，今天没有（记于 syn-wall-door-audit 票 03）。
//
// 断言取自封存现场 db81745 的同名判定（dead-session-salvage 票 02 裁定第 3 条的吸收
// 扫描），以本处理器的单对象形状重写。
func TestAConflictingItemDoesNotRetractAnEarlierSavedItem(t *testing.T) {
	loaded := domain.NewCommercialRegistry()
	contractSpec := publishSpec(t, domain.CustomerContractObject, "contract-1", "v1")
	registeredEffective(t, loaded, contractSpec, publishApproval(t, "contract-1"))
	registry := &publicationRegistryDouble{loaded: loaded}
	handler := application.NewPublishCommercialAuthorityHandler(registry, fixedClock{at: pubNow})

	// 第一项：全新对象，正常落库。
	productSpec := publishSpec(t, domain.ServiceProductObject, "product-1", "v1")
	first, err := handler.Handle(context.Background(), application.PublishCommercialAuthorityCommand{
		Spec:         productSpec,
		Approval:     publishApproval(t, "product-1"),
		RoleStanding: domain.ApprovalRoleConfirmed,
	})
	if err != nil {
		t.Fatalf("第一项 Handle：%v", err)
	}
	if first.Outcome() != application.CommercialVersionPublishedEffective {
		t.Fatalf("第一项 outcome = %q, want PUBLISHED_EFFECTIVE", first.Outcome())
	}

	// 第二项：同批的另一个对象，正文与册上不符，撞冲突。
	changed := contractSpec
	changed.ContentDigest = pcValue(t, domain.NewCommercialContentDigest, "sha256:contract-another-body")
	second, err := handler.Handle(context.Background(), application.PublishCommercialAuthorityCommand{
		Spec:         changed,
		Approval:     publishApproval(t, "contract-1"),
		RoleStanding: domain.ApprovalRoleConfirmed,
	})
	if err != nil {
		t.Fatalf("第二项 Handle：%v", err)
	}
	if second.Outcome() != application.CommercialPublicationConflicted {
		t.Fatalf("第二项 outcome = %q, want CONTENT_CONFLICT", second.Outcome())
	}

	if len(registry.savedVersions) != 1 {
		t.Fatalf("saved = %d, want 1——两项各自成败，冲突项不入册也不带走别人", len(registry.savedVersions))
	}
	if registry.savedVersions[0].ObjectID() != productSpec.ObjectID {
		t.Fatalf("册上留下的是 %q，不是第一项——后项冲突把已合法的前项撤走了",
			registry.savedVersions[0].ObjectID())
	}
}

// Covers: 票 03 缺件 1「按批发布版本化声明……写 commercial_version + 各 kind 声明表」：
// 声明随其拥有版本同一次发布登记，拥有对象是取效后的版本（声明构造门要求已生效），
// 每个通道的落点逐项入报告。
func TestDeclarationsPublishWithTheirOwningVersion(t *testing.T) {
	registry := &publicationRegistryDouble{}
	handler := application.NewPublishCommercialAuthorityHandler(registry, fixedClock{at: pubNow})

	interval, err := domain.NewEffectiveInterval(pubStartsAt, pubStartsAt.Add(90*24*time.Hour))
	if err != nil {
		t.Fatalf("适用期间：%v", err)
	}
	applicability, err := domain.NewRulePackageApplicability(
		pcValue(t, domain.NewCommercialObjectID, "product-1"),
		pcValue(t, domain.NewCommercialObjectID, "contract-1"),
		pcValue(t, domain.NewLegalEntityReference, "legal-1"),
		pcValue(t, domain.NewCommercialScopeReference, "scope-1"),
		interval,
	)
	if err != nil {
		t.Fatalf("五维适用性：%v", err)
	}
	ingressRule, err := domain.NewAssembledRule(domain.MinimumIngressIdentityRules,
		pcValue(t, domain.NewRuleReference, "RULE/ingress-identity"))
	if err != nil {
		t.Fatalf("装配规则：%v", err)
	}
	reachAsOf, err := domain.NewAsOfPolicy(domain.NetworkReachabilityJudgment,
		pcValue(t, domain.NewAsOfSemanticsReference, "AT_ACCEPTANCE"),
		pcValue(t, domain.NewAsOfPolicyVersion, "asof-policy/v1"))
	if err != nil {
		t.Fatalf("时点锚：%v", err)
	}

	result, err := handler.Handle(context.Background(), application.PublishCommercialAuthorityCommand{
		Spec:         publishSpec(t, domain.AcceptanceRulePackageObject, "rules-1", "v1"),
		Approval:     publishApproval(t, "rules-1"),
		RoleStanding: domain.ApprovalRoleConfirmed,
		Declarations: application.CommercialDeclarations{
			AsOfPolicies: []domain.AsOfPolicy{reachAsOf},
			AcceptanceContent: &application.AcceptanceContentDeclaration{
				ApplicableGroups: []domain.AcceptanceCheckGroupType{domain.RequiredDocumentCheckGroup},
				ManualReview:     domain.ManualReviewRequired,
			},
			IntakeQualification: &application.IntakeQualificationDeclaration{
				Sources: []domain.DeclaredIntakeSource{domain.DeclaredNodeIntake},
			},
			FinalRules: []domain.FinalizationDeclaration{{
				Outcome:   domain.DeclaredEffectiveDelivery,
				FinalKind: pcValue(t, domain.NewRuleReference, "FINAL/effective-delivery"),
			}},
			RulePackageBody: &application.RulePackageBodyDeclaration{
				Applicability: applicability,
				Rules:         []domain.AssembledRule{ingressRule},
			},
		},
	})
	if err != nil {
		t.Fatalf("Handle：%v", err)
	}
	if result.Outcome() != application.CommercialVersionPublishedEffective {
		t.Fatalf("outcome = %q, want PUBLISHED_EFFECTIVE", result.Outcome())
	}
	wantLog := []string{"as-of", "acceptance-content", "intake-qualification", "final-rule", "rule-package-body"}
	if len(registry.declarationLog) != len(wantLog) {
		t.Fatalf("声明写入 = %v, want %v", registry.declarationLog, wantLog)
	}
	for index, name := range wantLog {
		if registry.declarationLog[index] != name {
			t.Fatalf("声明写入 = %v, want %v", registry.declarationLog, wantLog)
		}
	}
	if len(registry.savedAsOf) != 1 ||
		registry.savedAsOf[0].RulePackage().Status() != domain.CommercialVersionEffective {
		t.Fatal("时点锚声明的拥有对象不是取效后的版本")
	}
	reports := result.Declarations()
	if len(reports) != len(wantLog) {
		t.Fatalf("报告 = %d 条, want %d", len(reports), len(wantLog))
	}
	for _, report := range reports {
		if report.Outcome != ports.DeclarationSaved {
			t.Fatalf("报告 %s = %s, want SAVED", report.Channel, report.Outcome)
		}
	}
}

// Covers: 声明只能随已到生效边界的发布登记——只增仓储没有「日后取效时补声明」的口，
// 收下一个挂在`已计划生效`版本上的声明等于登记一份永远读不出的正文。整项拒绝，
// 版本与声明都不写。
func TestDeclarationsOnAPlannedVersionAreRefusedUpFront(t *testing.T) {
	registry := &publicationRegistryDouble{}
	handler := application.NewPublishCommercialAuthorityHandler(registry, fixedClock{at: pubNow})

	spec := publishSpec(t, domain.AcceptanceRulePackageObject, "rules-future", "v1")
	future, err := domain.NewEffectiveInterval(pubNow.Add(30*24*time.Hour), time.Time{})
	if err != nil {
		t.Fatalf("未来区间：%v", err)
	}
	spec.Effective = future
	reachAsOf, err := domain.NewAsOfPolicy(domain.NetworkReachabilityJudgment,
		pcValue(t, domain.NewAsOfSemanticsReference, "AT_ACCEPTANCE"),
		pcValue(t, domain.NewAsOfPolicyVersion, "asof-policy/v1"))
	if err != nil {
		t.Fatalf("时点锚：%v", err)
	}

	if _, err := handler.Handle(context.Background(), application.PublishCommercialAuthorityCommand{
		Spec:         spec,
		Approval:     publishApproval(t, "rules-future"),
		RoleStanding: domain.ApprovalRoleConfirmed,
		Declarations: application.CommercialDeclarations{
			AsOfPolicies: []domain.AsOfPolicy{reachAsOf},
		},
	}); err == nil {
		t.Fatal("挂在未生效版本上的声明被收下了")
	}
	if len(registry.savedVersions) != 0 || len(registry.declarationLog) != 0 {
		t.Fatal("拒收的发布写了库")
	}
}

// Covers: 声明的拥有对象类别由领域构造门把守（ADR-0042/0058 的归属纪律）——把收寄
// 资格挂在客户合同上是装配错误，整项拒绝且一行不写，不是静默丢弃那一条声明。
func TestDeclarationsForTheWrongOwnerKindAreRejected(t *testing.T) {
	registry := &publicationRegistryDouble{}
	handler := application.NewPublishCommercialAuthorityHandler(registry, fixedClock{at: pubNow})

	if _, err := handler.Handle(context.Background(), application.PublishCommercialAuthorityCommand{
		Spec:         publishSpec(t, domain.CustomerContractObject, "contract-1", "v1"),
		Approval:     publishApproval(t, "contract-1"),
		RoleStanding: domain.ApprovalRoleConfirmed,
		Declarations: application.CommercialDeclarations{
			IntakeQualification: &application.IntakeQualificationDeclaration{
				Sources: []domain.DeclaredIntakeSource{domain.DeclaredNodeIntake},
			},
		},
	}); err == nil {
		t.Fatal("挂错拥有对象的声明被收下了")
	}
	if len(registry.savedVersions) != 0 || len(registry.declarationLog) != 0 {
		t.Fatal("拒收的发布写了库")
	}
}

// Covers: open-decisions F-3——版本壳指名的规则包与正文件的规则包引用都在场时必须
// 相等，不等整项拒绝，不静默选一处。
func TestContractContentMustAgreeWithTheShellReference(t *testing.T) {
	registry := &publicationRegistryDouble{}
	handler := application.NewPublishCommercialAuthorityHandler(registry, fixedClock{at: pubNow})

	spec := publishSpec(t, domain.CustomerContractObject, "contract-1", "v1")
	spec.References = map[domain.CommercialObjectKind]domain.CommercialObjectID{
		domain.AcceptanceRulePackageObject: pcValue(t, domain.NewCommercialObjectID, "rules-1"),
	}
	loaded := domain.NewCommercialRegistry()
	registeredEffective(t, loaded,
		publishSpec(t, domain.AcceptanceRulePackageObject, "rules-1", "v1"),
		publishApproval(t, "rules-1"))
	registry.loaded = loaded

	if _, err := handler.Handle(context.Background(), application.PublishCommercialAuthorityCommand{
		Spec:         spec,
		Approval:     publishApproval(t, "contract-1"),
		RoleStanding: domain.ApprovalRoleConfirmed,
		Declarations: application.CommercialDeclarations{
			ContractContent: &application.ContractContentDeclaration{
				RulePackage: pcValue(t, domain.NewCommercialObjectID, "rules-OTHER"),
			},
		},
	}); !errors.Is(err, domain.ErrRulePackageReferenceMismatch) {
		t.Fatalf("err = %v, want ErrRulePackageReferenceMismatch", err)
	}
	if len(registry.savedVersions) != 0 || len(registry.declarationLog) != 0 {
		t.Fatal("分歧的正文写了库")
	}
}

// settlementApplicability 造一份六维齐全的结算适用范围。合同维走
// domain.NewQualifiedVersionLabel，与闭包解出合同后拿去命中的那个串同出一处（ADR-0080）。
func settlementApplicability(t *testing.T, chargeScope, currency string) domain.SettlementApplicability {
	t.Helper()
	contract, err := domain.NewQualifiedVersionLabel(
		pcValue(t, domain.NewCommercialObjectID, "contract-1"),
		pcValue(t, domain.NewCommercialVersionLabel, "v1"),
	)
	if err != nil {
		t.Fatalf("两段式合同指称：%v", err)
	}
	interval, err := domain.NewEffectiveInterval(pubStartsAt, time.Time{})
	if err != nil {
		t.Fatalf("适用区间：%v", err)
	}
	applicability, err := domain.NewSettlementApplicability(
		pcValue(t, domain.NewLegalEntityReference, "legal-1"),
		pcValue(t, domain.NewCounterpartyReference, "account-1"),
		contract,
		pcValue(t, domain.NewChargeScopeReference, chargeScope),
		pcValue(t, domain.NewCurrencyCode, currency),
		interval,
	)
	if err != nil {
		t.Fatalf("六维适用范围：%v", err)
	}
	return applicability
}

// Covers: 票 commercial-closure-settlement-key/02——结算政策正文随它自己那一版发布登记。
// 在这一路接上之前，`SaveSettlementPolicy` 全仓没有生产调用方：库表、端口、适配器都在，
// 权威册里却一份结算政策也放不进去，闭包问「这个范围的结算约定是哪一份」只能答`无适用依据`。
//
// 六维原样交给持久化面是本条的重点：发布通道不得代填、不得归并任何一维（ADR-0044）。
func TestASettlementPolicyBodyPublishesWithItsOwnVersion(t *testing.T) {
	registry := &publicationRegistryDouble{}
	handler := application.NewPublishCommercialAuthorityHandler(registry, fixedClock{at: pubNow})
	applicability := settlementApplicability(t, "charge-prepaid", "CNY")

	result, err := handler.Handle(context.Background(), application.PublishCommercialAuthorityCommand{
		Spec:         publishSpec(t, domain.SettlementPolicyObject, "settlement-1", "v1"),
		Approval:     publishApproval(t, "settlement-1"),
		RoleStanding: domain.ApprovalRoleConfirmed,
		Declarations: application.CommercialDeclarations{
			SettlementPolicyBody: &application.SettlementPolicyBodyDeclaration{
				Method:        domain.PrepaidMethod,
				Applicability: applicability,
			},
		},
	})
	if err != nil {
		t.Fatalf("Handle：%v", err)
	}
	if result.Outcome() != application.CommercialVersionPublishedEffective {
		t.Fatalf("outcome = %q, want PUBLISHED_EFFECTIVE", result.Outcome())
	}

	if len(registry.savedSettlement) != 1 {
		t.Fatalf("结算政策册收到 %d 份, want 1", len(registry.savedSettlement))
	}
	saved := registry.savedSettlement[0]
	if saved.Version().Status() != domain.CommercialVersionEffective {
		t.Fatalf("拥有版本 = %q, want EFFECTIVE", saved.Version().Status())
	}
	if saved.Method() != domain.PrepaidMethod {
		t.Fatalf("结算方式 = %q, want PREPAID", saved.Method())
	}
	if saved.Applicability() != applicability {
		t.Fatalf("六维适用范围被改动了：%#v", saved.Applicability())
	}

	reports := result.Declarations()
	if len(reports) != 1 ||
		reports[0].Channel != application.SettlementPolicyBodyChannel ||
		reports[0].Outcome != ports.DeclarationSaved {
		t.Fatalf("报告 = %#v, want SETTLEMENT_POLICY_BODY=SAVED 一条", reports)
	}
}

// Covers: 结算政策正文的拥有对象类别由 domain.NewSettlementPolicy 把守——把它挂在客户
// 合同版本上是装配错误，整项拒绝且一行不写，与其余九路同一条纪律（ADR-0042/0058）。
func TestASettlementPolicyBodyOnAnotherObjectKindIsRejected(t *testing.T) {
	registry := &publicationRegistryDouble{}
	handler := application.NewPublishCommercialAuthorityHandler(registry, fixedClock{at: pubNow})

	if _, err := handler.Handle(context.Background(), application.PublishCommercialAuthorityCommand{
		Spec:         publishSpec(t, domain.CustomerContractObject, "contract-1", "v1"),
		Approval:     publishApproval(t, "contract-1"),
		RoleStanding: domain.ApprovalRoleConfirmed,
		Declarations: application.CommercialDeclarations{
			SettlementPolicyBody: &application.SettlementPolicyBodyDeclaration{
				Method:        domain.PrepaidMethod,
				Applicability: settlementApplicability(t, "charge-prepaid", "CNY"),
			},
		},
	}); !errors.Is(err, domain.ErrInvalidSettlementPolicy) {
		t.Fatalf("err = %v, want ErrInvalidSettlementPolicy", err)
	}
	if len(registry.savedVersions) != 0 || len(registry.savedSettlement) != 0 {
		t.Fatal("挂错拥有对象的结算政策正文写了库")
	}
}

// Covers: 结算政策册交回的是它自己那一族落点，应用层把它逐值折成声明通道的落点。
// `内容冲突`因此留在报告里而不是变成 error——事务保持可用，由商业责任方对着报告修正
// （ADR-0031）；折的是落点，不是把两族正文说成同一种东西（见 declarationWrites 处注释）。
func TestASettlementPolicyConflictLandsInTheReportNotInAnError(t *testing.T) {
	registry := &publicationRegistryDouble{settlementOutcome: ports.SettlementPolicyContentConflict}
	handler := application.NewPublishCommercialAuthorityHandler(registry, fixedClock{at: pubNow})

	result, err := handler.Handle(context.Background(), application.PublishCommercialAuthorityCommand{
		Spec:         publishSpec(t, domain.SettlementPolicyObject, "settlement-1", "v1"),
		Approval:     publishApproval(t, "settlement-1"),
		RoleStanding: domain.ApprovalRoleConfirmed,
		Declarations: application.CommercialDeclarations{
			SettlementPolicyBody: &application.SettlementPolicyBodyDeclaration{
				Method:        domain.TermsMethod,
				Applicability: settlementApplicability(t, "charge-terms", "CNY"),
			},
		},
	})
	if err != nil {
		t.Fatalf("Handle：%v——内容冲突不是 error", err)
	}
	reports := result.Declarations()
	if len(reports) != 1 || reports[0].Outcome != ports.DeclarationContentConflict {
		t.Fatalf("报告 = %#v, want SETTLEMENT_POLICY_BODY=CONTENT_CONFLICT 一条", reports)
	}
}

func creditPolicyBody(t *testing.T, limit domain.CreditLimit) *application.CreditPolicyBodyDeclaration {
	t.Helper()
	interval, err := domain.NewEffectiveInterval(pubStartsAt, time.Time{})
	if err != nil {
		t.Fatalf("适用区间：%v", err)
	}
	return &application.CreditPolicyBodyDeclaration{
		LegalEntity: pcValue(t, domain.NewLegalEntityReference, "legal-1"),
		Level:       pcValue(t, domain.NewAuthorityLevel, "level-commercial"),
		ChargeType:  pcValue(t, domain.NewChargeTypeReference, "charge-freight"),
		Limit:       limit,
		Effective:   interval,
	}
}

// Covers: 票 party-commercial-context-gaps/03——信用政策正文随它自己那一版发布登记。在这一路
// 接上之前，信用政策版本壳能入册、能被选中，选中之后额度无处可取。额度原样交给持久化面：
// 金额或比例哪一格在场由 CreditLimit 自己说，发布通道不代填、不换格。
func TestACreditPolicyBodyPublishesWithItsOwnVersion(t *testing.T) {
	registry := &publicationRegistryDouble{}
	handler := application.NewPublishCommercialAuthorityHandler(registry, fixedClock{at: pubNow})
	ratio, err := domain.NewCreditRatioLimit(1500)
	if err != nil {
		t.Fatalf("比例额度：%v", err)
	}

	result, err := handler.Handle(context.Background(), application.PublishCommercialAuthorityCommand{
		Spec:         publishSpec(t, domain.CreditPolicyObject, "credit-1", "v1"),
		Approval:     publishApproval(t, "credit-1"),
		RoleStanding: domain.ApprovalRoleConfirmed,
		Declarations: application.CommercialDeclarations{CreditPolicyBody: creditPolicyBody(t, ratio)},
	})
	if err != nil {
		t.Fatalf("Handle：%v", err)
	}
	if result.Outcome() != application.CommercialVersionPublishedEffective {
		t.Fatalf("outcome = %q, want PUBLISHED_EFFECTIVE", result.Outcome())
	}
	if len(registry.savedCredit) != 1 {
		t.Fatalf("信用政策册收到 %d 份, want 1", len(registry.savedCredit))
	}
	saved := registry.savedCredit[0]
	if saved.Version().Status() != domain.CommercialVersionEffective {
		t.Fatalf("拥有版本 = %q, want EFFECTIVE", saved.Version().Status())
	}
	if bps, ok := saved.AuthorizedLimit().RatioBasisPoints(); !ok || bps != 1500 {
		t.Fatalf("额度 = (%d, %v)，比例格没有原样到达持久化面", bps, ok)
	}
	reports := result.Declarations()
	if len(reports) != 1 ||
		reports[0].Channel != application.CreditPolicyBodyChannel ||
		reports[0].Outcome != ports.DeclarationSaved {
		t.Fatalf("报告 = %#v, want CREDIT_POLICY_BODY=SAVED 一条", reports)
	}
}

// Covers: 信用政策正文的拥有对象类别由 domain.NewCreditPolicy 把守——挂在别的版本上整项拒绝
// 且一行不写；信用政策册的`内容冲突`折进报告而不是 error（ADR-0031）。
func TestACreditPolicyBodyIsGuardedLikeTheOtherChannels(t *testing.T) {
	amount, err := domain.NewCreditAmountLimit(500000)
	if err != nil {
		t.Fatalf("金额额度：%v", err)
	}

	t.Run("another object kind is rejected before any write", func(t *testing.T) {
		registry := &publicationRegistryDouble{}
		handler := application.NewPublishCommercialAuthorityHandler(registry, fixedClock{at: pubNow})
		if _, err := handler.Handle(context.Background(), application.PublishCommercialAuthorityCommand{
			Spec:         publishSpec(t, domain.SettlementPolicyObject, "settlement-1", "v1"),
			Approval:     publishApproval(t, "settlement-1"),
			RoleStanding: domain.ApprovalRoleConfirmed,
			Declarations: application.CommercialDeclarations{CreditPolicyBody: creditPolicyBody(t, amount)},
		}); !errors.Is(err, domain.ErrInvalidCreditPolicy) {
			t.Fatalf("err = %v, want ErrInvalidCreditPolicy", err)
		}
		if len(registry.savedVersions) != 0 || len(registry.savedCredit) != 0 {
			t.Fatal("挂错拥有对象的信用政策正文写了库")
		}
	})

	t.Run("a content conflict lands in the report", func(t *testing.T) {
		registry := &publicationRegistryDouble{creditOutcome: ports.CreditPolicyContentConflict}
		handler := application.NewPublishCommercialAuthorityHandler(registry, fixedClock{at: pubNow})
		result, err := handler.Handle(context.Background(), application.PublishCommercialAuthorityCommand{
			Spec:         publishSpec(t, domain.CreditPolicyObject, "credit-1", "v1"),
			Approval:     publishApproval(t, "credit-1"),
			RoleStanding: domain.ApprovalRoleConfirmed,
			Declarations: application.CommercialDeclarations{CreditPolicyBody: creditPolicyBody(t, amount)},
		})
		if err != nil {
			t.Fatalf("Handle：%v——内容冲突不是 error", err)
		}
		reports := result.Declarations()
		if len(reports) != 1 || reports[0].Outcome != ports.DeclarationContentConflict {
			t.Fatalf("报告 = %#v, want CREDIT_POLICY_BODY=CONTENT_CONFLICT 一条", reports)
		}
	})
}

// Covers: 票 party-commercial-context-gaps/03——供应商商业协议正文随它自己那一版发布登记。
// CONTEXT：协议「在批准生效后，才能用于新的采购决定和供应商预期成本计算」；正文不落库，
// settlement-accounting 问采购定价方案时什么也拿不到。
func TestASupplierAgreementBodyPublishesWithItsOwnVersion(t *testing.T) {
	registry := &publicationRegistryDouble{}
	handler := application.NewPublishCommercialAuthorityHandler(registry, fixedClock{at: pubNow})
	interval, err := domain.NewEffectiveInterval(pubStartsAt, time.Time{})
	if err != nil {
		t.Fatalf("适用区间：%v", err)
	}

	result, err := handler.Handle(context.Background(), application.PublishCommercialAuthorityCommand{
		Spec:         publishSpec(t, domain.SupplierAgreementObject, "agreement-1", "v1"),
		Approval:     publishApproval(t, "agreement-1"),
		RoleStanding: domain.ApprovalRoleConfirmed,
		Declarations: application.CommercialDeclarations{
			SupplierAgreementBody: &application.SupplierAgreementBodyDeclaration{
				Supplier:     pcValue(t, domain.NewPartyID, "supplier-1"),
				LegalEntity:  pcValue(t, domain.NewLegalEntityReference, "legal-1"),
				Scope:        pcValue(t, domain.NewCommercialScopeReference, "scope-procurement"),
				PurchasePlan: pcValue(t, domain.NewPricingPlanReference, "plan-buy-1"),
				Effective:    interval,
			},
		},
	})
	if err != nil {
		t.Fatalf("Handle：%v", err)
	}
	if len(registry.savedSupplier) != 1 {
		t.Fatalf("供应商协议册收到 %d 份, want 1", len(registry.savedSupplier))
	}
	saved := registry.savedSupplier[0]
	if saved.PurchasePricingPlan().String() != "plan-buy-1" || saved.Supplier().String() != "supplier-1" {
		t.Fatalf("正文被改动了：%#v", saved)
	}
	if saved.Direction() != domain.BuyDirection {
		t.Fatalf("方向 = %q，供应商协议只能是采购", saved.Direction())
	}
	reports := result.Declarations()
	if len(reports) != 1 ||
		reports[0].Channel != application.SupplierAgreementBodyChannel ||
		reports[0].Outcome != ports.DeclarationSaved {
		t.Fatalf("报告 = %#v, want SUPPLIER_AGREEMENT_BODY=SAVED 一条", reports)
	}
}

func pricePolicyBody(t *testing.T, direction, planDirection domain.PriceDirection, conversion domain.PlanBindingConversion) *application.PricePolicyBodyDeclaration {
	t.Helper()
	interval, err := domain.NewEffectiveInterval(pubStartsAt, time.Time{})
	if err != nil {
		t.Fatalf("适用区间：%v", err)
	}
	return &application.PricePolicyBodyDeclaration{
		Direction:     direction,
		PricingPlan:   pcValue(t, domain.NewPricingPlanReference, "plan-1"),
		PlanDirection: planDirection,
		Conversion:    conversion,
		Scope:         pcValue(t, domain.NewCommercialScopeReference, "pricing-scope-1"),
		Effective:     interval,
	}
}

func sellCaliber(t *testing.T, withFx bool) *application.PricePolicyCaliberDeclaration {
	t.Helper()
	tax, err := domain.NewTaxCaliber(domain.TaxExclusive, pcValue(t, domain.NewTaxClassificationReference, "vat-standard"))
	if err != nil {
		t.Fatalf("税务口径：%v", err)
	}
	volumetric, err := domain.NewVolumetricCaliber(domain.SellDirection,
		pcValue(t, domain.NewVolumetricFactorReference, "sell-divisor-5000-cm"))
	if err != nil {
		t.Fatalf("体积口径：%v", err)
	}
	declaration := &application.PricePolicyCaliberDeclaration{Tax: tax, Volumetric: volumetric}
	if withFx {
		fx, err := domain.NewFxCaliber(
			pcValue(t, domain.NewFxQuoteTypeReference, "boc-cash-selling"),
			pcValue(t, domain.NewAsOfSemanticsReference, "AT_ORDER_DATE"),
			pcValue(t, domain.NewAsOfPolicyVersion, "asof-policy/v3"),
		)
		if err != nil {
			t.Fatalf("汇率口径：%v", err)
		}
		declaration.Fx = &fx
	}
	return declaration
}

// Covers: 票 party-commercial-context-gaps/06——价格政策正文随它自己那一版发布登记。在这一路接上
// 之前 `SavePricePolicy` 全仓没有生产调用方：租户连方向与定价方案都登不进去。发布期邻接答复
// （planDirection）与声明的转换按 ADR-0057 原样交给持久化面，不推断、不代填。
func TestAPricePolicyBodyPublishesWithItsOwnVersion(t *testing.T) {
	registry := &publicationRegistryDouble{}
	handler := application.NewPublishCommercialAuthorityHandler(registry, fixedClock{at: pubNow})

	result, err := handler.Handle(context.Background(), application.PublishCommercialAuthorityCommand{
		Spec:         publishSpec(t, domain.PriceRuleObject, "price-1", "v1"),
		Approval:     publishApproval(t, "price-1"),
		RoleStanding: domain.ApprovalRoleConfirmed,
		Declarations: application.CommercialDeclarations{
			PricePolicyBody: pricePolicyBody(t, domain.SellDirection, domain.BuyDirection, domain.PlanBindingFrozenBuyEvaluation),
		},
	})
	if err != nil {
		t.Fatalf("Handle：%v", err)
	}
	if result.Outcome() != application.CommercialVersionPublishedEffective {
		t.Fatalf("outcome = %q, want PUBLISHED_EFFECTIVE", result.Outcome())
	}
	if len(registry.savedPrice) != 1 {
		t.Fatalf("价格政策册收到 %d 份, want 1", len(registry.savedPrice))
	}
	if registry.savedPrice[0].Direction() != domain.SellDirection ||
		registry.savedPlanDirections[0] != domain.BuyDirection ||
		registry.savedConversions[0] != domain.PlanBindingFrozenBuyEvaluation {
		t.Fatalf("方向 / 方案方向 / 转换没有原样到达持久化面：%v %v %v",
			registry.savedPrice[0].Direction(), registry.savedPlanDirections[0], registry.savedConversions[0])
	}
	if len(registry.savedCaliber) != 0 {
		t.Fatal("没声明口径却写了口径册")
	}
	reports := result.Declarations()
	if len(reports) != 1 || reports[0].Channel != application.PricePolicyBodyChannel || reports[0].Outcome != ports.DeclarationSaved {
		t.Fatalf("报告 = %#v, want PRICE_POLICY_BODY=SAVED 一条", reports)
	}
}

// Covers: AT-PC-033——SELL 政策绑 BUY 价卡而未声明转换是发布冲突，整项拒绝一行不写；这道门在
// 发布面由 NewCommercialPricePolicy 把守，装载面再走一遍（ADR-0057）。
func TestAPricePolicyBodyWithAnUndeclaredCrossDirectionBindingIsRejected(t *testing.T) {
	registry := &publicationRegistryDouble{}
	handler := application.NewPublishCommercialAuthorityHandler(registry, fixedClock{at: pubNow})

	if _, err := handler.Handle(context.Background(), application.PublishCommercialAuthorityCommand{
		Spec:         publishSpec(t, domain.PriceRuleObject, "price-1", "v1"),
		Approval:     publishApproval(t, "price-1"),
		RoleStanding: domain.ApprovalRoleConfirmed,
		Declarations: application.CommercialDeclarations{
			PricePolicyBody: pricePolicyBody(t, domain.SellDirection, domain.BuyDirection, domain.PlanBindingConversionNone),
		},
	}); !errors.Is(err, domain.ErrPriceDirectionBindingConflict) {
		t.Fatalf("err = %v, want ErrPriceDirectionBindingConflict", err)
	}
	if len(registry.savedVersions) != 0 || len(registry.savedPrice) != 0 {
		t.Fatal("未声明转换的跨向绑定写了库")
	}
}

// Covers: 票 02/06——口径随价格政策正文同一次发布登记，正文先写、口径后写（0022 的外键要求
// 正文行先在）；汇率一格可缺，缺席按缺席落而不是零值。
func TestAPricePolicyCaliberPublishesAfterItsBody(t *testing.T) {
	registry := &publicationRegistryDouble{}
	handler := application.NewPublishCommercialAuthorityHandler(registry, fixedClock{at: pubNow})
	body := pricePolicyBody(t, domain.SellDirection, domain.SellDirection, domain.PlanBindingConversionNone)
	body.Caliber = sellCaliber(t, true)

	result, err := handler.Handle(context.Background(), application.PublishCommercialAuthorityCommand{
		Spec:         publishSpec(t, domain.PriceRuleObject, "price-1", "v1"),
		Approval:     publishApproval(t, "price-1"),
		RoleStanding: domain.ApprovalRoleConfirmed,
		Declarations: application.CommercialDeclarations{PricePolicyBody: body},
	})
	if err != nil {
		t.Fatalf("Handle：%v", err)
	}
	if len(registry.savedCaliber) != 1 {
		t.Fatalf("口径册收到 %d 份, want 1", len(registry.savedCaliber))
	}
	saved := registry.savedCaliber[0]
	if fx, declared := saved.Fx(); !declared || fx.QuoteType().String() != "boc-cash-selling" {
		t.Fatalf("汇率口径没有原样到达：(%v, %v)", fx, declared)
	}
	if saved.Tax().Disposition() != domain.TaxExclusive {
		t.Fatalf("税务口径 = %q", saved.Tax().Disposition())
	}
	if registry.declarationLog[len(registry.declarationLog)-2] != "price-policy-body" ||
		registry.declarationLog[len(registry.declarationLog)-1] != "price-policy-caliber" {
		t.Fatalf("写入顺序 = %v，口径必须跟在正文后面", registry.declarationLog)
	}
	reports := result.Declarations()
	if len(reports) != 2 ||
		reports[0].Channel != application.PricePolicyBodyChannel ||
		reports[1].Channel != application.PricePolicyCaliberChannel ||
		reports[1].Outcome != ports.DeclarationSaved {
		t.Fatalf("报告 = %#v, want PRICE_POLICY_BODY 与 PRICE_POLICY_CALIBER 各一条", reports)
	}

	t.Run("without fx lands as absent", func(t *testing.T) {
		registry := &publicationRegistryDouble{}
		handler := application.NewPublishCommercialAuthorityHandler(registry, fixedClock{at: pubNow})
		body := pricePolicyBody(t, domain.SellDirection, domain.SellDirection, domain.PlanBindingConversionNone)
		body.Caliber = sellCaliber(t, false)
		if _, err := handler.Handle(context.Background(), application.PublishCommercialAuthorityCommand{
			Spec:         publishSpec(t, domain.PriceRuleObject, "price-1", "v1"),
			Approval:     publishApproval(t, "price-1"),
			RoleStanding: domain.ApprovalRoleConfirmed,
			Declarations: application.CommercialDeclarations{PricePolicyBody: body},
		}); err != nil {
			t.Fatalf("Handle：%v", err)
		}
		if _, declared := registry.savedCaliber[0].Fx(); declared {
			t.Fatal("没声明汇率口径却以在场落库")
		}
	})
}

// Covers: 口径里的体积方向必须与政策方向一致——采购政策带一份销售方向的体积口径，整项拒绝
// 一行不写（正文也不写：两者是同一份声明的两半）。
func TestAPricePolicyCaliberDisagreeingWithTheBodyDirectionIsRejected(t *testing.T) {
	registry := &publicationRegistryDouble{}
	handler := application.NewPublishCommercialAuthorityHandler(registry, fixedClock{at: pubNow})
	body := pricePolicyBody(t, domain.BuyDirection, domain.BuyDirection, domain.PlanBindingConversionNone)
	body.Caliber = sellCaliber(t, false)

	if _, err := handler.Handle(context.Background(), application.PublishCommercialAuthorityCommand{
		Spec:         publishSpec(t, domain.PriceRuleObject, "price-1", "v1"),
		Approval:     publishApproval(t, "price-1"),
		RoleStanding: domain.ApprovalRoleConfirmed,
		Declarations: application.CommercialDeclarations{PricePolicyBody: body},
	}); !errors.Is(err, domain.ErrPricePolicyCaliberDirectionMismatch) {
		t.Fatalf("err = %v, want ErrPricePolicyCaliberDirectionMismatch", err)
	}
	if len(registry.savedVersions) != 0 || len(registry.savedPrice) != 0 || len(registry.savedCaliber) != 0 {
		t.Fatal("方向不一致的口径连同正文写了库")
	}
}

// Covers: 发布是写权威的动作——整册读不回时不得闭眼登记，照原样上抛等重试；这与解析
// 用例把读失败折成空视图相反（那边表达`权威不可读`并停在未决）。
func TestAnUnreadableRegistryBlocksPublication(t *testing.T) {
	registry := &publicationRegistryDouble{loadErr: errors.New("库不可达")}
	handler := application.NewPublishCommercialAuthorityHandler(registry, fixedClock{at: pubNow})

	if _, err := handler.Handle(context.Background(), application.PublishCommercialAuthorityCommand{
		Spec:         publishSpec(t, domain.AcceptanceRulePackageObject, "rules-1", "v1"),
		Approval:     publishApproval(t, "rules-1"),
		RoleStanding: domain.ApprovalRoleConfirmed,
	}); err == nil {
		t.Fatal("整册读不回却继续发布了")
	}
	if len(registry.savedVersions) != 0 {
		t.Fatal("读失败后仍写了库")
	}
}
