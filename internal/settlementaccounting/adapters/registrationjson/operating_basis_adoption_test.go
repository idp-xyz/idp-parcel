package registrationjson

import (
	"errors"
	"strings"
	"testing"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

func TestOperatingBasisAdoptionRequiresBothBooleans(t *testing.T) {
	if _, _, err := OperatingBasisAdoptionFromJSON([]byte(`{"tenantId":"SYN-TENANT-01","confirmed":true}`)); err == nil {
		t.Fatal("缺 estimated 被收下了")
	}
	if _, _, err := OperatingBasisAdoptionFromJSON([]byte(`{"tenantId":"SYN-TENANT-01","estimated":true}`)); err == nil {
		t.Fatal("缺 confirmed 被收下了")
	}
	if _, _, err := OperatingBasisAdoptionFromJSON([]byte(`{"tenantId":"SYN-TENANT-01","estimated":false,"confirmed":false}`)); !errors.Is(err, domain.ErrOperatingBasisAdoptionEmpty) {
		t.Fatalf("两个都是假应拒，err=%v", err)
	}
	tenant, adoption, err := OperatingBasisAdoptionFromJSON([]byte(`{"tenantId":"SYN-TENANT-01","estimated":true,"confirmed":true}`))
	if err != nil || tenant.String() != "SYN-TENANT-01" || !adoption.Estimated() || !adoption.Confirmed() {
		t.Fatalf("两者都采用 = %s %+v err=%v", tenant, adoption, err)
	}
	if _, _, err := OperatingBasisAdoptionFromJSON([]byte(`{"tenantId":"SYN-TENANT-01","estimated":true,"confirmed":false,"extra":1}`)); err == nil || !strings.Contains(err.Error(), "unknown") {
		t.Fatalf("未知字段应拒，err=%v", err)
	}
}
