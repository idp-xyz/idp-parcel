package domain_test

import (
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 本文件钉 AtRegistryPrecision 这一条规则本身，以及它在领域里的两种用法：同键内容相等与严格先后的构造门。
// 夹具全为合成值（S 级）。

var (
	// registryNanos 带纳秒尾巴；registryMicros 是它在登记册上的样子，测试里独立按微秒截出，不经被测函数。
	registryNanos  = time.Date(2026, 9, 1, 9, 0, 0, 123456789, time.UTC)
	registryMicros = registryNanos.Truncate(time.Microsecond)
)

// Covers: 票 operator-channel/30 裁定 1——去掉纳秒尾巴、跨一微秒仍不同、非 UTC 与对应的 UTC 时刻同值、零值仍是零值。
func TestAtRegistryPrecisionFoldsAnInstantToTheMicrosecond(t *testing.T) {
	if got := domain.AtRegistryPrecision(registryNanos); !got.Equal(registryMicros) {
		t.Fatalf("纳秒尾巴应去掉：%s，期望 %s", got.Format(time.RFC3339Nano), registryMicros.Format(time.RFC3339Nano))
	}
	if domain.AtRegistryPrecision(registryNanos).Equal(domain.AtRegistryPrecision(registryNanos.Add(time.Microsecond))) {
		t.Fatal("相差一微秒折完仍应不同")
	}
	shanghai := registryNanos.In(time.FixedZone("CST", 8*3600))
	folded := domain.AtRegistryPrecision(shanghai)
	if folded != domain.AtRegistryPrecision(registryNanos) || folded.Location() != time.UTC {
		t.Fatalf("非 UTC 输入应与对应的 UTC 时刻同值：%s（%s）", folded.Format(time.RFC3339Nano), folded.Location())
	}
	if !domain.AtRegistryPrecision(time.Time{}).IsZero() {
		t.Fatal("零值折完仍应是零值：开放的终点、首版没有的改变时间都靠它")
	}
}

// onRegistry 是登记册读回的样子：测试里独立按微秒截，不调被测函数，免得用例同义反复。
func onRegistry(at time.Time) time.Time { return at.UTC().Truncate(time.Microsecond) }

var (
	registryUntilNanos   = time.Date(2026, 12, 1, 9, 0, 0, 987654321, time.UTC)
	registryChangedNanos = time.Date(2026, 9, 3, 9, 0, 0, 555555555, time.UTC)
)

// credentialOnRange 立一份首版，适用起止由调用方给；changedAt 非零时再以它作废出第二版。
func credentialOnRange(t *testing.T, from, until, changedAt time.Time) domain.ExternalCarrierCredential {
	t.Helper()
	if changedAt.IsZero() {
		spec := credentialSpec(t)
		spec.Applicability = mustApplicability(t, from, until)
		first, err := domain.RegisterExternalCarrierCredential(spec)
		if err != nil {
			t.Fatalf("首版：%v", err)
		}
		return first
	}
	revoked, err := credentialOnRangeErr(t, from, until, changedAt)
	if err != nil {
		t.Fatalf("作废：%v", err)
	}
	return revoked
}

// Covers: 票 operator-channel/30 裁定 2——ExternalCarrierCredential.Equal 按登记册精度比适用起止与改变时间：纳秒版与
// 微秒版两个方向都判等；任一格跨一微秒两个方向都判不等。作废把终点落在改变时间上，所以终点一格在首版上比。
func TestCredentialEqualityHoldsAtRegistryPrecision(t *testing.T) {
	from, until, changed := onRegistry(registryNanos), onRegistry(registryUntilNanos), onRegistry(registryChangedNanos)
	cases := []struct {
		name          string
		nanos, micros domain.ExternalCarrierCredential
		apart         domain.ExternalCarrierCredential
	}{
		{
			name:   "applicability from",
			nanos:  credentialOnRange(t, registryNanos, registryUntilNanos, time.Time{}),
			micros: credentialOnRange(t, from, until, time.Time{}),
			apart:  credentialOnRange(t, from.Add(time.Microsecond), until, time.Time{}),
		},
		{
			name:   "applicability until",
			nanos:  credentialOnRange(t, registryNanos, registryUntilNanos, time.Time{}),
			micros: credentialOnRange(t, from, until, time.Time{}),
			apart:  credentialOnRange(t, from, until.Add(time.Microsecond), time.Time{}),
		},
		{
			name:   "changed at",
			nanos:  credentialOnRange(t, registryNanos, time.Time{}, registryChangedNanos),
			micros: credentialOnRange(t, from, time.Time{}, changed),
			apart:  credentialOnRange(t, from, time.Time{}, changed.Add(time.Microsecond)),
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if !testCase.nanos.Equal(testCase.micros) || !testCase.micros.Equal(testCase.nanos) {
				t.Fatal("纳秒版与它在登记册上的微秒版应判等，两个方向都是")
			}
			if testCase.nanos.Equal(testCase.apart) || testCase.apart.Equal(testCase.nanos) {
				t.Fatal("跨一微秒应判不等，两个方向都是")
			}
		})
	}
}

