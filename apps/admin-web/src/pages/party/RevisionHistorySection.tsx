import { useEffect, useState } from 'react';
import { Button, Timeline } from '@idpxyz/ui-primitives';
import { isOperatorRefusal } from '../../auth/access-refusal';
import { OperatorRefusalNote, SectionError, TechnicalDetails, httpRows } from '../../components/states';
import { revealClass, useDelayedReveal } from '../../components/use-delayed-reveal';
import type { ApiResult } from '../catalogue-api';
import { problemNote } from './presentation';
import type { RevisionTimelineItem } from './revision-timeline';

// 参与方身份各册（责任法人对象页、业务参与方抽屉……）「修订历史」区的共用件（票 admin-web-group-legal-entities/13 第 9 条）。
// 此前 GroupLegalEntitiesPage 与 BusinessPartiesPage 各持一份同形的历史区组件（票 03 立、票 12 照抄），只在读口、回显
// 标识的字段名、主语一词与判读函数上不同；「何时重取」这条规则一改就得每份副本各改一遍，本条正是为改它而来，于是抽成
// 一份，各册交一份 RevisionHistoryRegister 进来。判读仍在 *-revisions.ts（纯函数，node:test 钉着），这里只摆。
//
// 何时重取：按 subjectId **与 revision** 记依赖。票 13 第 6 条之后 useRegisterList 重取不再清上一份答案，抽屉在列表
// 重取期间开着不卸载；登记签给同一对象登了下一笔（或停用），新答案到来后行的 revision 变了而 subjectId 没变——
// 只按 subjectId 记依赖会让历史区继续显上一条链。票 12 (c) 在参与方页用 key 带修订号补这一格、法人页没有；各消费方
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
  /**
   * 可选的页面内缓存（公司主体那一册给，见 legal-entity-read-cache.ts）：`peek` 取这个对象这一版读过的那份先显，`loadCached`
   * 经缓存去问。调用方同时给 `cacheVersion` 才用；两者都要是稳定引用，理由同 load。
   */
  peek?: (subjectId: string, version: string) => ApiResult<Body> | undefined;
  loadCached?: (subjectId: string, version: string) => Promise<ApiResult<Body>>;
}

/**
 * 与时间线同轮廓的占位：只摆一行「正在读取」时这一签先塌成一行、答案到了再撑开，看着是一闪。答得快时不露
 * （components/use-delayed-reveal.ts），位置照占。
 */
function RevisionTimelineSkeleton() {
  const revealed = useDelayedReveal();
  return (
    <div role="status" aria-label="正在读取修订历史" className={`mt-2 flex animate-pulse flex-col gap-3 ${revealClass(revealed)}`}>
      {[0, 1].map((index) => (
        <div key={index} className="flex gap-3">
          <div className="mt-1 h-3 w-3 shrink-0 rounded-full bg-idpxyz-hover" />
          <div className="flex-1 rounded-md border border-idpxyz-border px-4 py-3">
            <div className="h-3.5 w-32 rounded bg-idpxyz-hover" />
            <div className="mt-2 h-3 w-3/4 rounded bg-idpxyz-hover" />
          </div>
        </div>
      ))}
    </div>
  );
}

export function RevisionHistorySection<Body>({
  register,
  subjectId,
  revision,
  cacheVersion,
}: {
  register: RevisionHistoryRegister<Body>;
  /** 所问对象的标识。 */
  subjectId: string;
  /** 列表行上的最新修订号；变了就重取，理由见文件头。 */
  revision: number;
  /** 给了且册带缓存时，读过的那份先显、后台再问（左边换行时这一签不再先清空再撑开）。 */
  cacheVersion?: string;
}) {
  const { load, peek, loadCached } = register;
  const [reloadKey, setReloadKey] = useState(0);
  // 答案钉在发问那一刻的键上：换了对象或修订，旧答案即使后到也不算数；没答到之前先看缓存。
  const requestKey = `${subjectId}\u0000${revision}\u0000${reloadKey}`;
  const [result, setResult] = useState<{ key: string; answer: ApiResult<Body> } | null>(null);
  const useCache = cacheVersion !== undefined && peek !== undefined && loadCached !== undefined;

  useEffect(() => {
    let cancelled = false;
    const request = useCache ? loadCached(subjectId, cacheVersion) : load(subjectId);
    void request.then((next) => {
      if (!cancelled) setResult({ key: requestKey, answer: next });
    });
    return () => {
      cancelled = true;
    };
  }, [load, loadCached, useCache, subjectId, cacheVersion, requestKey]);

  const retry = () => setReloadKey((value) => value + 1);
  const note = 'mt-1 text-[12px] text-idpxyz-breadcrumb';
  const answer = result?.key === requestKey ? result.answer : (useCache ? peek(subjectId, cacheVersion) ?? null : null);

  if (answer === null) return <RevisionTimelineSkeleton />;
  if (answer.kind === 'unconfigured') {
    return (
      <p className={note}>
        暂时读不到修订历史：接入渠道还没有配置。这不代表这个{register.subject}没有历史；配置好之后再看这一段。
      </p>
    );
  }
  if (isOperatorRefusal(answer)) {
    return <OperatorRefusalNote kind={answer.kind} what="修订历史" className={note} />;
  }
  if (answer.kind === 'callerProblem') {
    return (
      <div className={note}>
        <p>调用方式问题：{problemNote(answer.code)}</p>
        <TechnicalDetails rows={httpRows(answer.status, answer.code)} />
      </div>
    );
  }
  // 这两态是「这一段没读到、可重试」——区块级错误（票 admin-web-ux-alignment/06 第 2 条首用）：只红历史区这一段，抽屉
  // 其余各格照常。标题 / 说明的分法与目录页 catalogueViewState 同一口径：状态句作标题，问题码注释或传输层消息作说明。
  if (answer.kind === 'noAnswer') {
    return (
      <SectionError
        title="服务端没有形成答案"
        description={problemNote(answer.code)}
        technical={httpRows(answer.status, answer.code)}
        onRetry={retry}
      />
    );
  }
  if (answer.kind === 'transport') {
    return <SectionError title="无法连接主数据读取服务" description={answer.message} onRetry={retry} />;
  }
  const echoed = register.echoedSubjectId(answer.body);
  if (echoed !== subjectId) {
    return (
      <p className={note}>
        答案回显的{register.subject}（{echoed}）与所问（{subjectId}）不符，已丢弃。
        <Button variant="ghost" size="sm" className="ml-2 text-[12px] text-idpxyz-breadcrumb" onClick={retry}>
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
