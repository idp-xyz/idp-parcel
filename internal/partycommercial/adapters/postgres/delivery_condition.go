package postgres

import (
	"context"
	"fmt"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// 本文件是交付条件声明族（0030，ADR-0133 决定四，票 party-commercial-context-gaps/11）在持久化面的两半：
// 按回指答「有没有」的读口挂在 DeliveryConditions，写口挂在 CommercialPublications（与其余具名 Save 同居）。
// 单独成文件是让这一族的读写在一处看全，也让并行的会话少碰共享文件（source_data_amendment.go 同一句）。

// DeliveryConditions 实现 ports.DeliveryConditionView：按（租户，商业解析回指）答交付条件引用 / 没有 / error。
//
// 路是 ADR-0133 决定一钉死的那条：回指 → 闭包（CommercialResolutions.LoadResolution）→ 闭包已采用的服务产品版本
// 与客户合同版本 → 各读一层声明。四格怎么分由 domain.DeliveryConditionReferenceFor 一处算，这里只负责把两层「在不在」
// 读出来；每一层在场时都经领域构造门重建——坏行走 error，不折成「没有」。只读；正文属实例半边，写口在下面。
type DeliveryConditions struct {
	db          *bentopg.DB
	resolutions *CommercialResolutions
}

func NewDeliveryConditions(db *bentopg.DB) (*DeliveryConditions, error) {
	resolutions, err := NewCommercialResolutions(db)
	if err != nil {
		return nil, err
	}
	return &DeliveryConditions{db: db, resolutions: resolutions}, nil
}

var _ ports.DeliveryConditionView = (*DeliveryConditions)(nil)

func (repository *DeliveryConditions) LoadDeliveryConditionReference(
	ctx context.Context,
	tenant domain.TenantID,
	resolution domain.ResolutionID,
) (domain.DeliveryConditionReference, bool, error) {
	none := domain.DeliveryConditionReference{}
	closure, found, err := repository.resolutions.LoadResolution(ctx, tenant, resolution)
	if err != nil {
		return none, false, fmt.Errorf("load delivery condition reference: %w", err)
	}
	if !found {
		return none, false, fmt.Errorf("load delivery condition reference: %w", domain.ErrDeliveryConditionClosureAbsent)
	}
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return none, false, fmt.Errorf("load delivery condition reference: %w", err)
	}

	productDeclared := false
	if adopted, ok := closure.AdoptedFor(domain.ServiceProductObject); ok {
		if _, productDeclared, err = loadDeliveryConditionLayer(ctx, querier, tenant, adopted.Version()); err != nil {
			return none, false, fmt.Errorf("load delivery condition reference: product layer: %w", err)
		}
	}
	contractDeclared := false
	if adopted, ok := closure.AdoptedFor(domain.CustomerContractObject); ok {
		if _, contractDeclared, err = loadDeliveryConditionLayer(ctx, querier, tenant, adopted.Version()); err != nil {
			return none, false, fmt.Errorf("load delivery condition reference: contract layer: %w", err)
		}
	}

	reference, found, err := domain.DeliveryConditionReferenceFor(closure, productDeclared, contractDeclared)
	if err != nil {
		return none, false, fmt.Errorf("load delivery condition reference: %w", err)
	}
	return reference, found, nil
}

// deliveryConditionRow 是父行的字面：合同层的所收紧产品版本两列可空，两条规则引用必填。
type deliveryConditionRow struct {
	tightensObjectID, tightensVersion *string
	recipientScopeRule, proofRule     string
}

