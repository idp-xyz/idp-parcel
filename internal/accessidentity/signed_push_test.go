package accessidentity_test

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
)

// 推送在集成客户端授予的区间正中收到：授予从 clientMintAt 前一天起、到后一天止（clientStandingWith）。
var pushReceivedAt = clientMintAt

const (
	pushSourceName = "SYN-PUSH-SOURCE/bank-01"
	pushWindow     = 5 * time.Minute
	pushClockSkew  = 30 * time.Second
	pushBody       = `{"fact":"SYN-FUNDS-FACT-01","amount":"1.00"}`
)

// pushKey 是用例里现生成的一把签名密钥：HMAC 共享密钥或 Ed25519 密钥对。材料不入仓、不出用例。
type pushKey struct {
	id      string
	form    accessidentity.PushSignatureForm
	secret  []byte
	private ed25519.PrivateKey
	public  ed25519.PublicKey
}

func newHMACPushKey(t *testing.T, id string) pushKey {
	t.Helper()
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		t.Fatal(err)
	}
	return pushKey{id: id, form: accessidentity.PushSignatureHMACSHA256, secret: secret}
}

func newEd25519PushKey(t *testing.T, id string) pushKey {
	t.Helper()
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return pushKey{id: id, form: accessidentity.PushSignatureEd25519, private: private, public: public}
}

func (key pushKey) reference() string { return "SYN-PUSH-KEY-REF/" + key.id }

func (key pushKey) material() []byte {
	if key.form == accessidentity.PushSignatureEd25519 {
		return key.public
	}
	return key.secret
}

