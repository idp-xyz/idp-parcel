package accessidentity_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
)

const (
	operatorTenant = "SYN-TENANT-01"
	operatorIssuer = "https://id.syn.example/dex"
)

var mintAt = time.Date(2026, 9, 25, 2, 0, 0, 0, time.UTC)

type verifierFake struct {
	subject accessidentity.OperatorSubject
	err     error
}

func (fake verifierFake) VerifyOperatorCredential(context.Context, accessidentity.OperatorCredential) (accessidentity.OperatorSubject, error) {
	return fake.subject, fake.err
}

type registryFake struct {
	standing accessidentity.OperatorStanding
	found    bool
	err      error
}

func (fake registryFake) FindOperator(context.Context, accessidentity.OperatorSubject) (accessidentity.OperatorStanding, bool, error) {
	return fake.standing, fake.found, fake.err
}

func operatorSubject(t *testing.T) accessidentity.OperatorSubject {
	t.Helper()
	subject, err := accessidentity.NewOperatorSubject(operatorIssuer, "SYN-OPERATOR-01")
	if err != nil {
		t.Fatal(err)
	}
	return subject
}

// standingWith 登一个绑在 operatorTenant 上、持有 faces 的操作者；授予从 mintAt 前一天起生效、不设终点。
func standingWith(t *testing.T, faces ...accessidentity.CapabilityFace) accessidentity.OperatorStanding {
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
	for index, face := range faces {
		grant, err := accessidentity.NewOperatorGrant(operatorTenant, "SYN-GRANT-"+string(rune('A'+index)), subject, face, interval, "SYN-BASIS-GRANT")
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

func newMinter(t *testing.T, verifier accessidentity.OperatorCredentialVerifier, registry accessidentity.OperatorRegistry) *accessidentity.OperatorMinter {
	t.Helper()
	minter, err := accessidentity.NewOperatorMinter(verifier, registry, func() time.Time { return mintAt })
	if err != nil {
		t.Fatalf("NewOperatorMinter: %v", err)
	}
	return minter
}

func mint(minter *accessidentity.OperatorMinter, tenant string, face accessidentity.CapabilityFace) (accessidentity.OperatorEnvelope, error) {
	return minter.MintOperator(context.Background(), accessidentity.NewOperatorCredential("presented.operator.token"),
		accessidentity.OperatorRequest{TenantID: tenant, Face: face})
}

func TestGrantedOperatorIsMintedIntoAnOperatorEnvelope(t *testing.T) {
	standing := standingWith(t, accessidentity.CapabilityRegistryConfigurationWrite, accessidentity.CapabilityMasterDataAndOperationsRead)
	minter := newMinter(t, verifierFake{subject: operatorSubject(t)}, registryFake{standing: standing, found: true})

	envelope, err := mint(minter, operatorTenant, accessidentity.CapabilityRegistryConfigurationWrite)
	if err != nil {
		t.Fatalf("granted operator: %v", err)
	}
	if !envelope.Minted() || envelope.TenantID() != operatorTenant || envelope.Subject() != operatorSubject(t) {
		t.Fatalf("envelope = minted %v, tenant %q, subject %v", envelope.Minted(), envelope.TenantID(), envelope.Subject())
	}
	if envelope.Source() != accessidentity.OperatorSourceAdminConsole {
		t.Fatalf("source = %q, want the admin console", envelope.Source())
	}
	for _, face := range []accessidentity.CapabilityFace{accessidentity.CapabilityRegistryConfigurationWrite, accessidentity.CapabilityMasterDataAndOperationsRead} {
		if !envelope.Holds(face) {
			t.Fatalf("envelope does not carry the effective grant %s", face)
		}
	}
}

// grades 是答复代数的四格：三格对外（ADR-0100 决定四），外加依赖故障。每一格只许被自己那个哨兵认出。
var grades = map[string]error{
	"not configured":       accessidentity.ErrAccessChannelNotConfigured,
	"credential rejected":  accessidentity.ErrCredentialRejected,
	"not granted":          accessidentity.ErrOperatorNotGranted,
	"verifier unavailable": accessidentity.ErrCredentialVerifierUnavailable,
	"registry unavailable": accessidentity.ErrOperatorRegistryUnavailable,
}

func assertGrade(t *testing.T, err error, want string) {
	t.Helper()
	for name, sentinel := range grades {
		if got := errors.Is(err, sentinel); got != (name == want) {
			t.Fatalf("err = %v: errors.Is(%s) = %v, want only %q", err, name, got, want)
		}
	}
}

func TestOperatorAnswerGradesDoNotStandInForEachOther(t *testing.T) {
	subject := operatorSubject(t)
	writer := standingWith(t, accessidentity.CapabilityRegistryConfigurationWrite)
	revokedAt := mintAt.Add(-time.Hour)
	cases := map[string]struct {
		verifier accessidentity.OperatorCredentialVerifier
		registry accessidentity.OperatorRegistry
		tenant   string
		want     string
	}{
		"issuer parameters unset": {
			verifier: accessidentity.UnconfiguredOperatorCredentialVerifier{},
			registry: registryFake{standing: writer, found: true}, tenant: operatorTenant, want: "not configured",
		},
		"token rejected": {
			verifier: verifierFake{err: accessidentity.ErrCredentialRejected},
			registry: registryFake{standing: writer, found: true}, tenant: operatorTenant, want: "credential rejected",
		},
		"issuer key set unreachable": {
			verifier: verifierFake{err: accessidentity.ErrCredentialVerifierUnavailable},
			registry: registryFake{standing: writer, found: true}, tenant: operatorTenant, want: "verifier unavailable",
		},
		"operator not in the register": {
			verifier: verifierFake{subject: subject},
			registry: registryFake{found: false}, tenant: operatorTenant, want: "not granted",
		},
		"operator bound to another tenant": {
			verifier: verifierFake{subject: subject},
			registry: registryFake{standing: writer, found: true}, tenant: "SYN-TENANT-02", want: "not granted",
		},
		"request names no tenant": {
			verifier: verifierFake{subject: subject},
			registry: registryFake{standing: writer, found: true}, tenant: "", want: "not granted",
		},
		"no grant for the requested face": {
			verifier: verifierFake{subject: subject},
			registry: registryFake{standing: standingWith(t, accessidentity.CapabilityMasterDataAndOperationsRead), found: true},
			tenant:   operatorTenant, want: "not granted",
		},
		"grant revoked before the request": {
			verifier: verifierFake{subject: subject},
			registry: registryFake{standing: revokedStanding(t, revokedAt), found: true}, tenant: operatorTenant, want: "not granted",
		},
		"register cannot be read": {
			verifier: verifierFake{subject: subject},
			registry: registryFake{err: errors.New("connection refused")}, tenant: operatorTenant, want: "registry unavailable",
		},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			minter := newMinter(t, testCase.verifier, testCase.registry)
			envelope, err := mint(minter, testCase.tenant, accessidentity.CapabilityRegistryConfigurationWrite)
			assertGrade(t, err, testCase.want)
			if envelope.Minted() {
				t.Fatalf("a refused presentation still produced a minted envelope")
			}
		})
	}
}

