// Package identity 是各上下文标识签发器共用的内核：把一段密码学随机量编成一个可读的
// 不透明标识。
//
// 它到字符串为止。领域侧的 `NewXxxID` 由各上下文自己调——标识类型归各上下文所有，平台
// 件不认识它们，也不该为了认识它们而反向依赖十个业务包。
//
// 提炼自 node-operations、visibility-exception 与 customs-compliance 三处同形实现。被抹平
// 的只有编码、ctx 处置与熵源注入这三样任意变异；各上下文「一个端口一个类型、不合并成全能
// 签发器」那条约束不在此列——它防的是「一个编排依赖它根本不签发的身份」，与内核共用无关。
package identity

import (
	"crypto/rand"
	"encoding/base32"
	"errors"
	"fmt"
	"io"
	"strings"
)

// entropyBytes 是随机段的字节数（128 位）。
//
// 取随机而不取库序列，因为标识跨租户共用一个命名空间：自增号能让一个租户从自己拿到的
// 编号推出另一个租户的业务量，而按 ADR-0003 运营集团租户是最高数据隔离边界。128 位下
// 签发方不必与库或彼此协调就能担保不重，这正是编排可以在事务外先取标识再落库的前提。
const entropyBytes = 16

// encoding 用 RFC 4648 base32 的大写字母表且不加填充。
//
// 选它而不选 hex 的理由是人念得出也抄得对——运维照着工单念一个标识是真实场景。该字母表
// 是 A-Z 与 2-7，本身就不含 0、1、8、9，因此 0 与 O、1 与 I 不可能混；hex 的 0/O 恰好撞上
// 这一对。16 字节编成 26 字符，也比 hex 的 32 字符短。
var encoding = base32.StdEncoding.WithPadding(base32.NoPadding)

// separator 分开前缀与随机段。它归拼接处所有，不写进各上下文的前缀常量里：否则前缀就会
// 长出「带不带尾横线」这种各写各的差别，而那正是提炼要消掉的东西。
const separator = "-"

var (
	ErrInvalidPrefix  = errors.New("platform identity: invalid prefix")
	ErrInvalidEntropy = errors.New("platform identity: invalid entropy reader")
	ErrUnusableMinter = errors.New("platform identity: minter is unusable")
)

// Minter 签发「前缀 + 随机段」形状的标识。零值不可用，必须经 NewMinter 构造。
type Minter struct {
	prefix  string
	entropy io.Reader
}

// NewMinter 构造签发内核。
//
// 前缀不得含分隔符：横线归拼接处所有，前缀里再出现一个就把段数变成了可变的，读的人无从
// 知道哪一段是类别。前缀也不该带上下文名——领域侧各标识已是互不相通的 Go 类型，编译期
// 就拦得住张冠李戴，前缀只为日志与工单里的可读性，再背一个上下文名只是变长。后一条这里
// 守不住（平台件不知道调用方属哪个上下文），只能靠约定。
func NewMinter(prefix string, options ...Option) (Minter, error) {
	if strings.TrimSpace(prefix) == "" {
		return Minter{}, fmt.Errorf("%w: prefix is blank", ErrInvalidPrefix)
	}
	if strings.Contains(prefix, separator) {
		return Minter{}, fmt.Errorf("%w: prefix %q contains %q", ErrInvalidPrefix, prefix, separator)
	}

	minter := Minter{prefix: prefix, entropy: rand.Reader}
	for index, option := range options {
		if option == nil {
			return Minter{}, fmt.Errorf("%w: option %d is nil", ErrInvalidEntropy, index)
		}
		if err := option(&minter); err != nil {
			return Minter{}, err
		}
	}
	return minter, nil
}

// Option 调整签发器的装配。
type Option func(*Minter) error

// WithEntropy 替换熵源。生产装配不用它——默认已是 crypto/rand.Reader；它的用处是让用例
// 能证「熵源出错时如实报错」那一支，那一支拿真实熵源逼不出来。
//
// nil 交回错误而不是静默回落到默认熵源：回落会让一处传错的装配看起来一切正常，而它本来
// 要换的那个熵源根本没生效。
func WithEntropy(entropy io.Reader) Option {
	return func(minter *Minter) error {
		if entropy == nil {
			return fmt.Errorf("%w: entropy reader is nil", ErrInvalidEntropy)
		}
		minter.entropy = entropy
		return nil
	}
}

// Next 签发一个标识。
//
// 签名不收 context：本实现只读本机熵源，没有可取消的等待，也没有跨进程往返，收一个 ctx
// 就是演戏。各上下文的端口仍带 ctx——那是留给「由库序列签发」那类实现的，本实现如实不用。
//
// 熵源读不满时如实报错，绝不退化到时间戳或计数器兜底：那种兜底会在最需要唯一性的时刻
// （熵池异常）恰好交出最容易撞的标识。
func (minter Minter) Next() (string, error) {
	if minter.entropy == nil {
		return "", fmt.Errorf("%w: construct it with NewMinter", ErrUnusableMinter)
	}

	buffer := make([]byte, entropyBytes)
	if _, err := io.ReadFull(minter.entropy, buffer); err != nil {
		return "", fmt.Errorf("platform identity: read entropy: %w", err)
	}
	return minter.prefix + separator + encoding.EncodeToString(buffer), nil
}
