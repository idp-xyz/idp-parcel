package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	pspartycommercial "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/partycommercial"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	pcdomain "go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// CommercialResolutionKeys 实现 pspartycommercial.ResolutionKeySource 的登记面半边
// （syn-wall-door-audit 票 03 件 2）：把（租户+货主客户账户）映射到形成闭包解析键的
// 四项实例参数——商业范围、责任法人候选、选择锚点与必需依据种类。
//
// 无行即显式未配置：FormResolutionKey 交回 formed=false，商业依据适配器据以停在
// `COMMERCIAL_RESOLUTION_KEY_NOT_CONFIGURED`。本适配器不代拟任何一项，尤其不拿系统
// 当前时间顶锚点（AT-PC-018）；锚点时刻与策略版本都来自登记行。
type CommercialResolutionKeys struct {
	db *bentopg.DB
}

func NewCommercialResolutionKeys(db *bentopg.DB) (*CommercialResolutionKeys, error) {
	if db == nil {
		return nil, fmt.Errorf("parcel shipment postgres: db is nil")
	}
	return &CommercialResolutionKeys{db: db}, nil
}

var _ pspartycommercial.ResolutionKeySource = (*CommercialResolutionKeys)(nil)

// ResolutionKeyRegistration 是一行登记的输入。全部字段都是显式实例参数：锚点时刻
// 是登记下来的值，不是任何时钟；必需依据种类由登记方逐项指名。
type ResolutionKeyRegistration struct {
	TenantID          psdomain.TenantID
	CustomerAccountID psdomain.CustomerAccountID
	Scope             pcdomain.CommercialScopeReference
	LegalEntity       pcdomain.LegalEntityReference
	AnchorPolicy      pcdomain.AnchorPolicyVersion
	AnchorAt          time.Time
	RequiredBases     []pcdomain.CommercialObjectKind
}

// ResolutionKeySaveOutcome 是一次登记在持久化面的落点（ADR-0031 同款）：`已登记`是
// 重放，`内容冲突`是同一（租户+客户）被登记成另一套键参数——换范围或换锚点属于换一套
// 解析口径，要以显式新决定处理，不静默覆盖。
type ResolutionKeySaveOutcome uint8

const (
	ResolutionKeySaveOutcomeInvalid ResolutionKeySaveOutcome = iota
	ResolutionKeySaved
	ResolutionKeyAlreadyRegistered
	ResolutionKeyContentConflict
)

func (outcome ResolutionKeySaveOutcome) String() string {
	switch outcome {
	case ResolutionKeySaved:
		return "SAVED"
	case ResolutionKeyAlreadyRegistered:
		return "ALREADY_REGISTERED"
	case ResolutionKeyContentConflict:
		return "CONTENT_CONFLICT"
	default:
		return ""
	}
}

