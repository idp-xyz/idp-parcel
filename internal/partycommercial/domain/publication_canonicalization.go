package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// 本文件是商业发布的服务端规范化与内容摘要（ADR-0126 Decision 一）。摘要只在这里算：
// 预览口、录入口与受控批文的对账门都调同一个函数，同一份正文在三处得到逐字节相同的串
// ——这正是「表单不算摘要」那句硬句能成立的结构性理由。

var (
	// ErrRegisterNotCanonicalized 是「这一册今天还没接进服务端规范化」的答复。它是一格答案不是缺陷：
	// 首例只接信用政策，其余各册由各自的子票接进同一个版本号（加册不换号，ADR-0126 Decision 一）。
	ErrRegisterNotCanonicalized = errors.New("party commercial: this register is not canonicalized by this build")
	// ErrPublicationContentAbsent 表示已接的册没带正文——正文缺席时没有东西可折成文档。
	ErrPublicationContentAbsent = errors.New("party commercial: publication content is absent")
	// ErrPublicationContentKindMismatch 表示正文的册与版本壳声明的类别不是同一个：一册的正文不得冒
	// 另一册的名，也不得让「壳说信用政策、正文是结算政策」这种输入静默折成某一册的摘要。
	ErrPublicationContentKindMismatch = errors.New("party commercial: publication content does not belong to the declared kind")
	// ErrCanonicalizationUnsupported 表示声明的摘要带着本构建不认识的规范化版本：既不能重算也不能比，
	// 与「算出来不相等」是两格——后者改正文或改串，前者要换构建（ADR-0014）。
	ErrCanonicalizationUnsupported = errors.New("party commercial: canonicalization version not supported by this build")
)

// publicationCanonicalizationVersion 标识商业发布内容摘要所依据的规范化文档形状。摘要只在同一
// 规范化版本内可比；**既有册的文档形状变化才递增**，加一册不换号（ADR-0126 Decision 一），
// 理由是一册从「没接」到「接了」不改任何已算出的字节。见 ADR-0014。
//
// PCC-1：信用政策正文（责任法人 × 权限等级 × 费用类型 × 额度恰一格 × 区间）。
const publicationCanonicalizationVersion = "PCC-1"

// canonicalDigestSeparator 把版本前缀与十六进制摘要分开：`PCC-1:<hex>`。串自带版本是 ADR-0014
// 「已保存摘要必须携带产生它的规范化版本」在本上下文的落法（先例 parcel-shipment 的 `PSC-1:<sha256>`），
// 不给版本册加列——一列会让没版本的旧串与有版本的新串在库上长成同一种东西。
const canonicalDigestSeparator = ":"

// CurrentPublicationCanonicalizationVersion 报出本构建按哪套形状规范化商业发布正文。
func CurrentPublicationCanonicalizationVersion() string { return publicationCanonicalizationVersion }

// Canonicalization 报出本摘要串携带的规范化版本。没带版本的串（今天册上 `sha256:syn-…` 那类声明串）
// 答 false：按 ADR-0014 它们是不完整的摘要，与算出的串不可比——调用方据此分辨「能对账」与「无从对账」，
// 而不是拿一个空版本去比。
//
// 判据是前缀 `PCC-` 加数字：`sha256:` 那类前缀不是规范化版本，不得被读成一个。
func (digest CommercialContentDigest) Canonicalization() (string, bool) {
	version, _, found := strings.Cut(digest.value, canonicalDigestSeparator)
	if !found || !isPublicationCanonicalizationVersion(version) {
		return "", false
	}
	return version, true
}

