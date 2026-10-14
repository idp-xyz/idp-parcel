package accessidentity

import (
	"context"
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"
)

// ErrPushSignatureRejected 表示这份推送的签名不成立：推送源未登记、此刻没有生效的密钥、没有一项签名能被生效的密钥验过，
// 或签名头根本不成形。几种情形同答这一格、不说哪一半不对，理由同 ErrCredentialRejected：出示方还没证明自己持有推送源的
// 密钥，细分会把「这个推送源在册」告诉它。恢复动作在推送方——用登记过的密钥照约定的原文重签。
//
// 它不折进 ErrCredentialRejected：那一格是客户端凭据令牌不过，恢复动作是去换令牌；推送没有令牌可换。
var ErrPushSignatureRejected = errors.New("access identity: signed push signature rejected")

// ErrPushTimestampOutsideWindow 表示签名成立，但签名时刻早于接收时刻减窗口、或晚于接收时刻加时钟偏差。它判在签名之后：
// 没验过签名就答越窗，等于告诉一个没证明身份的调用方这个推送源在册。恢复动作是推送方校钟、以当前时刻重签再发，与换
// 密钥不是一件事，所以不折进签名不过。
var ErrPushTimestampOutsideWindow = errors.New("access identity: signed push timestamp is outside the acceptance window")

// ErrPushReplayed 表示同一推送源下同一份签名原文此前已被收下过。窗口之内逐字节再发一遍的就是它——截获者重放、或中间
// 环节重复投递。推送方以新的尝试时间戳重签的重试不是它：原文变了，交给各命令口按载荷摘要去判同身份重放（ADR-0149
// 决定四第一条），本格不替它们再立一份幂等口径。
var ErrPushReplayed = errors.New("access identity: signed push was already received")

// ErrPushSourceRegistryUnavailable 表示推送源册读不动。它不折进签名不过，理由同 ErrIntegrationClientRegistryUnavailable：
// 折过去会让推送方去换一把其实没毛病的密钥。
var ErrPushSourceRegistryUnavailable = errors.New("access identity: push source registry is unavailable")

// ErrPushSigningKeyUnavailable 表示在册的签名密钥从受限存储取不回，或取回的东西不是那一形密钥。恢复动作是运维去救存储
// 或补那一份，不是推送方重签。
var ErrPushSigningKeyUnavailable = errors.New("access identity: push signing key cannot be retrieved")

// ErrPushReplayLedgerUnavailable 表示重放账读写不动。它不折进重放：那会让推送方以为这份推送已被收下，而它其实一行都
// 没进账。
var ErrPushReplayLedgerUnavailable = errors.New("access identity: push replay ledger cannot be read or written")

// ErrInvalidPushSourceID 表示推送源标识是空的。
var ErrInvalidPushSourceID = errors.New("access identity: push source id is empty")

// ErrIncompletePushSourceRegistration 表示推送源册的一行缺件，建不成。
var ErrIncompletePushSourceRegistration = errors.New("access identity: push source registration is incomplete")

// ErrPushSignatureFormUnknown 表示给出的签名形态不在 PushSignatureForm 那两形里。
var ErrPushSignatureFormUnknown = errors.New("access identity: push signature form is unknown")

// ErrPushSigningKeyOutsideSource 表示推送源现状里混进了别的推送源或别的租户名下的密钥。
var ErrPushSigningKeyOutsideSource = errors.New("access identity: signing key does not belong to the push source")

// ErrRetirementDoesNotMatchKey 表示一笔停用指的不是它要停的那一把密钥。
var ErrRetirementDoesNotMatchKey = errors.New("access identity: retirement does not match the signing key")

// PushSourceID 是一次推送所认领的推送源登记行的键，通常由入向连接器从回调地址上取。它自己不是身份，理由同 ChannelKey：
// 调用方能自由指定它；租户与客户端一律取自认领到的那一行。键不含租户——推送不说自己属哪个租户，同一个推送源标识至多
// 一行。
type PushSourceID struct{ value string }

func NewPushSourceID(value string) (PushSourceID, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return PushSourceID{}, ErrInvalidPushSourceID
	}
	return PushSourceID{value: trimmed}, nil
}

func (source PushSourceID) String() string { return source.value }

// PushSignatureForm 是一把推送签名密钥的形态，也是签名头里一项签名的形态。两形照 Standard Webhooks：HMAC-SHA256 共享
// 密钥（签名标识 v1）与 Ed25519 对方公钥（v1a）。这是产品定的封闭集；某个推送源用哪一形、哪几把是租户取值。
type PushSignatureForm string

