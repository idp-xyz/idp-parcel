package accessidentity_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
)

// Covers: 票 operator-channel/21 做什么「每行只有设备标识、作业范围、登记还是停用；停用了的带停用时刻」——
// 查阅行只有设备标识、作业范围、登记还是停用；停用行带停用时刻，登记行不带。依据留在登记行上，不进查阅行。
func TestCatalogueRowKeepsOnlyTheListedFields(t *testing.T) {
	scope := nodeScope(t)
	registered := recordedDevice(t, operatorTenant, workDeviceID, scope, time.Time{})
	row, err := registered.CatalogueRow()
	if err != nil {
		t.Fatal(err)
	}
	if row.DeviceID() != workDeviceID || row.WorkScope() != scope || row.Status() != accessidentity.DeviceCatalogueRegistered {
		t.Fatalf("登记行 = %s %s %s", row.DeviceID(), row.WorkScope().Kind(), row.Status())
	}
	if _, deactivated := row.DeactivatedAt(); deactivated {
		t.Fatal("登记行带着停用时刻")
	}
	if registered.Registration().Basis() == "" {
		t.Fatal("依据应留在登记行上")
	}

	deactivatedAt := mintAt
	off := recordedDevice(t, operatorTenant, workDeviceID, scope, deactivatedAt)
	row, err = off.CatalogueRow()
	if err != nil {
		t.Fatal(err)
	}
	at, ok := row.DeactivatedAt()
	if row.Status() != accessidentity.DeviceCatalogueDeactivated || !ok || !at.Equal(deactivatedAt) {
		t.Fatalf("停用行 status %s at %s ok %v", row.Status(), at, ok)
	}

	if _, err := accessidentity.NewDeviceCatalogueRow(" ", scope, nil); !errors.Is(err, accessidentity.ErrIncompleteDeviceRegistration) {
		t.Fatalf("空设备标识 err = %v", err)
	}
	zero := time.Time{}
	if _, err := accessidentity.NewDeviceCatalogueRow(workDeviceID, scope, &zero); !errors.Is(err, accessidentity.ErrIncompleteDeviceRegistration) {
		t.Fatalf("零值停用时刻 err = %v", err)
	}
	if _, err := accessidentity.NewDeviceCatalogueRow(workDeviceID, accessidentity.WorkScope{}, nil); !errors.Is(err, accessidentity.ErrIncompleteDeviceRegistration) {
		t.Fatalf("空作业范围 err = %v", err)
	}
}