// sign 照 Standard Webhooks 签 `msg_id.timestamp.payload`，交回 webhook-signature 头里的一项。
func (key pushKey) sign(id string, timestamp int64, body string) string {
	content := []byte(id + "." + strconv.FormatInt(timestamp, 10) + "." + body)
	if key.form == accessidentity.PushSignatureEd25519 {
		return "v1a," + base64.StdEncoding.EncodeToString(ed25519.Sign(key.private, content))
	}
	mac := hmac.New(sha256.New, key.secret)
	mac.Write(content)
	return "v1," + base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

type pushKeyStoreFake struct {
	materials map[string][]byte
	err       error
}

func keyStoreOf(keys ...pushKey) pushKeyStoreFake {
	materials := map[string][]byte{}
	for _, key := range keys {
		materials[key.reference()] = key.material()
	}
	return pushKeyStoreFake{materials: materials}
}

func (fake pushKeyStoreFake) ResolvePushSigningKey(
	_ context.Context,
	reference accessidentity.CredentialReference,
) (accessidentity.PushSigningKeyMaterial, error) {
	if fake.err != nil {
		return accessidentity.PushSigningKeyMaterial{}, fake.err
	}
	material, ok := fake.materials[reference.String()]
	if !ok {
		return accessidentity.PushSigningKeyMaterial{}, fmt.Errorf("restricted store holds nothing under %s", reference)
	}
	return accessidentity.NewPushSigningKeyMaterial(material), nil
}

type pushSourceRegistryFake struct {
	standing accessidentity.PushSourceStanding
	found    bool
	err      error
}

func (fake pushSourceRegistryFake) FindPushSource(
	context.Context,
	accessidentity.PushSourceID,
) (accessidentity.PushSourceStanding, bool, error) {
	return fake.standing, fake.found, fake.err
}

// pushLedgerFake 按（推送源、签名原文摘要）记收下过的推送。
type pushLedgerFake struct {
	seen map[string]bool
	err  error
}

func newPushLedger() *pushLedgerFake { return &pushLedgerFake{seen: map[string]bool{}} }

func (fake *pushLedgerFake) RecordPushReceipt(_ context.Context, receipt accessidentity.PushReceipt) (bool, error) {
	if fake.err != nil {
		return false, fake.err
	}
	key := receipt.Source().String() + "|" + receipt.Digest()
	if fake.seen[key] {
		return false, nil
	}
	fake.seen[key] = true
	return true, nil
}

func pushSourceOf(t *testing.T) accessidentity.PushSourceID {
	t.Helper()
	source, err := accessidentity.NewPushSourceID(pushSourceName)
	if err != nil {
		t.Fatalf("推送源标识：%v", err)
	}
	return source
}

// registeredPushKey 是推送源名下登记的一把钥：endsAt 与 retiresAt 取零值表示不设终点、未停用。
type registeredPushKey struct {
	key       pushKey
	startsAt  time.Time
	endsAt    time.Time
	retiresAt time.Time
}

func openKey(key pushKey) registeredPushKey {
	return registeredPushKey{key: key, startsAt: pushReceivedAt.Add(-24 * time.Hour)}
}

func pushStandingWith(t *testing.T, keys ...registeredPushKey) accessidentity.PushSourceStanding {
	t.Helper()
	source := pushSourceOf(t)
	binding, err := accessidentity.NewPushSourceBinding(source, clientSubjectOf(t, "SYN-CLIENT-01"), clientTenant,
		pushWindow, pushClockSkew, "SYN-PUSH-SOURCE-BASIS-01")
	if err != nil {
		t.Fatalf("推送源绑定：%v", err)
	}
	var recorded []accessidentity.RecordedPushSigningKey
	for _, registered := range keys {
		key, err := accessidentity.NewPushSigningKey(clientTenant, source, registered.key.id, registered.key.form,
			credentialReferenceOf(t, registered.key.reference()), clientIntervalOf(t, registered.startsAt, registered.endsAt),
			"SYN-PUSH-KEY-BASIS-"+registered.key.id)
		if err != nil {
			t.Fatalf("签名密钥 %s：%v", registered.key.id, err)
		}
		var retirement *accessidentity.PushSigningKeyRetirement
		if !registered.retiresAt.IsZero() {
			retired, err := accessidentity.NewPushSigningKeyRetirement(clientTenant, source, registered.key.id,
				registered.retiresAt, "SYN-PUSH-KEY-RETIREMENT-BASIS")
			if err != nil {
				t.Fatalf("停用 %s：%v", registered.key.id, err)
			}
			retirement = &retired
		}
		entry, err := accessidentity.NewRecordedPushSigningKey(key, retirement)
		if err != nil {
			t.Fatalf("在册密钥 %s：%v", registered.key.id, err)
		}
		recorded = append(recorded, entry)
	}
	standing, err := accessidentity.NewPushSourceStanding(binding, recorded)
	if err != nil {
		t.Fatalf("推送源现状：%v", err)
	}
	return standing
}

// pushOf 造一份在 signedAt 签出的 Standard Webhooks 推送，签名头里每个 signer 一项。
func pushOf(t *testing.T, id string, signedAt time.Time, signers ...pushKey) accessidentity.SignedPush {
	t.Helper()
	var signatures []string
	for _, signer := range signers {
		signatures = append(signatures, signer.sign(id, signedAt.Unix(), pushBody))
	}
	push, err := accessidentity.ParseStandardWebhook(id, strconv.FormatInt(signedAt.Unix(), 10),
		strings.Join(signatures, " "), []byte(pushBody))
	if err != nil {
		t.Fatalf("ParseStandardWebhook: %v", err)
	}
	return push
}

type pushMinterParts struct {
	sources accessidentity.PushSourceRegistry
	keys    accessidentity.PushSigningKeyResolver
	clients accessidentity.IntegrationClientRegistry
	ledger  accessidentity.PushReplayLedger
	at      time.Time
}

func grantedPushParts(t *testing.T, standing accessidentity.PushSourceStanding, keys ...pushKey) pushMinterParts {
	t.Helper()
	return pushMinterParts{
		sources: pushSourceRegistryFake{standing: standing, found: true},
		keys:    keyStoreOf(keys...),
		clients: clientRegistryFake{standing: clientStandingWith(t, false, accessidentity.FactExternalFunds), found: true},
		ledger:  newPushLedger(),
		at:      pushReceivedAt,
	}
}

func newPushMinter(t *testing.T, parts pushMinterParts) *accessidentity.SignedPushMinter {
	t.Helper()
	minter, err := accessidentity.NewSignedPushMinter(parts.sources, parts.keys, parts.clients,
		accessidentity.UnconfiguredAdmissionScope{}, parts.ledger, func() time.Time { return parts.at })
	if err != nil {
		t.Fatalf("NewSignedPushMinter: %v", err)
	}
	return minter
}

func mintPush(t *testing.T, minter *accessidentity.SignedPushMinter, push accessidentity.SignedPush) (accessidentity.IntegrationClientEnvelope, error) {
	t.Helper()
	return minter.MintSignedPush(context.Background(), pushSourceOf(t), push,
		accessidentity.IntegrationClientRequest{FactType: accessidentity.FactExternalFunds})
}

func assertPushMintedAsTheBoundClient(t *testing.T, envelope accessidentity.IntegrationClientEnvelope, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("signed push: %v", err)
	}
	if !envelope.Minted() || envelope.TenantID() != clientTenant || envelope.Client() != clientSubjectOf(t, "SYN-CLIENT-01") ||
		envelope.SourceIdentity() != clientSource || !envelope.Holds(accessidentity.FactExternalFunds) {
		t.Fatalf("envelope = minted %v, tenant %q, client %v, source %q, holds funds %v", envelope.Minted(), envelope.TenantID(),
			envelope.Client(), envelope.SourceIdentity(), envelope.Holds(accessidentity.FactExternalFunds))
	}
}

// Covers: 票 operator-channel/12 完成判据「两种签名形态各有用例」之 HMAC 共享密钥一形，与做什么
// 「校验通过后以该推送源绑定的集成客户端身份铸信封」——租户、客户端与来源身份只取自集成客户端册上的绑定。
func TestHMACSignedPushIsMintedAsTheBoundIntegrationClient(t *testing.T) {
	key := newHMACPushKey(t, "SYN-PUSH-KEY-HMAC-01")
	minter := newPushMinter(t, grantedPushParts(t, pushStandingWith(t, openKey(key)), key))
	envelope, err := mintPush(t, minter, pushOf(t, "msg_SYN01", pushReceivedAt, key))
	assertPushMintedAsTheBoundClient(t, envelope, err)
}

