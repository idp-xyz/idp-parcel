package accessidentity_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
)

var clientMintAt = time.Date(2026, 10, 9, 2, 0, 0, 0, time.UTC)

type clientVerifierFake struct {
	verified accessidentity.VerifiedClientToken
	err      error
}

func (fake clientVerifierFake) VerifyIntegrationClientCredential(
	context.Context,
	accessidentity.IntegrationClientCredential,
) (accessidentity.VerifiedClientToken, error) {
	return fake.verified, fake.err
}

type clientRegistryFake struct {
	standing accessidentity.IntegrationClientStanding
	found    bool
	err      error
}

func (fake clientRegistryFake) FindIntegrationClient(
	context.Context,
	accessidentity.IntegrationClientSubject,
) (accessidentity.IntegrationClientStanding, bool, error) {
	return fake.standing, fake.found, fake.err
}

func verifiedToken(t *testing.T, certificateBound bool) accessidentity.VerifiedClientToken {
	t.Helper()
	return accessidentity.NewVerifiedClientToken(clientSubjectOf(t, "SYN-CLIENT-01"), certificateBound)
}

// clientStandingWith 登一个绑在 clientTenant 上的客户端，按 facts 各授一笔；授予从 clientMintAt 前一天起、到后一天止。
func clientStandingWith(t *testing.T, certificateBound bool, facts ...accessidentity.ExternalFactType) accessidentity.IntegrationClientStanding {
	t.Helper()
	subject := clientSubjectOf(t, "SYN-CLIENT-01")
	interval := clientIntervalOf(t, clientMintAt.Add(-24*time.Hour), clientMintAt.Add(24*time.Hour))
	var grants []accessidentity.RecordedClientGrant
	for index, fact := range facts {
		grants = append(grants, recordedClientGrantOf(t,
			clientGrantOf(t, fmt.Sprintf("SYN-CLIENT-GRANT-%02d", index+1), subject, fact, interval), nil))
	}
	standing, err := accessidentity.NewIntegrationClientStanding(clientBindingOf(t, subject, clientTenant, certificateBound), grants)
	if err != nil {
		t.Fatal(err)
	}
	return standing
}

func newClientMinter(
	t *testing.T,
	verifier accessidentity.IntegrationClientCredentialVerifier,
	registry accessidentity.IntegrationClientRegistry,
	at time.Time,
) *accessidentity.IntegrationClientMinter {
	t.Helper()
	minter, err := accessidentity.NewIntegrationClientMinter(verifier, registry, accessidentity.UnconfiguredAdmissionScope{}, func() time.Time { return at })
	if err != nil {
		t.Fatalf("NewIntegrationClientMinter: %v", err)
	}
	return minter
}

func mintClient(minter *accessidentity.IntegrationClientMinter, request accessidentity.IntegrationClientRequest) (accessidentity.IntegrationClientEnvelope, error) {
	return minter.MintIntegrationClient(context.Background(),
		accessidentity.NewIntegrationClientCredential("presented.client.token", nil), request)
}

// Covers: 票面做什么第 3 条「集成客户端信封：租户、客户端、来源身份、授予集」——身份三件只取自册上的绑定；授予集是铸造
// 那一刻生效的各类事实，不只所请求的那一类。
func TestGrantedIntegrationClientIsMintedIntoAnIntegrationClientEnvelope(t *testing.T) {
	standing := clientStandingWith(t, false, accessidentity.FactExternalFunds, accessidentity.FactCustomsExternalResult)
	minter := newClientMinter(t, clientVerifierFake{verified: verifiedToken(t, false)}, clientRegistryFake{standing: standing, found: true}, clientMintAt)

	envelope, err := mintClient(minter, accessidentity.IntegrationClientRequest{FactType: accessidentity.FactExternalFunds})
	if err != nil {
		t.Fatalf("granted client: %v", err)
	}
	if !envelope.Minted() || envelope.TenantID() != clientTenant || envelope.Client() != clientSubjectOf(t, "SYN-CLIENT-01") ||
		envelope.SourceIdentity() != clientSource {
		t.Fatalf("envelope = minted %v, tenant %q, client %v, source %q", envelope.Minted(), envelope.TenantID(), envelope.Client(), envelope.SourceIdentity())
	}
	for _, fact := range []accessidentity.ExternalFactType{accessidentity.FactExternalFunds, accessidentity.FactCustomsExternalResult} {
		if !envelope.Holds(fact) {
			t.Fatalf("envelope does not carry the effective grant %s", fact)
		}
	}
	if envelope.Holds(accessidentity.FactRegulatoryCredential) {
		t.Fatal("envelope carries a fact type the client was never granted")
	}
}

