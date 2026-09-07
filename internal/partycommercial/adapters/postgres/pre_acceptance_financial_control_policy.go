package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// 接受前财务控制策略册的持久化面（票 party-commercial-context-gaps/07，ADR-0115，0024 迁移）。写口挂在
// CommercialPublications 上与其余正文册同笔登记；读口是独立的 PreAcceptanceFinancialControlPolicyContents
// ——正文不进整册装载（LoadForScope），理由见 ports.PreAcceptanceFinancialControlPolicyContentView。
//
// 判读纪律同客户服务规则册（customer_service_rule.go）：**先读回既有正文再决定写不写**。正文横跨父子
// 两表，逐行 DO NOTHING 会把半份新正文并进旧正文——冲突路径必须一行不写，否则「绝不覆盖」只对父行成立。

// SavePreAcceptanceFinancialControlPolicy 登记一份接受前财务控制策略版本的正文。撞键不覆盖：同内容是
// 重放，异内容（共同通过条件、任一控制项的种类、范围、顺序、失败处置或责任不同，或行数不同）是需要
// 商业责任方修正的冲突。
func (repository *CommercialPublications) SavePreAcceptanceFinancialControlPolicy(
	ctx context.Context,
	policy domain.PreAcceptanceFinancialControlPolicy,
) (ports.PreAcceptanceFinancialControlPolicySaveOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.PreAcceptanceFinancialControlPolicySaveOutcomeInvalid,
			fmt.Errorf("save pre-acceptance financial control policy: %w", err)
	}

	incoming := controlPolicyItemsOf(policy)
	if policy.JointPassCondition().String() == "" || len(incoming) == 0 {
		// 零值正文过不了 NewPreAcceptanceFinancialControlPolicy，走到这里只可能是绕开构造门的零值。拦在
		// INSERT 前，否则 CHECK 会以一条技术错误报出一件领域上早该拒绝的事。
		return ports.PreAcceptanceFinancialControlPolicySaveOutcomeInvalid,
			fmt.Errorf("save pre-acceptance financial control policy: %w", domain.ErrInvalidPreAcceptanceFinancialControlPolicy)
	}
	tenant, kind, objectID, label := ownerColumns(policy.Version())

	var existingJointPass string
	present, err := scanControlPolicyParent(ctx, executor, &existingJointPass, tenant, kind, objectID, label)
	if err != nil {
		return ports.PreAcceptanceFinancialControlPolicySaveOutcomeInvalid,
			fmt.Errorf("save pre-acceptance financial control policy: %w", err)
	}
	if present {
		if existingJointPass != policy.JointPassCondition().String() {
			return ports.PreAcceptanceFinancialControlPolicyContentConflict, nil
		}
		existingItems, err := scanControlPolicyItems(ctx, executor, tenant, kind, objectID, label)
		if err != nil {
			return ports.PreAcceptanceFinancialControlPolicySaveOutcomeInvalid,
				fmt.Errorf("save pre-acceptance financial control policy: %w", err)
		}
		if !sameControlPolicyItems(existingItems, incoming) {
			return ports.PreAcceptanceFinancialControlPolicyContentConflict, nil
		}
		return ports.PreAcceptanceFinancialControlPolicyAlreadyRegistered, nil
	}

	if _, err := executor.Exec(ctx,
		`INSERT INTO party_commercial.pre_acceptance_financial_control_policy
			(tenant_id, object_kind, object_id, version_label, joint_pass_condition)
		 VALUES ($1, $2, $3, $4, $5)`,
		tenant, kind, objectID, label, policy.JointPassCondition().String(),
	); err != nil {
		return ports.PreAcceptanceFinancialControlPolicySaveOutcomeInvalid,
			fmt.Errorf("save pre-acceptance financial control policy: %w", err)
	}
	for _, item := range policy.Items() {
		if _, err := executor.Exec(ctx,
			`INSERT INTO party_commercial.pre_acceptance_financial_control_item
				(tenant_id, object_kind, object_id, version_label,
				 control_kind, charge_scope_ref, evaluation_order, failure_disposition, responsibility_ref)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
			tenant, kind, objectID, label,
			item.Kind().String(), item.Scope().String(), item.EvaluationOrder(),
			item.FailureDisposition().String(), item.Responsibility().String(),
		); err != nil {
			return ports.PreAcceptanceFinancialControlPolicySaveOutcomeInvalid,
				fmt.Errorf("save pre-acceptance financial control policy: %w", err)
		}
	}
	return ports.PreAcceptanceFinancialControlPolicySaved, nil
}

// scannedControlItem 是一行控制项的比对形。键是（种类 + 范围），与库上主键同构；顺序、处置与责任是
// 比对的内容——任一不同都是另一份正文。
type scannedControlItem struct {
	order          int
	disposition    string
	responsibility string
}

func controlItemKey(kind, scope string) string {
	return kind + "\x1f" + scope
}

func controlPolicyItemsOf(policy domain.PreAcceptanceFinancialControlPolicy) map[string]scannedControlItem {
	items := make(map[string]scannedControlItem)
	for _, item := range policy.Items() {
		items[controlItemKey(item.Kind().String(), item.Scope().String())] = scannedControlItem{
			order:          item.EvaluationOrder(),
			disposition:    item.FailureDisposition().String(),
			responsibility: item.Responsibility().String(),
		}
	}
	return items
}

// sameControlPolicyItems 按键比对两份控制项集合；声明顺序不构成不同的内容，判断顺序是内容的一部分。
func sameControlPolicyItems(left, right map[string]scannedControlItem) bool {
	if len(left) != len(right) {
		return false
	}
	for key, item := range left {
		if right[key] != item {
			return false
		}
	}
	return true
}

func scanControlPolicyParent(
	ctx context.Context,
	executor bentopg.Executor,
	jointPass *string,
	tenant string, kind uint8, objectID, label string,
) (bool, error) {
	rows, err := executor.Query(ctx,
		`SELECT joint_pass_condition
		   FROM party_commercial.pre_acceptance_financial_control_policy
		  WHERE tenant_id = $1 AND object_kind = $2 AND object_id = $3 AND version_label = $4`,
		tenant, kind, objectID, label)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	present := false
	for rows.Next() {
		if err := rows.Scan(jointPass); err != nil {
			return false, err
		}
		present = true
	}
	return present, rows.Err()
}

func scanControlPolicyItems(
	ctx context.Context,
	executor bentopg.Executor,
	tenant string, kind uint8, objectID, label string,
) (map[string]scannedControlItem, error) {
	items := make(map[string]scannedControlItem)
	rows, err := executor.Query(ctx,
		`SELECT control_kind, charge_scope_ref, evaluation_order, failure_disposition, responsibility_ref
		   FROM party_commercial.pre_acceptance_financial_control_item
		  WHERE tenant_id = $1 AND object_kind = $2 AND object_id = $3 AND version_label = $4`,
		tenant, kind, objectID, label)
	if err != nil {
		return items, err
	}
	defer rows.Close()
	for rows.Next() {
		var controlKind, scope string
		var item scannedControlItem
		if err := rows.Scan(&controlKind, &scope, &item.order, &item.disposition, &item.responsibility); err != nil {
			return items, err
		}
		items[controlItemKey(controlKind, scope)] = item
	}
	return items, rows.Err()
}

// PreAcceptanceFinancialControlPolicyContents 实现 ports.PreAcceptanceFinancialControlPolicyContentView：
// 按已唯一选出的策略版本取回正文。
//
// 只读。正文属实例半边，本适配器不提供写口（写口在 CommercialPublications 上随发布同笔），也不在读不到
// 时代拟任何控制项——缺正文就是未登记，settlement-accounting 据以停在`未配置`格。
type PreAcceptanceFinancialControlPolicyContents struct {
	db *bentopg.DB
}

func NewPreAcceptanceFinancialControlPolicyContents(db *bentopg.DB) (*PreAcceptanceFinancialControlPolicyContents, error) {
	if db == nil {
		return nil, fmt.Errorf("party commercial postgres: db is nil")
	}
	return &PreAcceptanceFinancialControlPolicyContents{db: db}, nil
}

var _ ports.PreAcceptanceFinancialControlPolicyContentView = (*PreAcceptanceFinancialControlPolicyContents)(nil)

// LoadPreAcceptanceFinancialControlPolicy 取回策略正文。
//
// found=false = 正文未登记（无父行）。显式租户与版本必须同一身份，否则 error 且不交内容
// （ADR-0003/0040，租户是身份不是过滤器）。父行与子表由一条语句取回：ReadExecutor 不保证多条语句同一
// 快照，分两次会拼出从未同时存在的父子状态。读回的正文过 NewPreAcceptanceFinancialControlPolicy 重建——
// 有父无子在那里被拒，上抛不折成未登记。
func (repository *PreAcceptanceFinancialControlPolicyContents) LoadPreAcceptanceFinancialControlPolicy(
	ctx context.Context,
	tenant domain.TenantID,
	policy domain.CommercialVersion,
) (domain.PreAcceptanceFinancialControlPolicy, bool, error) {
	none := domain.PreAcceptanceFinancialControlPolicy{}
	if tenant.String() == "" ||
		policy.ObjectID().String() == "" || policy.Version().String() == "" {
		return none, false, fmt.Errorf("load pre-acceptance financial control policy: tenant and policy identity are required")
	}
	if tenant != policy.Tenant() {
		return none, false, fmt.Errorf("load pre-acceptance financial control policy: tenant does not own this policy")
	}
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return none, false, fmt.Errorf("load pre-acceptance financial control policy: %w", err)
	}

	var jointPassRaw string
	var itemsJSON []byte
	err = querier.QueryRow(ctx,
		`SELECT parent.joint_pass_condition,
		        (SELECT COALESCE(
		                    json_agg(
		                        json_build_object(
		                            'control',        item.control_kind,
		                            'chargeScope',    item.charge_scope_ref,
		                            'order',          item.evaluation_order,
		                            'onFailure',      item.failure_disposition,
		                            'responsibility', item.responsibility_ref
		                        )
		                        ORDER BY item.evaluation_order
		                    ),
		                    '[]'::json
		                )
		           FROM party_commercial.pre_acceptance_financial_control_item AS item
		          WHERE item.tenant_id     = parent.tenant_id
		            AND item.object_kind   = parent.object_kind
		            AND item.object_id     = parent.object_id
		            AND item.version_label = parent.version_label)
		   FROM party_commercial.pre_acceptance_financial_control_policy AS parent
		  WHERE parent.tenant_id     = $1
		    AND parent.object_kind   = $2
		    AND parent.object_id     = $3
		    AND parent.version_label = $4`,
		tenant.String(),
		uint8(domain.PreAcceptanceFinancialControlPolicyObject),
		policy.ObjectID().String(),
		policy.Version().String(),
	).Scan(&jointPassRaw, &itemsJSON)
	if errors.Is(err, pgx.ErrNoRows) {
		return none, false, nil
	}
	if err != nil {
		return none, false, fmt.Errorf("load pre-acceptance financial control policy: %w", err)
	}

	content, err := controlPolicyFrom(policy, jointPassRaw, itemsJSON)
	if err != nil {
		return none, false, fmt.Errorf("load pre-acceptance financial control policy: %w", err)
	}
	return content, true, nil
}

func controlPolicyFrom(
	version domain.CommercialVersion,
	jointPassRaw string,
	itemsJSON []byte,
) (domain.PreAcceptanceFinancialControlPolicy, error) {
	none := domain.PreAcceptanceFinancialControlPolicy{}
	jointPass, err := jointPassConditionFrom(jointPassRaw)
	if err != nil {
		return none, err
	}
	items, err := controlItemsFromJSON(itemsJSON)
	if err != nil {
		return none, err
	}
	return domain.NewPreAcceptanceFinancialControlPolicy(version, jointPass, items)
}

// controlItemDocument 是子表一行在 json_agg 里的形状，键名与受控 CLI 批文的控制项同名——两处读的是
// 同一份领域对象的两种表示，键名一致让人对得上，但它们各自独立解码、互不复用结构。
type controlItemDocument struct {
	Control        string `json:"control"`
	ChargeScope    string `json:"chargeScope"`
	Order          int    `json:"order"`
	OnFailure      string `json:"onFailure"`
	Responsibility string `json:"responsibility"`
}

func controlItemsFromJSON(raw []byte) ([]domain.PreAcceptanceControlItem, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var documents []controlItemDocument
	if err := json.Unmarshal(raw, &documents); err != nil {
		return nil, fmt.Errorf("control items are not this adapter's shape: %w", err)
	}
	items := make([]domain.PreAcceptanceControlItem, 0, len(documents))
	for _, document := range documents {
		kind, err := preAcceptanceControlKindFrom(document.Control)
		if err != nil {
			return nil, err
		}
		scope, err := domain.NewChargeScopeReference(document.ChargeScope)
		if err != nil {
			return nil, err
		}
		disposition, err := controlFailureDispositionFrom(document.OnFailure)
		if err != nil {
			return nil, err
		}
		responsibility, err := domain.NewControlResponsibilityReference(document.Responsibility)
		if err != nil {
			return nil, err
		}
		item, err := domain.NewPreAcceptanceControlItem(kind, scope, document.Order, disposition, responsibility)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

// ListPreAcceptanceFinancialControlPolicies 上列接受前财务控制策略版本壳，正文（0024）左连接
// （票 admin-write-faces/06）。
//
// 装载方向与 ListCustomerServiceRules 同派：版本侧驱动，正文左连接——壳可先入册、正文随发布登记，
// 只列正文行会让未登正文的已发布策略版本从目录上消失，而那个状态正是票 06 立票时「发布成功后管理台
// 找不到它」的状态，目录必须让它可见。子表由一个子查询聚成 json 数组，与父行同一条语句取回。
//
// 目录不重建领域对象、不形成判断：有父行而零子行是坏数据（领域要求至少一项），拦它归内容读口
// LoadPreAcceptanceFinancialControlPolicy；这里照 ListCustomerServiceRules 的先例如实交回空集合。
func (catalogue *OperationsCatalogue) ListPreAcceptanceFinancialControlPolicies(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
) ([]ports.PreAcceptanceFinancialControlPolicyRow, error) {
	if err := requirePositiveLimit("list pre-acceptance financial control policies", limit); err != nil {
		return nil, err
	}
	querier, err := catalogue.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("list pre-acceptance financial control policies: %w", err)
	}

	rows, err := querier.Query(ctx,
		`SELECT version.object_id, version.version_label, version.scope_ref, version.status,
		        version.effective_starts_at, version.effective_ends_at, version.published_at,
		        content.joint_pass_condition, content.registered_at,
		        (SELECT COALESCE(
		                    json_agg(
		                        json_build_object(
		                            'control',        item.control_kind,
		                            'chargeScope',    item.charge_scope_ref,
		                            'order',          item.evaluation_order,
		                            'onFailure',      item.failure_disposition,
		                            'responsibility', item.responsibility_ref
		                        )
		                        ORDER BY item.evaluation_order
		                    ),
		                    '[]'::json
		                )
		           FROM party_commercial.pre_acceptance_financial_control_item AS item
		          WHERE item.tenant_id     = version.tenant_id
		            AND item.object_kind   = version.object_kind
		            AND item.object_id     = version.object_id
		            AND item.version_label = version.version_label)
		   FROM party_commercial.commercial_version AS version
		   LEFT JOIN party_commercial.pre_acceptance_financial_control_policy AS content
		          ON content.tenant_id     = version.tenant_id
		         AND content.object_kind   = version.object_kind
		         AND content.object_id     = version.object_id
		         AND content.version_label = version.version_label
		  WHERE version.tenant_id   = $1
		    AND version.object_kind = $2
		  ORDER BY version.published_at DESC, version.object_id, version.version_label
		  LIMIT $3`,
		tenant.String(),
		uint8(domain.PreAcceptanceFinancialControlPolicyObject),
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list pre-acceptance financial control policies: %w", err)
	}
	defer rows.Close()

	policyRows := make([]ports.PreAcceptanceFinancialControlPolicyRow, 0, limit)
	for rows.Next() {
		var row ports.PreAcceptanceFinancialControlPolicyRow
		var status int16
		var endsAt, registeredAt *time.Time
		var jointPass *string
		var itemsJSON []byte
		if err := rows.Scan(
			&row.ObjectID, &row.VersionLabel, &row.Scope, &status,
			&row.EffectiveStartsAt, &endsAt, &row.PublishedAt,
			&jointPass, &registeredAt,
			&itemsJSON,
		); err != nil {
			return nil, fmt.Errorf("list pre-acceptance financial control policies: %w", err)
		}
		statusWord := domain.CommercialVersionStatus(status).String()
		if statusWord == "" {
			return nil, fmt.Errorf("list pre-acceptance financial control policies: 版本状态 %d 不在封闭集内", status)
		}
		row.Status = statusWord
		if endsAt != nil {
			row.EffectiveEndsAt = *endsAt
			row.HasEffectiveEnd = true
		}
		// registered_at 在正文父行上 NOT NULL，它的在场即正文的在场。
		if registeredAt != nil {
			row.HasContent = true
			row.RegisteredAt = *registeredAt
			row.JointPassCondition = *jointPass
		}
		if row.Controls, err = controlItemRowsFromJSON(itemsJSON); err != nil {
			return nil, fmt.Errorf("list pre-acceptance financial control policies: %w", err)
		}
		policyRows = append(policyRows, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list pre-acceptance financial control policies: %w", err)
	}
	return policyRows, nil
}

// controlItemRowsFromJSON 只转写，不校验种类与处置是否在封闭集内——判据同 claimDeadlineRowsFromJSON：
// 目录不重建领域对象，拦坏数据归内容读口。
func controlItemRowsFromJSON(raw []byte) ([]ports.PreAcceptanceControlItemRow, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var documents []controlItemDocument
	if err := json.Unmarshal(raw, &documents); err != nil {
		return nil, fmt.Errorf("control items are not this adapter's shape: %w", err)
	}
	items := make([]ports.PreAcceptanceControlItemRow, 0, len(documents))
	for _, document := range documents {
		items = append(items, ports.PreAcceptanceControlItemRow{
			Kind:               document.Control,
			ChargeScope:        document.ChargeScope,
			EvaluationOrder:    document.Order,
			FailureDisposition: document.OnFailure,
			Responsibility:     document.Responsibility,
		})
	}
	return items, nil
}

// 三个封闭集的名字镜像，default 报错不吸收——库上 CHECK 已经钉死，读回集外取值即库与领域分叉。

func preAcceptanceControlKindFrom(raw string) (domain.PreAcceptanceControlKind, error) {
	for _, kind := range []domain.PreAcceptanceControlKind{
		domain.PrepaidFreezeControl,
		domain.CreditCheckControl,
	} {
		if kind.String() == raw {
			return kind, nil
		}
	}
	return domain.PreAcceptanceControlKindInvalid, fmt.Errorf("unknown pre-acceptance control kind %q", strings.TrimSpace(raw))
}

func controlFailureDispositionFrom(raw string) (domain.ControlFailureDisposition, error) {
	for _, disposition := range []domain.ControlFailureDisposition{
		domain.RejectOnControlFailure,
		domain.AuthorizedDispositionOnControlFailure,
	} {
		if disposition.String() == raw {
			return disposition, nil
		}
	}
	return domain.ControlFailureDispositionInvalid, fmt.Errorf("unknown control failure disposition %q", strings.TrimSpace(raw))
}

func jointPassConditionFrom(raw string) (domain.JointPassCondition, error) {
	if domain.AllControlsPass.String() == raw {
		return domain.AllControlsPass, nil
	}
	return domain.JointPassConditionInvalid, fmt.Errorf("unknown joint pass condition %q", strings.TrimSpace(raw))
}
