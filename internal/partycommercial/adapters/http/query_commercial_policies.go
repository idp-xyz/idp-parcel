package commercialhttp

import (
	"context"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// CommercialPolicyCatalogueReader 是商业策略目录端点消费的读口。
type CommercialPolicyCatalogueReader interface {
	ListAcceptanceRulePackages(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
	) ([]ports.AcceptanceRulePackageRow, error)
	ListPreAcceptanceControls(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
	) ([]ports.PreAcceptanceControlRow, error)
	ListPricePolicies(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
	) ([]ports.PricePolicyRow, error)
	ListSettlementPolicies(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
	) ([]ports.SettlementPolicyRow, error)
	ListAsOfPolicyDeclarations(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
	) ([]ports.AsOfPolicyRow, error)
	ListAuthorizationRules(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
	) ([]ports.AuthorizationRuleRow, error)
	ListCreditPolicies(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
	) ([]ports.CreditPolicyRow, error)
}

// 编译期锁缝:读口形状与端口保持一致。
var _ CommercialPolicyCatalogueReader = ports.CommercialPolicyCatalogueRead(nil)

// outcomeCommercialPoliciesListed 是本端点唯一的业务成格,kind 随响应回显;空册也是
// 这一格(ADR-0077 Decision 四)。
const outcomeCommercialPoliciesListed = "COMMERCIAL_POLICIES_LISTED"

// 策略种类的封闭集(票 master-data-wiring/05:?kind= 分派)。种类命名**册子**而不是
// 商业对象类别:接受前财务控制声明挂在客户合同版本下、时点锚声明挂在接单规则包版本
// 下,拿对象类别当种类名会指错拥有者。没有正文册的对象类别(如客户服务规则版本)不在
// 集合里——预留一格就是替租户拟一种它还没有的册子。
//
// kindAuthorizationRule 与 kindCreditPolicy 的名字**恰好**是商业对象类别,不是上面那条
// 的例外:它拦的是拥有者指错,而这两格上列的对象就是那类版本自己(取消授权目录挂在授权
// 规则下、信用正文挂在信用政策下)——拥有者与被列者同一,与 kindAcceptanceRulePackage 同形。
const (
	kindAcceptanceRulePackage = "ACCEPTANCE_RULE_PACKAGE"
	kindPreAcceptanceControl  = "PRE_ACCEPTANCE_CONTROL"
	kindPricePolicy           = "PRICE_POLICY"
	kindSettlementPolicy      = "SETTLEMENT_POLICY"
	kindAsOfPolicy            = "AS_OF_POLICY"
	kindAuthorizationRule     = "AUTHORIZATION_RULE"
	kindCreditPolicy          = "CREDIT_POLICY"
)

// NewQueryCommercialPoliciesEndpoint 交回商业策略目录查阅的 HTTP 入口
// (GET /commercial-policies?kind=,ADR-0077、票 master-data-wiring/05)。
//
// kind 缺席或集外按坏请求拒:各册子的行形状互不相同,替调用方选一种就是猜。kind
// 的在场与取值属传输形状(与方法检查同级,先于 Intake),读它不构成读业务内容——
// 未配置 Intake 对全部种类同答 403,分支选择不泄露任何东西。
func NewQueryCommercialPoliciesEndpoint(
	intake CommercialCatalogueIntake,
	reader CommercialPolicyCatalogueReader,
) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			response.Header().Set("Allow", http.MethodGet)
			writeProblem(response, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}

		kind := request.URL.Query().Get("kind")
		switch kind {
		case kindAcceptanceRulePackage, kindPreAcceptanceControl,
			kindPricePolicy, kindSettlementPolicy, kindAsOfPolicy,
			kindAuthorizationRule, kindCreditPolicy:
		default:
			writeProblem(response, http.StatusBadRequest, codeMalformedRequest)
			return
		}

		query, err := intake.IntakeCatalogueQuery(request.Context(), request)
		if err != nil {
			writeCatalogueIntakeProblem(response, err)
			return
		}
		tenant := query.Scope.Tenant()

		switch kind {
		case kindAcceptanceRulePackage:
			serveRulePackages(response, request, reader, tenant, query.Limit)
		case kindPreAcceptanceControl:
			servePreAcceptanceControls(response, request, reader, tenant, query.Limit)
		case kindPricePolicy:
			servePricePolicies(response, request, reader, tenant, query.Limit)
		case kindSettlementPolicy:
			serveSettlementPolicies(response, request, reader, tenant, query.Limit)
		case kindAsOfPolicy:
			serveAsOfPolicies(response, request, reader, tenant, query.Limit)
		case kindAuthorizationRule:
			serveAuthorizationRules(response, request, reader, tenant, query.Limit)
		case kindCreditPolicy:
			serveCreditPolicies(response, request, reader, tenant, query.Limit)
		}
	})
}

