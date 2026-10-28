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
  listTakeovers,
  type AuthorityIntervalListResponseBody,
  type AuthorityIntervalRecord,
  type ResumptionListResponseBody,
  type ResumptionRecord,
  type SuspensionListResponseBody,
  type SuspensionRecord,
  type TakeoverListResponseBody,
  type TakeoverRecord,
} from './api';
import { PrincipalRef } from '../../components/PrincipalRef';

// 主责上下文与场景出处的唯一来源是 navigation 的 moduleInfoById，只读引用，不抄第二份。
const info = moduleInfoById['stage-admission'];

// 本页接 GET /governance-registers 四册（票 admin-skeleton-closure-batch/02 阶段二，
// 接管册 ADR-0155）。
// 旧骨架「归属尚未裁定，不在本管理台接线」的未配置文案由 ADR-0083 取代：归属已裁定
// ——受众是运营方的治理操作员（与运行 parcel-governance-register 的是同一方，
// syn-wall-door-audit 票 12），承载面就是本管理台（ADR-0020），此前错的不是位置，
// 是页面没把作用域说出口。本页依 ADR-0083 Decision 四明示：登记的是产品实例级治理
// 事实（本产品此刻拿什么去评审、谁在写生产），不按租户隔离。
//
// 仍选 DetailPageTemplate：治理者看的是「这个产品实例」一个对象的登记面，不是从
// 队列里逐条消化的复核件。四册各成一个业务区块；阶段评审查阅仍未开，如实说明，
// 不留白也不造数。决定动作不在本页——登记走受控 CLI，HTTP 命令入口不存在
// （ADR-0083 Consequences，接管写面仍另票），headerActions 不设。

/** 四册通用的区块小表：空册如实说「册空」而不是留白——留白读起来像数据缺件。 */
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

// 空册文案四册同句：读取入口已配置是本态与未配置态的分界（ADR-0077 Decision 四）。
const emptyRegisterNote = '还没有登记。登记由管理员用命令行工具完成，本页只看。';

/** 查阅仍未开的那一格：说的是查阅分批，不是数据缺件。 */
function secondBatchNote(text: string) {
  return <p className="text-[12px] text-idpxyz-textMuted">{text}</p>;
}

/**
 * 阶段决定与暂停恢复：产品实例级治理登记册的查阅面（ADR-0083）。
 * 四册照登转写——生产权威区间、暂停决定、恢复决定、对象级接管；恢复与接管的盘点
 * 明细住 jsonb，属详情读法，本页列引用与时点。页面只读，不设登记与决定动作。
 */
