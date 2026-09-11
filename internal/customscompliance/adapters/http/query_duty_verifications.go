package customshttp

import (
	"context"
	"net/http"
	"time"

	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

// DutyVerificationCatalogueReader 是本端点消费的读口：税费付款核对登记册的列表读面。
// 查阅不触发判断、决定或披露——接存储读面，不接应用编排（ADR-0077 Decision 一）。
type DutyVerificationCatalogueReader interface {
	ListDutyVerifications(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
	) ([]ports.DutyVerificationRecord, error)
}

// 编译期锁缝：读口形状与端口保持一致——本端点不新造查询语义。
var _ DutyVerificationCatalogueReader = ports.DutyVerificationCatalogueRead(nil)

// 业务结果单格：付款核对册只有一本，端点不设 registry 分派。空册如实答空列表走 2xx
// 成格（ADR-0077 Decision 四）。
const outcomeDutyVerificationsListed = "DUTY_VERIFICATIONS_LISTED"

// NewQueryDutyVerificationsEndpoint 交回税费付款核对册查阅的 HTTP 入口
// （GET /customs-duty-verifications，票 sa-cc/10）。独立端点的理由在
// NewQueryDutyCollaborationsEndpoint 文件头，此处不复述。
//
// 三轴逐键原值、不折总状态：覆盖 / 差额 / 有效性三列各自封闭，响应形里刻意没有任何
// 「status」「settled」之类的合成列（ADR-0137 决定三；CONTEXT「分别表达，不能实现为一组互斥总状态」）——在这里折一次，页面就成了第二处判断权威，而放行门禁那一道怎么读三态
// 是按监管程序登记进来的规则，不是查阅面的常量。全部版本连指纹原样上列：迟到事实按新
// 版本追加、不按到达顺序覆盖（UC-CC-009），哪版是当前由读者按 verifiedAt 判读。「待关联」
// 是资金事实册上的派生（没有核对引用它的事实就是待关联），不是核对册的列，本端点不代算。
func NewQueryDutyVerificationsEndpoint(
	intake CatalogueQueryIntake,
	reader DutyVerificationCatalogueReader,
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

		records, err := reader.ListDutyVerifications(request.Context(), query.Scope.Tenant(), query.Limit)
		if err != nil {
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}

		bodies := make([]dutyVerificationBody, 0, len(records))
		for _, record := range records {
			verification := record.Verification
			bodies = append(bodies, dutyVerificationBody{
				Duty:       record.Key.Duty.String(),
				Funds:      record.Key.Funds.String(),
				Scope:      record.Key.Scope.String(),
				Version:    record.Key.Digest,
				Coverage:   verification.Coverage().String(),
				Delta:      verification.Delta().String(),
				Validity:   verification.Validity().String(),
				Basis:      record.Basis,
				VerifiedAt: verification.VerifiedAt().UTC().Format(time.RFC3339Nano),
			})
		}
		writeJSON(response, http.StatusOK, dutyVerificationListResponse{
			Outcome:       outcomeDutyVerificationsListed,
			Verifications: bodies,
		})
	})
}

type dutyVerificationListResponse struct {
	Outcome       string                 `json:"outcome"`
	Verifications []dutyVerificationBody `json:"verifications"`
}

// dutyVerificationBody 逐字段透出一版核对。version 是幂等键上的内容指纹（同三维同内容
// 重放不出第二版，改判换指纹追加新版）；coverage / delta / validity 三轴各自封闭
// （NONE / PARTIAL / COVERED、NO_DELTA / SHORT / EXCESS / PENDING、VALID / INVALIDATED /
// CONFLICTING / PENDING），集外词形在读口重建处就上抛，传输层不折第四格；basis 是「凭什么
// 把这笔资金关联到这版税费」的证据引用——金额相等不能单独作为关联（UC-CC-009）。
type dutyVerificationBody struct {
	Duty       string `json:"duty"`
	Funds      string `json:"funds"`
	Scope      string `json:"scope"`
	Version    string `json:"version"`
	Coverage   string `json:"coverage"`
	Delta      string `json:"delta"`
	Validity   string `json:"validity"`
	Basis      string `json:"basis"`
	VerifiedAt string `json:"verifiedAt"`
}
