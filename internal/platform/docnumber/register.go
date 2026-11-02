package docnumber

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// RuleVersion 是规则册上一类单据的一版租户规则（ADR-0355 决定五、六）：模板与生效时刻，外加谁在什么时刻登的。
// 只增不改：换规则是新的一版，已开立的单据号不随之改。
type RuleVersion struct {
	Tenant        string
	Type          TypeCode
	Template      string
	EffectiveFrom time.Time
	RegisteredBy  string
	RegisteredAt  time.Time
}

// TimeZoneVersion 是租户单据编号时区的一版（ADR-0355 决定七）：IANA 时区名与生效时刻。它是租户取值，产品不给
// 默认；没有任何一版生效时，开立方没有业务时区的单据开立不带号。
type TimeZoneVersion struct {
	Tenant        string
	TimeZone      string
	EffectiveFrom time.Time
	RegisteredBy  string
	RegisteredAt  time.Time
}

// RuleBook 是规则册的存储端口。两种版本都按（租户，[类型，]生效时刻）为键只增不改。
type RuleBook interface {
	// AppendRuleVersion 追加一版。同键已有一版时不落，交回在册那一版与 false。
	AppendRuleVersion(ctx context.Context, version RuleVersion) (stored RuleVersion, appended bool, err error)
	// AppendTimeZoneVersion 追加一版单据编号时区，同键语义同上。
	AppendTimeZoneVersion(ctx context.Context, version TimeZoneVersion) (stored TimeZoneVersion, appended bool, err error)
	// EffectiveRuleVersion 交回 at 时刻生效的那一版：生效时刻不晚于 at 的最晚一版。没有答 false。
	EffectiveRuleVersion(ctx context.Context, tenant string, code TypeCode, at time.Time) (RuleVersion, bool, error)
	// EffectiveTimeZoneVersion 交回 at 时刻生效的单据编号时区，没有答 false。
	EffectiveTimeZoneVersion(ctx context.Context, tenant string, at time.Time) (TimeZoneVersion, bool, error)
	// RuleVersions 按类型代码、生效时刻排序交回该租户的全部规则版本。
	RuleVersions(ctx context.Context, tenant string) ([]RuleVersion, error)
	// TimeZoneVersions 按生效时刻排序交回该租户的全部单据编号时区版本。
	TimeZoneVersions(ctx context.Context, tenant string) ([]TimeZoneVersion, error)
}

// Clock 是本包取「此刻」的端口。登记时刻、取号时刻都取服务端时钟，不取载荷声明的时刻（ADR-0355 决定七）。
type Clock interface {
	Now() time.Time
}

var (
	// ErrRegistrationRejected 是登记被拒的总哨兵，具体拒因另以哨兵包在同一个错误里（模板检查各格、
	// ErrTypeNotDeclared、ErrScrambleSecretNotConfigured、ErrTimeZoneNotRecognized、ErrRegistrationIncomplete）。
	ErrRegistrationRejected = errors.New("document numbering: registration rejected")
	// ErrRegistrationIncomplete 表示登记缺了必给的一格（租户、登记人、生效时刻）。
	ErrRegistrationIncomplete = errors.New("document numbering: registration is incomplete")
	// ErrTimeZoneNotRecognized 表示给的不是可用的 IANA 时区名。
	ErrTimeZoneNotRecognized = errors.New("document numbering: time zone is not a recognized IANA name")
)

// RegistrationOutcome 是一次登记形成的答案。
type RegistrationOutcome string

const (
	// VersionRegistered：新落一版。
	VersionRegistered RegistrationOutcome = "VERSION_REGISTERED"
	// VersionExisting：同一生效时刻已有内容相同的一版，是重放，册上什么也没变。
	VersionExisting RegistrationOutcome = "VERSION_EXISTING"
	// EffectiveFromTaken：同一生效时刻已有内容不同的一版。版本只增不改，要换就另取一个生效时刻。
	EffectiveFromTaken RegistrationOutcome = "EFFECTIVE_FROM_TAKEN"
)

// RuleRegistration 是规则版本登记的答案：Version 是答案所指的那一版（新落的、已在册的或占着那一刻的）。
type RuleRegistration struct {
	Outcome RegistrationOutcome
	Version RuleVersion
}

// TimeZoneRegistration 是单据编号时区登记的答案，Version 的语义同 RuleRegistration。
type TimeZoneRegistration struct {
	Outcome RegistrationOutcome
	Version TimeZoneVersion
}

// RegisterRuleCommand 登记一版规则。RegisteredBy 是登记人（在线口取操作者主体，受控登记口写明那一道运维边界）。
type RegisterRuleCommand struct {
	Tenant        string
	Type          TypeCode
	Template      string
	EffectiveFrom time.Time
	RegisteredBy  string
}

// RegisterTimeZoneCommand 登记一版单据编号时区。
type RegisterTimeZoneCommand struct {
	Tenant        string
	TimeZone      string
	EffectiveFrom time.Time
	RegisteredBy  string
}

// RegistrarDeps 是登记编排的依赖。
type RegistrarDeps struct {
	Catalogue *Catalogue
	Book      RuleBook
	Secrets   ScrambleSecrets
	Clock     Clock
}

// Registrar 是规则册的登记编排。写册要在一笔事务里，事务边界归装配点。
type Registrar struct {
	catalogue *Catalogue
	book      RuleBook
	secrets   ScrambleSecrets
	clock     Clock
}

func NewRegistrar(deps RegistrarDeps) (*Registrar, error) {
	if deps.Catalogue == nil || deps.Book == nil || deps.Secrets == nil || deps.Clock == nil {
		return nil, errors.New("document numbering: registrar needs a catalogue, a rule book, scramble secrets and a clock")
	}
	return &Registrar{catalogue: deps.Catalogue, book: deps.Book, secrets: deps.Secrets, clock: deps.Clock}, nil
}

