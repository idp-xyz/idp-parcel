package commercialhttp

import (
	"context"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// CommercialRelationCatalogueReader 是商业关系载体目录两个端点消费的读口。
type CommercialRelationCatalogueReader interface {
	ListCustomerContracts(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
	) ([]ports.CustomerContractCatalogueRow, error)
	ListSupplierAgreements(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
	) ([]ports.SupplierAgreementCatalogueRow, error)
}

// 编译期锁缝:读口形状与端口保持一致。
var _ CommercialRelationCatalogueReader = ports.CommercialRelationCatalogueRead(nil)

// 两个端点各自唯一的业务成格;空册也是这一格(ADR-0077 Decision 四)。
const (
	outcomeCustomerContractsListed  = "CUSTOMER_CONTRACTS_LISTED"
	outcomeSupplierAgreementsListed = "SUPPLIER_AGREEMENTS_LISTED"
)

// 客户合同与供应商协议**各立入口**,不并进 /commercial-policies 的 kind 分派,也不
// 合成第三个带 kind 的入口。两层理由:
//
// 其一,合同与协议不是策略。并进去之后那个参数名就开始说谎,而路径是对外契约的一
// 部分,日后改的代价比现在分开大。
//
// 其二——这条更要紧——**那个 kind 分的是页签,不是种类**。/commercial-policies 服务
// 的是「商业规则与策略」一页里的五个页签,一页一入口;而合同与协议是管理台上两张
// 独立的页,照 /commercial-service-products 的先例各配一个入口。把两张页折进一个带
// kind 的入口,会让「页面加一个页签」和「产品多一类目录」在契约上长成同一个动作。

// NewQueryCustomerContractsEndpoint 交回客户与合同目录查阅的 HTTP 入口
// (GET /commercial-customer-contracts,ADR-0077)。
func NewQueryCustomerContractsEndpoint(
	intake CommercialCatalogueIntake,
	reader CommercialRelationCatalogueReader,
) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			response.Header().Set("Allow", http.MethodGet)
			writeProblem(response, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}

		query, err := intake.IntakeCatalogueQuery(request.Context(), request)
		if err != nil {
			writeCatalogueIntakeProblem(response, err)
			return
		}

		rows, err := reader.ListCustomerContracts(request.Context(), query.Scope.Tenant(), query.Limit)
		if err != nil {
			// 读不回是答案未形成,不是「空目录」——伪装成后者会让一次该重试的故障变成
			// 一份看起来如实的空册。
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}

		bodies := make([]customerContractBody, 0, len(rows))
		for _, row := range rows {
			bodies = append(bodies, customerContractBodyOf(row))
		}
		writeJSON(response, http.StatusOK, customerContractListResponse{
			Outcome:   outcomeCustomerContractsListed,
			Contracts: bodies,
		})
	})
}

// NewQuerySupplierAgreementsEndpoint 交回供应商协议目录查阅的 HTTP 入口
// (GET /commercial-supplier-agreements,ADR-0077)。
func NewQuerySupplierAgreementsEndpoint(
	intake CommercialCatalogueIntake,
	reader CommercialRelationCatalogueReader,
) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			response.Header().Set("Allow", http.MethodGet)
			writeProblem(response, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}

		query, err := intake.IntakeCatalogueQuery(request.Context(), request)
		if err != nil {
			writeCatalogueIntakeProblem(response, err)
			return
		}

		rows, err := reader.ListSupplierAgreements(request.Context(), query.Scope.Tenant(), query.Limit)
		if err != nil {
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}

		bodies := make([]supplierAgreementBody, 0, len(rows))
		for _, row := range rows {
			bodies = append(bodies, supplierAgreementBodyOf(row))
		}
		writeJSON(response, http.StatusOK, supplierAgreementListResponse{
			Outcome:    outcomeSupplierAgreementsListed,
			Agreements: bodies,
		})
	})
}

type customerContractListResponse struct {
	Outcome   string                 `json:"outcome"`
	Contracts []customerContractBody `json:"contracts"`
}

