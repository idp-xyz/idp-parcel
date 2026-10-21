package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// Covers: AT-TF-287 的构造门那一半（ADR-0230 决定五）——重问间隔只收正的整分钟，租户、版本与登记依据必备；到期时刻是此刻减去间隔。
func TestAReaskCadenceTakesWholePositiveMinutes(t *testing.T) {
	tenant, err := domain.NewTenantID("SYN-TENANT-01")
	if err != nil {
		t.Fatal(err)
	}
	version, err := domain.NewDeliveryDispatchReaskCadenceVersion("SYN-REASK/v1")
	if err != nil {
		t.Fatal(err)
	}
	for name, interval := range map[string]time.Duration{
		"零":     0,
		"负":     -time.Hour,
		"不足一分钟": 30 * time.Second,
		"带零头":   90 * time.Second,
	} {
		if _, err := domain.NewDeliveryDispatchReaskCadence(tenant, version, interval, "SYN-BASIS"); !errors.Is(err, domain.ErrInvalidDeliveryDispatchReaskCadence) {
			t.Errorf("%s：err = %v, want ErrInvalidDeliveryDispatchReaskCadence", name, err)
		}
	}
	if _, err := domain.NewDeliveryDispatchReaskCadence(domain.TenantID{}, version, time.Hour, "SYN-BASIS"); err == nil {
		t.Error("空租户也登得进")
	}
	if _, err := domain.NewDeliveryDispatchReaskCadence(tenant, version, time.Hour, " "); err == nil {
		t.Error("登记依据空白也登得进")
	}
	cadence, err := domain.NewDeliveryDispatchReaskCadence(tenant, version, 6*time.Hour, "SYN-BASIS")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
	if due := cadence.DueBy(now); !due.Equal(now.Add(-6 * time.Hour)) {
		t.Fatalf("DueBy = %s, want 此刻减六小时", due)
	}
}
