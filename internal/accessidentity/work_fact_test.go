package accessidentity_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
)

const (
	workDeviceID      = "SYN-DEVICE-SCANNER-01"
	otherWorkDeviceID = "SYN-DEVICE-SCANNER-02"
)

func workScope(t *testing.T, kind accessidentity.WorkScopeKind, reference string) accessidentity.WorkScope {
	t.Helper()
	scope, err := accessidentity.NewWorkScope(kind, reference)
	if err != nil {
		t.Fatalf("NewWorkScope(%s, %q): %v", kind, reference, err)
	}
	return scope
}

func nodeScope(t *testing.T) accessidentity.WorkScope {
	t.Helper()
	return workScope(t, accessidentity.WorkScopeNode, "SYN-NODE-SZX-01")
}

func otherNodeScope(t *testing.T) accessidentity.WorkScope {
	t.Helper()
	return workScope(t, accessidentity.WorkScopeNode, "SYN-NODE-SIN-01")
}

// workFactStanding 登一个绑在 operatorTenant 上、在 scopes 的每个作业范围各持一笔作业事实登记授予的操作者；
// 授予从 mintAt 前一天起生效、不设终点。
func workFactStanding(t *testing.T, scopes ...accessidentity.WorkScope) accessidentity.OperatorStanding {
	t.Helper()
	subject := operatorSubject(t)
	binding, err := accessidentity.NewOperatorBinding(subject, operatorTenant, "SYN-BASIS-BINDING")
	if err != nil {
		t.Fatal(err)
	}
	interval, err := accessidentity.NewEffectiveInterval(mintAt.Add(-24*time.Hour), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	var grants []accessidentity.RecordedGrant
	for index, scope := range scopes {
		grant, err := accessidentity.NewWorkFactRegistrationGrant(operatorTenant, "SYN-GRANT-WF-"+string(rune('A'+index)), subject, scope, interval, "SYN-BASIS-GRANT")
		if err != nil {
			t.Fatal(err)
		}
		recorded, err := accessidentity.NewRecordedGrant(grant, nil)
		if err != nil {
			t.Fatal(err)
		}
		grants = append(grants, recorded)
	}
	standing, err := accessidentity.NewOperatorStanding(binding, grants)
	if err != nil {
		t.Fatal(err)
	}
	return standing
}

type deviceRegistryFake struct {
	devices map[string]accessidentity.RecordedDevice
	err     error
	calls   *int
}

func (fake deviceRegistryFake) FindDevice(_ context.Context, tenantID, deviceID string) (accessidentity.RecordedDevice, bool, error) {
	if fake.calls != nil {
		*fake.calls++
	}
	if fake.err != nil {
		return accessidentity.RecordedDevice{}, false, fake.err
	}
	device, found := fake.devices[tenantID+"/"+deviceID]
	return device, found, nil
}

// recordedDevice 登一台设备；deactivatedAt 取零值表示未停用。
func recordedDevice(t *testing.T, tenant, deviceID string, scope accessidentity.WorkScope, deactivatedAt time.Time) accessidentity.RecordedDevice {
	t.Helper()
	registration, err := accessidentity.NewDeviceRegistration(tenant, deviceID, scope, "SYN-BASIS-DEVICE")
	if err != nil {
		t.Fatal(err)
	}
	if deactivatedAt.IsZero() {
		recorded, err := accessidentity.NewRecordedDevice(registration, nil)
		if err != nil {
			t.Fatal(err)
		}
		return recorded
	}
	deactivation, err := accessidentity.NewDeviceDeactivation(tenant, deviceID, deactivatedAt, "SYN-BASIS-DEVICE-OFF")
	if err != nil {
		t.Fatal(err)
	}
	recorded, err := accessidentity.NewRecordedDevice(registration, &deactivation)
	if err != nil {
		t.Fatal(err)
	}
	return recorded
}

func devicesWith(devices ...accessidentity.RecordedDevice) deviceRegistryFake {
	book := map[string]accessidentity.RecordedDevice{}
	for _, device := range devices {
		registration := device.Registration()
		book[registration.TenantID()+"/"+registration.DeviceID()] = device
	}
	return deviceRegistryFake{devices: book}
}

func newWorkFactMinter(
	t *testing.T,
	verifier accessidentity.OperatorCredentialVerifier,
	operators accessidentity.OperatorRegistry,
	devices accessidentity.DeviceRegistry,
) *accessidentity.WorkFactMinter {
	t.Helper()
	minter, err := accessidentity.NewWorkFactMinter(verifier, operators, devices, accessidentity.UnconfiguredAdmissionScope{}, func() time.Time { return mintAt })
	if err != nil {
		t.Fatalf("NewWorkFactMinter: %v", err)
	}
	return minter
}

func mintWorkFact(minter *accessidentity.WorkFactMinter, tenant string, scope accessidentity.WorkScope, deviceID string) (accessidentity.WorkFactEnvelope, error) {
	return minter.MintWorkFact(context.Background(), accessidentity.NewOperatorCredential("presented.frontline.token"),
		accessidentity.WorkFactRequest{TenantID: tenant, WorkScope: scope, DeviceID: deviceID})
}

func TestWorkFactRegistrationIsGrantedPerWorkScope(t *testing.T) {
	subject := operatorSubject(t)
	interval, err := accessidentity.NewEffectiveInterval(mintAt.Add(-24*time.Hour), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := accessidentity.NewOperatorGrant(operatorTenant, "SYN-GRANT-X", subject,
		accessidentity.CapabilityWorkFactRegistration, interval, "SYN-BASIS-GRANT"); !errors.Is(err, accessidentity.ErrWorkScopeRequired) {
		t.Fatalf("whole-face work fact grant: err = %v, want ErrWorkScopeRequired", err)
	}

	standing := workFactStanding(t, nodeScope(t))
	if !standing.HoldsWorkFactAt(nodeScope(t), mintAt) {
		t.Fatalf("operator does not hold the work scope it was granted")
	}
	if standing.HoldsWorkFactAt(otherNodeScope(t), mintAt) {
		t.Fatalf("a grant at one node answered for another node")
	}
	if standing.HoldsWorkFactAt(workScope(t, accessidentity.WorkScopeOffsite, "SYN-NODE-SZX-01"), mintAt) {
		t.Fatalf("a node grant answered for an off-site scope that merely shares its reference")
	}
	if standing.HoldsAt(accessidentity.CapabilityWorkFactRegistration, mintAt) {
		t.Fatalf("asking the whole face answered yes: that would grant every work scope at once")
	}
	grant := standing.Grants()[0].Grant()
	if grant.Face() != accessidentity.CapabilityWorkFactRegistration || grant.WorkScope() != nodeScope(t) {
		t.Fatalf("grant = face %s, scope %v", grant.Face(), grant.WorkScope())
	}
}

func TestWorkScopeKindsAreAClosedLiteralSet(t *testing.T) {
	for _, value := range []string{"NODE", "OFFSITE"} {
		if _, err := accessidentity.ParseWorkScopeKind(value); err != nil {
			t.Fatalf("ParseWorkScopeKind(%q): %v", value, err)
		}
	}
	for _, value := range []string{"node", "Offsite", "", "ZONE"} {
		if _, err := accessidentity.ParseWorkScopeKind(value); !errors.Is(err, accessidentity.ErrWorkScopeKindUnknown) {
			t.Fatalf("ParseWorkScopeKind(%q): err = %v, want ErrWorkScopeKindUnknown", value, err)
		}
	}
	if _, err := accessidentity.NewWorkScope("ZONE", "SYN-ZONE-01"); !errors.Is(err, accessidentity.ErrWorkScopeKindUnknown) {
		t.Fatalf("unknown kind: err = %v, want ErrWorkScopeKindUnknown", err)
	}
	if _, err := accessidentity.NewWorkScope(accessidentity.WorkScopeNode, "  "); !errors.Is(err, accessidentity.ErrInvalidWorkScope) {
		t.Fatalf("blank reference: err = %v, want ErrInvalidWorkScope", err)
	}
}

func TestGrantedOperatorOnARegisteredDeviceIsMintedIntoAWorkFactEnvelope(t *testing.T) {
	scope := nodeScope(t)
	minter := newWorkFactMinter(t, verifierFake{subject: operatorSubject(t)},
		registryFake{standing: workFactStanding(t, scope), found: true},
		devicesWith(recordedDevice(t, operatorTenant, workDeviceID, scope, time.Time{})))

	envelope, err := mintWorkFact(minter, operatorTenant, scope, workDeviceID)
	if err != nil {
		t.Fatalf("granted operator on a registered device: %v", err)
	}
	if !envelope.Minted() || envelope.TenantID() != operatorTenant || envelope.Subject() != operatorSubject(t) ||
		envelope.DeviceID() != workDeviceID || envelope.WorkScope() != scope {
		t.Fatalf("envelope = minted %v, tenant %q, subject %v, device %q, scope %v",
			envelope.Minted(), envelope.TenantID(), envelope.Subject(), envelope.DeviceID(), envelope.WorkScope())
	}
	if envelope.Source() != accessidentity.WorkFactSourceFrontlineClient {
		t.Fatalf("source = %q, want the frontline client", envelope.Source())
	}
}

// workFactGrades 是作业事实登记口的答复格：ADR-0100 决定四的三格与 ADR-0149 决定二另加的设备一格对外，外加核验方与
// 两本册各自的依赖故障、准入范围两格。每一格只许被自己那个哨兵认出。
var workFactGrades = map[string]error{
	"not configured":              accessidentity.ErrAccessChannelNotConfigured,
	"credential rejected":         accessidentity.ErrCredentialRejected,
	"not granted":                 accessidentity.ErrOperatorNotGranted,
	"device not in use":           accessidentity.ErrDeviceNotInUse,
	"verifier unavailable":        accessidentity.ErrCredentialVerifierUnavailable,
	"registry unavailable":        accessidentity.ErrOperatorRegistryUnavailable,
	"device registry unavailable": accessidentity.ErrDeviceRegistryUnavailable,
	"outside admission":           accessidentity.ErrOutsideAdmissionScope,
	"admission unavailable":       accessidentity.ErrAdmissionScopeUnavailable,
}

func assertWorkFactGrade(t *testing.T, err error, want string) {
	t.Helper()
	for name, sentinel := range workFactGrades {
		if got := errors.Is(err, sentinel); got != (name == want) {
			t.Fatalf("err = %v: errors.Is(%s) = %v, want only %q", err, name, got, want)
		}
	}
}

func TestWorkFactAnswerGradesDoNotStandInForEachOther(t *testing.T) {
	scope := nodeScope(t)
	subject := operatorSubject(t)
	granted := registryFake{standing: workFactStanding(t, scope), found: true}
	inUse := devicesWith(recordedDevice(t, operatorTenant, workDeviceID, scope, time.Time{}))
	cases := map[string]struct {
		verifier  accessidentity.OperatorCredentialVerifier
		operators accessidentity.OperatorRegistry
		devices   accessidentity.DeviceRegistry
		tenant    string
		want      string
	}{
		"issuer parameters unset": {
			verifier: accessidentity.UnconfiguredOperatorCredentialVerifier{}, operators: granted, devices: inUse, want: "not configured",
		},
		"token rejected": {
			verifier: verifierFake{err: accessidentity.ErrCredentialRejected}, operators: granted, devices: inUse, want: "credential rejected",
		},
		"issuer key set unreachable": {
			verifier: verifierFake{err: accessidentity.ErrCredentialVerifierUnavailable}, operators: granted, devices: inUse, want: "verifier unavailable",
		},
		"operator not in the register": {
			verifier: verifierFake{subject: subject}, operators: registryFake{found: false}, devices: inUse, want: "not granted",
		},
		"operator bound to another tenant": {
			verifier: verifierFake{subject: subject}, operators: granted, devices: inUse, tenant: "SYN-TENANT-02", want: "not granted",
		},
		"grant covers another work scope": {
			verifier: verifierFake{subject: subject}, operators: registryFake{standing: workFactStanding(t, otherNodeScope(t)), found: true},
			devices: inUse, want: "not granted",
		},
		"only a registry write grant": {
			verifier:  verifierFake{subject: subject},
			operators: registryFake{standing: standingWith(t, accessidentity.CapabilityRegistryConfigurationWrite), found: true},
			devices:   inUse, want: "not granted",
		},
		"device not registered": {
			verifier: verifierFake{subject: subject}, operators: granted, devices: devicesWith(), want: "device not in use",
		},
		"device deactivated before the request": {
			verifier: verifierFake{subject: subject}, operators: granted,
			devices: devicesWith(recordedDevice(t, operatorTenant, workDeviceID, scope, mintAt.Add(-time.Hour))), want: "device not in use",
		},
		"device deactivated at the request instant": {
			verifier: verifierFake{subject: subject}, operators: granted,
			devices: devicesWith(recordedDevice(t, operatorTenant, workDeviceID, scope, mintAt)), want: "device not in use",
		},
		"device registered for another work scope": {
			verifier: verifierFake{subject: subject}, operators: granted,
			devices: devicesWith(recordedDevice(t, operatorTenant, workDeviceID, otherNodeScope(t), time.Time{})), want: "device not in use",
		},
		"device registered under another tenant": {
			verifier: verifierFake{subject: subject}, operators: granted,
			devices: devicesWith(recordedDevice(t, "SYN-TENANT-02", workDeviceID, scope, time.Time{})), want: "device not in use",
		},
		"operator register cannot be read": {
			verifier: verifierFake{subject: subject}, operators: registryFake{err: errors.New("connection refused")}, devices: inUse,
			want: "registry unavailable",
		},
		"device register cannot be read": {
			verifier: verifierFake{subject: subject}, operators: granted, devices: deviceRegistryFake{err: errors.New("connection refused")},
			want: "device registry unavailable",
		},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			tenant := testCase.tenant
			if tenant == "" {
				tenant = operatorTenant
			}
			minter := newWorkFactMinter(t, testCase.verifier, testCase.operators, testCase.devices)
			envelope, err := mintWorkFact(minter, tenant, scope, workDeviceID)
			assertWorkFactGrade(t, err, testCase.want)
			if envelope.Minted() {
				t.Fatalf("a refused presentation still produced a minted envelope")
			}
		})
	}
}

