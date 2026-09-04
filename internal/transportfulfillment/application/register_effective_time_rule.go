package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// ErrUnexpectedEffectiveTimeRuleSave 说明规则目录交回了封闭集合以外的写入结果。
var ErrUnexpectedEffectiveTimeRuleSave = errors.New("transport fulfillment: unexpected effective time rule save outcome")

// EffectiveTimeRuleRegistrationOutcome 是轨迹源有效时间规则登记的应用结果代数（label-channel/19）。
//
// 首登与换版是同一个入口：一源一链，该源此刻没有版本就是首登，有当前版就是回指它的换版——登记方
// 只登「这一版正文」，不必先问目录里有没有版本。两格分开答，因为读的人续办不同：`已换版`带着被
// 回指的前版，此后按新版判的事实与此前按旧版判的事实要分得开。
type EffectiveTimeRuleRegistrationOutcome uint8

const (
	EffectiveTimeRuleRegistrationOutcomeInvalid EffectiveTimeRuleRegistrationOutcome = iota
	EffectiveTimeRuleRegistered
	EffectiveTimeRuleRevised
	EffectiveTimeRuleExistingVersion
	EffectiveTimeRuleContentConflict
	EffectiveTimeRuleRegistrationNotAccepted
	EffectiveTimeRuleRegistrationUndecided
)

func (outcome EffectiveTimeRuleRegistrationOutcome) String() string {
	switch outcome {
	case EffectiveTimeRuleRegistered:
		return "RULE_REGISTERED"
	case EffectiveTimeRuleRevised:
		return "RULE_REVISED"
	case EffectiveTimeRuleExistingVersion:
		return "EXISTING_VERSION"
	case EffectiveTimeRuleContentConflict:
		return "CONTENT_CONFLICT"
	case EffectiveTimeRuleRegistrationNotAccepted:
		return "INPUT_NOT_ACCEPTED"
	case EffectiveTimeRuleRegistrationUndecided:
		return "REGISTRATION_UNDECIDED"
	default:
		return ""
	}
}

// EffectiveTimeRuleRegistrationUndecidedReason 指名登记停在哪一步等谁。本编排只有一条缝，也就只有一格。
type EffectiveTimeRuleRegistrationUndecidedReason uint8

const (
	EffectiveTimeRuleRegistrationUndecidedReasonNone EffectiveTimeRuleRegistrationUndecidedReason = iota
	EffectiveTimeRuleRegistryUnavailable
)

func (reason EffectiveTimeRuleRegistrationUndecidedReason) String() string {
	switch reason {
	case EffectiveTimeRuleRegistryUnavailable:
		return "EFFECTIVE_TIME_RULE_REGISTRY_UNAVAILABLE"
	default:
		return ""
	}
}

// RegisterEffectiveTimeRuleCommand 携带一版规则的全部输入：所属源、版本，与正文三件（源时间字段的含义、
// 锚点、偏移）。词取 domain 的封闭集原词（EVENT_OCCURRENCE / SOURCE_PROCESSING；OCCURRED_AT / RECEIVED_AT），
// 词不在集合内即未受理。
//
// 版本由登记方指名而不是这里铸：同一版本重放要能被认出来（ADR-0031），铸出来的号做不到这一点。
type RegisterEffectiveTimeRuleCommand struct {
	TenantID          domain.TenantID
	Source            string
	Version           string
	SourceTimeMeaning string
	Anchor            string
	Offset            time.Duration
}

type RegisterEffectiveTimeRuleResult struct {
	outcome      EffectiveTimeRuleRegistrationOutcome
	reason       EffectiveTimeRuleRegistrationUndecidedReason
	record       ports.EffectiveTimeRuleRecord
	hasRecord    bool
	continuation string
}

func (result RegisterEffectiveTimeRuleResult) Outcome() EffectiveTimeRuleRegistrationOutcome {
	return result.outcome
}

// UndecidedReason 只在`未决`时非零。
func (result RegisterEffectiveTimeRuleResult) UndecidedReason() EffectiveTimeRuleRegistrationUndecidedReason {
	return result.reason
}

