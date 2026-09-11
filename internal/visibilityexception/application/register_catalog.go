package application

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

// RegisterCatalogOutcome 是一次目录登记的应用处理结果。
//
// 只有两格，没有`未决`：登记是租户的管理动作，依赖调不通时没有一个如实的中间答案可
// 记——重试即可，所以那一路交回错误。业务上的拒绝一律落`已拒绝`并指名缺哪一件。
type RegisterCatalogOutcome uint8

const (
	RegisterCatalogOutcomeInvalid RegisterCatalogOutcome = iota
	CatalogRegistered
	CatalogRegistrationRefused
)

func (outcome RegisterCatalogOutcome) String() string {
	switch outcome {
	case CatalogRegistered:
		return "REGISTERED"
	case CatalogRegistrationRefused:
		return "REFUSED"
	default:
		return ""
	}
}

// CatalogRefusalReason 指名这一笔登记差在哪一件。逐格分开是因为恢复动作各不相同：
// 缺版本号要登记方给一个，缺发布批准责任要走审批，区间倒序要改时间，条目撞键要改
// 内容，版本号已登记要换号，区间重叠要改区间。折成一格会让人去补错东西。
type CatalogRefusalReason uint8

const (
	CatalogRefusalReasonNone CatalogRefusalReason = iota
	CatalogScopeMissing
	CatalogVersionMissing
	CatalogApprovalMissing
	CatalogEffectiveRangeMissing
	CatalogEffectiveRangeReversed
	CatalogEntriesMissing
	CatalogEntryIncomplete
	CatalogEntryDuplicated
	CatalogVersionNotOverwritable
	CatalogVersionOverlaps
)

func (reason CatalogRefusalReason) String() string {
	switch reason {
	case CatalogScopeMissing:
		return "SCOPE_MISSING"
	case CatalogVersionMissing:
		return "VERSION_MISSING"
	case CatalogApprovalMissing:
		return "APPROVAL_MISSING"
	case CatalogEffectiveRangeMissing:
		return "EFFECTIVE_RANGE_MISSING"
	case CatalogEffectiveRangeReversed:
		return "EFFECTIVE_RANGE_REVERSED"
	case CatalogEntriesMissing:
		return "ENTRIES_MISSING"
	case CatalogEntryIncomplete:
		return "ENTRY_INCOMPLETE"
	case CatalogEntryDuplicated:
		return "ENTRY_DUPLICATED"
	case CatalogVersionNotOverwritable:
		return "VERSION_NOT_OVERWRITABLE"
	case CatalogVersionOverlaps:
		return "VERSION_OVERLAPS_EXISTING"
	default:
		return ""
	}
}

type RegisterCatalogResult struct {
	outcome RegisterCatalogOutcome
	refusal CatalogRefusalReason
}

func (result RegisterCatalogResult) Outcome() RegisterCatalogOutcome {
	return result.outcome
}

// RefusalReason 只在被拒时有值。
func (result RegisterCatalogResult) RefusalReason() CatalogRefusalReason {
	return result.refusal
}

func registered() RegisterCatalogResult {
	return RegisterCatalogResult{outcome: CatalogRegistered}
}

func refused(reason CatalogRefusalReason) RegisterCatalogResult {
	return RegisterCatalogResult{outcome: CatalogRegistrationRefused, refusal: reason}
}

// RegisterMilestoneMappingCommand 登记一版里程碑映射（`PAR-VIS-01`）。租户显式随命令
// 到达（ADR-0003）：目录行只在租户内唯一。
type RegisterMilestoneMappingCommand struct {
	TenantID domain.TenantID
	Header   ports.CatalogVersionHeader
	Entries  []ports.MilestoneMappingEntry
}

// RegisterTriageRulesCommand 登记一版分诊规则（`PAR-VIS-05`）。
type RegisterTriageRulesCommand struct {
	TenantID domain.TenantID
	Header   ports.CatalogVersionHeader
	Entries  []ports.TriageRuleEntry
}

// RegisterNotificationPolicyCommand 登记一条通知策略（`PAR-VIS-07`）。没有版本抬头，
// 理由见 ports.NotificationPolicyRegistration。
type RegisterNotificationPolicyCommand struct {
	TenantID      domain.TenantID
	Policy        domain.DisclosurePolicyReference
	Channel       domain.NotificationChannelReference
	DeadlineAfter time.Duration
	Obligation    domain.DisclosurePolicyReference
	ApprovedBy    string
}

