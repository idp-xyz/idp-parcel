package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// OperationsCatalogue 实现两个主数据目录查阅读端口(ADR-0077):服务产品目录与商业
// 策略目录。只读——目录上列不形成判断、决定或披露,所以这里只有 SELECT,没有任何
// Save;登记仍走 CommercialPublications 与 cmd/parcel-commercial 的受控通道。
//
// 每条语句显式携带租户条件(ADR-0003/0040:作用域是身份的一部分,不是过滤器)。
// 排序都以登记/声明/发布时间倒序、同刻按对象与版本号正序收尾,保证分页可重复。
type OperationsCatalogue struct {
	db *bentopg.DB
}

func NewOperationsCatalogue(db *bentopg.DB) (*OperationsCatalogue, error) {
	if db == nil {
		return nil, fmt.Errorf("party commercial postgres: db is nil")
	}
	return &OperationsCatalogue{db: db}, nil
}

var _ ports.ServiceProductCatalogueRead = (*OperationsCatalogue)(nil)
var _ ports.CommercialPolicyCatalogueRead = (*OperationsCatalogue)(nil)
var _ ports.CommercialRelationCatalogueRead = (*OperationsCatalogue)(nil)

// requirePositiveLimit 把「忘了传页大小」挡在读口上:静默答一页会把缺参变成一个
// 没人决定过的页大小(ADR-0077 Decision 五,判据与运营追踪读口同款)。
func requirePositiveLimit(operation string, limit int) error {
	if limit < 1 {
		return fmt.Errorf("%s: limit must be positive, got %d", operation, limit)
	}
	return nil
}

// ListServiceProducts 上列服务产品版本(壳)并左连接形态册。
//
// 上列对象是版本壳而不是形态行,理由在 ports.ServiceProductCatalogueRow 上;装载
// 方向与 LoadForScope 同派(0008 迁移自注:装载由 commercial_version 侧驱动)。
// 版本册只收已发布之后的状态,读回集外取值即坏数据,上抛不折成空。
func (catalogue *OperationsCatalogue) ListServiceProducts(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
) ([]ports.ServiceProductCatalogueRow, error) {
	if err := requirePositiveLimit("list service products", limit); err != nil {
		return nil, err
	}
	querier, err := catalogue.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("list service products: %w", err)
	}

	rows, err := querier.Query(ctx,
		`SELECT version.object_id, version.version_label, version.scope_ref, version.status,
		        version.effective_starts_at, version.effective_ends_at, version.published_at,
		        product.form
		   FROM party_commercial.commercial_version AS version
		   LEFT JOIN party_commercial.service_product_form AS product
		          ON product.tenant_id     = version.tenant_id
		         AND product.object_kind   = version.object_kind
		         AND product.object_id     = version.object_id
		         AND product.version_label = version.version_label
		  WHERE version.tenant_id   = $1
		    AND version.object_kind = $2
		  ORDER BY version.published_at DESC, version.object_id, version.version_label
		  LIMIT $3`,
		tenant.String(),
		uint8(domain.ServiceProductObject),
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list service products: %w", err)
	}
	defer rows.Close()

	catalogueRows := make([]ports.ServiceProductCatalogueRow, 0, limit)
	for rows.Next() {
		var row ports.ServiceProductCatalogueRow
		var status int16
		var endsAt *time.Time
		var form *string
		if err := rows.Scan(
			&row.ObjectID, &row.VersionLabel, &row.Scope, &status,
			&row.EffectiveStartsAt, &endsAt, &row.PublishedAt, &form,
		); err != nil {
			return nil, fmt.Errorf("list service products: %w", err)
		}
		statusWord := domain.CommercialVersionStatus(status).String()
		if statusWord == "" {
			return nil, fmt.Errorf("list service products: 版本状态 %d 不在封闭集内", status)
		}
		row.Status = statusWord
		if endsAt != nil {
			row.EffectiveEndsAt = *endsAt
			row.HasEffectiveEnd = true
		}
		if form != nil {
			row.Form = *form
			row.HasForm = true
		}
		catalogueRows = append(catalogueRows, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list service products: %w", err)
	}
	return catalogueRows, nil
}

