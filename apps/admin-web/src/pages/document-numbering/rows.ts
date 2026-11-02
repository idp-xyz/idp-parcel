// 「设置 → 单据编号」的行模型、格式读法、版本次序与答复话术（ADR-0355 决定五至八）。纯逻辑，node:test 钉着；页面只负责摆。
//
// 格式（模板）的合法与否只由服务端判（预览口与登记口，文法只在平台库一处，ADR-0355 决定四）。这里把模板拆成「字面 + 部件」
// 只为了**读给人听**：`P{yyyy}{mm}{dd}{seq:6}` 读成「P · 年 月 日 · 6 位流水」。拆不认识的部件原样露出，不判它错——
// 判错是预览口的事，前端另判一份就会在平台加部件那天与服务端说两样话。

import { operatorRefusalMessage, isOperatorRefusal } from '../../auth/access-refusal';
import { shortPrincipalRef } from '../../components/principal-ref';
import type { TechnicalRow } from '../../components/TechnicalDetails';
import {
  documentTypeWords,
  genericIssuerWord,
  issuerWordsByType,
  registrationRejectionWords,
  resetPeriodWords,
  templateTokenWords,
} from '../../vocabulary/document-numbering';
import { matchesSearch } from '../party/list-order';
import { formatInstant } from '../moment';
import type {
  ApiResult,
  DocumentNumberingReadBody,
  DocumentTypeRecord,
  PreviewBody,
  PreviewOverlapRecord,
  RuleRegistrationBody,
  RuleSource,
  RuleVersionRecord,
  TimeZoneRegistrationBody,
  TimeZoneVersionRecord,
} from './api';

// —— 单据类型的叫法 ——

export interface DocumentTypeName {
  name: string;
  numberName: string;
  /** 词典里有没有这一类。没有时 name 是类型代码原样，页面另说一句，不冒充中文叫法。 */
  known: boolean;
}

export function documentTypeNameOf(code: string): DocumentTypeName {
  const words = documentTypeWords[code];
  return words ? { ...words, known: true } : { name: code, numberName: '单据号', known: false };
}

// —— 格式的读法 ——

export type TemplateSegment =
  | { kind: 'literal'; text: string }
  | { kind: 'token'; raw: string; label: string; date: boolean; known: boolean };

// 与平台库的部件写法同形（`{名}` 或 `{名:位数}`）；只用来切分，不用来判合法。
const tokenPattern = /\{([a-z0-9]+)(?::(\d+))?\}/g;

const dateTokens = new Set(['yyyy', 'yy', 'mm', 'dd', 'q', 'hh', 'mi', 'ss', 'ms', 'fy', 'fyyy']);

// 平台库在没写位数时的取法：加扰流水 6 位、随机数 4 位；普通流水不补零。
const defaultWidths: Record<string, number> = { seqx: 6, seqx36: 6, rand: 4 };

function tokenLabel(name: string, width: number | undefined): { label: string; known: boolean } {
  const word = templateTokenWords[name];
  if (word === undefined) return { label: `{${name}}`, known: false };
  if (name === 'yy') return { label: '年（两位）', known: true };
  if (name === 'seq' || name === 'seq36') {
    return { label: width ? `${width} 位${word}` : word, known: true };
  }
  const effective = width ?? defaultWidths[name];
  if (effective !== undefined) return { label: `${effective} 位${word}`, known: true };
  return { label: word, known: true };
}

/** 把一份模板拆成字面与部件。空模板答空数组。 */
export function templateSegmentsOf(template: string): TemplateSegment[] {
  const segments: TemplateSegment[] = [];
  let cursor = 0;
  for (const match of template.matchAll(tokenPattern)) {
    const index = match.index ?? 0;
    if (index > cursor) segments.push({ kind: 'literal', text: template.slice(cursor, index) });
    const name = match[1];
    const width = match[2] !== undefined ? Number(match[2]) : undefined;
    const { label, known } = tokenLabel(name, width);
    segments.push({ kind: 'token', raw: match[0], label, date: dateTokens.has(name), known });
    cursor = index + match[0].length;
  }
  if (cursor < template.length) segments.push({ kind: 'literal', text: template.slice(cursor) });
  return segments;
}

