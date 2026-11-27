// 拣货工位的待拣队列：把还没拣的拣货任务排成作业的次序（纯 TypeScript，node:test 直接测）。
//
// 看本仓全部波次时向拣货任务读口要待执行的（按仓与任务状态筛，缺省按放出时刻从新到旧），不再把波次整页取来自己挑。
// 一次最多翻几页，翻不完就在结果里如实带出看了多少条，由界面照实说，不假装看全了。
// 指名了波次（地址里的 `?wave=`）时按波次号读那一张，待执行的格在这一张里挑。

import type { ApiResult } from '../../catalogue-api';
import type { CatalogueConditions } from '../../catalogue-query';
import type { WaveListBody, WaveOneBody, WaveRecord } from '../api';

/** 合成一格拿的几条任务里的一条，和它自己要拣几件。 */
export interface PickMember {
  taskId: string;
  waveId: string;
  requestId: string;
  quantity: number;
}

/**
 * 一格：去哪个库位、拣哪个商品、拣几件。平常一格就是一条拣货任务。
 *
 * 货合在一起拿的波次（单品单件波次，或放出时选了合并拣货的波次）里，同一库位、同一位货主的同一种货的几条任务
 * 合成一格拿（`members`）：这一格的应拣数量是各条任务应拣数量的合计。合的只是拣货的人看到的这一层——账上仍是
 * 一条任务一笔，提交时一条一条交。`taskId`、`waveId`、`requestId` 取其中头一条的，只用来认这一格。
 */
export interface PickCell {
  taskId: string;
  waveId: string;
  requestId: string;
  locationCode: string;
  goodsCode: string;
  unit: string;
  stockState: string;
  /** 应拣数量。 */
  quantity: number;
  /** 这一格来自单品单件波次：它的货合在一起拿、复核打包时扫货配单，不按出库单分周转箱。 */
  singleUnit?: boolean;
  /** 这一格来自合并拣货的波次：它的货合在一起拿，拿回来到分拣才按出库单分开，拣货时不分周转箱。 */
  batch?: boolean;
  /** 合成这一格的几条任务；没有这一项是一条任务一格。 */
  members?: readonly PickMember[];
}

/** 这一格的货是不是合在一起拿的（不按出库单分周转箱）。 */
export const pickedTogether = (cell: PickCell): boolean => cell.singleUnit === true || cell.batch === true;

/** 合成一格的任务里取出一条，当成一条任务一格的样子去提交：应拣数量是这条任务自己的。 */
export function memberCell(cell: PickCell, member: PickMember): PickCell {
  const { members: _members, ...rest } = cell;
  return { ...rest, taskId: member.taskId, waveId: member.waveId, requestId: member.requestId, quantity: member.quantity };
}

/**
 * 合成的一格拿到了 `quantity` 件，按任务的先后分下去：排在前面的任务记满，轮到的那一条不够就按剩下的记，
 * 排在后面、一件都没分到的任务按 0 件记。后两种都是短拣，后端各自换别的库位补；每条任务都在交回的名单里，
 * 这一格交完就没有留在队里的任务。
 */
export function allotMembers(cell: PickCell, quantity: number): { cell: PickCell; quantity: number }[] {
  let left = Math.max(0, quantity);
  return (cell.members ?? []).map((member) => {
    const given = Math.min(member.quantity, left);
    left -= given;
    return { cell: memberCell(cell, member), quantity: given };
  });
}

/** 一次最多翻几页（每页 200 条待执行的任务）。 */
export const MAX_WAVE_PAGES = 5;

// 库位码里的数字按数值比：A-1-2 排在 A-1-10 前面，拣货的人顺着库位走。比不出先后的再按任务号定，次序才稳定。
const collator = new Intl.Collator('en', { numeric: true });

export function compareLocationCodes(left: string, right: string): number {
  return collator.compare(left, right);
}

function compareCells(left: PickCell, right: PickCell): number {
  const byLocation = compareLocationCodes(left.locationCode, right.locationCode);
  if (byLocation !== 0) return byLocation;
  if (left.taskId === right.taskId) return 0;
  return left.taskId < right.taskId ? -1 : 1;
}

export interface PickScope {
  warehouse: string;
  /** 只做这个波次；null 是本仓全部波次。 */
  wave: string | null;
  /** 此刻拣货的这个人的操作者标识；认不出为 null 或不给，这时不按拣货人筛。 */
  me?: string | null;
}

type Assigned = Pick<WaveRecord, 'assignee' | 'zoneAssignees'>;

