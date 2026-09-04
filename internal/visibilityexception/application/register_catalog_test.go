package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/visibilityexception/application"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

var registerBaseAt = time.Date(2026, 5, 12, 9, 0, 0, 0, time.UTC)

// recordingRegistry 记下最后一次写入并按预设答复。它不模拟库上的键与区间——那些由
// 真库用例证（adapters/postgres/catalog_registration_test.go）；本层要证的是缺件在
// 到达写入口之前就被挡下，以及写入口的三格答复怎么译成用例结果。
type recordingRegistry struct {
	outcome ports.CatalogRegistrationOutcome
	err     error

	calls    int
	mapping  ports.MilestoneMappingRegistration
	triage   ports.TriageRuleRegistration
	notify   ports.NotificationPolicyRegistration
	claim    ports.ClaimEligibilityRegistration
	authz    ports.ClaimAuthorizationRegistration
	disclose ports.DisclosurePolicyRegistration
}

func newRecordingRegistry() *recordingRegistry {
	return &recordingRegistry{outcome: ports.CatalogVersionRegistered}
}

func (registry *recordingRegistry) answer() (ports.CatalogRegistrationOutcome, error) {
	registry.calls++
	if registry.err != nil {
		return ports.CatalogRegistrationOutcomeInvalid, registry.err
	}
	return registry.outcome, nil
}

func (registry *recordingRegistry) RegisterMilestoneMapping(
	_ context.Context, _ domain.TenantID, registration ports.MilestoneMappingRegistration,
) (ports.CatalogRegistrationOutcome, error) {
	registry.mapping = registration
	return registry.answer()
}

func (registry *recordingRegistry) RegisterTriageRules(
	_ context.Context, _ domain.TenantID, registration ports.TriageRuleRegistration,
) (ports.CatalogRegistrationOutcome, error) {
	registry.triage = registration
	return registry.answer()
}

func (registry *recordingRegistry) RegisterNotificationPolicy(
	_ context.Context, _ domain.TenantID, registration ports.NotificationPolicyRegistration,
) (ports.CatalogRegistrationOutcome, error) {
	registry.notify = registration
	return registry.answer()
}

func (registry *recordingRegistry) RegisterClaimEligibility(
	_ context.Context, _ domain.TenantID, registration ports.ClaimEligibilityRegistration,
) (ports.CatalogRegistrationOutcome, error) {
	registry.claim = registration
	return registry.answer()
}

func (registry *recordingRegistry) RegisterClaimAuthorization(
	_ context.Context, _ domain.TenantID, registration ports.ClaimAuthorizationRegistration,
) (ports.CatalogRegistrationOutcome, error) {
	registry.authz = registration
	return registry.answer()
}

func (registry *recordingRegistry) RegisterDisclosurePolicy(
	_ context.Context, _ domain.TenantID, registration ports.DisclosurePolicyRegistration,
) (ports.CatalogRegistrationOutcome, error) {
	registry.disclose = registration
	return registry.answer()
}

func newRegistration(t *testing.T, registry ports.CatalogRegistry) *application.CatalogRegistration {
	t.Helper()
	service, err := application.NewCatalogRegistration(registry)
	if err != nil {
		t.Fatalf("构造登记用例：%v", err)
	}
	return service
}

func registerTenant(t *testing.T) domain.TenantID {
	t.Helper()
	return catalogScalar(t, domain.NewTenantID, "tenant-a")
}

func catalogScalar[T any](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	built, err := construct(raw)
	if err != nil {
		t.Fatalf("构造 %q：%v", raw, err)
	}
	return built
}

