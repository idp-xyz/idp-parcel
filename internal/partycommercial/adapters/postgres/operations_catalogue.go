package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
)

// OperationsCatalogue 实现两个主数据目录查阅读端口(ADR-0077):服务产品目录与商业
// 策略目录。只读——目录上列不形成判断、决定或披露,所以这里只有 SELECT,没有任何
// Save;登记仍走 CommercialPublications 与 cmd/parcel-commercial 的受控通道。
//
// 每条语句显式携带租户条件(ADR-0003/0040:作用域是身份的一部分,不是过滤器)。
// 各册的排序、筛选与 q 声明在 ports 的 …Pages 上(ADR-0144),同值按行标识与排序同向收尾,游标翻页不重不漏。
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

// versionKey 是按身份读回（ADR-0361）比列表多出来的两格键过滤：对象与版本。租户与类别已在 listed 的 WHERE 上，
// 合起来正是 commercial_version 的主键——范围不是键，所以这里没有范围。
//
// 按身份读回不另写语句：与取页同一段 listed、同一段扫描，只差这两格。另写一条就会长出第二套行转写，列表上那一行与
// 读回的那一行迟早在某个格上不同，而管理台的对象页正是拿读回的那份当「目录上这一版」来显示的。
type versionKey struct {
	objectID string
	version  string
}

func keyOf(objectID domain.CommercialObjectID, version domain.CommercialVersionLabel) *versionKey {
	return &versionKey{objectID: objectID.String(), version: version.String()}
}

// filter 交出接在 listed 外层 WHERE 末尾的两格键过滤与追加了键值的参数，占位符号接着 listed 已占用的参数续编。
func (key *versionKey) filter(args []any) (string, []any) {
	next := len(args) + 1
	clause := fmt.Sprintf(" AND object_id = $%d AND version_label = $%d", next, next+1)
	return clause, append(args, key.objectID, key.version)
}

// firstRow 把按键取回的至多一行折成「在不在」：键是主键，取回多于一行是语句写错，取不回是不在册——
// 跨租户与不在册同形，读口不区分。
func firstRow[Row any](rows []Row, err error) (Row, bool, error) {
	var none Row
	if err != nil {
		return none, false, err
	}
	if len(rows) == 0 {
		return none, false, nil
	}
	return rows[0], true, nil
}

// deliveryConditionColumns 是两条目录查询共用的交付条件一节:0030 父行的两条规则引用、所收紧的产品版本与声明时刻,
// 方式子表以相关子查询各聚各的(与 ListAcceptanceRulePackages 同一条理由:与别的子表并列 LEFT JOIN 会互相做笛卡尔积)。
// 父表按主键左连接到版本上、至多一行,不产生扇出;recipient_scope_rule_ref 在父表上 NOT NULL,它的在场即这一层的在场。
// 调用方在 FROM 里以别名 delivery 左连接 0030 父表。
// 方式按 COLLATE "C" 排:领域 declareDeliveryConditions 用 Go 字节序排方式,目录只转写不重建,序要与
// DeliveryConditionContent.Methods() 逐字同;方式引用是开放引用,大小写混排或含非 ASCII 时语言排序(en_US 之类)与字节序
// 不同,不钉 collation 就成了「部署库的 locale 决定目录序」(票 admin-write-faces/26 ①)。
const deliveryConditionColumns = `
		        delivery.recipient_scope_rule_ref, delivery.proof_of_delivery_rule_ref,
		        delivery.tightens_object_id, delivery.tightens_version_label, delivery.declared_at AS delivery_declared_at,
		        (SELECT COALESCE(json_agg(method.method_ref ORDER BY method.method_ref COLLATE "C"), '[]'::json)
		           FROM party_commercial.delivery_condition_method AS method
		          WHERE method.tenant_id     = delivery.tenant_id
		            AND method.object_kind   = delivery.object_kind
		            AND method.object_id     = delivery.object_id
		            AND method.version_label = delivery.version_label) AS delivery_methods`

// deliveryConditionSelection 是 deliveryConditionColumns 在 listed 里导出的列名，次序同 deliveryConditionScan.targets。
const deliveryConditionSelection = `recipient_scope_rule_ref, proof_of_delivery_rule_ref,
		        tightens_object_id, tightens_version_label, delivery_declared_at, delivery_methods`

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

// serviceProductListed 上列服务产品版本(壳)并左连接形态册与交付条件册(0030 产品层)。
//
// 上列对象是版本壳而不是形态行,理由在 ports.ServiceProductCatalogueRow 上;装载
// 方向与 LoadForScope 同派(0008 迁移自注:装载由 commercial_version 侧驱动)。
// 版本册只收已发布之后的状态,读回集外取值即坏数据,上抛不折成空。
//
// ListServiceProducts 与 FindServiceProduct 都经它（ADR-0361）：列表在上面取页，读回在上面多两格键过滤。
var serviceProductListed = `WITH listed AS (
        SELECT version.object_id, version.version_label, version.scope_ref, version.status,
               version.effective_starts_at, version.effective_ends_at, version.published_at,
               ` + catalogueClosureColumns + `,
               product.form,` + deliveryConditionColumns + `,
               ` + versionStatusCode + ` AS status_code
          FROM party_commercial.commercial_version AS version
          LEFT JOIN party_commercial.service_product_form AS product
                 ON product.tenant_id     = version.tenant_id
                AND product.object_kind   = version.object_kind
                AND product.object_id     = version.object_id
                AND product.version_label = version.version_label` + deliveryConditionJoin + `
          ` + versionClosureJoins("version") + `
         WHERE version.tenant_id   = $1
           AND version.object_kind = $2
     )`