const (
	PushSignatureHMACSHA256 PushSignatureForm = "HMAC_SHA256"
	PushSignatureEd25519    PushSignatureForm = "ED25519"
)

// ParsePushSignatureForm 按字面取值认形态，大小写不折叠：册里存的就是这些字面量。
func ParsePushSignatureForm(value string) (PushSignatureForm, error) {
	switch form := PushSignatureForm(value); form {
	case PushSignatureHMACSHA256, PushSignatureEd25519:
		return form, nil
	}
	return "", ErrPushSignatureFormUnknown
}

func (form PushSignatureForm) String() string { return string(form) }

// PushSourceBinding 是推送源册的一行：一个只能推送的外部源绑在集成客户端册的一个客户端主体上，校验通过后以那个客户端
// 的身份铸信封（ADR-0149 决定三第三条）。绑定登在册上而不是部署参数里：推送源随租户与外部方增减，一个部署里有若干个，
// 是行不是一件全局参数。租户随行登，须是那个客户端所绑的租户（库里经外键回指集成客户端册）。
//
// 窗口与时钟偏差是租户对这个推送源的取值，产品不给缺省：窗口是签名时刻最多可早于收到时刻多久，时钟偏差是最多可晚于
// 收到时刻多久。两件随外部方的发送节奏与钟的质量而定，不看具体是哪个外部方答不出「是多少」。两件都按整秒登：
// Standard Webhooks 的时间戳只到秒，再细的窗口判不出来。
type PushSourceBinding struct {
	source    PushSourceID
	client    IntegrationClientSubject
	tenantID  string
	window    time.Duration
	clockSkew time.Duration
	basis     string
}

func NewPushSourceBinding(
	source PushSourceID,
	client IntegrationClientSubject,
	tenantID string,
	window time.Duration,
	clockSkew time.Duration,
	basis string,
) (PushSourceBinding, error) {
	tenant := strings.TrimSpace(tenantID)
	reference := strings.TrimSpace(basis)
	if source.value == "" || client.issuer == "" || tenant == "" || reference == "" ||
		window <= 0 || window%time.Second != 0 || clockSkew < 0 || clockSkew%time.Second != 0 {
		return PushSourceBinding{}, ErrIncompletePushSourceRegistration
	}
	return PushSourceBinding{
		source:    source,
		client:    client,
		tenantID:  tenant,
		window:    window,
		clockSkew: clockSkew,
		basis:     reference,
	}, nil
}

func (binding PushSourceBinding) Source() PushSourceID             { return binding.source }
func (binding PushSourceBinding) Client() IntegrationClientSubject { return binding.client }
func (binding PushSourceBinding) TenantID() string                 { return binding.tenantID }
func (binding PushSourceBinding) Window() time.Duration            { return binding.window }
func (binding PushSourceBinding) ClockSkew() time.Duration         { return binding.clockSkew }
func (binding PushSourceBinding) Basis() string                    { return binding.basis }

// PushSigningKey 是推送源名下登记的一把签名密钥：形态、受控引用与生效区间。密钥本体不入库——共享密钥与对方公钥都只存
// 受控引用，核验那一刻经 PushSigningKeyResolver 去受限存储取（ADR-0149 越权风险点 4）。公钥本身不是秘密，仍只存引用，
// 是为了两形走同一条取用与轮换路径。
//
// 轮换是登记新钥、再给旧钥登一笔停用：两者的生效时段重叠多久就并存多久，长短是租户取值。键是（推送源、密钥标识），
// 停用了再启用是另一把。
type PushSigningKey struct {
	tenantID  string
	source    PushSourceID
	keyID     string
	form      PushSignatureForm
	reference CredentialReference
	interval  EffectiveInterval
	basis     string
}

func NewPushSigningKey(
	tenantID string,
	source PushSourceID,
	keyID string,
	form PushSignatureForm,
	reference CredentialReference,
	interval EffectiveInterval,
	basis string,
) (PushSigningKey, error) {
	// PushSignatureForm 是字符串底型，包外转型递进来的值也要在这里被拦下。
	if _, err := ParsePushSignatureForm(string(form)); err != nil {
		return PushSigningKey{}, err
	}
	tenant := strings.TrimSpace(tenantID)
	id := strings.TrimSpace(keyID)
	recordedBasis := strings.TrimSpace(basis)
	if tenant == "" || source.value == "" || id == "" || reference.value == "" || interval.startsAt.IsZero() || recordedBasis == "" {
		return PushSigningKey{}, ErrIncompletePushSourceRegistration
	}
	return PushSigningKey{
		tenantID:  tenant,
		source:    source,
		keyID:     id,
		form:      form,
		reference: reference,
		interval:  interval,
		basis:     recordedBasis,
	}, nil
}