func revokedStanding(t *testing.T, revokedAt time.Time) accessidentity.OperatorStanding {
	t.Helper()
	standing := standingWith(t, accessidentity.CapabilityRegistryConfigurationWrite)
	grant := standing.Grants()[0].Grant()
	revocation, err := accessidentity.NewGrantRevocation(operatorTenant, grant.GrantID(), revokedAt, "SYN-BASIS-REVOKE")
	if err != nil {
		t.Fatal(err)
	}
	recorded, err := accessidentity.NewRecordedGrant(grant, &revocation)
	if err != nil {
		t.Fatal(err)
	}
	revoked, err := accessidentity.NewOperatorStanding(standing.Binding(), []accessidentity.RecordedGrant{recorded})
	if err != nil {
		t.Fatal(err)
	}
	return revoked
}

func TestZeroOperatorEnvelopeIsUnusable(t *testing.T) {
	var envelope accessidentity.OperatorEnvelope
	if envelope.Minted() || envelope.TenantID() != "" || envelope.Holds(accessidentity.CapabilityRegistryConfigurationWrite) {
		t.Fatalf("zero envelope reads as minted %v, tenant %q", envelope.Minted(), envelope.TenantID())
	}
}

func TestOperatorMinterNeedsAllItsDependencies(t *testing.T) {
	verifier := verifierFake{}
	registry := registryFake{}
	now := func() time.Time { return mintAt }
	for name, build := range map[string]func() (*accessidentity.OperatorMinter, error){
		"verifier": func() (*accessidentity.OperatorMinter, error) {
			return accessidentity.NewOperatorMinter(nil, registry, now)
		},
		"registry": func() (*accessidentity.OperatorMinter, error) {
			return accessidentity.NewOperatorMinter(verifier, nil, now)
		},
		"clock": func() (*accessidentity.OperatorMinter, error) {
			return accessidentity.NewOperatorMinter(verifier, registry, nil)
		},
	} {
		if _, err := build(); !errors.Is(err, accessidentity.ErrNilDependency) {
			t.Fatalf("minter without %s: err = %v, want ErrNilDependency", name, err)
		}
	}
}