func serveRulePackages(
	response http.ResponseWriter,
	request *http.Request,
	reader CommercialPolicyCatalogueReader,
	tenant domain.TenantID,
	limit int,
) {
	rows, err := reader.ListAcceptanceRulePackages(request.Context(), tenant, limit)
	if err != nil {
		writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
		return
	}
	bodies := make([]rulePackageBody, 0, len(rows))
	for _, row := range rows {
		bodies = append(bodies, rulePackageBodyOf(row))
	}
	writeJSON(response, http.StatusOK, rulePackageListResponse{
		Outcome:  outcomeCommercialPoliciesListed,
		Kind:     kindAcceptanceRulePackage,
		Policies: bodies,
	})
}

func servePreAcceptanceControls(
	response http.ResponseWriter,
	request *http.Request,
	reader CommercialPolicyCatalogueReader,
	tenant domain.TenantID,
	limit int,
) {
	rows, err := reader.ListPreAcceptanceControls(request.Context(), tenant, limit)
	if err != nil {
		writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
		return
	}
	bodies := make([]preAcceptanceControlBody, 0, len(rows))
	for _, row := range rows {
		body := preAcceptanceControlBody{
			ContractObjectID: row.ContractObjectID,
			ContractVersion:  row.ContractVersion,
			Requirement:      row.Requirement,
			DeclaredAt:       rfc3339(row.DeclaredAt),
		}
		// 依据只在`不适用`时在场,库上 CHECK 与 requirement 绑定,这里如实转写不补。
		body.NotApplicableBasis = row.NotApplicableBasis
		bodies = append(bodies, body)
	}
	writeJSON(response, http.StatusOK, preAcceptanceControlListResponse{
		Outcome:  outcomeCommercialPoliciesListed,
		Kind:     kindPreAcceptanceControl,
		Policies: bodies,
	})
}

func servePricePolicies(
	response http.ResponseWriter,
	request *http.Request,
	reader CommercialPolicyCatalogueReader,
	tenant domain.TenantID,
	limit int,
) {
	rows, err := reader.ListPricePolicies(request.Context(), tenant, limit)
	if err != nil {
		writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
		return
	}
	bodies := make([]pricePolicyBody, 0, len(rows))
	for _, row := range rows {
		bodies = append(bodies, pricePolicyBodyOf(row))
	}
	writeJSON(response, http.StatusOK, pricePolicyListResponse{
		Outcome:  outcomeCommercialPoliciesListed,
		Kind:     kindPricePolicy,
		Policies: bodies,
	})
}

func serveSettlementPolicies(
	response http.ResponseWriter,
	request *http.Request,
	reader CommercialPolicyCatalogueReader,
	tenant domain.TenantID,
	limit int,
) {
	rows, err := reader.ListSettlementPolicies(request.Context(), tenant, limit)
	if err != nil {
		writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
		return
	}
	bodies := make([]settlementPolicyBody, 0, len(rows))
	for _, row := range rows {
		body := settlementPolicyBody{
			ObjectID:          row.ObjectID,
			Version:           row.VersionLabel,
			Method:            row.Method,
			LegalEntity:       row.LegalEntity,
			Counterparty:      row.Counterparty,
			ContractLabel:     row.ContractLabel,
			ChargeScope:       row.ChargeScope,
			Currency:          row.Currency,
			EffectiveStartsAt: rfc3339(row.EffectiveStartsAt),
			RegisteredAt:      rfc3339(row.RegisteredAt),
		}
		if row.HasEffectiveEnd {
			body.EffectiveEndsAt = rfc3339(row.EffectiveEndsAt)
		}
		bodies = append(bodies, body)
	}
	writeJSON(response, http.StatusOK, settlementPolicyListResponse{
		Outcome:  outcomeCommercialPoliciesListed,
		Kind:     kindSettlementPolicy,
		Policies: bodies,
	})
}