// ListAcceptanceRulePackages 上列接单规则包正文册,连同挂在同一份规则包上的两族阶段
// 内容声明(0013 的收寄资格与终局规则)。一条语句取回全部父子——ReadExecutor 不保证
// 两条语句同一快照,分次取会拼出从未同时存在的父子状态(与 LoadAcceptanceRulePackage
// 同一条理由)。有父零子在内容读口是坏数据,在目录上列如实交回空集合:上列不重建领域
// 对象、不形成判断,拦坏数据仍归内容读口。
//
// **三族子表用相关子查询各聚各的,不再叠 LEFT JOIN + GROUP BY。** 多族子表同时以
// LEFT JOIN 挂上来会互相做笛卡尔积——规则 3 条 × 允许来源 2 条会摊成 6 行,`json_agg`
// 于是把每族都数重。本函数原先只有一族,那个形状看不出问题;第二族一挂就出。两个
// 声明壳仍走 LEFT JOIN:它们与父同主键、至多一行,不产生扇出,而「壳在不在」正需要
// 连接后的 NULL 来判。
func (catalogue *OperationsCatalogue) ListAcceptanceRulePackages(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
) ([]ports.AcceptanceRulePackageRow, error) {
	if err := requirePositiveLimit("list acceptance rule packages", limit); err != nil {
		return nil, err
	}
	querier, err := catalogue.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("list acceptance rule packages: %w", err)
	}

	rows, err := querier.Query(ctx,
		`SELECT parent.object_id, parent.version_label,
		        parent.service_product_id, parent.contract_id, parent.legal_entity_ref,
		        parent.scope_ref, parent.effective_starts_at, parent.effective_ends_at,
		        parent.declared_at,
		        (SELECT COALESCE(
		                    json_agg(
		                        json_build_object(
		                            'category',  rule.rule_category,
		                            'reference', rule.rule_reference
		                        )
		                        ORDER BY rule.rule_category, rule.rule_reference
		                    ),
		                    '[]'::json
		                )
		           FROM party_commercial.acceptance_rule_package_rule AS rule
		          WHERE rule.tenant_id     = parent.tenant_id
		            AND rule.object_kind   = parent.object_kind
		            AND rule.object_id     = parent.object_id
		            AND rule.version_label = parent.version_label),
		        intake.object_id IS NOT NULL,
		        (SELECT COALESCE(json_agg(source.source_kind ORDER BY source.source_kind), '[]'::json)
		           FROM party_commercial.intake_allowed_source AS source
		          WHERE source.tenant_id     = parent.tenant_id
		            AND source.object_kind   = parent.object_kind
		            AND source.object_id     = parent.object_id
		            AND source.version_label = parent.version_label),
		        (SELECT COALESCE(json_agg(ref.rule_reference ORDER BY ref.rule_reference), '[]'::json)
		           FROM party_commercial.intake_qualification_ref AS ref
		          WHERE ref.tenant_id     = parent.tenant_id
		            AND ref.object_kind   = parent.object_kind
		            AND ref.object_id     = parent.object_id
		            AND ref.version_label = parent.version_label),
		        finals.object_id IS NOT NULL,
		        (SELECT COALESCE(
		                    json_agg(
		                        json_build_object(
		                            'outcome',   final.outcome,
		                            'finalKind', final.final_kind
		                        )
		                        ORDER BY final.outcome
		                    ),
		                    '[]'::json
		                )
		           FROM party_commercial.final_rule_declaration AS final
		          WHERE final.tenant_id     = parent.tenant_id
		            AND final.object_kind   = parent.object_kind
		            AND final.object_id     = parent.object_id
		            AND final.version_label = parent.version_label)
		   FROM party_commercial.acceptance_rule_package AS parent
		   LEFT JOIN party_commercial.intake_qualification_content AS intake
		          ON intake.tenant_id     = parent.tenant_id
		         AND intake.object_kind   = parent.object_kind
		         AND intake.object_id     = parent.object_id
		         AND intake.version_label = parent.version_label
		   LEFT JOIN party_commercial.final_rule_content AS finals
		          ON finals.tenant_id     = parent.tenant_id
		         AND finals.object_kind   = parent.object_kind
		         AND finals.object_id     = parent.object_id
		         AND finals.version_label = parent.version_label
		  WHERE parent.tenant_id = $1
		  ORDER BY parent.declared_at DESC, parent.object_id, parent.version_label
		  LIMIT $2`,
		tenant.String(),
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list acceptance rule packages: %w", err)
	}
	defer rows.Close()

	packageRows := make([]ports.AcceptanceRulePackageRow, 0, limit)
	for rows.Next() {
		var row ports.AcceptanceRulePackageRow
		var endsAt *time.Time
		var rulesJSON, sourcesJSON, refsJSON, finalsJSON []byte
		if err := rows.Scan(
			&row.ObjectID, &row.VersionLabel,
			&row.ServiceProduct, &row.Contract, &row.LegalEntity,
			&row.Scope, &row.EffectiveStartsAt, &endsAt, &row.DeclaredAt, &rulesJSON,
			&row.HasIntakeQualification, &sourcesJSON, &refsJSON,
			&row.HasFinalRules, &finalsJSON,
		); err != nil {
			return nil, fmt.Errorf("list acceptance rule packages: %w", err)
		}
		if endsAt != nil {
			row.EffectiveEndsAt = *endsAt
			row.HasEffectiveEnd = true
		}
		rules, err := assembledRuleRowsFromJSON(rulesJSON)
		if err != nil {
			return nil, fmt.Errorf("list acceptance rule packages: %w", err)
		}
		row.Rules = rules
		if row.AllowedIntakeSources, err = stringsFromJSON(sourcesJSON); err != nil {
			return nil, fmt.Errorf("list acceptance rule packages: allowed intake sources: %w", err)
		}
		if row.IntakeQualificationRefs, err = stringsFromJSON(refsJSON); err != nil {
			return nil, fmt.Errorf("list acceptance rule packages: intake qualification refs: %w", err)
		}
		if row.FinalRules, err = finalRuleRowsFromJSON(finalsJSON); err != nil {
			return nil, fmt.Errorf("list acceptance rule packages: %w", err)
		}
		packageRows = append(packageRows, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list acceptance rule packages: %w", err)
	}
	return packageRows, nil
}