// Covers: 票面完成判据「两种签名形态各有用例」之对方公钥一形（Ed25519，Standard Webhooks 的 v1a）：册上只登公钥的
// 受控引用，私钥只在推送方手里。
func TestEd25519SignedPushIsMintedAsTheBoundIntegrationClient(t *testing.T) {
	key := newEd25519PushKey(t, "SYN-PUSH-KEY-ED25519-01")
	minter := newPushMinter(t, grantedPushParts(t, pushStandingWith(t, openKey(key)), key))
	envelope, err := mintPush(t, minter, pushOf(t, "msg_SYN01", pushReceivedAt, key))
	assertPushMintedAsTheBoundClient(t, envelope, err)
}

// Covers: 对方公钥一形验的是这一把公钥，不是「头里有一项 v1a」。另一把私钥签的同一份原文答签名不过。
func TestAnEd25519SignatureFromAnotherKeyIsRejected(t *testing.T) {
	key := newEd25519PushKey(t, "SYN-PUSH-KEY-ED25519-01")
	stranger := newEd25519PushKey(t, "SYN-PUSH-KEY-ED25519-02")
	minter := newPushMinter(t, grantedPushParts(t, pushStandingWith(t, openKey(key)), key))
	_, err := mintPush(t, minter, pushOf(t, "msg_SYN01", pushReceivedAt, stranger))
	assertPushGrade(t, err, "signature rejected")
}

// pushGrades 是签名推送答复代数的各格，外加它会交回的集成客户端族各格。每一格只许被自己那个哨兵认出：签名不过不是
// 令牌不过，越窗不是签名不过，重放不是越窗，依赖故障不折进任何一格业务答复。
var pushGrades = map[string]error{
	"signature rejected":          accessidentity.ErrPushSignatureRejected,
	"timestamp outside window":    accessidentity.ErrPushTimestampOutsideWindow,
	"replayed":                    accessidentity.ErrPushReplayed,
	"not configured":              accessidentity.ErrAccessChannelNotConfigured,
	"not granted":                 accessidentity.ErrIntegrationClientNotGranted,
	"credential rejected":         accessidentity.ErrCredentialRejected,
	"outside admission":           accessidentity.ErrOutsideAdmissionScope,
	"source registry unavailable": accessidentity.ErrPushSourceRegistryUnavailable,
	"signing key unavailable":     accessidentity.ErrPushSigningKeyUnavailable,
	"replay ledger unavailable":   accessidentity.ErrPushReplayLedgerUnavailable,
	"client registry unavailable": accessidentity.ErrIntegrationClientRegistryUnavailable,
	"admission unavailable":       accessidentity.ErrAdmissionScopeUnavailable,
}

func assertPushGrade(t *testing.T, err error, want string) {
	t.Helper()
	for name, sentinel := range pushGrades {
		if got := errors.Is(err, sentinel); got != (name == want) {
			t.Fatalf("err = %v: errors.Is(%s) = %v, want only %q", err, name, got, want)
		}
	}
}

