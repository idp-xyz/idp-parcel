// 发布表单纯逻辑的共享层（票 admin-write-faces/22）：各册 *-form.ts 此前各自留一份的整数解析、时点归一、方案版本引用拼法，
// 这里各只有一份。伞票 07 并行写表单那阵子的纪律是「不跨文件借私有件」，代价是每张表单各留一份同形副本、改一处规则要逐张改；
// 各表单落齐之后本文件把它们抬到一处，各 *-form.ts 改为从这里导入。
//
// 只放**与哪一册无关**的纯函数：它们讨论的是「文本编不编得进 JSON 类型」「墙钟怎么换成时刻」这类线格式层面的事，
// 不涉及任何一册的载荷键与判据。某册自己的路径、草稿、载荷仍留在它自己的 *-form.ts。
//
// 本文件不算摘要、不裁任何门（伞票 07 硬句）：integerProblem 点名的只是「根本组不出那份载荷」的文本，零与负数照发让服务端说。

import type { PriceCardRecord } from '../pricing/api';
import { wallTimeToRfc3339 } from '../moment';

// 与服务端 int / *int64 的入口一致：只认十进制整数文本；小数、指数、字母都组不进那个类型。
const integerText = /^[+-]?\d+$/;

/**
 * 发布表单的时刻格（票 party-commercial-uiux/10）：日期时间控件交出的本地墙钟（`YYYY-MM-DDTHH:mm[:ss]`）按操作者
 * 的显示时区换成 UTC 的 RFC 3339，与登记表单的 WallTimeField 同一条换算（moment.ts 的 wallTimeToRfc3339）。此前十张发布
 * 表单有三套规则：只填日期补成当天零点 **UTC**（对国内租户落在早上 8 点）、要求手敲完整时刻、登记表单按本地时区——
 * 同叫「生效起点」，同样的输入在两张表上落在不同的时刻。
 *
 * 留空交空串：必填格照送，由服务端点名；选填格（止点）由调用方不带这一键。换不出来（正常的日期时间控件交不出这种值，
 * 粘贴或旧草稿可能）原样交出，同时由 wallMomentProblem 在本地点名、挡住预览，不替人猜。
 */
export function wallMomentOf(raw: string, timeZone: string): string {
  const text = raw.trim();
  if (text === '') return '';
  return wallTimeToRfc3339(text, timeZone) ?? text;
}

/** 编不成时刻的墙钟文本的本地问题；空与编得成都是 null。 */
export function wallMomentProblem(raw: string, timeZone: string): string | null {
  const text = raw.trim();
  if (text === '') return null;
  return wallTimeToRfc3339(text, timeZone) === null ? '时间格式不对，请用日期时间选择器重新选' : null;
}

/**
 * 整数格的文本 → 数值。空即缺席（undefined）；编不进 JSON 整数的文本也交回 undefined，由同一册的 *LocalProblems 在同一
 * 判据（integerProblem）上点名，两处不会一处放一处拦。
 */
export function integerOf(raw: string): number | undefined {
  const text = raw.trim();
  if (text === '' || !integerText.test(text)) return undefined;
  const value = Number(text);
  return Number.isSafeInteger(value) ? value : undefined;
}

/** 整数格的本地问题：空不是问题（缺席由服务端点名），非整数文本与超出精确范围的整数各一句。 */
export function integerProblem(raw: string): string | null {
  const text = raw.trim();
  if (text === '') return null;
  if (!integerText.test(text)) return '须为十进制整数文本（不接受小数、指数与字母）';
  if (!Number.isSafeInteger(Number(text))) return '超出页面能精确表示的整数范围';
  return null;
}

/**
 * 价卡目录行 → 方案版本引用串。写法与 settlement-accounting 拼方案版本引用同一条（`planId@planVersion`，
 * 见 internal/settlementaccounting/adapters/parcelpricing/buy_evaluation.go）；跨上下文只传引用，表单不读方案内容，
 * 方向与绑定换算是 PRICE_RULE 那册的事。
 */
export function planReferenceOf(card: Pick<PriceCardRecord, 'planId' | 'planVersion'>): string {
  return `${card.planId}@${card.planVersion}`;
}

/**
 * 认领了却没有一处渲染的路径（票 22 判据 3）。`claimed` 是表单交给 PublicationDraftFlow 的认领表，`rendered` 是它自己声明
 * 会渲染 Problems 的路径；差集非空就是服务端点到那一格时会被认领又不显示——公共半边按「已认领」不单列，表单又没地方显，
 * 那条问题被静默吞掉。差集按认领表的原序交回，便于测试点名。
 */
export function unrenderedClaimedPaths(claimed: readonly string[], rendered: readonly string[]): string[] {
  const shown = new Set(rendered);
  return claimed.filter((path) => !shown.has(path));
}
