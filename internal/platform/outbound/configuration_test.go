package outbound_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/platform/outbound"
)

// 「未给出超时即不得发起调用」这条要守得住，前提是给不出一个「没给超时」却能用的
// CallTimeout。零值与负值都得在构造期就拒掉——放过去之后它在调用点表现为一次立刻超时，
// 而立刻超时会被判成`答案未确定`，那是不得重发的那一格。
func TestACallTimeoutMustBeGivenAndPositive(t *testing.T) {
	t.Parallel()

	for _, duration := range []time.Duration{0, -time.Second} {
		timeout, err := outbound.NewCallTimeout(duration)
		if !errors.Is(err, outbound.ErrOutboundNotConfigured) {
			t.Errorf("超时 %v 应被拒为未配置，实得 %v", duration, err)
		}
		if timeout.Configured() {
			t.Errorf("超时 %v 被拒之后不该报告已配置", duration)
		}
	}

	timeout, err := outbound.NewCallTimeout(time.Second)
	if err != nil {
		t.Fatalf("正数超时应被接受：%v", err)
	}
	if !timeout.Configured() || timeout.Duration() != time.Second {
		t.Errorf("已配置超时应如实交回原值，实得 configured=%v duration=%v",
			timeout.Configured(), timeout.Duration())
	}
}

// 生产装配在真渠道就位之前交入的就是这个零值。它必须答「未配置」而不是看起来可用——
// 后者会让一次本该如实拒绝的调用带着空地址发出去。
func TestTheZeroChannelConfigurationIsNotConfigured(t *testing.T) {
	t.Parallel()

	var unset outbound.ChannelConfiguration
	if unset.Configured() {
		t.Error("零值配置不该报告已配置")
	}
	if unset.Timeout().Configured() {
		t.Error("零值配置里的超时不该报告已配置")
	}
}

func TestAChannelConfigurationRejectsAnyMissingPiece(t *testing.T) {
	t.Parallel()

	given, err := outbound.NewCallTimeout(time.Second)
	if err != nil {
		t.Fatalf("构造超时：%v", err)
	}

	cases := map[string]struct {
		endpoint   string
		credential string
		timeout    outbound.CallTimeout
	}{
		"缺端点":    {endpoint: "", credential: "credential-location", timeout: given},
		"端点只有空白": {endpoint: "   ", credential: "credential-location", timeout: given},
		"缺凭证位置":  {endpoint: "endpoint-location", credential: "", timeout: given},
		"凭证只有空白": {endpoint: "endpoint-location", credential: "\t", timeout: given},
		"缺超时":    {endpoint: "endpoint-location", credential: "credential-location"},
	}

	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			configuration, err := outbound.NewChannelConfiguration(testCase.endpoint, testCase.credential, testCase.timeout)
			if !errors.Is(err, outbound.ErrOutboundNotConfigured) {
				t.Errorf("应被拒为未配置，实得 %v", err)
			}
			if configuration.Configured() {
				t.Error("被拒之后不该报告已配置")
			}
		})
	}
}

func TestAFullyGivenChannelConfigurationIsUsable(t *testing.T) {
	t.Parallel()

	timeout, err := outbound.NewCallTimeout(2 * time.Second)
	if err != nil {
		t.Fatalf("构造超时：%v", err)
	}
	configuration, err := outbound.NewChannelConfiguration("endpoint-location", "credential-location", timeout)
	if err != nil {
		t.Fatalf("三样齐备应被接受：%v", err)
	}
	if !configuration.Configured() {
		t.Error("三样齐备应报告已配置")
	}
	if configuration.EndpointReference() != "endpoint-location" ||
		configuration.CredentialReference() != "credential-location" ||
		configuration.Timeout().Duration() != 2*time.Second {
		t.Errorf("配置应如实交回所给的位置与上限，实得 endpoint=%q credential=%q timeout=%v",
			configuration.EndpointReference(), configuration.CredentialReference(), configuration.Timeout().Duration())
	}
}