// Covers: 票 operator-channel/12 做什么「签名不过、时间戳越窗、重放各答其格」之前两格与依赖故障——推送源未登记、钥不对、请求体被改、
// 签名形态与在册密钥对不上、钥未生效或已停用同答签名不过，不说哪一半不对；签名成立而时间戳早于窗口或晚于时钟偏差答
// 越窗；推送源册、受限存储、重放账、集成客户端册读写不动各答依赖故障；受限存储没配答未配置。
func TestSignedPushAnswerGradesDoNotStandInForEachOther(t *testing.T) {
	key := newHMACPushKey(t, "SYN-PUSH-KEY-HMAC-01")
	stranger := newHMACPushKey(t, "SYN-PUSH-KEY-NEVER-REGISTERED")
	publicKey := newEd25519PushKey(t, "SYN-PUSH-KEY-ED25519-01")
	open := pushStandingWith(t, openKey(key))
	fresh := pushOf(t, "msg_SYN01", pushReceivedAt, key)
	altered, err := accessidentity.ParseStandardWebhook("msg_SYN01", strconv.FormatInt(pushReceivedAt.Unix(), 10),
		key.sign("msg_SYN01", pushReceivedAt.Unix(), pushBody), []byte(`{"fact":"SYN-FUNDS-FACT-01","amount":"9.00"}`))
	if err != nil {
		t.Fatal(err)
	}
	with := func(change func(*pushMinterParts)) pushMinterParts {
		parts := grantedPushParts(t, open, key)
		change(&parts)
		return parts
	}
	cases := map[string]struct {
		parts pushMinterParts
		push  accessidentity.SignedPush
		want  string
	}{
		"push source not registered": {
			parts: with(func(parts *pushMinterParts) { parts.sources = pushSourceRegistryFake{found: false} }), push: fresh, want: "signature rejected",
		},
		"signed with a key the source never registered": {
			parts: with(func(*pushMinterParts) {}), push: pushOf(t, "msg_SYN01", pushReceivedAt, stranger), want: "signature rejected",
		},
		"body altered after signing": {
			parts: with(func(*pushMinterParts) {}), push: altered, want: "signature rejected",
		},
		"public-key signature while the source holds only an HMAC key": {
			parts: with(func(*pushMinterParts) {}), push: pushOf(t, "msg_SYN01", pushReceivedAt, publicKey), want: "signature rejected",
		},
		"key not yet effective": {
			parts: with(func(parts *pushMinterParts) {
				parts.sources = pushSourceRegistryFake{found: true, standing: pushStandingWith(t,
					registeredPushKey{key: key, startsAt: pushReceivedAt.Add(time.Minute)})}
			}), push: fresh, want: "signature rejected",
		},
		"key retired before the push arrived": {
			parts: with(func(parts *pushMinterParts) {
				parts.sources = pushSourceRegistryFake{found: true, standing: pushStandingWith(t,
					registeredPushKey{key: key, startsAt: pushReceivedAt.Add(-24 * time.Hour), retiresAt: pushReceivedAt})}
			}), push: fresh, want: "signature rejected",
		},
		"signed earlier than the window": {
			parts: with(func(*pushMinterParts) {}), push: pushOf(t, "msg_SYN01", pushReceivedAt.Add(-pushWindow-time.Second), key),
			want: "timestamp outside window",
		},
		"signed later than the clock skew allows": {
			parts: with(func(*pushMinterParts) {}), push: pushOf(t, "msg_SYN01", pushReceivedAt.Add(pushClockSkew+time.Second), key),
			want: "timestamp outside window",
		},
		"push source register cannot be read": {
			parts: with(func(parts *pushMinterParts) {
				parts.sources = pushSourceRegistryFake{err: errors.New("connection refused")}
			}),
			push: fresh, want: "source registry unavailable",
		},
		"signing key cannot be retrieved from the restricted store": {
			parts: with(func(parts *pushMinterParts) { parts.keys = pushKeyStoreFake{err: errors.New("vault sealed")} }),
			push:  fresh, want: "signing key unavailable",
		},
		"restricted store holds nothing under the registered reference": {
			parts: with(func(parts *pushMinterParts) { parts.keys = keyStoreOf() }), push: fresh, want: "signing key unavailable",
		},
		"restricted store not configured": {
			parts: with(func(parts *pushMinterParts) { parts.keys = accessidentity.UnconfiguredPushSigningKeyResolver{} }),
			push:  fresh, want: "not configured",
		},
		"replay ledger cannot be written": {
			parts: with(func(parts *pushMinterParts) { parts.ledger = &pushLedgerFake{err: errors.New("connection refused")} }),
			push:  fresh, want: "replay ledger unavailable",
		},
		"bound client holds no grant for the fact type": {
			parts: with(func(parts *pushMinterParts) {
				parts.clients = clientRegistryFake{standing: clientStandingWith(t, false, accessidentity.FactCustomsExternalResult), found: true}
			}), push: fresh, want: "not granted",
		},
		"bound client is not in the integration client register": {
			parts: with(func(parts *pushMinterParts) { parts.clients = clientRegistryFake{found: false} }), push: fresh, want: "not granted",
		},
		"integration client register cannot be read": {
			parts: with(func(parts *pushMinterParts) {
				parts.clients = clientRegistryFake{err: errors.New("connection refused")}
			}),
			push: fresh, want: "client registry unavailable",
		},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			envelope, err := mintPush(t, newPushMinter(t, testCase.parts), testCase.push)
			assertPushGrade(t, err, testCase.want)
			if envelope.Minted() {
				t.Fatal("a refused push still produced a minted envelope")
			}
		})
	}
}

// Covers: 轮换期两把同形态的钥并存时，旧钥的本体已经从受限存储撤了，新钥单签仍收下；两把都取不回才答依赖故障，
// 不把「还有一把没取到」折成签名不过。部署没给存储仍立刻答未配置，不往下试。
func TestAMissingOldKeyDoesNotBlockTheNewKeyOfTheSameForm(t *testing.T) {
	oldKey := newHMACPushKey(t, "SYN-PUSH-KEY-OLD")
	newKey := newHMACPushKey(t, "SYN-PUSH-KEY-NEW")
	standing := pushStandingWith(t, openKey(oldKey), openKey(newKey))
	push := pushOf(t, "msg_SYN01", pushReceivedAt, newKey)

	parts := grantedPushParts(t, standing, newKey)
	envelope, err := mintPush(t, newPushMinter(t, parts), push)
	assertPushMintedAsTheBoundClient(t, envelope, err)

	parts.keys = keyStoreOf()
	_, err = mintPush(t, newPushMinter(t, parts), push)
	assertPushGrade(t, err, "signing key unavailable")
}

// Covers: 同形态两把钥都在册时，前一把对不上这份签名，后一把仍验得过。轮换期新旧都是共享密钥时靠这一格，不靠「形态不同所以前一把被跳过」。
func TestALaterKeyOfTheSameFormStillVerifiesWhenTheEarlierOneDoesNotMatch(t *testing.T) {
	oldKey := newHMACPushKey(t, "SYN-PUSH-KEY-OLD")
	newKey := newHMACPushKey(t, "SYN-PUSH-KEY-NEW")
	parts := grantedPushParts(t, pushStandingWith(t, openKey(oldKey), openKey(newKey)), oldKey, newKey)
	envelope, err := mintPush(t, newPushMinter(t, parts), pushOf(t, "msg_SYN01", pushReceivedAt, newKey))
	assertPushMintedAsTheBoundClient(t, envelope, err)
}