func serveAsOfPolicies(
	response http.ResponseWriter,
	request *http.Request,
	reader CommercialPolicyCatalogueReader,
	tenant domain.TenantID,
	limit int,
) {
	rows, err := reader.ListAsOfPolicyDeclarations(request.Context(), tenant, limit)
	if err != nil {
		writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
		return
	}
	bodies := make([]asOfPolicyBody, 0, len(rows))
	for _, row := range rows {
		bodies = append(bodies, asOfPolicyBody{
			RulePackageObjectID: row.RulePackageObjectID,
			RulePackageVersion:  row.RulePackageVersion,
			JudgmentType:        row.JudgmentType,
			SemanticsRef:        row.SemanticsRef,
			PolicyVersion:       row.PolicyVersion,
			DeclaredAt:          rfc3339(row.DeclaredAt),
		})
	}
	writeJSON(response, http.StatusOK, asOfPolicyListResponse{
		Outcome:  outcomeCommercialPoliciesListed,
		Kind:     kindAsOfPolicy,
		Policies: bodies,
	})
}

func serveAuthorizationRules(
	response http.ResponseWriter,
	request *http.Request,
	reader CommercialPolicyCatalogueReader,
	tenant domain.TenantID,
	limit int,
) {
	rows, err := reader.ListAuthorizationRules(request.Context(), tenant, limit)
	if err != nil {
		writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
		return
	}
	bodies := make([]authorizationRuleBody, 0, len(rows))
	for _, row := range rows {
		bodies = append(bodies, authorizationRuleBodyOf(row))
	}
	writeJSON(response, http.StatusOK, authorizationRuleListResponse{
		Outcome:  outcomeCommercialPoliciesListed,
		Kind:     kindAuthorizationRule,
		Policies: bodies,
	})
}

func serveCreditPolicies(
	response http.ResponseWriter,
	request *http.Request,
	reader CommercialPolicyCatalogueReader,
	tenant domain.TenantID,
	limit int,
) {
	rows, err := reader.ListCreditPolicies(request.Context(), tenant, limit)
	if err != nil {
		writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
		return
	}
	bodies := make([]creditPolicyBody, 0, len(rows))
	for _, row := range rows {
		bodies = append(bodies, creditPolicyBodyOf(row))
	}
	writeJSON(response, http.StatusOK, creditPolicyListResponse{
		Outcome:  outcomeCommercialPoliciesListed,
		Kind:     kindCreditPolicy,
		Policies: bodies,
	})
}

// 各册子各自的响应与行体。字段名不共享一套泛化壳:各册行形状互不相同,共享壳要么
// 空出大半字段,要么把强类型折成 any——kind 回显加各自成形的 policies 数组,调用方
// 按 kind 择形状。

type rulePackageListResponse struct {
	Outcome  string            `json:"outcome"`
	Kind     string            `json:"kind"`
	Policies []rulePackageBody `json:"policies"`
}

type assembledRuleBody struct {
	Category  string `json:"category"`
	Reference string `json:"reference"`
}

type finalRuleBody struct {
	Outcome   string `json:"outcome"`
	FinalKind string `json:"finalKind"`
}

