package domain_test

import (
	"testing"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
)

// 测量与作废的内容摘要是落册的：重放与读回都拿现算的摘要去比册上存下的，所以摘要的字节一个都不能变——改摘要的
// 内部结构、字段名或字段顺序，已经在册的行就被判成别的内容。这里钉死三个黄金值，分别是手工录入的测量、手工录入的
// 作废与设备上报的作废（后两者分别带与不带录入人和原件两个可省的格）。内容摘要的写法要改，先出新的规范化版本，
// 不改这三个值。夹具全为合成（S 级）。

const (
	goldenManualMeasurementDigest  = "NOC-1:c5f17080b9e3b16b33a6a819f00736f2bd3c4ec0b4675af149a64158b1336465"
	goldenManualInvalidationDigest = "NOC-1:bc4f735fb191185ad69749e8ff189fbfd3e88841c9a6a449f188874ea57fd2b9"
	goldenDeviceInvalidationDigest = "NOC-1:ec92ab2bdb074e8109a798484b4c3de191671ce761df3c3e26139eecd8d66aad"
)

func goldenInvalidation(t *testing.T, manual bool) domain.MeasurementInvalidation {
	t.Helper()
	spec := domain.MeasurementInvalidationSpec{
		Source:         measurementSource(t, "void-1"),
		TargetSourceID: "measure-1",
		Reason:         "称错件",
	}
	if manual {
		spec.EnteredBy = "operator-1"
		spec.OriginalDocument = mustValue(t, domain.NewMeasurementOriginalDocument, "slip-9")
	}
	invalidation, err := domain.NewMeasurementInvalidation(spec)
	if err != nil {
		t.Fatalf("作废：%v", err)
	}
	return invalidation
}

func TestTheContentDigestsOfAMeasurementAndItsInvalidationAreFrozen(t *testing.T) {
	spec := measurementSpec(t)
	spec.EnteredBy = "operator-1"
	spec.OriginalDocument = mustValue(t, domain.NewMeasurementOriginalDocument, "slip-9")
	measurement, err := domain.NewActualMeasurement(spec)
	if err != nil {
		t.Fatalf("形成手工测量：%v", err)
	}
	for name, test := range map[string]struct{ got, want string }{
		"手工录入的测量": {measurement.ContentDigest(), goldenManualMeasurementDigest},
		"手工录入的作废": {goldenInvalidation(t, true).ContentDigest(), goldenManualInvalidationDigest},
		"设备上报的作废": {goldenInvalidation(t, false).ContentDigest(), goldenDeviceInvalidationDigest},
	} {
		if test.got != test.want {
			t.Errorf("%s：摘要 = %s，want %s", name, test.got, test.want)
		}
	}
}
