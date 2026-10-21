package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// cadenceRegistryDouble 是重问间隔册的内存替身：按（租户，版本）首登，撞键答 false。beforeSave 扮演并发下另一方先落了同一版。
type cadenceRegistryDouble struct {
	rows       map[string]ports.DeliveryDispatchReaskCadenceRecord
	err        error
	beforeSave func()
}

func (double *cadenceRegistryDouble) FindByVersion(
	_ context.Context, tenant domain.TenantID, version domain.DeliveryDispatchReaskCadenceVersion,
) (ports.DeliveryDispatchReaskCadenceRecord, bool, error) {
	if double.err != nil {
		return ports.DeliveryDispatchReaskCadenceRecord{}, false, double.err
	}
	record, found := double.rows[tenant.String()+"/"+version.String()]
	return record, found, nil
}

func (double *cadenceRegistryDouble) Save(_ context.Context, record ports.DeliveryDispatchReaskCadenceRecord) (bool, error) {
	if double.beforeSave != nil {
		double.beforeSave()
		double.beforeSave = nil
	}
	key := record.Cadence.TenantID().String() + "/" + record.Cadence.Version().String()
	if _, taken := double.rows[key]; taken {
		return false, nil
	}
	double.rows[key] = record
	return true, nil
}

var cadenceRegisteredAt = time.Date(2026, 10, 21, 8, 0, 0, 0, time.UTC)

func registerCadence(t *testing.T, registry *cadenceRegistryDouble, version string, minutes int) application.RegisterReaskCadenceResult {
	t.Helper()
	result, err := application.NewRegisterReaskCadenceHandler(application.RegisterReaskCadenceDeps{
		Cadences: registry, Clock: handoverClock{at: cadenceRegisteredAt},
	}).Register(t.Context(), application.RegisterReaskCadenceCommand{
		TenantID: value(t, domain.NewTenantID, "tenant-1"), Version: version, IntervalMinutes: minutes,
	})
	if err != nil {
		t.Fatalf("登记重问间隔：%v", err)
	}
	return result
}

// Covers: AT-TF-287（ADR-0230 决定五）——重问间隔是登记出来的租户取值：首登答已登记、记下登记时刻；同版本同正文是重放、交回在册那
// 一版；同版本异正文是冲突、在册那一版不动；间隔不是正的整分钟、版本空白都不受理；并发下另一方先落了同一版，读回它再分。
func TestAReaskCadenceIsRegisteredOncePerVersion(t *testing.T) {
	registry := &cadenceRegistryDouble{rows: map[string]ports.DeliveryDispatchReaskCadenceRecord{}}
	first := registerCadence(t, registry, "reask/v1", 360)
	record, _ := first.Record()
	if first.Outcome() != application.ReaskCadenceRegistered || record.Cadence.Interval() != 6*time.Hour ||
		!record.RecordedAt.Equal(cadenceRegisteredAt) {
		t.Fatalf("首登 = %s %+v", first.Outcome(), record)
	}
	if replay := registerCadence(t, registry, "reask/v1", 360); replay.Outcome() != application.ReaskCadenceExistingVersion {
		t.Fatalf("重放 = %s, want EXISTING_VERSION", replay.Outcome())
	}
	conflict := registerCadence(t, registry, "reask/v1", 60)
	held, _ := conflict.Record()
	if conflict.Outcome() != application.ReaskCadenceContentConflict || held.Cadence.Interval() != 6*time.Hour || len(registry.rows) != 1 {
		t.Fatalf("冲突 = %s 交回 %s、册 %d 版, want CONTENT_CONFLICT、在册的六小时、1 版", conflict.Outcome(), held.Cadence.Interval(), len(registry.rows))
	}
	for name, minutes := range map[string]int{"零分钟": 0, "负": -5, "溢出": int(^uint(0) >> 1)} {
		if result := registerCadence(t, registry, "reask/v2", minutes); result.Outcome() != application.ReaskCadenceRegistrationNotAccepted {
			t.Errorf("%s = %s, want INPUT_NOT_ACCEPTED", name, result.Outcome())
		}
	}
	if result := registerCadence(t, registry, " ", 60); result.Outcome() != application.ReaskCadenceRegistrationNotAccepted {
		t.Errorf("版本空白 = %s, want INPUT_NOT_ACCEPTED", result.Outcome())
	}

	registry.beforeSave = func() {
		cadence, err := domain.NewDeliveryDispatchReaskCadence(value(t, domain.NewTenantID, "tenant-1"),
			value(t, domain.NewDeliveryDispatchReaskCadenceVersion, "reask/v3"), 30*time.Minute)
		if err != nil {
			t.Fatal(err)
		}
		registry.rows["tenant-1/reask/v3"] = ports.DeliveryDispatchReaskCadenceRecord{Cadence: cadence}
	}
	if raced := registerCadence(t, registry, "reask/v3", 30); raced.Outcome() != application.ReaskCadenceExistingVersion {
		t.Fatalf("并发落败同正文 = %s, want EXISTING_VERSION", raced.Outcome())
	}

	registry.err = errors.New("cadence register down")
	if undecided := registerCadence(t, registry, "reask/v4", 30); undecided.Outcome() != application.ReaskCadenceRegistrationUndecided {
		t.Fatalf("读不动 = %s, want REGISTRATION_UNDECIDED", undecided.Outcome())
	}
}