// serviceProductRead：q 另覆盖形态，管理台今天检索的就是标识、版本、范围与形态。
var serviceProductRead = catalogueRead[ports.ServiceProductCatalogueRow]{
	binding:   ports.ServiceProductPages.MustBind(versionShellColumns("form")),
	listed:    serviceProductListed,
	selection: versionShellSelection + `, form, ` + deliveryConditionSelection,
	scan:      scanServiceProduct,
	position: func(row ports.ServiceProductCatalogueRow, sortField string) cataloguepage.Position {
		return versionShellPosition(sortField, row.ObjectID, row.VersionLabel, row.PublishedAt, row.EffectiveStartsAt)
	},
}

func scanServiceProduct(rows pgx.Rows) (ports.ServiceProductCatalogueRow, error) {
	var row ports.ServiceProductCatalogueRow
	var status int16
	var closure catalogueClosureScan
	var endsAt *time.Time
	var form *string
	var delivery deliveryConditionScan
	targets := append([]any{
		&row.ObjectID, &row.VersionLabel, &row.Scope, &status,
		&row.EffectiveStartsAt, &endsAt, &row.PublishedAt}, closure.targets()...)
	targets = append(targets, &form)
	targets = append(targets, delivery.targets()...)
	if err := rows.Scan(targets...); err != nil {
		return row, err
	}
	var err error
	if row.Status, err = versionStatusWord(status, closure.retiredAt != nil); err != nil {
		return row, err
	}
	row.SelectionEndsAt, row.HasSelectionCut, row.RetiredAt, row.Successor = closure.echo()
	if endsAt != nil {
		row.EffectiveEndsAt = *endsAt
		row.HasEffectiveEnd = true
	}
	if form != nil {
		row.Form = *form
		row.HasForm = true
	}
	if row.DeliveryConditions, err = delivery.row(); err != nil {
		return row, err
	}
	return row, nil
}

func (catalogue *OperationsCatalogue) ListServiceProducts(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
	query cataloguepage.Query,
) (cataloguepage.Result[ports.ServiceProductCatalogueRow], error) {
	return serviceProductRead.page(ctx, catalogue.db, "list service products", limit, query,
		tenant.String(), uint8(domain.ServiceProductObject))
}

func (catalogue *OperationsCatalogue) FindServiceProduct(
	ctx context.Context,
	tenant domain.TenantID,
	objectID domain.CommercialObjectID,
	version domain.CommercialVersionLabel,
) (ports.ServiceProductCatalogueRow, bool, error) {
	return serviceProductRead.find(ctx, catalogue.db, "find service product", keyOf(objectID, version),
		tenant.String(), uint8(domain.ServiceProductObject))
}

// acceptanceRulePackageListed 上列接单规则包正文册,连同挂在同一份规则包上的两族阶段
// 内容声明(0013 的收寄资格与终局规则)。一条语句取回全部父子——ReadExecutor 不保证
// 两条语句同一快照,分次取会拼出从未同时存在的父子状态(与 LoadAcceptanceRulePackage
// 同一条理由)。有父零子在内容读口是坏数据,在目录上列如实交回空集合:上列不重建领域
// 对象、不形成判断,拦坏数据仍归内容读口。
//
// **三族子表用相关子查询各聚各的,不再叠 LEFT JOIN + GROUP BY。** 多族子表同时以
// LEFT JOIN 挂上来会互相做笛卡尔积——规则 3 条 × 允许来源 2 条会摊成 6 行,`json_agg`
// 于是把每族都数重。这条语句原先只有一族,那个形状看不出问题;第二族一挂就出。两个
// 声明壳仍走 LEFT JOIN:它们与父同主键、至多一行,不产生扇出,而「壳在不在」正需要
// 连接后的 NULL 来判。
//
// ListAcceptanceRulePackages 与 FindAcceptanceRulePackage 都经它（ADR-0361）：列表在上面取页，读回在上面多两格键过滤。
const acceptanceRulePackageListed = `WITH listed AS (
        SELECT parent.object_id, parent.version_label,
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
                   AND rule.version_label = parent.version_label) AS rules,
               intake.object_id IS NOT NULL AS intake_declared,
               (SELECT COALESCE(json_agg(source.source_kind ORDER BY source.source_kind), '[]'::json)
                  FROM party_commercial.intake_allowed_source AS source
                 WHERE source.tenant_id     = parent.tenant_id
                   AND source.object_kind   = parent.object_kind
                   AND source.object_id     = parent.object_id
                   AND source.version_label = parent.version_label) AS intake_sources,
               (SELECT COALESCE(json_agg(ref.rule_reference ORDER BY ref.rule_reference), '[]'::json)
                  FROM party_commercial.intake_qualification_ref AS ref
                 WHERE ref.tenant_id     = parent.tenant_id
                   AND ref.object_kind   = parent.object_kind
                   AND ref.object_id     = parent.object_id
                   AND ref.version_label = parent.version_label) AS intake_refs,
               finals.object_id IS NOT NULL AS finals_declared,
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
                   AND final.version_label = parent.version_label) AS final_rules
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
     )`