// clientGrades 是集成客户端答复代数的各格：对外沿 ADR-0100 决定四（未配置、令牌不过、未授予），外加不在准入范围与
// 各种依赖故障。每一格只许被自己那个哨兵认出。
var clientGrades = map[string]error{
	"not configured":        accessidentity.ErrAccessChannelNotConfigured,
	"credential rejected":   accessidentity.ErrCredentialRejected,
	"not granted":           accessidentity.ErrIntegrationClientNotGranted,
	"verifier unavailable":  accessidentity.ErrCredentialVerifierUnavailable,
	"registry unavailable":  accessidentity.ErrIntegrationClientRegistryUnavailable,
	"outside admission":     accessidentity.ErrOutsideAdmissionScope,
	"admission unavailable": accessidentity.ErrAdmissionScopeUnavailable,
}

func assertClientGrade(t *testing.T, err error, want string) {
	t.Helper()
	for name, sentinel := range clientGrades {
		if got := errors.Is(err, sentinel); got != (name == want) {
			t.Fatalf("err = %v: errors.Is(%s) = %v, want only %q", err, name, got, want)
		}
	}
}

// Covers: 票面完成判据「客户端未登记、授予不含该事实类型、区间外、令牌无效各答其格」——令牌那一侧的三格原样交回；册的
// 那一侧未登记、无此类授予、区间外（起点前、终点后）与已撤销同答「未授予」、不说哪一半不对（ADR-0100 决定四照
// ADR-0055 决定四的探针同答）；册读不动答依赖故障而不是未授予；要求证书绑定而令牌未绑答令牌不过。
func TestIntegrationClientAnswerGradesDoNotStandInForEachOther(t *testing.T) {
	unbound := clientVerifierFake{verified: verifiedToken(t, false)}
	funds := clientRegistryFake{standing: clientStandingWith(t, false, accessidentity.FactExternalFunds), found: true}
	revoked := func() clientRegistryFake {
		standing := clientStandingWith(t, false, accessidentity.FactExternalFunds)
		grant := standing.Grants()[0].Grant()
		revocation, err := accessidentity.NewGrantRevocation(clientTenant, grant.GrantID(), clientMintAt.Add(-time.Hour), "SYN-REVOKE-BASIS")
		if err != nil {
			t.Fatal(err)
		}
		revokedStanding, err := accessidentity.NewIntegrationClientStanding(standing.Binding(),
			[]accessidentity.RecordedClientGrant{recordedClientGrantOf(t, grant, &revocation)})
		if err != nil {
			t.Fatal(err)
		}
		return clientRegistryFake{standing: revokedStanding, found: true}
	}()
	cases := map[string]struct {
		verifier accessidentity.IntegrationClientCredentialVerifier
		registry accessidentity.IntegrationClientRegistry
		at       time.Time
		want     string
	}{
		"issuer parameters unset": {
			verifier: accessidentity.UnconfiguredIntegrationClientCredentialVerifier{}, registry: funds, at: clientMintAt, want: "not configured",
		},
		"token rejected": {
			verifier: clientVerifierFake{err: accessidentity.ErrCredentialRejected}, registry: funds, at: clientMintAt, want: "credential rejected",
		},
		"issuer key set unreachable": {
			verifier: clientVerifierFake{err: accessidentity.ErrCredentialVerifierUnavailable}, registry: funds, at: clientMintAt, want: "verifier unavailable",
		},
		"client not in the register": {
			verifier: unbound, registry: clientRegistryFake{found: false}, at: clientMintAt, want: "not granted",
		},
		"grant lacks the requested fact type": {
			verifier: unbound, registry: clientRegistryFake{standing: clientStandingWith(t, false, accessidentity.FactCustomsExternalResult), found: true},
			at: clientMintAt, want: "not granted",
		},
		"request before the grant interval": {
			verifier: unbound, registry: funds, at: clientMintAt.Add(-25 * time.Hour), want: "not granted",
		},
		"request after the grant interval": {
			verifier: unbound, registry: funds, at: clientMintAt.Add(24 * time.Hour), want: "not granted",
		},
		"grant revoked before the request": {
			verifier: unbound, registry: revoked, at: clientMintAt, want: "not granted",
		},
		"register cannot be read": {
			verifier: unbound, registry: clientRegistryFake{err: errors.New("connection refused")}, at: clientMintAt, want: "registry unavailable",
		},
		"client requires a certificate-bound token but the token is not bound": {
			verifier: unbound, registry: clientRegistryFake{standing: clientStandingWith(t, true, accessidentity.FactExternalFunds), found: true},
			at: clientMintAt, want: "credential rejected",
		},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			minter := newClientMinter(t, testCase.verifier, testCase.registry, testCase.at)
			envelope, err := mintClient(minter, accessidentity.IntegrationClientRequest{FactType: accessidentity.FactExternalFunds})
			assertClientGrade(t, err, testCase.want)
			if envelope.Minted() {
				t.Fatal("a refused presentation still produced a minted envelope")
			}
		})
	}
}