// RegisterClaimEligibilityCommand 登记一份合同责任范围的索赔资格声明（`PAR-VIS-08`）。
type RegisterClaimEligibilityCommand struct {
	TenantID     domain.TenantID
	Header       ports.CatalogApprovalHeader
	Contract     domain.ContractScopeReference
	CoveredKinds []domain.ClaimKindReference
}

// RegisterClaimAuthorizationCommand 登记一个货主客户账户的申请人授权名单（`PAR-VIS-08`
// 的授权角）。
type RegisterClaimAuthorizationCommand struct {
	TenantID   domain.TenantID
	Header     ports.CatalogApprovalHeader
	Customer   domain.CustomerAccountReference
	Applicants []domain.ApplicantReference
}

// RegisterDisclosurePolicyCommand 登记一版披露策略（`PAR-VIS-09`）。
type RegisterDisclosurePolicyCommand struct {
	TenantID domain.TenantID
	Header   ports.CatalogVersionHeader
	Entries  []ports.DisclosurePolicyEntry
}

// RegisterExceptionDisclosureRulesCommand 登记一版异常披露规则（`PAR-VIS-07` 的披露与
// 自动发布范围半边，0023）。与 RegisterDisclosurePolicyCommand 是相邻的两本册：那册按
// 客户答四维展示什么，这册按（客户+信号类型+可信度）答异常要不要对外说、能不能自动发。
type RegisterExceptionDisclosureRulesCommand struct {
	TenantID domain.TenantID
	Header   ports.CatalogVersionHeader
	Entries  []ports.ExceptionDisclosureRuleEntry
}

// RegisterConflictSignalRuleCommand 登记本租户的冲突信号规则（`PAR-VIS-04`，0025）。没有
// 版本抬头：一租户一条，换版是治理动作不是接续闭合（ports.ConflictSignalRuleRegistry）。
type RegisterConflictSignalRuleCommand struct {
	TenantID   domain.TenantID
	Kind       domain.ExceptionSignalKindReference
	Rule       domain.SignalRuleVersionReference
	Confidence domain.ConfidenceReference
	ApprovedBy string
}

// CatalogRegistry 是本用例对写入口的全部要求：目录册写口（ports.CatalogRegistry）加异常
// 披露规则与冲突信号规则两册的写口。ports 里各写口分立是 mech/08 当时为了不拆替身；用例只有
// 一个、受控入口只有一个、留痕只有一处，所以在这里合成一口——生产写入方
// postgres.CatalogRegistrar 各写口本就齐备，代价只落在测试替身补齐那两册的写法。
type CatalogRegistry interface {
	ports.CatalogRegistry
	ports.ExceptionDisclosureRuleRegistry
	ports.ConflictSignalRuleRegistry
}

// CatalogRegistration 是 VE 五类规则与策略目录的版本化登记用例。
//
// 它站在写入口之前，只作一件事：**把不完整的登记挡在库外**。目录内容属实例半边、
// 待租户提供（`PAR-VIS-01`/`05`/`07`/`08`/`09` 全部「待提供」），所以本用例一个默认值
// 都不补——缺版本号就拒，缺发布批准责任就拒，缺条目就拒。补一个占位值会把「还没人
// 登记」变成一次有依据的判断，而那些判断里有几格（索赔`不予受理`）是永久的。
//
// 有效区间的重叠不在这里判：只有写入口看得见同一租户已经登记过什么，也只有它能把
// 判定与插入放进同一把锁。用例把写入口的两格拒绝译成指名的拒绝理由，不自己先查一遍
// ——先查后写在并发下判得不准，还会让人以为已经有人守着。
//
// 事务边界不归本用例：一版抬头与它的整版条目必须同一提交，而事务由进程级入口开启
// （与本上下文其余写路一致）。
type CatalogRegistration struct {
	registry CatalogRegistry
}

func NewCatalogRegistration(registry CatalogRegistry) (*CatalogRegistration, error) {
	if registry == nil {
		return nil, fmt.Errorf("visibility exception application: catalog registry is required")
	}
	return &CatalogRegistration{registry: registry}, nil
}