// acceptanceRulePackageRead 落列见 ports.AcceptanceRulePackagePages；q 覆盖对象、版本与规则包上列的产品、合同、法人与
// 范围引用。
var acceptanceRulePackageRead = catalogueRead[ports.AcceptanceRulePackageRow]{
	binding: ports.AcceptanceRulePackagePages.MustBind(policyContentColumns("declaredAt", "declared_at", nil,
		"object_id", "version_label", "service_product_id", "contract_id", "legal_entity_ref", "scope_ref")),
	listed: acceptanceRulePackageListed,
	selection: `object_id, version_label, service_product_id, contract_id, legal_entity_ref,
		        scope_ref, effective_starts_at, effective_ends_at, declared_at, rules,
		        intake_declared, intake_sources, intake_refs, finals_declared, final_rules`,
	scan: scanAcceptanceRulePackage,
	position: func(row ports.AcceptanceRulePackageRow, sortField string) cataloguepage.Position {
		return policyContentPosition(sortField, row.ObjectID, row.VersionLabel, row.DeclaredAt, row.EffectiveStartsAt)
	},
}

func scanAcceptanceRulePackage(rows pgx.Rows) (ports.AcceptanceRulePackageRow, error) {
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
		return row, err
	}
	if endsAt != nil {
		row.EffectiveEndsAt = *endsAt
		row.HasEffectiveEnd = true
	}
	var err error
	if row.Rules, err = assembledRuleRowsFromJSON(rulesJSON); err != nil {
		return row, err
	}
	if row.AllowedIntakeSources, err = stringsFromJSON(sourcesJSON); err != nil {
		return row, fmt.Errorf("allowed intake sources: %w", err)
	}
	if row.IntakeQualificationRefs, err = stringsFromJSON(refsJSON); err != nil {
		return row, fmt.Errorf("intake qualification refs: %w", err)
	}
	if row.FinalRules, err = finalRuleRowsFromJSON(finalsJSON); err != nil {
		return row, err
	}
	return row, nil
}

func (catalogue *OperationsCatalogue) ListAcceptanceRulePackages(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
	query cataloguepage.Query,
) (cataloguepage.Result[ports.AcceptanceRulePackageRow], error) {
	return acceptanceRulePackageRead.page(ctx, catalogue.db, "list acceptance rule packages", limit, query, tenant.String())
}

func (catalogue *OperationsCatalogue) FindAcceptanceRulePackage(
	ctx context.Context,
	tenant domain.TenantID,
	objectID domain.CommercialObjectID,
	version domain.CommercialVersionLabel,
) (ports.AcceptanceRulePackageRow, bool, error) {
	return acceptanceRulePackageRead.find(ctx, catalogue.db, "find acceptance rule package", keyOf(objectID, version),
		tenant.String())
}

// policyContentPosition 给策略目录各正文册编本页末行的位置：at 是本册缺省序那一维（声明或登记时刻），另两维生效起点与对象号
// 在各册行上同名同义（ports.policyContentPages）。
func policyContentPosition(sortField, objectID, version string, at, effectiveStartsAt time.Time) cataloguepage.Position {
	value := objectID
	switch sortField {
	case "declaredAt", "registeredAt":
		value = cataloguepage.FormatInstant(at)
	case "effectiveStartsAt":
		value = cataloguepage.FormatInstant(effectiveStartsAt)
	}
	return cataloguepage.Position{Value: value, Identity: []string{objectID, version}}
}