/**
 * 一条任务此刻归谁拣，与后端 application.WaveHandler 的 pickAssignee 同一个口径：它所在库位的库区单独派了人就是那个人，
 * 没有就是整个波次派给的人；都没派为 undefined，谁都可以拣。
 */
export function taskAssignee(wave: Assigned, task: { zoneCode?: string }): string | undefined {
  const zoned = task.zoneCode ? wave.zoneAssignees?.find((entry) => entry.zoneCode === task.zoneCode)?.assignee : undefined;
  return zoned || wave.assignee || undefined;
}

/**
 * 看本仓全部波次时，派给了别人的格不摆：那是别人的活，拣了后端也不收。指名了波次就照摆——人是特意选的它，
 * 能不能拣由后端答。认不出自己是谁时不筛，免得把自己的活也藏起来。
 */
export function assignedElsewhere(wave: Assigned, task: { zoneCode?: string }, scope: PickScope): boolean {
  const assignee = taskAssignee(wave, task);
  return scope.wave === null && Boolean(scope.me) && assignee !== undefined && assignee !== scope.me;
}

/**
 * 从读到的波次挑出要拣的格：本仓的、（指名了波次时）那个波次的、任务状态是待执行的；按库位码排。
 * 数量不是正整数的任务不收：后端不收小于 1 的实拣数量，放进来只会卡住这一格。
 */
export function cellsOfWaves(waves: readonly WaveRecord[], scope: PickScope): PickCell[] {
  const cells = new Map<string, PickCell>();
  // 货合在一起拿的波次里能合的任务：键是库位、货主、商品、单位、库存状态。货主要在键里——商品编码只在一位货主之内唯一。
  const groups = new Map<string, { cell: PickCell; members: PickMember[] }>();
  for (const wave of waves) {
    if (wave.warehouseCode !== scope.warehouse) continue;
    if (scope.wave !== null && wave.waveId !== scope.wave) continue;
    for (const task of wave.tasks ?? []) {
      if (task.status !== 'OPEN') continue;
      if (assignedElsewhere(wave, task, scope)) continue;
      if (!Number.isInteger(task.quantity) || task.quantity < 1) continue;
      const cell: PickCell = {
        taskId: task.taskId,
        waveId: wave.waveId,
        requestId: task.requestId,
        locationCode: task.locationCode,
        goodsCode: task.goodsCode,
        unit: task.unit,
        stockState: task.stockState,
        quantity: task.quantity,
        ...(wave.singleUnit === true ? { singleUnit: true } : {}),
        ...(wave.batchPick === true ? { batch: true } : {}),
      };
      // 读口没带货主（旧后端）时不合：分不清是不是同一位货主的货
      if (!pickedTogether(cell) || !task.ownerAccount) {
        cells.set(task.taskId, cell);
        continue;
      }
      // 单品单件波次与合并拣货的波次不混在一格里：前者到复核打包扫货配单，后者要去分拣，拿回来走的不是一条路
      const key = [wave.singleUnit === true ? 'single' : 'batch', task.locationCode, task.ownerAccount, task.goodsCode, task.unit, task.stockState].join(
        '\u0000',
      );
      const member: PickMember = { taskId: task.taskId, waveId: wave.waveId, requestId: task.requestId, quantity: task.quantity };
      const group = groups.get(key);
      if (group === undefined) groups.set(key, { cell, members: [member] });
      else if (!group.members.some((known) => known.taskId === member.taskId)) group.members.push(member);
    }
  }
  for (const group of groups.values()) {
    if (group.members.length === 1) {
      cells.set(group.cell.taskId, group.cell);
      continue;
    }
    // 成员按任务号排，头一条的号认这一格：同一批任务读几次，合出来的是同一格、同一个次序
    const members = [...group.members].sort((left, right) => (left.taskId < right.taskId ? -1 : left.taskId > right.taskId ? 1 : 0));
    const head = members[0];
    const quantity = members.reduce((sum, member) => sum + member.quantity, 0);
    cells.set(head.taskId, { ...group.cell, taskId: head.taskId, waveId: head.waveId, requestId: head.requestId, quantity, members });
  }
  return [...cells.values()].sort(compareCells);
}

/** 「全部波次」在下拉里的值：下拉不收空串；波次号是放波次时系统给的号，不会是它。 */
export const allWavesChoice = '__all__';

export interface WaveChoice {
  value: string;
  label: string;
}

/**
 * 「换波次」下拉的选项：全部波次在最前，后面是读到的还有格没拣的波次，次序照队列里给的（新的在前）。地址里指名的波次不在其中时
 * 也列出来，下拉上写的才是此刻正在做的这一个。
 */