func mappingCommand(t *testing.T) application.RegisterMilestoneMappingCommand {
	t.Helper()
	return application.RegisterMilestoneMappingCommand{
		TenantID: registerTenant(t),
		Header: ports.CatalogVersionHeader{
			Version:       "map/v1",
			ApprovedBy:    "tracking-ops",
			EffectiveFrom: registerBaseAt,
		},
		Entries: []ports.MilestoneMappingEntry{{
			Source:    domain.SourceNodeOperations,
			Kind:      catalogScalar(t, domain.NewSourceFactKind, "NODE_INTAKE"),
			Milestone: catalogScalar(t, domain.NewMilestoneReference, "ARRIVED_AT_NODE"),
		}},
	}
}

// Covers: 票 09「缺的最小机制件」——版本化登记口把整版抬头与条目交给写入口。这条钉的
// 是正路走得通，同时钉住条目原样到达：登记口不得在途中替换、补齐或丢弃任何一维。
func TestMilestoneMappingRegistrationReachesTheRegistry(t *testing.T) {
	registry := newRecordingRegistry()
	service := newRegistration(t, registry)

	result, err := service.RegisterMilestoneMapping(t.Context(), mappingCommand(t))
	if err != nil {
		t.Fatalf("登记映射：%v", err)
	}
	if result.Outcome() != application.CatalogRegistered {
		t.Fatalf("正路应答已登记，实得 %s / %s", result.Outcome(), result.RefusalReason())
	}
	if registry.calls != 1 {
		t.Fatalf("写入口应被调用一次，实得 %d", registry.calls)
	}
	if registry.mapping.Header.Version != "map/v1" ||
		registry.mapping.Header.ApprovedBy != "tracking-ops" {
		t.Fatalf("抬头没有原样到达写入口，实得 %+v", registry.mapping.Header)
	}
	if registry.mapping.Header.HasEffectiveTo {
		t.Fatal("命令没有给终点，登记口不该替它补一个——那会把当前版本登成已停用")
	}
	if len(registry.mapping.Entries) != 1 ||
		registry.mapping.Entries[0].Milestone.String() != "ARRIVED_AT_NODE" {
		t.Fatalf("条目没有原样到达写入口，实得 %+v", registry.mapping.Entries)
	}
}

// Covers: 红线「只建机制，实例值留空拒默认」。缺件逐格指名，因为恢复动作不同：缺版本
// 号要登记方给一个，缺批准责任要走审批，缺区间要定生效时间。折成一格会让人去补错东西。
func TestIncompleteMilestoneRegistrationIsRefusedByNamedGap(t *testing.T) {
	for _, testCase := range []struct {
		name   string
		mutate func(*application.RegisterMilestoneMappingCommand)
		reason application.CatalogRefusalReason
	}{
		{"缺版本号", func(command *application.RegisterMilestoneMappingCommand) {
			command.Header.Version = "  "
		}, application.CatalogVersionMissing},
		{"缺发布批准责任", func(command *application.RegisterMilestoneMappingCommand) {
			command.Header.ApprovedBy = ""
		}, application.CatalogApprovalMissing},
		{"缺生效时间", func(command *application.RegisterMilestoneMappingCommand) {
			command.Header.EffectiveFrom = time.Time{}
		}, application.CatalogEffectiveRangeMissing},
		{"终点不晚于起点", func(command *application.RegisterMilestoneMappingCommand) {
			command.Header.HasEffectiveTo = true
			command.Header.EffectiveTo = registerBaseAt
		}, application.CatalogEffectiveRangeReversed},
		{"一条条目都没有", func(command *application.RegisterMilestoneMappingCommand) {
			command.Entries = nil
		}, application.CatalogEntriesMissing},
		{"条目缺源上下文", func(command *application.RegisterMilestoneMappingCommand) {
			command.Entries[0].Source = domain.SourceContextInvalid
		}, application.CatalogEntryIncomplete},
		{"条目缺里程碑", func(command *application.RegisterMilestoneMappingCommand) {
			command.Entries[0].Milestone = domain.MilestoneReference{}
		}, application.CatalogEntryIncomplete},
		{"缺租户", func(command *application.RegisterMilestoneMappingCommand) {
			command.TenantID = domain.TenantID{}
		}, application.CatalogScopeMissing},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			registry := newRecordingRegistry()
			service := newRegistration(t, registry)
			command := mappingCommand(t)
			testCase.mutate(&command)

			result, err := service.RegisterMilestoneMapping(t.Context(), command)
			if err != nil {
				t.Fatalf("缺件是业务答复不是错误，实得：%v", err)
			}
			if result.Outcome() != application.CatalogRegistrationRefused {
				t.Fatalf("「%s」应被拒，实得 %s", testCase.name, result.Outcome())
			}
			if result.RefusalReason() != testCase.reason {
				t.Fatalf("「%s」应指名 %s，实得 %s",
					testCase.name, testCase.reason, result.RefusalReason())
			}
			if registry.calls != 0 {
				t.Fatal("缺件的登记不该到达写入口")
			}
		})
	}
}