// policyContentColumns 是策略目录里自带版本与生效区间的正文册共用的落列：at 是本册缺省序那一维落的列，keyword 是 q 覆盖的列。
func policyContentColumns(at, atColumn string, filters map[string]string, keyword ...string) cataloguepage.Columns {
	return cataloguepage.Columns{
		Sorts:    map[string]string{at: atColumn, "effectiveStartsAt": "effective_starts_at", "objectId": "object_id"},
		Identity: []string{"object_id", "version_label"},
		Filters:  filters,
		Keyword:  keyword,
	}
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

// preAcceptanceControlRead 上列接受前财务控制声明册。拥有对象是客户合同版本
// (object_kind CHECK 钉在 2),行内标识因此指名合同。requirement 在库上存的就是领域的词(0007 的 CHECK 与
// domain.PreAcceptanceControlRequirement 同一组),筛选按原词比;q 覆盖合同对象、合同版本与不适用依据。
var preAcceptanceControlRead = catalogueRead[ports.PreAcceptanceControlRow]{
	binding: ports.PreAcceptanceControlPages.MustBind(cataloguepage.Columns{
		Sorts:    map[string]string{"declaredAt": "declared_at", "contractObjectId": "object_id"},
		Identity: []string{"object_id", "version_label"},
		Filters:  map[string]string{"requirement": "requirement"},
		Keyword:  []string{"object_id", "version_label", "not_applicable_basis"},
	}),
	listed: `WITH listed AS (
        SELECT object_id, version_label, requirement, not_applicable_basis, declared_at
          FROM party_commercial.pre_acceptance_control_declaration
         WHERE tenant_id = $1
     )`,
	selection: `object_id, version_label, requirement, not_applicable_basis, declared_at`,
	scan: func(rows pgx.Rows) (ports.PreAcceptanceControlRow, error) {
		var row ports.PreAcceptanceControlRow
		var basis *string
		if err := rows.Scan(
			&row.ContractObjectID, &row.ContractVersion, &row.Requirement, &basis, &row.DeclaredAt,
		); err != nil {
			return row, err
		}
		if basis != nil {
			row.NotApplicableBasis = *basis
		}
		return row, nil
	},
	position: func(row ports.PreAcceptanceControlRow, sortField string) cataloguepage.Position {
		value := row.ContractObjectID
		if sortField == "declaredAt" {
			value = cataloguepage.FormatInstant(row.DeclaredAt)
		}
		return cataloguepage.Position{Value: value, Identity: []string{row.ContractObjectID, row.ContractVersion}}
	},
}

func (catalogue *OperationsCatalogue) ListPreAcceptanceControls(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
	query cataloguepage.Query,
) (cataloguepage.Result[ports.PreAcceptanceControlRow], error) {
	return preAcceptanceControlRead.page(ctx, catalogue.db, "list pre-acceptance controls", limit, query, tenant.String())
}

// pricePolicyRead 上列商业价格政策册。发布期保全的方案方向与转换照列转写(ADR-0057)；口径册
// (0022)左连接挂在正文行上,HasCaliber 说明有没有——0010 早于 0022,只有正文没有口径的行是合法
// 状态。汇率三列半缺是库与领域分叉的坏数据,上抛不吸收。
//
// direction 在库上存的就是领域的词(0010 的 CHECK 与 domain.PriceDirection 同一组),筛选按原词比;q 覆盖对象、版本、
// 方案引用与政策范围。ListPricePolicies 与 FindPricePolicy 都经它（ADR-0361）：列表在上面取页，读回在上面多两格键过滤。
var pricePolicyRead = catalogueRead[ports.PricePolicyRow]{
	binding: ports.PricePolicyPages.MustBind(policyContentColumns("registeredAt", "registered_at",
		map[string]string{"direction": "direction"},
		"object_id", "version_label", "plan_ref", "policy_scope_ref")),
	listed: `WITH listed AS (
        SELECT price.object_id, price.version_label, price.direction, price.plan_ref, price.plan_direction,
               price.binding_conversion, price.policy_scope_ref,
               price.effective_starts_at, price.effective_ends_at, price.registered_at,
               caliber.tax_disposition, caliber.tax_classification_ref, caliber.volumetric_factor_ref,
               caliber.fx_quote_type_ref, caliber.fx_as_of_semantics_ref, caliber.fx_as_of_policy_version,
               caliber.registered_at AS caliber_registered_at
          FROM party_commercial.commercial_price_policy AS price
          LEFT JOIN party_commercial.price_policy_caliber AS caliber
                 ON caliber.tenant_id     = price.tenant_id
                AND caliber.object_kind   = price.object_kind
                AND caliber.object_id     = price.object_id
                AND caliber.version_label = price.version_label
         WHERE price.tenant_id = $1
     )`,
	selection: `object_id, version_label, direction, plan_ref, plan_direction,
		        binding_conversion, policy_scope_ref, effective_starts_at, effective_ends_at, registered_at,
		        tax_disposition, tax_classification_ref, volumetric_factor_ref,
		        fx_quote_type_ref, fx_as_of_semantics_ref, fx_as_of_policy_version, caliber_registered_at`,
	scan: scanPricePolicy,
	position: func(row ports.PricePolicyRow, sortField string) cataloguepage.Position {
		return policyContentPosition(sortField, row.ObjectID, row.VersionLabel, row.RegisteredAt, row.EffectiveStartsAt)
	},
}

func scanPricePolicy(rows pgx.Rows) (ports.PricePolicyRow, error) {
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
		return row, err
	}
	if endsAt != nil {
		row.EffectiveEndsAt = *endsAt
		row.HasEffectiveEnd = true
	}
	if taxDisposition != nil {
		row.HasCaliber = true
		row.TaxDisposition = *taxDisposition
		row.TaxClassification = stringOrEmpty(taxClassification)
		row.HistoricalVolumetricFactor = stringOrEmpty(volumetricFactor)
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
			return row, fmt.Errorf("%s/%s 的汇率口径半缺", row.ObjectID, row.VersionLabel)
		}
	}
	return row, nil
}

func (catalogue *OperationsCatalogue) ListPricePolicies(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
	query cataloguepage.Query,
) (cataloguepage.Result[ports.PricePolicyRow], error) {
	return pricePolicyRead.page(ctx, catalogue.db, "list price policies", limit, query, tenant.String())
}

func (catalogue *OperationsCatalogue) FindPricePolicy(
	ctx context.Context,
	tenant domain.TenantID,
	objectID domain.CommercialObjectID,
	version domain.CommercialVersionLabel,
) (ports.PricePolicyRow, bool, error) {
	return pricePolicyRead.find(ctx, catalogue.db, "find price policy", keyOf(objectID, version), tenant.String())
}

// settlementPolicyRead 上列结算政策册:方式与六维适用范围平铺(ADR-0044)。
//
// method 在库上存的就是领域的词(0011 的 CHECK 与 domain.SettlementMethod 同一组),筛选按原词比;q 覆盖对象、版本与
// 适用范围里的法人、对手方、合同与费用范围引用。ListSettlementPolicies 与 FindSettlementPolicy 都经它（ADR-0361）：
// 列表在上面取页，读回在上面多两格键过滤。
var settlementPolicyRead = catalogueRead[ports.SettlementPolicyRow]{
	binding: ports.SettlementPolicyPages.MustBind(policyContentColumns("registeredAt", "registered_at",
		map[string]string{"method": "method"},
		"object_id", "version_label", "legal_entity_ref", "counterparty_ref", "contract_label", "charge_scope_ref")),
	listed: `WITH listed AS (
        SELECT object_id, version_label, method, legal_entity_ref, counterparty_ref,
               contract_label, charge_scope_ref, currency_code,
               effective_starts_at, effective_ends_at, registered_at
          FROM party_commercial.commercial_settlement_policy
         WHERE tenant_id = $1
     )`,
	selection: `object_id, version_label, method, legal_entity_ref, counterparty_ref,
		        contract_label, charge_scope_ref, currency_code,
		        effective_starts_at, effective_ends_at, registered_at`,
	scan: func(rows pgx.Rows) (ports.SettlementPolicyRow, error) {
		var row ports.SettlementPolicyRow
		var endsAt *time.Time
		if err := rows.Scan(
			&row.ObjectID, &row.VersionLabel, &row.Method, &row.LegalEntity, &row.Counterparty,
			&row.ContractLabel, &row.ChargeScope, &row.Currency,
			&row.EffectiveStartsAt, &endsAt, &row.RegisteredAt,
		); err != nil {
			return row, err
		}
		if endsAt != nil {
			row.EffectiveEndsAt = *endsAt
			row.HasEffectiveEnd = true
		}
		return row, nil
	},
	position: func(row ports.SettlementPolicyRow, sortField string) cataloguepage.Position {
		return policyContentPosition(sortField, row.ObjectID, row.VersionLabel, row.RegisteredAt, row.EffectiveStartsAt)
	},
}

