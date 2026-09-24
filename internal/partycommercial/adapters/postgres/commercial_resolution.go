package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// CommercialResolutions 实现 ports.CommercialResolutionStore：按标识固定并取回一次解析。
//
// Save 撞键不覆盖：ON CONFLICT DO NOTHING 保事务可用，零行命中后读回摘要——同内容是
// 重放，异内容是冲突。
type CommercialResolutions struct {
	db *bentopg.DB
}

func NewCommercialResolutions(db *bentopg.DB) (*CommercialResolutions, error) {
	if db == nil {
		return nil, fmt.Errorf("party commercial postgres: db is nil")
	}
	return &CommercialResolutions{db: db}, nil
}

var _ ports.CommercialResolutionStore = (*CommercialResolutions)(nil)

func (repository *CommercialResolutions) LoadResolution(
	ctx context.Context,
	tenant domain.TenantID,
	resolution domain.ResolutionID,
) (domain.CommercialClosure, bool, error) {
	if tenant.String() == "" || resolution.String() == "" {
		return domain.CommercialClosure{}, false, fmt.Errorf("load commercial resolution: tenant and resolution ID are required")
	}
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return domain.CommercialClosure{}, false, fmt.Errorf("load commercial resolution: %w", err)
	}

	var raw []byte
	err = querier.QueryRow(ctx,
		`SELECT snapshot
		   FROM party_commercial.commercial_resolution
		  WHERE tenant_id = $1 AND resolution_id = $2`,
		tenant.String(),
		resolution.String(),
	).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.CommercialClosure{}, false, nil
	}
	if err != nil {
		return domain.CommercialClosure{}, false, fmt.Errorf("load commercial resolution: %w", err)
	}

	var document closureDocument
	if err := json.Unmarshal(raw, &document); err != nil {
		return domain.CommercialClosure{}, false, fmt.Errorf("load commercial resolution: 快照不是本适配器写下的形状：%w", err)
	}
	closure, err := document.closure()
	if err != nil {
		return domain.CommercialClosure{}, false, fmt.Errorf("load commercial resolution: %w", err)
	}
	return closure, true, nil
}

func (repository *CommercialResolutions) Save(
	ctx context.Context,
	closure domain.CommercialClosure,
) (ports.ResolutionSaveOutcome, error) {
	if closure.ResolutionID().String() == "" || closure.Outcome() != domain.UniquelyResolved {
		return ports.ResolutionSaveOutcomeInvalid, fmt.Errorf("save commercial resolution: uniquely resolved closure with ID is required")
	}
	key := closure.ResolutionKey()
	if key.TenantID.String() == "" {
		return ports.ResolutionSaveOutcomeInvalid, fmt.Errorf("save commercial resolution: tenant is required")
	}

	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.ResolutionSaveOutcomeInvalid, fmt.Errorf("save commercial resolution: %w", err)
	}

	document := documentOfClosure(closure)
	raw, err := json.Marshal(document)
	if err != nil {
		return ports.ResolutionSaveOutcomeInvalid, fmt.Errorf("save commercial resolution: %w", err)
	}
	digest := sha256.Sum256(raw)
	contentDigest := hex.EncodeToString(digest[:])

	tag, err := executor.Exec(ctx,
		`INSERT INTO party_commercial.commercial_resolution
			(tenant_id, resolution_id, customer_account_id, outcome, content_digest, snapshot)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 ON CONFLICT DO NOTHING`,
		key.TenantID.String(),
		closure.ResolutionID().String(),
		key.CustomerAccountID.String(),
		uint8(closure.Outcome()),
		contentDigest,
		raw,
	)
	if err != nil {
		return ports.ResolutionSaveOutcomeInvalid, fmt.Errorf("save commercial resolution: %w", err)
	}
	if tag.RowsAffected() > 0 {
		return ports.ResolutionSaved, nil
	}

	var existingDigest string
	err = executor.QueryRow(ctx,
		`SELECT content_digest
		   FROM party_commercial.commercial_resolution
		  WHERE tenant_id = $1 AND resolution_id = $2`,
		key.TenantID.String(),
		closure.ResolutionID().String(),
	).Scan(&existingDigest)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.ResolutionSaveOutcomeInvalid, fmt.Errorf("save commercial resolution: 撞键后读不回既有行")
	}
	if err != nil {
		return ports.ResolutionSaveOutcomeInvalid, fmt.Errorf("save commercial resolution: %w", err)
	}
	if existingDigest == contentDigest {
		return ports.ResolutionAlreadyRecorded, nil
	}
	return ports.ResolutionContentConflict, nil
}

