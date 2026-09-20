import { useEffect, useState } from 'react';
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@idpxyz/ui-primitives';
import { moduleInfoById } from '../../navigation';
import { ListPageTemplate, type ListColumn } from '../../templates';
import { MultiRegistrationPanel, chipClass, type RegistrationTarget } from '../../components/registration';
import { catalogueViewState, formatRange } from '../catalogue-view';
import {
  listVisibilityCatalogues,
  registerVisibilityCatalogue,
  visibilityRegistrationEndpoints,
  type ApiResult,
  type DisclosureCellRecord,
  type VisibilityCatalogueListResponseBody,
  type VisibilityRegistrableCatalogueKind,
} from './catalogue-api';
import {
  autoReleaseLabel,
  disclosableLabel,
  disclosureStateLabels,
  labelOf,
  problemNote,
  registrationOutcomeLabels,
  registrationRefusalReasonLabels,
  registrationSnapshotHints,
  registrationTitles,
  visibilityCatalogueKindLabels,
} from './presentation';

// VE 目录拆三页,本页装**对外披露口径**那组:通知策略与披露策略(票
// admin-web-page-wiring-frontier/02 的页面裁决),外加异常披露规则(票
// ve-disclosure-policy-view/03)。三册同答「对客户说什么、何时必须说」——通知策略管
// 义务与时限,披露策略管每客户四维内容,异常披露规则管某客户的某类异常信号披露与否、
// 能否自动发布、内容从哪来;判断规则与索赔前置各归各页。
//
// **异常披露规则(0023)与披露策略(0012)是相邻的两本册,不是一册的两个名字**:前者按
// 客户 × 信号类型 × 可信度立条目,后者按客户立条目;版本引用同为披露决定带着走的那个
// 披露策略引用,但形状、登记口与消费方都不同。签词、列名与提示句里「规则」「策略」两字
// 一处不混——混了就是把 0023 的条目读成 0012 的条目。
//
// 通知策略册没有版本与区间列:版本化由披露策略引用本身承担(0010),换版即换引用、
// 新旧两行并存,页面不给它补一列假版本。披露策略与异常披露规则行随条目展开,理由与
// 判断规则页同:空版本被登记入口拦死(checkEntries),行展开不藏版本。

const info = moduleInfoById['disclosure-policies'];

type DisclosureKind = 'NOTIFICATION_POLICY' | 'DISCLOSURE_POLICY' | 'EXCEPTION_DISCLOSURE_RULE';

const disclosureKinds: DisclosureKind[] = [
  'NOTIFICATION_POLICY',
  'DISCLOSURE_POLICY',
  'EXCEPTION_DISCLOSURE_RULE',
];

// 登记签铺本页读签里的每一册。异常披露规则的写签跟着本页这枚读签走(票
// ve-disclosure-policy-view/02 步二,03 的读签先落、写签后到):同一本册在哪页读就在哪页登,
// 登进去的结果切回读签就看得见。
const disclosureRegistrableKinds: Extract<DisclosureKind, VisibilityRegistrableCatalogueKind>[] = [
  'NOTIFICATION_POLICY',
  'DISCLOSURE_POLICY',
  'EXCEPTION_DISCLOSURE_RULE',
];

// 三册各自的「来源提示句」:空册与描述里说清这一签读的是哪本册,规则与策略分开写。
const disclosureKindNotes: Record<DisclosureKind, string> = {
  NOTIFICATION_POLICY: '通知策略(0010):按披露策略引用答义务判据与相对时限,页面不会生成默认策略。',
  DISCLOSURE_POLICY:
    '披露策略(0012):按客户答里程碑、ETA、终局、备注四维展示什么,页面不会生成默认策略。',
  EXCEPTION_DISCLOSURE_RULE:
    '异常披露规则(0023):按客户 × 信号类型 × 可信度答披露条件成不成立、允不允许自动发布、内容来处;与披露策略是相邻两册,页面不会生成默认规则。',
};

type DisclosureListBody = Extract<
  VisibilityCatalogueListResponseBody,
  { kind: DisclosureKind }
>;

interface PolicyRow {
  key: string;
  values: Readonly<Record<string, string>>;
}

function col(id: string, header: string, mono = false): ListColumn<PolicyRow> {
  return {
    id,
    header,
    className: mono ? 'font-mono text-xs' : undefined,
    render: (row) => row.values[id] ?? '—',
  };
}