func stringsFromJSON(raw []byte) ([]string, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var values []string
	if err := json.Unmarshal(raw, &values); err != nil {
		return nil, fmt.Errorf("not a string array: %w", err)
	}
	return values, nil
}

type finalRuleDocument struct {
	Outcome   string `json:"outcome"`
	FinalKind string `json:"finalKind"`
}

// finalRuleRowsFromJSON 只转写,不校验责任结果是否在封闭四值内——库上 CHECK 已经把
// 集外取值挡在写侧,这里再判一次会把「库被绕过写脏」与「本适配器读错列」两件事说成
// 同一句话。集外值随行透出,页面词表照 labelOf 的规矩回落显示原码。
func finalRuleRowsFromJSON(raw []byte) ([]ports.FinalRuleRow, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var documents []finalRuleDocument
	if err := json.Unmarshal(raw, &documents); err != nil {
		return nil, fmt.Errorf("final rules are not this adapter's shape: %w", err)
	}
	finals := make([]ports.FinalRuleRow, 0, len(documents))
	for _, document := range documents {
		finals = append(finals, ports.FinalRuleRow{
			Outcome:   document.Outcome,
			FinalKind: document.FinalKind,
		})
	}
	return finals, nil
}

func assembledRuleRowsFromJSON(raw []byte) ([]ports.AssembledRuleRow, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var documents []assembledRuleDocument
	if err := json.Unmarshal(raw, &documents); err != nil {
		return nil, fmt.Errorf("assembled rules are not this adapter's shape: %w", err)
	}
	rules := make([]ports.AssembledRuleRow, 0, len(documents))
	for _, document := range documents {
		rules = append(rules, ports.AssembledRuleRow{
			Category:  document.Category,
			Reference: document.Reference,
		})
	}
	return rules, nil
}

// ListPreAcceptanceControls 上列接受前财务控制声明册。拥有对象是客户合同版本
// (object_kind CHECK 钉在 2),行内标识因此指名合同。
func (catalogue *OperationsCatalogue) ListPreAcceptanceControls(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
) ([]ports.PreAcceptanceControlRow, error) {
	if err := requirePositiveLimit("list pre-acceptance controls", limit); err != nil {
		return nil, err
	}
	querier, err := catalogue.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("list pre-acceptance controls: %w", err)
	}

	rows, err := querier.Query(ctx,
		`SELECT object_id, version_label, requirement, not_applicable_basis, declared_at
		   FROM party_commercial.pre_acceptance_control_declaration
		  WHERE tenant_id = $1
		  ORDER BY declared_at DESC, object_id, version_label
		  LIMIT $2`,
		tenant.String(),
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list pre-acceptance controls: %w", err)
	}
	defer rows.Close()

	controlRows := make([]ports.PreAcceptanceControlRow, 0, limit)
	for rows.Next() {
		var row ports.PreAcceptanceControlRow
		var basis *string
		if err := rows.Scan(
			&row.ContractObjectID, &row.ContractVersion, &row.Requirement, &basis, &row.DeclaredAt,
		); err != nil {
			return nil, fmt.Errorf("list pre-acceptance controls: %w", err)
		}
		if basis != nil {
			row.NotApplicableBasis = *basis
		}
		controlRows = append(controlRows, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list pre-acceptance controls: %w", err)
	}
	return controlRows, nil
}