export function waveChoices(current: string | null, waves: ReadonlyArray<{ waveId: string; open: number }>): WaveChoice[] {
  const named = current !== null && !waves.some((entry) => entry.waveId === current) ? [{ waveId: current, open: 0 }, ...waves] : waves;
  return [
    { value: allWavesChoice, label: '全部波次' },
    ...named.map((entry) => ({ value: entry.waveId, label: entry.open > 0 ? `${entry.waveId}（还有 ${entry.open} 格）` : entry.waveId })),
  ];
}

/** 下拉里选中的值换回波次号；「全部波次」是 null。 */
export function waveOfChoice(value: string): string | null {
  return value === allWavesChoice ? null : value;
}

export interface PickQueue {
  cells: PickCell[];
  /** 这一趟读到了几个波次。 */
  wavesRead: number;
  /** 这一趟读到了几条任务。看本仓全部波次时，翻不完说的是这个数。 */
  tasksRead: number;
  pagesRead: number;
  /** 翻到页数上限，读口还有下一页：更早放出、还没拣完的波次没有看，里面没拣的格这里看不到。 */
  truncated: boolean;
  /** 指名了波次，而没有这个波次（或它不属于本仓）。 */
  waveMissing: boolean;
  /** 读到的波次里各还有几格没拣（只列还有的），新的在前；供「换波次」选。带着各自派给了谁。 */
  waves: Array<{ waveId: string; open: number; assignee?: string }>;
  /** 有没拣的格派给了别人（整个波次，或其中几个库区）、所以没摆进队列的波次数；只在看本仓全部波次时才会有。 */
  assignedElsewhere: number;
}

type ReadFailure = Exclude<ApiResult<unknown>, { kind: 'outcome' }>;

export type PickQueueAnswer = { kind: 'queue'; queue: PickQueue } | { kind: 'failed'; failure: ReadFailure };

export interface PickQueueInput extends PickScope {
  list: (conditions: CatalogueConditions, after: string | null) => Promise<ApiResult<WaveListBody>>;
  one: (waveId: string) => Promise<ApiResult<WaveOneBody>>;
  maxPages?: number;
}

function queueOf(waves: readonly WaveRecord[], scope: PickScope, read: Pick<PickQueue, 'pagesRead' | 'truncated'>): PickQueue {
  const scoped = waves.filter(
    (candidate) => candidate.warehouseCode === scope.warehouse && (scope.wave === null || candidate.waveId === scope.wave),
  );
  return {
    cells: cellsOfWaves(waves, scope),
    wavesRead: waves.length,
    tasksRead: waves.reduce((sum, candidate) => sum + (candidate.tasks?.length ?? 0), 0),
    ...read,
    waveMissing: scope.wave !== null && scoped.length === 0,
    waves: scoped
      .map((candidate) => ({
        waveId: candidate.waveId,
        open: (candidate.tasks ?? []).filter((task) => task.status === 'OPEN').length,
        ...(candidate.assignee ? { assignee: candidate.assignee } : {}),
      }))
      .filter((entry) => entry.open > 0),
    assignedElsewhere: scoped.filter((candidate) => (candidate.tasks ?? []).some((task) => task.status === 'OPEN' && assignedElsewhere(candidate, task, scope))).length,
  };
}

export async function loadPickQueue(input: PickQueueInput): Promise<PickQueueAnswer> {
  const { warehouse, wave, list, one, me } = input;
  if (wave !== null) {
    const answer = await one(wave);
    if (answer.kind !== 'outcome') return { kind: 'failed', failure: answer };
    const found = answer.body.wave ? [answer.body.wave] : [];
    return { kind: 'queue', queue: queueOf(found, { warehouse, wave, me }, { pagesRead: 1, truncated: false }) };
  }

  const maxPages = input.maxPages ?? MAX_WAVE_PAGES;
  const conditions: CatalogueConditions = { filters: { warehouseCode: [warehouse], status: ['OPEN'] }, pageSize: 200 };
  const waves: WaveRecord[] = [];
  let after: string | null = null;
  let pagesRead = 0;
  do {
    const answer: ApiResult<WaveListBody> = await list(conditions, after);
    if (answer.kind !== 'outcome') return { kind: 'failed', failure: answer };
    pagesRead += 1;
    waves.push(...answer.body.waves);
    after = answer.body.page.next;
  } while (after !== null && pagesRead < maxPages);

  return { kind: 'queue', queue: queueOf(waves, { warehouse, wave, me }, { pagesRead, truncated: after !== null }) };
}
