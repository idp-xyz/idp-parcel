package finalconsume_test

import (
	"errors"
	"reflect"
	"testing"
	"unsafe"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/finalconsume"
	psapplication "go.idp.xyz/idp-parcel/internal/parcelshipment/application"
)

func TestAnUnmappedFinalOutcomeIsNotSilentlyConsumed(t *testing.T) {
	err := finalconsume.Consumption(psapplication.FormParcelFinalResult{})
	if !errors.Is(err, finalconsume.ErrUnexpectedFinalOutcome) {
		t.Fatalf("err = %v, want ErrUnexpectedFinalOutcome", err)
	}
}

// resultOf 造一份只带结果名的终局结果。结果的字段包外不可写，而这里要钉的只是「哪个结果名落哪一格」，
// 不值得为它搭一整条委托与规则的替身链；带着真实委托状态的整链见 cmd/parcel-dispatch 的 TestTheWiredFinalAdoptionFormsNoFinalForRequestsThatAreNotAccepted。
func resultOf(outcome psapplication.ParcelFinalAdoptionOutcome) psapplication.FormParcelFinalResult {
	var result psapplication.FormParcelFinalResult
	field := reflect.ValueOf(&result).Elem().FieldByName("outcome")
	reflect.NewAt(field.Type(), unsafe.Pointer(field.UnsafeAddr())).Elem().Set(reflect.ValueOf(outcome))
	return result
}

// Covers: UC-PS-004 结果语义「委托已终结」「委托待决定」落到消费门——已撤回、已拒绝是终态，入账收工；
// 只是已提交的委托之后会被接受，同一事实从另一个读口看到时与「反查不到已接受目标」同归等依赖，回滚重投。
func TestTheConsumptionGateFinishesEndedRequestsButRedeliversRequestsAwaitingDecision(t *testing.T) {
	if err := finalconsume.Consumption(resultOf(psapplication.FinalRequestEnded)); err != nil {
		t.Fatalf("FinalRequestEnded = %v, want nil（终态，入账收工）", err)
	}
	err := finalconsume.Consumption(resultOf(psapplication.FinalRequestAwaitingDecision))
	if !errors.Is(err, finalconsume.ErrFinalUndecided) {
		t.Fatalf("FinalRequestAwaitingDecision = %v, want ErrFinalUndecided（等依赖，重投）", err)
	}
}
