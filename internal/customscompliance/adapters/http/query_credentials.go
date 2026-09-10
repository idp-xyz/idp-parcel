package customshttp

import (
	"context"
	"net/http"
	"time"

	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

// CredentialCatalogueReader 是本端点消费的读口：监管凭证登记册的列表读面。查阅不触发
// 判断、决定或披露——接存储读面，不接应用编排（ADR-0077 Decision 一）；凭证适用性判断
// （JudgeApplicability）是另一个调用面的事，本端点一格不折。
type CredentialCatalogueReader interface {
	ListCredentials(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
	) ([]ports.CredentialCatalogueEntry, error)
}

// 编译期锁缝：读口形状与端口保持一致——本端点不新造查询语义。
var _ CredentialCatalogueReader = ports.CredentialCatalogueRead(nil)

// 业务结果单格：凭证册只有一本，端点不设 registry 分派（封闭集为一时参数只会造出一个
// 恒定值，判据同 /customs-gate-conditions）。空册如实答空列表走 2xx 成格（ADR-0077
// Decision 四）。
const outcomeCredentialsListed = "CREDENTIALS_LISTED"

// NewQueryCredentialsEndpoint 交回监管凭证册查阅的 HTTP 入口（GET /customs-credentials，
// 票 sa-cc/10）。
//
// 独立端点而不并进 /customs-case-registers 的分派：那个端点的三册与本册虽同归
// customs-cases 页，票 sa-cc/10 裁决「端点各立三个入口」——本册是就绪门禁第 4 道的依据册，
// 不是案件配置四册之一，分派参数的封闭集不为它开第四格；三册读面同批各立入口，形状一致。
//
// 逐版原样上列：凭证是不可变版本，一身份一版，换期限或额度是另一张凭证（0014 自注），
// 所以没有「当前版 / 历史版」的折叠可做。次数额度「来源未提供」以 uses 缺席表达——写成 0
// 会被读成额度已用尽，而那一格在本册根本没有表达方式（余额不是登记内容）。
func NewQueryCredentialsEndpoint(
	intake CatalogueQueryIntake,
	reader CredentialCatalogueReader,
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

		entries, err := reader.ListCredentials(request.Context(), query.Scope.Tenant(), query.Limit)
		if err != nil {
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}

		bodies := make([]credentialBody, 0, len(entries))
		for _, entry := range entries {
			credential := entry.Credential
			body := credentialBody{
				Credential:   credential.ID().String(),
				Issuer:       credential.Issuer().String(),
				Holder:       credential.Holder().String(),
				Procedure:    credential.Procedure().String(),
				ValidFrom:    credential.ValidFrom().UTC().Format(time.RFC3339Nano),
				ValidTo:      credential.ValidTo().UTC().Format(time.RFC3339Nano),
				RegisteredAt: entry.RegisteredAt.UTC().Format(time.RFC3339Nano),
			}
			if uses, provided := credential.Uses(); provided {
				body.Uses = &uses
			}
			bodies = append(bodies, body)
		}
		writeJSON(response, http.StatusOK, credentialListResponse{
			Outcome:     outcomeCredentialsListed,
			Credentials: bodies,
		})
	})
}

type credentialListResponse struct {
	Outcome     string           `json:"outcome"`
	Credentials []credentialBody `json:"credentials"`
}

// credentialBody 逐字段透出一版监管凭证。uses 只在来源提供了次数额度时在场：领域把零约定
// 为「来源未提供」并经 Uses 的第二个返回值显式交出，传输层照那道区分落成缺席 / 在场——
// 不写 0，0 在读者眼里是「额度已用尽」，而本册不登余额。registeredAt 是登记动作的时钟，
// 与 validFrom / validTo 两端是三件事。
type credentialBody struct {
	Credential   string `json:"credential"`
	Issuer       string `json:"issuer"`
	Holder       string `json:"holder"`
	Procedure    string `json:"procedure"`
	ValidFrom    string `json:"validFrom"`
	ValidTo      string `json:"validTo"`
	Uses         *int   `json:"uses,omitempty"`
	RegisteredAt string `json:"registeredAt"`
}
