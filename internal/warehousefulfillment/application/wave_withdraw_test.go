package application_test

import (
	"testing"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// 能不能从波次撤出只看这张请求在这个波次里的任务：有任务等着拣、没有一条拣过。
func TestARequestCanBeWithdrawnOnlyWhileNothingOfItWasPickedInThatWave(t *testing.T) {
	wave, err := domain.NewWaveID("WAVE-1")
	if err != nil {
		t.Fatal(err)
	}
	other, err := domain.NewWaveID("WAVE-2")
	if err != nil {
		t.Fatal(err)
	}
	task := func(in domain.WaveID, status string) ports.PickTask {
		return ports.PickTask{WaveID: in, Status: status}
	}
	for name, testCase := range map[string]struct {
		tasks []ports.PickTask
		want  bool
	}{
		"都等着拣": {[]ports.PickTask{task(wave, "OPEN"), task(wave, "OPEN")}, true},
		"拣满了一条、还有一条等着拣": {[]ports.PickTask{task(wave, "DONE"), task(wave, "OPEN")}, false},
		"少拣了一条、还有一条等着拣": {[]ports.PickTask{task(wave, "SHORT"), task(wave, "OPEN")}, false},
		"都拣完了":           {[]ports.PickTask{task(wave, "DONE")}, false},
		"任务都作废了":         {[]ports.PickTask{task(wave, "CANCELLED"), task(wave, "LATE")}, false},
		"在这个波次里没有任务":     {[]ports.PickTask{task(other, "OPEN")}, false},
		"没有任务":           {nil, false},
		"别的波次里拣过，不看":     {[]ports.PickTask{task(other, "DONE"), task(other, "SHORT"), task(wave, "OPEN")}, true},
		"这个波次里作废过一条又等着拣": {[]ports.PickTask{task(wave, "CANCELLED"), task(wave, "LATE"), task(wave, "OPEN")}, true},
	} {
		if got := application.CanWithdrawFromWave(testCase.tasks, wave); got != testCase.want {
			t.Errorf("%s：得 %v，应为 %v", name, got, testCase.want)
		}
	}
}