func (service *CatalogRegistration) RegisterMilestoneMapping(
	ctx context.Context,
	command RegisterMilestoneMappingCommand,
) (RegisterCatalogResult, error) {
	if reason := checkVersionHeader(command.TenantID, command.Header); reason != CatalogRefusalReasonNone {
		return refused(reason), nil
	}
	if reason := checkEntries(command.Entries, milestoneEntryComplete, milestoneEntryKey); reason != CatalogRefusalReasonNone {
		return refused(reason), nil
	}

	outcome, err := service.registry.RegisterMilestoneMapping(ctx, command.TenantID,
		ports.MilestoneMappingRegistration{Header: command.Header, Entries: command.Entries})
	if err != nil {
		return RegisterCatalogResult{}, fmt.Errorf("register milestone mapping: %w", err)
	}
	return translateRegistryOutcome(outcome)
}

func (service *CatalogRegistration) RegisterTriageRules(
	ctx context.Context,
	command RegisterTriageRulesCommand,
) (RegisterCatalogResult, error) {
	if reason := checkVersionHeader(command.TenantID, command.Header); reason != CatalogRefusalReasonNone {
		return refused(reason), nil
	}
	if reason := checkEntries(command.Entries, triageEntryComplete, triageEntryKey); reason != CatalogRefusalReasonNone {
		return refused(reason), nil
	}

	outcome, err := service.registry.RegisterTriageRules(ctx, command.TenantID,
		ports.TriageRuleRegistration{Header: command.Header, Entries: command.Entries})
	if err != nil {
		return RegisterCatalogResult{}, fmt.Errorf("register triage rules: %w", err)
	}
	return translateRegistryOutcome(outcome)
}

// RegisterNotificationPolicy 登记一条通知策略。时限必须为正：非正时限算出的截止时间
// 在披露决定之前或与之相等，那样的通知一生成就已逾期。
func (service *CatalogRegistration) RegisterNotificationPolicy(
	ctx context.Context,
	command RegisterNotificationPolicyCommand,
) (RegisterCatalogResult, error) {
	switch {
	case !present(command.TenantID.String()) || !present(command.Policy.String()):
		return refused(CatalogScopeMissing), nil
	case !present(command.ApprovedBy):
		return refused(CatalogApprovalMissing), nil
	case !present(command.Channel.String()),
		command.DeadlineAfter <= 0,
		!present(command.Obligation.String()):
		return refused(CatalogEntryIncomplete), nil
	}

	outcome, err := service.registry.RegisterNotificationPolicy(ctx, command.TenantID,
		ports.NotificationPolicyRegistration{
			Policy:        command.Policy,
			Channel:       command.Channel,
			DeadlineAfter: command.DeadlineAfter,
			Obligation:    command.Obligation,
			ApprovedBy:    command.ApprovedBy,
		})
	if err != nil {
		return RegisterCatalogResult{}, fmt.Errorf("register notification policy: %w", err)
	}
	return translateRegistryOutcome(outcome)
}

// RegisterClaimEligibility 登记一份索赔资格声明。空覆盖集在这里就拒：一份不承担任何
// 索赔类型的责任范围声明，后果与 0011 竭力避免的那张空表相同——由它得出的`不予受理`
// 是 ADR-0051 认可的永久格，审过不再审，答错了没有第二次机会。要么给出类型，要么根本
// 不登这份声明（没有声明行时，缺一个类型是「没人声明过」，那一格可续办）。
func (service *CatalogRegistration) RegisterClaimEligibility(
	ctx context.Context,
	command RegisterClaimEligibilityCommand,
) (RegisterCatalogResult, error) {
	if reason := checkApprovalHeader(command.TenantID, command.Header, command.Contract.String()); reason != CatalogRefusalReasonNone {
		return refused(reason), nil
	}
	if reason := checkEntries(command.CoveredKinds, referencePresent, referenceKey); reason != CatalogRefusalReasonNone {
		return refused(reason), nil
	}

	outcome, err := service.registry.RegisterClaimEligibility(ctx, command.TenantID,
		ports.ClaimEligibilityRegistration{
			Header:       command.Header,
			Contract:     command.Contract,
			CoveredKinds: command.CoveredKinds,
		})
	if err != nil {
		return RegisterCatalogResult{}, fmt.Errorf("register claim eligibility: %w", err)
	}
	return translateRegistryOutcome(outcome)
}