// Covers: 同一版内两条条目撞键必须在用例里拒掉，不能指望库上的主键。撞键的 INSERT 会
// 把整个事务打进中止态（ADR-0031 的理由），而登记常与同一批别的目录写入共事务；何况
// 「同一源上下文同一事实类型归到两个里程碑」本身就是登记方要改的矛盾，不是重放。
func TestDuplicateEntryKeysWithinOneVersionAreRefused(t *testing.T) {
	registry := newRecordingRegistry()
	service := newRegistration(t, registry)
	command := mappingCommand(t)
	command.Entries = append(command.Entries, ports.MilestoneMappingEntry{
		Source:    domain.SourceNodeOperations,
		Kind:      catalogScalar(t, domain.NewSourceFactKind, "NODE_INTAKE"),
		Milestone: catalogScalar(t, domain.NewMilestoneReference, "DEPARTED_NODE"),
	})

	result, err := service.RegisterMilestoneMapping(t.Context(), command)
	if err != nil {
		t.Fatalf("撞键是业务答复不是错误，实得：%v", err)
	}
	if result.RefusalReason() != application.CatalogEntryDuplicated {
		t.Fatalf("应指名条目撞键，实得 %s", result.RefusalReason())
	}
	if registry.calls != 0 {
		t.Fatal("撞键的登记不该到达写入口")
	}
}

// Covers: 票 09 红线「同一时点两个适用版本是错误，登记口须在写入侧防重叠」。写入口的
// 两格拒绝各自译成指名的拒绝理由——一个改区间重登，一个换版本号，恢复动作不同。
func TestRegistryRefusalsTranslateToNamedReasons(t *testing.T) {
	for _, testCase := range []struct {
		name    string
		outcome ports.CatalogRegistrationOutcome
		reason  application.CatalogRefusalReason
	}{
		{"版本号已登记", ports.CatalogVersionAlreadyRegistered, application.CatalogVersionNotOverwritable},
		{"同一时点已有适用版本", ports.CatalogVersionOverlapsExisting, application.CatalogVersionOverlaps},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			registry := newRecordingRegistry()
			registry.outcome = testCase.outcome
			service := newRegistration(t, registry)

			result, err := service.RegisterMilestoneMapping(t.Context(), mappingCommand(t))
			if err != nil {
				t.Fatalf("写入口的业务答复不该变成错误，实得：%v", err)
			}
			if result.Outcome() != application.CatalogRegistrationRefused {
				t.Fatalf("「%s」应被拒，实得 %s", testCase.name, result.Outcome())
			}
			if result.RefusalReason() != testCase.reason {
				t.Fatalf("「%s」应指名 %s，实得 %s",
					testCase.name, testCase.reason, result.RefusalReason())
			}
		})
	}
}

// Covers: 依赖调不通与登记方给错件分成两条路——前者重试，后者改件重登。写入口报错时
// 用例交回错误，不吞成一个拒绝理由。
func TestRegistryFailureSurfacesAsError(t *testing.T) {
	registry := newRecordingRegistry()
	registry.err = errors.New("目录库不可达")
	service := newRegistration(t, registry)

	if _, err := service.RegisterMilestoneMapping(t.Context(), mappingCommand(t)); err == nil {
		t.Fatal("写入口报错时用例必须交回错误，不能吞成拒绝")
	}
}

