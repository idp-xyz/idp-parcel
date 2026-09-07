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

// 合同委派册的持久化面（票 party-commercial-context-gaps/08，ADR-0116，0025 迁移）。写口挂在
// CommercialPublications 上与其余声明同笔登记；读口是独立的 ContractDelegations——委派不进整册装载
// （LoadForScope），理由见 ports.ContractDelegationContentView。
//
// 判读纪律同接受前财务控制策略册（pre_acceptance_financial_control_policy.go）：**先读回既有声明再决定
// 写不写**。声明横跨父子两表，逐行 DO NOTHING 会把半份新声明并进旧声明——冲突路径必须一行不写，
// 否则「绝不覆盖」只对父行成立。

// SaveContractDelegations 登记一个客户合同版本声明的全部委派。撞键不覆盖：同内容是重放，异内容（任一
// 条的委派方、区间不同，或多一条少一条）是需要商业责任方在新合同版本里修正的冲突。
func (repository *CommercialPublications) SaveContractDelegations(
	ctx context.Context,
	content domain.ContractDelegationContent,
) (ports.DeclarationSaveOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.DeclarationSaveOutcomeInvalid, fmt.Errorf("save contract delegations: %w", err)
	}

	incoming := delegationRowsOf(content)
	if len(incoming) == 0 {
		// 零值声明过不了 NewContractDelegationContent，走到这里只可能是绕开构造门的零值。拦在 INSERT 前，
		// 否则外键会以一条技术错误报出一件领域上早该拒绝的事。
		return ports.DeclarationSaveOutcomeInvalid,
			fmt.Errorf("save contract delegations: %w", domain.ErrInvalidContractDelegation)
	}
	tenant, kind, objectID, label := ownerColumns(content.Contract())

	present, err := parentRowPresent(ctx, executor,
		`SELECT 1
		   FROM party_commercial.contract_delegation_content
		  WHERE tenant_id = $1 AND object_kind = $2 AND object_id = $3 AND version_label = $4`,
		tenant, kind, objectID, label)
	if err != nil {
		return ports.DeclarationSaveOutcomeInvalid, fmt.Errorf("save contract delegations: %w", err)
	}
	if present {
		existing, err := scanDelegationRows(ctx, executor, tenant, kind, objectID, label)
		if err != nil {
			return ports.DeclarationSaveOutcomeInvalid, fmt.Errorf("save contract delegations: %w", err)
		}
		if !sameDelegationRows(existing, incoming) {
			return ports.DeclarationContentConflict, nil
		}
		return ports.DeclarationAlreadyRegistered, nil
	}

	if _, err := executor.Exec(ctx,
		`INSERT INTO party_commercial.contract_delegation_content
			(tenant_id, object_kind, object_id, version_label)
		 VALUES ($1, $2, $3, $4)`,
		tenant, kind, objectID, label,
	); err != nil {
		return ports.DeclarationSaveOutcomeInvalid, fmt.Errorf("save contract delegations: %w", err)
	}
	for _, delegation := range content.Delegations() {
		var endsAt *time.Time
		if end, bounded := delegation.Effective().EndsAt(); bounded {
			utc := end.UTC()
			endsAt = &utc
		}
		if _, err := executor.Exec(ctx,
			`INSERT INTO party_commercial.contract_delegation
				(tenant_id, object_kind, object_id, version_label,
				 action, scope_ref, authority_level,
				 delegator_kind, delegator_ref, effective_starts_at, effective_ends_at)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
			tenant, kind, objectID, label,
			delegation.Action().String(), delegation.Scope().String(), delegation.Level().String(),
			delegation.Delegator().Kind().String(), delegation.Delegator().Reference(),
			delegation.Effective().StartsAt().UTC(), endsAt,
		); err != nil {
			return ports.DeclarationSaveOutcomeInvalid, fmt.Errorf("save contract delegations: %w", err)
		}
	}
	return ports.DeclarationSaved, nil
}

// scannedDelegation 是一条委派的比对形。键是（动作 + 范围 + 等级），与库上主键同构；委派方与区间是
// 比对的内容——任一不同都是另一份声明。
type scannedDelegation struct {
	delegatorKind string
	delegatorRef  string
	startsAt      time.Time
	endsAt        *time.Time
}

func (row scannedDelegation) same(other scannedDelegation) bool {
	sameEnds := (row.endsAt == nil && other.endsAt == nil) ||
		(row.endsAt != nil && other.endsAt != nil && row.endsAt.Equal(*other.endsAt))
	return row.delegatorKind == other.delegatorKind &&
		row.delegatorRef == other.delegatorRef &&
		row.startsAt.Equal(other.startsAt) &&
		sameEnds
}

func delegationRowKey(action, scope, level string) string {
	return action + "\x1f" + scope + "\x1f" + level
}

func delegationRowsOf(content domain.ContractDelegationContent) map[string]scannedDelegation {
	rows := make(map[string]scannedDelegation)
	for _, delegation := range content.Delegations() {
		row := scannedDelegation{
			delegatorKind: delegation.Delegator().Kind().String(),
			delegatorRef:  delegation.Delegator().Reference(),
			startsAt:      delegation.Effective().StartsAt().UTC(),
		}
		if end, bounded := delegation.Effective().EndsAt(); bounded {
			utc := end.UTC()
			row.endsAt = &utc
		}
		rows[delegationRowKey(delegation.Action().String(), delegation.Scope().String(), delegation.Level().String())] = row
	}
	return rows
}

// sameDelegationRows 按键比对两份委派集合；声明顺序不构成不同的内容。
func sameDelegationRows(left, right map[string]scannedDelegation) bool {
	if len(left) != len(right) {
		return false
	}
	for key, row := range left {
		other, present := right[key]
		if !present || !row.same(other) {
			return false
		}
	}
	return true
}

func scanDelegationRows(
	ctx context.Context,
	executor bentopg.Executor,
	tenant string, kind uint8, objectID, label string,
) (map[string]scannedDelegation, error) {
	rows := make(map[string]scannedDelegation)
	result, err := executor.Query(ctx,
		`SELECT action, scope_ref, authority_level, delegator_kind, delegator_ref, effective_starts_at, effective_ends_at
		   FROM party_commercial.contract_delegation
		  WHERE tenant_id = $1 AND object_kind = $2 AND object_id = $3 AND version_label = $4`,
		tenant, kind, objectID, label)
	if err != nil {
		return rows, err
	}
	defer result.Close()
	for result.Next() {
		var action, scope, level string
		var row scannedDelegation
		if err := result.Scan(&action, &scope, &level, &row.delegatorKind, &row.delegatorRef, &row.startsAt, &row.endsAt); err != nil {
			return rows, err
		}
		rows[delegationRowKey(action, scope, level)] = row
	}
	return rows, result.Err()
}

// ContractDelegations 实现 ports.ContractDelegationContentView 与 ports.EffectiveContractDelegationView：
// 按已唯一选出的合同版本点读它的全部委派，或按（租户+范围+时点）装载当时有效的委派供裁定解实际决定方。
//
// 只读。委派属实例半边，本适配器不提供写口（写口在 CommercialPublications 上随发布同笔），也不在读不到
// 时代拟任何委派——缺行就是没委派，Authorize 据以答 ErrDelegationAbsent。
type ContractDelegations struct {
	db *bentopg.DB
}

func NewContractDelegations(db *bentopg.DB) (*ContractDelegations, error) {
	if db == nil {
		return nil, fmt.Errorf("party commercial postgres: db is nil")
	}
	return &ContractDelegations{db: db}, nil
}

var _ ports.ContractDelegationContentView = (*ContractDelegations)(nil)
var _ ports.EffectiveContractDelegationView = (*ContractDelegations)(nil)

// LoadContractDelegations 取回一个合同版本声明的全部委派。
//
// found=false = 未声明（无父行）。显式租户与版本必须同一身份，否则 error 且不交内容（ADR-0003/0040，租户
// 是身份不是过滤器）。父行与子表由一条语句取回：ReadExecutor 不保证多条语句同一快照，分两次会拼出从未
// 同时存在的父子状态。读回的委派过 NewContractDelegationContent 重建——有父无子在那里被拒，上抛不折成
// 未声明。
func (repository *ContractDelegations) LoadContractDelegations(
	ctx context.Context,
	tenant domain.TenantID,
	contract domain.CommercialVersion,
) (domain.ContractDelegationContent, bool, error) {
	none := domain.ContractDelegationContent{}
	if tenant.String() == "" ||
		contract.ObjectID().String() == "" || contract.Version().String() == "" {
		return none, false, fmt.Errorf("load contract delegations: tenant and contract identity are required")
	}
	if tenant != contract.Tenant() {
		return none, false, fmt.Errorf("load contract delegations: tenant does not own this contract")
	}
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return none, false, fmt.Errorf("load contract delegations: %w", err)
	}

	var rowsJSON []byte
	err = querier.QueryRow(ctx,
		`SELECT (SELECT COALESCE(
		                    json_agg(
		                        json_build_object(
		                            'action',        child.action,
		                            'scope',         child.scope_ref,
		                            'level',         child.authority_level,
		                            'delegatorKind', child.delegator_kind,
		                            'delegator',     child.delegator_ref,
		                            'startsAt',      child.effective_starts_at,
		                            'endsAt',        child.effective_ends_at
		                        )
		                        ORDER BY child.action, child.scope_ref, child.authority_level
		                    ),
		                    '[]'::json
		                )
		           FROM party_commercial.contract_delegation AS child
		          WHERE child.tenant_id     = parent.tenant_id
		            AND child.object_kind   = parent.object_kind
		            AND child.object_id     = parent.object_id
		            AND child.version_label = parent.version_label)
		   FROM party_commercial.contract_delegation_content AS parent
		  WHERE parent.tenant_id     = $1
		    AND parent.object_kind   = $2
		    AND parent.object_id     = $3
		    AND parent.version_label = $4`,
		tenant.String(),
		uint8(domain.CustomerContractObject),
		contract.ObjectID().String(),
		contract.Version().String(),
	).Scan(&rowsJSON)
	if errors.Is(err, pgx.ErrNoRows) {
		return none, false, nil
	}
	if err != nil {
		return none, false, fmt.Errorf("load contract delegations: %w", err)
	}

	declarations, err := delegationDeclarationsFromJSON(rowsJSON)
	if err != nil {
		return none, false, fmt.Errorf("load contract delegations: %w", err)
	}
	content, err := domain.NewContractDelegationContent(contract, declarations)
	if err != nil {
		return none, false, fmt.Errorf("load contract delegations: %w", err)
	}
	return content, true, nil
}

// LoadEffectiveDelegations 取回该租户该范围在业务时点仍有效的全部委派，各带自己的合同版本。
//
// 空切片不是错误：交给 Authorize 译 ErrDelegationAbsent。合同版本从 commercial_version 的快照重建（0025
// 的外键保证它在），委派行只存版本键不存第二份快照——版本的重建来源只信一处（判据同 versionDocument
// 的头注）。顺序按合同、再按（动作、范围、等级），让装载结果稳定可比。
func (repository *ContractDelegations) LoadEffectiveDelegations(
	ctx context.Context,
	tenant domain.TenantID,
	scope domain.CommercialScopeReference,
	at time.Time,
) ([]domain.ContractDelegation, error) {
	if tenant.String() == "" || scope.String() == "" || at.IsZero() {
		return nil, fmt.Errorf("load effective delegations: tenant, scope and as-of time are required")
	}
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("load effective delegations: %w", err)
	}

	rows, err := querier.Query(ctx,
		`SELECT version.snapshot,
		        child.action, child.scope_ref, child.authority_level,
		        child.delegator_kind, child.delegator_ref,
		        child.effective_starts_at, child.effective_ends_at
		   FROM party_commercial.contract_delegation AS child
		   JOIN party_commercial.commercial_version AS version
		     ON version.tenant_id     = child.tenant_id
		    AND version.object_kind   = child.object_kind
		    AND version.object_id     = child.object_id
		    AND version.version_label = child.version_label
		  WHERE child.tenant_id = $1
		    AND child.scope_ref = $2
		    AND child.effective_starts_at <= $3
		    AND (child.effective_ends_at IS NULL OR child.effective_ends_at > $3)
		  ORDER BY child.object_id, child.version_label, child.action, child.scope_ref, child.authority_level`,
		tenant.String(),
		scope.String(),
		at.UTC(),
	)
	if err != nil {
		return nil, fmt.Errorf("load effective delegations: %w", err)
	}
	defer rows.Close()

	var delegations []domain.ContractDelegation
	for rows.Next() {
		var snapshot []byte
		var document delegationDocument
		if err := rows.Scan(
			&snapshot,
			&document.Action, &document.Scope, &document.Level,
			&document.DelegatorKind, &document.Delegator,
			&document.StartsAt, &document.EndsAt,
		); err != nil {
			return nil, fmt.Errorf("load effective delegations: %w", err)
		}
		var versionDoc versionDocument
		if err := json.Unmarshal(snapshot, &versionDoc); err != nil {
			return nil, fmt.Errorf("load effective delegations: 快照不是本适配器写下的形状：%w", err)
		}
		contract, err := versionDoc.version()
		if err != nil {
			return nil, fmt.Errorf("load effective delegations: %w", err)
		}
		declaration, err := document.declaration()
		if err != nil {
			return nil, fmt.Errorf("load effective delegations: %w", err)
		}
		delegation, err := domain.NewContractDelegation(
			contract, declaration.Delegator, declaration.Action, declaration.Scope, declaration.Level, declaration.Effective)
		if err != nil {
			return nil, fmt.Errorf("load effective delegations: %w", err)
		}
		delegations = append(delegations, delegation)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("load effective delegations: %w", err)
	}
	return delegations, nil
}

// delegationDocument 是子表一行在 json_agg 里的形状，也是按范围装载时逐列扫进的形状——两条路读的是同一
// 份领域对象的同一种表示，键名与受控 CLI 批文的委派一节同名，但各自独立解码、互不复用结构。
type delegationDocument struct {
	Action        string     `json:"action"`
	Scope         string     `json:"scope"`
	Level         string     `json:"level"`
	DelegatorKind string     `json:"delegatorKind"`
	Delegator     string     `json:"delegator"`
	StartsAt      time.Time  `json:"startsAt"`
	EndsAt        *time.Time `json:"endsAt"`
}

func (document delegationDocument) declaration() (domain.ContractDelegationDeclaration, error) {
	none := domain.ContractDelegationDeclaration{}
	action, err := authorizedActionFrom(document.Action)
	if err != nil {
		return none, err
	}
	scope, err := domain.NewCommercialScopeReference(document.Scope)
	if err != nil {
		return none, err
	}
	level, err := domain.NewAuthorityLevel(document.Level)
	if err != nil {
		return none, err
	}
	delegator, err := delegatorFrom(document.DelegatorKind, document.Delegator)
	if err != nil {
		return none, err
	}
	endsAt := time.Time{}
	if document.EndsAt != nil {
		endsAt = *document.EndsAt
	}
	effective, err := domain.NewEffectiveInterval(document.StartsAt, endsAt)
	if err != nil {
		return none, err
	}
	return domain.ContractDelegationDeclaration{
		Delegator: delegator,
		Action:    action,
		Scope:     scope,
		Level:     level,
		Effective: effective,
	}, nil
}

func delegationDeclarationsFromJSON(raw []byte) ([]domain.ContractDelegationDeclaration, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var documents []delegationDocument
	if err := json.Unmarshal(raw, &documents); err != nil {
		return nil, fmt.Errorf("delegations are not this adapter's shape: %w", err)
	}
	declarations := make([]domain.ContractDelegationDeclaration, 0, len(documents))
	for _, document := range documents {
		declaration, err := document.declaration()
		if err != nil {
			return nil, err
		}
		declarations = append(declarations, declaration)
	}
	return declarations, nil
}

// delegatorFrom 是 domain.DelegatorKind 的名字镜像，default 报错不吸收——库上 CHECK 已经钉死两值，读回
// 集外取值即库与领域分叉。
func delegatorFrom(kind, reference string) (domain.Delegator, error) {
	switch kind {
	case domain.CustomerAccountDelegator.String():
		account, err := domain.NewCustomerAccountID(reference)
		if err != nil {
			return domain.Delegator{}, err
		}
		return domain.DelegatedByCustomerAccount(account)
	case domain.LegalEntityDelegator.String():
		entity, err := domain.NewLegalEntityReference(reference)
		if err != nil {
			return domain.Delegator{}, err
		}
		return domain.DelegatedByLegalEntity(entity)
	default:
		return domain.Delegator{}, fmt.Errorf("unknown delegator kind %q", strings.TrimSpace(kind))
	}
}