func (catalogue *OperationsCatalogue) ListSettlementPolicies(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
	query cataloguepage.Query,
) (cataloguepage.Result[ports.SettlementPolicyRow], error) {
	return settlementPolicyRead.page(ctx, catalogue.db, "list settlement policies", limit, query, tenant.String())
}

func (catalogue *OperationsCatalogue) FindSettlementPolicy(
	ctx context.Context,
	tenant domain.TenantID,
	objectID domain.CommercialObjectID,
	version domain.CommercialVersionLabel,
) (ports.SettlementPolicyRow, bool, error) {
	return settlementPolicyRead.find(ctx, catalogue.db, "find settlement policy", keyOf(objectID, version), tenant.String())
}

// asOfPolicyRead 上列时点锚声明册:某规则包为某类下游判断声明的时点
// 语义与政策版本。表不存时点值,这里也就没有时点值可列。
//
// judgment_type 在库上存的就是领域的词(0005 的 CHECK 与 domain.JudgmentType 同一组),筛选按原词比;q 覆盖规则包对象与
// 版本、时点语义引用与政策版本。
var asOfPolicyRead = catalogueRead[ports.AsOfPolicyRow]{
	binding: ports.AsOfPolicyPages.MustBind(cataloguepage.Columns{
		Sorts:    map[string]string{"declaredAt": "declared_at", "rulePackageObjectId": "object_id"},
		Identity: []string{"object_id", "version_label", "judgment_type"},
		Filters:  map[string]string{"judgmentType": "judgment_type"},
		Keyword:  []string{"object_id", "version_label", "semantics_ref", "policy_version"},
	}),
	listed: `WITH listed AS (
        SELECT object_id, version_label, judgment_type, semantics_ref, policy_version, declared_at
          FROM party_commercial.as_of_policy_declaration
         WHERE tenant_id = $1
     )`,
	selection: `object_id, version_label, judgment_type, semantics_ref, policy_version, declared_at`,
	scan: func(rows pgx.Rows) (ports.AsOfPolicyRow, error) {
		var row ports.AsOfPolicyRow
		err := rows.Scan(
			&row.RulePackageObjectID, &row.RulePackageVersion,
			&row.JudgmentType, &row.SemanticsRef, &row.PolicyVersion, &row.DeclaredAt,
		)
		return row, err
	},
	position: func(row ports.AsOfPolicyRow, sortField string) cataloguepage.Position {
		value := row.RulePackageObjectID
		if sortField == "declaredAt" {
			value = cataloguepage.FormatInstant(row.DeclaredAt)
		}
		return cataloguepage.Position{
			Value:    value,
			Identity: []string{row.RulePackageObjectID, row.RulePackageVersion, row.JudgmentType},
		}
	},
}

func (catalogue *OperationsCatalogue) ListAsOfPolicyDeclarations(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
	query cataloguepage.Query,
) (cataloguepage.Result[ports.AsOfPolicyRow], error) {
	return asOfPolicyRead.page(ctx, catalogue.db, "list as-of policy declarations", limit, query, tenant.String())
}

// customerContractListed 上列客户合同版本(壳),左连接正文册、控制约定册与交付条件册(0030 合同层)。
//
// 四张表一条语句取回,不分两次:ReadExecutor 不保证两条语句同一快照,分次会拼出
// 从未同时存在的壳/正文/绑定组合(与 ListAcceptanceRulePackages 同一条理由)。
//
// 两级 LEFT JOIN 的第二级挂在**正文**上而不是壳上,这不是写法偏好:0012 的外键
// 就是绑定→正文,挂壳上会在正文缺席时把绑定行也带进来,而那种行库上根本不存在,
// 读出来只会是一份自相矛盾的证据。交付条件父表按主键挂在壳上(与正文各自可缺,是两层),
// 方式子表走相关子查询——与约定行并列 LEFT JOIN 会互相做笛卡尔积。
//
// ListCustomerContracts 与 FindCustomerContract 都经它（ADR-0361）：列表在上面取页，读回在上面多两格键过滤。
var customerContractListed = `WITH listed AS (
        SELECT version.object_id, version.version_label, version.scope_ref, version.status,
               version.effective_starts_at, version.effective_ends_at, version.published_at,
               ` + catalogueClosureColumns + `,
               content.rule_package_id, content.customer_account_id, content.declared_at AS content_declared_at,
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
               ) AS bindings,` + deliveryConditionColumns + `,
               ` + versionStatusCode + ` AS status_code
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
                AND binding.version_label = content.version_label` + deliveryConditionJoin + `
          ` + versionClosureJoins("version") + `
         WHERE version.tenant_id   = $1
           AND version.object_kind = $2
         GROUP BY version.tenant_id, version.object_kind,
                  version.object_id, version.version_label, version.scope_ref, version.status,
                  version.effective_starts_at, version.effective_ends_at, version.published_at,
                  content.rule_package_id, content.customer_account_id, content.declared_at,
                  latest_correction.corrected_ends_at, retirement.successor_label, retirement.retired_at,
                  delivery.tenant_id, delivery.object_kind, delivery.object_id, delivery.version_label,
                  delivery.recipient_scope_rule_ref, delivery.proof_of_delivery_rule_ref,
                  delivery.tightens_object_id, delivery.tightens_version_label, delivery.declared_at
     )`

