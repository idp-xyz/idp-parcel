package domain_test

import (
	"testing"

	"go.idp.xyz/idp-parcel/internal/pilotgovernance/domain"
)

// 受控通道命令集合封闭在领域里：入口的词往返得回同一个取值，集合外的词答假而不是猜近似，
// 无效取值的 String() 是空串（enum 门禁另守「每个取值都被 String() 提到」）。

func TestChannelCommandRoundTripsThroughItsCommandLineSpelling(t *testing.T) {
	for _, command := range []domain.ChannelCommand{
		domain.ChannelCommandAuthorityInterval,
		domain.ChannelCommandSuspend,
		domain.ChannelCommandResume,
	} {
		parsed, ok := domain.ParseChannelCommand(command.String())
		if !ok || parsed != command {
			t.Fatalf("%q 往返 = (%v, %v)，要原取值", command.String(), parsed, ok)
		}
	}
	if domain.ChannelCommandInvalid.String() != "" {
		t.Fatalf("无效取值的 String() = %q，要空串", domain.ChannelCommandInvalid.String())
	}
}

func TestChannelCommandRefusesWordsOutsideTheClosedSet(t *testing.T) {
	for _, text := range []string{"", "takeover", "stage-review", "Suspend", " suspend"} {
		if parsed, ok := domain.ParseChannelCommand(text); ok || parsed != domain.ChannelCommandInvalid {
			t.Fatalf("%q 被译成 %v（ok=%v）；集合外的词不得进留痕", text, parsed, ok)
		}
	}
}
