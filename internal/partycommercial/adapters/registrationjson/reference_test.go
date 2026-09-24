package registrationjson

import (
	"strings"
	"testing"
	"time"

	pcdomain "go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/referenceconfig"
)

// Covers: ADR-0147 决定五——每份已发布的注册号类型参考配置都经采用路径的同一段翻译过领域构造门，
// 随附的样例按该目录的判断方法判为合格；样例缺席的类型不算过门。
func TestEveryReleasedRegistrationNumberTypeReferencePassesTheDomainGatesAndItsSamples(t *testing.T) {
	tenant, err := pcdomain.NewTenantID("tenant-1")
	if err != nil {
		t.Fatalf("租户：%v", err)
	}
	effectiveFrom := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	checked := 0
	for _, reference := range referenceconfig.Released() {
		if !strings.HasPrefix(reference.Identifier(), registrationNumberTypeReferenceDirectory) {
			continue
		}
		adopted, err := registrationNumberTypesFromReference(reference)
		if err != nil {
			t.Fatalf("%s 过不了领域构造门：%v", reference, err)
		}
		basis, err := pcdomain.NewRegistrationNumberTypeBasisReference(reference.Citation())
		if err != nil {
			t.Fatalf("%s 引用串装不进依据格：%v", reference, err)
		}
		lifecycle, err := pcdomain.NewRegistrationNumberTypeLifecycle(effectiveFrom)
		if err != nil {
			t.Fatalf("生命周期：%v", err)
		}
		for _, numberType := range adopted {
			spec := numberType.spec
			spec.Basis = basis
			registration, err := pcdomain.NewRegistrationNumberTypeRegistration(tenant, numberType.country, numberType.code, 1, spec, lifecycle)
			if err != nil {
				t.Fatalf("%s/%s 构造修订：%v", reference, numberType.code, err)
			}
			catalogue, err := pcdomain.NewRegistrationNumberTypeCatalogue(tenant, numberType.country,
				[]pcdomain.RegistrationNumberTypeRegistration{registration})
			if err != nil {
				t.Fatalf("%s/%s 构造目录：%v", reference, numberType.code, err)
			}
			if len(numberType.samples) == 0 {
				t.Fatalf("%s/%s 没有随附样例", reference, numberType.code)
			}
			for _, sample := range numberType.samples {
				check, err := catalogue.Check(numberType.code, spec.Layer, sample, effectiveFrom)
				if err != nil || check.Outcome() != pcdomain.RegistrationNumberAccepted {
					t.Fatalf("%s/%s 样例 %q = %s, %v", reference, numberType.code, sample, check.Outcome(), err)
				}
			}
			checked++
		}
	}
	if checked == 0 {
		t.Fatalf("没有任何已发布的注册号类型参考配置被检过")
	}
}
