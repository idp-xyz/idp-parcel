/**
 * 周转箱：拣货时装一张出库单的货的箱子，拣货车上的格口也按一个箱登记。状态词与后端 `ports.ToteFree` 那一组同一批。
 */

/** 空着与在用是要看的两队，数出来；已停用只增不减，不数。 */
export const liveToteStates = ['IN_USE', 'FREE'] as const;

const stateLabels: Record<string, string> = {
  FREE: '空着',
  IN_USE: '在用',
  RETIRED: '已停用',
};

/** 陌生的状态词原样示出，不猜。 */
export function toteStateLabel(state: string): string {
  return stateLabels[state] ?? state;
}

/** 状态胶囊的色：在用是进行中，空着是可用，已停用是中性。陌生的词按中性。 */
export function toteStateBadge(state: string): string {
  if (state === 'IN_USE') return 'PICKING';
  if (state === 'FREE') return 'ACTIVE';
  return 'CANCELLED';
}

/** 只有空着的箱能停用：在用的箱里还有货，已停用的不必再停。与后端 `RetireTote` 同一口径。 */
export function canRetireTote(state: string): boolean {
  return state === 'FREE';
}