// Covers: 分诊条目的封闭四走向。走向是零值即登记方没给，库上的 CHECK 是第二道网。
func TestTriageRegistrationRequiresAClosedOutcome(t *testing.T) {
	registry := newRecordingRegistry()
	service := newRegistration(t, registry)
	command := application.RegisterTriageRulesCommand{
		TenantID: registerTenant(t),
		Header: ports.CatalogVersionHeader{
			Version:       "triage/v1",
			ApprovedBy:    "exception-ops",
			EffectiveFrom: registerBaseAt,
		},
		Entries: []ports.TriageRuleEntry{{
			Kind:       catalogScalar(t, domain.NewExceptionSignalKindReference, "CUSTOMS_HOLD"),
			Confidence: catalogScalar(t, domain.NewConfidenceReference, "HIGH"),
		}},
	}

	result, err := service.RegisterTriageRules(t.Context(), command)
	if err != nil {
		t.Fatalf("登记分诊：%v", err)
	}
	if result.RefusalReason() != application.CatalogEntryIncomplete {
		t.Fatalf("缺走向应指名条目缺维，实得 %s", result.RefusalReason())
	}

	// 自动建案条目必带责任团队：光有走向没有团队，案件建不起来，仍是条目缺维。
	command.Entries[0].Outcome = domain.AutoEstablishCase
	if result, err = service.RegisterTriageRules(t.Context(), command); err != nil {
		t.Fatalf("补齐走向后登记分诊：%v", err)
	}
	if result.RefusalReason() != application.CatalogEntryIncomplete {
		t.Fatalf("自动建案缺团队应指名条目缺维，实得 %s", result.RefusalReason())
	}

	command.Entries[0].Team = catalogScalar(t, domain.NewResponsibleTeamReference, "team/customs-desk")
	if result, err = service.RegisterTriageRules(t.Context(), command); err != nil {
		t.Fatalf("补齐团队后登记分诊：%v", err)
	}
	if result.Outcome() != application.CatalogRegistered {
		t.Fatalf("补齐团队后应登记成功，实得 %s / %s", result.Outcome(), result.RefusalReason())
	}
	if registry.triage.Entries[0].Outcome != domain.AutoEstablishCase ||
		registry.triage.Entries[0].Team.String() != "team/customs-desk" {
		t.Fatalf("走向与团队没有原样到达写入口，实得 %+v", registry.triage.Entries[0])
	}

	// 反向也拒：非建案走向挂团队是把「谁来复核」误写进「建案归谁」那一格，矛盾输入不收。
	command.Entries[0].Outcome = domain.ManualReviewRequired
	if result, err = service.RegisterTriageRules(t.Context(), command); err != nil {
		t.Fatalf("人工复核带团队登记分诊：%v", err)
	}
	if result.RefusalReason() != application.CatalogEntryIncomplete {
		t.Fatalf("人工复核条目带团队应拒为条目缺维（矛盾输入），实得 %s", result.RefusalReason())
	}
}

