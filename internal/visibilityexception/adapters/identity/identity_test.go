package identity_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/identity"
)

// mintedValue 抹掉七个工厂各自的返回类型，让同一组断言能逐个工厂跑一遍。七个实现
// 只差一个前缀和一个构造器，逐个手写断言正是抄漏的来处。
type mintedValue struct {
	name string
	// prefix 是该工厂应当打上的前缀。
	prefix string
	mint   func(identity.Option) (string, error)
}

func allFactories() []mintedValue {
	return []mintedValue{
		{"投影版本", "PRJ", func(option identity.Option) (string, error) {
			value, err := identity.NewProjectionFactory(option).NextProjectionVersionID(context.Background())
			return value.String(), err
		}},
		{"客户视图版本", "CVW", func(option identity.Option) (string, error) {
			value, err := identity.NewCustomerViewFactory(option).NextCustomerViewVersionID(context.Background())
			return value.String(), err
		}},
		{"发作期", "EPS", func(option identity.Option) (string, error) {
			value, err := identity.NewSignalEpisodeFactory(option).NextEpisodeID(context.Background())
			return value.String(), err
		}},
		{"通知", "NTF", func(option identity.Option) (string, error) {
			value, err := identity.NewNotificationFactory(option).NextNotificationID(context.Background())
			return value.String(), err
		}},
		{"处置请求", "DRQ", func(option identity.Option) (string, error) {
			value, err := identity.NewDispositionRequestFactory(option).NextDispositionRequestID(context.Background())
			return value.String(), err
		}},
		{"预测版本", "ETA", func(option identity.Option) (string, error) {
			value, err := identity.NewETAFactory(option).NextETAVersionID(context.Background())
			return value.String(), err
		}},
		{"追偿事项", "RCV", func(option identity.Option) (string, error) {
			value, err := identity.NewRecoveryFactory(option).NextRecoveryMatterID(context.Background())
			return value.String(), err
		}},
	}
}

// 生产熵源即默认装配；传 nil Option 走的正是生产那条路。
func productionOption() identity.Option { return identity.WithEntropy(nil) }

func TestEachFactoryMintsItsOwnPrefixAndNeverRepeats(t *testing.T) {
	seen := make(map[string]string)
	for _, factory := range allFactories() {
		for range 64 {
			value, err := factory.mint(productionOption())
			if err != nil {
				t.Fatalf("%s 签发失败：%v", factory.name, err)
			}
			if !strings.HasPrefix(value, factory.prefix+"-") {
				t.Fatalf("%s 的标识 %q 没打上前缀 %q", factory.name, value, factory.prefix)
			}
			// 26 字符随机段 + 前缀 + 连字符：长度失守说明熵位或编码被改动过。
			if len(value) != len(factory.prefix)+1+26 {
				t.Fatalf("%s 的标识 %q 长度 = %d", factory.name, value, len(value))
			}
			if owner, repeated := seen[value]; repeated {
				t.Fatalf("%s 与 %s 撞出同一标识 %q", factory.name, owner, value)
			}
			seen[value] = factory.name
		}
	}
}

type failingEntropy struct{ err error }

func (source failingEntropy) Read([]byte) (int, error) { return 0, source.err }

func TestEntropyFailureIsReportedNotPapered(t *testing.T) {
	broken := errors.New("熵池不可用")
	for _, factory := range allFactories() {
		value, err := factory.mint(identity.WithEntropy(failingEntropy{err: broken}))
		if !errors.Is(err, broken) {
			t.Fatalf("%s 吞掉了熵源故障：err=%v", factory.name, err)
		}
		if value != "" {
			t.Fatalf("%s 在熵源故障下仍交出了标识 %q", factory.name, value)
		}
	}
}

// 熵源只给出半份随机位时同样不得签发：短读下拼出的标识只有一半熵，而它看起来与
// 正常标识毫无分别。
func TestShortEntropyReadIsRejected(t *testing.T) {
	for _, factory := range allFactories() {
		value, err := factory.mint(identity.WithEntropy(strings.NewReader("只有八字节")))
		if err == nil {
			t.Fatalf("%s 用短读的熵签出了 %q", factory.name, value)
		}
	}
}