// RegisterClaimAuthorization 登记一个客户账户的申请人授权名单。
//
// 空名单**允许**，与上面那条空覆盖集相反：名单在场而申请人不在列，核出的是「不受理」
// （0018），名单换版后同一索赔照常再审，是一次可恢复的判断；而空覆盖集通向永久格。
// 两处的空集合语义相反，用同一条规则处理必错一边。名单里有空引用仍然要拒——那是登记
// 方漏填，不是「不授权任何人」。
func (service *CatalogRegistration) RegisterClaimAuthorization(
	ctx context.Context,
	command RegisterClaimAuthorizationCommand,
) (RegisterCatalogResult, error) {
	if reason := checkApprovalHeader(command.TenantID, command.Header, command.Customer.String()); reason != CatalogRefusalReasonNone {
		return refused(reason), nil
	}
	if reason := checkOptionalEntries(command.Applicants, referencePresent, referenceKey); reason != CatalogRefusalReasonNone {
		return refused(reason), nil
	}

	outcome, err := service.registry.RegisterClaimAuthorization(ctx, command.TenantID,
		ports.ClaimAuthorizationRegistration{
			Header:     command.Header,
			Customer:   command.Customer,
			Applicants: command.Applicants,
		})
	if err != nil {
		return RegisterCatalogResult{}, fmt.Errorf("register claim authorization: %w", err)
	}
	return translateRegistryOutcome(outcome)
}

func (service *CatalogRegistration) RegisterDisclosurePolicy(
	ctx context.Context,
	command RegisterDisclosurePolicyCommand,
) (RegisterCatalogResult, error) {
	if reason := checkVersionHeader(command.TenantID, command.Header); reason != CatalogRefusalReasonNone {
		return refused(reason), nil
	}
	if reason := checkEntries(command.Entries, disclosureEntryComplete, disclosureEntryKey); reason != CatalogRefusalReasonNone {
		return refused(reason), nil
	}

	outcome, err := service.registry.RegisterDisclosurePolicy(ctx, command.TenantID,
		ports.DisclosurePolicyRegistration{Header: command.Header, Entries: command.Entries})
	if err != nil {
		return RegisterCatalogResult{}, fmt.Errorf("register disclosure policy: %w", err)
	}
	return translateRegistryOutcome(outcome)
}

// RegisterExceptionDisclosureRules 登记一版异常披露规则。抬头与条目纪律同其余区间型目录；
// 条目的成对判据见 exceptionDisclosureEntryComplete。
func (service *CatalogRegistration) RegisterExceptionDisclosureRules(
	ctx context.Context,
	command RegisterExceptionDisclosureRulesCommand,
) (RegisterCatalogResult, error) {
	if reason := checkVersionHeader(command.TenantID, command.Header); reason != CatalogRefusalReasonNone {
		return refused(reason), nil
	}
	if reason := checkEntries(command.Entries, exceptionDisclosureEntryComplete, exceptionDisclosureEntryKey); reason != CatalogRefusalReasonNone {
		return refused(reason), nil
	}

	outcome, err := service.registry.RegisterExceptionDisclosureRules(ctx, command.TenantID,
		ports.ExceptionDisclosureRuleRegistration{Header: command.Header, Entries: command.Entries})
	if err != nil {
		return RegisterCatalogResult{}, fmt.Errorf("register exception disclosure rules: %w", err)
	}
	return translateRegistryOutcome(outcome)
}

// RegisterConflictSignalRule 登记本租户的冲突信号规则，形照 RegisterNotificationPolicy：
// 缺租户是缺管辖，缺批准责任是缺审批，规则三件（类型、识别规则版本、可信度依据）任缺一件
// 都是条目缺维——「每个信号必须保存对象、类型、规则版本、判断时间、事实依据、可信度」
// （CONTEXT），编排形成信号时一样都不补，登记口因此也一样都不放。写入口撞既有行答
// AlreadyRegistered，照目录册代数译成版本不可覆盖：换规则版本是治理动作，原行不被顶替。
func (service *CatalogRegistration) RegisterConflictSignalRule(
	ctx context.Context,
	command RegisterConflictSignalRuleCommand,
) (RegisterCatalogResult, error) {
	switch {
	case !present(command.TenantID.String()):
		return refused(CatalogScopeMissing), nil
	case !present(command.ApprovedBy):
		return refused(CatalogApprovalMissing), nil
	case !present(command.Kind.String()),
		!present(command.Rule.String()),
		!present(command.Confidence.String()):
		return refused(CatalogEntryIncomplete), nil
	}

	outcome, err := service.registry.RegisterConflictSignalRule(ctx, command.TenantID,
		ports.ConflictSignalRuleRegistration{
			Kind:       command.Kind,
			Rule:       command.Rule,
			Confidence: command.Confidence,
			ApprovedBy: command.ApprovedBy,
		})
	if err != nil {
		return RegisterCatalogResult{}, fmt.Errorf("register conflict signal rule: %w", err)
	}
	return translateRegistryOutcome(outcome)
}

