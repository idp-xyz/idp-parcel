// Package identity 为 customs-compliance 的标识签发端口提供生产实现。
//
// 签发内核共用 internal/platform/identity；本包只决定两件属本上下文的事：两个标识空间
// 各取什么前缀，以及签出的字符串交给哪个领域构造函数。
//
// 内核里已经写下的取舍不在这里复述（为什么取随机不取库序列、为什么用 base32、为什么
// 不收 ctx、为什么熵源必须可注入）。要改那些去改内核，改在这里只会让本上下文与其余
// 三个上下文重新分叉。
package identity

import (
	"context"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
	platformidentity "go.idp.xyz/idp-parcel/internal/platform/identity"
)

// 前缀只为让两个标识空间在日志与工单里一眼分得开，不参与任何判断。它们不含分隔符，
// 也不含上下文名——领域侧各标识已是互不相通的 Go 类型，张冠李戴编译期就拦得住。
//
// 提交版本这一格取 `DECLV` 而不是自然缩写 `SUBV`：后者已被 parcel-shipment 的提交
// 版本占用。前缀去掉上下文名之后，两个上下文的同名概念就会在同一个命名空间里撞上，
// 而前缀的全部用处正是让人在日志里分辨它们。
const (
	casePrefix    = "CASE"
	versionPrefix = "DECLV"
)

// CaseIdentities 实现 ports.CaseIdentityFactory。
type CaseIdentities struct {
	minter platformidentity.Minter
}

func NewCaseIdentities(options ...platformidentity.Option) (*CaseIdentities, error) {
	minter, err := platformidentity.NewMinter(casePrefix, options...)
	if err != nil {
		return nil, fmt.Errorf("customs compliance identity: %w", err)
	}
	return &CaseIdentities{minter: minter}, nil
}

var _ ports.CaseIdentityFactory = (*CaseIdentities)(nil)

func (factory *CaseIdentities) MintCaseID(_ context.Context) (domain.CustomsCaseID, error) {
	minted, err := factory.minter.Next()
	if err != nil {
		return domain.CustomsCaseID{}, fmt.Errorf("mint case ID: %w", err)
	}
	return domain.NewCustomsCaseID(minted)
}

// DeclarationVersions 实现 ports.DeclarationVersionFactory。它与案件工厂分开，不是
// 同一个类型挂两个方法：两者由不同编排触发，合并会让建案的编排依赖它根本不签发的
// 提交版本号。
type DeclarationVersions struct {
	minter platformidentity.Minter
}

func NewDeclarationVersions(options ...platformidentity.Option) (*DeclarationVersions, error) {
	minter, err := platformidentity.NewMinter(versionPrefix, options...)
	if err != nil {
		return nil, fmt.Errorf("customs compliance identity: %w", err)
	}
	return &DeclarationVersions{minter: minter}, nil
}

var _ ports.DeclarationVersionFactory = (*DeclarationVersions)(nil)

// NextSubmissionVersion 签发一个新的提交版本号。签不出来时如实上抛：提交版本不可覆盖，
// 拿一个可预测的替代值顶上会撞号。
func (factory *DeclarationVersions) NextSubmissionVersion(
	_ context.Context,
) (domain.SubmissionVersionID, error) {
	minted, err := factory.minter.Next()
	if err != nil {
		return domain.SubmissionVersionID{}, fmt.Errorf("mint submission version ID: %w", err)
	}
	return domain.NewSubmissionVersionID(minted)
}