type closureDocument struct {
	Outcome      uint8     `json:"outcome"`
	ResolutionID string    `json:"resolutionId"`
	TenantID     string    `json:"tenantId"`
	Customer     string    `json:"customerAccountId"`
	LegalEntity  string    `json:"legalEntity"`
	Scope        string    `json:"scope"`
	Purpose      uint8     `json:"purpose"`
	Direction    uint8     `json:"priceDirection"`
	AnchorAt     time.Time `json:"anchorAt"`
	AnchorPolicy string    `json:"anchorPolicy"`
	// Settlement 只在必需依据含结算政策时出现（ADR-0044 的「含则必填、不含则必缺」）。
	// 它必须原样写下来：解析键的最小身份把选择器算在内，落库时丢掉，读回的键就立不起来，
	// 于是一份写得进去的闭包永远读不回来——写成功、读失败，两侧都不报错。
	Settlement *settlementSelectorDocument `json:"settlement,omitempty"`
	// Credit 只在必需依据含信用政策时出现（ADR-0127，同一条「含则必填、不含则必缺」）；不写
	// 下来读回的键同样立不起来。
	Credit *creditSelectorDocument `json:"credit,omitempty"`
	// ServiceProduct 是键上委托声明的服务产品（票 psb/17），只在声明时出现。丢了它，提交前重解按范围重跑，同一范围
	// 两个产品即冲突，一份仍然成立的解析被判成已失效。omitempty：不声明的快照逐字节不变，内容摘要随之不变。
	ServiceProduct string            `json:"serviceProduct,omitempty"`
	ViewRevision   string            `json:"viewRevision"`
	Adopted        []adoptedDocument `json:"adopted"`
}

// creditSelectorDocument 两维齐全：信用政策没有由闭包解出的那一维，键上给的就是全部。
type creditSelectorDocument struct {
	Level      string `json:"level"`
	ChargeType string `json:"chargeType"`
}

// settlementSelectorDocument 只有三维：闭包解析键上的选择器按定义不带合同，那一维是本次
// 闭包解出来的结论（ADR-0080）。已采用的那一版合同另有去处——它就在 Adopted 里，还随结算
// 政策正文的适用范围一起写下。这里再写一遍，等于给同一件事留两个可以互相打架的记录。
type settlementSelectorDocument struct {
	Counterparty string `json:"counterparty"`
	ChargeScope  string `json:"chargeScope"`
	Currency     string `json:"currency"`
}

type adoptedDocument struct {
	Kind    uint8           `json:"kind"`
	Version versionDocument `json:"version"`
	// Form 只在采用了服务产品时出现（ADR-0050）。omitempty：既有不含形态的快照
	// 读回仍是缺席，不发明 NETWORK_SERVICE。
	Form string `json:"form,omitempty"`
	// SettlementPolicy 只在采用了结算政策时出现（ADR-0044）。方式与六维适用范围整份留在
	// 快照里，不回登记册按版本重读：登记册那份正文改一次，一次已固定的解析就会改口说自己
	// 当初采用的是别的方式，而快照的全部意义就是不许它改口（ADR-0028）。
	SettlementPolicy *settlementPolicyDocument `json:"settlementPolicy,omitempty"`
	// CreditBasis 只在采用了信用政策时出现（ADR-0127）。额度整份留在快照里，不回登记册按版本
	// 重读，理由与结算政策同一条：正文改一次，已固定的解析就会改口说自己当初授权的是别的额度。
	// 出处不另写——它就是本项的 Version。
	CreditBasis *creditBasisDocument `json:"creditBasis,omitempty"`
}

// creditBasisDocument 镜像 domain.CreditLimit 的两格封闭：恰一在场。两格分列而不是「一个数加
// 一列标记」，与 0020 同一条理由——值落在哪一格本身就是判别式。比例格自 ADR-0129 起带基数一键；
// 缺键的存量快照读回为未声明（重建门如实读回，ADR-0028），不补默认。
type creditBasisDocument struct {
	AmountMinor      *int64  `json:"amountMinor,omitempty"`
	RatioBasisPoints *int64  `json:"ratioBasisPoints,omitempty"`
	RatioBase        *string `json:"ratioBase,omitempty"`
}