func TestDeactivatedDeviceRefusesTheOperatorWhileAnotherRegisteredDeviceStillServes(t *testing.T) {
	scope := nodeScope(t)
	minter := newWorkFactMinter(t, verifierFake{subject: operatorSubject(t)},
		registryFake{standing: workFactStanding(t, scope), found: true},
		devicesWith(
			recordedDevice(t, operatorTenant, workDeviceID, scope, mintAt.Add(-time.Minute)),
			recordedDevice(t, operatorTenant, otherWorkDeviceID, scope, time.Time{}),
		))

	if _, err := mintWorkFact(minter, operatorTenant, scope, workDeviceID); !errors.Is(err, accessidentity.ErrDeviceNotInUse) {
		t.Fatalf("same operator on the deactivated device: err = %v, want ErrDeviceNotInUse", err)
	}
	envelope, err := mintWorkFact(minter, operatorTenant, scope, otherWorkDeviceID)
	if err != nil || envelope.DeviceID() != otherWorkDeviceID || envelope.Subject() != operatorSubject(t) {
		t.Fatalf("same operator on another registered device: device %q, subject %v, err %v", envelope.DeviceID(), envelope.Subject(), err)
	}
}

// 同一份出示在两个时钟下铸出的信封逐格相同：服务端的时刻不进信封，事实身份与发生时间仍只由请求携带（ADR-0023）。
func TestWorkFactEnvelopeCarriesNoServerInstant(t *testing.T) {
	scope := nodeScope(t)
	operators := registryFake{standing: workFactStanding(t, scope), found: true}
	devices := devicesWith(recordedDevice(t, operatorTenant, workDeviceID, scope, time.Time{}))
	var envelopes []accessidentity.WorkFactEnvelope
	for _, at := range []time.Time{mintAt, mintAt.Add(37 * time.Minute)} {
		minter, err := accessidentity.NewWorkFactMinter(verifierFake{subject: operatorSubject(t)}, operators, devices,
			accessidentity.UnconfiguredAdmissionScope{}, func() time.Time { return at })
		if err != nil {
			t.Fatal(err)
		}
		envelope, err := mintWorkFact(minter, operatorTenant, scope, workDeviceID)
		if err != nil {
			t.Fatalf("mint at %s: %v", at, err)
		}
		envelopes = append(envelopes, envelope)
	}
	if envelopes[0] != envelopes[1] {
		t.Fatalf("envelopes minted at two instants differ: %+v vs %+v", envelopes[0], envelopes[1])
	}
}