// translateRegistryOutcome 把写入口的三格译成用例结果。写入口交回一个它自己都不认识
// 的格是实现坏了，不是业务答案——交回错误，不吸收成某一格。
func translateRegistryOutcome(outcome ports.CatalogRegistrationOutcome) (RegisterCatalogResult, error) {
	switch outcome {
	case ports.CatalogVersionRegistered:
		return registered(), nil
	case ports.CatalogVersionAlreadyRegistered:
		return refused(CatalogVersionNotOverwritable), nil
	case ports.CatalogVersionOverlapsExisting:
		return refused(CatalogVersionOverlaps), nil
	default:
		return RegisterCatalogResult{}, fmt.Errorf(
			"visibility exception application: 未知目录登记结果 %d", outcome)
	}
}

// checkVersionHeader 核区间型目录的抬头四件。生效时间零值即登记方没给——绝对时刻的
// 零值不是一个能用的生效边界，拿它登记等于让这一版从公元元年起就适用。
func checkVersionHeader(tenant domain.TenantID, header ports.CatalogVersionHeader) CatalogRefusalReason {
	switch {
	case !present(tenant.String()):
		return CatalogScopeMissing
	case !present(header.Version):
		return CatalogVersionMissing
	case !present(header.ApprovedBy):
		return CatalogApprovalMissing
	case header.EffectiveFrom.IsZero():
		return CatalogEffectiveRangeMissing
	case header.HasEffectiveTo && !header.EffectiveTo.After(header.EffectiveFrom):
		return CatalogEffectiveRangeReversed
	default:
		return CatalogRefusalReasonNone
	}
}

// checkApprovalHeader 核无区间那两份目录的抬头。scope 是它们的身份维（合同责任范围
// 或货主客户账户）——那两张表以它作键，缺了连登记对象都指不出来。
func checkApprovalHeader(
	tenant domain.TenantID,
	header ports.CatalogApprovalHeader,
	scope string,
) CatalogRefusalReason {
	switch {
	case !present(tenant.String()) || !present(scope):
		return CatalogScopeMissing
	case !present(header.Version):
		return CatalogVersionMissing
	case !present(header.ApprovedBy):
		return CatalogApprovalMissing
	default:
		return CatalogRefusalReasonNone
	}
}

// checkEntries 核一版条目：至少一条、逐条齐备、彼此不撞键。
//
// 撞键必须在这里拒，不能指望库上的主键：撞键的 INSERT 会把整个事务打进中止态
// （ADR-0031 不捕 23505 的同一条理由），而登记常与同一批别的目录写入共事务。何况
// 「同一键归到两个结果」是登记方要改的矛盾，不是一次重放。
func checkEntries[Entry any](
	entries []Entry,
	complete func(Entry) bool,
	keyOf func(Entry) string,
) CatalogRefusalReason {
	if len(entries) == 0 {
		return CatalogEntriesMissing
	}
	return checkOptionalEntries(entries, complete, keyOf)
}

// checkOptionalEntries 与 checkEntries 同，但允许空集合——只用于申请人授权名单，那里
// 的空集合是一次可恢复的判断而不是永久格（理由见 RegisterClaimAuthorization）。
func checkOptionalEntries[Entry any](
	entries []Entry,
	complete func(Entry) bool,
	keyOf func(Entry) string,
) CatalogRefusalReason {
	seen := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		if !complete(entry) {
			return CatalogEntryIncomplete
		}
		key := keyOf(entry)
		if _, duplicated := seen[key]; duplicated {
			return CatalogEntryDuplicated
		}
		seen[key] = struct{}{}
	}
	return CatalogRefusalReasonNone
}