type settlementPolicyDocument struct {
	Method       uint8      `json:"method"`
	LegalEntity  string     `json:"legalEntity"`
	Counterparty string     `json:"counterparty"`
	Contract     string     `json:"contract"`
	ChargeScope  string     `json:"chargeScope"`
	Currency     string     `json:"currency"`
	StartsAt     time.Time  `json:"startsAt"`
	EndsAt       *time.Time `json:"endsAt,omitempty"`
}

func documentOfClosure(closure domain.CommercialClosure) closureDocument {
	key := closure.ResolutionKey()
	document := closureDocument{
		Outcome:      uint8(closure.Outcome()),
		ResolutionID: closure.ResolutionID().String(),
		TenantID:     key.TenantID.String(),
		Customer:     key.CustomerAccountID.String(),
		LegalEntity:  key.LegalEntityCandidate.String(),
		Scope:        key.Scope.String(),
		Purpose:      uint8(key.Purpose),
		Direction:    uint8(key.PriceDirection),
		AnchorAt:     key.Anchor.At().UTC(),
		AnchorPolicy: key.Anchor.PolicyVersion().String(),
	}
	if !key.Settlement.Empty() {
		document.Settlement = &settlementSelectorDocument{
			Counterparty: key.Settlement.Counterparty.String(),
			ChargeScope:  key.Settlement.ChargeScope.String(),
			Currency:     key.Settlement.Currency.String(),
		}
	}
	if !key.Credit.Empty() {
		document.Credit = &creditSelectorDocument{
			Level:      key.Credit.Level.String(),
			ChargeType: key.Credit.ChargeType.String(),
		}
	}
	document.ServiceProduct = key.ServiceProduct.String()
	if revision, ok := closure.ViewRevision(); ok {
		document.ViewRevision = revision.String()
	}
	for _, adopted := range closure.Adopted() {
		item := adoptedDocument{
			Kind:    uint8(adopted.Kind()),
			Version: documentOfVersion(adopted.Version()),
		}
		if product, ok := adopted.ServiceProduct(); ok {
			item.Form = product.Form().String()
		}
		if policy, ok := adopted.SettlementPolicy(); ok {
			item.SettlementPolicy = documentOfSettlementPolicy(policy)
		}
		if credit, ok := adopted.CreditBasis(); ok {
			item.CreditBasis = documentOfCreditBasis(credit)
		}
		document.Adopted = append(document.Adopted, item)
	}
	return document
}

func documentOfCreditBasis(basis domain.CreditBasis) *creditBasisDocument {
	minor, bps, ratioBase := creditLimitColumns(basis.AuthorizedLimit())
	return &creditBasisDocument{AmountMinor: minor, RatioBasisPoints: bps, RatioBase: ratioBase}
}

func documentOfSettlementPolicy(policy domain.SettlementPolicy) *settlementPolicyDocument {
	applicability := policy.Applicability()
	document := &settlementPolicyDocument{
		Method:       uint8(policy.Method()),
		LegalEntity:  applicability.LegalEntity().String(),
		Counterparty: applicability.Counterparty().String(),
		Contract:     applicability.Contract().String(),
		ChargeScope:  applicability.ChargeScope().String(),
		Currency:     applicability.Currency().String(),
		StartsAt:     applicability.Effective().StartsAt().UTC(),
	}
	// 开放结束是合法形状（一份政策可以适用到被替代为止），所以终止时刻缺席与零值必须
	// 分开写：写成零值读回时 NewEffectiveInterval 会把它当成有界区间的坏边界而整份拒掉。
	if end, bounded := applicability.Effective().EndsAt(); bounded {
		utc := end.UTC()
		document.EndsAt = &utc
	}
	return document
}