// Covers: 准入范围照集成客户端铸造器走——请求带了准入要求时，权威串没配答不在准入范围，读不动答依赖故障且不折进
// 不在准入范围；没带准入要求的请求不判这一格（本文件其余用例）。本票不补准入范围桥。
func TestSignedPushAdmissionFollowsTheIntegrationClientMinter(t *testing.T) {
	key := newHMACPushKey(t, "SYN-PUSH-KEY-HMAC-01")
	parts := grantedPushParts(t, pushStandingWith(t, openKey(key)), key)
	requirement := accessidentity.AdmissionRequirement{
		Capability: accessidentity.ExternalFactAdmissionCapability,
		FactKind:   accessidentity.FactExternalFunds.String(),
	}
	request := accessidentity.IntegrationClientRequest{
		FactType:  accessidentity.FactExternalFunds,
		Admission: &requirement,
	}
	mint := func(t *testing.T, admission accessidentity.AdmissionScope) error {
		t.Helper()
		minter, err := accessidentity.NewSignedPushMinter(parts.sources, parts.keys, parts.clients, admission, parts.ledger,
			func() time.Time { return parts.at })
		if err != nil {
			t.Fatal(err)
		}
		_, err = minter.MintSignedPush(context.Background(), pushSourceOf(t), pushOf(t, "msg_SYN01", pushReceivedAt, key), request)
		return err
	}
	assertPushGrade(t, mint(t, accessidentity.UnconfiguredAdmissionScope{}), "outside admission")
	assertPushGrade(t, mint(t, pushAdmissionFake{err: errors.New("governance unread")}), "admission unavailable")

	envelope, err := mintPush(t, newPushMinter(t, parts), pushOf(t, "msg_SYN02", pushReceivedAt, key))
	assertPushMintedAsTheBoundClient(t, envelope, err)
}

type pushAdmissionFake struct{ err error }

func (fake pushAdmissionFake) Admits(context.Context, string, accessidentity.AdmissionRequirement, time.Time) (bool, error) {
	if fake.err != nil {
		return false, fake.err
	}
	return true, nil
}

// Covers: 窗口两端都含：签名时刻恰在窗口起点、恰在时钟偏差终点的推送照常收下。
func TestSignedPushAtTheEdgesOfTheWindowIsAccepted(t *testing.T) {
	key := newHMACPushKey(t, "SYN-PUSH-KEY-HMAC-01")
	for name, signedAt := range map[string]time.Time{
		"signed exactly a window ago":             pushReceivedAt.Add(-pushWindow),
		"signed exactly the clock skew ahead":     pushReceivedAt.Add(pushClockSkew),
		"signed at the instant it is received at": pushReceivedAt,
	} {
		t.Run(name, func(t *testing.T) {
			minter := newPushMinter(t, grantedPushParts(t, pushStandingWith(t, openKey(key)), key))
			envelope, err := mintPush(t, minter, pushOf(t, "msg_SYN01", signedAt, key))
			assertPushMintedAsTheBoundClient(t, envelope, err)
		})
	}
}

// Covers: 票面完成判据「同一推送重放被识别」——同一份签名原文第二次出示答重放；推送方按 Standard Webhooks 重试（同一
// 推送标识、新的尝试时间戳、重签）不是重放，交给各命令口的载荷摘要去判同身份重放（ADR-0149 决定四第一条）；第一次被拒
// 的推送不入账，授予补上之后原样再发照常收下。
func TestTheSamePushReplayedIsRecognisedButARetrySignedAfreshIsNot(t *testing.T) {
	key := newHMACPushKey(t, "SYN-PUSH-KEY-HMAC-01")
	parts := grantedPushParts(t, pushStandingWith(t, openKey(key)), key)
	minter := newPushMinter(t, parts)
	push := pushOf(t, "msg_SYN01", pushReceivedAt.Add(-time.Minute), key)

	envelope, err := mintPush(t, minter, push)
	assertPushMintedAsTheBoundClient(t, envelope, err)
	envelope, err = mintPush(t, minter, push)
	assertPushGrade(t, err, "replayed")
	if envelope.Minted() {
		t.Fatal("a replayed push still produced a minted envelope")
	}

	retry := pushOf(t, "msg_SYN01", pushReceivedAt, key)
	envelope, err = mintPush(t, minter, retry)
	assertPushMintedAsTheBoundClient(t, envelope, err)

	refusedFirst := pushOf(t, "msg_SYN02", pushReceivedAt, key)
	ungranted := parts
	ungranted.clients = clientRegistryFake{standing: clientStandingWith(t, false, accessidentity.FactCustomsExternalResult), found: true}
	_, err = mintPush(t, newPushMinter(t, ungranted), refusedFirst)
	assertPushGrade(t, err, "not granted")
	envelope, err = mintPush(t, minter, refusedFirst)
	assertPushMintedAsTheBoundClient(t, envelope, err)
}

