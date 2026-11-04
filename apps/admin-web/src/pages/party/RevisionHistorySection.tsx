import { Button, Timeline } from '@idpxyz/ui-primitives';
import { isOperatorRefusal } from '../../auth/access-refusal';
import { OperatorRefusalNote, SectionError, TechnicalDetails, httpRows } from '../../components/states';
import { revealClass, useDelayedReveal } from '../../components/use-delayed-reveal';
import type { ApiResult } from '../catalogue-api';
import { useAnswer, type AnswerKey } from '../../query/answers';
import { problemNote } from './presentation';
import type { RevisionTimelineItem } from './revision-timeline';

// 参与方身份各册（责任法人对象页、业务参与方抽屉……）「修订历史」区的共用件（票 admin-web-group-legal-entities/13 第 9 条）。
// 此前 GroupLegalEntitiesPage 与 BusinessPartiesPage 各持一份同形的历史区组件（票 03 立、票 12 照抄），只在读口、回显
// 标识的字段名、主语一词与判读函数上不同；「何时重取」这条规则一改就得每份副本各改一遍，本条正是为改它而来，于是抽成
// 一份，各册交一份 RevisionHistoryRegister 进来。判读仍在 *-revisions.ts（纯函数，node:test 钉着），这里只摆。
//
// 何时重取：键里带 subjectId **与版本**（经取数层读，ADR-0360）。useRegisterList 重取不清上一份答案，抽屉在列表重取期间
// 开着不卸载；登记签给同一对象登了下一笔（或停用），新答案到来后行的 revision 变了而 subjectId 没变——键里只有 subjectId
// 会让历史区继续显上一条链。版本由调用方给（公司主体给身份版本，与签上计数同键），不给就按行上的修订号；由本件一处守住，
// 调用方不必记得给 key。
//
// 换行或换版本就是另一把键，慢答案落在它自己的键上，不会盖到眼前这一格；答案顶层回显的标识再核一次，对不上就不摆——
// 摆一段别的对象的历史比空着更坏。读口墙前照旧显未配置：403 是「今天没有问到」，不是「这个对象没有历史」，两句续办不同（前者去
// 配渠道，后者去查写侧），措辞把这一格点出来；不用 UnconfiguredState 大块——抽屉里一段区，一句话够。

/** 一册交给历史区的那几样：读口、回显标识、判读、主语与读口路径。作模块级常量交进来，见 load 的说明。 */
export interface RevisionHistoryRegister<Body> {
  /** 主语（「法人」/「参与方」），进未配置与回显不符两句。 */
  subject: string;
  /** 读口的方法与路径，进未配置那句，让人知道去配哪一口。 */
  endpoint: string;
  /** 按标识取整条修订链。 */
  load: (subjectId: string) => Promise<ApiResult<Body>>;
  /** 这条链在取数层的键（ADR-0360）：同一条链在别处也读时给同一把键，缓存是同一份。 */
  answerKey: (subjectId: string, version: string) => AnswerKey;
  /** 答案顶层回显的标识——与所问不符即丢弃。 */
  echoedSubjectId: (body: Body) => string;
  /** 判读成时间线条目（在 *-revisions.ts）。 */
  timelineOf: (body: Body) => RevisionTimelineItem[];
  /** 条数注释句（在 *-revisions.ts）。 */
  noteOf: (count: number) => string;
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
  version,
}: {
  register: RevisionHistoryRegister<Body>;
  /** 所问对象的标识。 */
  subjectId: string;
  /** 列表行上的最新修订号；不给版本时按它进键，理由见文件头。 */
  revision: number;
  /** 进键的版本；公司主体给身份版本（legalEntityReadVersion），与签上计数同键。不给按修订号。 */
  version?: string;
}) {
  const { answer, retry } = useAnswer(register.answerKey(subjectId, version ?? String(revision)), () => register.load(subjectId));
  const note = 'mt-1 text-[12px] text-idpxyz-breadcrumb';

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
