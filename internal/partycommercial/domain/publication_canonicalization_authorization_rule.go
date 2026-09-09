package domain

import (
	"fmt"
	"sort"
)

// 本文件是授权规则册接进服务端规范化的那一格（加册不换号，仍是 PCC-1——ADR-0126 Decision 一；票
// admin-write-faces/17）。版本壳之外归本册的正文只有一份声明：0013 的取消授权目录（哪种请求方被允许
// 取消、依据哪条规则）。受控批文把它写成 declarations.cancellationAuthority 一键；这里把它折进本册一格，
// 键名镜像批文。授权授予册（AuthorizedAction 一族）不经这条发布路，不在这里。

// AuthorizationRuleBody 是授权规则版本的正文输入面：NewCancellationAuthorityContent 收的那几行，只是不带
// 拥有它们的（已生效）版本——预览与录入发生在发布之前，那时没有已生效版本可挂。
type AuthorizationRuleBody struct {
	CancellationAuthority []CancellationAuthorityDeclaration
}

// validate 在折成文档前把目录过一遍与发布时相同的门（declaredCancellationAuthorityByParty）：至少一行、每行
// 立得住、同一请求方不两行。不另造校验——预览要在录入之前就把「同一请求方第二行」答给操作者，不能等到
// 发布那一刻，而两处判的必须是同一条规则。
func (body AuthorizationRuleBody) validate() error {
	_, err := declaredCancellationAuthorityByParty(body.CancellationAuthority)
	return err
}

// DeclaredCancellationPartyNamed 按 String() 的原词反查请求方：规范化文档与运营操作者面载荷里的 party 都是
// 那一个词，名单只在 String() 一处，这里只是反查。集合外（含空串）答 false。
func DeclaredCancellationPartyNamed(name string) (DeclaredCancellationParty, bool) {
	return closedCodeNamed(DeclaredCancellationParty.valid, DeclaredCancellationParty.String, name)
}

// canonicalizeAuthorizationRule 是 CanonicalizePublicationContent 里授权规则那一支：正文缺席与立不住的目录各答
// 自己那一格（补正文 / 改正文），不折成一个「算不出」。
func canonicalizeAuthorizationRule(content PublicationContent) (CanonicalPublicationContent, error) {
	if content.AuthorizationRule == nil {
		return CanonicalPublicationContent{}, ErrPublicationContentAbsent
	}
	if err := content.AuthorizationRule.validate(); err != nil {
		return CanonicalPublicationContent{}, err
	}
	return canonicalDigestOf(canonicalPublicationDocument{
		Canonicalization:  publicationCanonicalizationVersion,
		Kind:              content.Kind.String(),
		AuthorizationRule: canonicalAuthorizationRuleBodyOf(*content.AuthorizationRule),
	})
}

// canonicalAuthorizationRuleBody 镜像批文 declarations.cancellationAuthority 的形状（行键 party / rule）。目录按
// 请求方（枚举序）排序写出：目录按请求方成表（RuleFor 按请求方取、Declarations() 按请求方序交出），表单里换
// 行序不是换正文，摘要不该跟着变。
type canonicalAuthorizationRuleBody struct {
	CancellationAuthority []canonicalCancellationRule `json:"cancellationAuthority"`
}

type canonicalCancellationRule struct {
	Party string `json:"party"`
	Rule  string `json:"rule"`
}

func canonicalAuthorizationRuleBodyOf(body AuthorizationRuleBody) *canonicalAuthorizationRuleBody {
	rows := make([]CancellationAuthorityDeclaration, len(body.CancellationAuthority))
	copy(rows, body.CancellationAuthority)
	sort.Slice(rows, func(left, right int) bool {
		return rows[left].Party < rows[right].Party
	})
	document := &canonicalAuthorizationRuleBody{
		CancellationAuthority: make([]canonicalCancellationRule, 0, len(rows)),
	}
	for _, row := range rows {
		document.CancellationAuthority = append(document.CancellationAuthority,
			canonicalCancellationRule{Party: row.Party.String(), Rule: row.Rule.String()})
	}
	return document
}

// body 把文档里的一节折回领域正文。每一行过构造门；零行、同一请求方两行留给 validate——快照是数据，正文
// 立不立得住仍由构造门说。
func (document canonicalAuthorizationRuleBody) body() (AuthorizationRuleBody, error) {
	body := AuthorizationRuleBody{}
	for index, row := range document.CancellationAuthority {
		party, known := DeclaredCancellationPartyNamed(row.Party)
		if !known {
			return AuthorizationRuleBody{}, fmt.Errorf("cancellationAuthority[%d].party: %w: %q",
				index, ErrCancellationAuthorityNotConfigured, row.Party)
		}
		rule, err := NewRuleReference(row.Rule)
		if err != nil {
			return AuthorizationRuleBody{}, fmt.Errorf("cancellationAuthority[%d].rule: %w", index, err)
		}
		body.CancellationAuthority = append(body.CancellationAuthority,
			CancellationAuthorityDeclaration{Party: party, Rule: rule})
	}
	if err := body.validate(); err != nil {
		return AuthorizationRuleBody{}, err
	}
	return body, nil
}
