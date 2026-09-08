package commercialhttp

import (
	"fmt"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// 本文件是授权规则册在运营操作者面载荷上的那一节（票 admin-write-faces/17）：线格式与逐格翻译。它挂在
// publication_draft_payload.go 的 CommercialPublicationPayload 上一格，解码、身份与逐格问题的规矩都在那一份。

// AuthorizationRuleBodyPayload 镜像受控批文 declarations.cancellationAuthority 与规范化文档的键名：本册的正文就是
// 取消授权目录一张两列几行的表（请求方 × 规则引用）。授权授予（AuthorizedAction 一族）不经这条发布路，载荷里出现
// 别的键由严格解码按未知键拒。
type AuthorizationRuleBodyPayload struct {
	CancellationAuthority []CancellationAuthorityRulePayload `json:"cancellationAuthority"`
}

// CancellationAuthorityRulePayload 是目录的一行：party 是封闭二值的原词（CUSTOMER / OPERATIONS，由服务端词表读口供
// 表单下拉），rule 是引用串。
type CancellationAuthorityRulePayload struct {
	Party string `json:"party"`
	Rule  string `json:"rule"`
}

// body 把目录逐行过领域构造门；每一格的问题落在 authorizationRule.cancellationAuthority[i].<键> 上，收齐后由调用方一次
// 交回。零行、同一请求方两行是跨行的判，留给领域 validate 在预览上答`未受理`带成因——这里只翻格。
func (payload AuthorizationRuleBodyPayload) body(problems *PublicationPayloadProblems) domain.AuthorizationRuleBody {
	body := domain.AuthorizationRuleBody{
		CancellationAuthority: make([]domain.CancellationAuthorityDeclaration, 0, len(payload.CancellationAuthority)),
	}
	for index, row := range payload.CancellationAuthority {
		path := fmt.Sprintf("authorizationRule.cancellationAuthority[%d]", index)
		party, known := domain.DeclaredCancellationPartyNamed(row.Party)
		if !known {
			problems.add(path+".party", fmt.Errorf("集合外的取消请求方 %q", row.Party))
		}
		body.CancellationAuthority = append(body.CancellationAuthority, domain.CancellationAuthorityDeclaration{
			Party: party,
			Rule:  requireField(problems, path+".rule", domain.NewRuleReference, row.Rule),
		})
	}
	return body
}