// Covers: 票面做什么第 2 条「可选 mTLS 绑定」——要求证书绑定的客户端出示绑定且对得上的令牌照常铸成。
func TestCertificateBoundTokenSatisfiesAClientThatRequiresIt(t *testing.T) {
	minter := newClientMinter(t, clientVerifierFake{verified: verifiedToken(t, true)},
		clientRegistryFake{standing: clientStandingWith(t, true, accessidentity.FactExternalFunds), found: true}, clientMintAt)
	if envelope, err := mintClient(minter, accessidentity.IntegrationClientRequest{FactType: accessidentity.FactExternalFunds}); err != nil || !envelope.Minted() {
		t.Fatalf("certificate-bound token for a client that requires it: minted %v, err %v", envelope.Minted(), err)
	}
}

var fundsAdmission = &accessidentity.AdmissionRequirement{Capability: "SYN-CAP/external-funds", FactKind: "SYN-FACT/external-funds-fact"}

// Covers: 票 operator-channel/14「接进各族铸造随 10、11」——请求带准入要求时铸造前判准入范围，判在授予之后；不覆盖答
// 「不在准入范围」、读不动答依赖故障，两格不互相顶替。
func TestAdmissionScopeIsJudgedAfterTheIntegrationClientGrant(t *testing.T) {
	granted := clientRegistryFake{standing: clientStandingWith(t, false, accessidentity.FactExternalFunds), found: true}
	cases := map[string]struct {
		registry    accessidentity.IntegrationClientRegistry
		admitted    bool
		err         error
		requirement *accessidentity.AdmissionRequirement
		want        string
		wantCalls   int
	}{
		"interval covers the write":                    {registry: granted, admitted: true, requirement: fundsAdmission, wantCalls: 1},
		"no interval covers the write":                 {registry: granted, requirement: fundsAdmission, want: "outside admission", wantCalls: 1},
		"admission scope cannot be read":               {registry: granted, err: errors.New("connection refused"), requirement: fundsAdmission, want: "admission unavailable", wantCalls: 1},
		"request carries no admission requirement":     {registry: granted, err: errors.New("must not be consulted"), wantCalls: 0},
		"client without the grant is refused up front": {registry: clientRegistryFake{found: false}, admitted: true, requirement: fundsAdmission, want: "not granted", wantCalls: 0},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			calls := 0
			minter, err := accessidentity.NewIntegrationClientMinter(clientVerifierFake{verified: verifiedToken(t, false)}, testCase.registry,
				admissionFake{admitted: testCase.admitted, err: testCase.err, calls: &calls}, func() time.Time { return clientMintAt })
			if err != nil {
				t.Fatal(err)
			}
			envelope, err := mintClient(minter, accessidentity.IntegrationClientRequest{FactType: accessidentity.FactExternalFunds, Admission: testCase.requirement})
			if testCase.want == "" {
				if err != nil || !envelope.Minted() {
					t.Fatalf("err = %v, minted = %v; want a minted envelope", err, envelope.Minted())
				}
			} else {
				assertClientGrade(t, err, testCase.want)
			}
			if calls != testCase.wantCalls {
				t.Fatalf("admission scope consulted %d times, want %d", calls, testCase.wantCalls)
			}
		})
	}
}

// Covers: 请求没说清要交哪一类事实是装配缺陷，不是任何一格答复——折成「未授予」会让管授予的人去登一笔永远不会被问到的授予。
func TestRequestWithoutAKnownFactTypeIsAnAssemblyDefect(t *testing.T) {
	minter := newClientMinter(t, clientVerifierFake{verified: verifiedToken(t, false)},
		clientRegistryFake{standing: clientStandingWith(t, false, accessidentity.FactExternalFunds), found: true}, clientMintAt)
	for _, fact := range []accessidentity.ExternalFactType{"", "ANYTHING"} {
		_, err := mintClient(minter, accessidentity.IntegrationClientRequest{FactType: fact})
		if !errors.Is(err, accessidentity.ErrExternalFactTypeUnknown) {
			t.Fatalf("fact type %q: err = %v, want ErrExternalFactTypeUnknown", fact, err)
		}
		for name, sentinel := range clientGrades {
			if errors.Is(err, sentinel) {
				t.Fatalf("fact type %q: the defect reads as the %s grade", fact, name)
			}
		}
	}
}

