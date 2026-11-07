import { statusToneText } from '../../components/status-tone';
import type { WorkbenchChip, WorkbenchKpi } from '../../templates';
import type { StatusTone } from '../../vocabulary/status';

// 指标即筛：按状态词计数的每一格既是页头指标，也是筛选胶囊；点一下按这个状态筛，再点一下回到全部。格数随数据走，计数为零的
// 状态不出格。计数数的是交进来的行：只对一次拿全的列表是总数，按游标翻页的列表一页里数出来的不是。

const kpiToneClass: Partial<Record<StatusTone, string>> = {
  good: statusToneText.success,
  info: statusToneText.info,
  warn: statusToneText.warning,
  bad: statusToneText.danger,
};

export function statusFilterParts(
  counts: readonly { label: string; tone: StatusTone; count: number }[],
  total: number,
  active: string,
  setActive: (label: string) => void,
): { kpis: WorkbenchKpi[]; chips: WorkbenchChip[] } {
  const toggle = (label: string) => setActive(active === label ? 'ALL' : label);
  return {
    kpis: counts.map((item) => ({
      label: item.label,
      value: item.count,
      tone: kpiToneClass[item.tone],
      active: active === item.label,
      onClick: () => toggle(item.label),
    })),
    chips: [
      { id: 'ALL', label: '全部', count: total, active: active === 'ALL', onClick: () => setActive('ALL') },
      ...counts.map((item) => ({
        id: item.label,
        label: item.label,
        count: item.count,
        active: active === item.label,
        onClick: () => toggle(item.label),
      })),
    ],
  };
}