/** 一句话读出格式：「P · 年 月 日 · 6 位流水」。字面原样，相邻的日期部件并成一组，组间用间隔点。 */
export function templateReading(template: string): string {
  const groups: string[] = [];
  let lastWasDate = false;
  for (const segment of templateSegmentsOf(template)) {
    if (segment.kind === 'literal') {
      groups.push(segment.text);
      lastWasDate = false;
    } else if (segment.date && lastWasDate) {
      groups[groups.length - 1] += ` ${segment.label}`;
    } else {
      groups.push(segment.label);
      lastWasDate = segment.date;
    }
  }
  return groups.join(' · ');
}

// —— 时区与登记人 ——

/**
 * 「中国标准时间」；拿不到中文名时原样交回 IANA 名（时区是租户取值，产品不另建城市表）。名字按 at 那一刻取：同一个区
 * 冬夏叫法不同，几十年前的叫法也可能对不上今天的偏移，所以不拿一个固定的老时刻去问。ICU 给不出中文名时答的是
 * 「GMT-11:00」一类偏移串，那不是名字，同样交回 IANA 名。
 */
export function zoneWords(zone: string | undefined, at: Date = new Date()): string {
  if (!zone) return '—';
  try {
    const name = new Intl.DateTimeFormat('zh-CN', { timeZone: zone, timeZoneName: 'long' })
      .formatToParts(at)
      .find((part) => part.type === 'timeZoneName')?.value;
    if (!name || name === zone || /^(GMT|UTC)([+-]|$)/.test(name)) return zone;
    return name;
  } catch {
    return zone;
  }
}

/** 登记人：受控登记命令说「受控登记命令」，操作者取短名（全文由调用处放进 title）。 */
export function registrantWords(ref: string): string {
  if (ref.startsWith('CONTROLLED-CLI/')) return '受控登记命令';
  return shortPrincipalRef(ref.startsWith('OPERATOR/') ? ref.slice('OPERATOR/'.length) : ref);
}

// —— 行模型 ——

export interface DateZoneCell {
  /** 列上的一句。 */
  short: string;
  /** 详情里的完整一句。 */
  sentence: string;
  /** 这一类此刻开立会不会不带号（单据编号时区未配置、又没有开立方业务时区可用）。 */
  withoutNumber: boolean;
}

export function dateZoneOf(record: DocumentTypeRecord): DateZoneCell {
  const zone = record.dateTimeZone;
  const configured = zone.numberingTimeZoneStatus === 'CONFIGURED';
  const tenantZone = configured ? zoneWords(zone.numberingTimeZone) : '';
  if (zone.source === 'ISSUER_BUSINESS_TIME_ZONE') {
    const issuer = issuerWordsByType[record.documentType] ?? genericIssuerWord;
    return configured
      ? {
          short: `${issuer}的业务时区`,
          sentence: `按开立这张单的${issuer}登记的业务时区出日期；${issuer}没登业务时区时，按单据编号时区（${tenantZone}）。`,
          withoutNumber: false,
        }
      : {
          short: `${issuer}的业务时区`,
          sentence: `按开立这张单的${issuer}登记的业务时区出日期；${issuer}没登业务时区、单据编号时区又未配置时，开立照常但不带单据号。`,
          withoutNumber: false,
        };
  }
  return configured
    ? { short: tenantZone, sentence: `按本公司的单据编号时区（${tenantZone}）出日期。`, withoutNumber: false }
    : {
        short: '单据编号时区未配置',
        sentence: '这一类单据没有开立方业务时区，日期按本公司的单据编号时区出；单据编号时区未配置，此刻开立照常但不带单据号，事后也不补。',
        withoutNumber: true,
      };
}

export interface ResetWords {
  short: string;
  sentence: string;
}

export function resetWordsOf(code: string | undefined): ResetWords {
  if (code === undefined || code === '') return { short: '—', sentence: '' };
  return resetPeriodWords[code] ?? { short: code, sentence: '' };
}