func (key PushSigningKey) TenantID() string               { return key.tenantID }
func (key PushSigningKey) Source() PushSourceID           { return key.source }
func (key PushSigningKey) KeyID() string                  { return key.keyID }
func (key PushSigningKey) Form() PushSignatureForm        { return key.form }
func (key PushSigningKey) Reference() CredentialReference { return key.reference }
func (key PushSigningKey) Interval() EffectiveInterval    { return key.interval }
func (key PushSigningKey) Basis() string                  { return key.basis }

// PushSigningKeyRetirement 是一把签名密钥的停用：自 retiresAt 起它不再验得过任何推送。停用是终局，与 GrantRevocation
// 同理——被停的那把钥在停用之前验过的推送，是那段时间里它确实有效的证据，所以停用另成一行而不改密钥那一行。
type PushSigningKeyRetirement struct {
	tenantID  string
	source    PushSourceID
	keyID     string
	retiresAt time.Time
	basis     string
}

func NewPushSigningKeyRetirement(tenantID string, source PushSourceID, keyID string, retiresAt time.Time, basis string) (PushSigningKeyRetirement, error) {
	tenant := strings.TrimSpace(tenantID)
	id := strings.TrimSpace(keyID)
	reference := strings.TrimSpace(basis)
	if tenant == "" || source.value == "" || id == "" || retiresAt.IsZero() || reference == "" {
		return PushSigningKeyRetirement{}, ErrIncompletePushSourceRegistration
	}
	return PushSigningKeyRetirement{tenantID: tenant, source: source, keyID: id, retiresAt: retiresAt, basis: reference}, nil
}

func (retirement PushSigningKeyRetirement) TenantID() string     { return retirement.tenantID }
func (retirement PushSigningKeyRetirement) Source() PushSourceID { return retirement.source }
func (retirement PushSigningKeyRetirement) KeyID() string        { return retirement.keyID }
func (retirement PushSigningKeyRetirement) RetiresAt() time.Time { return retirement.retiresAt }
func (retirement PushSigningKeyRetirement) Basis() string        { return retirement.basis }

// RecordedPushSigningKey 是册上的一把签名密钥，连同它的停用（若有）。
type RecordedPushSigningKey struct {
	key        PushSigningKey
	retirement PushSigningKeyRetirement
	retired    bool
}

// NewRecordedPushSigningKey 的 retirement 取 nil 表示未停用。
func NewRecordedPushSigningKey(key PushSigningKey, retirement *PushSigningKeyRetirement) (RecordedPushSigningKey, error) {
	if retirement == nil {
		return RecordedPushSigningKey{key: key}, nil
	}
	if retirement.tenantID != key.tenantID || retirement.source != key.source || retirement.keyID != key.keyID {
		return RecordedPushSigningKey{}, ErrRetirementDoesNotMatchKey
	}
	return RecordedPushSigningKey{key: key, retirement: *retirement, retired: true}, nil
}

func (recorded RecordedPushSigningKey) Key() PushSigningKey { return recorded.key }

func (recorded RecordedPushSigningKey) Retirement() (PushSigningKeyRetirement, bool) {
	return recorded.retirement, recorded.retired
}

// EffectiveAt 答这把钥在 at 那一刻验不验得了推送：在区间内，且还没到停用时刻。判据是收到的那一刻，不是推送自报的签名
// 时刻：拿已停用的钥在窗口内倒填一个停用前的时间戳，照样验不过。
func (recorded RecordedPushSigningKey) EffectiveAt(at time.Time) bool {
	if !recorded.key.interval.Contains(at) {
		return false
	}
	return !recorded.retired || at.Before(recorded.retirement.retiresAt)
}

// PushSourceStanding 是一个推送源在册上的全部：它的绑定，与它名下每一把签名密钥（连同停用）。它只答「册上怎么写」，
// 不答「这份推送成不成立」——后者是铸造那一步的事。
type PushSourceStanding struct {
	binding PushSourceBinding
	keys    []RecordedPushSigningKey
}