var workFactRequirement = &accessidentity.AdmissionRequirement{Capability: "WORK_FACT_REGISTRATION", FactKind: "NODE_RECEPTION"}

func TestDeviceIsLookedUpOnlyAfterTheGrantAndAdmissionOnlyAfterTheDevice(t *testing.T) {
	scope := nodeScope(t)
	granted := registryFake{standing: workFactStanding(t, scope), found: true}
	inUse := []accessidentity.RecordedDevice{recordedDevice(t, operatorTenant, workDeviceID, scope, time.Time{})}
	cases := map[string]struct {
		operators          accessidentity.OperatorRegistry
		devices            []accessidentity.RecordedDevice
		admitted           bool
		requirement        *accessidentity.AdmissionRequirement
		want               string
		wantDeviceCalls    int
		wantAdmissionCalls int
	}{
		"operator without the grant is refused before the device is looked up": {
			operators: registryFake{found: false}, devices: inUse, admitted: true, requirement: workFactRequirement,
			want: "not granted", wantDeviceCalls: 0, wantAdmissionCalls: 0,
		},
		"unregistered device is refused before admission": {
			operators: granted, admitted: true, requirement: workFactRequirement,
			want: "device not in use", wantDeviceCalls: 1, wantAdmissionCalls: 0,
		},
		"interval covers the write": {
			operators: granted, devices: inUse, admitted: true, requirement: workFactRequirement,
			wantDeviceCalls: 1, wantAdmissionCalls: 1,
		},
		"no interval covers the write": {
			operators: granted, devices: inUse, requirement: workFactRequirement,
			want: "outside admission", wantDeviceCalls: 1, wantAdmissionCalls: 1,
		},
		"face that carries no admission requirement": {
			operators: granted, devices: inUse,
			wantDeviceCalls: 1, wantAdmissionCalls: 0,
		},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			deviceCalls, admissionCalls := 0, 0
			devices := devicesWith(testCase.devices...)
			devices.calls = &deviceCalls
			minter, err := accessidentity.NewWorkFactMinter(verifierFake{subject: operatorSubject(t)}, testCase.operators, devices,
				admissionFake{admitted: testCase.admitted, calls: &admissionCalls}, func() time.Time { return mintAt })
			if err != nil {
				t.Fatal(err)
			}
			envelope, err := minter.MintWorkFact(context.Background(), accessidentity.NewOperatorCredential("presented.frontline.token"),
				accessidentity.WorkFactRequest{TenantID: operatorTenant, WorkScope: scope, DeviceID: workDeviceID, Admission: testCase.requirement})
			if testCase.want == "" {
				if err != nil || !envelope.Minted() {
					t.Fatalf("err = %v, minted = %v; want a minted envelope", err, envelope.Minted())
				}
			} else {
				assertWorkFactGrade(t, err, testCase.want)
			}
			if deviceCalls != testCase.wantDeviceCalls || admissionCalls != testCase.wantAdmissionCalls {
				t.Fatalf("device register consulted %d times, admission %d; want %d and %d",
					deviceCalls, admissionCalls, testCase.wantDeviceCalls, testCase.wantAdmissionCalls)
			}
		})
	}
}

