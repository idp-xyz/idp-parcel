import { useEffect, useState } from 'react';
import { Button, Timeline } from '@idpxyz/ui-primitives';
import type { ApiResult } from '../catalogue-api';
import { problemNote } from './presentation';
import type { RevisionTimelineItem } from './revision-timeline';

// 参与方身份两册（责任法人、业务参与方）详情抽屉「修订历史」区的共用件（票 admin-web-group-legal-entities/13 第 9 条）。
// 此前 GroupLegalEntitiesPage 与 BusinessPartiesPage 各持一份同形的历史区组件（票 03 立、票 12 照抄），只在读口、回显
// 标识的字段名、主语两字与判读函数上不同；把「何时重取」这条规则改一次要改两处，本条正是为改它而来，于是抽成一份，
// 两册各交一份 RevisionHistoryRegister 进来。判读仍在 *-revisions.ts（纯函数，node:test 钉着），这里只摆。
//
// 何时重取：按 subjectId **与 revision** 记依赖。票 13 第 6 条之后 useRegisterList 重取不再清上一份答案，抽屉在列表
// 重取期间开着不卸载；登记签给同一对象登了下一笔（或停用），新答案到来后行的 revision 变了而 subjectId 没变——
// 只按 subjectId 记依赖会让历史区继续显上一条链。票 12 (c) 在参与方页用 key 带修订号补这一格、法人页没有；两页
// 现在同一做法、由本件一处守住，调用方不必记得给 key。
//
// 每次换行或换修订重取，未回的旧请求按 cancelled 丢；答案顶层回显的标识再核一次，对不上就不摆——摆一段别的对象
// 的历史比空着更坏。读口墙前照旧显未配置：403 是「今天没有问到」，不是「这个对象没有历史」，两句续办不同（前者去
// 配渠道，后者去查写侧），措辞把这一格点出来；不用 UnconfiguredState 大块——抽屉里一段区，一句话够。

/** 一册交给历史区的那几样：读口、回显标识、判读、主语与读口路径。作模块级常量交进来，见 load 的说明。 */
export interface RevisionHistoryRegister<Body> {
  /** 主语（「法人」/「参与方」），进未配置与回显不符两句。 */
  subject: string;
  /** 读口的方法与路径，进未配置那句，让人知道去配哪一口。 */
  endpoint: string;
  /**
   * 按标识取整条修订链。要是稳定引用（模块级函数）：它在 effect 依赖里，写成内联箭头会每次渲染重取。
   */
  load: (subjectId: string) => Promise<ApiResult<Body>>;
  /** 答案顶层回显的标识——与所问不符即丢弃。 */
  echoedSubjectId: (body: Body) => string;
  /** 判读成时间线条目（在 *-revisions.ts）。 */
  timelineOf: (body: Body) => RevisionTimelineItem[];
  /** 条数注释句（在 *-revisions.ts）。 */
  noteOf: (count: number) => string;
}

export function RevisionHistorySection<Body>({
  register,
  subjectId,
  revision,
}: {
  register: RevisionHistoryRegister<Body>;
  /** 所问对象的标识。 */
  subjectId: string;
  /** 列表行上的最新修订号；变了就重取，理由见文件头。 */
  revision: number;
}) {
  const { load } = register;
  const [answer, setAnswer] = useState<ApiResult<Body> | null>(null);
  const [reloadKey, setReloadKey] = useState(0);

  useEffect(() => {
    let cancelled = false;
    setAnswer(null);
    void load(subjectId).then((next) => {
      if (!cancelled) setAnswer(next);
    });
    return () => {
      cancelled = true;
    };
  }, [load, subjectId, revision, reloadKey]);

  const retry = () => setReloadKey((value) => value + 1);
  const note = 'mt-1 text-[12px] text-idpxyz-textMuted';

  if (answer === null) {
    return <p className={note}>正在读取修订历史…</p>;
  }
  if (answer.kind === 'unconfigured') {
    return (
      <p className={note}>
        访问通道尚未配置：修订历史读口（{register.endpoint}）当前不可用（403）。这不是「这个{register.subject}
        没有历史」——今天没有问到；配置该上下文的访问通道后重新打开抽屉。
      </p>
    );
  }
  if (answer.kind === 'callerProblem') {
    return (
      <p className={note}>
        调用方式问题（HTTP {answer.status}）：{problemNote(answer.code)}
      </p>
    );
  }
  if (answer.kind === 'noAnswer' || answer.kind === 'transport') {
    return (
      <p className={note}>
        {answer.kind === 'noAnswer'
          ? `服务端未形成答案（HTTP ${answer.status}）：${problemNote(answer.code)}`
          : `无法连接主数据读取服务：${answer.message}`}
        <Button variant="ghost" size="sm" className="ml-2" onClick={retry}>
          重试
        </Button>
      </p>
    );
  }
  const echoed = register.echoedSubjectId(answer.body);
  if (echoed !== subjectId) {
    return (
      <p className={note}>
        答案回显的{register.subject}（{echoed}）与所问（{subjectId}）不符，已丢弃。
        <Button variant="ghost" size="sm" className="ml-2" onClick={retry}>
          重试
        </Button>
      </p>
    );
  }

  const items = register.timelineOf(answer.body);
  return (
    <>
      <p className={note}>{register.noteOf(items.length)}</p>
      {items.length > 0 ? <Timeline className="mt-3" items={items} /> : null}
    </>
  );
}
