// 集团与法人页详情抽屉「修订历史」区的判读纯逻辑（票 admin-web-group-legal-entities/03 第 5 条）。
// 全部是纯函数，node:test 钉着；页面只负责摆（Timeline 原语）。
//
// 序照读口交回的修订号升序，页面不重排：从上往下读就是「登记 → （更正）→ 停用」这条链本身的次序，
// 抽屉头上显的最新修订落在最下面一格。不做 diff——两笔之间改了什么由并排的描述让人自己比
// （票面第 1 条：读口只交事实），这里不猜「这一笔改了名字还是换了依据」。

import type { LegalEntityRevisionRecord } from './api';

/** 与 @idpxyz/ui-primitives 的 TimelineItem 同形的那几格；只用到 default / warning 两种色调。 */
export interface RevisionTimelineItem {
  id: string;
  title: string;
  description: string;
  timestamp: string;
  variant: 'default' | 'warning';
}

/**
 * 一笔修订一项。停用那一笔标题带`已停用`（CONTEXT「身份生命周期」原词）、色调 warning；
 * 其余笔只显修订号——读口没给每一笔算状态（理由在 api.ts 的 LegalEntityRevisionRecord 注释），
 * 这里也不替它算：历史上被顶替的那几笔算出来的「状态」不是任何时刻的事实。
 *
 * 三个时刻都经注入的 `format`（页面传 moment.ts 的 formatInstant——它依赖装配点配置的呈现时区，
 * 纯函数不该自己去碰那个全局），一格也不原样漏 ISO 串。
 */
export function legalEntityRevisionTimeline(
  revisions: readonly LegalEntityRevisionRecord[],
  format: (iso: string) => string,
): RevisionTimelineItem[] {
  return revisions.map((row) => {
    const deactivated = row.deactivatedAt !== undefined;
    let description =
      `依据 ${row.basis} · 生效自 ${format(row.effectiveFrom)} · 参与方身份 ${row.partyId}`;
    if (deactivated) {
      // 停用依据在库上与停用时点成对（0015 的 paired 约束），这里仍按键在不在读，不假定一定成对。
      const basis = row.deactivationBasis !== undefined ? `（依据 ${row.deactivationBasis}）` : '';
      description += ` · 停用于 ${format(row.deactivatedAt as string)}${basis}`;
    }
    return {
      id: `r${row.revision}`,
      title: deactivated ? `r${row.revision} · 已停用` : `r${row.revision}`,
      description,
      timestamp: `登记于 ${format(row.registeredAt)}`,
      variant: deactivated ? 'warning' : 'default',
    };
  });
}

/**
 * 历史区头上的计数句。零笔说的是「册上没有这个法人的修订」——那是读口交回的如实答案（票 03 裁
 * 200 + []），与「没问到」（未配置 / 出错，页面另显）分得开；抽屉只对列表里的行开，所以零笔在
 * 正常数据下不会出现，出现了就该去查写侧。
 */
export function revisionHistoryNote(count: number): string {
  if (count === 0) return '登记册上没有这个法人的修订。';
  return `共 ${count} 笔修订，按修订号从早到晚。`;
}