export interface RuleRow {
  code: string;
  type: DocumentTypeName;
  source: RuleSource;
  template: string;
  defaultTemplate: string;
  effectiveFrom?: string;
  reset: ResetWords;
  dateZone: DateZoneCell;
  sample?: string;
  record: DocumentTypeRecord;
}

export function ruleRowsOf(body: DocumentNumberingReadBody): RuleRow[] {
  return body.documentTypes.map((record) => ({
    code: record.documentType,
    type: documentTypeNameOf(record.documentType),
    source: record.source,
    template: record.template,
    defaultTemplate: record.defaultTemplate,
    effectiveFrom: record.effectiveFrom,
    reset: resetWordsOf(record.resetPeriod),
    dateZone: dateZoneOf(record),
    sample: record.sample,
    record,
  }));
}

export type SourceFilter = 'ALL' | RuleSource;

export function filterRuleRows(rows: readonly RuleRow[], filter: { search: string; source: SourceFilter }): RuleRow[] {
  return rows.filter(
    (row) =>
      (filter.source === 'ALL' || row.source === filter.source) &&
      matchesSearch(filter.search, [row.type.name, row.type.numberName, row.sample ?? '', row.template, templateReading(row.template), row.code]),
  );
}

export interface RuleCounts {
  total: number;
  tenantRule: number;
  productDefault: number;
  withoutNumber: number;
}

export function countRuleRows(rows: readonly RuleRow[]): RuleCounts {
  return {
    total: rows.length,
    tenantRule: rows.filter((row) => row.source === 'TENANT_RULE').length,
    productDefault: rows.filter((row) => row.source === 'PRODUCT_DEFAULT').length,
    withoutNumber: rows.filter((row) => row.dateZone.withoutNumber).length,
  };
}

export function ruleCountSummary(total: number, shown: number): string {
  return shown === total ? `共 ${total} 类单据` : `显示 ${shown} / 共 ${total} 类单据`;
}

// —— 历次版本 ——

/** 一版此刻的处境：词取用语词典总表（生效中 / 未生效 / 已替代）。 */
export type VersionStanding = 'IN_FORCE' | 'NOT_YET_EFFECTIVE' | 'SUPERSEDED';

export interface RuleHistoryEntry {
  key: string;
  /** 产品默认那一行没有生效时刻与登记人。 */
  productDefault: boolean;
  template: string;
  effectiveFrom?: string;
  registeredBy?: string;
  registeredAt?: string;
  standing: VersionStanding;
}

/** 与服务端同一判法：生效时刻不晚于 at 的版本里取最后一版（版本按生效时刻升序，生效时刻两两不同）。 */
function inForceIndex(effectiveFroms: readonly string[], at: string): number {
  const now = Date.parse(at);
  let found = -1;
  effectiveFroms.forEach((value, index) => {
    if (Date.parse(value) <= now) found = index;
  });
  return found;
}

function standingOf(index: number, inForce: number, effectiveFrom: string, at: string): VersionStanding {
  if (Date.parse(effectiveFrom) > Date.parse(at)) return 'NOT_YET_EFFECTIVE';
  return index === inForce ? 'IN_FORCE' : 'SUPERSEDED';
}

/**
 * 历次规则版本，新的在上，最底下垫一行产品默认：产品默认是本公司没登任何版本、或登的版本都还没到生效时刻时用的那一份。
 * 产品默认的处境按读口答的来源定，不自己再算一遍。
 */
export function ruleHistoryOf(record: DocumentTypeRecord, at: string): RuleHistoryEntry[] {
  const versions = [...record.versions].sort((left, right) => Date.parse(left.effectiveFrom) - Date.parse(right.effectiveFrom));
  const inForce = inForceIndex(
    versions.map((version) => version.effectiveFrom),
    at,
  );
  const entries: RuleHistoryEntry[] = versions.map((version: RuleVersionRecord, index) => ({
    key: `${version.effectiveFrom}|${version.template}`,
    productDefault: false,
    template: version.template,
    effectiveFrom: version.effectiveFrom,
    registeredBy: version.registeredBy,
    registeredAt: version.registeredAt,
    standing: standingOf(index, inForce, version.effectiveFrom, at),
  }));
  entries.reverse();
  entries.push({
    key: 'PRODUCT_DEFAULT',
    productDefault: true,
    template: record.defaultTemplate,
    standing: record.source === 'PRODUCT_DEFAULT' ? 'IN_FORCE' : 'SUPERSEDED',
  });
  return entries;
}