// RegisterRule 登记一版租户规则：类型须已声明、模板须过模板检查，用了加扰而该租户没配密钥即拒。
//
// 生效时刻可以早于登记时刻：号在开立时取、开立后不变，生效时刻早于登记的那一段改不到已开立的单据，册上同时记着
// 登记时刻，读的人分得清。拒它反倒让重灌种子与受控登记口没法用一个固定的生效时刻重放。
func (registrar *Registrar) RegisterRule(ctx context.Context, command RegisterRuleCommand) (RuleRegistration, error) {
	if err := requireRegistrationBasics(command.Tenant, command.RegisteredBy, command.EffectiveFrom); err != nil {
		return RuleRegistration{}, err
	}
	if _, declared := registrar.catalogue.Declaration(command.Type); !declared {
		return RuleRegistration{}, fmt.Errorf("%w: %w: %s", ErrRegistrationRejected, ErrTypeNotDeclared, command.Type)
	}
	checked, err := checkTemplate(command.Type, command.Template)
	if err != nil {
		return RuleRegistration{}, fmt.Errorf("%w: %w", ErrRegistrationRejected, err)
	}
	if checked.scrambled {
		if _, configured := registrar.secrets.ScrambleSecret(command.Tenant); !configured {
			return RuleRegistration{}, fmt.Errorf("%w: %w", ErrRegistrationRejected, ErrScrambleSecretNotConfigured)
		}
	}

	version := RuleVersion{
		Tenant:        command.Tenant,
		Type:          command.Type,
		Template:      command.Template,
		EffectiveFrom: storedInstant(command.EffectiveFrom),
		RegisteredBy:  command.RegisteredBy,
		RegisteredAt:  storedInstant(registrar.clock.Now()),
	}
	stored, appended, err := registrar.book.AppendRuleVersion(ctx, version)
	if err != nil {
		return RuleRegistration{}, fmt.Errorf("document numbering: append rule version: %w", err)
	}
	switch {
	case appended:
		return RuleRegistration{Outcome: VersionRegistered, Version: stored}, nil
	case stored.Template == version.Template:
		return RuleRegistration{Outcome: VersionExisting, Version: stored}, nil
	default:
		return RuleRegistration{Outcome: EffectiveFromTaken, Version: stored}, nil
	}
}

// RegisterTimeZone 登记一版单据编号时区。生效时刻可早于登记时刻，理由同 RegisterRule。
func (registrar *Registrar) RegisterTimeZone(ctx context.Context, command RegisterTimeZoneCommand) (TimeZoneRegistration, error) {
	if err := requireRegistrationBasics(command.Tenant, command.RegisteredBy, command.EffectiveFrom); err != nil {
		return TimeZoneRegistration{}, err
	}
	if _, err := loadTimeZone(command.TimeZone); err != nil {
		return TimeZoneRegistration{}, fmt.Errorf("%w: %w", ErrRegistrationRejected, err)
	}

	version := TimeZoneVersion{
		Tenant:        command.Tenant,
		TimeZone:      command.TimeZone,
		EffectiveFrom: storedInstant(command.EffectiveFrom),
		RegisteredBy:  command.RegisteredBy,
		RegisteredAt:  storedInstant(registrar.clock.Now()),
	}
	stored, appended, err := registrar.book.AppendTimeZoneVersion(ctx, version)
	if err != nil {
		return TimeZoneRegistration{}, fmt.Errorf("document numbering: append numbering time zone version: %w", err)
	}
	switch {
	case appended:
		return TimeZoneRegistration{Outcome: VersionRegistered, Version: stored}, nil
	case stored.TimeZone == version.TimeZone:
		return TimeZoneRegistration{Outcome: VersionExisting, Version: stored}, nil
	default:
		return TimeZoneRegistration{Outcome: EffectiveFromTaken, Version: stored}, nil
	}
}

func requireRegistrationBasics(tenant, registeredBy string, effectiveFrom time.Time) error {
	switch {
	case strings.TrimSpace(tenant) == "":
		return fmt.Errorf("%w: %w: tenant", ErrRegistrationRejected, ErrRegistrationIncomplete)
	case strings.TrimSpace(registeredBy) == "":
		return fmt.Errorf("%w: %w: registered by", ErrRegistrationRejected, ErrRegistrationIncomplete)
	case effectiveFrom.IsZero():
		return fmt.Errorf("%w: %w: effective from", ErrRegistrationRejected, ErrRegistrationIncomplete)
	}
	return nil
}

// storedInstant 把时刻收成库里存得下的精度（PostgreSQL timestamptz 到微秒）并统一到 UTC。不收的话，同一份载荷
// 重放时键里的生效时刻与在册那一版差在纳秒上，比对就会把重放判成换版。
func storedInstant(at time.Time) time.Time {
	return at.UTC().Truncate(time.Microsecond)
}

// loadTimeZone 只收 IANA 时区名。空串与 Local 都被 time.LoadLocation 接受，却分别读成 UTC 与进程所在机器的
// 时区——那等于替租户挑了一个时区，正是 ADR-0355 决定七禁的顶上。
func loadTimeZone(name string) (*time.Location, error) {
	if strings.TrimSpace(name) == "" || name != strings.TrimSpace(name) || name == "Local" {
		return nil, fmt.Errorf("%w: %q", ErrTimeZoneNotRecognized, name)
	}
	location, err := time.LoadLocation(name)
	if err != nil {
		return nil, fmt.Errorf("%w: %q: %v", ErrTimeZoneNotRecognized, name, err)
	}
	return location, nil
}