// customerContractRead：q 另覆盖规则包与客户账户，管理台今天检索的就是这几格。
var customerContractRead = catalogueRead[ports.CustomerContractCatalogueRow]{
	binding: ports.CustomerContractPages.MustBind(versionShellColumns("rule_package_id", "customer_account_id")),
	listed:  customerContractListed,
	selection: versionShellSelection + `, rule_package_id, customer_account_id, content_declared_at, bindings, ` +
		deliveryConditionSelection,
	scan: scanCustomerContract,
	position: func(row ports.CustomerContractCatalogueRow, sortField string) cataloguepage.Position {
		return versionShellPosition(sortField, row.ObjectID, row.VersionLabel, row.PublishedAt, row.EffectiveStartsAt)
	},
}

func scanCustomerContract(rows pgx.Rows) (ports.CustomerContractCatalogueRow, error) {
	var row ports.CustomerContractCatalogueRow
	var status int16
	var closure catalogueClosureScan
	var endsAt *time.Time
	var rulePackage *string
	var customerAccount *string
	var declaredAt *time.Time
	var bindingsJSON []byte
	var delivery deliveryConditionScan
	targets := append([]any{
		&row.ObjectID, &row.VersionLabel, &row.Scope, &status,
		&row.EffectiveStartsAt, &endsAt, &row.PublishedAt}, closure.targets()...)
	targets = append(targets, &rulePackage, &customerAccount, &declaredAt, &bindingsJSON)
	targets = append(targets, delivery.targets()...)
	if err := rows.Scan(targets...); err != nil {
		return row, err
	}
	var err error
	if row.Status, err = versionStatusWord(status, closure.retiredAt != nil); err != nil {
		return row, err
	}
	row.SelectionEndsAt, row.HasSelectionCut, row.RetiredAt, row.Successor = closure.echo()
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
	// customer_account_id 在 0037 之前写下的旧行上是 NULL(没有这一格可补),如实交回空串。
	if customerAccount != nil {
		row.CustomerAccountID = *customerAccount
	}
	if declaredAt != nil {
		row.DeclaredAt = *declaredAt
	}
	if row.Bindings, err = controlBindingRowsFromJSON(bindingsJSON); err != nil {
		return row, err
	}
	if row.DeliveryConditions, err = delivery.row(); err != nil {
		return row, err
	}
	return row, nil
}

func (catalogue *OperationsCatalogue) ListCustomerContracts(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
	query cataloguepage.Query,
) (cataloguepage.Result[ports.CustomerContractCatalogueRow], error) {
	return customerContractRead.page(ctx, catalogue.db, "list customer contracts", limit, query,
		tenant.String(), uint8(domain.CustomerContractObject))
}

func (catalogue *OperationsCatalogue) FindCustomerContract(
	ctx context.Context,
	tenant domain.TenantID,
	objectID domain.CommercialObjectID,
	version domain.CommercialVersionLabel,
) (ports.CustomerContractCatalogueRow, bool, error) {
	return customerContractRead.find(ctx, catalogue.db, "find customer contract", keyOf(objectID, version),
		tenant.String(), uint8(domain.CustomerContractObject))
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

// authorizationRuleListed 上列授权规则版本(壳),连同挂在它上面的取消授权目录。
//
// 目录壳与父同主键、至多一行,走 LEFT JOIN——「壳在不在」正需要连接后的 NULL 来判;
// 请求方声明走相关子查询,理由见 acceptanceRulePackageListed 的头注。这里今天只有一族
// 子表,叠 LEFT JOIN + GROUP BY 也不会错,用子查询是为了第二族挂上来时不必重走一遍
// 那个坑。
//
// ListAuthorizationRules 与 FindAuthorizationRule 都经它（ADR-0361）：列表在上面取页，读回在上面多两格键过滤。
var authorizationRuleListed = `WITH listed AS (
        SELECT version.object_id, version.version_label, version.scope_ref, version.status,
               version.effective_starts_at, version.effective_ends_at, version.published_at,
               ` + catalogueClosureColumns + `,
               content.declared_at AS authority_declared_at,
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
                   AND declaration.version_label = version.version_label) AS authorities,
               ` + versionStatusCode + ` AS status_code
          FROM party_commercial.commercial_version AS version
          LEFT JOIN party_commercial.cancellation_authority_content AS content
                 ON content.tenant_id     = version.tenant_id
                AND content.object_kind   = version.object_kind
                AND content.object_id     = version.object_id
                AND content.version_label = version.version_label
          ` + versionClosureJoins("version") + `
         WHERE version.tenant_id   = $1
           AND version.object_kind = $2
     )`

var authorizationRuleRead = catalogueRead[ports.AuthorizationRuleRow]{
	binding:   ports.AuthorizationRulePages.MustBind(versionShellColumns()),
	listed:    authorizationRuleListed,
	selection: versionShellSelection + `, authority_declared_at, authorities`,
	scan: func(rows pgx.Rows) (ports.AuthorizationRuleRow, error) {
		var row ports.AuthorizationRuleRow
		var status int16
		var closure catalogueClosureScan
		var endsAt *time.Time
		var declaredAt *time.Time
		var authoritiesJSON []byte
		targets := append([]any{
			&row.ObjectID, &row.VersionLabel, &row.Scope, &status,
			&row.EffectiveStartsAt, &endsAt, &row.PublishedAt}, closure.targets()...)
		targets = append(targets, &declaredAt, &authoritiesJSON)
		if err := rows.Scan(targets...); err != nil {
			return row, err
		}
		var err error
		if row.Status, err = versionStatusWord(status, closure.retiredAt != nil); err != nil {
			return row, err
		}
		row.SelectionEndsAt, row.HasSelectionCut, row.RetiredAt, row.Successor = closure.echo()
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
			return row, err
		}
		return row, nil
	},
	position: func(row ports.AuthorizationRuleRow, sortField string) cataloguepage.Position {
		return versionShellPosition(sortField, row.ObjectID, row.VersionLabel, row.PublishedAt, row.EffectiveStartsAt)
	},
}