// loadDeliveryConditionLayer 按拥有版本读回一层声明。无父行 = 这一层没有声明（found=false）；父行在场即按层走
// 领域构造门重建（产品层 DeclareProductDeliveryConditions / 合同层 DeclareContractDeliveryConditions），零方式、
// 空引用、层与所收紧两列对不上都是坏声明，走 error。父 LEFT JOIN 子一次取回；合同层读回**不重核**收紧——登记时
// 核过的那份就是它，闭包日后采用另一版产品也不改已登记的合同层（ADR-0133 决定二的同一立场）。
func loadDeliveryConditionLayer(
	ctx context.Context,
	querier bentopg.Querier,
	tenant domain.TenantID,
	owner domain.CommercialVersion,
) (domain.DeliveryConditionContent, bool, error) {
	none := domain.DeliveryConditionContent{}
	if err := requireOwnedVersion(tenant, owner, "load delivery conditions"); err != nil {
		return none, false, err
	}
	rows, err := querier.Query(ctx,
		`SELECT parent.tightens_object_id,
		        parent.tightens_version_label,
		        parent.recipient_scope_rule_ref,
		        parent.proof_of_delivery_rule_ref,
		        child.method_ref
		   FROM party_commercial.delivery_condition AS parent
		   LEFT JOIN party_commercial.delivery_condition_method AS child
		          ON child.tenant_id     = parent.tenant_id
		         AND child.object_kind   = parent.object_kind
		         AND child.object_id     = parent.object_id
		         AND child.version_label = parent.version_label
		  WHERE parent.tenant_id     = $1
		    AND parent.object_kind   = $2
		    AND parent.object_id     = $3
		    AND parent.version_label = $4
		  ORDER BY child.method_ref`,
		tenant.String(), uint8(owner.Kind()), owner.ObjectID().String(), owner.Version().String(),
	)
	if err != nil {
		return none, false, fmt.Errorf("load delivery conditions: %w", err)
	}
	defer rows.Close()

	present := false
	var parent deliveryConditionRow
	var methods []string
	for rows.Next() {
		var method *string
		if err := rows.Scan(&parent.tightensObjectID, &parent.tightensVersion,
			&parent.recipientScopeRule, &parent.proofRule, &method); err != nil {
			return none, false, fmt.Errorf("load delivery conditions: %w", err)
		}
		present = true
		if method != nil {
			methods = append(methods, *method)
		}
	}
	if err := rows.Err(); err != nil {
		return none, false, fmt.Errorf("load delivery conditions: %w", err)
	}
	if !present {
		return none, false, nil
	}
	content, err := deliveryConditionsFromRows(owner, parent, methods)
	if err != nil {
		return none, false, fmt.Errorf("load delivery conditions: %w", err)
	}
	return content, true, nil
}

// deliveryConditionsFromRows 把父子行的字面译回一层声明并过构造门。产品层带着所收紧两列、合同层缺它们，都是库与
// 领域分叉，由构造门（零值 tightens 拒 / 产品层的门不认 tightens）报出来，这里不吸收。
func deliveryConditionsFromRows(
	owner domain.CommercialVersion,
	parent deliveryConditionRow,
	methods []string,
) (domain.DeliveryConditionContent, error) {
	terms := domain.DeliveryConditionTerms{}
	var err error
	if terms.RecipientScopeRule, err = domain.NewDeliveryRuleReference(parent.recipientScopeRule); err != nil {
		return domain.DeliveryConditionContent{}, err
	}
	if terms.ProofOfDeliveryRule, err = domain.NewDeliveryRuleReference(parent.proofRule); err != nil {
		return domain.DeliveryConditionContent{}, err
	}
	for _, raw := range methods {
		method, err := domain.NewDeliveryMethodReference(raw)
		if err != nil {
			return domain.DeliveryConditionContent{}, err
		}
		terms.Methods = append(terms.Methods, method)
	}
	if owner.Kind() == domain.ServiceProductObject {
		if parent.tightensObjectID != nil || parent.tightensVersion != nil {
			return domain.DeliveryConditionContent{}, fmt.Errorf("%w: product layer row carries a tightening target",
				domain.ErrDeliveryConditionOwner)
		}
		return domain.DeclareProductDeliveryConditions(owner, terms)
	}
	if parent.tightensObjectID == nil || parent.tightensVersion == nil {
		return domain.DeliveryConditionContent{}, fmt.Errorf("%w: contract layer row lacks its tightening target",
			domain.ErrDeliveryConditionNotConfigured)
	}
	objectID, err := domain.NewCommercialObjectID(*parent.tightensObjectID)
	if err != nil {
		return domain.DeliveryConditionContent{}, err
	}
	version, err := domain.NewCommercialVersionLabel(*parent.tightensVersion)
	if err != nil {
		return domain.DeliveryConditionContent{}, err
	}
	tightens, err := domain.NewTightenedProductVersion(objectID, version)
	if err != nil {
		return domain.DeliveryConditionContent{}, err
	}
	return domain.DeclareContractDeliveryConditions(owner, tightens, terms)
}

