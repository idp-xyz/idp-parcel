package docnumber

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.idp.xyz/idp-platform-go/numbering"
)

// MaxSkippedNumbers 是一次取号里最多跳过的已用号数（ADR-0355 决定八：产品策略）。一次取号最多从序列取这么多个值：
// 前面哪一个没被用过就出那一个；这么多个全都已用，就不再取下一个，答「编号规则撞号」。
//
// 取 20：撞号只可能撞上换格式之前已用的号。零星撞几个（旧号在新格式下恰好散落成同形）由跳过吸收；成片地撞，
// 说明新模板与旧号段整段重叠，那是规则册该改的格式问题，跳得再多也只是把一整段流水烧成空号——而空号是审计
// 依据「号码连续」的租户最在意的东西（越权风险点 7）。每跳一次是事务内的一次序列取值加一次「是否已用」的
// 索引查，二十次仍在几十毫秒之内，开立事务与序列行锁不会因此被拖长到影响同类单据并发开立。到上限仍撞就答
// 「编号规则撞号」，让人去看规则册，而不是替租户悄悄烧号。
const MaxSkippedNumbers = 20

var (
	// ErrTransactionRequired 表示取号不在调用方的开立事务里。取号与单据落库须同一事务（ADR-0355 决定三），事务外
	// 取到的号回滚不释放、提交不随单，所以事务外一律不取。
	ErrTransactionRequired = errors.New("document numbering: assignment must run inside the issuing transaction")
	// ErrInvalidIssueRequest 表示取号请求本身不成立（缺租户、缺「是否已用」查询、类型没声明等），是调用方的编程错误。
	ErrInvalidIssueRequest = errors.New("document numbering: invalid issue request")
	// ErrIssuerTimeZoneNotDeclared 表示类型声明说开立方没有业务时区，调用方却交来了一个——声明与实现不一致，
	// 由声明的那一处改，不在这里悄悄择一。
	ErrIssuerTimeZoneNotDeclared = errors.New("document numbering: this document type declares no issuer business time zone")
)

// TransactionScope 答 ctx 是否带着调用方的开立事务。
type TransactionScope interface {
	RequireTransaction(ctx context.Context) error
}

// IssueRequest 是一次取号。
type IssueRequest struct {
	Tenant string
	Type   TypeCode
	// IssuerTimeZone 是开立方已登记的业务时区（IANA 名）。开立方没有或没登时留空，取租户的单据编号时区。
	IssuerTimeZone string
	// IsUsed 答渲染出的号在本类单据里是否已被用过。调用方在同一事务内查自己的表——（租户，类型，单据号）唯一
	// 由单据所在表担保（ADR-0355 决定八），本包不读任何单据表。
	IsUsed func(ctx context.Context, number string) (bool, error)
}

// AssignmentOutcome 是取号的答案。
type AssignmentOutcome string

const (
	// NumberAssigned：取到了号。
	NumberAssigned AssignmentOutcome = "NUMBER_ASSIGNED"
	// NumberingTimeZoneNotConfigured：开立方没交业务时区、租户也没有生效的单据编号时区。单据照常开立、不带号，
	// 无号原因记「单据编号时区未配置」（ADR-0355 决定七）；序列一个值也没动。
	NumberingTimeZoneNotConfigured AssignmentOutcome = "NUMBERING_TIME_ZONE_NOT_CONFIGURED"
	// NumberingRuleCollision：从序列取了 MaxSkippedNumbers 个值，渲染出的号全都已用（ADR-0355 决定八）。这一次开立
	// 不落单，调用方必须回滚开立事务：取过的那些值都没出号，事务一提交它们就全成了空号，而决定三只许随单落定的跳号
	// 成为空号。回滚连同它们一起释放，下一次开立还会撞上同一段，直到规则册换了格式——这正是要页面指到规则册的原因。
	NumberingRuleCollision AssignmentOutcome = "NUMBERING_RULE_COLLISION"
)

// RuleSource 说一类单据此刻用的是哪份规则。
type RuleSource string

const (
	SourceProductDefault RuleSource = "PRODUCT_DEFAULT"
	SourceTenantRule     RuleSource = "TENANT_RULE"
)