// rulePackageBody 除规则集外还带两族阶段内容声明(0013:收寄资格、终局规则)。
//
// 两个 *Declared 布尔与票 01 的 contentRegistered 同款:未声明与「声明了但为空」都
// 表现为空数组,恢复动作却相反,少了布尔调用方分不开。allowedIntakeSources 与
// intakeQualificationRefs 分两个字段而不是并成一栏——前者不允许空、后者允许显式空,
// 两者的「空」不是同一件事。
type rulePackageBody struct {
	ObjectID          string              `json:"objectId"`
	Version           string              `json:"version"`
	ServiceProduct    string              `json:"serviceProduct"`
	Contract          string              `json:"contract"`
	LegalEntity       string              `json:"legalEntity"`
	Scope             string              `json:"scope"`
	EffectiveStartsAt string              `json:"effectiveStartsAt"`
	EffectiveEndsAt   string              `json:"effectiveEndsAt,omitempty"`
	DeclaredAt        string              `json:"declaredAt"`
	Rules             []assembledRuleBody `json:"rules"`

	IntakeQualificationDeclared bool     `json:"intakeQualificationDeclared"`
	AllowedIntakeSources        []string `json:"allowedIntakeSources"`
	IntakeQualificationRefs     []string `json:"intakeQualificationRefs"`

	FinalRulesDeclared bool            `json:"finalRulesDeclared"`
	FinalRules         []finalRuleBody `json:"finalRules"`
}

func rulePackageBodyOf(row ports.AcceptanceRulePackageRow) rulePackageBody {
	body := rulePackageBody{
		ObjectID:          row.ObjectID,
		Version:           row.VersionLabel,
		ServiceProduct:    row.ServiceProduct,
		Contract:          row.Contract,
		LegalEntity:       row.LegalEntity,
		Scope:             row.Scope,
		EffectiveStartsAt: rfc3339(row.EffectiveStartsAt),
		DeclaredAt:        rfc3339(row.DeclaredAt),
		Rules:             make([]assembledRuleBody, 0, len(row.Rules)),

		IntakeQualificationDeclared: row.HasIntakeQualification,
		AllowedIntakeSources:        append([]string{}, row.AllowedIntakeSources...),
		IntakeQualificationRefs:     append([]string{}, row.IntakeQualificationRefs...),

		FinalRulesDeclared: row.HasFinalRules,
		FinalRules:         make([]finalRuleBody, 0, len(row.FinalRules)),
	}
	if row.HasEffectiveEnd {
		body.EffectiveEndsAt = rfc3339(row.EffectiveEndsAt)
	}
	for _, rule := range row.Rules {
		body.Rules = append(body.Rules, assembledRuleBody{
			Category:  rule.Category,
			Reference: rule.Reference,
		})
	}
	for _, final := range row.FinalRules {
		body.FinalRules = append(body.FinalRules, finalRuleBody{
			Outcome:   final.Outcome,
			FinalKind: final.FinalKind,
		})
	}
	return body
}

type preAcceptanceControlListResponse struct {
	Outcome  string                     `json:"outcome"`
	Kind     string                     `json:"kind"`
	Policies []preAcceptanceControlBody `json:"policies"`
}

type preAcceptanceControlBody struct {
	ContractObjectID   string `json:"contractObjectId"`
	ContractVersion    string `json:"contractVersion"`
	Requirement        string `json:"requirement"`
	NotApplicableBasis string `json:"notApplicableBasis,omitempty"`
	DeclaredAt         string `json:"declaredAt"`
}

type pricePolicyListResponse struct {
	Outcome  string            `json:"outcome"`
	Kind     string            `json:"kind"`
	Policies []pricePolicyBody `json:"policies"`
}

// pricePolicyBody 是价格政策正文加它声明的计价口径（0022,票 party-commercial-context-gaps/06）。
//
// caliberDeclared 与 caliber 节成对:0010 早于 0022,只有正文没有口径的行是合法状态,布尔让调用方
// 分得开「没登记口径」与「口径节缺了」。口径节里三处可缺的键(分类、系数、fx)都是口径说出的
// 真话——不适用因而没有分类、采购方向因而没有系数、不涉外币因而没有汇率——所以用 omitempty 让键
// 不在场,而不是补空串让人去猜空串是「没有」还是「没填」。
type pricePolicyBody struct {
	ObjectID          string `json:"objectId"`
	Version           string `json:"version"`
	Direction         string `json:"direction"`
	PlanRef           string `json:"planRef"`
	PlanDirection     string `json:"planDirection"`
	BindingConversion string `json:"bindingConversion"`
	PolicyScope       string `json:"policyScope"`
	EffectiveStartsAt string `json:"effectiveStartsAt"`
	EffectiveEndsAt   string `json:"effectiveEndsAt,omitempty"`
	RegisteredAt      string `json:"registeredAt"`

	CaliberDeclared bool                    `json:"caliberDeclared"`
	Caliber         *pricePolicyCaliberBody `json:"caliber,omitempty"`
}