func NewPushSourceStanding(binding PushSourceBinding, keys []RecordedPushSigningKey) (PushSourceStanding, error) {
	copied := make([]RecordedPushSigningKey, 0, len(keys))
	for _, recorded := range keys {
		if recorded.key.source != binding.source || recorded.key.tenantID != binding.tenantID {
			return PushSourceStanding{}, ErrPushSigningKeyOutsideSource
		}
		copied = append(copied, recorded)
	}
	return PushSourceStanding{binding: binding, keys: copied}, nil
}

func (standing PushSourceStanding) Binding() PushSourceBinding { return standing.binding }

func (standing PushSourceStanding) Keys() []RecordedPushSigningKey {
	return append([]RecordedPushSigningKey(nil), standing.keys...)
}

// PushSourceRegistry 是推送源册的装载口：按推送源标识取它在册上的全部。不在册时答 found=false 而不返回 error，理由同
// IntegrationClientRegistry。
type PushSourceRegistry interface {
	FindPushSource(ctx context.Context, source PushSourceID) (PushSourceStanding, bool, error)
}

// PushSourceRegistrationOutcome 是推送源册登记口的答复。撞键从不覆盖：同内容重放答原结果，异内容答冲突，册上已有的那
// 一行原样不动。它不复用 IntegrationClientRegistrationOutcome，理由同那个类型不复用操作者的：两本册的「未登记」指的不是
// 同一种东西。
type PushSourceRegistrationOutcome string

const (
	PushSourceRegistrationRecorded          PushSourceRegistrationOutcome = "RECORDED"
	PushSourceRegistrationAlreadyRegistered PushSourceRegistrationOutcome = "ALREADY_REGISTERED"
	PushSourceRegistrationContentConflict   PushSourceRegistrationOutcome = "CONTENT_CONFLICT"
	// PushSourceBoundToAnotherTenant 只由 RegisterPushSource 答：这个推送源标识已登在别的租户名下。不说是哪个租户，理由同
	// IntegrationClientBoundToAnotherTenant。
	PushSourceBoundToAnotherTenant PushSourceRegistrationOutcome = "PUSH_SOURCE_BOUND_TO_ANOTHER_TENANT"
	// PushSourceClientNotRegistered 只由 RegisterPushSource 答：本租户的集成客户端册上没有要绑的那个客户端主体。
	PushSourceClientNotRegistered PushSourceRegistrationOutcome = "CLIENT_NOT_REGISTERED"
	// PushSourceNotRegistered 只由 RegisterPushSigningKey 答：本租户册上没有这个推送源。
	PushSourceNotRegistered PushSourceRegistrationOutcome = "PUSH_SOURCE_NOT_REGISTERED"
	// PushSigningKeyNotRegistered 只由 RegisterPushSigningKeyRetirement 答：本租户册上没有这把钥。
	PushSigningKeyNotRegistered PushSourceRegistrationOutcome = "SIGNING_KEY_NOT_REGISTERED"
)

func (outcome PushSourceRegistrationOutcome) String() string { return string(outcome) }

// PushSourceRegistrar 是推送源册的登记口。写入要落在调用方开的框架事务里，理由同 IntegrationClientRegistrar。
type PushSourceRegistrar interface {
	RegisterPushSource(ctx context.Context, binding PushSourceBinding) (PushSourceRegistrationOutcome, error)
	RegisterPushSigningKey(ctx context.Context, key PushSigningKey) (PushSourceRegistrationOutcome, error)
	RegisterPushSigningKeyRetirement(ctx context.Context, retirement PushSigningKeyRetirement) (PushSourceRegistrationOutcome, error)
}

// SignedPush 是一份经入向连接器拆出的签名推送：推送标识、签名时刻、原始请求体与签名头里认得出的各项签名。签名原文按
// Standard Webhooks 拼成「推送标识.时间戳.请求体」——推送标识与时间戳都在签名里，改了任何一件签名就不成立，窗口与重放
// 的判据才有意义。它只能经 ParseStandardWebhook 建出：原文怎么拼由产品定，不由各连接器各拼一份。零值不带任何签名，
// 铸造器答签名不过。
type SignedPush struct {
	id         string
	timestamp  string
	signedAt   time.Time
	body       []byte
	signatures []pushSignature
}

type pushSignature struct {
	form  PushSignatureForm
	value []byte
}

// standardWebhookSignatureForms 是 Standard Webhooks 签名头里的签名标识到形态的对照。
var standardWebhookSignatureForms = map[string]PushSignatureForm{
	"v1":  PushSignatureHMACSHA256,
	"v1a": PushSignatureEd25519,
}