// Covers: `PAR-VIS-07` 的三件（渠道、时限、义务判据）加发布批准责任。时限是相对量且
// 必须为正——非正时限算出的截止时间在披露决定之前或与之相等，通知一生成就已逾期。
func TestNotificationPolicyRegistrationRequiresChannelDeadlineAndApproval(t *testing.T) {
	base := func() application.RegisterNotificationPolicyCommand {
		return application.RegisterNotificationPolicyCommand{
			TenantID:      registerTenant(t),
			Policy:        catalogScalar(t, domain.NewDisclosurePolicyReference, "disclose/v1"),
			Channel:       catalogScalar(t, domain.NewNotificationChannelReference, "SMS"),
			DeadlineAfter: 2 * time.Hour,
			Obligation:    catalogScalar(t, domain.NewDisclosurePolicyReference, "DELIVERED"),
			ApprovedBy:    "customer-service",
		}
	}
	for _, testCase := range []struct {
		name   string
		mutate func(*application.RegisterNotificationPolicyCommand)
		reason application.CatalogRefusalReason
	}{
		{"缺披露策略引用", func(command *application.RegisterNotificationPolicyCommand) {
			command.Policy = domain.DisclosurePolicyReference{}
		}, application.CatalogScopeMissing},
		{"缺渠道", func(command *application.RegisterNotificationPolicyCommand) {
			command.Channel = domain.NotificationChannelReference{}
		}, application.CatalogEntryIncomplete},
		{"时限为零", func(command *application.RegisterNotificationPolicyCommand) {
			command.DeadlineAfter = 0
		}, application.CatalogEntryIncomplete},
		{"时限为负", func(command *application.RegisterNotificationPolicyCommand) {
			command.DeadlineAfter = -time.Hour
		}, application.CatalogEntryIncomplete},
		{"缺义务判据", func(command *application.RegisterNotificationPolicyCommand) {
			command.Obligation = domain.DisclosurePolicyReference{}
		}, application.CatalogEntryIncomplete},
		{"缺发布批准责任", func(command *application.RegisterNotificationPolicyCommand) {
			command.ApprovedBy = " "
		}, application.CatalogApprovalMissing},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			registry := newRecordingRegistry()
			service := newRegistration(t, registry)
			command := base()
			testCase.mutate(&command)

			result, err := service.RegisterNotificationPolicy(t.Context(), command)
			if err != nil {
				t.Fatalf("缺件是业务答复不是错误，实得：%v", err)
			}
			if result.RefusalReason() != testCase.reason {
				t.Fatalf("「%s」应指名 %s，实得 %s",
					testCase.name, testCase.reason, result.RefusalReason())
			}
			if registry.calls != 0 {
				t.Fatal("缺件的登记不该到达写入口")
			}
		})
	}

	registry := newRecordingRegistry()
	service := newRegistration(t, registry)
	result, err := service.RegisterNotificationPolicy(t.Context(), base())
	if err != nil {
		t.Fatalf("登记通知策略：%v", err)
	}
	if result.Outcome() != application.CatalogRegistered {
		t.Fatalf("正路应答已登记，实得 %s / %s", result.Outcome(), result.RefusalReason())
	}
	if registry.notify.DeadlineAfter != 2*time.Hour || registry.notify.ApprovedBy != "customer-service" {
		t.Fatalf("时限与批准责任没有原样到达写入口，实得 %+v", registry.notify)
	}
}

// Covers: 0011 的硬理由「凭一张空表永久拒掉索赔」。一份不承担任何索赔类型的责任范围
// 声明，后果与那张空表相同——由它得出的`不予受理`是 ADR-0051 认可的永久格，审过不再
// 审。因此空覆盖集在登记口就拒，逼登记方要么给出类型、要么根本不登这份声明。
func TestClaimEligibilityRegistrationRefusesAnEmptyCoveredKindSet(t *testing.T) {
	registry := newRecordingRegistry()
	service := newRegistration(t, registry)
	command := application.RegisterClaimEligibilityCommand{
		TenantID: registerTenant(t),
		Header:   ports.CatalogApprovalHeader{Version: "claim/v1", ApprovedBy: "customer-service"},
		Contract: catalogScalar(t, domain.NewContractScopeReference, "contract/v1"),
	}

	result, err := service.RegisterClaimEligibility(t.Context(), command)
	if err != nil {
		t.Fatalf("登记索赔声明：%v", err)
	}
	if result.RefusalReason() != application.CatalogEntriesMissing {
		t.Fatalf("空覆盖集应被拒，实得 %s", result.RefusalReason())
	}
	if registry.calls != 0 {
		t.Fatal("空覆盖集的声明不该到达写入口")
	}

	command.CoveredKinds = []domain.ClaimKindReference{
		catalogScalar(t, domain.NewClaimKindReference, "LOSS"),
	}
	if result, err = service.RegisterClaimEligibility(t.Context(), command); err != nil {
		t.Fatalf("补齐覆盖类型后登记：%v", err)
	}
	if result.Outcome() != application.CatalogRegistered {
		t.Fatalf("补齐后应登记成功，实得 %s / %s", result.Outcome(), result.RefusalReason())
	}
}