func (document closureDocument) closure() (domain.CommercialClosure, error) {
	resolutionID, err := domain.NewResolutionID(document.ResolutionID)
	if err != nil {
		return domain.CommercialClosure{}, err
	}
	anchorPolicy, err := domain.NewAnchorPolicyVersion(document.AnchorPolicy)
	if err != nil {
		return domain.CommercialClosure{}, err
	}
	anchor, err := domain.NewSelectionAnchor(document.AnchorAt, anchorPolicy)
	if err != nil {
		return domain.CommercialClosure{}, err
	}
	viewRevision, err := domain.NewAuthorityViewRevision(document.ViewRevision)
	if err != nil {
		return domain.CommercialClosure{}, err
	}

	key := domain.ClosureResolutionKey{
		Purpose:        domain.ResolutionPurpose(document.Purpose),
		PriceDirection: domain.PriceDirection(document.Direction),
		Anchor:         anchor,
	}
	if key.TenantID, err = domain.NewTenantID(document.TenantID); err != nil {
		return domain.CommercialClosure{}, err
	}
	if key.CustomerAccountID, err = domain.NewCustomerAccountID(document.Customer); err != nil {
		return domain.CommercialClosure{}, err
	}
	if key.LegalEntityCandidate, err = domain.NewLegalEntityReference(document.LegalEntity); err != nil {
		return domain.CommercialClosure{}, err
	}
	if key.Scope, err = domain.NewCommercialScopeReference(document.Scope); err != nil {
		return domain.CommercialClosure{}, err
	}
	if document.Settlement != nil {
		if key.Settlement, err = document.Settlement.selector(); err != nil {
			return domain.CommercialClosure{}, err
		}
	}
	if document.Credit != nil {
		if key.Credit, err = document.Credit.selector(); err != nil {
			return domain.CommercialClosure{}, err
		}
	}
	if document.ServiceProduct != "" {
		if key.ServiceProduct, err = domain.NewCommercialObjectID(document.ServiceProduct); err != nil {
			return domain.CommercialClosure{}, err
		}
	}

	adopted := make([]domain.RehydrateAdoptedBasisSpec, 0, len(document.Adopted))
	bases := make([]domain.CommercialObjectKind, 0, len(document.Adopted))
	for _, item := range document.Adopted {
		version, err := item.Version.version()
		if err != nil {
			return domain.CommercialClosure{}, err
		}
		kind := domain.CommercialObjectKind(item.Kind)
		spec := domain.RehydrateAdoptedBasisSpec{Kind: kind, Version: version}
		if item.Form != "" {
			product, err := rehydrateServiceProduct(version, item.Form)
			if err != nil {
				return domain.CommercialClosure{}, err
			}
			spec.ServiceProduct = product
			spec.HasServiceProduct = true
		}
		if item.SettlementPolicy != nil {
			policy, err := item.SettlementPolicy.policy(version)
			if err != nil {
				return domain.CommercialClosure{}, err
			}
			spec.SettlementPolicy = policy
			spec.HasSettlementPolicy = true
		}
		if item.CreditBasis != nil {
			// 两格折回领域；两空 / 两满是本适配器绝不会写出的形状，报错不吸收。比例格走重建门：ADR-0129 之前
			// 固定的快照没有基数键，如实读回为未声明——快照的全部意义就是不许它改口，这里不替它补一个基数。
			limit, err := creditLimitFrom(item.CreditBasis.AmountMinor, item.CreditBasis.RatioBasisPoints, item.CreditBasis.RatioBase)
			if err != nil {
				return domain.CommercialClosure{}, fmt.Errorf("load commercial resolution: %w", err)
			}
			spec.CreditLimit = limit
			spec.HasCreditBasis = true
		}
		adopted = append(adopted, spec)
		bases = append(bases, kind)
	}
	key.RequiredBases = bases

	return domain.RehydrateCommercialClosure(domain.RehydrateCommercialClosureSpec{
		Outcome:      domain.ResolutionOutcome(document.Outcome),
		ResolutionID: resolutionID,
		Key:          key,
		Anchor:       anchor,
		ViewRevision: viewRevision,
		Adopted:      adopted,
	})
}

func (document settlementSelectorDocument) selector() (domain.SettlementSelector, error) {
	var selector domain.SettlementSelector
	var err error
	if selector.Counterparty, err = domain.NewCounterpartyReference(document.Counterparty); err != nil {
		return domain.SettlementSelector{}, fmt.Errorf("load commercial resolution: %w", err)
	}
	if selector.ChargeScope, err = domain.NewChargeScopeReference(document.ChargeScope); err != nil {
		return domain.SettlementSelector{}, fmt.Errorf("load commercial resolution: %w", err)
	}
	if selector.Currency, err = domain.NewCurrencyCode(document.Currency); err != nil {
		return domain.SettlementSelector{}, fmt.Errorf("load commercial resolution: %w", err)
	}
	return selector, nil
}

