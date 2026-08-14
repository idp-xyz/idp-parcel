package identity_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	platformidentity "go.idp.xyz/idp-parcel/internal/platform/identity"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/identity"
)

// factory 抹掉七个工厂各自的构造器与返回类型，让同一组断言逐个工厂跑一遍。七个实现
// 只差一个前缀，逐个手写断言正是抄漏的来处——本包用例守的就是这一类抄漏。
type factory struct {
	name string
	// prefix 是该工厂应当打上的前缀。
	prefix string
	mint   func(...platformidentity.Option) (string, error)
}

func allFactories() []factory {
	return []factory{
		{"投影版本", "PRJ", func(options ...platformidentity.Option) (string, error) {
			built, err := identity.NewProjectionVersions(options...)
			if err != nil {
				return "", err
			}
			value, err := built.NextProjectionVersionID(context.Background())
			return value.String(), err
		}},
		{"客户视图版本", "CVW", func(options ...platformidentity.Option) (string, error) {
			built, err := identity.NewCustomerViewVersions(options...)
			if err != nil {
				return "", err
			}
			value, err := built.NextCustomerViewVersionID(context.Background())
			return value.String(), err
		}},
		{"发作期", "EPS", func(options ...platformidentity.Option) (string, error) {
			built, err := identity.NewSignalEpisodes(options...)
			if err != nil {
				return "", err
			}
			value, err := built.NextEpisodeID(context.Background())
			return value.String(), err
		}},
		{"通知", "NTF", func(options ...platformidentity.Option) (string, error) {
			built, err := identity.NewNotifications(options...)
			if err != nil {
				return "", err
			}
			value, err := built.NextNotificationID(context.Background())
			return value.String(), err
		}},
		{"处置请求", "DRQ", func(options ...platformidentity.Option) (string, error) {
			built, err := identity.NewDispositionRequests(options...)
			if err != nil {
				return "", err
			}
			value, err := built.NextDispositionRequestID(context.Background())
			return value.String(), err
		}},
		{"预测版本", "ETA", func(options ...platformidentity.Option) (string, error) {
			built, err := identity.NewETAVersions(options...)
			if err != nil {
				return "", err
			}
			value, err := built.NextETAVersionID(context.Background())
			return value.String(), err
		}},
		{"追偿事项", "RCV", func(options ...platformidentity.Option) (string, error) {
			built, err := identity.NewRecoveryMatters(options...)
			if err != nil {
				return "", err
			}
			value, err := built.NextRecoveryMatterID(context.Background())
			return value.String(), err
		}},
	}
}

// 不传选项走的正是生产装配那条路（内核默认 crypto/rand.Reader）。
func TestEachFactoryMintsItsOwnPrefixAndNeverRepeats(t *testing.T) {
	seen := make(map[string]string)
	for _, subject := range allFactories() {
		for range 64 {
			value, err := subject.mint()
			if err != nil {
				t.Fatalf("%s 签发失败：%v", subject.name, err)
			}
			if !strings.HasPrefix(value, subject.prefix+"-") {
				t.Fatalf("%s 的标识 %q 没打上前缀 %q", subject.name, value, subject.prefix)
			}
			// 前缀 + 横线 + 26 字符随机段。长度失守说明熵位或编码被改动过。
			if len(value) != len(subject.prefix)+1+26 {
				t.Fatalf("%s 的标识 %q 长度 = %d", subject.name, value, len(value))
			}
			if owner, repeated := seen[value]; repeated {
				t.Fatalf("%s 与 %s 撞出同一标识 %q", subject.name, owner, value)
			}
			seen[value] = subject.name
		}
	}
}

type failingEntropy struct{ err error }

func (source failingEntropy) Read([]byte) (int, error) { return 0, source.err }

func TestEntropyFailureIsReportedNotPapered(t *testing.T) {
	broken := errors.New("熵池不可用")
	for _, subject := range allFactories() {
		value, err := subject.mint(platformidentity.WithEntropy(failingEntropy{err: broken}))
		if !errors.Is(err, broken) {
			t.Fatalf("%s 吞掉了熵源故障：err=%v", subject.name, err)
		}
		if value != "" {
			t.Fatalf("%s 在熵源故障下仍交出了标识 %q", subject.name, value)
		}
	}
}

// 熵源只给出半份随机位时同样不得签发：短读下拼出的标识只有一半熵，而它看起来与
// 正常标识毫无分别。
func TestShortEntropyReadIsRejected(t *testing.T) {
	for _, subject := range allFactories() {
		value, err := subject.mint(platformidentity.WithEntropy(strings.NewReader("只有八字节")))
		if err == nil {
			t.Fatalf("%s 用短读的熵签出了 %q", subject.name, value)
		}
	}
}

// nil 熵源在构造期就被内核拦下，七个工厂都不得把它静默回落成默认熵源——回落会让
// 一处传错的装配看起来一切正常，而它本来要换的那个熵源根本没生效。
func TestNilEntropyIsRefusedAtConstruction(t *testing.T) {
	for _, subject := range allFactories() {
		value, err := subject.mint(platformidentity.WithEntropy(nil))
		if !errors.Is(err, platformidentity.ErrInvalidEntropy) {
			t.Fatalf("%s 接受了 nil 熵源：err=%v value=%q", subject.name, err, value)
		}
	}
}