type pricePolicyCaliberBody struct {
	TaxDisposition    string         `json:"taxDisposition"`
	TaxClassification string         `json:"taxClassification,omitempty"`
	VolumetricFactor  string         `json:"volumetricFactor,omitempty"`
	Fx                *fxCaliberBody `json:"fx,omitempty"`
	RegisteredAt      string         `json:"registeredAt"`
}

type fxCaliberBody struct {
	QuoteType         string `json:"quoteType"`
	AsOfSemantics     string `json:"asOfSemantics"`
	AsOfPolicyVersion string `json:"asOfPolicyVersion"`
}

func pricePolicyBodyOf(row ports.PricePolicyRow) pricePolicyBody {
	body := pricePolicyBody{
		ObjectID:          row.ObjectID,
		Version:           row.VersionLabel,
		Direction:         row.Direction,
		PlanRef:           row.PlanRef,
		PlanDirection:     row.PlanDirection,
		BindingConversion: row.BindingConversion,
		PolicyScope:       row.PolicyScope,
		EffectiveStartsAt: rfc3339(row.EffectiveStartsAt),
		RegisteredAt:      rfc3339(row.RegisteredAt),
		CaliberDeclared:   row.HasCaliber,
	}
	if row.HasEffectiveEnd {
		body.EffectiveEndsAt = rfc3339(row.EffectiveEndsAt)
	}
	if row.HasCaliber {
		caliber := &pricePolicyCaliberBody{
			TaxDisposition:    row.TaxDisposition,
			TaxClassification: row.TaxClassification,
			VolumetricFactor:  row.VolumetricFactor,
			RegisteredAt:      rfc3339(row.CaliberRegisteredAt),
		}
		if row.HasFx {
			caliber.Fx = &fxCaliberBody{
				QuoteType:         row.FxQuoteType,
				AsOfSemantics:     row.FxAsOfSemantics,
				AsOfPolicyVersion: row.FxAsOfPolicyVersion,
			}
		}
		body.Caliber = caliber
	}
	return body
}

type settlementPolicyListResponse struct {
	Outcome  string                 `json:"outcome"`
	Kind     string                 `json:"kind"`
	Policies []settlementPolicyBody `json:"policies"`
}

type settlementPolicyBody struct {
	ObjectID          string `json:"objectId"`
	Version           string `json:"version"`
	Method            string `json:"method"`
	LegalEntity       string `json:"legalEntity"`
	Counterparty      string `json:"counterparty"`
	ContractLabel     string `json:"contractLabel"`
	ChargeScope       string `json:"chargeScope"`
	Currency          string `json:"currency"`
	EffectiveStartsAt string `json:"effectiveStartsAt"`
	EffectiveEndsAt   string `json:"effectiveEndsAt,omitempty"`
	RegisteredAt      string `json:"registeredAt"`
}

type asOfPolicyListResponse struct {
	Outcome  string           `json:"outcome"`
	Kind     string           `json:"kind"`
	Policies []asOfPolicyBody `json:"policies"`
}

type asOfPolicyBody struct {
	RulePackageObjectID string `json:"rulePackageObjectId"`
	RulePackageVersion  string `json:"rulePackageVersion"`
	JudgmentType        string `json:"judgmentType"`
	SemanticsRef        string `json:"semanticsRef"`
	PolicyVersion       string `json:"policyVersion"`
	DeclaredAt          string `json:"declaredAt"`
}

type creditPolicyListResponse struct {
	Outcome  string             `json:"outcome"`
	Kind     string             `json:"kind"`
	Policies []creditPolicyBody `json:"policies"`
}