func (document creditSelectorDocument) selector() (domain.CreditSelector, error) {
	var selector domain.CreditSelector
	var err error
	if selector.Level, err = domain.NewAuthorityLevel(document.Level); err != nil {
		return domain.CreditSelector{}, fmt.Errorf("load commercial resolution: %w", err)
	}
	if selector.ChargeType, err = domain.NewChargeTypeReference(document.ChargeType); err != nil {
		return domain.CreditSelector{}, fmt.Errorf("load commercial resolution: %w", err)
	}
	return selector, nil
}

// policy 把快照里的结算正文译回政策。方式按封闭二值逐格认，未知取值响亮失败：一份读不懂
// 方式的政策若吸收成预付，一个约定了账期的客户会在这里被冻资金（ADR-0044 只留两格，第三
// 格「客户级默认」正是本上下文明禁的那个）。
func (document settlementPolicyDocument) policy(
	version domain.CommercialVersion,
) (domain.SettlementPolicy, error) {
	var method domain.SettlementMethod
	switch document.Method {
	case uint8(domain.PrepaidMethod):
		method = domain.PrepaidMethod
	case uint8(domain.TermsMethod):
		method = domain.TermsMethod
	default:
		return domain.SettlementPolicy{}, fmt.Errorf(
			"load commercial resolution: 无法翻译的结算方式 %d", document.Method)
	}

	legalEntity, err := domain.NewLegalEntityReference(document.LegalEntity)
	if err != nil {
		return domain.SettlementPolicy{}, fmt.Errorf("load commercial resolution: %w", err)
	}
	counterparty, err := domain.NewCounterpartyReference(document.Counterparty)
	if err != nil {
		return domain.SettlementPolicy{}, fmt.Errorf("load commercial resolution: %w", err)
	}
	contract, err := domain.NewCommercialVersionLabel(document.Contract)
	if err != nil {
		return domain.SettlementPolicy{}, fmt.Errorf("load commercial resolution: %w", err)
	}
	chargeScope, err := domain.NewChargeScopeReference(document.ChargeScope)
	if err != nil {
		return domain.SettlementPolicy{}, fmt.Errorf("load commercial resolution: %w", err)
	}
	currency, err := domain.NewCurrencyCode(document.Currency)
	if err != nil {
		return domain.SettlementPolicy{}, fmt.Errorf("load commercial resolution: %w", err)
	}
	endsAt := time.Time{}
	if document.EndsAt != nil {
		endsAt = *document.EndsAt
	}
	effective, err := domain.NewEffectiveInterval(document.StartsAt, endsAt)
	if err != nil {
		return domain.SettlementPolicy{}, fmt.Errorf("load commercial resolution: %w", err)
	}
	applicability, err := domain.NewSettlementApplicability(
		legalEntity, counterparty, contract, chargeScope, currency, effective)
	if err != nil {
		return domain.SettlementPolicy{}, fmt.Errorf("load commercial resolution: %w", err)
	}
	policy, err := domain.NewSettlementPolicy(version, method, applicability)
	if err != nil {
		return domain.SettlementPolicy{}, fmt.Errorf("load commercial resolution: %w", err)
	}
	return policy, nil
}

// rehydrateServiceProduct 把快照里的形态字符串译回产品。未知取值响亮失败，不吸收成
// NETWORK_SERVICE——那正是 ADR-0050 要堵的默认值。
func rehydrateServiceProduct(version domain.CommercialVersion, form string) (domain.ServiceProduct, error) {
	var parsed domain.ServiceProductForm
	switch form {
	case domain.NetworkServiceForm.String():
		parsed = domain.NetworkServiceForm
	case domain.LabelChannelServiceForm.String():
		parsed = domain.LabelChannelServiceForm
	default:
		return domain.ServiceProduct{}, fmt.Errorf("load commercial resolution: 无法翻译的服务形态 %q", form)
	}
	product, err := domain.NewServiceProduct(version, parsed)
	if err != nil {
		return domain.ServiceProduct{}, fmt.Errorf("load commercial resolution: %w", err)
	}
	return product, nil
}
