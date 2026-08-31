import { useEffect, useState, type ReactNode } from 'react';
import {
  Table,
  TableHeader,
  TableBody,
  TableRow,
  TableHead,
  TableCell,
} from '@idpxyz/ui-primitives';
import {
  DetailPageTemplate,
  type DetailSection,
  type TemplateViewState,
} from '../../templates';
import { moduleInfoById } from '../../navigation';
import type { ApiResult } from '../catalogue-api';
import { formatInstant, formatRange } from '../catalogue-view';
import {
  listAuthorityIntervals,
  listResumptions,
  listSuspensions,
  type AuthorityIntervalListResponseBody,
  type AuthorityIntervalRecord,
  type ResumptionListResponseBody,
  type ResumptionRecord,
  type SuspensionListResponseBody,
  type SuspensionRecord,
} from './api';

// 主责上下文与场景出处的唯一来源是 navigation 的 moduleInfoById，只读引用，不抄第二份。
const info = moduleInfoById['stage-admission'];

// 本页接 GET /governance-registers 三册（票 admin-skeleton-closure-batch/02 阶段二）。
// 旧骨架「归属尚未裁定，不在本管理台接线」的未配置文案由 ADR-0083 取代：归属已裁定
// ——受众是运营方的治理操作员（与运行 parcel-governance-register 的是同一方，
// syn-wall-door-audit 票 12），承载面就是本管理台（ADR-0020），此前错的不是位置，
// 是页面没把作用域说出口。本页依 ADR-0083 Decision 四明示：登记的是产品实例级治理
// 事实（本产品此刻拿什么去评审、谁在写生产），不按租户隔离。
//
// 仍选 DetailPageTemplate：治理者看的是「这个产品实例」一个对象的登记面，不是从
// 队列里逐条消化的复核件。三册各成一个业务区块；阶段评审与对象级接管两格如实说明
// 属第二批未开（票 12 首批三类裁定），不留白也不造数。决定动作不在本页——登记走
// 受控 CLI，HTTP 命令入口不存在（ADR-0083 Consequences），headerActions 不设。

/** 三册通用的区块小表：空册如实说「册空」而不是留白——留白读起来像数据缺件。 */
function registryTable<Row>(
  rows: Row[],
  headers: string[],
  renderRow: (row: Row) => ReactNode,
  emptyNote: string,
) {
  if (rows.length === 0) {
    return <p className="text-[12px] text-idpxyz-textMuted">{emptyNote}</p>;
  }
  return (
    <Table>
      <TableHeader>
        <TableRow>
          {headers.map((header) => (
            <TableHead key={header}>{header}</TableHead>
          ))}
        </TableRow>
      </TableHeader>
      <TableBody>{rows.map(renderRow)}</TableBody>
    </Table>
  );
}

const cellClass = 'font-mono text-xs';

// 空册文案三册同句：读取入口已配置是本态与未配置态的分界（ADR-0077 Decision 四）。
const emptyRegisterNote =
  '登记册为空：读取入口已配置，尚无登记。登记走 parcel-governance-register 受控 CLI，本页不预置数据。';

/** 第二批未开的两格共用说明形：说的是机制分批，不是数据缺件。 */
function secondBatchNote(text: string) {
  return <p className="text-[12px] text-idpxyz-textMuted">{text}</p>;
}

/**
 * 阶段决定与暂停恢复：产品实例级治理登记册的查阅面（ADR-0083）。
 * 三册照登转写——生产权威区间、暂停决定、恢复决定；恢复的盘点明细住 jsonb 属
 * 详情读法，本页列引用与时点。页面只读，不设登记与决定动作。
 */
