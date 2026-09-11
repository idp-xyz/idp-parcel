package customshttp

import (
	"context"
	"net/http"
	"time"

	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

// DutyCollaborationCatalogueReader 是本端点消费的读口：税费付款协作事项登记册的列表
// 读面。查阅不触发判断、决定或披露——接存储读面，不接应用编排（ADR-0077 Decision 一）。
type DutyCollaborationCatalogueReader interface {
	ListDutyCollaborations(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
	) ([]domain.DutyPaymentCollaboration, error)
}

// 编译期锁缝：读口形状与端口保持一致——本端点不新造查询语义。
var _ DutyCollaborationCatalogueReader = ports.DutyCollaborationCatalogueRead(nil)

// 业务结果单格：协作事项册只有一本，端点不设 registry 分派。空册如实答空列表走 2xx
// 成格（ADR-0077 Decision 四）。
const outcomeDutyCollaborationsListed = "DUTY_COLLABORATIONS_LISTED"

// NewQueryDutyCollaborationsEndpoint 交回税费付款协作事项册查阅的 HTTP 入口
// （GET /customs-duty-collaborations，票 sa-cc/10）。
//
// 独立端点、不与付款核对册合成一个 `?registry=` 分派：两册同归 customs-restrictions 页，
// 但票 sa-cc/10 裁决「端点各立三个入口」——协作事项与核对是 UC-CC-009「七层对象必须分离」里的两层，
// 各自一口让「一层的译装接到另一层的端点上」在装配处就对不上。
//
// 义务依据两格逐字段转写、互不串格：核定税费格带 duty 不带 noPayBasis，明确无需付款格
// 反之——领域构造只放这两种形状（FormDutyCollaboration），第三种「没有结果所以不用付」
// 在类型上没有格，传输层也不给它留。它不等于支付指令、付款交易、客户回收或监管放行
// ——响应形里没有那些键。
func NewQueryDutyCollaborationsEndpoint(
	intake CatalogueQueryIntake,
	reader DutyCollaborationCatalogueReader,
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

		collaborations, err := reader.ListDutyCollaborations(request.Context(), query.Scope.Tenant(), query.Limit)
		if err != nil {
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}

		bodies := make([]dutyCollaborationBody, 0, len(collaborations))
		for _, collaboration := range collaborations {
			body := dutyCollaborationBody{
				Scope:       collaboration.Scope().String(),
				Kind:        collaboration.Kind().String(),
				Obligor:     collaboration.Obligor().String(),
				Requirement: collaboration.Requirement().String(),
				Target:      collaboration.Target().String(),
				FormedAt:    collaboration.FormedAt().UTC().Format(time.RFC3339Nano),
			}
			if duty, assessed := collaboration.Duty(); assessed {
				body.Duty = duty.String()
			}
			if basis, notRequired := collaboration.NoPayBasis(); notRequired {
				body.NoPayBasis = basis
			}
			bodies = append(bodies, body)
		}
		writeJSON(response, http.StatusOK, dutyCollaborationListResponse{
			Outcome:        outcomeDutyCollaborationsListed,
			Collaborations: bodies,
		})
	})
}

type dutyCollaborationListResponse struct {
	Outcome        string                  `json:"outcome"`
	Collaborations []dutyCollaborationBody `json:"collaborations"`
}

// dutyCollaborationBody 逐字段透出一份协作事项。kind 封闭二值（ASSESSED_DUTY /
// EXPLICITLY_NOT_REQUIRED）；duty 与 noPayBasis 按格只在一个上在场——两者同在或同缺都
// 立不起领域对象，读口上就抛了，传输层不会遇到。法定义务人（obligor）、实际付款方与
// 最终承担费用的客户可以不同、不能互相推导（CONTEXT「不能互相推导」）——这里只有法定义务人
// 一列，另两个各归其所有者。
type dutyCollaborationBody struct {
	Scope       string `json:"scope"`
	Kind        string `json:"kind"`
	Duty        string `json:"duty,omitempty"`
	NoPayBasis  string `json:"noPayBasis,omitempty"`
	Obligor     string `json:"obligor"`
	Requirement string `json:"requirement"`
	Target      string `json:"target"`
	FormedAt    string `json:"formedAt"`
}