// Record 在`已登记`、`已换版`、`已有版本`时给出那一版；`内容冲突`时给出撞上的既有版本。
func (result RegisterEffectiveTimeRuleResult) Record() (ports.EffectiveTimeRuleRecord, bool) {
	return result.record, result.hasRecord
}

func (result RegisterEffectiveTimeRuleResult) ContinuationReference() string {
	return result.continuation
}

type RegisterEffectiveTimeRuleDeps struct {
	Rules ports.EffectiveTimeRuleRegistry
	Clock ports.Clock
}

// RegisterEffectiveTimeRuleHandler 是有效时间规则目录的写侧编排。没有意图交付：规则是本上下文自己的
// 实例参数，收编执行器按需来问（ports.EffectiveTimeRules），没有谁要在它变动时被通知。
//
// **登记新版不回填此前留为待判断的事实**（票 19 第 4 问）：规则形成的是「按该源规则判」的判断，事实上
// 记的是「按哪一版判的」；对存量待判断事实成批套用新登的规则，等于让一次登记替所有者对一批他没看过的
// 事实各作一次判断，而逐条走 JudgeEffectiveTimeHandler 的显式判断（票 21 的在线面）恰好保留了「谁、就
// 哪一条、给了什么」。要批量重判，另立入口另裁。
type RegisterEffectiveTimeRuleHandler struct {
	deps RegisterEffectiveTimeRuleDeps
}

func NewRegisterEffectiveTimeRuleHandler(deps RegisterEffectiveTimeRuleDeps) *RegisterEffectiveTimeRuleHandler {
	return &RegisterEffectiveTimeRuleHandler{deps: deps}
}

// Register 登一版规则：受理（正文三件与身份的完备性由领域构造门把门）→ 幂等按（租户+源+版本）分
// 重放/冲突 → 该源有当前版则新版回指它，没有则立首版 → 原子提交。
func (handler *RegisterEffectiveTimeRuleHandler) Register(
	ctx context.Context,
	command RegisterEffectiveTimeRuleCommand,
) (RegisterEffectiveTimeRuleResult, error) {
	source, version, content, err := ruleShapeFrom(command)
	if err != nil {
		return RegisterEffectiveTimeRuleResult{outcome: EffectiveTimeRuleRegistrationNotAccepted}, nil
	}
	key := ports.EffectiveTimeRuleKey{TenantID: command.TenantID, Source: source, Version: version}

	existing, found, err := handler.deps.Rules.FindByKey(ctx, key)
	if err != nil {
		return ruleRegistryUndecided(key), nil
	}
	if found {
		return ruleReplayOrConflict(existing, key, content), nil
	}

	current, found, err := handler.deps.Rules.FindCurrent(ctx, command.TenantID, source)
	if err != nil {
		return ruleRegistryUndecided(key), nil
	}

	var rule domain.EffectiveTimeRule
	outcome := EffectiveTimeRuleRegistered
	if found {
		rule, err = current.Rule.Revise(version, content)
		outcome = EffectiveTimeRuleRevised
	} else {
		rule, err = domain.RegisterEffectiveTimeRule(domain.EffectiveTimeRuleSpec{
			TenantID: command.TenantID,
			Source:   source,
			Version:  version,
			Content:  content,
		})
	}
	if err != nil {
		return RegisterEffectiveTimeRuleResult{outcome: EffectiveTimeRuleRegistrationNotAccepted}, nil
	}

	return handler.commit(ctx, ports.EffectiveTimeRuleRecord{
		Key:        key,
		Rule:       rule,
		RecordedAt: handler.deps.Clock.Now(),
	}, outcome)
}

