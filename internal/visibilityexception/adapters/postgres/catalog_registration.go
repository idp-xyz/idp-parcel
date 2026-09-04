package postgres

import (
	"context"
	"fmt"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

// 目录族锁号。写入侧防重叠要把「判定」与「插入」关进同一把锁，而锁只需按（租户+族）
// 排他——不同族的登记互不相干，同族并发登记两个重叠区间才是要挡的那件事。
//
// 号只用作事务锁键，不落库：跨版本换了值也不影响任何已存数据，撞号最多多一次串行。
const (
	catalogFamilyMilestoneMapping int32 = 1
	catalogFamilyTriageRules      int32 = 2
	catalogFamilyDisclosurePolicy int32 = 3
)

// CatalogRegistrar 实现 ports.CatalogRegistry：VE 五类规则与策略目录的写入方。
//
// 在它之前，这七张目录表除测试外没有任何写入路径——五个只读装载口因此永远只能答
// `未配置`，`MAPPING_NOT_CONFIGURED`、`TRIAGE_RULES_NOT_CONFIGURED`、
// `NOTIFICATION_POLICY_NOT_CONFIGURED`、`ELIGIBILITY_CATALOGUE_NOT_CONFIGURED`
// 与披露空册那五处哨兵同根堵在这里。本类型只开门，不带任何默认条目：目录内容属实例
// 半边（`PAR-VIS-01`/`05`/`07`/`08`/`09` 全部待提供），种一行默认就是替租户作了它没
// 作的决定。
//
// 租户随每次调用到达，不在装配期固定——与本包那几个只读视图相反。视图是按租户绑好
// 交给编排的读口，而登记是管理动作，同一个进程入口可能替不同租户登记，把租户焊进
// 构造器只会逼调用方按租户造一堆写入方。
//
// 所有方法都在调用方的事务内执行（RequireExecutor）：一版抬头与它的整版条目必须同一
// 提交。半版目录比没有目录更坏——读口会把「条目还没写完」当成一次已作出的判断（未
// 归类、人工复核，乃至索赔`不予受理`那个永久格）。
type CatalogRegistrar struct {
	db *bentopg.DB
}

func NewCatalogRegistrar(db *bentopg.DB) (*CatalogRegistrar, error) {
	if db == nil {
		return nil, fmt.Errorf("visibility exception postgres: db is nil")
	}
	return &CatalogRegistrar{db: db}, nil
}

var _ ports.CatalogRegistry = (*CatalogRegistrar)(nil)

// RegisterMilestoneMapping 登记一版里程碑映射（`PAR-VIS-01`）。
//
// 条目按（源上下文+事实类型）建键——「标准里程碑映射按源上下文与事实类型版本化登记，
// 一行覆盖此后同类型事实，不得按单条事实引用建目录」（CONTEXT 硬句）。
func (registrar *CatalogRegistrar) RegisterMilestoneMapping(
	ctx context.Context,
	tenant domain.TenantID,
	registration ports.MilestoneMappingRegistration,
) (ports.CatalogRegistrationOutcome, error) {
	executor, err := registrar.db.RequireExecutor(ctx)
	if err != nil {
		return ports.CatalogRegistrationOutcomeInvalid, fmt.Errorf("register milestone mapping: %w", err)
	}
	outcome, err := registrar.admitVersion(ctx, executor, versionAdmission{
		family:     catalogFamilyMilestoneMapping,
		table:      "visibility_exception.milestone_mapping_version",
		versionCol: "mapping_version",
		tenant:     tenant,
		header:     registration.Header,
	})
	if outcome != ports.CatalogVersionRegistered || err != nil {
		return outcome, wrapRegister("register milestone mapping", err)
	}

	if _, err := executor.Exec(ctx,
		`INSERT INTO visibility_exception.milestone_mapping_version
			(tenant_id, mapping_version, effective_from, effective_to, approved_by)
		 VALUES ($1, $2, $3, $4, $5)`,
		tenant.String(), registration.Header.Version,
		registration.Header.EffectiveFrom.UTC(), effectiveTo(registration.Header),
		registration.Header.ApprovedBy,
	); err != nil {
		return ports.CatalogRegistrationOutcomeInvalid, fmt.Errorf("register milestone mapping: %w", err)
	}
	for _, entry := range registration.Entries {
		if _, err := executor.Exec(ctx,
			`INSERT INTO visibility_exception.milestone_mapping_entry
				(tenant_id, mapping_version, source_context, source_fact_kind, milestone_ref)
			 VALUES ($1, $2, $3, $4, $5)`,
			tenant.String(), registration.Header.Version,
			entry.Source.String(), entry.Kind.String(), entry.Milestone.String(),
		); err != nil {
			return ports.CatalogRegistrationOutcomeInvalid,
				fmt.Errorf("register milestone mapping: 条目 %s/%s：%w",
					entry.Source, entry.Kind, err)
		}
	}
	return ports.CatalogVersionRegistered, nil
}

// RegisterTriageRules 登记一版分诊规则（`PAR-VIS-05`）。条目含可信度维：四走向的分界
// 立在它上面（「高可信、高影响且命中版本化分诊规则的信号可以自动建立或关联案件」）。
func (registrar *CatalogRegistrar) RegisterTriageRules(
	ctx context.Context,
	tenant domain.TenantID,
	registration ports.TriageRuleRegistration,
) (ports.CatalogRegistrationOutcome, error) {
	executor, err := registrar.db.RequireExecutor(ctx)
	if err != nil {
		return ports.CatalogRegistrationOutcomeInvalid, fmt.Errorf("register triage rules: %w", err)
	}
	outcome, err := registrar.admitVersion(ctx, executor, versionAdmission{
		family:     catalogFamilyTriageRules,
		table:      "visibility_exception.triage_rule_version",
		versionCol: "rule_version",
		tenant:     tenant,
		header:     registration.Header,
	})
	if outcome != ports.CatalogVersionRegistered || err != nil {
		return outcome, wrapRegister("register triage rules", err)
	}

	if _, err := executor.Exec(ctx,
		`INSERT INTO visibility_exception.triage_rule_version
			(tenant_id, rule_version, effective_from, effective_to, approved_by)
		 VALUES ($1, $2, $3, $4, $5)`,
		tenant.String(), registration.Header.Version,
		registration.Header.EffectiveFrom.UTC(), effectiveTo(registration.Header),
		registration.Header.ApprovedBy,
	); err != nil {
		return ports.CatalogRegistrationOutcomeInvalid, fmt.Errorf("register triage rules: %w", err)
	}
	for _, entry := range registration.Entries {
		// 团队用指针而不是空串：0022 的成对约束正是靠 NULL 分辨「这一条不带团队」与
		// 「登记了一个空团队」，与披露条目内容列同一道理。
		if _, err := executor.Exec(ctx,
			`INSERT INTO visibility_exception.triage_rule_entry
				(tenant_id, rule_version, signal_kind, confidence_ref, outcome, responsible_team)
			 VALUES ($1, $2, $3, $4, $5, $6)`,
			tenant.String(), registration.Header.Version,
			entry.Kind.String(), entry.Confidence.String(), entry.Outcome.String(),
			nullIfBlank(entry.Team.String()),
		); err != nil {
			return ports.CatalogRegistrationOutcomeInvalid,
				fmt.Errorf("register triage rules: 条目 %s/%s：%w",
					entry.Kind, entry.Confidence, err)
		}
	}
	return ports.CatalogVersionRegistered, nil
}

// RegisterDisclosurePolicy 登记一版披露策略（`PAR-VIS-09`）。四维各自独立落状态与内容
// 来处：展示带内容、待确认与不展示不带，由 domain.ViewDimension 在构造期担保，库上的
// 四条 shape 约束是第二道网。
func (registrar *CatalogRegistrar) RegisterDisclosurePolicy(
	ctx context.Context,
	tenant domain.TenantID,
	registration ports.DisclosurePolicyRegistration,
) (ports.CatalogRegistrationOutcome, error) {
	executor, err := registrar.db.RequireExecutor(ctx)
	if err != nil {
		return ports.CatalogRegistrationOutcomeInvalid, fmt.Errorf("register disclosure policy: %w", err)
	}
	outcome, err := registrar.admitVersion(ctx, executor, versionAdmission{
		family:     catalogFamilyDisclosurePolicy,
		table:      "visibility_exception.disclosure_policy_version",
		versionCol: "policy_version",
		tenant:     tenant,
		header:     registration.Header,
	})
	if outcome != ports.CatalogVersionRegistered || err != nil {
		return outcome, wrapRegister("register disclosure policy", err)
	}

	if _, err := executor.Exec(ctx,
		`INSERT INTO visibility_exception.disclosure_policy_version
			(tenant_id, policy_version, effective_from, effective_to, approved_by)
		 VALUES ($1, $2, $3, $4, $5)`,
		tenant.String(), registration.Header.Version,
		registration.Header.EffectiveFrom.UTC(), effectiveTo(registration.Header),
		registration.Header.ApprovedBy,
	); err != nil {
		return ports.CatalogRegistrationOutcomeInvalid, fmt.Errorf("register disclosure policy: %w", err)
	}
	for _, entry := range registration.Entries {
		// 拆成（状态, 内容）两列复用读侧那支：内容用指针而不是空串，列上的 shape 约束
		// 正是靠 NULL 分辨「这一维不带引用」与「签发了一个空引用」。
		milestonesState, milestonesContent := dimensionColumnsOf(entry.Milestones)
		etaState, etaContent := dimensionColumnsOf(entry.ETA)
		finalState, finalContent := dimensionColumnsOf(entry.Final)
		noteState, noteContent := dimensionColumnsOf(entry.Note)
		if _, err := executor.Exec(ctx,
			`INSERT INTO visibility_exception.disclosure_policy_entry
				(tenant_id, policy_version, customer_account_ref,
				 milestones_state, eta_state, final_state, note_state,
				 milestones_content, eta_content, final_content, note_content)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
			tenant.String(), registration.Header.Version, entry.Customer.String(),
			milestonesState, etaState, finalState, noteState,
			milestonesContent, etaContent, finalContent, noteContent,
		); err != nil {
			return ports.CatalogRegistrationOutcomeInvalid,
				fmt.Errorf("register disclosure policy: 条目 %s：%w", entry.Customer, err)
		}
	}
	return ports.CatalogVersionRegistered, nil
}

// RegisterNotificationPolicy 登记一条通知策略（`PAR-VIS-07`）。
//
// 这份目录没有区间也没有版本表（0010）：键里的披露策略引用指名的就是版本化披露规则，
// 换版换的是引用值、新旧两行并存，因此不必防区间重叠，只需不覆盖既有行。撞键落
// `已登记`而不是覆盖——覆盖会让一条已经批准的渠道与时限悄悄换掉，而据它发出的历史
// 通知仍然指着这一行。
//
// 时限以微秒传入再由库折成 interval：Go 的 Duration 没有月与日的进位语义，而合同写的
// 「N 个月内」要按 Postgres 那套算（读口的截止时间正是库用 interval 加出来的）。
func (registrar *CatalogRegistrar) RegisterNotificationPolicy(
	ctx context.Context,
	tenant domain.TenantID,
	registration ports.NotificationPolicyRegistration,
) (ports.CatalogRegistrationOutcome, error) {
	executor, err := registrar.db.RequireExecutor(ctx)
	if err != nil {
		return ports.CatalogRegistrationOutcomeInvalid, fmt.Errorf("register notification policy: %w", err)
	}
	tag, err := executor.Exec(ctx,
		`INSERT INTO visibility_exception.notification_policy
			(tenant_id, disclosure_policy_ref, channel_ref, deadline_after,
			 obligation_ref, approved_by)
		 VALUES ($1, $2, $3, $4::bigint * interval '1 microsecond', $5, $6)
		 ON CONFLICT DO NOTHING`,
		tenant.String(), registration.Policy.String(), registration.Channel.String(),
		registration.DeadlineAfter.Microseconds(), registration.Obligation.String(),
		registration.ApprovedBy,
	)
	if err != nil {
		return ports.CatalogRegistrationOutcomeInvalid, fmt.Errorf("register notification policy: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.CatalogVersionAlreadyRegistered, nil
	}
	return ports.CatalogVersionRegistered, nil
}

// RegisterClaimEligibility 登记一份合同责任范围的索赔资格声明与它承担的索赔类型
// （`PAR-VIS-08` 的合同角）。
//
// 声明与覆盖类型同一提交：只有声明在场，「不在集合内」才说得通（0011）。分两步会出现
// 一段「声明已在、覆盖类型还没写」的窗口，那期间任一索赔都会被判成`不予受理`——ADR-0051
// 的永久格，审过不再审。
//
// 撞既有声明落`已登记`而不是覆盖：表以（租户+合同范围）为键、版本存在列上，结构上一
// 个范围只容一行，覆盖等于把一份已批准的声明连同它支撑过的永久判定一起换掉。
func (registrar *CatalogRegistrar) RegisterClaimEligibility(
	ctx context.Context,
	tenant domain.TenantID,
	registration ports.ClaimEligibilityRegistration,
) (ports.CatalogRegistrationOutcome, error) {
	executor, err := registrar.db.RequireExecutor(ctx)
	if err != nil {
		return ports.CatalogRegistrationOutcomeInvalid, fmt.Errorf("register claim eligibility: %w", err)
	}
	tag, err := executor.Exec(ctx,
		`INSERT INTO visibility_exception.claim_contract_scope
			(tenant_id, contract_scope_ref, rule_version, approved_by)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT DO NOTHING`,
		tenant.String(), registration.Contract.String(),
		registration.Header.Version, registration.Header.ApprovedBy,
	)
	if err != nil {
		return ports.CatalogRegistrationOutcomeInvalid, fmt.Errorf("register claim eligibility: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.CatalogVersionAlreadyRegistered, nil
	}
	for _, kind := range registration.CoveredKinds {
		if _, err := executor.Exec(ctx,
			`INSERT INTO visibility_exception.claim_covered_kind
				(tenant_id, contract_scope_ref, claim_kind_ref)
			 VALUES ($1, $2, $3)`,
			tenant.String(), registration.Contract.String(), kind.String(),
		); err != nil {
			return ports.CatalogRegistrationOutcomeInvalid,
				fmt.Errorf("register claim eligibility: 覆盖类型 %s：%w", kind, err)
		}
	}
	return ports.CatalogVersionRegistered, nil
}

// RegisterClaimAuthorization 登记一个货主客户账户的申请人授权名单（`PAR-VIS-08` 的
// 授权角）。目录行与名单同一提交，理由同索赔声明：只有目录在场，「不在名单里」才说
// 得通（0018）。空名单是正当登记——「目录在场、当前不授权任何人」，与「还没登记」不
// 是一回事，后者由目录行不在场表达。
func (registrar *CatalogRegistrar) RegisterClaimAuthorization(
	ctx context.Context,
	tenant domain.TenantID,
	registration ports.ClaimAuthorizationRegistration,
) (ports.CatalogRegistrationOutcome, error) {
	executor, err := registrar.db.RequireExecutor(ctx)
	if err != nil {
		return ports.CatalogRegistrationOutcomeInvalid, fmt.Errorf("register claim authorization: %w", err)
	}
	tag, err := executor.Exec(ctx,
		`INSERT INTO visibility_exception.claim_authorization_catalogue
			(tenant_id, customer_ref, rule_version, approved_by)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT DO NOTHING`,
		tenant.String(), registration.Customer.String(),
		registration.Header.Version, registration.Header.ApprovedBy,
	)
	if err != nil {
		return ports.CatalogRegistrationOutcomeInvalid, fmt.Errorf("register claim authorization: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.CatalogVersionAlreadyRegistered, nil
	}
	for _, applicant := range registration.Applicants {
		if _, err := executor.Exec(ctx,
			`INSERT INTO visibility_exception.claim_authorized_applicant
				(tenant_id, customer_ref, applicant_ref)
			 VALUES ($1, $2, $3)`,
			tenant.String(), registration.Customer.String(), applicant.String(),
		); err != nil {
			return ports.CatalogRegistrationOutcomeInvalid,
				fmt.Errorf("register claim authorization: 名单成员 %s：%w", applicant, err)
		}
	}
	return ports.CatalogVersionRegistered, nil
}

// versionAdmission 是三份区间型目录共用的准入输入。三张版本表的列同形（版本号、
// 区间、批准责任），准入判定也逐字相同，各写一份只会让「写入侧防重叠」这条规则在
// 三处各表达一次，改一处漏两处。
type versionAdmission struct {
	family     int32
	table      string
	versionCol string
	tenant     domain.TenantID
	header     ports.CatalogVersionHeader
}

// admitVersion 判一版能不能进，能进就把前一版接续闭合，**在此之前不改任何一行**。
//
// 次序要紧：先判后改。若先接续闭合再发现要拒，调用方一旦提交，就只剩一个被停用的
// 当前版本而没有接替者——目录从「有当前版本」静默变成「此刻无适用版本」，读口据此
// 作出的判断（未归类、人工复核）看起来还是有依据的。所以两道判定都排在唯一那次
// UPDATE 之前，拒绝时事务干净得像没来过。
//
// 判定与写入靠事务级咨询锁关在一起（先例：迁移执行器的 pg_advisory_lock）。锁按
// （租户+目录族）取，同族并发登记因此串行——没有它，两笔各自查得「不重叠」再各自
// 插入，重叠就这样进了库，而票 09 的红线正是「登记口须在写入侧防重叠，不靠读侧兜」。
// 锁不管绕过本写入方的直插；那种数据仍由读口的 ErrAmbiguousCatalog 兜住。
//
// 接续闭合照 ADR-0068：登记未闭新版时，同租户此前那个未闭版本按新版生效时间补上
// 终点，不动其余任何列。那不是改写历史，是「新版本自明确生效时间起参与新判断」在
// 旧版本区间上的镜像。登记已闭区间（补历史）不触发接续。
func (registrar *CatalogRegistrar) admitVersion(
	ctx context.Context,
	executor bentopg.Executor,
	admission versionAdmission,
) (ports.CatalogRegistrationOutcome, error) {
	if _, err := executor.Exec(ctx,
		`SELECT pg_advisory_xact_lock(hashtext($1), $2)`,
		admission.tenant.String(), admission.family,
	); err != nil {
		return ports.CatalogRegistrationOutcomeInvalid, fmt.Errorf("取目录族锁：%w", err)
	}

	var taken bool
	if err := executor.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM `+admission.table+
			` WHERE tenant_id = $1 AND `+admission.versionCol+` = $2)`,
		admission.tenant.String(), admission.header.Version,
	).Scan(&taken); err != nil {
		return ports.CatalogRegistrationOutcomeInvalid, fmt.Errorf("查版本号是否已占：%w", err)
	}
	if taken {
		return ports.CatalogVersionAlreadyRegistered, nil
	}

	closesPredecessor := !admission.header.HasEffectiveTo
	var overlaps bool
	// 重叠判据是半开区间相交：既有 [ef, et) 与新登 [from, to) 相交当且仅当
	// ef < to 且 et > from（未闭区间以 infinity 代入终点）。会被接续闭合的那个前版
	// 排除在外——它闭合后正好终止于新版起点，不再相交；不排除它，最常见的正当动作
	// 「发布新的当前版本」就永远登不进来。
	if err := executor.QueryRow(ctx,
		`SELECT EXISTS (
		            SELECT 1 FROM `+admission.table+`
		             WHERE tenant_id = $1
		               AND effective_from < COALESCE($3::timestamptz, 'infinity')
		               AND COALESCE(effective_to, 'infinity') > $2
		               AND NOT ($4 AND effective_to IS NULL AND effective_from < $2)
		        )`,
		admission.tenant.String(), admission.header.EffectiveFrom.UTC(),
		effectiveTo(admission.header), closesPredecessor,
	).Scan(&overlaps); err != nil {
		return ports.CatalogRegistrationOutcomeInvalid, fmt.Errorf("查区间重叠：%w", err)
	}
	if overlaps {
		return ports.CatalogVersionOverlapsExisting, nil
	}

	if closesPredecessor {
		if _, err := executor.Exec(ctx,
			`UPDATE `+admission.table+`
			    SET effective_to = $2
			  WHERE tenant_id = $1 AND effective_to IS NULL AND effective_from < $2`,
			admission.tenant.String(), admission.header.EffectiveFrom.UTC(),
		); err != nil {
			return ports.CatalogRegistrationOutcomeInvalid, fmt.Errorf("接续闭合前版本：%w", err)
		}
	}
	return ports.CatalogVersionRegistered, nil
}

func wrapRegister(operation string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", operation, err)
}

func effectiveTo(header ports.CatalogVersionHeader) *time.Time {
	if !header.HasEffectiveTo {
		return nil
	}
	utc := header.EffectiveTo.UTC()
	return &utc
}