// Covers: 票 operator-channel/30 裁定 2——MasterDocument.Equal 按登记册精度比改变时间：纳秒版与微秒版两个方向都判等；
// 跨一微秒两个方向都判不等。
func TestMasterDocumentEqualityHoldsAtRegistryPrecision(t *testing.T) {
	first := mustMasterDocument(t, masterDocumentSpec(t))
	revokedAt := func(at time.Time) domain.MasterDocument {
		t.Helper()
		revoked, err := first.Revoke(at, mustRef(t, domain.NewMasterDocumentVersion, "MDV-2"))
		if err != nil {
			t.Fatalf("撤销：%v", err)
		}
		return revoked
	}
	nanos, micros := revokedAt(registryChangedNanos), revokedAt(onRegistry(registryChangedNanos))
	apart := revokedAt(onRegistry(registryChangedNanos).Add(time.Microsecond))
	if !nanos.Equal(micros) || !micros.Equal(nanos) {
		t.Fatal("纳秒版与它在登记册上的微秒版应判等，两个方向都是")
	}
	if nanos.Equal(apart) || apart.Equal(nanos) {
		t.Fatal("跨一微秒应判不等，两个方向都是")
	}
}

var (
	// sameMicroStart、sameMicroEnd 落在同一微秒里：按纳秒是严格先后，在登记册上相等。
	sameMicroStart = time.Date(2026, 9, 1, 9, 0, 0, 123456100, time.UTC)
	sameMicroEnd   = time.Date(2026, 9, 1, 9, 0, 0, 123456900, time.UTC)
	nextMicroEnd   = onRegistry(sameMicroStart).Add(time.Microsecond)
)

// Covers: 票 operator-channel/30 裁定 3——严格先后的构造门两侧先折再判：同一微秒内的窗口与区间不放行，跨一微秒放行。
func TestStrictOrderingGatesHoldAtRegistryPrecision(t *testing.T) {
	gates := map[string]func(from, to time.Time) error{
		"credential applicability": func(from, to time.Time) error {
			_, err := domain.NewCredentialApplicability(from, to)
			return err
		},
		"dispatch task window": func(from, to time.Time) error {
			spec := dispatchTaskSpec(t)
			spec.WindowFrom, spec.WindowTo = from, to
			_, err := domain.OpenDispatchTask(spec)
			return err
		},
		"dispatch task reschedule": func(from, to time.Time) error {
			_, err := openedTask(t).Reschedule(from, to, onRegistry(sameMicroEnd).Add(time.Hour))
			return err
		},
		"fulfillment attempt planned window": func(from, to time.Time) error {
			spec := attemptSpec(t, "ATT-NS")
			spec.PlannedFrom, spec.PlannedTo = from, to
			_, err := domain.FormFulfillmentAttempt(spec)
			return err
		},
	}
	for name, gate := range gates {
		t.Run(name, func(t *testing.T) {
			if gate(sameMicroStart, sameMicroEnd) == nil {
				t.Fatal("同一微秒内的一对在登记册上相等，不该放行")
			}
			if err := gate(sameMicroStart, nextMicroEnd); err != nil {
				t.Fatalf("跨一微秒应放行：%v", err)
			}
		})
	}
}

// Covers: 票 operator-channel/30 裁定 3、4——作废点与起点同一微秒、恰在起点都不放行；跨一微秒放行。
// 既定终点之后不能再收也按登记册精度判：与终点同一微秒的作废点在登记册上就是终点本身。
func TestClosingAnApplicabilityHoldsAtRegistryPrecision(t *testing.T) {
	revoke := func(from, until, at time.Time) error {
		_, err := credentialOnRangeErr(t, from, until, at)
		return err
	}
	if revoke(sameMicroStart, time.Time{}, sameMicroEnd) == nil {
		t.Fatal("作废点与起点同一微秒，收出的是空区间，不该放行")
	}
	whole := time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC)
	if revoke(whole, time.Time{}, whole) == nil {
		t.Fatal("作废点恰在起点，收出的是空区间，不该放行")
	}
	if err := revoke(sameMicroStart, time.Time{}, nextMicroEnd); err != nil {
		t.Fatalf("跨一微秒应放行：%v", err)
	}
	until := onRegistry(registryUntilNanos)
	if err := revoke(sameMicroStart, until, until.Add(500*time.Nanosecond)); err != nil {
		t.Fatalf("与既定终点同一微秒的作废点在登记册上不晚于终点，应放行：%v", err)
	}
	if revoke(sameMicroStart, until, until.Add(time.Microsecond)) == nil {
		t.Fatal("晚于既定终点一微秒，不该放行")
	}
}

// credentialOnRangeErr 立一份首版再以 at 作废，交回作废的答复。
func credentialOnRangeErr(t *testing.T, from, until, at time.Time) (domain.ExternalCarrierCredential, error) {
	t.Helper()
	spec := credentialSpec(t)
	spec.Applicability = mustApplicability(t, from, until)
	first, err := domain.RegisterExternalCarrierCredential(spec)
	if err != nil {
		t.Fatalf("首版：%v", err)
	}
	return first.Revoke(at, mustRef(t, domain.NewExternalCarrierCredentialVersion, "CREDV-2"))
}