// commit 提交记录；并发下另一方先提交时读回赢家按内容分重放/冲突，读不回自己那一版（撞的是「一源一
// 首版」或「一版一后继」那两道索引）就是未决——重放会读到对方那一版成为当前版，再答一个正当的换版。
func (handler *RegisterEffectiveTimeRuleHandler) commit(
	ctx context.Context,
	record ports.EffectiveTimeRuleRecord,
	outcome EffectiveTimeRuleRegistrationOutcome,
) (RegisterEffectiveTimeRuleResult, error) {
	saved, err := handler.deps.Rules.Save(ctx, record)
	if err != nil {
		return ruleRegistryUndecided(record.Key), nil
	}
	switch saved {
	case ports.EffectiveTimeRuleSaved:
		return RegisterEffectiveTimeRuleResult{outcome: outcome, record: record, hasRecord: true}, nil
	case ports.EffectiveTimeRuleAlreadyRegistered:
		winner, found, err := handler.deps.Rules.FindByKey(ctx, record.Key)
		if err != nil || !found {
			return ruleRegistryUndecided(record.Key), nil
		}
		return ruleReplayOrConflict(winner, record.Key, record.Rule.Content()), nil
	default:
		return RegisterEffectiveTimeRuleResult{}, fmt.Errorf("%w: %d", ErrUnexpectedEffectiveTimeRuleSave, saved)
	}
}

// ruleReplayOrConflict 按正文比对同键的两个版本（ADR-0031）：正文相同是重放，不同是冲突——冲突保留原
// 版本，改它登一个新版本号，不按最后到达顶替。只比正文不比前版：前版是目录按当时的当前版派生的，
// 同一份正文重放时它不在登记方手上。
func ruleReplayOrConflict(
	existing ports.EffectiveTimeRuleRecord,
	key ports.EffectiveTimeRuleKey,
	content domain.EffectiveTimeRuleContent,
) RegisterEffectiveTimeRuleResult {
	if existing.Key == key && existing.Rule.Content() == content {
		return RegisterEffectiveTimeRuleResult{outcome: EffectiveTimeRuleExistingVersion, record: existing, hasRecord: true}
	}
	return RegisterEffectiveTimeRuleResult{outcome: EffectiveTimeRuleContentConflict, record: existing, hasRecord: true}
}

func ruleShapeFrom(command RegisterEffectiveTimeRuleCommand) (
	domain.TrackingSourceReference,
	domain.EffectiveTimeRuleVersion,
	domain.EffectiveTimeRuleContent,
	error,
) {
	none := domain.TrackingSourceReference{}
	noVersion := domain.EffectiveTimeRuleVersion{}
	if strings.TrimSpace(command.TenantID.String()) == "" {
		return none, noVersion, domain.EffectiveTimeRuleContent{}, errors.New("blank tenant")
	}
	source, err := domain.NewTrackingSourceReference(command.Source)
	if err != nil {
		return none, noVersion, domain.EffectiveTimeRuleContent{}, err
	}
	version, err := domain.NewEffectiveTimeRuleVersion(command.Version)
	if err != nil {
		return none, noVersion, domain.EffectiveTimeRuleContent{}, err
	}
	meaning, err := domain.ParseSourceTimeMeaning(strings.TrimSpace(command.SourceTimeMeaning))
	if err != nil {
		return none, noVersion, domain.EffectiveTimeRuleContent{}, err
	}
	anchor, err := domain.ParseEffectiveTimeAnchor(strings.TrimSpace(command.Anchor))
	if err != nil {
		return none, noVersion, domain.EffectiveTimeRuleContent{}, err
	}
	return source, version, domain.EffectiveTimeRuleContent{
		SourceTimeMeaning: meaning,
		Anchor:            anchor,
		Offset:            command.Offset,
	}, nil
}

func ruleRegistryUndecided(key ports.EffectiveTimeRuleKey) RegisterEffectiveTimeRuleResult {
	digest := sha256.Sum256([]byte(strings.Join([]string{
		EffectiveTimeRuleRegistryUnavailable.String(), key.TenantID.String(), key.Source.String(), key.Version.String(),
	}, "\x00")))
	return RegisterEffectiveTimeRuleResult{
		outcome:      EffectiveTimeRuleRegistrationUndecided,
		reason:       EffectiveTimeRuleRegistryUnavailable,
		continuation: "CONT-" + hex.EncodeToString(digest[:8]),
	}
}
