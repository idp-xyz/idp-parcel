import { useEffect, useState } from 'react';
import { moduleInfoById } from '../../navigation';
import { ListPageTemplate, type ListColumn } from '../../templates';
import { catalogueViewState, formatRange } from '../catalogue-view';
import {
  listVisibilityCatalogues,
  type ApiResult,
  type DisclosureCellRecord,
  type VisibilityCatalogueListResponseBody,
} from './catalogue-api';
import {
  disclosureStateLabels,
  labelOf,
  visibilityCatalogueKindLabels,
} from './presentation';

// VE 六类目录拆三页,本页装**对外披露口径**那组:通知策略与披露策略(票
// admin-web-page-wiring-frontier/02 的页面裁决)。两册同答「对客户说什么、何时必须
// 说」——通知策略管义务与时限,披露策略管每客户四维内容;判断规则与索赔前置各归
// 各页。
//
// 通知策略册没有版本与区间列:版本化由披露策略引用本身承担(0010),换版即换引用、
// 新旧两行并存,页面不给它补一列假版本。披露策略行随条目(客户)展开,理由与判断
// 规则页同:空版本被登记入口拦死(checkEntries),行展开不藏版本。

const info = moduleInfoById['disclosure-policies'];

type DisclosureKind = 'NOTIFICATION_POLICY' | 'DISCLOSURE_POLICY';

const disclosureKinds: DisclosureKind[] = ['NOTIFICATION_POLICY', 'DISCLOSURE_POLICY'];

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
};

const chipClass = (active: boolean) =>
  `px-2.5 py-1 text-[12px] rounded border ${
    active
      ? 'border-idpxyz-accent text-idpxyz-accent'
      : 'border-idpxyz-border text-idpxyz-textMuted hover:bg-idpxyz-hover'
  }`;

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
  }
}

export function DisclosurePoliciesPage() {
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

  // 通知策略册不带版本壳,计数只报条数;披露策略报版数与条目数。
  const summary =
    body?.kind === 'NOTIFICATION_POLICY'
      ? `${visibilityCatalogueKindLabels[body.kind]} ${rows.length} 条`
      : body
        ? `${visibilityCatalogueKindLabels[body.kind]} ${body.catalogues.length} 版 / ${rows.length} 条`
        : undefined;

  return (
    <ListPageTemplate<PolicyRow>
      title={info.title}
      description={`${info.owner}——通知策略与披露策略两册对外披露口径。通知时限是相对量,绝对截止点由披露决定时间加出;披露策略按客户逐行答里程碑、ETA、终局、备注四维,content 只在「展示」态在场,渠道与义务判据是开放引用按原词直示`}
      search={{
        value: search,
        onChange: setSearch,
        placeholder: '搜索策略引用、渠道或客户账户',
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
        emptyDescription: '读取入口已配置,但该目录为空;页面不会生成默认策略。',
      })}
    />
  );
}