// Covers: 票面完成判据「密钥轮换期新旧并存可配置」——旧钥登一笔停用、新钥登自己的起点，两者生效时段重叠多久就并存多久，
// 长短全在登记上；重叠期内任一把单签或两把并签都收，新钥起点前只认旧钥，旧钥停用后只认新钥。新旧两把不同形态，顺带证
// 一个推送源可以从共享密钥轮换到对方公钥。
func TestKeyRotationAcceptsOldAndNewKeysExactlyWhileBothAreEffective(t *testing.T) {
	oldKey := newHMACPushKey(t, "SYN-PUSH-KEY-OLD")
	newKey := newEd25519PushKey(t, "SYN-PUSH-KEY-NEW")
	newKeyStartsAt := pushReceivedAt.Add(-time.Hour)
	oldKeyRetiresAt := pushReceivedAt.Add(time.Hour)
	standing := pushStandingWith(t,
		registeredPushKey{key: oldKey, startsAt: pushReceivedAt.Add(-20 * time.Hour), retiresAt: oldKeyRetiresAt},
		registeredPushKey{key: newKey, startsAt: newKeyStartsAt})
	cases := map[string]struct {
		at      time.Time
		signers []pushKey
		want    string
	}{
		"overlap, old key alone":                          {at: pushReceivedAt, signers: []pushKey{oldKey}},
		"overlap, new key alone":                          {at: pushReceivedAt, signers: []pushKey{newKey}},
		"overlap, both keys in one header":                {at: pushReceivedAt, signers: []pushKey{oldKey, newKey}},
		"before the new key starts, old key alone":        {at: newKeyStartsAt.Add(-time.Minute), signers: []pushKey{oldKey}},
		"before the new key starts, new key alone":        {at: newKeyStartsAt.Add(-time.Minute), signers: []pushKey{newKey}, want: "signature rejected"},
		"once the old key is retired, old key alone":      {at: oldKeyRetiresAt, signers: []pushKey{oldKey}, want: "signature rejected"},
		"once the old key is retired, both keys together": {at: oldKeyRetiresAt, signers: []pushKey{oldKey, newKey}},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			parts := grantedPushParts(t, standing, oldKey, newKey)
			parts.at = testCase.at
			envelope, err := mintPush(t, newPushMinter(t, parts), pushOf(t, "msg_SYN01", testCase.at, testCase.signers...))
			if testCase.want == "" {
				assertPushMintedAsTheBoundClient(t, envelope, err)
				return
			}
			assertPushGrade(t, err, testCase.want)
		})
	}
}

// Covers: 签名头不成形是签名不过的一种，在认推送源之前就答——缺推送标识、时间戳不是十进制整数、签名头为空、只有本校验器
// 不认的签名标识、签名不是 base64。认不出的项跳过而不是整头拒：轮换或升级期间头里可以并列别的版本。
func TestMalformedStandardWebhookHeadersAreASignatureRejection(t *testing.T) {
	key := newHMACPushKey(t, "SYN-PUSH-KEY-HMAC-01")
	timestamp := strconv.FormatInt(pushReceivedAt.Unix(), 10)
	signature := key.sign("msg_SYN01", pushReceivedAt.Unix(), pushBody)
	for name, headers := range map[string][3]string{
		"webhook-id missing":                 {"", timestamp, signature},
		"timestamp in RFC 3339":              {"msg_SYN01", pushReceivedAt.Format(time.RFC3339), signature},
		"timestamp signed":                   {"msg_SYN01", "+" + timestamp, signature},
		"signature header empty":             {"msg_SYN01", timestamp, ""},
		"only an unknown signature version":  {"msg_SYN01", timestamp, "v2," + base64.StdEncoding.EncodeToString([]byte("SYN"))},
		"signature not base64":               {"msg_SYN01", timestamp, "v1,@@@"},
		"signature without a version comma":  {"msg_SYN01", timestamp, strings.TrimPrefix(signature, "v1,")},
		"timestamp beyond the integer range": {"msg_SYN01", timestamp + "000000000000", signature},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := accessidentity.ParseStandardWebhook(headers[0], headers[1], headers[2], []byte(pushBody))
			assertPushGrade(t, err, "signature rejected")
		})
	}
	mixed, err := accessidentity.ParseStandardWebhook("msg_SYN01", timestamp, "v2,SYN "+signature, []byte(pushBody))
	if err != nil {
		t.Fatalf("a header with one unknown and one known signature: %v", err)
	}
	minter := newPushMinter(t, grantedPushParts(t, pushStandingWith(t, openKey(key)), key))
	envelope, err := mintPush(t, minter, mixed)
	assertPushMintedAsTheBoundClient(t, envelope, err)
}