// ListPricePolicies 上列商业价格政策册。发布期保全的方案方向与转换照列转写(ADR-0057)。
func (catalogue *OperationsCatalogue) ListPricePolicies(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
) ([]ports.PricePolicyRow, error) {
	if err := requirePositiveLimit("list price policies", limit); err != nil {
		return nil, err
	}
	querier, err := catalogue.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("list price policies: %w", err)
	}

	rows, err := querier.Query(ctx,
		`SELECT object_id, version_label, direction, plan_ref, plan_direction,
		        binding_conversion, policy_scope_ref,
		        effective_starts_at, effective_ends_at, registered_at
		   FROM party_commercial.commercial_price_policy
		  WHERE tenant_id = $1
		  ORDER BY registered_at DESC, object_id, version_label
		  LIMIT $2`,
		tenant.String(),
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list price policies: %w", err)
	}
	defer rows.Close()

	policyRows := make([]ports.PricePolicyRow, 0, limit)
	for rows.Next() {
		var row ports.PricePolicyRow
		var endsAt *time.Time
		if err := rows.Scan(
			&row.ObjectID, &row.VersionLabel, &row.Direction, &row.PlanRef, &row.PlanDirection,
			&row.BindingConversion, &row.PolicyScope,
			&row.EffectiveStartsAt, &endsAt, &row.RegisteredAt,
		); err != nil {
			return nil, fmt.Errorf("list price policies: %w", err)
		}
		if endsAt != nil {
			row.EffectiveEndsAt = *endsAt
			row.HasEffectiveEnd = true
		}
		policyRows = append(policyRows, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list price policies: %w", err)
	}
	return policyRows, nil
}

// ListSettlementPolicies 上列结算政策册:方式与六维适用范围平铺(ADR-0044)。
func (catalogue *OperationsCatalogue) ListSettlementPolicies(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
) ([]ports.SettlementPolicyRow, error) {
	if err := requirePositiveLimit("list settlement policies", limit); err != nil {
		return nil, err
	}
	querier, err := catalogue.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("list settlement policies: %w", err)
	}

	rows, err := querier.Query(ctx,
		`SELECT object_id, version_label, method, legal_entity_ref, counterparty_ref,
		        contract_label, charge_scope_ref, currency_code,
		        effective_starts_at, effective_ends_at, registered_at
		   FROM party_commercial.commercial_settlement_policy
		  WHERE tenant_id = $1
		  ORDER BY registered_at DESC, object_id, version_label
		  LIMIT $2`,
		tenant.String(),
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list settlement policies: %w", err)
	}
	defer rows.Close()

	policyRows := make([]ports.SettlementPolicyRow, 0, limit)
	for rows.Next() {
		var row ports.SettlementPolicyRow
		var endsAt *time.Time
		if err := rows.Scan(
			&row.ObjectID, &row.VersionLabel, &row.Method, &row.LegalEntity, &row.Counterparty,
			&row.ContractLabel, &row.ChargeScope, &row.Currency,
			&row.EffectiveStartsAt, &endsAt, &row.RegisteredAt,
		); err != nil {
			return nil, fmt.Errorf("list settlement policies: %w", err)
		}
		if endsAt != nil {
			row.EffectiveEndsAt = *endsAt
			row.HasEffectiveEnd = true
		}
		policyRows = append(policyRows, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list settlement policies: %w", err)
	}
	return policyRows, nil
}

