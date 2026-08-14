// Package identity 是 visibility-exception 七个标识签发端口的生产实现。
//
// 标识是机制半边：签发一个不重的不透明编号既不需要租户参数，也不需要任何商业规则，
// 因此这里不存在「等真实参数」的一格——七个端口今天就该有真实现，而不是测试替身。
//
// 七个工厂各自成型，不合并成一个「全能签发器」。端口注释已把理由写死：投影派生、
// 视图派生、发作期、通知、处置请求、预测与追偿由不同用例触发，合并会让一个编排
// 依赖它根本不签发的身份。共用内核（internal/platform/identity）不改变这一点——
// 装配时每个类型仍只交得出它自己那一个标识。
//
// 编码、ctx 处置与熵源注入三样都归内核，理由写在那里，本包不复述第二遍。
package identity

import (
	"context"

	platformidentity "go.idp.xyz/idp-parcel/internal/platform/identity"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

// 各类标识的前缀。取值只是日志与工单里的可读性约定，不承载业务含义，也不参与任何
// 判断：领域侧七个 ID 已是互不相通的 Go 类型，张冠李戴在编译期就被拦住，前缀是给
// 人看的那一份。分隔符归拼接处所有，因此这里一律不带横线。
const (
	projectionPrefix   = "PRJ"
	customerViewPrefix = "CVW"
	episodePrefix      = "EPS"
	notificationPrefix = "NTF"
	dispositionPrefix  = "DRQ"
	etaPrefix          = "ETA"
	recoveryPrefix     = "RCV"
)

// ProjectionVersions 实现 ports.ProjectionIdentityFactory。
type ProjectionVersions struct {
	minter platformidentity.Minter
}

func NewProjectionVersions(options ...platformidentity.Option) (*ProjectionVersions, error) {
	minter, err := platformidentity.NewMinter(projectionPrefix, options...)
	if err != nil {
		return nil, err
	}
	return &ProjectionVersions{minter: minter}, nil
}

var _ ports.ProjectionIdentityFactory = (*ProjectionVersions)(nil)

// NextProjectionVersionID 签发一个投影版本标识。
//
// ctx 不参与签发：内核只读本机熵源，没有可取消的等待，也没有跨进程往返。端口留着
// ctx 是给「由库序列签发」那类实现的，本实现如实不用。本包各方法同此。
func (factory *ProjectionVersions) NextProjectionVersionID(
	_ context.Context,
) (domain.ProjectionVersionID, error) {
	value, err := factory.minter.Next()
	if err != nil {
		return domain.ProjectionVersionID{}, err
	}
	return domain.NewProjectionVersionID(value)
}

// CustomerViewVersions 实现 ports.CustomerViewIdentityFactory。
type CustomerViewVersions struct {
	minter platformidentity.Minter
}

func NewCustomerViewVersions(options ...platformidentity.Option) (*CustomerViewVersions, error) {
	minter, err := platformidentity.NewMinter(customerViewPrefix, options...)
	if err != nil {
		return nil, err
	}
	return &CustomerViewVersions{minter: minter}, nil
}

var _ ports.CustomerViewIdentityFactory = (*CustomerViewVersions)(nil)

func (factory *CustomerViewVersions) NextCustomerViewVersionID(
	_ context.Context,
) (domain.CustomerViewVersionID, error) {
	value, err := factory.minter.Next()
	if err != nil {
		return domain.CustomerViewVersionID{}, err
	}
	return domain.NewCustomerViewVersionID(value)
}

// SignalEpisodes 实现 ports.SignalEpisodeIdentityFactory。
type SignalEpisodes struct {
	minter platformidentity.Minter
}

func NewSignalEpisodes(options ...platformidentity.Option) (*SignalEpisodes, error) {
	minter, err := platformidentity.NewMinter(episodePrefix, options...)
	if err != nil {
		return nil, err
	}
	return &SignalEpisodes{minter: minter}, nil
}

var _ ports.SignalEpisodeIdentityFactory = (*SignalEpisodes)(nil)

func (factory *SignalEpisodes) NextEpisodeID(_ context.Context) (domain.EpisodeID, error) {
	value, err := factory.minter.Next()
	if err != nil {
		return domain.EpisodeID{}, err
	}
	return domain.NewEpisodeID(value)
}

// Notifications 实现 ports.NotificationIdentityFactory。
type Notifications struct {
	minter platformidentity.Minter
}

func NewNotifications(options ...platformidentity.Option) (*Notifications, error) {
	minter, err := platformidentity.NewMinter(notificationPrefix, options...)
	if err != nil {
		return nil, err
	}
	return &Notifications{minter: minter}, nil
}

var _ ports.NotificationIdentityFactory = (*Notifications)(nil)

func (factory *Notifications) NextNotificationID(_ context.Context) (domain.NotificationID, error) {
	value, err := factory.minter.Next()
	if err != nil {
		return domain.NotificationID{}, err
	}
	return domain.NewNotificationID(value)
}

// DispositionRequests 实现 ports.DispositionRequestIdentityFactory。
type DispositionRequests struct {
	minter platformidentity.Minter
}

func NewDispositionRequests(options ...platformidentity.Option) (*DispositionRequests, error) {
	minter, err := platformidentity.NewMinter(dispositionPrefix, options...)
	if err != nil {
		return nil, err
	}
	return &DispositionRequests{minter: minter}, nil
}

var _ ports.DispositionRequestIdentityFactory = (*DispositionRequests)(nil)

func (factory *DispositionRequests) NextDispositionRequestID(
	_ context.Context,
) (domain.DispositionRequestID, error) {
	value, err := factory.minter.Next()
	if err != nil {
		return domain.DispositionRequestID{}, err
	}
	return domain.NewDispositionRequestID(value)
}

// ETAVersions 实现 ports.ETAIdentityFactory。
type ETAVersions struct {
	minter platformidentity.Minter
}

func NewETAVersions(options ...platformidentity.Option) (*ETAVersions, error) {
	minter, err := platformidentity.NewMinter(etaPrefix, options...)
	if err != nil {
		return nil, err
	}
	return &ETAVersions{minter: minter}, nil
}

var _ ports.ETAIdentityFactory = (*ETAVersions)(nil)

func (factory *ETAVersions) NextETAVersionID(_ context.Context) (domain.ETAVersionID, error) {
	value, err := factory.minter.Next()
	if err != nil {
		return domain.ETAVersionID{}, err
	}
	return domain.NewETAVersionID(value)
}

// RecoveryMatters 实现 ports.RecoveryIdentityFactory。
type RecoveryMatters struct {
	minter platformidentity.Minter
}

func NewRecoveryMatters(options ...platformidentity.Option) (*RecoveryMatters, error) {
	minter, err := platformidentity.NewMinter(recoveryPrefix, options...)
	if err != nil {
		return nil, err
	}
	return &RecoveryMatters{minter: minter}, nil
}

var _ ports.RecoveryIdentityFactory = (*RecoveryMatters)(nil)

func (factory *RecoveryMatters) NextRecoveryMatterID(
	_ context.Context,
) (domain.RecoveryMatterID, error) {
	value, err := factory.minter.Next()
	if err != nil {
		return domain.RecoveryMatterID{}, err
	}
	return domain.NewRecoveryMatterID(value)
}
