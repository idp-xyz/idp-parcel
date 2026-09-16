// 参与方身份两册（责任法人、业务参与方）详情抽屉「修订历史」区共用的判读纯逻辑（票 admin-web-group-legal-
// entities/03 立法人册那份，票 12 把它泛化成两册共用——票面第 5 条：字段同形就泛化一份，别抄第二份）。
// 全部是纯函数，node:test 经两册各自的薄适配钉着；页面只负责摆（Timeline 原语）。
//
// 两册的修订行在这几格上同形：修订号、依据、生效时点、停用两件、登记时间——它们都来自同一个
// domain.IdentityLifecycle 与同一条「登记按修订版本化只增不覆盖」的纪律（CONTEXT Lifecycles 下「参与方身份
// （业务参与方、责任法人、货主客户账户）」）。各册多出的那一格是这一笔修订的**内容**（法人册是钉着哪个
// 参与方身份，参与方册是名称），由适配方以一个回调交进来，放在描述里同一个位置：两张抽屉并排看时版式一致，
// 读的人不必为每册重学一遍哪格是哪格。
//
// 序照读口交回的修订号升序，页面不重排：从上往下读就是「登记 → （更正）→ 停用」这条链本身的次序，
// 抽屉头上显的最新修订落在最下面一格。不做 diff——两笔之间改了什么由并排的描述让人自己比
// （读口只交事实），这里不猜「这一笔改了名字还是换了依据」。

/** 与 @idpxyz/ui-primitives 的 TimelineItem 同形的那几格；只用到 default / warning 两种色调。 */
export interface RevisionTimelineItem {
  id: string;
  title: string;
  description: string;
  timestamp: string;
  variant: 'default' | 'warning';
}

/**
 * 两册修订行的公共部分。适配方的记录类型只需在结构上包含这几格；各册多出的格（partyId / partyName）
 * 不在这里——它们是内容，经 `content` 回调进描述。
 */
export interface IdentityRevisionRecord {
  revision: number;
  basis: string;
  effectiveFrom: string;
  deactivatedAt?: string;
  deactivationBasis?: string;
  registeredAt: string;
}

/**
 * 一笔修订一项。停用那一笔标题带`已停用`（CONTEXT 原词）、色调 warning；其余笔只显修订号——读口没给
 * 每一笔算状态（理由在 api.ts 各修订记录类型的注释），这里也不替它算：历史上被顶替的那几笔算出来的
 * 「状态」不是任何时刻的事实。
 *
 * 三个时刻都经注入的 `format`（页面传 moment.ts 的 formatInstant——它依赖装配点配置的呈现时区，
 * 纯函数不该自己去碰那个全局），一格也不原样漏 ISO 串。
 *
 * `content` 交回这一笔本册特有的那段描述（不含分隔符），拼在「依据 · 生效自」之后、停用两件之前。
 */
export function identityRevisionTimeline<Row extends IdentityRevisionRecord>(
  revisions: readonly Row[],
  format: (iso: string) => string,
  content: (row: Row) => string,
): RevisionTimelineItem[] {
  return revisions.map((row) => {
    const deactivated = row.deactivatedAt !== undefined;
    let description = `依据 ${row.basis} · 生效自 ${format(row.effectiveFrom)} · ${content(row)}`;
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
 * 历史区头上的计数句。零笔说的是「册上没有这个 <主语> 的修订」——那是读口交回的如实答案（两册都裁
 * 200 + []），与「没问到」（未配置 / 出错，页面另显）分得开；抽屉只对列表里的行开，所以零笔在正常数据下
 * 不会出现，出现了就该去查写侧。主语由适配方交进来：判读能共用，句子里点名的册不能共用。
 */
export function identityRevisionHistoryNote(count: number, subject: string): string {
  if (count === 0) return `登记册上没有这个${subject}的修订。`;
  return `共 ${count} 笔修订，按修订号从早到晚。`;
}
