// 加工、拆解与换标指令：生命周期在进度条上的落点、列表每行与单据页头区共用的下一步，以及单据上「这份指令要做什么」的
// 算法（纯 TypeScript，node:test 直接测）。
//
// 一份指令受理后停在待执行，现场在作业台执行。一份指令只执行一次：执行就进已执行，按这一次实际完成的数量入账，完成数量
// 可以少于计划数量，不会停在「部分完成」等第二次；没执行的才能作废，已执行的不能。已执行与已作废之后不再变。

import type { DocumentTrack, TrackStep } from './document-track';

// 状态码 DONE 在通用词表里是「已完成」，这一页叫「已执行」：完成数量可以少于计划数量，说「已完成」会让人以为计划数量都做完了。
export const instructionStatusLabels: Readonly<Record<string, string>> = {
  PENDING: '待执行',
  DONE: '已执行',
  VOID: '已作废',
};

export function instructionStatusLabel(status: string): string {
  return instructionStatusLabels[status] ?? status;
}

export const instructionSteps: readonly TrackStep[] = [
  { id: 'PENDING', label: '待执行' },
  { id: 'DONE', label: '已执行' },
];

/**
 * 待执行卡在等现场去作业台做，是这份指令上唯一要人动手的一步，用待办色标出。作废是人决定停下，不是失败，用中性的停止标记，
 * 步子都不算走过。读口没答出的陌生状态词原样示在第一步上，不替它猜走到了哪。
 */
export function instructionTrack(status: string): DocumentTrack {
  switch (status) {
    case 'PENDING':
      return { steps: instructionSteps, currentId: 'PENDING', currentLabel: '待执行' };
    case 'DONE':
      return { steps: instructionSteps, currentId: 'DONE' };
    case 'VOID':
      return { steps: instructionSteps, currentId: 'PENDING', halt: { label: '已作废', tone: 'neutral' } };
    default:
      return { steps: instructionSteps, currentId: 'PENDING', currentLabel: status };
  }
}

export interface InstructionNextStep {
  label: string;
  /** 执行本身在作业台做，页面只指路。 */
  moduleId: string;
}

/** 只有待执行的有下一步；已执行与已作废的不会再有现场要做的事。 */
export function instructionNextStep(status: string): InstructionNextStep | null {
  return status === 'PENDING' ? { label: '去作业台执行', moduleId: 'warehouse-workstation' } : null;
}

/** 这份指令停在待执行多久了，从哪一刻算：从受理算。别的状态不在等谁，答 undefined。 */
export function instructionWaitingSince(instruction: { status: string; receivedAt: string }): string | undefined {
  return instruction.status === 'PENDING' ? instruction.receivedAt : undefined;
}

/** 作废走哪个动作：换标与加工、拆解是两种运营决定授予，各有各的作废口。 */
export function instructionVoidActionId(kind: string): 'relabel-void' | 'processing-void' {
  return kind === 'RELABEL' ? 'relabel-void' : 'processing-void';
}

/** 没有这一种授予时说谁能作废：授予的叫法跟着指令种类走。 */
export function instructionGrantLabel(kind: string): string {
  return kind === 'RELABEL' ? '换标指令' : '加工指令';
}

/** 单据页的种类叫法；种类读不到（还没加载、陌生）时只叫「指令」。 */
export function instructionTitle(kind: string | undefined): string {
  switch (kind) {
    case 'PROCESS':
      return '加工指令';
    case 'DISASSEMBLE':
      return '拆解指令';
    case 'RELABEL':
      return '换标指令';
    default:
      return '指令';
  }
}

export interface InstructionFacts {
  kind: string;
  goodsCode: string;
  unit?: string;
  quantity: number;
  doneQuantity: number;
  fromAlias?: string;
  toAlias?: string;
  batchNumber?: string;
  expiryDate?: string;
  components?: ReadonlyArray<{ goodsCode: string; quantity: number }>;
}