// SaveDeliveryConditions 登记一层交付条件声明。先读回再判：同拥有版本、同方式集合、同两条规则引用（合同层再加同一个
// 所收紧的产品版本）是重放；任一格不同是内容冲突，冲突一行不写。合同层写前在同一事务里读回它指名的那一版产品层，
// 经 TightensWithin 核「只能在其内收紧」：产品层不在册（本范围册上没有那一版、或那一版没有声明）与放宽都返回 error
// ——那是登记方的输入立不住，与构造门拒件同一类，随事务整项回滚。
//
// 所收紧的产品版本在**合同版本自己的范围册**上找：闭包解析按同一把范围键同时采用产品与合同（ResolveCommercialClosure），
// 合同能收紧的只能是将来会与它同时被采用的那一版产品；跨范围的产品对这份合同的客户不可见，对着它收紧什么也证明不了。
func (repository *CommercialPublications) SaveDeliveryConditions(
	ctx context.Context,
	content domain.DeliveryConditionContent,
) (ports.DeclarationSaveOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.DeclarationSaveOutcomeInvalid, fmt.Errorf("save delivery conditions: %w", err)
	}
	owner := content.Owner()
	tenant, kind, objectID, label := ownerColumns(owner)

	tightens, tightening := content.Tightens()
	if tightening {
		registry, err := repository.LoadForScope(ctx, owner.Tenant(), owner.Scope())
		if err != nil {
			return ports.DeclarationSaveOutcomeInvalid, fmt.Errorf("save delivery conditions: %w", err)
		}
		productVersion, found := registry.Lookup(owner.Tenant(), domain.ServiceProductObject, tightens.ObjectID(), tightens.Version())
		if !found {
			return ports.DeclarationSaveOutcomeInvalid, fmt.Errorf(
				"save delivery conditions: %w: service product %s/%s is not on this scope's register",
				domain.ErrDeliveryConditionTighteningTarget, tightens.ObjectID(), tightens.Version())
		}
		product, found, err := loadDeliveryConditionLayer(ctx, executor, owner.Tenant(), productVersion)
		if err != nil {
			return ports.DeclarationSaveOutcomeInvalid, fmt.Errorf("save delivery conditions: %w", err)
		}
		if !found {
			return ports.DeclarationSaveOutcomeInvalid, fmt.Errorf(
				"save delivery conditions: %w: service product %s/%s has no delivery conditions to tighten within",
				domain.ErrDeliveryConditionWidened, tightens.ObjectID(), tightens.Version())
		}
		if err := content.TightensWithin(product); err != nil {
			return ports.DeclarationSaveOutcomeInvalid, fmt.Errorf("save delivery conditions: %w", err)
		}
	}

	existing, present, err := loadDeliveryConditionLayer(ctx, executor, owner.Tenant(), owner)
	if err != nil {
		return ports.DeclarationSaveOutcomeInvalid, fmt.Errorf("save delivery conditions: %w", err)
	}
	if present {
		if sameDeliveryConditions(existing, content) {
			return ports.DeclarationAlreadyRegistered, nil
		}
		return ports.DeclarationContentConflict, nil
	}

	var tightensObjectID, tightensVersion *string
	if tightening {
		object, version := tightens.ObjectID().String(), tightens.Version().String()
		tightensObjectID, tightensVersion = &object, &version
	}
	if _, err := executor.Exec(ctx,
		`INSERT INTO party_commercial.delivery_condition
			(tenant_id, object_kind, object_id, version_label,
			 tightens_object_id, tightens_version_label,
			 recipient_scope_rule_ref, proof_of_delivery_rule_ref)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		tenant, kind, objectID, label,
		tightensObjectID, tightensVersion,
		content.RecipientScopeRule().String(), content.ProofOfDeliveryRule().String(),
	); err != nil {
		return ports.DeclarationSaveOutcomeInvalid, fmt.Errorf("save delivery conditions: %w", err)
	}
	for _, method := range content.Methods() {
		if _, err := executor.Exec(ctx,
			`INSERT INTO party_commercial.delivery_condition_method
				(tenant_id, object_kind, object_id, version_label, method_ref)
			 VALUES ($1, $2, $3, $4, $5)`,
			tenant, kind, objectID, label, method.String(),
		); err != nil {
			return ports.DeclarationSaveOutcomeInvalid, fmt.Errorf("save delivery conditions: %w", err)
		}
	}
	return ports.DeclarationSaved, nil
}

// sameDeliveryConditions 判两份同拥有版本的声明是不是同一份内容：方式集合（已排序去重）、两条规则引用、所收紧的产品
// 版本逐格相等。方式顺序不构成不同的内容——Methods 交回的是稳定序。
func sameDeliveryConditions(left, right domain.DeliveryConditionContent) bool {
	leftTightens, leftTightening := left.Tightens()
	rightTightens, rightTightening := right.Tightens()
	if leftTightening != rightTightening || leftTightens != rightTightens {
		return false
	}
	if left.RecipientScopeRule() != right.RecipientScopeRule() || left.ProofOfDeliveryRule() != right.ProofOfDeliveryRule() {
		return false
	}
	leftMethods, rightMethods := left.Methods(), right.Methods()
	if len(leftMethods) != len(rightMethods) {
		return false
	}
	for index := range leftMethods {
		if leftMethods[index] != rightMethods[index] {
			return false
		}
	}
	return true
}