// Covers: 窗口与时钟偏差是租户对这个推送源的取值，产品不给缺省——窗口缺、为负、或不是整秒（时间戳本身只到秒），时钟偏差
// 为负，登记依据为空，一律建不成；时钟偏差为零是租户可以作的选择。
func TestPushSourceBindingHasNoDefaultWindowOrClockSkew(t *testing.T) {
	source := pushSourceOf(t)
	client := clientSubjectOf(t, "SYN-CLIENT-01")
	for name, spec := range map[string]struct {
		window, skew time.Duration
		tenant       string
		basis        string
	}{
		"window unset":                  {window: 0, skew: pushClockSkew, tenant: clientTenant, basis: "SYN-BASIS"},
		"window negative":               {window: -time.Minute, skew: pushClockSkew, tenant: clientTenant, basis: "SYN-BASIS"},
		"window not whole seconds":      {window: pushWindow + time.Millisecond, skew: pushClockSkew, tenant: clientTenant, basis: "SYN-BASIS"},
		"clock skew negative":           {window: pushWindow, skew: -time.Second, tenant: clientTenant, basis: "SYN-BASIS"},
		"clock skew not whole seconds":  {window: pushWindow, skew: 1500 * time.Millisecond, tenant: clientTenant, basis: "SYN-BASIS"},
		"tenant blank":                  {window: pushWindow, skew: pushClockSkew, tenant: " ", basis: "SYN-BASIS"},
		"registration basis left blank": {window: pushWindow, skew: pushClockSkew, tenant: clientTenant, basis: " "},
	} {
		if _, err := accessidentity.NewPushSourceBinding(source, client, spec.tenant, spec.window, spec.skew, spec.basis); !errors.Is(err, accessidentity.ErrIncompletePushSourceRegistration) {
			t.Fatalf("%s: err = %v, want ErrIncompletePushSourceRegistration", name, err)
		}
	}
	if _, err := accessidentity.NewPushSourceBinding(source, client, clientTenant, pushWindow, 0, "SYN-BASIS"); err != nil {
		t.Fatalf("zero clock skew: %v", err)
	}
}

// Covers: 推送源现状里混进别的推送源或别的租户名下的钥建不成；停用只认它停的那一把。
func TestPushSourceStandingRefusesKeysOfAnotherSource(t *testing.T) {
	source := pushSourceOf(t)
	other, err := accessidentity.NewPushSourceID("SYN-PUSH-SOURCE/carrier-01")
	if err != nil {
		t.Fatal(err)
	}
	binding, err := accessidentity.NewPushSourceBinding(source, clientSubjectOf(t, "SYN-CLIENT-01"), clientTenant, pushWindow, pushClockSkew, "SYN-BASIS")
	if err != nil {
		t.Fatal(err)
	}
	interval := clientIntervalOf(t, pushReceivedAt.Add(-time.Hour), time.Time{})
	keyOf := func(tenant string, owner accessidentity.PushSourceID, id string) accessidentity.RecordedPushSigningKey {
		t.Helper()
		key, err := accessidentity.NewPushSigningKey(tenant, owner, id, accessidentity.PushSignatureHMACSHA256,
			credentialReferenceOf(t, "SYN-PUSH-KEY-REF/"+id), interval, "SYN-BASIS")
		if err != nil {
			t.Fatal(err)
		}
		recorded, err := accessidentity.NewRecordedPushSigningKey(key, nil)
		if err != nil {
			t.Fatal(err)
		}
		return recorded
	}
	for name, key := range map[string]accessidentity.RecordedPushSigningKey{
		"key of another push source": keyOf(clientTenant, other, "SYN-K1"),
		"key under another tenant":   keyOf("SYN-TENANT-02", source, "SYN-K1"),
	} {
		if _, err := accessidentity.NewPushSourceStanding(binding, []accessidentity.RecordedPushSigningKey{key}); !errors.Is(err, accessidentity.ErrPushSigningKeyOutsideSource) {
			t.Fatalf("%s: err = %v, want ErrPushSigningKeyOutsideSource", name, err)
		}
	}
	key, err := accessidentity.NewPushSigningKey(clientTenant, source, "SYN-K1", accessidentity.PushSignatureHMACSHA256,
		credentialReferenceOf(t, "SYN-PUSH-KEY-REF/SYN-K1"), interval, "SYN-BASIS")
	if err != nil {
		t.Fatal(err)
	}
	retirement, err := accessidentity.NewPushSigningKeyRetirement(clientTenant, source, "SYN-K2", pushReceivedAt, "SYN-BASIS")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := accessidentity.NewRecordedPushSigningKey(key, &retirement); !errors.Is(err, accessidentity.ErrRetirementDoesNotMatchKey) {
		t.Fatalf("retirement of another key: err = %v, want ErrRetirementDoesNotMatchKey", err)
	}
}