const kindColumns: Record<DisclosureKind, ListColumn<PolicyRow>[]> = {
  NOTIFICATION_POLICY: [
    col('policy', '策略引用', true),
    col('channel', '适用渠道', true),
    // 相对量原词转写(interval 文本):绝对截止点在目录上不存在,由披露决定时间加出。
    col('deadlineAfter', '要求时限(相对量)', true),
    col('obligation', '义务判据', true),
    col('approvedBy', '批准人', true),
  ],
  DISCLOSURE_POLICY: [
    col('version', '策略版本', true),
    col('effective', '生效区间', true),
    col('customer', '客户账户', true),
    col('milestones', '里程碑维'),
    col('eta', 'ETA 维'),
    col('final', '终局维'),
    col('note', '备注维'),
    col('approvedBy', '批准人', true),
  ],
  // 规则册的列名叫「规则版本」不叫「策略版本」:同一个引用值在两册里承担的是两件事。
  EXCEPTION_DISCLOSURE_RULE: [
    col('version', '规则版本', true),
    col('effective', '生效区间', true),
    col('customer', '客户账户', true),
    col('signalKind', '信号类型', true),
    col('confidence', '可信度', true),
    col('disclosable', '披露条件'),
    col('autoRelease', '自动发布'),
    col('content', '内容来处', true),
    col('approvedBy', '批准人', true),
  ],
};

// 一维一格:态词配封闭三格词表,content 按 0012 只在「展示」态在场——在场就跟在态
// 词后原样示出;页面不替服务端做第二道 shape 校验,来什么显什么。
function disclosureCell(cell: DisclosureCellRecord): string {
  const state = labelOf(disclosureStateLabels, cell.state);
  return cell.content ? `${state}:${cell.content}` : state;
}

function rowsOf(body: DisclosureListBody): PolicyRow[] {
  switch (body.kind) {
    case 'NOTIFICATION_POLICY':
      return body.catalogues.map((record) => ({
        // 策略引用即租户内行键(版本化由引用承担),不再拼别的列。
        key: `notify:${record.policy}`,
        values: {
          policy: record.policy,
          channel: record.channel,
          deadlineAfter: record.deadlineAfter,
          obligation: record.obligation,
          approvedBy: record.approvedBy,
        },
      }));
    case 'DISCLOSURE_POLICY':
      return body.catalogues.flatMap((catalogue) =>
        catalogue.entries.map((entry) => ({
          // 条目键取登记入口的防撞键(客户,checkEntries keyOf)。
          key: `disclose:${catalogue.version}:${entry.customer}`,
          values: {
            version: catalogue.version,
            effective: formatRange(catalogue.effectiveFrom, catalogue.effectiveTo),
            customer: entry.customer,
            milestones: disclosureCell(entry.milestones),
            eta: disclosureCell(entry.eta),
            final: disclosureCell(entry.final),
            note: disclosureCell(entry.note),
            approvedBy: catalogue.approvedBy,
          },
        })),
      );
    case 'EXCEPTION_DISCLOSURE_RULE':
      return body.catalogues.flatMap((catalogue) =>
        catalogue.entries.map((entry) => ({
          // 条目键取 0023 的主键三维(客户 + 信号类型 + 可信度)。
          key: `disclose-rule:${catalogue.version}:${entry.customer}:${entry.signalKind}:${entry.confidence}`,
          values: {
            version: catalogue.version,
            effective: formatRange(catalogue.effectiveFrom, catalogue.effectiveTo),
            customer: entry.customer,
            signalKind: entry.signalKind,
            confidence: entry.confidence,
            disclosable: disclosableLabel(entry.disclosable),
            autoRelease: autoReleaseLabel(entry.autoRelease),
            // content 只在披露条件成立时在场(0023 成对约束),缺席由列渲染的「—」如实示出。
            ...(entry.content ? { content: entry.content } : {}),
            approvedBy: catalogue.approvedBy,
          },
        })),
      );
  }
}

