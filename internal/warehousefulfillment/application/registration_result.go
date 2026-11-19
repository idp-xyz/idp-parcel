package application

import "fmt"

// RegistrationOutcome 是各册登记用例共用的结果代数（ADR-0031）：`REGISTERED` 是本次落册；`ALREADY_REGISTERED` 是同一修订号、
// 内容相同的重放，回显在册的那一份；`CONTENT_CONFLICT` 是同一修订号、内容不同，不覆盖在册的；`NOT_ACCEPTED` 是新修订没通过
// 核对，一个字节没写，并且一定带一个封闭的拒绝理由；`NOT_CONFIGURED` 只属于不指名仓的册（库位类型、包材）：本租户一个仓
// 都没登记，租户取值还没到，不是拒绝（UC-WF-001「结果语义契约」），所以不带拒绝理由、指出的格与最新修订号，也不落册。
//
// 各册的字面量逐字相同（UC-WF-001「结果语义契约」）：登记方读一册学会的结果词，在别的册上照样成立。形状（落点 + 拒绝理由 +
// 指出的格）因此也只立一套，往后某册要加的结果落在下面的结果与拒绝理由枚举里再添一格，不另抄一份。
type RegistrationOutcome uint8

const (
	RegistrationOutcomeInvalid RegistrationOutcome = iota
	RegistrationRegistered
	RegistrationAlreadyRegistered
	RegistrationContentConflict
	RegistrationNotAccepted
	RegistrationNotConfigured
)

func (outcome RegistrationOutcome) String() string {
	switch outcome {
	case RegistrationRegistered:
		return "REGISTERED"
	case RegistrationAlreadyRegistered:
		return "ALREADY_REGISTERED"
	case RegistrationContentConflict:
		return "CONTENT_CONFLICT"
	case RegistrationNotAccepted:
		return "NOT_ACCEPTED"
	case RegistrationNotConfigured:
		return "NOT_CONFIGURED"
	default:
		return ""
	}
}

// RegistrationRefusal 是 `NOT_ACCEPTED` 的原因。逐格分开是因为登记方的续办各不相同：修订错位要重读册面，不可改的格要换成
// 原来的值，内容不合要改那一格的取值，指名的对象查不到要先去登记它。
type RegistrationRefusal uint8

const (
	RegistrationRefusalNone RegistrationRefusal = iota
	RegistrationRefusalRevisionOutOfSequence
	RegistrationRefusalImmutableField
	RegistrationRefusalContentInvalid
	RegistrationRefusalReferenceNotFound
)

func (refusal RegistrationRefusal) String() string {
	switch refusal {
	case RegistrationRefusalRevisionOutOfSequence:
		return "REVISION_OUT_OF_SEQUENCE"
	case RegistrationRefusalImmutableField:
		return "IMMUTABLE_FIELD"
	case RegistrationRefusalContentInvalid:
		return "CONTENT_INVALID"
	case RegistrationRefusalReferenceNotFound:
		return "REFERENCE_NOT_FOUND"
	default:
		return ""
	}
}

// RegistrationResult 携带落点、`NOT_ACCEPTED` 的原因与细节。输入被拒是已形成的答案，不是技术失败；只有册读写不动才另走
// error。Revision 是回显的那一笔的类型，Field 是这一册里「指出哪一格」的取值类型：两者随册变，结果的其余部分各册一致。
type RegistrationResult[Revision any, Field fmt.Stringer] struct {
	outcome        RegistrationOutcome
	refusal        RegistrationRefusal
	field          Field
	latestRevision int
	registered     Revision
	hasRegistered  bool
}

func (result RegistrationResult[Revision, Field]) Outcome() RegistrationOutcome {
	return result.outcome
}

// Refusal 只在 NOT_ACCEPTED 时有值。
func (result RegistrationResult[Revision, Field]) Refusal() RegistrationRefusal {
	return result.refusal
}

// Field 指出 IMMUTABLE_FIELD、CONTENT_INVALID 与 REFERENCE_NOT_FOUND 差在哪一格；别的结果上是零值。
func (result RegistrationResult[Revision, Field]) Field() Field {
	return result.field
}

// LatestRevision 只在 REVISION_OUT_OF_SEQUENCE 时有值：在册的最新修订号，册上没有这个对象时是 0。
func (result RegistrationResult[Revision, Field]) LatestRevision() int {
	return result.latestRevision
}

// Revision 只在 REGISTERED 与 ALREADY_REGISTERED 时有：落册或在册的那一笔。重放回显的是在册那一份，登记责任方与登记时刻
// 因此是当初落册时的，不是这次重投的。
func (result RegistrationResult[Revision, Field]) Revision() (Revision, bool) {
	return result.registered, result.hasRegistered
}
