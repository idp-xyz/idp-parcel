// 发布表单逐格问题的中文说法。
//
// 服务端按格交回 `{field, problem, code, params}`：`problem` 是领域错误或解析错误的原话（`err.Error()`），例如
// `party commercial: blank value: commercial object ID`、`须为 RFC 3339 时刻：parsing time …`，配置员读不懂、照原话也改不对；
// `code` 与 `params` 是稳定的问题码与参数（ADR-0316）。认得的码直接按码查话术，不依赖原话的措辞；码缺席、是兜底的 `INVALID`
// 或前端不认得的新码时，按原话的形状译（下面的 `rules`），再认不出就给通用的一句，不会显错话。原话不丢，交给页面收进可展开的一格。
// `rules` 只服务没有码的问题，随服务端把更多格贴上码，它能认的越来越少。

export interface FieldProblemText {
  /** 给配置员看的一句中文。 */
  text: string;
  /** 服务端原话；与 `text` 相同（原话本就是中文白话）时为 null，页面不再显第二遍。 */
  original: string | null;
}

const generic = '填写的内容不合要求';

interface Rule {
  pattern: RegExp;
  text: (match: RegExpMatchArray) => string;
}

// 主语里带英文键名时不拿它当主语，键名对配置员没有信息。
function subjectOr(subject: string | undefined, fallback: string): string {
  const trimmed = (subject ?? '').replace(/（[^）]*）/g, '').trim();
  return trimmed === '' || /[A-Za-z]/.test(trimmed) ? fallback : trimmed;
}

// 次序要紧：专门的句子排在通用形状前面，`party commercial:` 兜底排最后。
const rules: Rule[] = [
  { pattern: /^party commercial: blank value/, text: () => '必填，不能留空' },
  // 时刻格留空时服务端答的也是解析错误（解析空串），对配置员那是「没填」，不是「格式不对」。
  { pattern: /^须为 RFC 3339 时刻：parsing time "" /, text: () => '必填，不能留空' },
  { pattern: /^须为 RFC 3339 时刻/, text: () => '时间格式不对：请填完整时刻，例如 2026-10-01T00:00:00+08:00' },
  { pattern: /^金额额度不带基数/, text: () => '按金额设额度时，不要选比例的基数' },
  { pattern: /^比例额度须声明其基数/, text: () => '按比例设额度时，必须选比例的基数' },
  { pattern: /^时长须为正整数天/, text: () => '天数必须是正整数' },
  { pattern: /^判断顺序须为从 1 起的正整数/, text: () => '判断顺序必须是从 1 开始的正整数' },
  { pattern: /^计不计费要协议自己说/, text: () => '请选择是否计费' },
  { pattern: /^空清单或同一原因两条/, text: () => '至少填一条，同一原因不能出现两次' },
  { pattern: /^(.*?)须恰一格在场/, text: (m) => `${subjectOr(m[1], '这几格')}只能填其中一项` },
  { pattern: /^集合外的(\S+?)\s/, text: (m) => `${m[1]}不在可选范围内` },
  { pattern: /^(.*?)须在场/, text: (m) => `${subjectOr(m[1], '这一格')}必须填写` },
  { pattern: /^party commercial: /, text: () => generic },
];

// 按码给的话术。每个码的参数键是服务端契约的一部分（Go publication_problem_codes.go 各常量的注释）；缺参数时用不带主语的说法。
const codeTexts: Record<string, (params: Record<string, string>) => string> = {
  REQUIRED: (params) => (params.subject ? `${params.subject}必须填写` : '必填，不能留空'),
  NOT_IN_SET: (params) => `${params.what ?? '这一格'}不在可选范围内`,
  TIME_FORMAT: () => '时间格式不对：请填完整时刻，例如 2026-10-01T00:00:00+08:00',
  EXACTLY_ONE: (params) => `${params.subject ?? '这几格'}只能填其中一项`,
  RATIO_BASE_NOT_ALLOWED: () => '按金额设额度时，不要选比例的基数',
  RATIO_BASE_REQUIRED: () => '按比例设额度时，必须选比例的基数',
  DAYS_NOT_POSITIVE: () => '天数必须是正整数',
  ORDER_NOT_POSITIVE: () => '判断顺序必须是从 1 开始的正整数',
  CHARGED_UNDECLARED: () => '请选择是否计费',
  LIST_EMPTY_OR_DUPLICATE: () => '至少填一条，同一原因不能出现两次',
};

// 问题在页面里以「行」的形式（一个字符串）流过各个表单组件，那些组件只认字符串。带码的问题在入口处就按码译好，并把原话
// 用这个分隔符接在后面一起带过去，由 fieldProblemText 在显示那一刻拆开；原话仍收进可展开的一格。分隔符是 Unicode 的
// 「单元分隔符」符号，服务端原话里自己不写它；只有没带码、又把用户输入原样回显的原话才可能夹带它，那种行会按第一个分隔符被
// 错拆，条件极苛刻，且只影响那一行的显示。
const codedLineSeparator = '\u241F';

/**
 * 一条服务端问题在页面里的「行」。认得的码按码译成中文、原话跟在后面；没有码、是 `INVALID` 或不认得的码时就是原话本身，
 * 之后由 fieldProblemText 按原话的形状译。
 */
export function codedProblemLine(record: { problem: string; code?: string; params?: Record<string, string> }): string {
  const translate = record.code === undefined ? undefined : codeTexts[record.code];
  if (translate === undefined) return record.problem;
  const text = translate(record.params ?? {});
  return text === record.problem ? text : `${text}${codedLineSeparator}${record.problem}`;
}

export function fieldProblemText(raw: string): FieldProblemText {
  const separatorAt = raw.indexOf(codedLineSeparator);
  if (separatorAt >= 0) {
    const original = raw.slice(separatorAt + 1);
    return { text: raw.slice(0, separatorAt), original: original === '' ? null : original };
  }
  for (const rule of rules) {
    const match = raw.match(rule.pattern);
    if (match) {
      const text = rule.text(match);
      return { text, original: text === raw ? null : raw };
    }
  }
  // 认不出的原话：看得出是服务端或解析器的原话（领域错误前缀、Go 的解析错误）才给通用一句、原话收起；其余照原样显。
  // 同一个显示件也显前端本地校验的问题，那些本来就是给人读的中文，可能夹着「P7D」这类例子，不能被当成原话吞掉。
  if (serverShaped.test(raw)) return { text: generic, original: raw };
  return { text: raw, original: null };
}

const serverShaped = /^party commercial:|parsing time|^json:|cannot unmarshal|invalid character|unknown field/;

/**
 * 清单逐项的问题：服务端点名到 `methods[0]`、`materials[2]` 这一级，页面上清单是一块多行文本，
 * 所以把下标说成「第 n 项」放在前面；原话连同路径收进原话那一格，排查时还对得上。
 */
export function itemProblemText(relativePath: string, raw: string): FieldProblemText {
  const translated = fieldProblemText(raw);
  const index = /\[(\d+)\]/.exec(relativePath);
  const prefix = index ? `第 ${Number(index[1]) + 1} 项：` : '';
  return {
    text: `${prefix}${translated.text}`,
    original: translated.original === null ? null : `${relativePath}：${translated.original}`,
  };
}