// customerContractBody 逐字段透出版本壳与正文。
//
// contentRegistered 是一个显式布尔而不是靠 bindings 数组的长度去推:**无正文行**与
// **有正文行零绑定**在「零绑定」上撞成同一个可观察签名,而两者的恢复动作相反(前者
// 去登记正文,后者无事可做——那份合同就是没对任何费用范围作约定)。把这个区别做进
// 结构,读的人不必去分辨;0012 迁移专门用父子两表表达的就是它。
type customerContractBody struct {
	ObjectID          string               `json:"objectId"`
	Version           string               `json:"version"`
	Scope             string               `json:"scope"`
	Status            string               `json:"status"`
	EffectiveStartsAt string               `json:"effectiveStartsAt"`
	EffectiveEndsAt   string               `json:"effectiveEndsAt,omitempty"`
	PublishedAt       string               `json:"publishedAt"`
	ContentRegistered bool                 `json:"contentRegistered"`
	RulePackageID     string               `json:"rulePackageId,omitempty"`
	DeclaredAt        string               `json:"declaredAt,omitempty"`
	Bindings          []controlBindingBody `json:"bindings"`
}

// controlBindingBody 里 policyId 与 basis 恰有一个在场,与库上 CHECK 同形;装载口
// 已经拦下两空的坏行,这里照实转写。
type controlBindingBody struct {
	ChargeScope string `json:"chargeScope"`
	PolicyID    string `json:"policyId,omitempty"`
	Basis       string `json:"inapplicabilityBasis,omitempty"`
}

func customerContractBodyOf(row ports.CustomerContractCatalogueRow) customerContractBody {
	body := customerContractBody{
		ObjectID:          row.ObjectID,
		Version:           row.VersionLabel,
		Scope:             row.Scope,
		Status:            row.Status,
		EffectiveStartsAt: rfc3339(row.EffectiveStartsAt),
		PublishedAt:       rfc3339(row.PublishedAt),
		ContentRegistered: row.HasContent,
		Bindings:          make([]controlBindingBody, 0, len(row.Bindings)),
	}
	if row.HasEffectiveEnd {
		body.EffectiveEndsAt = rfc3339(row.EffectiveEndsAt)
	}
	if row.HasContent {
		body.RulePackageID = row.RulePackageID
		body.DeclaredAt = rfc3339(row.DeclaredAt)
	}
	for _, binding := range row.Bindings {
		body.Bindings = append(body.Bindings, controlBindingBody{
			ChargeScope: binding.ChargeScope,
			PolicyID:    binding.PolicyID,
			Basis:       binding.InapplicabilityBasis,
		})
	}
	return body
}

type supplierAgreementListResponse struct {
	Outcome    string                  `json:"outcome"`
	Agreements []supplierAgreementBody `json:"agreements"`
}

// supplierAgreementBody 只有版本壳。供应商、采购定价方案与方向在领域对象上,但没有
// 正文表可读(见 ports.SupplierAgreementCatalogueRow),因此这里没有对应字段——缺的
// 是登记面,不是转写。
type supplierAgreementBody struct {
	ObjectID          string `json:"objectId"`
	Version           string `json:"version"`
	Scope             string `json:"scope"`
	Status            string `json:"status"`
	EffectiveStartsAt string `json:"effectiveStartsAt"`
	EffectiveEndsAt   string `json:"effectiveEndsAt,omitempty"`
	PublishedAt       string `json:"publishedAt"`
}

func supplierAgreementBodyOf(row ports.SupplierAgreementCatalogueRow) supplierAgreementBody {
	body := supplierAgreementBody{
		ObjectID:          row.ObjectID,
		Version:           row.VersionLabel,
		Scope:             row.Scope,
		Status:            row.Status,
		EffectiveStartsAt: rfc3339(row.EffectiveStartsAt),
		PublishedAt:       rfc3339(row.PublishedAt),
	}
	if row.HasEffectiveEnd {
		body.EffectiveEndsAt = rfc3339(row.EffectiveEndsAt)
	}
	return body
}