function DisclosurePoliciesTable() {
  const [kind, setKind] = useState<DisclosureKind>('NOTIFICATION_POLICY');
  const [search, setSearch] = useState('');
  const [reloadKey, setReloadKey] = useState(0);
  const [loaded, setLoaded] = useState<{
    kind: DisclosureKind;
    answer: ApiResult<DisclosureListBody>;
  } | null>(null);

  useEffect(() => {
    let cancelled = false;
    void listVisibilityCatalogues(kind).then((answer) => {
      if (!cancelled) setLoaded({ kind, answer });
    });
    return () => {
      cancelled = true;
    };
  }, [kind, reloadKey]);

  const answer = loaded?.kind === kind ? loaded.answer : null;
  const body = answer?.kind === 'outcome' ? answer.body : null;
  const rows = body ? rowsOf(body) : [];
  const needle = search.trim().toLowerCase();
  const visibleRows = needle
    ? rows.filter((row) =>
        Object.values(row.values).some((value) => value.toLowerCase().includes(needle)),
      )
    : rows;
  const retry = () => setReloadKey((value) => value + 1);

  // 通知策略册不带版本壳,计数只报条数;披露策略与异常披露规则报版数与条目数。
  const summary =
    body?.kind === 'NOTIFICATION_POLICY'
      ? `${visibilityCatalogueKindLabels[body.kind]} ${rows.length} 条`
      : body
        ? `${visibilityCatalogueKindLabels[body.kind]} ${body.catalogues.length} 版 / ${rows.length} 条`
        : undefined;

  return (
    <ListPageTemplate<PolicyRow>
      title={info.title}
      description={`${info.owner}——通知策略、披露策略与异常披露规则三册对外披露口径。通知时限是相对量,绝对截止点由披露决定时间加出;披露策略按客户逐行答里程碑、ETA、终局、备注四维,content 只在「展示」态在场;异常披露规则(0023)是与披露策略相邻的另一本册,按客户 × 信号类型 × 可信度答披露条件、自动发布与内容来处。渠道、义务判据、信号类型与可信度都是开放引用按原词直示。当前签:${disclosureKindNotes[kind]}`}
      search={{
        value: search,
        onChange: setSearch,
        placeholder: '搜索策略引用、渠道、客户账户或信号类型',
      }}
      filters={
        <>
          {disclosureKinds.map((candidate) => (
            <button
              key={candidate}
              type="button"
              className={chipClass(candidate === kind)}
              onClick={() => setKind(candidate)}
            >
              {visibilityCatalogueKindLabels[candidate]}
            </button>
          ))}
        </>
      }
      filterSummary={summary}
      columns={kindColumns[kind]}
      rows={visibleRows}
      rowKey={(row) => row.key}
      viewState={catalogueViewState(answer, rows.length, retry, {
        module: info,
        endpoint: `GET /visibility-catalogues?kind=${kind}`,
        emptyTitle: `当前租户尚无${visibilityCatalogueKindLabels[kind]}`,
        // 空册即「未登记」:如实说这一签读的是哪本册、页面不替它生成默认,规则与策略各说各的。
        emptyDescription: `读取入口已配置,但该目录为空。${disclosureKindNotes[kind]}`,
      })}
    />
  );
}

// 登记签装本页读签里的每一册,判据同判断规则页:在哪页读就在哪页登。
const registrationTargets: RegistrationTarget[] = disclosureRegistrableKinds.map((candidate) => ({
  id: candidate,
  label: visibilityCatalogueKindLabels[candidate],
  title: registrationTitles[candidate],
  endpoint: `POST ${visibilityRegistrationEndpoints[candidate]}`,
  snapshotHint: registrationSnapshotHints[candidate],
  submit: (snapshot) => registerVisibilityCatalogue(candidate, snapshot),
  outcomeLabels: registrationOutcomeLabels,
  refusalReasonLabels: registrationRefusalReasonLabels,
}));

/**
 * 对外披露口径三册:逐册查阅,外加逐册的登记签(ADR-0085,票 admin-write-faces/02 切片 02d;
 * 异常披露规则的读签见票 ve-disclosure-policy-view/03,写签见票 02 步二)。
 *
 * 各册换版的走法不同,登记签照实呈现而不抹平:披露策略与异常披露规则按版本抬头翻旧插新
 * (两册相邻不相同,登记签的标题与提示句里「规则」「策略」两字不互换);通知策略没有版本
 * 抬头,版本化由披露策略引用本身承担,换版即换引用、新旧两行并存。哪一册都没有覆盖或删除
 * 动作,所以这里也只有登记一个动作。
 */
export function DisclosurePoliciesPage() {
  return (
    <div className="flex-1 flex flex-col overflow-hidden bg-idpxyz-editor">
      <Tabs defaultValue="catalogue" className="flex-1 flex flex-col overflow-hidden gap-0">
        <TabsList className="px-4 shrink-0">
          <TabsTrigger value="catalogue">披露口径</TabsTrigger>
          <TabsTrigger value="register">登记披露口径</TabsTrigger>
        </TabsList>
        <TabsContent
          value="catalogue"
          className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden"
        >
          <DisclosurePoliciesTable />
        </TabsContent>
        <TabsContent
          value="register"
          className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden"
        >
          <MultiRegistrationPanel
            moduleId="disclosure-policies"
            targets={registrationTargets}
            problemNote={problemNote}
          />
        </TabsContent>
      </Tabs>
    </div>
  );
}