// Covers: 请求没说清要交哪一类事实是装配缺陷，不是任何一格答复，理由同 TestRequestWithoutAKnownFactTypeIsAnAssemblyDefect。
func TestSignedPushRequestWithoutAKnownFactTypeIsAnAssemblyDefect(t *testing.T) {
	key := newHMACPushKey(t, "SYN-PUSH-KEY-HMAC-01")
	minter := newPushMinter(t, grantedPushParts(t, pushStandingWith(t, openKey(key)), key))
	_, err := minter.MintSignedPush(context.Background(), pushSourceOf(t), pushOf(t, "msg_SYN01", pushReceivedAt, key),
		accessidentity.IntegrationClientRequest{FactType: "ANYTHING"})
	if !errors.Is(err, accessidentity.ErrExternalFactTypeUnknown) {
		t.Fatalf("err = %v, want ErrExternalFactTypeUnknown", err)
	}
	for name, sentinel := range pushGrades {
		if errors.Is(err, sentinel) {
			t.Fatalf("the defect reads as the %s grade", name)
		}
	}
}

// Covers: 零值推送（没经 ParseStandardWebhook 建出的）不带任何签名，答签名不过而不是放行。
func TestZeroSignedPushIsASignatureRejection(t *testing.T) {
	key := newHMACPushKey(t, "SYN-PUSH-KEY-HMAC-01")
	minter := newPushMinter(t, grantedPushParts(t, pushStandingWith(t, openKey(key)), key))
	_, err := mintPush(t, minter, accessidentity.SignedPush{})
	assertPushGrade(t, err, "signature rejected")
}

func TestSignedPushMinterNeedsAllItsDependencies(t *testing.T) {
	sources := pushSourceRegistryFake{}
	keys := keyStoreOf()
	clients := clientRegistryFake{}
	admission := accessidentity.UnconfiguredAdmissionScope{}
	ledger := newPushLedger()
	now := func() time.Time { return pushReceivedAt }
	for name, build := range map[string]func() (*accessidentity.SignedPushMinter, error){
		"push source registry": func() (*accessidentity.SignedPushMinter, error) {
			return accessidentity.NewSignedPushMinter(nil, keys, clients, admission, ledger, now)
		},
		"key resolver": func() (*accessidentity.SignedPushMinter, error) {
			return accessidentity.NewSignedPushMinter(sources, nil, clients, admission, ledger, now)
		},
		"client registry": func() (*accessidentity.SignedPushMinter, error) {
			return accessidentity.NewSignedPushMinter(sources, keys, nil, admission, ledger, now)
		},
		"admission": func() (*accessidentity.SignedPushMinter, error) {
			return accessidentity.NewSignedPushMinter(sources, keys, clients, nil, ledger, now)
		},
		"replay ledger": func() (*accessidentity.SignedPushMinter, error) {
			return accessidentity.NewSignedPushMinter(sources, keys, clients, admission, nil, now)
		},
		"clock": func() (*accessidentity.SignedPushMinter, error) {
			return accessidentity.NewSignedPushMinter(sources, keys, clients, admission, ledger, nil)
		},
	} {
		if _, err := build(); !errors.Is(err, accessidentity.ErrNilDependency) {
			t.Fatalf("minter without %s: err = %v, want ErrNilDependency", name, err)
		}
	}
}

// Covers: 票面「密钥只存引用」与 ADR-0149 越权风险点 4——从受限存储取回的密钥本体经 fmt 各动词、slog 两种处理器、JSON
// 与错误信息都印不出；构造时按值拷进来，调用方事后改它手里那份改不到核验用的这一份。
func TestPushSigningKeyMaterialNeverRenders(t *testing.T) {
	secret := []byte("SYN-not-a-production-secret-0123456789")
	material := accessidentity.NewPushSigningKeyMaterial(secret)

	var jsonLog, textLog bytes.Buffer
	slog.New(slog.NewJSONHandler(&jsonLog, nil)).Info("resolved", "key", material)
	slog.New(slog.NewTextHandler(&textLog, nil)).Info("resolved", "key", material)
	marshalled, err := json.Marshal(material)
	if err != nil {
		t.Fatal(err)
	}
	for name, rendered := range map[string]string{
		"fmt verbs": fmt.Sprintf("%v|%+v|%#v|%s|%q|%x", material, material, material, material, material, material),
		"slog JSON": jsonLog.String(),
		"slog text": textLog.String(),
		"JSON":      string(marshalled),
		"error":     fmt.Errorf("resolve %v: %w", material, errors.New("boom")).Error(),
	} {
		if strings.Contains(rendered, "SYN-not") || strings.Contains(rendered, "53594e2d6e6f74") {
			t.Errorf("%s renders the key material: %s", name, rendered)
		}
	}

	key := newHMACPushKey(t, "SYN-PUSH-KEY-HMAC-01")
	handed := append([]byte(nil), key.secret...)
	resolver := staticResolver{material: accessidentity.NewPushSigningKeyMaterial(handed)}
	handed[0] ^= 0xff
	parts := grantedPushParts(t, pushStandingWith(t, openKey(key)), key)
	parts.keys = resolver
	envelope, err := mintPush(t, newPushMinter(t, parts), pushOf(t, "msg_SYN01", pushReceivedAt, key))
	assertPushMintedAsTheBoundClient(t, envelope, err)
}

type staticResolver struct {
	material accessidentity.PushSigningKeyMaterial
}

func (resolver staticResolver) ResolvePushSigningKey(context.Context, accessidentity.CredentialReference) (accessidentity.PushSigningKeyMaterial, error) {
	return resolver.material, nil
}
