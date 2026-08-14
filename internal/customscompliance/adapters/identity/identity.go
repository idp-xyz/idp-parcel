// Package identity 为 customs-compliance 的标识签发端口提供生产实现。
//
// 标识取自 crypto/rand 的 128 位随机量，不用库序列，也不编码任何业务维度。两条理由：
//
//   - 递增序列会跨租户泄漏业务量——案件号相邻即可推知另一租户在这段时间建了几个案，
//     而按 ADR-0003 运营集团租户是最高数据隔离边界。给每个租户各开一条序列可以绕开
//     这一点，但那要求签发时先读租户，而端口签名里根本没有租户，那是刻意的：标识
//     不承载归属，归属由存储键表达。
//   - 签发不落在事务里。库序列的 nextval 不随事务回滚（这一点本来正合适），但走库
//     就意味着一次网络往返可以让「建案」在拿不到号这一步停住，而随机量不会失败。
//
// 前缀只为让两个标识空间在日志与数据里一眼分得开，不参与任何判断。
package identity

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

const (
	casePrefix    = "CC-CASE-"
	versionPrefix = "CC-SUBV-"
)

// CaseIdentities 实现 ports.CaseIdentityFactory。
type CaseIdentities struct{}

func NewCaseIdentities() *CaseIdentities {
	return &CaseIdentities{}
}

var _ ports.CaseIdentityFactory = (*CaseIdentities)(nil)

func (factory *CaseIdentities) MintCaseID(_ context.Context) (domain.CustomsCaseID, error) {
	minted, err := mint(casePrefix)
	if err != nil {
		return domain.CustomsCaseID{}, fmt.Errorf("mint case ID: %w", err)
	}
	return domain.NewCustomsCaseID(minted)
}

// DeclarationVersions 实现 ports.DeclarationVersionFactory。它与案件工厂分开，不是
// 同一个类型挂两个方法：两者由不同编排触发，合并会让建案的编排依赖它根本不签发的
// 提交版本号。
type DeclarationVersions struct{}

func NewDeclarationVersions() *DeclarationVersions {
	return &DeclarationVersions{}
}

var _ ports.DeclarationVersionFactory = (*DeclarationVersions)(nil)

func (factory *DeclarationVersions) NextSubmissionVersion(_ context.Context) (domain.SubmissionVersionID, error) {
	minted, err := mint(versionPrefix)
	if err != nil {
		return domain.SubmissionVersionID{}, fmt.Errorf("mint submission version ID: %w", err)
	}
	return domain.NewSubmissionVersionID(minted)
}

// mint 造一个带前缀的 128 位随机标识。随机源失败照原样上抛：签不出号时编排停在未决
// （两个用例都为此留了格），拿一个可预测的替代值顶上会让不可覆盖的提交版本撞号。
func mint(prefix string) (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return prefix + hex.EncodeToString(raw), nil
}