// Register 登记一行解析键参数。先读回既有再决定写不写，冲突路径一行不动——判读纪律
// 与声明写入面相同（见 partycommercial/adapters/postgres 的声明写入注释）。
func (repository *CommercialResolutionKeys) Register(
	ctx context.Context,
	registration ResolutionKeyRegistration,
) (ResolutionKeySaveOutcome, error) {
	if err := registration.validate(); err != nil {
		return ResolutionKeySaveOutcomeInvalid, fmt.Errorf("register resolution key: %w", err)
	}
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ResolutionKeySaveOutcomeInvalid, fmt.Errorf("register resolution key: %w", err)
	}

	incomingBases := make([]string, 0, len(registration.RequiredBases))
	for _, kind := range registration.RequiredBases {
		incomingBases = append(incomingBases, kind.String())
	}

	var existingScope, existingLegal, existingPolicy string
	var existingAnchorAt time.Time
	var existingBases []string
	err = executor.QueryRow(ctx,
		`SELECT scope_ref, legal_entity_ref, anchor_policy_version, anchor_at, required_bases
		   FROM parcel_shipment.commercial_resolution_key_registration
		  WHERE tenant_id = $1 AND customer_account_id = $2`,
		registration.TenantID.String(),
		registration.CustomerAccountID.String(),
	).Scan(&existingScope, &existingLegal, &existingPolicy, &existingAnchorAt, &existingBases)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		if _, err := executor.Exec(ctx,
			`INSERT INTO parcel_shipment.commercial_resolution_key_registration
				(tenant_id, customer_account_id, scope_ref, legal_entity_ref,
				 anchor_policy_version, anchor_at, required_bases)
			 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			registration.TenantID.String(),
			registration.CustomerAccountID.String(),
			registration.Scope.String(),
			registration.LegalEntity.String(),
			registration.AnchorPolicy.String(),
			registration.AnchorAt.UTC(),
			incomingBases,
		); err != nil {
			return ResolutionKeySaveOutcomeInvalid, fmt.Errorf("register resolution key: %w", err)
		}
		return ResolutionKeySaved, nil
	case err != nil:
		return ResolutionKeySaveOutcomeInvalid, fmt.Errorf("register resolution key: %w", err)
	}

	if existingScope != registration.Scope.String() ||
		existingLegal != registration.LegalEntity.String() ||
		existingPolicy != registration.AnchorPolicy.String() ||
		!existingAnchorAt.Equal(registration.AnchorAt.UTC()) ||
		!sameBases(existingBases, incomingBases) {
		return ResolutionKeyContentConflict, nil
	}
	return ResolutionKeyAlreadyRegistered, nil
}

func (registration ResolutionKeyRegistration) validate() error {
	if registration.TenantID.String() == "" ||
		registration.CustomerAccountID.String() == "" ||
		registration.Scope.String() == "" ||
		registration.LegalEntity.String() == "" ||
		registration.AnchorPolicy.String() == "" {
		return fmt.Errorf("每一项键参数都必须显式给出")
	}
	if registration.AnchorAt.IsZero() {
		return fmt.Errorf("锚点时刻必须显式登记，不得留零值待人补默认")
	}
	if len(registration.RequiredBases) == 0 {
		return fmt.Errorf("必需依据种类至少一项")
	}
	seen := make(map[pcdomain.CommercialObjectKind]bool, len(registration.RequiredBases))
	for _, kind := range registration.RequiredBases {
		if kind.String() == "" {
			return fmt.Errorf("必需依据种类含集合外取值")
		}
		// 结算政策与价格规则要求键额外携带选择器/方向（ADR-0044/0034 含则必填），
		// 本登记面没有那些维度——放行等于登记一个永远立不起来的键。库内 CHECK 同拦。
		if kind == pcdomain.SettlementPolicyObject || kind == pcdomain.PriceRuleObject {
			return fmt.Errorf("%s 需要键携带额外选择维度，本登记面不承载", kind)
		}
		if seen[kind] {
			return fmt.Errorf("必需依据种类重复：%s", kind)
		}
		seen[kind] = true
	}
	return nil
}

func sameBases(existing, incoming []string) bool {
	if len(existing) != len(incoming) {
		return false
	}
	set := make(map[string]bool, len(existing))
	for _, base := range existing {
		set[base] = true
	}
	for _, base := range incoming {
		if !set[base] {
			return false
		}
	}
	return true
}

// FormResolutionKey 把一次消费方查询折成闭包解析键。
//
// 目的钉死在`接受控制`：商业依据适配器只服务接受流（其 adoptedResolution 注释），
// 计价目的的键要求价格方向等另一套维度，届时按新决定另开登记面。
//
// 查无行交回 (zero, false, nil)——显式未配置，与读取失败（error）分格：前者等租户
// 登记，后者等依赖恢复，压成一格调用方就不知道该催人还是该重试。
func (repository *CommercialResolutionKeys) FormResolutionKey(
	ctx context.Context,
	query psports.CommercialBasisQuery,
) (pcdomain.ClosureResolutionKey, bool, error) {
	none := pcdomain.ClosureResolutionKey{}
	tenant := query.Identity.TenantID().String()
	customer := query.Identity.CustomerAccountID().String()
	if tenant == "" || customer == "" {
		return none, false, fmt.Errorf("form resolution key: tenant and customer account are required")
	}
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return none, false, fmt.Errorf("form resolution key: %w", err)
	}

	var scopeRef, legalRef, policyVersion string
	var anchorAt time.Time
	var baseNames []string
	err = querier.QueryRow(ctx,
		`SELECT scope_ref, legal_entity_ref, anchor_policy_version, anchor_at, required_bases
		   FROM parcel_shipment.commercial_resolution_key_registration
		  WHERE tenant_id = $1 AND customer_account_id = $2`,
		tenant, customer,
	).Scan(&scopeRef, &legalRef, &policyVersion, &anchorAt, &baseNames)
	if errors.Is(err, pgx.ErrNoRows) {
		return none, false, nil
	}
	if err != nil {
		return none, false, fmt.Errorf("form resolution key: %w", err)
	}

	key := pcdomain.ClosureResolutionKey{Purpose: pcdomain.AcceptanceControlPurpose}
	if key.TenantID, err = pcdomain.NewTenantID(tenant); err != nil {
		return none, false, fmt.Errorf("form resolution key: %w", err)
	}
	if key.CustomerAccountID, err = pcdomain.NewCustomerAccountID(customer); err != nil {
		return none, false, fmt.Errorf("form resolution key: %w", err)
	}
	if key.Scope, err = pcdomain.NewCommercialScopeReference(scopeRef); err != nil {
		return none, false, fmt.Errorf("form resolution key: %w", err)
	}
	if key.LegalEntityCandidate, err = pcdomain.NewLegalEntityReference(legalRef); err != nil {
		return none, false, fmt.Errorf("form resolution key: %w", err)
	}
	anchorPolicy, err := pcdomain.NewAnchorPolicyVersion(policyVersion)
	if err != nil {
		return none, false, fmt.Errorf("form resolution key: %w", err)
	}
	if key.Anchor, err = pcdomain.NewSelectionAnchor(anchorAt, anchorPolicy); err != nil {
		return none, false, fmt.Errorf("form resolution key: %w", err)
	}
	key.RequiredBases = make([]pcdomain.CommercialObjectKind, 0, len(baseNames))
	for _, name := range baseNames {
		kind, err := commercialKindFrom(name)
		if err != nil {
			// 库内 CHECK 与领域封闭集是同一集合的两份镜像，出现集外取值说明两份已经
			// 分叉，那要人来看，不能就地当成没登记。
			return none, false, fmt.Errorf("form resolution key: %w", err)
		}
		key.RequiredBases = append(key.RequiredBases, kind)
	}
	return key, true, nil
}

func commercialKindFrom(name string) (pcdomain.CommercialObjectKind, error) {
	for _, kind := range []pcdomain.CommercialObjectKind{
		pcdomain.ServiceProductObject,
		pcdomain.CustomerContractObject,
		pcdomain.SupplierAgreementObject,
		pcdomain.AcceptanceRulePackageObject,
		pcdomain.PreAcceptanceFinancialControlPolicyObject,
		pcdomain.PriceRuleObject,
		pcdomain.SettlementPolicyObject,
		pcdomain.CreditPolicyObject,
		pcdomain.AuthorizationRuleObject,
	} {
		if kind.String() == name {
			return kind, nil
		}
	}
	return pcdomain.CommercialObjectKindInvalid, fmt.Errorf("unknown commercial object kind %q", name)
}