export function StageAdmissionPage() {
  const [reloadKey, setReloadKey] = useState(0);
  const [intervalsAnswer, setIntervalsAnswer] =
    useState<ApiResult<AuthorityIntervalListResponseBody> | null>(null);
  const [suspensionsAnswer, setSuspensionsAnswer] =
    useState<ApiResult<SuspensionListResponseBody> | null>(null);
  const [resumptionsAnswer, setResumptionsAnswer] =
    useState<ApiResult<ResumptionListResponseBody> | null>(null);

  useEffect(() => {
    let cancelled = false;
    setIntervalsAnswer(null);
    setSuspensionsAnswer(null);
    setResumptionsAnswer(null);
    void listAuthorityIntervals().then((answer) => {
      if (!cancelled) setIntervalsAnswer(answer);
    });
    void listSuspensions().then((answer) => {
      if (!cancelled) setSuspensionsAnswer(answer);
    });
    void listResumptions().then((answer) => {
      if (!cancelled) setResumptionsAnswer(answer);
    });
    return () => {
      cancelled = true;
    };
  }, [reloadKey]);

  const retry = () => setReloadKey((value) => value + 1);
  const answers: Array<ApiResult<unknown> | null> = [
    intervalsAnswer,
    suspensionsAnswer,
    resumptionsAnswer,
  ];

  // 三册合成一个页面态。未配置优先于错误：三册共用同一个 Intake，403 是对整个
  // 读取入口的确定陈述；错误态只取第一个失败册的事实，重试一次重取三册。
  let viewState: TemplateViewState = { kind: 'ready' };
  const unconfigured = answers.find((answer) => answer?.kind === 'unconfigured');
  const failed = answers.find(
    (answer) =>
      answer !== null &&
      answer.kind !== 'outcome' &&
      answer.kind !== 'unconfigured',
  );
  if (unconfigured) {
    viewState = {
      kind: 'unconfigured',
      title: '访问通道尚未配置',
      description: `${info.owner}的登记册读取入口（GET /governance-registers）当前不可用；这不是「登记册为空」。`,
      facts: {
        owner: info.owner,
        source: info.source,
        unlock: '配置该上下文的访问通道后重新查询；页面不会用演示数据代替。',
      },
    };
  } else if (failed) {
    viewState =
      failed.kind === 'transport'
        ? {
            kind: 'error',
            title: '无法连接治理登记册读取服务',
            description: failed.message,
            onRetry: retry,
          }
        : {
            kind: 'error',
            title:
              failed.kind === 'noAnswer'
                ? `服务端未形成答案（HTTP ${failed.status}）`
                : `调用方式问题（HTTP ${failed.status}）`,
            description: `服务端问题码：${failed.code}。重试将重新查询三册。`,
            onRetry: failed.kind === 'noAnswer' ? retry : undefined,
          };
  } else if (answers.some((answer) => answer === null)) {
    viewState = { kind: 'loading' };
  }

  const intervals =
    intervalsAnswer?.kind === 'outcome' ? intervalsAnswer.body.intervals : [];
  const suspensions =
    suspensionsAnswer?.kind === 'outcome' ? suspensionsAnswer.body.suspensions : [];
  const resumptions =
    resumptionsAnswer?.kind === 'outcome' ? resumptionsAnswer.body.resumptions : [];

  const sections: DetailSection[] = [
    {
      id: 'authority-intervals',
      title: '生产权威区间',
      description:
        '某类对象的某项能力、某种事实，当下由哪条线在写生产；到期时点缺席即开放区间（当前权威），不编造「无限远」时刻。',
      content: registryTable<AuthorityIntervalRecord>(
        intervals,
        ['对象作用域', '能力', '事实种类', '生产权威', '生效区间', '登记时间'],
        (row) => (
          <TableRow key={`${row.objectScope}:${row.capability}:${row.factKind}:${row.fromAt}`}>
            <TableCell className={cellClass}>{row.objectScope}</TableCell>
            <TableCell className={cellClass}>{row.capability}</TableCell>
            <TableCell className={cellClass}>{row.factKind}</TableCell>
            <TableCell className={cellClass}>{row.authority}</TableCell>
            <TableCell className={cellClass}>{formatRange(row.fromAt, row.toAt)}</TableCell>
            <TableCell className={cellClass}>{formatInstant(row.insertedAt)}</TableCell>
          </TableRow>
        ),
        emptyRegisterNote,
      ),
    },
    {
      id: 'suspensions',
      title: '暂停决定',
      description: '停下来永远安全：暂停立即生效，在途件的处置说明照登原文。',
      content: registryTable<SuspensionRecord>(
        suspensions,
        ['暂停标识', '作用域', '触发来源', '依据', '证据引用', '执行人', '发生时点', '生效时点', '在途处置'],
        (row) => (
          <TableRow key={row.suspensionId}>
            <TableCell className={cellClass}>{row.suspensionId}</TableCell>
            <TableCell className={cellClass}>{row.scope}</TableCell>
            <TableCell className={cellClass}>{row.triggerSource}</TableCell>
            <TableCell className={cellClass}>{row.basis}</TableCell>
            <TableCell className={cellClass}>{row.evidence}</TableCell>
            <TableCell className={cellClass}>{row.executedBy}</TableCell>
            <TableCell className={cellClass}>{formatInstant(row.occurredAt)}</TableCell>
            <TableCell className={cellClass}>{formatInstant(row.effectiveAt)}</TableCell>
            <TableCell className="text-xs">{row.inTransitNote}</TableCell>
          </TableRow>
        ),
        emptyRegisterNote,
      ),
    },
    {
      id: 'resumptions',
      title: '恢复决定',
      description:
        '恢复必须有放行证据与一致性核对；盘点明细属详情读法，本格列引用与时点。',
      content: registryTable<ResumptionRecord>(
        resumptions,
        ['暂停标识', '放行证据', '一致性核对', '盘点时点', '决定人', '决定时点', '生效时点'],
        (row) => (
          <TableRow key={`${row.suspensionId}:${row.decidedAt}`}>
            <TableCell className={cellClass}>{row.suspensionId}</TableCell>
            <TableCell className={cellClass}>{row.releaseEvidence}</TableCell>
            <TableCell className={cellClass}>{row.consistencyCheck}</TableCell>
            <TableCell className={cellClass}>{formatInstant(row.inventoryTakenAt)}</TableCell>
            <TableCell className={cellClass}>{row.decidedBy}</TableCell>
            <TableCell className={cellClass}>{formatInstant(row.decidedAt)}</TableCell>
            <TableCell className={cellClass}>{formatInstant(row.effectiveAt)}</TableCell>
          </TableRow>
        ),
        emptyRegisterNote,
      ),
    },
    {
      id: 'stage-review',
      title: '阶段评审',
      content: secondBatchNote(
        '第二批未开。首批登记口只开三类——生产权威区间、暂停、恢复（syn-wall-door-audit 票 12 首批裁定）；评审材料聚合与阶段判定的登记口属第二批。本格说的是机制分批，不是数据缺件，落地前不上桩数据。',
      ),
    },
    {
      id: 'takeover',
      title: '对象级接管',
      content: secondBatchNote(
        '第二批未开。对象级接管（逐对象把生产权威从试点线移回既有实现）的登记口属第二批；首批的暂停机制已覆盖「立即停下来」这半边。本格说的是机制分批，不是数据缺件，落地前不上桩数据。',
      ),
    },
  ];

  return (
    <DetailPageTemplate
      title={info.title}
      description={`${info.owner}——本页登记的是产品实例级治理事实，不按租户隔离（ADR-0083）`}
      basicTitle="登记面"
      basicFields={[
        {
          label: '作用域',
          value: '产品实例级：本产品此刻拿什么去评审、谁在写生产；无租户维（ADR-0083）',
        },
        {
          label: '登记口',
          value: 'parcel-governance-register 受控 CLI；本页只读，不设登记与决定动作',
        },
        { label: '权威区间', value: `${intervals.length} 行` },
        { label: '暂停决定', value: `${suspensions.length} 行` },
        { label: '恢复决定', value: `${resumptions.length} 行` },
      ]}
      sections={sections}
      viewState={viewState}
    />
  );
}