export function StageAdmissionPage() {
  const [reloadKey, setReloadKey] = useState(0);
  const [intervalsAnswer, setIntervalsAnswer] =
    useState<ApiResult<AuthorityIntervalListResponseBody> | null>(null);
  const [suspensionsAnswer, setSuspensionsAnswer] =
    useState<ApiResult<SuspensionListResponseBody> | null>(null);
  const [resumptionsAnswer, setResumptionsAnswer] =
    useState<ApiResult<ResumptionListResponseBody> | null>(null);
  const [takeoversAnswer, setTakeoversAnswer] =
    useState<ApiResult<TakeoverListResponseBody> | null>(null);

  useEffect(() => {
    let cancelled = false;
    setIntervalsAnswer(null);
    setSuspensionsAnswer(null);
    setResumptionsAnswer(null);
    setTakeoversAnswer(null);
    void listAuthorityIntervals().then((answer) => {
      if (!cancelled) setIntervalsAnswer(answer);
    });
    void listSuspensions().then((answer) => {
      if (!cancelled) setSuspensionsAnswer(answer);
    });
    void listResumptions().then((answer) => {
      if (!cancelled) setResumptionsAnswer(answer);
    });
    void listTakeovers().then((answer) => {
      if (!cancelled) setTakeoversAnswer(answer);
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
    takeoversAnswer,
  ];

  // 四册合成一个页面态。接入那一格（未配置、需要登录、没有授予）优先于错误：四册共用同一个 Intake，
  // 那是对整个读取入口的确定陈述；错误态只取第一个失败册的事实，重试一次重取四册。
  let viewState: TemplateViewState = { kind: 'ready' };
  const unconfigured = answers.find((answer) => answer?.kind === 'unconfigured');
  const refused = answers.find(
    (answer) => answer?.kind === 'loginRequired' || answer?.kind === 'notGranted',
  );
  const failed = answers.find(
    (answer) =>
      answer !== null &&
      answer.kind !== 'outcome' &&
      answer.kind !== 'unconfigured' &&
      answer.kind !== 'loginRequired' &&
      answer.kind !== 'notGranted',
  );
  if (unconfigured) {
    viewState = {
      kind: 'unconfigured',
      title: '访问通道尚未配置',
      description: '治理登记的读取入口当前不可用；这不是「还没有登记」。',
      facts: {
        owner: info.owner,
        source: info.source,
        unlock: '配置该上下文的访问通道后重新查询；页面不会用演示数据代替。',
      },
    };
  } else if (refused) {
    viewState = { kind: refused.kind, facts: { owner: info.owner, source: info.source } };
  } else if (failed) {
    viewState =
      failed.kind === 'transport'
        ? {
            kind: 'error',
            title: '暂时读不到治理登记',
            description: failed.message,
            onRetry: retry,
          }
        : {
            kind: 'error',
            title: failed.kind === 'noAnswer' ? '系统暂时没能读出治理登记' : '读取方式有问题',
            description: failed.kind === 'noAnswer' ? '请稍后重试，重试会重新读取四本登记。' : `请刷新页面后重试；仍然不行请联系系统管理员（错误码 ${failed.code}）。`,
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
  const takeovers =
    takeoversAnswer?.kind === 'outcome' ? takeoversAnswer.body.takeovers : [];

  const sections: DetailSection[] = [
    {
      id: 'authority-intervals',
      title: '谁在写生产',
      description: '哪类对象的哪项能力、哪种事实，眼下由哪条线在生产上写；没有截止时间就是现行。',
      content: registryTable<AuthorityIntervalRecord>(
        intervals,
        ['对象范围', '能力', '事实种类', '由谁写', '生效期间', '登记时间'],
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
      title: '暂停',
      description: '暂停立即生效；在途货物怎么处理照登原文。',
      content: registryTable<SuspensionRecord>(
        suspensions,
        ['暂停编号', '范围', '触发来源', '依据', '证据', '执行人', '发生时间', '生效时间', '在途货物怎么办'],
        (row) => (
          <TableRow key={row.suspensionId}>
            <TableCell className={cellClass}>{row.suspensionId}</TableCell>
            <TableCell className={cellClass}>{row.scope}</TableCell>
            <TableCell className={cellClass}>{row.triggerSource}</TableCell>
            <TableCell className={cellClass}>{row.basis}</TableCell>
            <TableCell className={cellClass}>{row.evidence}</TableCell>
            <TableCell className={cellClass}><PrincipalRef value={row.executedBy} /></TableCell>
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
      title: '恢复',
      description: '恢复要有放行证据和一致性核对；盘点明细另看，这里列引用与时间。',
      content: registryTable<ResumptionRecord>(
        resumptions,
        ['暂停编号', '放行证据', '一致性核对', '盘点时间', '决定人', '决定时间', '生效时间'],
        (row) => (
          <TableRow key={`${row.suspensionId}:${row.decidedAt}`}>
            <TableCell className={cellClass}>{row.suspensionId}</TableCell>
            <TableCell className={cellClass}>{row.releaseEvidence}</TableCell>
            <TableCell className={cellClass}>{row.consistencyCheck}</TableCell>
            <TableCell className={cellClass}>{formatInstant(row.inventoryTakenAt)}</TableCell>
            <TableCell className={cellClass}><PrincipalRef value={row.decidedBy} /></TableCell>
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
      content: secondBatchNote('阶段评审的查看还没有接通；登记已可用命令行工具完成。这里不列表、不造数。'),
    },
    {
      id: 'takeover',
      title: '接管',
      description: '原来那条线停写之后，指定范围由新的一条线接着写。这里列停写证据、期间与时间；盘点明细另看。',
      content: registryTable<TakeoverRecord>(
        takeovers,
        [
          '对象范围',
          '能力',
          '事实种类',
          '由谁写',
          '生效期间',
          '停写证据',
          '已接受事实',
          '外部未决',
          '实际控制',
          '责任',
          '下一步',
          '盘点时间',
          '生效时间',
        ],
        (row) => (
          <TableRow
            key={`${row.objectScope}:${row.capability}:${row.factKind}:${row.authority}:${row.fromAt}`}
          >
            <TableCell className={cellClass}>{row.objectScope}</TableCell>
            <TableCell className={cellClass}>{row.capability}</TableCell>
            <TableCell className={cellClass}>{row.factKind}</TableCell>
            <TableCell className={cellClass}>{row.authority}</TableCell>
            <TableCell className={cellClass}>{formatRange(row.fromAt, row.toAt)}</TableCell>
            <TableCell className={cellClass}>{row.stopEvidence}</TableCell>
            <TableCell className="text-xs">{row.acceptedFacts}</TableCell>
            <TableCell className="text-xs">{row.pendingExternals}</TableCell>
            <TableCell className="text-xs">{row.actualControl}</TableCell>
            <TableCell className="text-xs">{row.responsibilities}</TableCell>
            <TableCell className="text-xs">{row.nextAction}</TableCell>
            <TableCell className={cellClass}>{formatInstant(row.inventoryTakenAt)}</TableCell>
            <TableCell className={cellClass}>{formatInstant(row.effectiveAt)}</TableCell>
          </TableRow>
        ),
        emptyRegisterNote,
      ),
    },
  ];

  return (
    <DetailPageTemplate
      title="试点治理"
      description="试点阶段的四本登记：谁在写生产、暂停、恢复、接管；整个系统一本，不分公司"
      basicTitle="概况"
      basicFields={[
        { label: '登记方式', value: '由管理员用命令行工具登记；本页只看' },
        { label: '谁在写生产', value: `${intervals.length} 条` },
        { label: '暂停', value: `${suspensions.length} 条` },
        { label: '恢复', value: `${resumptions.length} 条` },
        { label: '接管', value: `${takeovers.length} 条` },
      ]}
      sections={sections}
      viewState={viewState}
    />
  );
}
