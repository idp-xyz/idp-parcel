package docnumber

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"go.idp.xyz/idp-platform-go/numbering"
)

// 模板检查的拒因。登记口按它们给出各自的答复码，页面据此说人话；默认模板撞上它们是装配期的编程错误。
var (
	// ErrTemplateUnparseable：平台 Parse 不收（未知令牌等），或模板为空。
	ErrTemplateUnparseable = errors.New("document numbering: template cannot be parsed")
	// ErrTemplateWithoutSequence：模板没有流水令牌（ADR-0355 决定五：只有随机数或没有流水的模板拒登）。
	ErrTemplateWithoutSequence = errors.New("document numbering: template has no sequence token")
	// ErrTemplateNeedsUnsuppliedValue：模板用了取号编排给不出取值的令牌（上下文值或校验位，见 checkTemplate）。
	ErrTemplateNeedsUnsuppliedValue = errors.New("document numbering: template uses a value the assignment cannot supply")
	// ErrTemplateUnrenderable：模板渲染不出一个可用的号——渲染出错、过长，或含空白与控制字符。
	ErrTemplateUnrenderable = errors.New("document numbering: template does not render a usable number")
)

// maxTemplateLength 与 maxNumberLength 是产品策略的上限：单据号要进条码、表格主列与客户系统的录入框，默认
// 模板最长的一份渲染出十几个字符；四十个字符已是它的两倍多，再长多半是模板写错了（例如 `{seq:100}`）。上限挡的
// 是错写，不是租户的格式偏好。
const (
	maxTemplateLength = 64
	maxNumberLength   = 40
)

// probeInstant 是检查模板时的渲染时刻。取哪一刻都行，只要固定：检查判的是模板用了哪些令牌，不是某天的号。
var probeInstant = time.Date(2026, time.November, 2, 10, 30, 45, 0, time.UTC)

type checkedTemplate struct {
	template *numbering.Template
	// scrambled 为真表示模板用了加扰流水（`{seqx}` 一族），渲染要租户的加扰密钥。
	scrambled bool
}

// checkTemplate 判一份模板能不能交给取号编排：平台 Parse 收它、含流水令牌、只用编排给得出取值的令牌、渲染出的
// 号可用。
//
// 用了哪些令牌只经平台公开的 Render 观察，不在本仓另写一份令牌文法——文法只在平台库一处（ADR-0355 决定四），
// 本仓再写一份就会在平台加令牌那天悄悄分叉。观察法：同一份输入只改一项再渲染一次，输出变了就是用到了那一项。
//
// 编排给不出取值、因而拒登的两类令牌：
//   - 上下文值 `{site}` `{owner}` `{region}` `{le}`：规则只按（租户，类型）登，开立时不带这些值，渲染出来是空串，
//     号就少了一段而没人察觉。`{type}` 例外，编排照平台入口把类型代码交进去。
//   - 校验位 `{check}`：规则版本只有模板与生效时刻（ADR-0355 决定五），不带校验位算法，渲染出来同样是空串。
func checkTemplate(code TypeCode, raw string) (checkedTemplate, error) {
	if strings.TrimSpace(raw) == "" {
		return checkedTemplate{}, fmt.Errorf("%w: template is blank", ErrTemplateUnparseable)
	}
	if utf8.RuneCountInString(raw) > maxTemplateLength {
		return checkedTemplate{}, fmt.Errorf("%w: template is longer than %d characters", ErrTemplateUnrenderable, maxTemplateLength)
	}
	template, err := numbering.Parse(raw)
	if err != nil {
		return checkedTemplate{}, fmt.Errorf("%w: %v", ErrTemplateUnparseable, err)
	}
	if !template.HasSequence() {
		return checkedTemplate{}, ErrTemplateWithoutSequence
	}

	base := probeInput(code)
	plain, err := template.Render(base)
	if err != nil {
		return checkedTemplate{}, fmt.Errorf("%w: %v", ErrTemplateUnrenderable, err)
	}
	if err := usableNumber(plain); err != nil {
		return checkedTemplate{}, err
	}

	// 哨兵不用 NUL：平台渲染 `{check}` 时拿 NUL 作占位、渲染完把 NUL 全换掉，NUL 哨兵会被一并抹掉。
	withContext := base
	withContext.Context = map[string]string{"type": string(code), "site": "\x01", "owner": "\x01", "region": "\x01", "le": "\x01"}
	if rendered, err := template.Render(withContext); err != nil || rendered != plain {
		return checkedTemplate{}, fmt.Errorf("%w: context values other than {type} are not supplied at assignment", ErrTemplateNeedsUnsuppliedValue)
	}
	withCheck := base
	withCheck.CheckAlgo = numbering.CheckMod10
	if rendered, err := template.Render(withCheck); err != nil || rendered != plain {
		return checkedTemplate{}, fmt.Errorf("%w: a rule version carries no check digit algorithm", ErrTemplateNeedsUnsuppliedValue)
	}

	// 加扰：换几把键各渲染一次，任一次与原输出不同就是用了加扰。只换一把会在窄域上碰巧相同（`{seqx:1}` 的域只有
	// 十个值），所以多换几把；不用加扰的模板对键完全不敏感，换多少把都不变。
	scrambled := false
	for _, key := range []string{"probe-1", "probe-2", "probe-3", "probe-4", "probe-5", "probe-6", "probe-7", "probe-8"} {
		withKey := base
		withKey.ScrambleKey = key
		rendered, err := template.Render(withKey)
		if err != nil {
			return checkedTemplate{}, fmt.Errorf("%w: %v", ErrTemplateUnrenderable, err)
		}
		if rendered != plain {
			scrambled = true
			break
		}
	}
	return checkedTemplate{template: template, scrambled: scrambled}, nil
}