// 作业范围与设备标识是铸造的前提，不是答复格：缺了是接入口漏传，答哪一格都会把人指去错的恢复动作。
func TestWorkFactRequestMustNameAWorkScopeAndADevice(t *testing.T) {
	scope := nodeScope(t)
	minter := newWorkFactMinter(t, verifierFake{subject: operatorSubject(t)},
		registryFake{standing: workFactStanding(t, scope), found: true},
		devicesWith(recordedDevice(t, operatorTenant, workDeviceID, scope, time.Time{})))
	for name, request := range map[string]accessidentity.WorkFactRequest{
		"no work scope": {TenantID: operatorTenant, DeviceID: workDeviceID},
		"no device":     {TenantID: operatorTenant, WorkScope: scope, DeviceID: "  "},
	} {
		envelope, err := minter.MintWorkFact(context.Background(), accessidentity.NewOperatorCredential("presented.frontline.token"), request)
		if err == nil || envelope.Minted() {
			t.Fatalf("%s: err = %v, minted = %v; want a refusal", name, err, envelope.Minted())
		}
		for grade, sentinel := range workFactGrades {
			if errors.Is(err, sentinel) {
				t.Fatalf("%s: err = %v reads as the %q grade", name, err, grade)
			}
		}
	}
}

// 作业事实登记按作业范围授予，管理台信封不收这一格：整格持有读起来就是所有作业范围一并授出，同「运营决定」一格。
func TestAdminConsoleEnvelopeNeverCarriesWorkFactRegistration(t *testing.T) {
	subject := operatorSubject(t)
	binding, err := accessidentity.NewOperatorBinding(subject, operatorTenant, "SYN-BASIS-BINDING")
	if err != nil {
		t.Fatal(err)
	}
	interval, err := accessidentity.NewEffectiveInterval(mintAt.Add(-24*time.Hour), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	write, err := accessidentity.NewOperatorGrant(operatorTenant, "SYN-GRANT-W", subject, accessidentity.CapabilityRegistryConfigurationWrite, interval, "SYN-BASIS-GRANT")
	if err != nil {
		t.Fatal(err)
	}
	work, err := accessidentity.NewWorkFactRegistrationGrant(operatorTenant, "SYN-GRANT-WF", subject, nodeScope(t), interval, "SYN-BASIS-GRANT")
	if err != nil {
		t.Fatal(err)
	}
	var grants []accessidentity.RecordedGrant
	for _, grant := range []accessidentity.OperatorGrant{write, work} {
		recorded, err := accessidentity.NewRecordedGrant(grant, nil)
		if err != nil {
			t.Fatal(err)
		}
		grants = append(grants, recorded)
	}
	standing, err := accessidentity.NewOperatorStanding(binding, grants)
	if err != nil {
		t.Fatal(err)
	}
	minter := newMinter(t, verifierFake{subject: subject}, registryFake{standing: standing, found: true})

	envelope, err := mint(minter, operatorTenant, accessidentity.CapabilityRegistryConfigurationWrite)
	if err != nil {
		t.Fatalf("registry write: %v", err)
	}
	if envelope.Holds(accessidentity.CapabilityWorkFactRegistration) {
		t.Fatalf("admin console envelope carries the work fact face")
	}
	if _, err := mint(minter, operatorTenant, accessidentity.CapabilityWorkFactRegistration); !errors.Is(err, accessidentity.ErrOperatorNotGranted) {
		t.Fatalf("admin console asked for the work fact face: err = %v, want ErrOperatorNotGranted", err)
	}
}

func TestZeroWorkFactEnvelopeIsUnusable(t *testing.T) {
	var envelope accessidentity.WorkFactEnvelope
	if envelope.Minted() || envelope.TenantID() != "" || envelope.DeviceID() != "" {
		t.Fatalf("zero envelope reads as minted %v, tenant %q, device %q", envelope.Minted(), envelope.TenantID(), envelope.DeviceID())
	}
}

func TestWorkFactMinterNeedsAllItsDependencies(t *testing.T) {
	verifier := verifierFake{}
	operators := registryFake{}
	devices := deviceRegistryFake{}
	admission := accessidentity.UnconfiguredAdmissionScope{}
	now := func() time.Time { return mintAt }
	for name, build := range map[string]func() (*accessidentity.WorkFactMinter, error){
		"verifier": func() (*accessidentity.WorkFactMinter, error) {
			return accessidentity.NewWorkFactMinter(nil, operators, devices, admission, now)
		},
		"operator register": func() (*accessidentity.WorkFactMinter, error) {
			return accessidentity.NewWorkFactMinter(verifier, nil, devices, admission, now)
		},
		"device register": func() (*accessidentity.WorkFactMinter, error) {
			return accessidentity.NewWorkFactMinter(verifier, operators, nil, admission, now)
		},
		"admission": func() (*accessidentity.WorkFactMinter, error) {
			return accessidentity.NewWorkFactMinter(verifier, operators, devices, nil, now)
		},
		"clock": func() (*accessidentity.WorkFactMinter, error) {
			return accessidentity.NewWorkFactMinter(verifier, operators, devices, admission, nil)
		},
	} {
		if _, err := build(); !errors.Is(err, accessidentity.ErrNilDependency) {
			t.Fatalf("minter without %s: err = %v, want ErrNilDependency", name, err)
		}
	}
}