export interface MaterialRow {
  direction: 'consume' | 'produce';
  goodsCode: string;
  /** 每 1 个成品对应多少个：成品自己是 1，组件按受理时固定下来的组成里的每件用量。 */
  perFinished: number;
  /** 按计划数量算，这一行共用掉或产出多少。 */
  total: number;
  /** 这一行是成品，不是组件。 */
  finished: boolean;
}

/**
 * 加工是用组件做出成品，拆解是把成品拆回组件，所以两者是同一张用料表的两个方向：加工用掉组件、产出成品，拆解用掉成品、
 * 产出组件。数量按计划数量乘每件用量；实际执行时的用料与产出以现场录的为准，页面只说计划。换标不用料，答空数组。
 */
export function instructionMaterials(instruction: InstructionFacts): MaterialRow[] {
  if (instruction.kind !== 'PROCESS' && instruction.kind !== 'DISASSEMBLE') return [];
  const finished: Omit<MaterialRow, 'direction'> = {
    goodsCode: instruction.goodsCode,
    perFinished: 1,
    total: instruction.quantity,
    finished: true,
  };
  const components: Array<Omit<MaterialRow, 'direction'>> = (instruction.components ?? []).map((component) => ({
    goodsCode: component.goodsCode,
    perFinished: component.quantity,
    total: component.quantity * instruction.quantity,
    finished: false,
  }));
  return instruction.kind === 'PROCESS'
    ? [...components.map((row): MaterialRow => ({ ...row, direction: 'consume' })), { ...finished, direction: 'produce' }]
    : [{ ...finished, direction: 'consume' }, ...components.map((row): MaterialRow => ({ ...row, direction: 'produce' }))];
}

/** 贴标别名没填就是没有贴标：换标可以从无到有，也可以把标去掉。 */
export const noAliasWord = '无贴标';

export interface RelabelPlan {
  /** null 即换之前没有贴标。 */
  fromAlias: string | null;
  /** null 即换之后不贴标。 */
  toAlias: string | null;
  /** 指令点名的批号与效期：点了名只换这一批，没点名换这个商品名下全部符合的库存。 */
  scope: string[];
}

export function relabelPlan(instruction: InstructionFacts): RelabelPlan | null {
  if (instruction.kind !== 'RELABEL') return null;
  const scope: string[] = [];
  if (instruction.batchNumber) scope.push(`批号 ${instruction.batchNumber}`);
  if (instruction.expiryDate) scope.push(`效期 ${instruction.expiryDate}`);
  return { fromAlias: instruction.fromAlias || null, toAlias: instruction.toAlias || null, scope };
}

/** 这份指令要做什么，一句话：页面正文的第一行，读的人不必先去对三张表。 */
export function instructionHeadline(instruction: InstructionFacts): string {
  const amount = `${instruction.quantity}${instruction.unit ? ` ${instruction.unit}` : ''}`;
  switch (instruction.kind) {
    case 'PROCESS':
      return `用组件加工出 ${instruction.goodsCode}，计划 ${amount}`;
    case 'DISASSEMBLE':
      return `把 ${instruction.goodsCode} 拆解成组件，计划 ${amount}`;
    case 'RELABEL': {
      const plan = relabelPlan(instruction);
      return `给 ${instruction.goodsCode} 换标，计划 ${amount}：从「${plan?.fromAlias ?? noAliasWord}」换成「${plan?.toAlias ?? noAliasWord}」`;
    }
    default:
      return `${instruction.goodsCode}，计划 ${amount}`;
  }
}

/**
 * 已执行而完成数量少于计划数量时，差多少；别的情形答 0。待执行还没有完成数量，已作废不会再有，都不算少做。
 */
export function instructionShortfall(instruction: { status: string; quantity: number; doneQuantity: number }): number {
  if (instruction.status !== 'DONE') return 0;
  return Math.max(0, instruction.quantity - instruction.doneQuantity);
}