func isPublicationCanonicalizationVersion(candidate string) bool {
	rest, ok := strings.CutPrefix(candidate, "PCC-")
	if !ok || rest == "" {
		return false
	}
	for _, character := range rest {
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
}

// CreditPolicyBody 是信用政策版本的正文，以领域值对象给出：它就是 NewCreditPolicy 收的那几项，
// 只是不带拥有它的（已生效）版本——预览与录入发生在发布之前，那时没有已生效版本可挂。
// 各项立不立得住由各自的构造门判过；这里只在折成文档前再核一遍非零，让一个绕过构造门拼出来的
// 零值正文折不出摘要。
type CreditPolicyBody struct {
	LegalEntity LegalEntityReference
	Level       AuthorityLevel
	ChargeType  ChargeTypeReference
	Limit       CreditLimit
	Effective   EffectiveInterval
}

func (body CreditPolicyBody) valid() bool {
	return body.LegalEntity.valid() && body.Level.valid() && body.ChargeType.valid() &&
		body.Limit.valid() && body.Effective.valid()
}

// PublicationContent 是一次商业发布的正文输入面：版本壳声明的类别，加上该类别的正文。各册的
// 正文各占一格、按类别只认自己那一格；今天只有信用政策一格，其余各册由子票在此加格。
//
// 是「类别 + 各册一格」而不是接口：十册正文各自是封闭结构，接口会让「哪一册接了」只能从实现
// 有没有推出来，而 ErrRegisterNotCanonicalized 要能对着类别直接答。
type PublicationContent struct {
	Kind         CommercialObjectKind
	CreditPolicy *CreditPolicyBody
}

// CanonicalPublicationContent 是规范化的结果：版本、摘要串与被摘要盖住的那份文档。摘要串已带版本前缀，
// 可直接作 CommercialVersionSpec.ContentDigest。
//
// 文档随结果交出，是因为待批准发布（ADR-0126 Decision 三）要存一份正文快照：存的就是这份文档——它恰是
// 摘要盖住的那些字节，快照与摘要因此是一样东西的两面，重建时折回正文再算一遍就能对上列里的摘要。
type CanonicalPublicationContent struct {
	canonicalization string
	digest           CommercialContentDigest
	document         []byte
}

func (canonical CanonicalPublicationContent) Canonicalization() string {
	return canonical.canonicalization
}

func (canonical CanonicalPublicationContent) Digest() CommercialContentDigest {
	return canonical.digest
}

// Document 交回规范化文档的字节（副本）：JSON，字段顺序由结构体钉死，键名镜像批文。
func (canonical CanonicalPublicationContent) Document() []byte {
	return append([]byte(nil), canonical.document...)
}

// RehydratePublicationContent 把一份存下来的规范化文档折回正文输入面。只认本构建自己的规范化版本：
// 别的版本既折不回也不该猜，答 ErrCanonicalizationUnsupported（ADR-0014）。折回的每一格都过领域构造门——
// 快照是数据，但正文立不立得住仍由构造门说；文档里没有任何一册的正文时答 ErrPublicationContentAbsent。
func RehydratePublicationContent(canonicalization string, document []byte) (PublicationContent, error) {
	none := PublicationContent{}
	if canonicalization != publicationCanonicalizationVersion {
		return none, fmt.Errorf("%w: document canonicalized as %q, this build canonicalizes %s",
			ErrCanonicalizationUnsupported, canonicalization, publicationCanonicalizationVersion)
	}
	var decoded canonicalPublicationDocument
	if err := json.Unmarshal(document, &decoded); err != nil {
		return none, fmt.Errorf("rehydrate publication content: %w", err)
	}
	if decoded.Canonicalization != canonicalization {
		return none, fmt.Errorf("%w: document says %q, column says %q",
			ErrCanonicalizationUnsupported, decoded.Canonicalization, canonicalization)
	}
	kind, known := CommercialObjectKindNamed(decoded.Kind)
	if !known {
		return none, fmt.Errorf("rehydrate publication content: %w: kind %q", ErrInvalidCommercialVersion, decoded.Kind)
	}
	content := PublicationContent{Kind: kind}
	if decoded.CreditPolicy != nil {
		body, err := decoded.CreditPolicy.body()
		if err != nil {
			return none, fmt.Errorf("rehydrate publication content: credit policy: %w", err)
		}
		content.CreditPolicy = &body
	}
	if content.CreditPolicy == nil {
		return none, ErrPublicationContentAbsent
	}
	return content, nil
}

// CommercialObjectKindNamed 按 String() 的原词反查类别：规范化文档里的 kind、运营操作者面载荷里的 kind 都是那一个词，
// 两处不各自抄一份名单——名单在 String() 一处，这里只是反查。集合外答 false。
func CommercialObjectKindNamed(name string) (CommercialObjectKind, bool) {
	for kind := ServiceProductObject; kind.valid(); kind++ {
		if kind.String() == name {
			return kind, true
		}
	}
	return CommercialObjectKindInvalid, false
}

// CanonicalizePublicationContent 按册把正文折成规范化文档并算出内容摘要（ADR-0126 Decision 一）。
//
// 文档只盖正文，不盖版本壳的身份四元与壳上的范围、区间：身份是键，范围与区间是 SaveVersion 逐列
// 比对的项，盖进摘要只会让「同内容换范围」从`内容冲突`折成两个串不同。文档带 kind，一册的正文
// 冒不了另一册的名。
//
// 没接的册答 ErrRegisterNotCanonicalized，已接的册正文缺席答 ErrPublicationContentAbsent，正文与类别
// 不符答 ErrPublicationContentKindMismatch——三格恢复动作各不相同（等子票 / 补正文 / 改壳或改正文），
// 不折成一个「算不出」。
func CanonicalizePublicationContent(content PublicationContent) (CanonicalPublicationContent, error) {
	none := CanonicalPublicationContent{}
	if !content.Kind.valid() {
		return none, ErrInvalidCommercialVersion
	}
	if content.CreditPolicy != nil && content.Kind != CreditPolicyObject {
		return none, ErrPublicationContentKindMismatch
	}
	switch content.Kind {
	case CreditPolicyObject:
		if content.CreditPolicy == nil {
			return none, ErrPublicationContentAbsent
		}
		if !content.CreditPolicy.valid() {
			return none, ErrInvalidCreditPolicy
		}
		return canonicalDigestOf(canonicalPublicationDocument{
			Canonicalization: publicationCanonicalizationVersion,
			Kind:             content.Kind.String(),
			CreditPolicy:     canonicalCreditPolicyBodyOf(*content.CreditPolicy),
		})
	default:
		return none, ErrRegisterNotCanonicalized
	}
}

// IsRegisterCanonicalized 答某一册今天接没接进服务端规范化。对账门（ADR-0126 Decision 二）用它分辨
// 「声明的串与算出的不等」与「这一册无从对账」。
func IsRegisterCanonicalized(kind CommercialObjectKind) bool {
	return kind == CreditPolicyObject
}

// canonicalPublicationDocument 是 PCC-1 的文档形状。字段顺序由结构体钉死——json.Marshal 按声明顺序
// 输出，这就是「规范化」的全部：同一份正文只有一种字节。各册一格、缺席省略，让文档的字节只由
// 在场的那一册决定。
type canonicalPublicationDocument struct {
	Canonicalization string                     `json:"canonicalization"`
	Kind             string                     `json:"kind"`
	CreditPolicy     *canonicalCreditPolicyBody `json:"creditPolicy,omitempty"`
}

// canonicalCreditPolicyBody 镜像批文 creditPolicyBodyDocument 的键名：额度两键恰一在场、区间上界可缺。
// 时刻一律 UTC RFC 3339 纳秒——同一时刻在两个时区写出两个串，摘要就成了两个。
type canonicalCreditPolicyBody struct {
	LegalEntity           string `json:"legalEntity"`
	AuthorityLevel        string `json:"authorityLevel"`
	ChargeType            string `json:"chargeType"`
	LimitMinor            *int64 `json:"limitMinor,omitempty"`
	LimitRatioBasisPoints *int64 `json:"limitRatioBasisPoints,omitempty"`
	EffectiveStartsAt     string `json:"effectiveStartsAt"`
	EffectiveEndsAt       string `json:"effectiveEndsAt,omitempty"`
}

func canonicalCreditPolicyBodyOf(body CreditPolicyBody) *canonicalCreditPolicyBody {
	document := &canonicalCreditPolicyBody{
		LegalEntity:       body.LegalEntity.String(),
		AuthorityLevel:    body.Level.String(),
		ChargeType:        body.ChargeType.String(),
		EffectiveStartsAt: canonicalTime(body.Effective.StartsAt()),
	}
	if minor, isAmount := body.Limit.AmountMinor(); isAmount {
		document.LimitMinor = &minor
	}
	if basisPoints, isRatio := body.Limit.RatioBasisPoints(); isRatio {
		document.LimitRatioBasisPoints = &basisPoints
	}
	if endsAt, bounded := body.Effective.EndsAt(); bounded {
		document.EffectiveEndsAt = canonicalTime(endsAt)
	}
	return document
}

// body 把文档里的一节折回领域正文。额度两键恰一在场由 creditLimitOf 判；时刻按写出时同一格式读回。
func (document canonicalCreditPolicyBody) body() (CreditPolicyBody, error) {
	legalEntity, err := NewLegalEntityReference(document.LegalEntity)
	if err != nil {
		return CreditPolicyBody{}, err
	}
	level, err := NewAuthorityLevel(document.AuthorityLevel)
	if err != nil {
		return CreditPolicyBody{}, err
	}
	chargeType, err := NewChargeTypeReference(document.ChargeType)
	if err != nil {
		return CreditPolicyBody{}, err
	}
	limit, err := creditLimitOf(document.LimitMinor, document.LimitRatioBasisPoints)
	if err != nil {
		return CreditPolicyBody{}, err
	}
	startsAt, err := time.Parse(time.RFC3339Nano, document.EffectiveStartsAt)
	if err != nil {
		return CreditPolicyBody{}, fmt.Errorf("effectiveStartsAt: %w", err)
	}
	var endsAt time.Time
	if document.EffectiveEndsAt != "" {
		if endsAt, err = time.Parse(time.RFC3339Nano, document.EffectiveEndsAt); err != nil {
			return CreditPolicyBody{}, fmt.Errorf("effectiveEndsAt: %w", err)
		}
	}
	effective, err := NewEffectiveInterval(startsAt, endsAt)
	if err != nil {
		return CreditPolicyBody{}, err
	}
	return CreditPolicyBody{
		LegalEntity: legalEntity,
		Level:       level,
		ChargeType:  chargeType,
		Limit:       limit,
		Effective:   effective,
	}, nil
}

// creditLimitOf 把文档里并存的两键折回两格封闭的额度：恰一在场才立得住，两空或两满是文档与领域分叉。
func creditLimitOf(minor, basisPoints *int64) (CreditLimit, error) {
	switch {
	case minor != nil && basisPoints == nil:
		return NewCreditAmountLimit(*minor)
	case minor == nil && basisPoints != nil:
		return NewCreditRatioLimit(*basisPoints)
	default:
		return CreditLimit{}, ErrInvalidCreditLimit
	}
}

func canonicalTime(at time.Time) string {
	return at.UTC().Format(time.RFC3339Nano)
}

// canonicalDigestOf 与 parcel-pricing 的 hashCanonical 同形：json.Marshal → SHA-256 → hex，再冠以
// 规范化版本。Marshal 失败上抛而不是交回空串——空串过不了 NewCommercialContentDigest，但让它在
// 这里就响亮，比让调用方去猜「为什么摘要是空的」好。
func canonicalDigestOf(document canonicalPublicationDocument) (CanonicalPublicationContent, error) {
	encoded, err := json.Marshal(document)
	if err != nil {
		return CanonicalPublicationContent{}, fmt.Errorf("canonicalize publication content: %w", err)
	}
	sum := sha256.Sum256(encoded)
	digest, err := NewCommercialContentDigest(
		document.Canonicalization + canonicalDigestSeparator + hex.EncodeToString(sum[:]))
	if err != nil {
		return CanonicalPublicationContent{}, err
	}
	return CanonicalPublicationContent{canonicalization: document.Canonicalization, digest: digest, document: encoded}, nil
}

// ReconcileDeclaredDigest 是受控批文那一半的对账门（ADR-0126 Decision 二）：声明的摘要串与算出的
// 逐字节比。
//
// 三格：相等答 nil；声明的串带着本构建不认识的规范化版本答 ErrCanonicalizationUnsupported（不能重算
// 也不能比）；其余不等答 ErrDeclaredDigestMismatch——包括没带版本的旧式声明串，它与算出的串就是两个
// 不同的串，这一册既已接进规范化，调用方该抄算出的那一个。
func ReconcileDeclaredDigest(declared CommercialContentDigest, canonical CanonicalPublicationContent) error {
	if declared == canonical.digest {
		return nil
	}
	if version, carried := declared.Canonicalization(); carried && version != canonical.canonicalization {
		return fmt.Errorf("%w: declared %s, this build canonicalizes %s",
			ErrCanonicalizationUnsupported, version, canonical.canonicalization)
	}
	return ErrDeclaredDigestMismatch
}

// ErrDeclaredDigestMismatch 表示声明的内容摘要与服务端按正文算出的不相等：这一份输入内部不自洽，
// 恢复动作是改批文（抄算出的串或改正文），与册上他版的`内容冲突`（换版本号）是两格（ADR-0031）。
var ErrDeclaredDigestMismatch = errors.New("party commercial: declared content digest does not match the canonical digest")