// Assignment 是一次取号的结果。Number 只在 NumberAssigned 时非空。
type Assignment struct {
	Outcome  AssignmentOutcome
	Number   string
	Source   RuleSource
	Template string
	// TimeZone 是日期所用的时区；单据编号时区未配置时为空。
	TimeZone string
	// IssuedAt 是取号所用的服务端时刻（ADR-0355 决定七：开立时刻取服务端落单那一刻）。
	IssuedAt time.Time
	// Skipped 是这一次从序列取到、因已用而弃掉的值数。取到号时是号之前那几个，随单提交成为空号（决定八唯一许可的
	// 空号）；答撞号时等于 MaxSkippedNumbers，即取过的每一个值——调用方回滚后它们一个也不留。
	Skipped int
}

// AssignerDeps 是取号编排的依赖。Sequences 与 Transactions 必须接在调用方开立事务所用的同一个库上。
type AssignerDeps struct {
	Catalogue    *Catalogue
	Book         RuleBook
	Sequences    numbering.SequenceAllocator
	Transactions TransactionScope
	Secrets      ScrambleSecrets
	Clock        Clock
}

// Assigner 是本仓的取号编排（ADR-0355 决定四、七、八）。不用平台的 `Assigner.Assign`：它每类单据只认装配时的一份
// 模板、只有一只时钟，满足不了租户换模板与按开立方业务时区出日期；本编排只用平台公开的 Parse、PeriodKey、Render
// 与 SequenceAllocator。
type Assigner struct {
	catalogue    *Catalogue
	book         RuleBook
	sequences    numbering.SequenceAllocator
	transactions TransactionScope
	secrets      ScrambleSecrets
	clock        Clock
}

func NewAssigner(deps AssignerDeps) (*Assigner, error) {
	if deps.Catalogue == nil || deps.Book == nil || deps.Sequences == nil || deps.Transactions == nil || deps.Secrets == nil || deps.Clock == nil {
		return nil, errors.New("document numbering: assigner needs a catalogue, a rule book, sequences, a transaction scope, scramble secrets and a clock")
	}
	return &Assigner{
		catalogue:    deps.Catalogue,
		book:         deps.Book,
		sequences:    deps.Sequences,
		transactions: deps.Transactions,
		secrets:      deps.Secrets,
		clock:        deps.Clock,
	}, nil
}