// ParseStandardWebhook 按 Standard Webhooks 的三个头拆一份推送：webhook-id、webhook-timestamp（Unix 秒，十进制整数）与
// webhook-signature（空格分隔的若干项，每项「签名标识,base64 签名」）。认不出的签名标识与解不开的项跳过——轮换或升级
// 期间头里可以并列本校验器不认的项；一项都不剩、缺推送标识、时间戳不是十进制整数，答 ErrPushSignatureRejected，在认
// 推送源之前就答。请求体按字节拷进来：原文就是收到的那串字节，不重新序列化。
func ParseStandardWebhook(id, timestamp, signatures string, body []byte) (SignedPush, error) {
	if id == "" {
		return SignedPush{}, fmt.Errorf("%w: webhook-id is missing", ErrPushSignatureRejected)
	}
	if timestamp == "" || strings.Trim(timestamp, "0123456789") != "" {
		return SignedPush{}, fmt.Errorf("%w: webhook-timestamp is not a decimal integer", ErrPushSignatureRejected)
	}
	seconds, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return SignedPush{}, fmt.Errorf("%w: webhook-timestamp is out of range", ErrPushSignatureRejected)
	}
	var parsed []pushSignature
	for _, item := range strings.Fields(signatures) {
		version, encoded, found := strings.Cut(item, ",")
		form, known := standardWebhookSignatureForms[version]
		if !found || !known {
			continue
		}
		value, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil || len(value) == 0 {
			continue
		}
		parsed = append(parsed, pushSignature{form: form, value: value})
	}
	if len(parsed) == 0 {
		return SignedPush{}, fmt.Errorf("%w: webhook-signature carries no signature this verifier recognises", ErrPushSignatureRejected)
	}
	return SignedPush{
		id:         id,
		timestamp:  timestamp,
		signedAt:   time.Unix(seconds, 0).UTC(),
		body:       append([]byte(nil), body...),
		signatures: parsed,
	}, nil
}

// ID 是推送方给这份推送的标识。它不是重放键，见 PushReceipt。
func (push SignedPush) ID() string { return push.id }

// SignedAt 是签名头里的时间戳，即推送方签这一次尝试的时刻。
func (push SignedPush) SignedAt() time.Time { return push.signedAt }

// Body 交出原始请求体的副本。
func (push SignedPush) Body() []byte { return append([]byte(nil), push.body...) }

func (push SignedPush) signedContent() []byte {
	content := make([]byte, 0, len(push.id)+len(push.timestamp)+len(push.body)+2)
	content = append(content, push.id...)
	content = append(content, '.')
	content = append(content, push.timestamp...)
	content = append(content, '.')
	return append(content, push.body...)
}

func (push SignedPush) carries(form PushSignatureForm) bool {
	for _, signature := range push.signatures {
		if signature.form == form {
			return true
		}
	}
	return false
}

// verifiedBy 答签名头里有没有一项 form 形态的签名能被 material 验过。取回的东西不成那一形密钥是存储那一侧的毛病，交回
// 错误而不答「没验过」：答没验过会让推送方去换一把其实没毛病的密钥。
func (push SignedPush) verifiedBy(form PushSignatureForm, material PushSigningKeyMaterial) (bool, error) {
	content := push.signedContent()
	switch form {
	case PushSignatureHMACSHA256:
		if len(material.bytes) == 0 {
			return false, errors.New("resolved HMAC secret is empty")
		}
		mac := hmac.New(sha256.New, material.bytes)
		mac.Write(content)
		expected := mac.Sum(nil)
		for _, signature := range push.signatures {
			if signature.form == form && hmac.Equal(signature.value, expected) {
				return true, nil
			}
		}
		return false, nil
	case PushSignatureEd25519:
		if len(material.bytes) != ed25519.PublicKeySize {
			return false, fmt.Errorf("resolved key is not a %d-byte Ed25519 public key", ed25519.PublicKeySize)
		}
		public := ed25519.PublicKey(material.bytes)
		for _, signature := range push.signatures {
			if signature.form == form && ed25519.Verify(public, content, signature.value) {
				return true, nil
			}
		}
		return false, nil
	}
	return false, ErrPushSignatureFormUnknown
}

// redactedPushSigningKey 是推送签名密钥本体在一切渲染路径上的替身。
const redactedPushSigningKey = "[REDACTED PUSH SIGNING KEY]"

