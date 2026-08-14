// Package identity 是 parcel-shipment 五个标识签发端口的生产实现。
//
// 标识是机制半边：签发一个不重的不透明编号既不需要租户参数，也不需要任何商业规则，
// 因此这里没有「等真实参数」的一格——五个端口今天就该有真实现，而不是测试替身。
//
// 五个工厂各自成型，不合并成一个全能签发器。端口注释已把理由写死：建单期、决定期、
// 承诺期、终局期与修订期由不同用例触发，合并会让一个编排依赖它根本不签发的身份。共用
// 内核（internal/platform/identity）不改变这一点——装配时每个类型仍只交得出它自己那
// 一两个标识。
package identity

import (
	"context"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	platformidentity "go.idp.xyz/idp-parcel/internal/platform/identity"
)

// 各类标识的前缀。取值只是日志与工单里的可读性约定，不承载业务含义，也不参与任何判断：
// 领域侧五个 ID 已是互不相通的 Go 类型，张冠李戴在编译期就被拦住，前缀是给人看的那一份。
const (
	submissionVersionPrefix      = "SUBV"
	acceptanceDecisionTaskPrefix = "ADTK"
	acceptanceDecisionPrefix     = "ADEC"
	commitmentVersionPrefix      = "CMTV"
	finalOutcomeVersionPrefix    = "FINV"
	sourceDataVersionPrefix      = "SDV"
)

// SubmissionIdentities 实现 ports.SubmissionIdentityFactory。它是五个工厂里唯一签发两个
// 标识的——端口本身就把这两个并在一起，因为建单期同一次编排要它们俩。
type SubmissionIdentities struct {
	versions platformidentity.Minter
	tasks    platformidentity.Minter
}

func NewSubmissionIdentities(options ...platformidentity.Option) (*SubmissionIdentities, error) {
	versions, err := platformidentity.NewMinter(submissionVersionPrefix, options...)
	if err != nil {
		return nil, err
	}
	tasks, err := platformidentity.NewMinter(acceptanceDecisionTaskPrefix, options...)
	if err != nil {
		return nil, err
	}
	return &SubmissionIdentities{versions: versions, tasks: tasks}, nil
}

var _ ports.SubmissionIdentityFactory = (*SubmissionIdentities)(nil)

// NextSubmissionVersionID 签发一个提交版本标识。
//
// ctx 不参与签发：本实现只读本机熵源，没有可取消的等待，也没有跨进程往返。端口留着 ctx
// 是给「由库序列签发」那类实现的，本实现如实不用。本包各方法同此。
func (factory *SubmissionIdentities) NextSubmissionVersionID(
	_ context.Context,
) (domain.SubmissionVersionID, error) {
	value, err := factory.versions.Next()
	if err != nil {
		return domain.SubmissionVersionID{}, err
	}
	return domain.NewSubmissionVersionID(value)
}

func (factory *SubmissionIdentities) NextAcceptanceDecisionTaskID(
	_ context.Context,
) (domain.AcceptanceDecisionTaskID, error) {
	value, err := factory.tasks.Next()
	if err != nil {
		return domain.AcceptanceDecisionTaskID{}, err
	}
	return domain.NewAcceptanceDecisionTaskID(value)
}

// AcceptanceDecisions 实现 ports.AcceptanceDecisionIdentity。
type AcceptanceDecisions struct {
	minter platformidentity.Minter
}

func NewAcceptanceDecisions(options ...platformidentity.Option) (*AcceptanceDecisions, error) {
	minter, err := platformidentity.NewMinter(acceptanceDecisionPrefix, options...)
	if err != nil {
		return nil, err
	}
	return &AcceptanceDecisions{minter: minter}, nil
}

var _ ports.AcceptanceDecisionIdentity = (*AcceptanceDecisions)(nil)

func (factory *AcceptanceDecisions) NextAcceptanceDecisionID(
	_ context.Context,
) (domain.AcceptanceDecisionID, error) {
	value, err := factory.minter.Next()
	if err != nil {
		return domain.AcceptanceDecisionID{}, err
	}
	return domain.NewAcceptanceDecisionID(value)
}

// CommitmentVersions 实现 ports.CommitmentIdentityFactory。
type CommitmentVersions struct {
	minter platformidentity.Minter
}

func NewCommitmentVersions(options ...platformidentity.Option) (*CommitmentVersions, error) {
	minter, err := platformidentity.NewMinter(commitmentVersionPrefix, options...)
	if err != nil {
		return nil, err
	}
	return &CommitmentVersions{minter: minter}, nil
}

var _ ports.CommitmentIdentityFactory = (*CommitmentVersions)(nil)

func (factory *CommitmentVersions) NextCommitmentVersionID(
	_ context.Context,
) (domain.CommitmentVersionID, error) {
	value, err := factory.minter.Next()
	if err != nil {
		return domain.CommitmentVersionID{}, err
	}
	return domain.NewCommitmentVersionID(value)
}

// FinalOutcomeVersions 实现 ports.FinalIdentityFactory。
type FinalOutcomeVersions struct {
	minter platformidentity.Minter
}

func NewFinalOutcomeVersions(options ...platformidentity.Option) (*FinalOutcomeVersions, error) {
	minter, err := platformidentity.NewMinter(finalOutcomeVersionPrefix, options...)
	if err != nil {
		return nil, err
	}
	return &FinalOutcomeVersions{minter: minter}, nil
}

var _ ports.FinalIdentityFactory = (*FinalOutcomeVersions)(nil)

func (factory *FinalOutcomeVersions) NextFinalOutcomeVersionID(
	_ context.Context,
) (domain.FinalOutcomeVersionID, error) {
	value, err := factory.minter.Next()
	if err != nil {
		return domain.FinalOutcomeVersionID{}, err
	}
	return domain.NewFinalOutcomeVersionID(value)
}

// SourceDataVersions 实现 ports.SourceDataVersionIdentity。
type SourceDataVersions struct {
	minter platformidentity.Minter
}

func NewSourceDataVersions(options ...platformidentity.Option) (*SourceDataVersions, error) {
	minter, err := platformidentity.NewMinter(sourceDataVersionPrefix, options...)
	if err != nil {
		return nil, err
	}
	return &SourceDataVersions{minter: minter}, nil
}

var _ ports.SourceDataVersionIdentity = (*SourceDataVersions)(nil)

func (factory *SourceDataVersions) NextSourceDataVersionID(
	_ context.Context,
) (domain.SourceDataVersionID, error) {
	value, err := factory.minter.Next()
	if err != nil {
		return domain.SourceDataVersionID{}, err
	}
	return domain.NewSourceDataVersionID(value)
}
