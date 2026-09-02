package outbound_test

import (
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/platform/outbound"
)

// 举证门那一条。`确证未受理`是唯一准许重发的格，而重发一次取面单调用产生的是真实的供应商
// 成本与第二个运输标识——所以举不出实据时必须降下来，不能报错让适配器随手编一个实据填上。
func TestProvenNotAcceptedWithoutEvidenceFallsBackToUndetermined(t *testing.T) {
	t.Parallel()

	outcome := outbound.NotAccepted("")
	if got := outcome.Disposition(); got != outbound.AnswerUndetermined {
		t.Errorf("举不出实据应降为答案未确定，实得 %q", got)
	}
	if outcome.AdmitsResend() {
		t.Error("降级之后不该准许重发——这一格正是要拦住的那次重发")
	}
	if !outcome.RequiresQueryToSettle() {
		t.Error("降级之后应要求查询收口")
	}
	if evidence := outcome.Evidence(); evidence != "" {
		t.Errorf("降级之后不该留下实据，实得 %q", evidence)
	}
}

func TestProvenNotAcceptedWithEvidenceAdmitsResend(t *testing.T) {
	t.Parallel()

	outcome := outbound.NotAccepted("connection never established")
	if got := outcome.Disposition(); got != outbound.ProvenNotAccepted {
		t.Errorf("举得出实据应落确证未受理，实得 %q", got)
	}
	if !outcome.AdmitsResend() {
		t.Error("确证未受理应准许重发同一请求")
	}
	if outcome.Evidence() != "connection never established" {
		t.Errorf("实据应原样留下，实得 %q", outcome.Evidence())
	}
}

// 零值 fail-closed。适配器早退、或某条分支忘了赋值时交回的就是它，而它绝不能看起来像一个
// 正经答复——尤其不能准许重发。
func TestTheZeroOutcomeIsNeitherAnAnswerNorResendable(t *testing.T) {
	t.Parallel()

	var unset outbound.Outcome
	if got := unset.Disposition(); got != outbound.DispositionInvalid {
		t.Errorf("零值应是无效格，实得 %q", got)
	}
	if unset.AdmitsResend() || unset.RequiresQueryToSettle() {
		t.Error("零值不该准许重发，也不该要求查询——它连是不是一次调用结果都说不上")
	}
}

func TestEachConstructorYieldsItsOwnGrade(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		outcome outbound.Outcome
		want    outbound.Disposition
	}{
		"接受":    {outbound.Accept(), outbound.Accepted},
		"对端拒绝":  {outbound.Reject("channel declined"), outbound.Rejected},
		"答案未确定": {outbound.Undetermined(), outbound.AnswerUndetermined},
		"未配置":   {outbound.Unconfigured(), outbound.NotConfigured},
	}

	for name, testCase := range cases {
		if got := testCase.outcome.Disposition(); got != testCase.want {
			t.Errorf("%s：实得 %q，想要 %q", name, got, testCase.want)
		}
	}
}

// 拒绝不要求举证，与`确证未受理`不同：这一格的前提是对端确实答了，答复本身就是实据。有渠道
// 只回一个拒绝而不给原因，硬要求会逼适配器编一个。
func TestRejectionNeedsNoEvidenceAndStillDoesNotAdmitResend(t *testing.T) {
	t.Parallel()

	outcome := outbound.Reject("")
	if got := outcome.Disposition(); got != outbound.Rejected {
		t.Errorf("无原因的拒绝仍应落对端拒绝，实得 %q", got)
	}
	if outcome.AdmitsResend() {
		t.Error("对端已经答了拒绝，重发同一请求只会再被拒一次；这一格的续办是交业务")
	}
	if outcome.RequiresQueryToSettle() {
		t.Error("对端已经答了，没有什么可查的")
	}
}

func TestAnUnconfiguredChannelIsRefusedBeforeAnyCall(t *testing.T) {
	t.Parallel()

	var unset outbound.ChannelConfiguration
	outcome, admitted := outbound.AdmitCall(unset)
	if admitted {
		t.Fatal("未配置的渠道不该被准许发起调用")
	}
	if got := outcome.Disposition(); got != outbound.NotConfigured {
		t.Errorf("未配置应交回未配置格，实得 %q", got)
	}

	timeout, err := outbound.NewCallTimeout(time.Second)
	if err != nil {
		t.Fatalf("构造超时：%v", err)
	}
	configured, err := outbound.NewChannelConfiguration("endpoint-location", "credential-location", timeout)
	if err != nil {
		t.Fatalf("构造配置：%v", err)
	}
	if _, admitted := outbound.AdmitCall(configured); !admitted {
		t.Error("配置齐备应被准许发起调用")
	}
}