export interface TimeZoneHistoryEntry {
  key: string;
  timeZone: string;
  effectiveFrom: string;
  registeredBy: string;
  registeredAt: string;
  standing: VersionStanding;
}

export function timeZoneHistoryOf(versions: readonly TimeZoneVersionRecord[], at: string): TimeZoneHistoryEntry[] {
  const sorted = [...versions].sort((left, right) => Date.parse(left.effectiveFrom) - Date.parse(right.effectiveFrom));
  const inForce = inForceIndex(
    sorted.map((version) => version.effectiveFrom),
    at,
  );
  return sorted
    .map((version, index) => ({
      key: `${version.effectiveFrom}|${version.timeZone}`,
      timeZone: version.timeZone,
      effectiveFrom: version.effectiveFrom,
      registeredBy: version.registeredBy,
      registeredAt: version.registeredAt,
      standing: standingOf(index, inForce, version.effectiveFrom, at),
    }))
    .reverse();
}

/** 单据编号时区此刻的一句（页头与对话框共用）。未配置时若已有将来才生效的一版，如实说出来。 */
export interface NumberingZoneState {
  configured: boolean;
  /** 已配置时是「中国标准时间」。 */
  current?: string;
  currentZone?: string;
  /** 未配置但已登了将来才生效的一版：那一版的时区与生效时刻。 */
  upcoming?: { zone: string; effectiveFrom: string };
}

export function numberingZoneStateOf(body: DocumentNumberingReadBody): NumberingZoneState {
  const zone = body.numberingTimeZone;
  if (zone.status === 'CONFIGURED' && zone.timeZone) {
    return { configured: true, current: zoneWords(zone.timeZone), currentZone: zone.timeZone };
  }
  const upcoming = timeZoneHistoryOf(body.numberingTimeZoneVersions, body.at)
    .filter((entry) => entry.standing === 'NOT_YET_EFFECTIVE')
    .pop();
  return upcoming ? { configured: false, upcoming: { zone: upcoming.timeZone, effectiveFrom: upcoming.effectiveFrom } } : { configured: false };
}

export const numberingZoneMissingSentence =
  '单据编号时区未配置——开立方没有业务时区的单据（如运单、异常案件）暂不带单据号。';

// —— 新格式表单 ——

/** 把部件写法插进格式里光标所在处（有选区就替换选区），交回新格式与插入后光标的位置。 */
export function insertToken(template: string, selectionStart: number, selectionEnd: number, token: string): { text: string; caret: number } {
  const start = Math.max(0, Math.min(selectionStart, template.length));
  const end = Math.max(start, Math.min(selectionEnd, template.length));
  return { text: template.slice(0, start) + token + template.slice(end), caret: start + token.length };
}

/** 部件说明里带 N 的写法按选好的位数落成实写法：`{seq:N}` 选 5 位 → `{seq:5}`。 */
export function tokenWithDigits(token: string, digits: number): string {
  return token.replace(':N}', `:${digits}}`);
}

export function templateProblem(template: string, currentTemplate: string): string | null {
  if (template.trim() === '') return '填一个格式';
  if (template === currentTemplate) return '和此刻用的格式一样，不用再登记';
  return null;
}

export function effectiveFromProblem(wallTime: string, instant: string | null): string | null {
  if (wallTime.trim() === '') return '填从什么时候起用';
  if (instant === null) return '时刻写法不对';
  return null;
}

/** 某一时刻在给定时区里的墙钟（`YYYY-MM-DDTHH:mm`），给 datetime-local 用。 */
export function wallTimeIn(instant: Date, timeZone: string): string {
  const parts = new Intl.DateTimeFormat('en-US', {
    timeZone,
    hourCycle: 'h23',
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  }).formatToParts(instant);
  const read = (type: Intl.DateTimeFormatPartTypes) => parts.find((part) => part.type === type)?.value ?? '00';
  const hour = read('hour') === '24' ? '00' : read('hour');
  return `${read('year')}-${read('month')}-${read('day')}T${hour}:${read('minute')}`;
}

