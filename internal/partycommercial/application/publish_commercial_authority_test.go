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
	_ domain.CommercialPricePolicy,
	_ domain.PriceDirection,
	_ domain.PlanBindingConversion,
) (ports.PricePolicySaveOutcome, error) {
	return ports.PricePolicySaveOutcomeInvalid, errors.New("发布用例不该触碰价格政策册")
}

func (double *publicationRegistryDouble) SaveSettlementPolicy(
	_ context.Context,
	_ domain.SettlementPolicy,
) (ports.SettlementPolicySaveOutcome, error) {
	return ports.SettlementPolicySaveOutcomeInvalid, errors.New("发布用例不该触碰结算政策册")
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
