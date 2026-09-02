package outbound_test

import (
	"testing"

	"go.idp.xyz/idp-parcel/internal/platform/outbound"
)

// 四条都扫完整个 uint8 值域而不是逐格写死。逐格写死只覆盖写下来的那几个：将来多一格时
// 它默认落进 false，而没有任何东西提醒作者去想清那一格准不准重发——恰恰是这一想漏了会
// 花钱。

func TestProvenNotAcceptedIsTheOnlyGradeThatAdmitsResend(t *testing.T) {
	t.Parallel()

	for value := 0; value <= 255; value++ {
		disposition := outbound.Disposition(value)
		admits := disposition.AdmitsResend()
		if disposition == outbound.ProvenNotAccepted {
			if !admits {
				t.Errorf("确证未受理应准许重发同一请求，实得不准")
			}
			continue
		}
		if admits {
			t.Errorf("取值 %d（%q）不该准许重发", value, disposition)
		}
	}
}

func TestAnswerUndeterminedIsTheOnlyGradeThatMustBeSettledByQuery(t *testing.T) {
	t.Parallel()

	for value := 0; value <= 255; value++ {
		disposition := outbound.Disposition(value)
		requires := disposition.RequiresQueryToSettle()
		if disposition == outbound.AnswerUndetermined {
			if !requires {
				t.Errorf("答案未确定只能靠查询或对账收口，实得不要求")
			}
			continue
		}
		if requires {
			t.Errorf("取值 %d（%q）不该要求查询收口", value, disposition)
		}
	}
}

// 两条纪律的合取。分开钉会让两半各自为真而合起来失守：某一格若既准重发又要求查询，
// 调用方两条路都走得通，而那正是同一包裹被重复购买面单的入口。
func TestNoGradeBothAdmitsResendAndRequiresQuery(t *testing.T) {
	t.Parallel()

	for value := 0; value <= 255; value++ {
		disposition := outbound.Disposition(value)
		if disposition.AdmitsResend() && disposition.RequiresQueryToSettle() {
			t.Errorf("取值 %d（%q）同时准许重发与要求查询，两条续办路径互斥", value, disposition)
		}
	}
}

// 未知取值一律 fail-closed。它不是假想：取值会从库里回读，也会从另一个版本的适配器传进来，
// 而这一格答一次 true 就是一次真实的重复下单。
func TestAnUnknownDispositionIsNeitherNamedNorResendable(t *testing.T) {
	t.Parallel()

	for _, unknown := range []outbound.Disposition{outbound.DispositionInvalid, outbound.Disposition(200)} {
		if name := unknown.String(); name != "" {
			t.Errorf("未知取值 %d 不该有名字，实得 %q", unknown, name)
		}
		if unknown.AdmitsResend() {
			t.Errorf("未知取值 %d 不该准许重发", unknown)
		}
		if unknown.RequiresQueryToSettle() {
			t.Errorf("未知取值 %d 不该要求查询收口——它连是不是一次调用结果都说不上", unknown)
		}
	}
}

// 与架构包那道枚举门禁互补：那一条是语法扫描，只看常量名在 String() 里被提到过；这一条
// 在运行期两个方向都验——声明的每一格都有名字，且没声明的一个都没有。
func TestEveryDeclaredGradeIsNamedAndNoOtherValueIs(t *testing.T) {
	t.Parallel()

	named := map[outbound.Disposition]string{
		outbound.Accepted:           "ACCEPTED",
		outbound.Rejected:           "REJECTED",
		outbound.AnswerUndetermined: "ANSWER_UNDETERMINED",
		outbound.ProvenNotAccepted:  "PROVEN_NOT_ACCEPTED",
		outbound.NotConfigured:      "NOT_CONFIGURED",
	}

	for value := 0; value <= 255; value++ {
		disposition := outbound.Disposition(value)
		want, declared := named[disposition]
		if !declared {
			want = ""
		}
		if got := disposition.String(); got != want {
			t.Errorf("取值 %d 的名字应为 %q，实得 %q", value, want, got)
		}
	}
}