// ListAsOfPolicyDeclarations 上列时点锚声明册:某规则包为某类下游判断声明的时点
// 语义与政策版本。表不存时点值,这里也就没有时点值可列。
func (catalogue *OperationsCatalogue) ListAsOfPolicyDeclarations(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
) ([]ports.AsOfPolicyRow, error) {
	if err := requirePositiveLimit("list as-of policy declarations", limit); err != nil {
		return nil, err
	}
	querier, err := catalogue.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("list as-of policy declarations: %w", err)
	}

	rows, err := querier.Query(ctx,
		`SELECT object_id, version_label, judgment_type, semantics_ref, policy_version, declared_at
		   FROM party_commercial.as_of_policy_declaration
		  WHERE tenant_id = $1
		  ORDER BY declared_at DESC, object_id, version_label, judgment_type
		  LIMIT $2`,
		tenant.String(),
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list as-of policy declarations: %w", err)
	}
	defer rows.Close()

	declarationRows := make([]ports.AsOfPolicyRow, 0, limit)
	for rows.Next() {
		var row ports.AsOfPolicyRow
		if err := rows.Scan(
			&row.RulePackageObjectID, &row.RulePackageVersion,
			&row.JudgmentType, &row.SemanticsRef, &row.PolicyVersion, &row.DeclaredAt,
		); err != nil {
			return nil, fmt.Errorf("list as-of policy declarations: %w", err)
		}
		declarationRows = append(declarationRows, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list as-of policy declarations: %w", err)
	}
	return declarationRows, nil
}

// ListCustomerContracts 上列客户合同版本(壳),左连接正文册与控制约定册。
//
// 三张表一条语句取回,不分两次:ReadExecutor 不保证两条语句同一快照,分次会拼出
// 从未同时存在的壳/正文/绑定组合(与 ListAcceptanceRulePackages 同一条理由)。
//
// 两级 LEFT JOIN 的第二级挂在**正文**上而不是壳上,这不是写法偏好:0012 的外键
// 就是绑定→正文,挂壳上会在正文缺席时把绑定行也带进来,而那种行库上根本不存在,
// 读出来只会是一份自相矛盾的证据。
func (catalogue *OperationsCatalogue) ListCustomerContracts(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
) ([]ports.CustomerContractCatalogueRow, error) {
	if err := requirePositiveLimit("list customer contracts", limit); err != nil {
		return nil, err
	}
	querier, err := catalogue.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("list customer contracts: %w", err)
	}

	rows, err := querier.Query(ctx,
		`SELECT version.object_id, version.version_label, version.scope_ref, version.status,
		        version.effective_starts_at, version.effective_ends_at, version.published_at,
		        content.rule_package_id, content.declared_at,
		        COALESCE(
		            json_agg(
		                json_build_object(
		                    'chargeScope', binding.charge_scope_ref,
		                    'policyId',    binding.policy_id,
		                    'basis',       binding.inapplicability_basis
		                )
		                ORDER BY binding.charge_scope_ref
		            ) FILTER (WHERE binding.charge_scope_ref IS NOT NULL),
		            '[]'::json
		        )
		   FROM party_commercial.commercial_version AS version
		   LEFT JOIN party_commercial.customer_contract_content AS content
		          ON content.tenant_id     = version.tenant_id
		         AND content.object_kind   = version.object_kind
		         AND content.object_id     = version.object_id
		         AND content.version_label = version.version_label
		   LEFT JOIN party_commercial.customer_contract_control_binding AS binding
		          ON binding.tenant_id     = content.tenant_id
		         AND binding.object_kind   = content.object_kind
		         AND binding.object_id     = content.object_id
		         AND binding.version_label = content.version_label
		  WHERE version.tenant_id   = $1
		    AND version.object_kind = $2
		  GROUP BY version.object_id, version.version_label, version.scope_ref, version.status,
		           version.effective_starts_at, version.effective_ends_at, version.published_at,
		           content.rule_package_id, content.declared_at
		  ORDER BY version.published_at DESC, version.object_id, version.version_label
		  LIMIT $3`,
		tenant.String(),
		uint8(domain.CustomerContractObject),
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list customer contracts: %w", err)
	}
	defer rows.Close()

	contractRows := make([]ports.CustomerContractCatalogueRow, 0, limit)
	for rows.Next() {
		var row ports.CustomerContractCatalogueRow
		var status int16
		var endsAt *time.Time
		var rulePackage *string
		var declaredAt *time.Time
		var bindingsJSON []byte
		if err := rows.Scan(
			&row.ObjectID, &row.VersionLabel, &row.Scope, &status,
			&row.EffectiveStartsAt, &endsAt, &row.PublishedAt,
			&rulePackage, &declaredAt, &bindingsJSON,
		); err != nil {
			return nil, fmt.Errorf("list customer contracts: %w", err)
		}
		statusWord := domain.CommercialVersionStatus(status).String()
		if statusWord == "" {
			return nil, fmt.Errorf("list customer contracts: 版本状态 %d 不在封闭集内", status)
		}
		row.Status = statusWord
		if endsAt != nil {
			row.EffectiveEndsAt = *endsAt
			row.HasEffectiveEnd = true
		}
		// rule_package_id 在正文表上 NOT NULL,它的在场即正文行的在场——不必再多取
		// 一列去问「有没有正文」。
		if rulePackage != nil {
			row.RulePackageID = *rulePackage
			row.HasContent = true
		}
		if declaredAt != nil {
			row.DeclaredAt = *declaredAt
		}
		bindings, err := controlBindingRowsFromJSON(bindingsJSON)
		if err != nil {
			return nil, fmt.Errorf("list customer contracts: %w", err)
		}
		row.Bindings = bindings
		contractRows = append(contractRows, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list customer contracts: %w", err)
	}
	return contractRows, nil
}