// Covers: 票 operator-channel/03 同款「零值信封不可用」——包外写得出零值，零值既不算铸过、也不持有任何一类授予。
func TestZeroIntegrationClientEnvelopeIsUnusable(t *testing.T) {
	var envelope accessidentity.IntegrationClientEnvelope
	if envelope.Minted() || envelope.TenantID() != "" || envelope.SourceIdentity() != "" || envelope.Holds(accessidentity.FactExternalFunds) {
		t.Fatalf("zero envelope reads as minted %v, tenant %q, source %q", envelope.Minted(), envelope.TenantID(), envelope.SourceIdentity())
	}
}

func TestIntegrationClientMinterNeedsAllItsDependencies(t *testing.T) {
	verifier := clientVerifierFake{}
	registry := clientRegistryFake{}
	admission := accessidentity.UnconfiguredAdmissionScope{}
	now := func() time.Time { return clientMintAt }
	for name, build := range map[string]func() (*accessidentity.IntegrationClientMinter, error){
		"verifier": func() (*accessidentity.IntegrationClientMinter, error) {
			return accessidentity.NewIntegrationClientMinter(nil, registry, admission, now)
		},
		"registry": func() (*accessidentity.IntegrationClientMinter, error) {
			return accessidentity.NewIntegrationClientMinter(verifier, nil, admission, now)
		},
		"admission": func() (*accessidentity.IntegrationClientMinter, error) {
			return accessidentity.NewIntegrationClientMinter(verifier, registry, nil, now)
		},
		"clock": func() (*accessidentity.IntegrationClientMinter, error) {
			return accessidentity.NewIntegrationClientMinter(verifier, registry, admission, nil)
		},
	} {
		if _, err := build(); !errors.Is(err, accessidentity.ErrNilDependency) {
			t.Fatalf("minter without %s: err = %v, want ErrNilDependency", name, err)
		}
	}
}

const presentedClientToken = "eyJhbGciOiJSUzI1NiJ9.eyJzdWIiOiJTWU4tQ0xJRU5ULTAxIn0.Y2xpZW50LXNpZ25hdHVyZQ"

// Covers: 票面「凭据本体不入库」与 ADR-0149 越权风险点 4——令牌本体经 fmt 各动词、slog 两种处理器、JSON 与错误信息都
// 印不出；核验方仍拿得到令牌与证书。
func TestIntegrationClientCredentialNeverRendersTheToken(t *testing.T) {
	certificate := []byte{0x30, 0x82, 0x01, 0x0a}
	credential := accessidentity.NewIntegrationClientCredential(" "+presentedClientToken+" ", certificate)

	var jsonLog, textLog bytes.Buffer
	slog.New(slog.NewJSONHandler(&jsonLog, nil)).Info("presented", "credential", credential)
	slog.New(slog.NewTextHandler(&textLog, nil)).Info("presented", "credential", credential)
	marshalled, err := json.Marshal(credential)
	if err != nil {
		t.Fatal(err)
	}
	renderings := map[string]string{
		"fmt verbs": fmt.Sprintf("%v|%+v|%#v|%s|%q|%x", credential, credential, credential, credential, credential, credential),
		"slog JSON": jsonLog.String(),
		"slog text": textLog.String(),
		"JSON":      string(marshalled),
		"error":     fmt.Errorf("verify %v: %w", credential, errors.New("boom")).Error(),
	}
	for name, rendered := range renderings {
		if strings.Contains(rendered, "eyJ") || strings.Contains(rendered, "Y2xpZW50LXNpZ25hdHVyZQ") {
			t.Errorf("%s renders the token: %s", name, rendered)
		}
	}
	if credential.Token() != presentedClientToken {
		t.Fatalf("Token() = %q, want the presented token for the verifier", credential.Token())
	}
	presented := credential.Certificate()
	if !bytes.Equal(presented, certificate) {
		t.Fatalf("Certificate() = %x, want the presented certificate", presented)
	}
	presented[0] = 0xff
	if !bytes.Equal(credential.Certificate(), certificate) {
		t.Fatal("Certificate() hands out the credential's own bytes; a caller could rewrite what the verifier sees")
	}
}

func TestUnconfiguredIntegrationClientVerifierAnswersNotConfiguredToAnyPresentation(t *testing.T) {
	var verifier accessidentity.IntegrationClientCredentialVerifier = accessidentity.UnconfiguredIntegrationClientCredentialVerifier{}
	for _, token := range []string{presentedClientToken, ""} {
		if _, err := verifier.VerifyIntegrationClientCredential(context.Background(),
			accessidentity.NewIntegrationClientCredential(token, nil)); !errors.Is(err, accessidentity.ErrAccessChannelNotConfigured) {
			t.Fatalf("token %q: err = %v, want ErrAccessChannelNotConfigured", token, err)
		}
	}
}
