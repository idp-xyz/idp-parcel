// 库内作业事实：每一行账上入了多少、没入的是为什么（纯 TypeScript，node:test 直接测）。
//
// 库内作业是已经发生的事实，没有生命周期。每一行登记时按账取货：能取走的入账；取不走的数量要么是账上本来就不够（账上不足），
// 要么账上有、却被冻结、盘点锁定、待拣分配或预占占着（被占用挡住）。没入账的部分另形成一条作业差异，等批准补账或驳回。
// 整箱行一箱一箱算：整箱不在账上是整箱缺失，在账上却被占着就不移，否则整箱移到目标库位。账实短少上报不入账，只形成差异。
// 这里的数是登记那一刻入的：后来批准差异补进去的，记在差异与库存流水上，不回改作业事实。

export const shortageKind = 'SHORTAGE';

export interface JobLineFacts {
  ownerAccount: string;
  goodsCode?: string;
  unit?: string;
  quantity?: number;
  cartonNumber?: string;
  sourceLocation?: string;
  targetLocation?: string;
  posted: number;
  postedInbound: number;
  shortOnBook: number;
  held: number;
  cartonMissing?: boolean;
  movementId?: string;
  discrepancyId?: string;
}

export type LineOutcome = 'full' | 'partial' | 'none' | 'cartonMissing' | 'reported';

export interface LineAccounting {
  outcome: LineOutcome;
  carton: boolean;
  /** 计划数量：整箱行是一箱，短少上报行是上报的数量。 */
  planned: number;
  /** 从来源取走并入账的数量，按来源单位。 */
  posted: number;
  /** 入到目标账的数量，按目标单位：包装形态转换时换了单位，两个数不相等。读口不答目标单位。 */
  postedInbound: number;
  /** 计划里没入账的数量。 */
  unposted: number;
  shortOnBook: number;
  held: number;
  /** 没入账的数量里，账上不足与被占用都说不清的那一部分：读口没说明原因，如实写出来，不硬凑进某一类。 */
  unexplained: number;
}

export function lineAccounting(kind: string, line: JobLineFacts): LineAccounting {
  const carton = Boolean(line.cartonNumber) || line.cartonMissing === true;
  const planned = carton ? 1 : (line.quantity ?? 0);
  if (kind === shortageKind) {
    // 短少上报只形成差异，这一行本身没有入账可说。
    return { outcome: 'reported', carton, planned, posted: 0, postedInbound: 0, unposted: 0, shortOnBook: 0, held: 0, unexplained: 0 };
  }
  const unposted = Math.max(0, planned - line.posted);
  const facts = { carton, planned, posted: line.posted, postedInbound: line.postedInbound, unposted, shortOnBook: line.shortOnBook, held: line.held };
  if (carton) {
    const outcome: LineOutcome = line.cartonMissing === true ? 'cartonMissing' : line.posted >= 1 ? 'full' : 'none';
    return { ...facts, outcome, unexplained: 0 };
  }
  const outcome: LineOutcome = planned > 0 && line.posted >= planned ? 'full' : line.posted > 0 ? 'partial' : 'none';
  return { ...facts, outcome, unexplained: Math.max(0, unposted - line.shortOnBook - line.held) };
}

/** 这一行的结果，一句话：数字在各列里，这里说它们合起来是怎么回事。 */
export function lineStory(acc: LineAccounting): string {
  switch (acc.outcome) {
    case 'reported':
      return '已上报，不直接改账';
    case 'cartonMissing':
      return '账上没有这一箱，没有移动';
    case 'full':
      return acc.carton ? '整箱已移到目标库位' : '全部入账';
    default: {
      if (acc.carton) return acc.held > 0 ? '这一箱被冻结或占用着，没有移动' : '没有移动';
      const head = acc.posted > 0 ? `入账 ${acc.posted} / 计划 ${acc.planned}` : `计划 ${acc.planned}，一件没入账`;
      const reasons = [
        acc.shortOnBook > 0 ? `账上不足 ${acc.shortOnBook}` : '',
        acc.held > 0 ? `被占用挡住 ${acc.held}` : '',
        acc.unexplained > 0 ? `另有 ${acc.unexplained} 读口没说明原因` : '',
      ].filter((reason) => reason !== '');
      return reasons.length > 0 ? `${head}：${reasons.join('，')}` : head;
    }
  }
}

