package accessidentity_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
)

func TestDeviceRegistrationNeedsEveryPart(t *testing.T) {
	scope := nodeScope(t)
	for name, build := range map[string]func() error{
		"tenant": func() error {
			_, err := accessidentity.NewDeviceRegistration(" ", workDeviceID, scope, "SYN-BASIS-DEVICE")
			return err
		},
		"device": func() error {
			_, err := accessidentity.NewDeviceRegistration(operatorTenant, "", scope, "SYN-BASIS-DEVICE")
			return err
		},
		"work scope": func() error {
			_, err := accessidentity.NewDeviceRegistration(operatorTenant, workDeviceID, accessidentity.WorkScope{}, "SYN-BASIS-DEVICE")
			return err
		},
		"basis": func() error {
			_, err := accessidentity.NewDeviceRegistration(operatorTenant, workDeviceID, scope, "\t")
			return err
		},
	} {
		if err := build(); !errors.Is(err, accessidentity.ErrIncompleteDeviceRegistration) {
			t.Fatalf("registration without %s: err = %v, want ErrIncompleteDeviceRegistration", name, err)
		}
	}

	registration, err := accessidentity.NewDeviceRegistration(" "+operatorTenant+" ", " "+workDeviceID, scope, "SYN-BASIS-DEVICE ")
	if err != nil {
		t.Fatal(err)
	}
	if registration.TenantID() != operatorTenant || registration.DeviceID() != workDeviceID ||
		registration.WorkScope() != scope || registration.Basis() != "SYN-BASIS-DEVICE" {
		t.Fatalf("registration = %q / %q / %v / %q", registration.TenantID(), registration.DeviceID(), registration.WorkScope(), registration.Basis())
	}
}

func TestDeviceDeactivationNeedsEveryPart(t *testing.T) {
	for name, build := range map[string]func() error{
		"tenant": func() error {
			_, err := accessidentity.NewDeviceDeactivation("", workDeviceID, mintAt, "SYN-BASIS-DEVICE-OFF")
			return err
		},
		"device": func() error {
			_, err := accessidentity.NewDeviceDeactivation(operatorTenant, " ", mintAt, "SYN-BASIS-DEVICE-OFF")
			return err
		},
		"instant": func() error {
			_, err := accessidentity.NewDeviceDeactivation(operatorTenant, workDeviceID, time.Time{}, "SYN-BASIS-DEVICE-OFF")
			return err
		},
		"basis": func() error {
			_, err := accessidentity.NewDeviceDeactivation(operatorTenant, workDeviceID, mintAt, "")
			return err
		},
	} {
		if err := build(); !errors.Is(err, accessidentity.ErrIncompleteDeviceRegistration) {
			t.Fatalf("deactivation without %s: err = %v, want ErrIncompleteDeviceRegistration", name, err)
		}
	}
}

func TestDeviceIsInUseUntilItsDeactivationInstant(t *testing.T) {
	deactivated := recordedDevice(t, operatorTenant, workDeviceID, nodeScope(t), mintAt)
	if !deactivated.InUseAt(mintAt.Add(-time.Nanosecond)) {
		t.Fatalf("device reads as out of use before its deactivation instant")
	}
	if deactivated.InUseAt(mintAt) || deactivated.InUseAt(mintAt.Add(time.Hour)) {
		t.Fatalf("device reads as in use at or after its deactivation instant")
	}
	if at, ok := deactivated.Deactivation(); !ok || !at.DeactivatedAt().Equal(mintAt) {
		t.Fatalf("deactivation = %v, %v", at, ok)
	}

	never := recordedDevice(t, operatorTenant, workDeviceID, nodeScope(t), time.Time{})
	if !never.InUseAt(mintAt.Add(1000 * time.Hour)) {
		t.Fatalf("a device never deactivated reads as out of use")
	}
	if _, ok := never.Deactivation(); ok {
		t.Fatalf("a device never deactivated reports a deactivation")
	}
}

func TestDeactivationAttachesOnlyToItsOwnDevice(t *testing.T) {
	registration, err := accessidentity.NewDeviceRegistration(operatorTenant, workDeviceID, nodeScope(t), "SYN-BASIS-DEVICE")
	if err != nil {
		t.Fatal(err)
	}
	for name, deactivation := range map[string]struct{ tenant, device string }{
		"another device": {tenant: operatorTenant, device: otherWorkDeviceID},
		"another tenant": {tenant: "SYN-TENANT-02", device: workDeviceID},
	} {
		other, err := accessidentity.NewDeviceDeactivation(deactivation.tenant, deactivation.device, mintAt, "SYN-BASIS-DEVICE-OFF")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := accessidentity.NewRecordedDevice(registration, &other); !errors.Is(err, accessidentity.ErrDeactivationDoesNotMatchDevice) {
			t.Fatalf("deactivation of %s: err = %v, want ErrDeactivationDoesNotMatchDevice", name, err)
		}
	}
}
