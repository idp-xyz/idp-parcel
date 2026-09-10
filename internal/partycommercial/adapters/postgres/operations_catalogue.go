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

// deliveryConditionColumns 是两条目录查询共用的交付条件一节:0030 父行的两条规则引用、所收紧的产品版本与声明时刻,
// 方式子表以相关子查询各聚各的(与 ListAcceptanceRulePackages 同一条理由:与别的子表并列 LEFT JOIN 会互相做笛卡尔积)。
// 父表按主键左连接到版本上、至多一行,不产生扇出;recipient_scope_rule_ref 在父表上 NOT NULL,它的在场即这一层的在场。
// 调用方在 FROM 里以别名 delivery 左连接 0030 父表。
const deliveryConditionColumns = `
		        delivery.recipient_scope_rule_ref, delivery.proof_of_delivery_rule_ref,
		        delivery.tightens_object_id, delivery.tightens_version_label, delivery.declared_at,
		        (SELECT COALESCE(json_agg(method.method_ref ORDER BY method.method_ref), '[]'::json)
		           FROM party_commercial.delivery_condition_method AS method
		          WHERE method.tenant_id     = delivery.tenant_id
		            AND method.object_kind   = delivery.object_kind
		            AND method.object_id     = delivery.object_id
		            AND method.version_label = delivery.version_label)`

const deliveryConditionJoin = `
		   LEFT JOIN party_commercial.delivery_condition AS delivery
		          ON delivery.tenant_id     = version.tenant_id
		         AND delivery.object_kind   = version.object_kind
		         AND delivery.object_id     = version.object_id
		         AND delivery.version_label = version.version_label`

// deliveryConditionScan 是与 deliveryConditionColumns 同序的扫描目标。
type deliveryConditionScan struct {
	recipientScope  *string
	proofOfDelivery *string
	tightensObject  *string
	tightensVersion *string
	declaredAt      *time.Time
	methodsJSON     []byte
}

func (scan *deliveryConditionScan) targets() []any {
	return []any{&scan.recipientScope, &scan.proofOfDelivery, &scan.tightensObject, &scan.tightensVersion, &scan.declaredAt, &scan.methodsJSON}
}

// row 把扫到的一节折成目录行上的可缺格:父行缺席答 nil(这一版没有交付条件),在场则方式集合与规则引用照字面转写。
// 父行在场而方式为空在内容读口是坏数据,目录上列如实交回空集合——上列不重建领域对象,拦坏数据仍归内容读口。
func (scan deliveryConditionScan) row() (*ports.DeliveryConditionCatalogueRow, error) {
	if scan.recipientScope == nil {
		return nil, nil
	}
	row := &ports.DeliveryConditionCatalogueRow{RecipientScopeRule: *scan.recipientScope}
	if scan.proofOfDelivery != nil {
		row.ProofOfDeliveryRule = *scan.proofOfDelivery
	}
	if scan.tightensObject != nil {
		row.TightensObjectID = *scan.tightensObject
	}
	if scan.tightensVersion != nil {
		row.TightensVersion = *scan.tightensVersion
	}
	if scan.declaredAt != nil {
		row.DeclaredAt = *scan.declaredAt
	}
	if err := json.Unmarshal(scan.methodsJSON, &row.Methods); err != nil {
		return nil, fmt.Errorf("delivery condition methods: %w", err)
	}
	if row.Methods == nil {
		row.Methods = []string{}
	}
	return row, nil
}