// PushSigningKeyMaterial 是从受限存储取回的一把密钥的本体：HMAC 的共享密钥字节，或 Ed25519 的 32 字节公钥。它只在核验
// 那一刻存在：不落库、不进日志——String、GoString 与 LogValue 一律只交出替身，做法同 IntegrationClientCredential。
type PushSigningKeyMaterial struct{ bytes []byte }

// NewPushSigningKeyMaterial 按值拷进来，交出之后存储那一侧再改它手里那份，改不到核验用的这一份。
func NewPushSigningKeyMaterial(raw []byte) PushSigningKeyMaterial {
	return PushSigningKeyMaterial{bytes: append([]byte(nil), raw...)}
}

func (PushSigningKeyMaterial) String() string { return redactedPushSigningKey }

func (PushSigningKeyMaterial) GoString() string { return redactedPushSigningKey }

func (PushSigningKeyMaterial) LogValue() slog.Value { return slog.StringValue(redactedPushSigningKey) }

// PushSigningKeyResolver 按受控引用去租户约定的受限存储取回密钥本体。取不回交回错误，铸造器答依赖故障；部署根本没给受限
// 存储时答 ErrAccessChannelNotConfigured，铸造器原样交回。
type PushSigningKeyResolver interface {
	ResolvePushSigningKey(ctx context.Context, reference CredentialReference) (PushSigningKeyMaterial, error)
}

// UnconfiguredPushSigningKeyResolver 是部署没给受限存储时的取用方：对任何引用都答 ErrAccessChannelNotConfigured。
type UnconfiguredPushSigningKeyResolver struct{}

func (UnconfiguredPushSigningKeyResolver) ResolvePushSigningKey(context.Context, CredentialReference) (PushSigningKeyMaterial, error) {
	return PushSigningKeyMaterial{}, ErrAccessChannelNotConfigured
}

// PushReceipt 是重放账上的一行：哪个推送源、哪份签名原文（SHA-256 摘要）、推送标识与签名时刻，以及这一行至少要留到
// 什么时候。
//
// 重放键取签名原文的摘要而不取推送标识。截获者改不动原文里的任何一个字节——时间戳也在签名里——所以窗口内能重放的只有
// 逐字节相同的那一份，摘要一份不漏；推送方的重试照 Standard Webhooks 带新的尝试时间戳重签，原文不同，不在这里拦，由
// 各命令口按载荷摘要答同身份重放。按推送标识拦会把合法重试也挡在命令口之外，等于在接入层另立一份幂等口径。
//
// 保留到签名时刻加窗口即够：过了那一刻，同一份原文先被越窗那一格拦下，账上不必再记它。
type PushReceipt struct {
	tenantID    string
	source      PushSourceID
	pushID      string
	digest      string
	signedAt    time.Time
	retainUntil time.Time
}

func NewPushReceipt(tenantID string, source PushSourceID, push SignedPush, retainUntil time.Time) (PushReceipt, error) {
	tenant := strings.TrimSpace(tenantID)
	if tenant == "" || source.value == "" || push.id == "" || retainUntil.Before(push.signedAt) {
		return PushReceipt{}, fmt.Errorf("access identity: push receipt is incomplete")
	}
	sum := sha256.Sum256(push.signedContent())
	return PushReceipt{
		tenantID:    tenant,
		source:      source,
		pushID:      push.id,
		digest:      "sha256:" + hex.EncodeToString(sum[:]),
		signedAt:    push.signedAt,
		retainUntil: retainUntil,
	}, nil
}

func (receipt PushReceipt) TenantID() string       { return receipt.tenantID }
func (receipt PushReceipt) Source() PushSourceID   { return receipt.source }
func (receipt PushReceipt) PushID() string         { return receipt.pushID }
func (receipt PushReceipt) Digest() string         { return receipt.digest }
func (receipt PushReceipt) SignedAt() time.Time    { return receipt.signedAt }
func (receipt PushReceipt) RetainUntil() time.Time { return receipt.retainUntil }

// PushReplayLedger 是重放账：记下一份推送已被收下。同一推送源下同一份签名原文已在账上时答 first=false，不覆盖、不报错；
// 读写不动交回错误。
//
// 写要落在调用方开的框架事务里，与这份推送带来的业务写同一次提交：业务写回滚了，这一行跟着消失，推送方原样再发不会被
// 当成重放；两份同样的推送并发到达，后到那一份在唯一键上等前一份的事务落定，再答重放。
type PushReplayLedger interface {
	RecordPushReceipt(ctx context.Context, receipt PushReceipt) (first bool, err error)
}