// Covers: 0018 的「目录在场而不在名单里 → 不受理（非永久格，名单换版照常再审）」。
// 空名单是一次可恢复的判断，与空覆盖集那条永久格不同形，因此**允许**登记——两处的
// 空集合语义相反，用同一条规则处理就会错一边。
func TestClaimAuthorizationRegistrationAcceptsAnEmptyApplicantList(t *testing.T) {
	registry := newRecordingRegistry()
	service := newRegistration(t, registry)

	result, err := service.RegisterClaimAuthorization(t.Context(),
		application.RegisterClaimAuthorizationCommand{
			TenantID: registerTenant(t),
			Header:   ports.CatalogApprovalHeader{Version: "authz/v1", ApprovedBy: "customer-service"},
			Customer: catalogScalar(t, domain.NewCustomerAccountReference, "acct-1"),
		})
	if err != nil {
		t.Fatalf("登记授权目录：%v", err)
	}
	if result.Outcome() != application.CatalogRegistered {
		t.Fatalf("空名单应登记成功（目录在场、无人获授权），实得 %s / %s",
			result.Outcome(), result.RefusalReason())
	}
	if registry.authz.Applicants != nil {
		t.Fatalf("登记口不该替空名单补人，实得 %+v", registry.authz.Applicants)
	}
}

// Covers: 披露条目四维用 domain.ViewDimension 承载——展示必带内容来处，待确认与不展示
// 必不带。零值维（登记方一维都没给）在用例里就被指名，不必等库上的 shape 约束。
func TestDisclosureRegistrationRequiresAllFourDimensions(t *testing.T) {
	registry := newRecordingRegistry()
	service := newRegistration(t, registry)
	shown, err := domain.ShowDimension(catalogScalar(t, domain.NewViewContentReference, "content/milestones"))
	if err != nil {
		t.Fatalf("构造展示维：%v", err)
	}
	command := application.RegisterDisclosurePolicyCommand{
		TenantID: registerTenant(t),
		Header: ports.CatalogVersionHeader{
			Version:       "disclose/v1",
			ApprovedBy:    "customer-service",
			EffectiveFrom: registerBaseAt,
		},
		Entries: []ports.DisclosurePolicyEntry{{
			Customer:   catalogScalar(t, domain.NewCustomerAccountReference, "acct-1"),
			Milestones: shown,
			ETA:        domain.PendDimension(),
			Final:      domain.WithholdDimension(),
		}},
	}

	result, err := service.RegisterDisclosurePolicy(t.Context(), command)
	if err != nil {
		t.Fatalf("登记披露策略：%v", err)
	}
	if result.RefusalReason() != application.CatalogEntryIncomplete {
		t.Fatalf("缺一维应指名条目缺维，实得 %s", result.RefusalReason())
	}

	command.Entries[0].Note = domain.PendDimension()
	if result, err = service.RegisterDisclosurePolicy(t.Context(), command); err != nil {
		t.Fatalf("补齐第四维后登记：%v", err)
	}
	if result.Outcome() != application.CatalogRegistered {
		t.Fatalf("四维齐备应登记成功，实得 %s / %s", result.Outcome(), result.RefusalReason())
	}
	content, shownOK := registry.disclose.Entries[0].Milestones.Content()
	if !shownOK || content.String() != "content/milestones" {
		t.Fatalf("展示维的内容来处没有原样到达写入口，实得 %+v", registry.disclose.Entries[0])
	}
}