// ListServiceProducts 上列服务产品版本(壳)并左连接形态册与交付条件册(0030 产品层)。
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
		        product.form,`+deliveryConditionColumns+`
		   FROM party_commercial.commercial_version AS version
		   LEFT JOIN party_commercial.service_product_form AS product
		          ON product.tenant_id     = version.tenant_id
		         AND product.object_kind   = version.object_kind
		         AND product.object_id     = version.object_id
		         AND product.version_label = version.version_label`+deliveryConditionJoin+`
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
		var delivery deliveryConditionScan
		targets := append([]any{
			&row.ObjectID, &row.VersionLabel, &row.Scope, &status,
			&row.EffectiveStartsAt, &endsAt, &row.PublishedAt, &form,
		}, delivery.targets()...)
		if err := rows.Scan(targets...); err != nil {
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
		if row.DeliveryConditions, err = delivery.row(); err != nil {
			return nil, fmt.Errorf("list service products: %w", err)
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

// ListPricePolicies 上列商业价格政策册。发布期保全的方案方向与转换照列转写(ADR-0057)；口径册
// (0022)左连接挂在正文行上,HasCaliber 说明有没有——0010 早于 0022,只有正文没有口径的行是合法
// 状态。汇率三列半缺是库与领域分叉的坏数据,上抛不吸收。
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
		`SELECT price.object_id, price.version_label, price.direction, price.plan_ref, price.plan_direction,
		        price.binding_conversion, price.policy_scope_ref,
		        price.effective_starts_at, price.effective_ends_at, price.registered_at,
		        caliber.tax_disposition, caliber.tax_classification_ref, caliber.volumetric_factor_ref,
		        caliber.fx_quote_type_ref, caliber.fx_as_of_semantics_ref, caliber.fx_as_of_policy_version,
		        caliber.registered_at
		   FROM party_commercial.commercial_price_policy AS price
		   LEFT JOIN party_commercial.price_policy_caliber AS caliber
		          ON caliber.tenant_id     = price.tenant_id
		         AND caliber.object_kind   = price.object_kind
		         AND caliber.object_id     = price.object_id
		         AND caliber.version_label = price.version_label
		  WHERE price.tenant_id = $1
		  ORDER BY price.registered_at DESC, price.object_id, price.version_label
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
		var taxDisposition, taxClassification, volumetricFactor *string
		var fxQuoteType, fxAsOfSemantics, fxAsOfPolicyVersion *string
		var caliberRegisteredAt *time.Time
		if err := rows.Scan(
			&row.ObjectID, &row.VersionLabel, &row.Direction, &row.PlanRef, &row.PlanDirection,
			&row.BindingConversion, &row.PolicyScope,
			&row.EffectiveStartsAt, &endsAt, &row.RegisteredAt,
			&taxDisposition, &taxClassification, &volumetricFactor,
			&fxQuoteType, &fxAsOfSemantics, &fxAsOfPolicyVersion,
			&caliberRegisteredAt,
		); err != nil {
			return nil, fmt.Errorf("list price policies: %w", err)
		}
		if endsAt != nil {
			row.EffectiveEndsAt = *endsAt
			row.HasEffectiveEnd = true
		}
		if taxDisposition != nil {
			row.HasCaliber = true
			row.TaxDisposition = *taxDisposition
			row.TaxClassification = stringOrEmpty(taxClassification)
			row.VolumetricFactor = stringOrEmpty(volumetricFactor)
			if caliberRegisteredAt != nil {
				row.CaliberRegisteredAt = *caliberRegisteredAt
			}
			switch {
			case fxQuoteType != nil && fxAsOfSemantics != nil && fxAsOfPolicyVersion != nil:
				row.HasFx = true
				row.FxQuoteType = *fxQuoteType
				row.FxAsOfSemantics = *fxAsOfSemantics
				row.FxAsOfPolicyVersion = *fxAsOfPolicyVersion
			case fxQuoteType == nil && fxAsOfSemantics == nil && fxAsOfPolicyVersion == nil:
			default:
				return nil, fmt.Errorf("list price policies: %s/%s 的汇率口径半缺",
					row.ObjectID, row.VersionLabel)
			}
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

// ListCustomerContracts 上列客户合同版本(壳),左连接正文册、控制约定册与交付条件册(0030 合同层)。
//
// 四张表一条语句取回,不分两次:ReadExecutor 不保证两条语句同一快照,分次会拼出
// 从未同时存在的壳/正文/绑定组合(与 ListAcceptanceRulePackages 同一条理由)。
//
// 两级 LEFT JOIN 的第二级挂在**正文**上而不是壳上,这不是写法偏好:0012 的外键
// 就是绑定→正文,挂壳上会在正文缺席时把绑定行也带进来,而那种行库上根本不存在,
// 读出来只会是一份自相矛盾的证据。交付条件父表按主键挂在壳上(与正文各自可缺,是两层),
// 方式子表走相关子查询——与约定行并列 LEFT JOIN 会互相做笛卡尔积。
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
		        ),`+deliveryConditionColumns+`
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
		         AND binding.version_label = content.version_label`+deliveryConditionJoin+`
		  WHERE version.tenant_id   = $1
		    AND version.object_kind = $2
		  GROUP BY version.object_id, version.version_label, version.scope_ref, version.status,
		           version.effective_starts_at, version.effective_ends_at, version.published_at,
		           content.rule_package_id, content.declared_at,
		           delivery.tenant_id, delivery.object_kind, delivery.object_id, delivery.version_label,
		           delivery.recipient_scope_rule_ref, delivery.proof_of_delivery_rule_ref,
		           delivery.tightens_object_id, delivery.tightens_version_label, delivery.declared_at
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
		var delivery deliveryConditionScan
		targets := append([]any{
			&row.ObjectID, &row.VersionLabel, &row.Scope, &status,
			&row.EffectiveStartsAt, &endsAt, &row.PublishedAt,
			&rulePackage, &declaredAt, &bindingsJSON,
		}, delivery.targets()...)
		if err := rows.Scan(targets...); err != nil {
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
		if row.DeliveryConditions, err = delivery.row(); err != nil {
			return nil, fmt.Errorf("list customer contracts: %w", err)
		}
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

// ListAuthorizationRules 上列授权规则版本(壳),连同挂在它上面的取消授权目录。
//
// 目录壳与父同主键、至多一行,走 LEFT JOIN——「壳在不在」正需要连接后的 NULL 来判;
// 请求方声明走相关子查询,理由见 ListAcceptanceRulePackages 上那段。这里今天只有一族
// 子表,叠 LEFT JOIN + GROUP BY 也不会错,用子查询是为了第二族挂上来时不必重走一遍
// 那个坑。
func (catalogue *OperationsCatalogue) ListAuthorizationRules(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
) ([]ports.AuthorizationRuleRow, error) {
	if err := requirePositiveLimit("list authorization rules", limit); err != nil {
		return nil, err
	}
	querier, err := catalogue.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("list authorization rules: %w", err)
	}

	rows, err := querier.Query(ctx,
		`SELECT version.object_id, version.version_label, version.scope_ref, version.status,
		        version.effective_starts_at, version.effective_ends_at, version.published_at,
		        content.declared_at,
		        (SELECT COALESCE(
		                    json_agg(
		                        json_build_object(
		                            'party',     declaration.party,
		                            'reference', declaration.rule_reference
		                        )
		                        ORDER BY declaration.party
		                    ),
		                    '[]'::json
		                )
		           FROM party_commercial.cancellation_authority_declaration AS declaration
		          WHERE declaration.tenant_id     = version.tenant_id
		            AND declaration.object_kind   = version.object_kind
		            AND declaration.object_id     = version.object_id
		            AND declaration.version_label = version.version_label)
		   FROM party_commercial.commercial_version AS version
		   LEFT JOIN party_commercial.cancellation_authority_content AS content
		          ON content.tenant_id     = version.tenant_id
		         AND content.object_kind   = version.object_kind
		         AND content.object_id     = version.object_id
		         AND content.version_label = version.version_label
		  WHERE version.tenant_id   = $1
		    AND version.object_kind = $2
		  ORDER BY version.published_at DESC, version.object_id, version.version_label
		  LIMIT $3`,
		tenant.String(),
		uint8(domain.AuthorizationRuleObject),
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list authorization rules: %w", err)
	}
	defer rows.Close()

	ruleRows := make([]ports.AuthorizationRuleRow, 0, limit)
	for rows.Next() {
		var row ports.AuthorizationRuleRow
		var status int16
		var endsAt *time.Time
		var declaredAt *time.Time
		var authoritiesJSON []byte
		if err := rows.Scan(
			&row.ObjectID, &row.VersionLabel, &row.Scope, &status,
			&row.EffectiveStartsAt, &endsAt, &row.PublishedAt,
			&declaredAt, &authoritiesJSON,
		); err != nil {
			return nil, fmt.Errorf("list authorization rules: %w", err)
		}
		statusWord := domain.CommercialVersionStatus(status).String()
		if statusWord == "" {
			return nil, fmt.Errorf("list authorization rules: 版本状态 %d 不在封闭集内", status)
		}
		row.Status = statusWord
		if endsAt != nil {
			row.EffectiveEndsAt = *endsAt
			row.HasEffectiveEnd = true
		}
		// declared_at 在目录壳上 NOT NULL,它的在场即壳的在场。
		if declaredAt != nil {
			row.DeclaredAt = *declaredAt
			row.HasCancellationAuthority = true
		}
		if row.CancellationAuthorities, err = cancellationAuthorityRowsFromJSON(authoritiesJSON); err != nil {
			return nil, fmt.Errorf("list authorization rules: %w", err)
		}
		ruleRows = append(ruleRows, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list authorization rules: %w", err)
	}
	return ruleRows, nil
}

type cancellationAuthorityDocument struct {
	Party     string `json:"party"`
	Reference string `json:"reference"`
}

// cancellationAuthorityRowsFromJSON 只转写,不校验请求方是否在封闭二值内——判据同
// finalRuleRowsFromJSON。
func cancellationAuthorityRowsFromJSON(raw []byte) ([]ports.CancellationAuthorityRow, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var documents []cancellationAuthorityDocument
	if err := json.Unmarshal(raw, &documents); err != nil {
		return nil, fmt.Errorf("cancellation authorities are not this adapter's shape: %w", err)
	}
	authorities := make([]ports.CancellationAuthorityRow, 0, len(documents))
	for _, document := range documents {
		authorities = append(authorities, ports.CancellationAuthorityRow{
			Party:         document.Party,
			RuleReference: document.Reference,
		})
	}
	return authorities, nil
}

// ListSupplierAgreements 上列供应商协议版本壳,正文(0021)左连接。
//
// 装载方向与 ListCustomerContracts 同派:版本侧驱动,正文左连接——壳可先入册、正文随发布
// 登记,只列正文行会让未登正文的已发布协议从目录上消失。方向不在行上:领域恒为 BUY、库上
// 不成列。
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
		`SELECT version.object_id, version.version_label, version.scope_ref, version.status,
		        version.effective_starts_at, version.effective_ends_at, version.published_at,
		        content.supplier_party_id, content.legal_entity_ref, content.purchase_plan_ref,
		        content.agreement_scope_ref, content.effective_starts_at, content.effective_ends_at,
		        content.registered_at
		   FROM party_commercial.commercial_version AS version
		   LEFT JOIN party_commercial.supplier_agreement AS content
		          ON content.tenant_id     = version.tenant_id
		         AND content.object_kind   = version.object_kind
		         AND content.object_id     = version.object_id
		         AND content.version_label = version.version_label
		  WHERE version.tenant_id   = $1
		    AND version.object_kind = $2
		  ORDER BY version.published_at DESC, version.object_id, version.version_label
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
		var supplier, legalEntity, purchasePlan, agreementScope *string
		var agreementStartsAt, agreementEndsAt, registeredAt *time.Time
		if err := rows.Scan(
			&row.ObjectID, &row.VersionLabel, &row.Scope, &status,
			&row.EffectiveStartsAt, &endsAt, &row.PublishedAt,
			&supplier, &legalEntity, &purchasePlan, &agreementScope,
			&agreementStartsAt, &agreementEndsAt, &registeredAt,
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
		// supplier_party_id 在正文表上 NOT NULL,它的在场即正文行的在场;其余正文列同为
		// NOT NULL(除区间终点),到这里非空是表形保证的,缺一列即坏数据。
		if supplier != nil {
			if legalEntity == nil || purchasePlan == nil || agreementScope == nil ||
				agreementStartsAt == nil || registeredAt == nil {
				return nil, fmt.Errorf("list supplier agreements: %s/%s 的正文行不完整",
					row.ObjectID, row.VersionLabel)
			}
			row.HasContent = true
			row.Supplier = *supplier
			row.LegalEntity = *legalEntity
			row.PurchasePlan = *purchasePlan
			row.AgreementScope = *agreementScope
			row.AgreementEffectiveStartsAt = *agreementStartsAt
			row.RegisteredAt = *registeredAt
			if agreementEndsAt != nil {
				row.AgreementEffectiveEndsAt = *agreementEndsAt
				row.HasAgreementEffectiveEnd = true
			}
		}
		agreementRows = append(agreementRows, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list supplier agreements: %w", err)
	}
	return agreementRows, nil
}