func (catalogue *OperationsCatalogue) ListAuthorizationRules(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
	query cataloguepage.Query,
) (cataloguepage.Result[ports.AuthorizationRuleRow], error) {
	return authorizationRuleRead.page(ctx, catalogue.db, "list authorization rules", limit, query,
		tenant.String(), uint8(domain.AuthorizationRuleObject))
}

func (catalogue *OperationsCatalogue) FindAuthorizationRule(
	ctx context.Context,
	tenant domain.TenantID,
	objectID domain.CommercialObjectID,
	version domain.CommercialVersionLabel,
) (ports.AuthorizationRuleRow, bool, error) {
	return authorizationRuleRead.find(ctx, catalogue.db, "find authorization rule", keyOf(objectID, version),
		tenant.String(), uint8(domain.AuthorizationRuleObject))
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

// catalogueClosureColumns 与 versionClosureJoins("version") 配对：目录只回显截止点（最新一条更正的终点）与退役的后继、时刻，
// 不回显更正的起点与引用。版本行的状态列随发布固定，退役另册保存；不折进来，管理台就会在退役之后照旧显示`已生效`，
// 与解析和点读答的两样。
const catalogueClosureColumns = `latest_correction.corrected_ends_at, retirement.successor_label, retirement.retired_at`

type catalogueClosureScan struct {
	cutAt     *time.Time
	successor *string
	retiredAt *time.Time
}

func (closure *catalogueClosureScan) targets() []any {
	return []any{&closure.cutAt, &closure.successor, &closure.retiredAt}
}

// echo 折出目录行上的四格：截止点及其在场标记，退役的时刻与后继。退役两格同在才填，与 ListSupplierAgreements 同口径。
func (closure *catalogueClosureScan) echo() (cutAt time.Time, hasCut bool, retiredAt time.Time, successor string) {
	if closure.cutAt != nil {
		cutAt, hasCut = *closure.cutAt, true
	}
	if closure.retiredAt != nil && closure.successor != nil {
		retiredAt, successor = *closure.retiredAt, *closure.successor
	}
	return cutAt, hasCut, retiredAt, successor
}

// versionStatusWord 把版本行的状态列折成目录上的那一格：退役了就是`已退役`，与整册装载施加退役之后的位置相同。
func versionStatusWord(status int16, retired bool) (string, error) {
	word := domain.CommercialVersionStatus(status).String()
	if word == "" {
		return "", fmt.Errorf("版本状态 %d 不在封闭集内", status)
	}
	if retired {
		return domain.CommercialVersionRetired.String(), nil
	}
	return word, nil
}

// versionStatusCode 是版本壳各册 status 维落的那一列，与 versionStatusWord 同一个折法：版本行的状态列随发布固定、退役另册
// 保存，目录行上那一格是折进退役之后的位置。筛选按同一个位置比，否则筛 RETIRED 筛不出已退役的版本、筛别的状态反倒混进
// 它们。要在 versionClosureJoins("version") 之后用。
var versionStatusCode = fmt.Sprintf(
	"CASE WHEN retirement.retired_at IS NOT NULL THEN %d ELSE version.status END", domain.CommercialVersionRetired)

// versionShellSelection 是版本壳各册在 listed 里导出、取页与读回打头的那几列：版本行七列加 catalogueClosureColumns。
const versionShellSelection = `object_id, version_label, scope_ref, status,
		        effective_starts_at, effective_ends_at, published_at,
		        corrected_ends_at, successor_label, retired_at`

// versionShellColumns 是版本壳各册共用的落列（声明在 ports 的 versionShellPages）：排序与标识落在版本行上，status 落在
// versionStatusCode 导出的 status_code 上；q 覆盖对象、版本与范围，extra 是各册另覆盖的正文列。
func versionShellColumns(extra ...string) cataloguepage.Columns {
	return cataloguepage.Columns{
		Sorts: map[string]string{
			"publishedAt": "published_at", "effectiveStartsAt": "effective_starts_at", "objectId": "object_id",
		},
		Identity: []string{"object_id", "version_label"},
		Filters:  map[string]string{"status": "status_code"},
		Codes:    map[string]func(string) (int64, error){"status": cataloguepage.Code[domain.CommercialVersionStatus]},
		Keyword:  append([]string{"object_id", "version_label", "scope_ref"}, extra...),
	}
}

// versionShellPosition 给版本壳各册编本页末行的位置，三个可排维在各册行上同名同义。
func versionShellPosition(sortField, objectID, version string, publishedAt, effectiveStartsAt time.Time) cataloguepage.Position {
	value := objectID
	switch sortField {
	case "publishedAt":
		value = cataloguepage.FormatInstant(publishedAt)
	case "effectiveStartsAt":
		value = cataloguepage.FormatInstant(effectiveStartsAt)
	}
	return cataloguepage.Position{Value: value, Identity: []string{objectID, version}}
}

// supplierAgreementListed 上列供应商协议版本壳,正文(0021)左连接。
//
// 装载方向与 ListCustomerContracts 同派:版本侧驱动,正文左连接——壳可先入册、正文随发布
// 登记,只列正文行会让未登正文的已发布协议从目录上消失。方向不在行上:领域恒为 BUY、库上
// 不成列。
//
// ListSupplierAgreements 与 FindSupplierAgreement 都经它（ADR-0361）：列表在上面取页，读回在上面多两格键过滤。
var supplierAgreementListed = `WITH listed AS (
        SELECT version.object_id, version.version_label, version.scope_ref, version.status,
               version.effective_starts_at, version.effective_ends_at, version.published_at,
               ` + catalogueClosureColumns + `,
               content.supplier_party_id, content.legal_entity_ref, content.purchase_plan_ref,
               content.agreement_scope_ref, content.effective_starts_at AS agreement_effective_starts_at,
               content.effective_ends_at AS agreement_effective_ends_at,
               content.registered_at AS agreement_registered_at,
               ` + versionStatusCode + ` AS status_code
          FROM party_commercial.commercial_version AS version
          LEFT JOIN party_commercial.supplier_agreement AS content
                 ON content.tenant_id     = version.tenant_id
                AND content.object_kind   = version.object_kind
                AND content.object_id     = version.object_id
                AND content.version_label = version.version_label
          ` + versionClosureJoins("version") + `
         WHERE version.tenant_id   = $1
           AND version.object_kind = $2
     )`

// supplierAgreementRead：q 另覆盖正文里的供应商、法人、价格表与协议范围，管理台今天检索的是转写后的各格。
var supplierAgreementRead = catalogueRead[ports.SupplierAgreementCatalogueRow]{
	binding: ports.SupplierAgreementPages.MustBind(versionShellColumns(
		"supplier_party_id", "legal_entity_ref", "purchase_plan_ref", "agreement_scope_ref")),
	listed: supplierAgreementListed,
	selection: versionShellSelection + `, supplier_party_id, legal_entity_ref, purchase_plan_ref,
		        agreement_scope_ref, agreement_effective_starts_at, agreement_effective_ends_at, agreement_registered_at`,
	scan: scanSupplierAgreement,
	position: func(row ports.SupplierAgreementCatalogueRow, sortField string) cataloguepage.Position {
		return versionShellPosition(sortField, row.ObjectID, row.VersionLabel, row.PublishedAt, row.EffectiveStartsAt)
	},
}

func scanSupplierAgreement(rows pgx.Rows) (ports.SupplierAgreementCatalogueRow, error) {
	var row ports.SupplierAgreementCatalogueRow
	var status int16
	var closure catalogueClosureScan
	var endsAt *time.Time
	var supplier, legalEntity, purchasePlan, agreementScope *string
	var agreementStartsAt, agreementEndsAt, registeredAt *time.Time
	targets := append([]any{
		&row.ObjectID, &row.VersionLabel, &row.Scope, &status,
		&row.EffectiveStartsAt, &endsAt, &row.PublishedAt}, closure.targets()...)
	targets = append(targets,
		&supplier, &legalEntity, &purchasePlan, &agreementScope,
		&agreementStartsAt, &agreementEndsAt, &registeredAt)
	if err := rows.Scan(targets...); err != nil {
		return row, err
	}
	var err error
	if row.Status, err = versionStatusWord(status, closure.retiredAt != nil); err != nil {
		return row, err
	}
	// 收尾照版本点读的口径折进来（ADR-0233 决定五）：退役改状态并回显后继与时刻，截止回显最新一条更正的终点；原区间
	// 照旧是那一格，不拿收尾覆盖它。
	row.SelectionEndsAt, row.HasSelectionCut, row.RetiredAt, row.Successor = closure.echo()
	if endsAt != nil {
		row.EffectiveEndsAt = *endsAt
		row.HasEffectiveEnd = true
	}
	// supplier_party_id 在正文表上 NOT NULL,它的在场即正文行的在场;其余正文列同为
	// NOT NULL(除区间终点),到这里非空是表形保证的,缺一列即坏数据。
	if supplier != nil {
		if legalEntity == nil || purchasePlan == nil || agreementScope == nil ||
			agreementStartsAt == nil || registeredAt == nil {
			return row, fmt.Errorf("%s/%s 的正文行不完整", row.ObjectID, row.VersionLabel)
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
	return row, nil
}

func (catalogue *OperationsCatalogue) ListSupplierAgreements(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
	query cataloguepage.Query,
) (cataloguepage.Result[ports.SupplierAgreementCatalogueRow], error) {
	return supplierAgreementRead.page(ctx, catalogue.db, "list supplier agreements", limit, query,
		tenant.String(), uint8(domain.SupplierAgreementObject))
}

func (catalogue *OperationsCatalogue) FindSupplierAgreement(
	ctx context.Context,
	tenant domain.TenantID,
	objectID domain.CommercialObjectID,
	version domain.CommercialVersionLabel,
) (ports.SupplierAgreementCatalogueRow, bool, error) {
	return supplierAgreementRead.find(ctx, catalogue.db, "find supplier agreement", keyOf(objectID, version),
		tenant.String(), uint8(domain.SupplierAgreementObject))
}