func probeInput(code TypeCode) numbering.RenderInput {
	return numbering.RenderInput{
		Seq:         1,
		At:          probeInstant,
		Context:     map[string]string{"type": string(code)},
		ScrambleKey: "probe-0",
		RandDigits:  zeroDigits,
	}
}

// zeroDigits 让 `{rand}` 在检查与预览里确定：随机段只示位数，不该让两次渲染因它而不同。
func zeroDigits(width int) (string, error) {
	return strings.Repeat("0", width), nil
}

func usableNumber(number string) error {
	if number == "" {
		return fmt.Errorf("%w: renders an empty number", ErrTemplateUnrenderable)
	}
	if utf8.RuneCountInString(number) > maxNumberLength {
		return fmt.Errorf("%w: renders longer than %d characters", ErrTemplateUnrenderable, maxNumberLength)
	}
	for _, r := range number {
		if unicode.IsSpace(r) || unicode.IsControl(r) || r == utf8.RuneError {
			return fmt.Errorf("%w: renders whitespace or control characters", ErrTemplateUnrenderable)
		}
	}
	return nil
}

// render 渲染一个号。加扰键照平台入口 `Assigner.Assign` 的拼法——密钥、租户、类型代码以竖线相连（ADR-0355 越权
// 风险点 1）：拼法不同，同一模板在本仓与平台入口会出不同的加扰号，日后改用平台入口时号就对不上了。平台在渲染时
// 再把周期键接上，本仓不另接。
func render(template *numbering.Template, code TypeCode, tenant, secret string, seq int64, at time.Time, randDigits func(int) (string, error)) (string, error) {
	return template.Render(numbering.RenderInput{
		Seq:         seq,
		At:          at,
		Context:     map[string]string{"type": string(code)},
		ScrambleKey: secret + "|" + tenant + "|" + string(code),
		RandDigits:  randDigits,
	})
}

// ResetPeriod 是模板推出的流水重置周期。
type ResetPeriod string

const (
	ResetDaily      ResetPeriod = "DAILY"
	ResetMonthly    ResetPeriod = "MONTHLY"
	ResetQuarterly  ResetPeriod = "QUARTERLY"
	ResetYearly     ResetPeriod = "YEARLY"
	ResetContinuous ResetPeriod = "CONTINUOUS"
)

func resetPeriodOf(template *numbering.Template) ResetPeriod {
	switch template.Granularity() {
	case numbering.Daily:
		return ResetDaily
	case numbering.Monthly:
		return ResetMonthly
	case numbering.Quarterly:
		return ResetQuarterly
	case numbering.Yearly:
		return ResetYearly
	default:
		return ResetContinuous
	}
}
