// 工作台首页「今日待办」的格子与判读（设计源「九、怎么落地」第三步：首页改成今日待办，各一格可点进去）。纯函数，node:test 钉着；
// 取数在 Workbench.tsx。
//
// 每一格只数服务端读口**已经答出来**的「在等人处理」的件数，不自造统计：队列读口给什么，这里数什么；读口给了总数（pendingCount /
// unhandledCount）就用它，没给就数行。读不到不报 0——未配置、无权查看、暂时读不到三种各显各的，0 只在读口真答了 0 时出现。
// 页名一律取导航（pageTitleById），不在这里另起一套名字。

/** 一格背后的读口答复，只看判读要用的那一点；与各域 ApiResult 的五格同构，避免首页静态引入各域的 api 模块。 */
export type TodoAnswer<Body> =
  | { kind: 'outcome'; body: Body }
  | { kind: 'unconfigured' }
  | { kind: 'loginRequired' }
  | { kind: 'notGranted' }
  | { kind: 'callerProblem' }
  | { kind: 'noAnswer' }
  | { kind: 'transport' };

export type TodoTileState =
  | { kind: 'loading' }
  | { kind: 'count'; value: number }
  | { kind: 'unconfigured' }
  | { kind: 'denied' }
  | { kind: 'unavailable' };

export function todoStateOf<Body>(answer: TodoAnswer<Body> | null, count: (body: Body) => number): TodoTileState {
  if (answer === null) return { kind: 'loading' };
  switch (answer.kind) {
    case 'outcome':
      return { kind: 'count', value: count(answer.body) };
    case 'unconfigured':
      return { kind: 'unconfigured' };
    case 'loginRequired':
    case 'notGranted':
      return { kind: 'denied' };
    default:
      return { kind: 'unavailable' };
  }
}

/** 格子上数字位的字：读不到不写 0。 */
export function todoValueText(state: TodoTileState): string {
  switch (state.kind) {
    case 'loading':
      return '…';
    case 'count':
      return String(state.value);
    case 'unconfigured':
      return '未接通';
    case 'denied':
      return '无权查看';
    case 'unavailable':
      return '读不到';
  }
}

/** 格子下方一句：有件数时说在等什么；读不到时说为什么、怎么办。 */
export function todoNoteText(state: TodoTileState, waiting: string): string {
  switch (state.kind) {
    case 'loading':
      return '正在读取…';
    case 'count':
      return state.value === 0 ? '暂时没有要处理的' : waiting;
    case 'unconfigured':
      return '这项数据的接入还没配置好，不是没有待办';
    case 'denied':
      return '你的账号没有查看这一项的权限';
    case 'unavailable':
      return '这次没有读到，稍后刷新再看';
  }
}

/** 有待办的格亮琥珀，其余中性：设计原则四「待处理琥珀」。 */
export function todoNeedsAttention(state: TodoTileState): boolean {
  return state.kind === 'count' && state.value > 0;
}

// —— 各格从读口答复里数什么。只取判读要的字段，形状与各域 api.ts 的答复体对齐（结构类型，不引那些模块）。

/** 待受理复核：复核已录完成、等下一轮判断的行照列不出队，不算「等人复核」。 */
export function acceptanceReviewCount(body: { entries: { reviewCompleted: boolean }[] }): number {
  return body.entries.filter((entry) => !entry.reviewCompleted).length;
}

/** 受限运单处置：队列里每一行都在等有权的人选去向。 */
export function authorizedDispositionCount(body: { entries: unknown[] }): number {
  return body.entries.length;
}

/** 货物处置：读口给了未决总数，列表可能截断，用总数。 */
export function postIntakeDispositionCount(body: { pendingCount: number }): number {
  return body.pendingCount;
}

/** 资金释放待处理：读口给了未处理总数。 */
export function controlReleaseCount(body: { unhandledCount: number }): number {
  return body.unhandledCount;
}

/** 异常案件：没有关闭时刻的就是还开着的。 */
export function openExceptionCaseCount(body: { cases: { closedAt?: string }[] }): number {
  return body.cases.filter((record) => !record.closedAt).length;
}

/** 审批与发布：列表里的每一份都还没发布（待批准或已批准待发布）。 */
export function pendingPublicationCount(body: { drafts: unknown[] }): number {
  return body.drafts.length;
}