/** 给定时区里下个月 1 日零点的墙钟：按月重排的格式最常从这一刻换。 */
export function nextMonthStartWallTime(instant: Date, timeZone: string): string {
  const [date] = wallTimeIn(instant, timeZone).split('T');
  const [year, month] = date.split('-').map(Number);
  const nextYear = month === 12 ? year + 1 : year;
  const nextMonth = month === 12 ? 1 : month + 1;
  return `${nextYear}-${String(nextMonth).padStart(2, '0')}-01T00:00`;
}

// —— 答复的话 ——

/** 一次读、预览或登记没形成业务答案时的那句话；码与 HTTP 状态另进技术详情，不上句子。 */
export function failureWords(answer: Exclude<ApiResult<unknown>, { kind: 'outcome' }>, what: string): string {
  if (answer.kind === 'unconfigured') return `${what}：这一口的接入渠道尚未配置，要部署方配置后才能用。`;
  if (isOperatorRefusal(answer)) return operatorRefusalMessage(answer.kind, what);
  if (answer.kind === 'callerProblem') {
    const words = registrationRejectionWords[answer.code];
    if (words) return words;
    return answer.detail ?? `${what}：服务端认为这次请求的写法不对，请刷新页面后重试。`;
  }
  if (answer.kind === 'noAnswer') return `${what}：服务端没有形成答案，什么也没有落，可稍后重试。`;
  return `${what}：请求没有到达服务端（${answer.message}）。`;
}

export function failureTechnicalRows(answer: ApiResult<unknown>): TechnicalRow[] {
  if (answer.kind === 'callerProblem' || answer.kind === 'noAnswer') {
    return [
      { label: 'HTTP 状态', value: String(answer.status) },
      { label: '问题码', value: answer.code },
    ];
  }
  return [];
}

export type PreviewView =
  | { kind: 'sample'; sample: string; zone: string; at: string; reset: ResetWords; overlaps: PreviewOverlapRecord[] }
  | { kind: 'rejected'; reason: string; words: string }
  | { kind: 'noZone'; reset: ResetWords }
  | { kind: 'failed'; words: string; technical: TechnicalRow[] };

export function previewViewOf(answer: ApiResult<PreviewBody>): PreviewView {
  if (answer.kind !== 'outcome') {
    return { kind: 'failed', words: failureWords(answer, '预览'), technical: failureTechnicalRows(answer) };
  }
  const body = answer.body;
  switch (body.outcome) {
    case 'PREVIEW_RENDERED':
      return {
        kind: 'sample',
        sample: body.sample,
        zone: zoneWords(body.timeZone),
        at: body.at,
        reset: resetWordsOf(body.resetPeriod),
        overlaps: body.overlaps ?? [],
      };
    case 'PREVIEW_REJECTED':
      return {
        kind: 'rejected',
        reason: body.reason,
        words: registrationRejectionWords[body.reason] ?? '这个格式登记不上，服务端给的理由界面还不认识；请把技术详情里的码报给产品支持。',
      };
    case 'NUMBERING_TIME_ZONE_NOT_CONFIGURED':
      return { kind: 'noZone', reset: resetWordsOf(body.resetPeriod) };
    default:
      return { kind: 'failed', words: '预览答了一个界面不认识的结果；请刷新页面后重试。', technical: [] };
  }
}

/** 重叠提示里那一份早先格式的叫法：「产品默认格式」或「2026-11-01 … 起的本公司格式」。 */
export function overlapSourceWords(overlap: PreviewOverlapRecord): string {
  if (overlap.source === 'PRODUCT_DEFAULT' || !overlap.effectiveFrom) return '产品默认格式';
  return `${formatInstant(overlap.effectiveFrom)} 起的本公司格式`;
}

export interface RegistrationAnswerView {
  /** 册上已有这一版（新登或原本就有）：页面重读规则册。 */
  landed: boolean;
  /** 业务上成了（新登）还是没成；决定话的语气。 */
  tone: 'success' | 'info' | 'warning';
  words: string;
  technical: TechnicalRow[];
}