// creditPolicyBody 的额度是两个指针键恰一在场:金额行只长 limitMinor、比例行只长
// limitRatioBasisPoints。用指针而不是 omitempty 的整数——零额度是合法的商业声明
// （「授予零信用」），omitempty 会把它抹成「没声明」，而那两件事要人做的事相反。
type creditPolicyBody struct {
	ObjectID              string `json:"objectId"`
	Version               string `json:"version"`
	LegalEntity           string `json:"legalEntity"`
	AuthorityLevel        string `json:"authorityLevel"`
	ChargeType            string `json:"chargeType"`
	LimitMinor            *int64 `json:"limitMinor,omitempty"`
	LimitRatioBasisPoints *int64 `json:"limitRatioBasisPoints,omitempty"`
	EffectiveStartsAt     string `json:"effectiveStartsAt"`
	EffectiveEndsAt       string `json:"effectiveEndsAt,omitempty"`
	RegisteredAt          string `json:"registeredAt"`
}

func creditPolicyBodyOf(row ports.CreditPolicyRow) creditPolicyBody {
	body := creditPolicyBody{
		ObjectID:          row.ObjectID,
		Version:           row.VersionLabel,
		LegalEntity:       row.LegalEntity,
		AuthorityLevel:    row.AuthorityLevel,
		ChargeType:        row.ChargeType,
		EffectiveStartsAt: rfc3339(row.EffectiveStartsAt),
		RegisteredAt:      rfc3339(row.RegisteredAt),
	}
	if row.HasAmount {
		minor := row.LimitMinor
		body.LimitMinor = &minor
	} else {
		bps := row.LimitRatioBasisPoints
		body.LimitRatioBasisPoints = &bps
	}
	if row.HasEffectiveEnd {
		body.EffectiveEndsAt = rfc3339(row.EffectiveEndsAt)
	}
	return body
}

type authorizationRuleListResponse struct {
	Outcome  string                  `json:"outcome"`
	Kind     string                  `json:"kind"`
	Policies []authorizationRuleBody `json:"policies"`
}

type cancellationAuthorityBody struct {
	Party         string `json:"party"`
	RuleReference string `json:"ruleReference"`
}

// authorizationRuleBody 是授权规则版本壳加它的取消授权目录。
//
// cancellationAuthorityDeclared 这个布尔在本族比别处更要紧:数组里少一个请求方**不是**
// 少一份声明,而是这份目录说出的真话(该请求方不许取消)。调用方只有先看布尔才知道
// 手上这份空缺属于哪一种,判据见 ports.AuthorizationRuleRow。
type authorizationRuleBody struct {
	ObjectID          string `json:"objectId"`
	Version           string `json:"version"`
	Scope             string `json:"scope"`
	Status            string `json:"status"`
	EffectiveStartsAt string `json:"effectiveStartsAt"`
	EffectiveEndsAt   string `json:"effectiveEndsAt,omitempty"`
	PublishedAt       string `json:"publishedAt"`

	CancellationAuthorityDeclared bool                        `json:"cancellationAuthorityDeclared"`
	DeclaredAt                    string                      `json:"declaredAt,omitempty"`
	CancellationAuthorities       []cancellationAuthorityBody `json:"cancellationAuthorities"`
}

func authorizationRuleBodyOf(row ports.AuthorizationRuleRow) authorizationRuleBody {
	body := authorizationRuleBody{
		ObjectID:          row.ObjectID,
		Version:           row.VersionLabel,
		Scope:             row.Scope,
		Status:            row.Status,
		EffectiveStartsAt: rfc3339(row.EffectiveStartsAt),
		PublishedAt:       rfc3339(row.PublishedAt),

		CancellationAuthorityDeclared: row.HasCancellationAuthority,
		CancellationAuthorities:       make([]cancellationAuthorityBody, 0, len(row.CancellationAuthorities)),
	}
	if row.HasEffectiveEnd {
		body.EffectiveEndsAt = rfc3339(row.EffectiveEndsAt)
	}
	if row.HasCancellationAuthority {
		body.DeclaredAt = rfc3339(row.DeclaredAt)
	}
	for _, authority := range row.CancellationAuthorities {
		body.CancellationAuthorities = append(body.CancellationAuthorities, cancellationAuthorityBody{
			Party:         authority.Party,
			RuleReference: authority.RuleReference,
		})
	}
	return body
}
