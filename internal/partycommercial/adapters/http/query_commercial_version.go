package commercialhttp

import (
	"context"
	"fmt"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

const (
	outcomeCommercialVersionRead   = "COMMERCIAL_VERSION_READ"
	outcomeCommercialVersionAbsent = "COMMERCIAL_VERSION_NOT_FOUND"

	// 路由模式里的路径参数名；装配点的 Pattern 与这里必须同字，否则 PathValue 恒空、端点恒答 400。
	// 与载体读回同名不是巧合：两口的身份都是（类别、对象、版本）三格。
	versionKindPathValue     = "kind"
	versionObjectIDPathValue = "objectId"
	versionLabelPathValue    = "version"
)

// NewQueryCommercialVersionEndpoint 交回商业版本按身份读回的 HTTP 入口
// （GET /commercial-version-readbacks/{kind}/{objectId}/{version}，ADR-0361 决定一）。
//
// 它给管理台的对象页一个能深链、能分享的地址：身份是（租户、类别、对象、版本），范围不是键。答复里的 `version`
// 与该类别目录行同形——它是目录列表的投影（读口按同一条语句多两格键过滤），对象页显示的就是目录上的那一行。
// 不在册答 200 加 `COMMERCIAL_VERSION_NOT_FOUND`（ADR-0022：状态码只说答案有没有形成）；跨租户与不在册同形，
// 不泄露存在性。类别或标识不合法答 400。
//
// 三个读口按类别分派：类别是领域的封闭集（`CommercialObjectKindNamed`），`kind` 回显领域原词（`PRICE_RULE`，不是
// 目录列表页签用的 `PRICE_POLICY`）。接受判断时点策略与接受前财务控制声明没有自己的版本（随规则包 / 合同），
// 不是本口的类别——它们本来就不在 `CommercialObjectKind` 里。
func NewQueryCommercialVersionEndpoint(
	intake CommercialCatalogueIntake,
	products ServiceProductCatalogueReader,
	relations CommercialRelationCatalogueReader,
	policies CommercialPolicyCatalogueReader,
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
		// 路径参数在 Intake 之后读：未配置要拒在一切请求内容之前（ADR-0055），路径也是请求内容。只有编码在本处理器之前判：
		// 路径段解码后含 NUL 或不是合法 UTF-8，装配处挂的 httpapi.RefuseUnstorableReadTarget 先答 400，那是传输形状。
		kind, known := domain.CommercialObjectKindNamed(request.PathValue(versionKindPathValue))
		objectID, objectErr := domain.NewCommercialObjectID(request.PathValue(versionObjectIDPathValue))
		version, versionErr := domain.NewCommercialVersionLabel(request.PathValue(versionLabelPathValue))
		if !known || objectErr != nil || versionErr != nil {
			writeProblem(response, http.StatusBadRequest, codeMalformedRequest)
			return
		}
		body, found, err := findCommercialVersion(request.Context(), commercialVersionReaders{products, relations, policies},
			query.Scope.Tenant(), kind, objectID, version)
		if err != nil {
			// 读不回是答案未形成，不是「不在册」：伪装成后者会让对象页把一次该重试的故障显示成这一版不存在。
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		if !found {
			writeJSON(response, http.StatusOK, commercialVersionReadResponse{Outcome: outcomeCommercialVersionAbsent})
			return
		}
		writeJSON(response, http.StatusOK, commercialVersionReadResponse{
			Outcome: outcomeCommercialVersionRead,
			Kind:    kind.String(),
			Version: body,
		})
	})
}

type commercialVersionReadResponse struct {
	Outcome string `json:"outcome"`
	Kind    string `json:"kind,omitempty"`
	// Version 是该类别目录行的那一份：各类别各自的 body 类型，由各自目录端点在用的 …BodyOf 折出，不另造第二套形状。
	Version any `json:"version,omitempty"`
}

type commercialVersionReaders struct {
	products  ServiceProductCatalogueReader
	relations CommercialRelationCatalogueReader
	policies  CommercialPolicyCatalogueReader
}

// findCommercialVersion 按类别挑读口与折 body 的函数。每个分支都走该类别目录端点在用的 …BodyOf：对象页与目录页
// 显示的是同一份转写，一边加字段另一边不会落下。类别集合在领域封闭，这里对不认得的类别答「不在册」而不是 panic
// ——`CommercialObjectKindNamed` 已把集合外的名字拦在 400，走到 default 只可能是集合扩了而本口还没接那一册。
func findCommercialVersion(
	ctx context.Context,
	readers commercialVersionReaders,
	tenant domain.TenantID,
	kind domain.CommercialObjectKind,
	objectID domain.CommercialObjectID,
	version domain.CommercialVersionLabel,
) (any, bool, error) {
	switch kind {
	case domain.ServiceProductObject:
		row, found, err := readers.products.FindServiceProduct(ctx, tenant, objectID, version)
		return readBody(row, found, err, serviceProductBodyOf)
	case domain.CustomerContractObject:
		row, found, err := readers.relations.FindCustomerContract(ctx, tenant, objectID, version)
		return readBody(row, found, err, customerContractBodyOf)
	case domain.SupplierAgreementObject:
		row, found, err := readers.relations.FindSupplierAgreement(ctx, tenant, objectID, version)
		return readBody(row, found, err, supplierAgreementBodyOf)
	case domain.AcceptanceRulePackageObject:
		row, found, err := readers.policies.FindAcceptanceRulePackage(ctx, tenant, objectID, version)
		return readBody(row, found, err, rulePackageBodyOf)
	case domain.PreAcceptanceFinancialControlPolicyObject:
		row, found, err := readers.policies.FindPreAcceptanceFinancialControlPolicy(ctx, tenant, objectID, version)
		return readBody(row, found, err, preAcceptanceFinancialControlPolicyBodyOf)
	case domain.PriceRuleObject:
		row, found, err := readers.policies.FindPricePolicy(ctx, tenant, objectID, version)
		return readBody(row, found, err, pricePolicyBodyOf)
	case domain.SettlementPolicyObject:
		row, found, err := readers.policies.FindSettlementPolicy(ctx, tenant, objectID, version)
		return readBody(row, found, err, settlementPolicyBodyOf)
	case domain.CreditPolicyObject:
		row, found, err := readers.policies.FindCreditPolicy(ctx, tenant, objectID, version)
		return readBody(row, found, err, creditPolicyBodyOf)
	case domain.AuthorizationRuleObject:
		row, found, err := readers.policies.FindAuthorizationRule(ctx, tenant, objectID, version)
		return readBody(row, found, err, authorizationRuleBodyOf)
	case domain.CustomerServiceRuleObject:
		row, found, err := readers.policies.FindCustomerServiceRule(ctx, tenant, objectID, version)
		return readBody(row, found, err, customerServiceRuleBodyOf)
	default:
		// 领域类别集里新加了一格而这里没接线：那是本处理器的缺口，答「答案未形成」让人看见，不能答成「不在册」。
		return nil, false, fmt.Errorf("commercial version readback: 类别 %s 没有接读口", kind.String())
	}
}

// readBody 把读口的（行、在不在、错）折成（body、在不在、错）：只在行在册时才折 body，各 …BodyOf 不必对零值行负责。
func readBody[Row, Body any](row Row, found bool, err error, bodyOf func(Row) Body) (any, bool, error) {
	if err != nil || !found {
		return nil, false, err
	}
	return bodyOf(row), true, nil
}