export function ruleRegistrationAnswerOf(answer: ApiResult<RuleRegistrationBody>, type: DocumentTypeName): RegistrationAnswerView {
  if (answer.kind !== 'outcome') {
    return { landed: false, tone: 'warning', words: failureWords(answer, '登记新格式'), technical: failureTechnicalRows(answer) };
  }
  const version = answer.body.version;
  const from = version ? formatInstant(version.effectiveFrom) : '';
  switch (answer.body.outcome) {
    case 'VERSION_REGISTERED':
      return {
        landed: true,
        tone: 'success',
        words: `已登记：自 ${from} 起，新开立的${type.name}按新格式出${type.numberName}；已开立的${type.numberName}不变。`,
        technical: [],
      };
    case 'VERSION_EXISTING':
      return { landed: true, tone: 'info', words: `册上已有完全相同的一版（同一格式、${from} 起），没有重复登记。`, technical: [] };
    case 'EFFECTIVE_FROM_TAKEN':
      return {
        landed: false,
        tone: 'warning',
        words: `${from} 这个生效时刻已经登过另一份格式（${templateReading(version?.template ?? '')}）；规则只增不改，请换一个生效时刻。`,
        technical: [],
      };
    default:
      return { landed: false, tone: 'warning', words: '登记口答了一个界面不认识的结果；请刷新页面看册上现状。', technical: [] };
  }
}

export function timeZoneRegistrationAnswerOf(answer: ApiResult<TimeZoneRegistrationBody>): RegistrationAnswerView {
  if (answer.kind !== 'outcome') {
    return { landed: false, tone: 'warning', words: failureWords(answer, '登记单据编号时区'), technical: failureTechnicalRows(answer) };
  }
  const version = answer.body.version;
  const from = version ? formatInstant(version.effectiveFrom) : '';
  const zone = version ? zoneWords(version.timeZone) : '';
  switch (answer.body.outcome) {
    case 'VERSION_REGISTERED':
      return {
        landed: true,
        tone: 'success',
        words: `已登记：自 ${from} 起，单据编号时区为${zone}。此前开立、没带单据号的单据不补号。`,
        technical: [],
      };
    case 'VERSION_EXISTING':
      return { landed: true, tone: 'info', words: `册上已有完全相同的一版（${zone}，${from} 起），没有重复登记。`, technical: [] };
    case 'EFFECTIVE_FROM_TAKEN':
      return {
        landed: false,
        tone: 'warning',
        words: `${from} 这个生效时刻已经登过另一个时区（${zone}）；只增不改，请换一个生效时刻。`,
        technical: [],
      };
    default:
      return { landed: false, tone: 'warning', words: '登记口答了一个界面不认识的结果；请刷新页面看册上现状。', technical: [] };
  }
}

// —— 时区候选 ——

export interface TimeZoneOption {
  zone: string;
  label: string;
}

/**
 * 时区候选：运行环境认得的全部 IANA 名，按此刻偏移再按名字排，每项写成「UTC+8 · 中国标准时间 · Asia/Shanghai」——
 * 同一个中文名常对着好几个 IANA 名，名字本身得留着才分得开。不预选任何一项：单据编号时区是租户取值，产品不给默认。
 */
export function timeZoneOptionsOf(
  zones: readonly string[],
  offsetLabel: (zone: string) => string,
  offsetMinutes: (zone: string) => number,
): TimeZoneOption[] {
  // 有的运行环境的候选表里没有裸 `UTC`；租户明说要 UTC 时得选得到（服务端收它，只是不替租户默认它）。
  const unique = [...new Set(zones.includes('UTC') ? zones : [...zones, 'UTC'])];
  return unique
    .map((zone) => ({ zone, offset: offsetMinutes(zone) }))
    .sort((left, right) => left.offset - right.offset || (left.zone < right.zone ? -1 : left.zone > right.zone ? 1 : 0))
    .map(({ zone }) => {
      const name = zoneWords(zone);
      return { zone, label: name === zone ? `${offsetLabel(zone)} · ${zone}` : `${offsetLabel(zone)} · ${name} · ${zone}` };
    });
}
