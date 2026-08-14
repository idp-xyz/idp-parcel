// Package identity 是 visibility-exception 七个标识签发端口的生产实现。
//
// 标识是机制半边：签发一个不重的不透明编号既不需要租户参数，也不需要任何商业规则，
// 因此这里不存在「等真实参数」的一格——七个端口今天就该有真实现，而不是测试替身。
//
// 七个工厂各自成型，不合并成一个「全能签发器」。端口注释已把理由写死：投影派生、
// 视图派生、发作期、通知、处置请求、预测与追偿由不同用例触发，合并会让一个编排
// 依赖它根本不签发的身份。这里照办——每个类型只实现一个端口，装配时也只能拿到
// 它要的那一个。
package identity

import (
	"context"
	"crypto/rand"
	"encoding/base32"
	"fmt"
	"io"

	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

// entropyBytes 是每个标识的随机位数（128 位）。
//
// 取随机而不取自增序列，是因为标识跨租户共用一个命名空间：自增号能让一个租户从自己
// 拿到的编号推出另一个租户的业务量，而租户是最高数据隔离边界（ADR-0003）——那条边界
// 不该被一个编号格式泄掉。取密码学随机而不取时间戳派生，理由同上：可预测即可枚举。
//
// 128 位下即便每秒签发一亿个标识、连签一百年，碰撞概率仍在 10^-18 量级，因此签发方
// 不需要与库或彼此协调就能担保不重——这正是编排能在事务外先取标识再落库的前提。
const entropyBytes = 16

// idEncoding 把随机位编成不带填充的大写 base32：全字母数字、大小写不敏感的场合不会
// 二义，人念得出也抄得对（运维照着工单念一个标识是真实场景）。16 字节编成 26 字符。
var idEncoding = base32.StdEncoding.WithPadding(base32.NoPadding)

// minter 是七个工厂共用的签发内核：前缀 + 随机段。
//
// 前缀只为可读性——领域侧七个 ID 已是互不相通的 Go 类型，编译期就拦得住张冠李戴；
// 但日志、库行和工单里它们都退化成字符串，前缀让人一眼看出手上这个编号是哪一类。
type minter struct {
	prefix  string
	entropy io.Reader
}

func newMinter(prefix string, options []Option) minter {
	minted := minter{prefix: prefix, entropy: rand.Reader}
	for _, option := range options {
		option(&minted)
	}
	return minted
}

// next 签发一个标识。熵源读不出来时如实报错，绝不退化到时间戳或计数器兜底——那种
// 兜底会在最需要唯一性的时刻（熵池异常）恰好交出最容易撞的标识。
func (minted minter) next() (string, error) {
	buffer := make([]byte, entropyBytes)
	if _, err := io.ReadFull(minted.entropy, buffer); err != nil {
		return "", fmt.Errorf("visibility exception identity: 读取熵源：%w", err)
	}
	return minted.prefix + "-" + idEncoding.EncodeToString(buffer), nil
}

// Option 调整签发器的装配。
type Option func(*minter)

// WithEntropy 替换熵源。生产装配不用它——默认已是 crypto/rand.Reader；它的用处是让
// 用例能证「熵源出错时如实报错」这一支，那一支拿真实熵源逼不出来。
func WithEntropy(entropy io.Reader) Option {
	return func(minted *minter) {
		if entropy != nil {
			minted.entropy = entropy
		}
	}
}

// 各类标识的前缀。取值只是可读性约定，不承载业务含义，也不参与任何判断。
const (
	projectionPrefix   = "PRJ"
	customerViewPrefix = "CVW"
	episodePrefix      = "EPS"
	notificationPrefix = "NTF"
	dispositionPrefix  = "DRQ"
	etaPrefix          = "ETA"
	recoveryPrefix     = "RCV"
)

// ProjectionFactory 签发投影版本标识。
type ProjectionFactory struct{ minter minter }

func NewProjectionFactory(options ...Option) *ProjectionFactory {
	return &ProjectionFactory{minter: newMinter(projectionPrefix, options)}
}

var _ ports.ProjectionIdentityFactory = (*ProjectionFactory)(nil)

// NextProjectionVersionID 签发一个投影版本标识。
//
// ctx 不参与签发：本实现只读本机熵源，没有可取消的等待，也没有跨进程往返。端口留着
// ctx 是给「由库序列签发」那类实现用的，本实现如实不用。七个工厂同此。
func (factory *ProjectionFactory) NextProjectionVersionID(_ context.Context) (domain.ProjectionVersionID, error) {
	value, err := factory.minter.next()
	if err != nil {
		return domain.ProjectionVersionID{}, err
	}
	return domain.NewProjectionVersionID(value)
}

// CustomerViewFactory 签发客户视图版本标识。
type CustomerViewFactory struct{ minter minter }

func NewCustomerViewFactory(options ...Option) *CustomerViewFactory {
	return &CustomerViewFactory{minter: newMinter(customerViewPrefix, options)}
}

var _ ports.CustomerViewIdentityFactory = (*CustomerViewFactory)(nil)

func (factory *CustomerViewFactory) NextCustomerViewVersionID(_ context.Context) (domain.CustomerViewVersionID, error) {
	value, err := factory.minter.next()
	if err != nil {
		return domain.CustomerViewVersionID{}, err
	}
	return domain.NewCustomerViewVersionID(value)
}

// SignalEpisodeFactory 签发发作期标识。
type SignalEpisodeFactory struct{ minter minter }

func NewSignalEpisodeFactory(options ...Option) *SignalEpisodeFactory {
	return &SignalEpisodeFactory{minter: newMinter(episodePrefix, options)}
}

var _ ports.SignalEpisodeIdentityFactory = (*SignalEpisodeFactory)(nil)

func (factory *SignalEpisodeFactory) NextEpisodeID(_ context.Context) (domain.EpisodeID, error) {
	value, err := factory.minter.next()
	if err != nil {
		return domain.EpisodeID{}, err
	}
	return domain.NewEpisodeID(value)
}

// NotificationFactory 签发通知标识。
type NotificationFactory struct{ minter minter }

func NewNotificationFactory(options ...Option) *NotificationFactory {
	return &NotificationFactory{minter: newMinter(notificationPrefix, options)}
}

var _ ports.NotificationIdentityFactory = (*NotificationFactory)(nil)

func (factory *NotificationFactory) NextNotificationID(_ context.Context) (domain.NotificationID, error) {
	value, err := factory.minter.next()
	if err != nil {
		return domain.NotificationID{}, err
	}
	return domain.NewNotificationID(value)
}

// DispositionRequestFactory 签发处置请求标识。
type DispositionRequestFactory struct{ minter minter }

func NewDispositionRequestFactory(options ...Option) *DispositionRequestFactory {
	return &DispositionRequestFactory{minter: newMinter(dispositionPrefix, options)}
}

var _ ports.DispositionRequestIdentityFactory = (*DispositionRequestFactory)(nil)

func (factory *DispositionRequestFactory) NextDispositionRequestID(_ context.Context) (domain.DispositionRequestID, error) {
	value, err := factory.minter.next()
	if err != nil {
		return domain.DispositionRequestID{}, err
	}
	return domain.NewDispositionRequestID(value)
}

// ETAFactory 签发预测版本标识。
type ETAFactory struct{ minter minter }

func NewETAFactory(options ...Option) *ETAFactory {
	return &ETAFactory{minter: newMinter(etaPrefix, options)}
}

var _ ports.ETAIdentityFactory = (*ETAFactory)(nil)

func (factory *ETAFactory) NextETAVersionID(_ context.Context) (domain.ETAVersionID, error) {
	value, err := factory.minter.next()
	if err != nil {
		return domain.ETAVersionID{}, err
	}
	return domain.NewETAVersionID(value)
}

// RecoveryFactory 签发追偿事项标识。
type RecoveryFactory struct{ minter minter }

func NewRecoveryFactory(options ...Option) *RecoveryFactory {
	return &RecoveryFactory{minter: newMinter(recoveryPrefix, options)}
}

var _ ports.RecoveryIdentityFactory = (*RecoveryFactory)(nil)

func (factory *RecoveryFactory) NextRecoveryMatterID(_ context.Context) (domain.RecoveryMatterID, error) {
	value, err := factory.minter.next()
	if err != nil {
		return domain.RecoveryMatterID{}, err
	}
	return domain.NewRecoveryMatterID(value)
}