type controlBindingDocument struct {
	ChargeScope string  `json:"chargeScope"`
	PolicyID    *string `json:"policyId"`
	Basis       *string `json:"basis"`
}

// controlBindingRowsFromJSON 把聚合出来的绑定数组转写成上列行。
//
// 两列同空要上抛而不是折成一行空约定:0012 的 CHECK 保证指名策略与显式不适用恰有
// 一个在场,读回两空说明库上那条约束没生效过——把它当成「没约定」会让一次结构性
// 损坏看起来像一份如实的记录。
func controlBindingRowsFromJSON(raw []byte) ([]ports.ControlBindingRow, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var documents []controlBindingDocument
	if err := json.Unmarshal(raw, &documents); err != nil {
		return nil, fmt.Errorf("control bindings are not this adapter's shape: %w", err)
	}
	bindings := make([]ports.ControlBindingRow, 0, len(documents))
	for _, document := range documents {
		if (document.PolicyID == nil) == (document.Basis == nil) {
			return nil, fmt.Errorf(
				"control binding %q: 指名策略与显式不适用恰有一个在场,读回的这行两者%s",
				document.ChargeScope,
				bothOrNeither(document.PolicyID == nil),
			)
		}
		binding := ports.ControlBindingRow{ChargeScope: document.ChargeScope}
		if document.PolicyID != nil {
			binding.PolicyID = *document.PolicyID
		}
		if document.Basis != nil {
			binding.InapplicabilityBasis = *document.Basis
		}
		bindings = append(bindings, binding)
	}
	return bindings, nil
}

func bothOrNeither(neither bool) string {
	if neither {
		return "皆无"
	}
	return "皆有"
}

// ListSupplierAgreements 上列供应商协议版本(壳)。
//
// 只有壳可列——供应商、采购定价方案与方向在领域的 SupplierAgreement 上,但没有正文
// 表可读(与信用政策同形,见 ports.SupplierAgreementCatalogueRow)。这里不左连接任何
// 东西,不是漏了。
func (catalogue *OperationsCatalogue) ListSupplierAgreements(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
) ([]ports.SupplierAgreementCatalogueRow, error) {
	if err := requirePositiveLimit("list supplier agreements", limit); err != nil {
		return nil, err
	}
	querier, err := catalogue.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("list supplier agreements: %w", err)
	}

	rows, err := querier.Query(ctx,
		`SELECT object_id, version_label, scope_ref, status,
		        effective_starts_at, effective_ends_at, published_at
		   FROM party_commercial.commercial_version
		  WHERE tenant_id   = $1
		    AND object_kind = $2
		  ORDER BY published_at DESC, object_id, version_label
		  LIMIT $3`,
		tenant.String(),
		uint8(domain.SupplierAgreementObject),
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list supplier agreements: %w", err)
	}
	defer rows.Close()

	agreementRows := make([]ports.SupplierAgreementCatalogueRow, 0, limit)
	for rows.Next() {
		var row ports.SupplierAgreementCatalogueRow
		var status int16
		var endsAt *time.Time
		if err := rows.Scan(
			&row.ObjectID, &row.VersionLabel, &row.Scope, &status,
			&row.EffectiveStartsAt, &endsAt, &row.PublishedAt,
		); err != nil {
			return nil, fmt.Errorf("list supplier agreements: %w", err)
		}
		statusWord := domain.CommercialVersionStatus(status).String()
		if statusWord == "" {
			return nil, fmt.Errorf("list supplier agreements: 版本状态 %d 不在封闭集内", status)
		}
		row.Status = statusWord
		if endsAt != nil {
			row.EffectiveEndsAt = *endsAt
			row.HasEffectiveEnd = true
		}
		agreementRows = append(agreementRows, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list supplier agreements: %w", err)
	}
	return agreementRows, nil
}