// Assign 在调用方的开立事务内取一个号：读此刻生效的模板（租户版或产品默认）→ 定时区 → 用服务端此刻在该时区下
// 算周期 → 经平台序列取（租户，类型代码，周期）的下一个值 → 渲染 → 经调用方的「是否已用」判撞，撞了取下一个值。
//
// 调用方按答案办：取到号就随单落库；答单据编号时区未配置，单据照常开立、不带号，记无号原因；答编号规则撞号或交回
// 错误，必须回滚开立事务——撞号时已取的流水只有回滚才释放（见 NumberingRuleCollision）。
//
// 先判事务、后读规则：单据编号时区未配置那一格不碰序列，若放在事务判之前，事务外调用就会在那一格上「成功」，
// 而同一个调用方换个租户才失败。
func (assigner *Assigner) Assign(ctx context.Context, request IssueRequest) (Assignment, error) {
	if err := assigner.transactions.RequireTransaction(ctx); err != nil {
		return Assignment{}, fmt.Errorf("%w: %w", ErrTransactionRequired, err)
	}
	if strings.TrimSpace(request.Tenant) == "" || request.IsUsed == nil {
		return Assignment{}, fmt.Errorf("%w: tenant and the used-number query are required", ErrInvalidIssueRequest)
	}
	declaration, declared := assigner.catalogue.Declaration(request.Type)
	if !declared {
		return Assignment{}, fmt.Errorf("%w: %w: %s", ErrInvalidIssueRequest, ErrTypeNotDeclared, request.Type)
	}
	if request.IssuerTimeZone != "" && declaration.DateTimeZone != DateTimeZoneFromIssuer {
		return Assignment{}, fmt.Errorf("%w: %s", ErrIssuerTimeZoneNotDeclared, request.Type)
	}

	now := assigner.clock.Now()
	raw, source, err := assigner.effectiveTemplate(ctx, request.Tenant, request.Type, now)
	if err != nil {
		return Assignment{}, err
	}
	// 每次取号都把生效模板再过一遍模板检查，而不是信登记那一刻的结论：平台库升级可能改了令牌的渲染，宁可在这里
	// 带原因失败，也不出一个少了一段的号。
	checked, err := checkTemplate(request.Type, raw)
	if err != nil {
		return Assignment{}, fmt.Errorf("document numbering: effective template of %s no longer passes the template check: %w", request.Type, err)
	}
	result := Assignment{Source: source, Template: raw, IssuedAt: now}

	location, zone, found, err := assigner.issuingTimeZone(ctx, request, now)
	if err != nil {
		return Assignment{}, err
	}
	if !found {
		result.Outcome = NumberingTimeZoneNotConfigured
		return result, nil
	}
	result.TimeZone = zone

	secret := ""
	if checked.scrambled {
		configured := false
		if secret, configured = assigner.secrets.ScrambleSecret(request.Tenant); !configured {
			// 登记时配过、后来从实例配置里撤了：这是部署故障，不是业务答案，开立随之失败，运维去补密钥。
			return Assignment{}, fmt.Errorf("document numbering: %s: %w", request.Type, ErrScrambleSecretNotConfigured)
		}
	}

	template := checked.template
	at := now.In(location)
	key := numbering.SequenceKey{Scope: request.Tenant, DocType: string(request.Type), Period: template.PeriodKey(at)}
	for skipped := 0; skipped < MaxSkippedNumbers; skipped++ {
		seq, err := assigner.sequences.Next(ctx, key)
		if err != nil {
			return Assignment{}, fmt.Errorf("document numbering: allocate %s/%s: %w", request.Tenant, request.Type, err)
		}
		number, err := render(template, request.Type, request.Tenant, secret, seq, at, nil)
		if err != nil {
			return Assignment{}, fmt.Errorf("document numbering: render %s: %w", request.Type, err)
		}
		used, err := request.IsUsed(ctx, number)
		if err != nil {
			return Assignment{}, fmt.Errorf("document numbering: check whether %s is used: %w", number, err)
		}
		if !used {
			result.Outcome, result.Number, result.Skipped = NumberAssigned, number, skipped
			return result, nil
		}
	}
	result.Outcome, result.Skipped = NumberingRuleCollision, MaxSkippedNumbers
	return result, nil
}

// effectiveTemplate 读 at 时刻生效的模板原文：租户在册的那一版，没有就是产品默认。
func (assigner *Assigner) effectiveTemplate(ctx context.Context, tenant string, code TypeCode, at time.Time) (string, RuleSource, error) {
	return effectiveTemplate(ctx, assigner.catalogue, assigner.book, tenant, code, at)
}

func effectiveTemplate(ctx context.Context, catalogue *Catalogue, book RuleBook, tenant string, code TypeCode, at time.Time) (string, RuleSource, error) {
	version, found, err := book.EffectiveRuleVersion(ctx, tenant, code, at)
	if err != nil {
		return "", "", fmt.Errorf("document numbering: read effective rule of %s: %w", code, err)
	}
	if !found {
		declaration, _ := catalogue.Declaration(code)
		return declaration.DefaultTemplate, SourceProductDefault, nil
	}
	return version.Template, SourceTenantRule, nil
}

// issuingTimeZone 定日期所用的时区：开立方交来的业务时区，没有就取租户此刻生效的单据编号时区；都没有答 found=false。
// 不拿 UTC 或进程时区顶——那是替租户定了时区（ADR-0355 决定七）。
func (assigner *Assigner) issuingTimeZone(ctx context.Context, request IssueRequest, at time.Time) (*time.Location, string, bool, error) {
	if request.IssuerTimeZone != "" {
		location, err := loadTimeZone(request.IssuerTimeZone)
		if err != nil {
			return nil, "", false, fmt.Errorf("%w: issuer business time zone: %w", ErrInvalidIssueRequest, err)
		}
		return location, request.IssuerTimeZone, true, nil
	}
	version, found, err := assigner.book.EffectiveTimeZoneVersion(ctx, request.Tenant, at)
	if err != nil {
		return nil, "", false, fmt.Errorf("document numbering: read numbering time zone: %w", err)
	}
	if !found {
		return nil, "", false, nil
	}
	location, err := loadTimeZone(version.TimeZone)
	if err != nil {
		return nil, "", false, fmt.Errorf("document numbering: registered numbering time zone no longer loads: %w", err)
	}
	return location, version.TimeZone, true, nil
}