export const lineOutcomeLabels: Readonly<Record<LineOutcome, string>> = {
  full: '全部入账',
  partial: '部分入账',
  none: '没入账',
  cartonMissing: '整箱缺失',
  reported: '已上报',
};

export const lineOutcomeTones: Readonly<Record<LineOutcome, 'success' | 'warning' | 'danger' | 'neutral'>> = {
  full: 'success',
  partial: 'warning',
  none: 'danger',
  cartonMissing: 'danger',
  reported: 'neutral',
};

/** 计划数量连单位：整箱行是一箱，商品行写数量与来源单位。 */
export function plannedLabel(line: { cartonNumber?: string; cartonMissing?: boolean; quantity?: number; unit?: string }): string {
  if (line.cartonNumber || line.cartonMissing === true) return '1 箱';
  return `${line.quantity ?? 0}${line.unit ? ` ${line.unit}` : ''}`;
}

/** 来源库位到目标库位。两头一样（转换发生在原库位）只写一个，只有一头的写那一头，都没有答 —。 */
export function lineLocations(line: { sourceLocation?: string; targetLocation?: string }): string {
  const source = line.sourceLocation || '';
  const target = line.targetLocation || '';
  if (source !== '' && target !== '' && source !== target) return `${source} → ${target}`;
  return source || target || '—';
}

export interface JobSummary {
  lines: number;
  cartonLines: number;
  full: number;
  partial: number;
  none: number;
  cartonMissing: number;
  reported: number;
  /** 没入完的行：部分入账、没入账与整箱缺失。上报行不算，它本来就不入账。 */
  incomplete: number;
  /** 这笔作业形成的差异号，去重，按行序。 */
  discrepancyIds: string[];
  /** 涉及的货主，去重，按行序。 */
  owners: string[];
}

export function jobSummary(kind: string, lines: readonly JobLineFacts[]): JobSummary {
  const summary: JobSummary = {
    lines: lines.length,
    cartonLines: 0,
    full: 0,
    partial: 0,
    none: 0,
    cartonMissing: 0,
    reported: 0,
    incomplete: 0,
    discrepancyIds: [],
    owners: [],
  };
  for (const line of lines) {
    const acc = lineAccounting(kind, line);
    if (acc.carton) summary.cartonLines += 1;
    summary[acc.outcome] += 1;
    if (acc.outcome === 'partial' || acc.outcome === 'none' || acc.outcome === 'cartonMissing') summary.incomplete += 1;
    if (line.discrepancyId && !summary.discrepancyIds.includes(line.discrepancyId)) summary.discrepancyIds.push(line.discrepancyId);
    if (!summary.owners.includes(line.ownerAccount)) summary.owners.push(line.ownerAccount);
  }
  return summary;
}

/**
 * 没入完的行各是什么原因，按原因数行：账上不足几行、被占用挡住几行、整箱缺失几行。一行可以既账上不足又被占用，
 * 记进两类，所以各类之和可以大于没入完的行数。
 */
export function incompleteReasons(kind: string, lines: readonly JobLineFacts[]): string[] {
  let short = 0;
  let held = 0;
  let missing = 0;
  let unexplained = 0;
  for (const line of lines) {
    const acc = lineAccounting(kind, line);
    if (acc.outcome === 'full' || acc.outcome === 'reported') continue;
    if (acc.outcome === 'cartonMissing') {
      missing += 1;
      continue;
    }
    if (acc.shortOnBook > 0) short += 1;
    if (acc.held > 0) held += 1;
    if (acc.unexplained > 0) unexplained += 1;
  }
  return [
    short > 0 ? `账上不足 ${short} 行` : '',
    held > 0 ? `被占用挡住 ${held} 行` : '',
    missing > 0 ? `整箱缺失 ${missing} 行` : '',
    unexplained > 0 ? `原因读口没说明 ${unexplained} 行` : '',
  ].filter((reason) => reason !== '');
}

/** 列表上一格写完这笔作业入账得怎么样；warn 为真时用提醒色。 */
export function jobSignal(kind: string, lines: readonly JobLineFacts[]): { text: string; warn: boolean } {
  if (lines.length === 0) return { text: '没有作业行', warn: false };
  const summary = jobSummary(kind, lines);
  if (kind === shortageKind) return { text: `${summary.reported} 行短少上报`, warn: false };
  if (summary.incomplete === 0) return { text: '全部入账', warn: false };
  return { text: `${summary.incomplete} / ${summary.lines} 行没入完`, warn: true };
}
