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

	calls           int
	mapping         ports.MilestoneMappingRegistration
	triage          ports.TriageRuleRegistration
	notify          ports.NotificationPolicyRegistration
	claim           ports.ClaimEligibilityRegistration
	authz           ports.ClaimAuthorizationRegistration
	disclose        ports.DisclosurePolicyRegistration
	disclosureRules ports.ExceptionDisclosureRuleRegistration
	conflictSignal  ports.ConflictSignalRuleRegistration
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

func (registry *recordingRegistry) RegisterExceptionDisclosureRules(
	_ context.Context, _ domain.TenantID, registration ports.ExceptionDisclosureRuleRegistration,
) (ports.CatalogRegistrationOutcome, error) {
	registry.disclosureRules = registration
	return registry.answer()
}

func (registry *recordingRegistry) RegisterConflictSignalRule(
	_ context.Context, _ domain.TenantID, registration ports.ConflictSignalRuleRegistration,
) (ports.CatalogRegistrationOutcome, error) {
	registry.conflictSignal = registration
	return registry.answer()
}

func newRegistration(t *testing.T, registry application.CatalogRegistry) *application.CatalogRegistration {
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

func exceptionDisclosureRuleCommand(t *testing.T) application.RegisterExceptionDisclosureRulesCommand {
	t.Helper()
	return application.RegisterExceptionDisclosureRulesCommand{
		TenantID: registerTenant(t),
		Header: ports.CatalogVersionHeader{
			Version:       "SYN-EDR-V1",
			ApprovedBy:    "SYN-approver-1",
			EffectiveFrom: registerBaseAt,
		},
		Entries: []ports.ExceptionDisclosureRuleEntry{{
			Customer:    catalogScalar(t, domain.NewCustomerAccountReference, "SYN-CUSTOMER-1"),
			Kind:        catalogScalar(t, domain.NewExceptionSignalKindReference, "SYN-SIGNAL-STALL"),
			Confidence:  catalogScalar(t, domain.NewConfidenceReference, "SYN-CONF-HIGH"),
			Disclosable: true,
			AutoRelease: true,
			Content:     catalogScalar(t, domain.NewDisclosureContentReference, "SYN-CONTENT-STALL"),
		}},
	}
}

// Covers: 票 ve-disclosure-policy-view/02 步一——异常披露规则（0023）并回 CatalogRegistration。
// 正路走得通且条目三格（披露与否、能否自动发布、内容来处）原样到达写入口：登记口不替换、
// 不补齐、不丢弃任何一维。
func TestExceptionDisclosureRuleRegistrationReachesTheRegistryVerbatim(t *testing.T) {
	registry := newRecordingRegistry()
	service := newRegistration(t, registry)

	result, err := service.RegisterExceptionDisclosureRules(t.Context(), exceptionDisclosureRuleCommand(t))
	if err != nil {
		t.Fatalf("登记异常披露规则：%v", err)
	}
	if result.Outcome() != application.CatalogRegistered {
		t.Fatalf("正路应答已登记，实得 %s / %s", result.Outcome(), result.RefusalReason())
	}
	if registry.calls != 1 {
		t.Fatalf("写入口应被调用一次，实得 %d", registry.calls)
	}
	if registry.disclosureRules.Header.Version != "SYN-EDR-V1" ||
		registry.disclosureRules.Header.ApprovedBy != "SYN-approver-1" ||
		registry.disclosureRules.Header.HasEffectiveTo {
		t.Fatalf("抬头没有原样到达写入口，实得 %+v", registry.disclosureRules.Header)
	}
	if len(registry.disclosureRules.Entries) != 1 {
		t.Fatalf("条目数 = %d，要 1", len(registry.disclosureRules.Entries))
	}
	entry := registry.disclosureRules.Entries[0]
	if !entry.Disclosable || !entry.AutoRelease || entry.Content.String() != "SYN-CONTENT-STALL" {
		t.Fatalf("条目三格没有原样到达写入口，实得 %+v", entry)
	}
}

// Covers: 0023 的两条成对纪律在用例门就拒——内容随披露（披露必带、不披露必不带）、自动发布
// 不越过披露。不在这里拒而交给库上的 CHECK，撞约束的 INSERT 会把整个事务打进中止态
// （ADR-0031 不捕 23505 的同一条理由），而且登记方拿到的是一条依赖故障而不是指名的拒绝，
// 恢复动作就错了（重试 vs 改内容）。抬头缺件走与其余区间型目录同一套判据，只钉一格。
func TestIncompleteExceptionDisclosureRuleRegistrationIsRefusedByNamedGap(t *testing.T) {
	for _, testCase := range []struct {
		name   string
		mutate func(*application.RegisterExceptionDisclosureRulesCommand)
		reason application.CatalogRefusalReason
	}{
		{"缺版本号", func(command *application.RegisterExceptionDisclosureRulesCommand) {
			command.Header.Version = " "
		}, application.CatalogVersionMissing},
		{"一条条目都没有", func(command *application.RegisterExceptionDisclosureRulesCommand) {
			command.Entries = nil
		}, application.CatalogEntriesMissing},
		{"条目缺客户账户", func(command *application.RegisterExceptionDisclosureRulesCommand) {
			command.Entries[0].Customer = domain.CustomerAccountReference{}
		}, application.CatalogEntryIncomplete},
		{"条目缺信号类型", func(command *application.RegisterExceptionDisclosureRulesCommand) {
			command.Entries[0].Kind = domain.ExceptionSignalKindReference{}
		}, application.CatalogEntryIncomplete},
		{"条目缺可信度", func(command *application.RegisterExceptionDisclosureRulesCommand) {
			command.Entries[0].Confidence = domain.ConfidenceReference{}
		}, application.CatalogEntryIncomplete},
		{"声明披露却缺内容来处", func(command *application.RegisterExceptionDisclosureRulesCommand) {
			command.Entries[0].Content = domain.DisclosureContentReference{}
		}, application.CatalogEntryIncomplete},
		{"声明不披露却带了内容", func(command *application.RegisterExceptionDisclosureRulesCommand) {
			command.Entries[0].Disclosable = false
			command.Entries[0].AutoRelease = false
		}, application.CatalogEntryIncomplete},
		{"不披露却声明自动发布", func(command *application.RegisterExceptionDisclosureRulesCommand) {
			command.Entries[0].Disclosable = false
			command.Entries[0].Content = domain.DisclosureContentReference{}
			command.Entries[0].AutoRelease = true
		}, application.CatalogEntryIncomplete},
		{"同版内两条落在同一（客户+类型+可信度）键上", func(command *application.RegisterExceptionDisclosureRulesCommand) {
			duplicate := command.Entries[0]
			duplicate.AutoRelease = false
			command.Entries = append(command.Entries, duplicate)
		}, application.CatalogEntryDuplicated},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			registry := newRecordingRegistry()
			service := newRegistration(t, registry)
			command := exceptionDisclosureRuleCommand(t)
			testCase.mutate(&command)

			result, err := service.RegisterExceptionDisclosureRules(t.Context(), command)
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

	// 键含可信度：同一（客户+类型）在两个可信度下各一条是 0023 主键允许的正当形状，
	// 不是撞键——把它拒了等于宣布同一类信号不分可信度一律同一披露决定。
	registry := newRecordingRegistry()
	service := newRegistration(t, registry)
	command := exceptionDisclosureRuleCommand(t)
	command.Entries = append(command.Entries, ports.ExceptionDisclosureRuleEntry{
		Customer:    command.Entries[0].Customer,
		Kind:        command.Entries[0].Kind,
		Confidence:  catalogScalar(t, domain.NewConfidenceReference, "SYN-CONF-LOW"),
		Disclosable: false,
	})
	result, err := service.RegisterExceptionDisclosureRules(t.Context(), command)
	if err != nil {
		t.Fatalf("登记两可信度条目：%v", err)
	}
	if result.Outcome() != application.CatalogRegistered {
		t.Fatalf("同客户同类型不同可信度应登记成功，实得 %s / %s", result.Outcome(), result.RefusalReason())
	}
}

func conflictSignalRuleCommand(t *testing.T) application.RegisterConflictSignalRuleCommand {
	t.Helper()
	return application.RegisterConflictSignalRuleCommand{
		TenantID:   registerTenant(t),
		Kind:       catalogScalar(t, domain.NewExceptionSignalKindReference, "SYN-SIGNAL-FACT-CONFLICT"),
		Rule:       catalogScalar(t, domain.NewSignalRuleVersionReference, "SYN-RULE-V1"),
		Confidence: catalogScalar(t, domain.NewConfidenceReference, "SYN-CONF-MEDIUM"),
		ApprovedBy: "SYN-approver-1",
	}
}

// Covers: 票 ve-disclosure-policy-view/02 步一——冲突信号规则（0025）并回 CatalogRegistration，
// 形照 RegisterNotificationPolicy：没有版本抬头（一租户一条），缺租户是缺管辖、缺批准责任
// 是缺审批，三件规则内容（类型、识别规则版本、可信度依据）任缺一件都是条目缺维——「每个信号
// 必须保存对象、类型、规则版本、判断时间、事实依据、可信度」（CONTEXT），编排一样都不补，
// 所以登记口也一样都不放。
func TestConflictSignalRuleRegistrationRequiresKindRuleConfidenceAndApproval(t *testing.T) {
	for _, testCase := range []struct {
		name   string
		mutate func(*application.RegisterConflictSignalRuleCommand)
		reason application.CatalogRefusalReason
	}{
		{"缺租户", func(command *application.RegisterConflictSignalRuleCommand) {
			command.TenantID = domain.TenantID{}
		}, application.CatalogScopeMissing},
		{"缺发布批准责任", func(command *application.RegisterConflictSignalRuleCommand) {
			command.ApprovedBy = "  "
		}, application.CatalogApprovalMissing},
		{"缺信号类型", func(command *application.RegisterConflictSignalRuleCommand) {
			command.Kind = domain.ExceptionSignalKindReference{}
		}, application.CatalogEntryIncomplete},
		{"缺识别规则版本", func(command *application.RegisterConflictSignalRuleCommand) {
			command.Rule = domain.SignalRuleVersionReference{}
		}, application.CatalogEntryIncomplete},
		{"缺可信度依据", func(command *application.RegisterConflictSignalRuleCommand) {
			command.Confidence = domain.ConfidenceReference{}
		}, application.CatalogEntryIncomplete},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			registry := newRecordingRegistry()
			service := newRegistration(t, registry)
			command := conflictSignalRuleCommand(t)
			testCase.mutate(&command)

			result, err := service.RegisterConflictSignalRule(t.Context(), command)
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
	result, err := service.RegisterConflictSignalRule(t.Context(), conflictSignalRuleCommand(t))
	if err != nil {
		t.Fatalf("登记冲突信号规则：%v", err)
	}
	if result.Outcome() != application.CatalogRegistered {
		t.Fatalf("正路应答已登记，实得 %s / %s", result.Outcome(), result.RefusalReason())
	}
	if registry.conflictSignal.Kind.String() != "SYN-SIGNAL-FACT-CONFLICT" ||
		registry.conflictSignal.Rule.String() != "SYN-RULE-V1" ||
		registry.conflictSignal.Confidence.String() != "SYN-CONF-MEDIUM" ||
		registry.conflictSignal.ApprovedBy != "SYN-approver-1" {
		t.Fatalf("规则三件与批准责任没有原样到达写入口，实得 %+v", registry.conflictSignal)
	}
}

// Covers: 0025 头注「一租户一条、不可覆盖」——写入口撞既有行答 AlreadyRegistered，用例照
// 目录册既有代数译成版本不可覆盖：换规则版本是一次治理动作，登记口不替它静默换掉一条
// 已据以形成过信号的规则；答案不是失败，原行未被顶替。
func TestConflictSignalRuleAlreadyRegisteredTranslatesToNotOverwritable(t *testing.T) {
	registry := newRecordingRegistry()
	registry.outcome = ports.CatalogVersionAlreadyRegistered
	service := newRegistration(t, registry)

	result, err := service.RegisterConflictSignalRule(t.Context(), conflictSignalRuleCommand(t))
	if err != nil {
		t.Fatalf("写入口的业务答复不该变成错误，实得：%v", err)
	}
	if result.Outcome() != application.CatalogRegistrationRefused ||
		result.RefusalReason() != application.CatalogVersionNotOverwritable {
		t.Fatalf("撞既有行应答 REFUSED / VERSION_NOT_OVERWRITABLE，实得 %s / %s",
			result.Outcome(), result.RefusalReason())
	}
}