func milestoneEntryComplete(entry ports.MilestoneMappingEntry) bool {
	return present(entry.Source.String()) &&
		present(entry.Kind.String()) &&
		present(entry.Milestone.String())
}

// milestoneEntryKey 与库上主键同维（源上下文+事实类型）：「一行覆盖此后同类型事实」
// （CONTEXT 硬句），所以同一版里同一（源上下文+类型）只能有一条。
func milestoneEntryKey(entry ports.MilestoneMappingEntry) string {
	return entry.Source.String() + "\x00" + entry.Kind.String()
}

// triageEntryComplete 除三件必备外还核团队与走向成对：`自动建案`必带责任团队（没有团队
// 的案件建不起来，CONTEXT「每个开放案件始终必须有一个内部案件责任团队」），其余走向必
// 不带——给一条人工复核条目挂个团队，是登记方把「谁来复核」误写进了「建案归谁」那一格，
// 矛盾输入不收而不是静默丢掉。
func triageEntryComplete(entry ports.TriageRuleEntry) bool {
	if !present(entry.Kind.String()) ||
		!present(entry.Confidence.String()) ||
		!present(entry.Outcome.String()) {
		return false
	}
	return (entry.Outcome == domain.AutoEstablishCase) == present(entry.Team.String())
}

// triageEntryKey 含可信度：四走向的分界正立在它上面，按类型单键会宣布同一类型的信号
// 不分可信度一律同一走向。
func triageEntryKey(entry ports.TriageRuleEntry) string {
	return entry.Kind.String() + "\x00" + entry.Confidence.String()
}

func disclosureEntryComplete(entry ports.DisclosurePolicyEntry) bool {
	return present(entry.Customer.String()) &&
		dimensionPresent(entry.Milestones) &&
		dimensionPresent(entry.ETA) &&
		dimensionPresent(entry.Final) &&
		dimensionPresent(entry.Note)
}

func disclosureEntryKey(entry ports.DisclosurePolicyEntry) string {
	return entry.Customer.String()
}

// exceptionDisclosureEntryComplete 除键三件必备外还核 0023 的两条成对纪律：内容随披露
// （披露必带内容快照、不披露必不带），自动发布不越过披露（不披露就谈不上自动发）。在
// 这里拒而不是交给库上的 CHECK：撞 CHECK 的 INSERT 会把整个事务打进中止态（ADR-0031
// 不捕 23505 的同一条理由），而且登记方拿到的是依赖故障而不是指名的拒绝——恢复动作从
// 「改内容」变成了「重试」，重试多少次都不会好。
func exceptionDisclosureEntryComplete(entry ports.ExceptionDisclosureRuleEntry) bool {
	if !present(entry.Customer.String()) ||
		!present(entry.Kind.String()) ||
		!present(entry.Confidence.String()) {
		return false
	}
	if entry.Disclosable != present(entry.Content.String()) {
		return false
	}
	return entry.Disclosable || !entry.AutoRelease
}

// exceptionDisclosureEntryKey 与 0023 主键同维（客户+信号类型+可信度）：披露决定按这三维
// 查规则，同一版里同一键只能有一条；键含可信度，因为同一类信号在不同可信度下披露与否
// 本就可以不同。
func exceptionDisclosureEntryKey(entry ports.ExceptionDisclosureRuleEntry) string {
	return entry.Customer.String() + "\x00" + entry.Kind.String() + "\x00" + entry.Confidence.String()
}

// dimensionPresent 只核这一维在不在。展示必带内容、待确认与不展示必不带那条不变量由
// domain.ShowDimension / PendDimension / WithholdDimension 在构造期担保——零值维的
// 状态译不出字符串，正是「登记方这一维一个字都没给」。
func dimensionPresent(dimension domain.ViewDimension) bool {
	return present(dimension.State().String())
}

// referencePresent / referenceKey 服务于纯引用列表（覆盖类型、授权申请人）。
func referencePresent[Reference fmt.Stringer](reference Reference) bool {
	return present(reference.String())
}

func referenceKey[Reference fmt.Stringer](reference Reference) string {
	return reference.String()
}

func present(value string) bool {
	return strings.TrimSpace(value) != ""
}
