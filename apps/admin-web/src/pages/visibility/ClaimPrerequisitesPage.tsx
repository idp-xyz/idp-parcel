import { useEffect, useState } from 'react';
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@idpxyz/ui-primitives';
import { moduleInfoById } from '../../navigation';
import { ListPageTemplate, type ListColumn } from '../../templates';
import { MultiRegistrationPanel, chipClass, type RegistrationTarget } from '../../components/registration';
import { catalogueViewState } from '../catalogue-view';
import {
  listVisibilityCatalogues,
  registerVisibilityCatalogue,
  visibilityRegistrationEndpoints,
  type ApiResult,
  type VisibilityCatalogueListResponseBody,
} from './catalogue-api';
import {
  problemNote,
  registrationOutcomeLabels,
  registrationRefusalReasonLabels,
  registrationSnapshotHints,
  registrationTitles,
  visibilityCatalogueKindLabels,
} from './presentation';

// VE 六类目录拆三页,本页装**索赔前置**那组:索赔资格与索赔授权(票
// admin-web-page-wiring-frontier/02 的页面裁决)。两册同答「谁、就什么可以提索赔」
// ——资格按客户合同版本圈覆盖种类,授权按客户账户圈代提申请人;判断规则与披露
// 口径各归各页。索赔项与追偿事项本身是案件,在追踪异常区的「索赔与追偿」页,
// 不在本页——目录是规则,案件不是。
//
// 两册行本身就是「对象 + 版本」一行,没有条目子册,不需要行展开。

const info = moduleInfoById['claim-prerequisites'];

type PrerequisiteKind = 'CLAIM_ELIGIBILITY' | 'CLAIM_AUTHORIZATION';

const prerequisiteKinds: PrerequisiteKind[] = ['CLAIM_ELIGIBILITY', 'CLAIM_AUTHORIZATION'];

type PrerequisiteListBody = Extract<
  VisibilityCatalogueListResponseBody,
  { kind: PrerequisiteKind }
>;

interface CatalogueRow {
  key: string;
  values: Readonly<Record<string, string>>;
}

function col(id: string, header: string, mono = false): ListColumn<CatalogueRow> {
  return {
    id,
    header,
    className: mono ? 'font-mono text-xs' : undefined,
    render: (row) => row.values[id] ?? '—',
  };
}

const kindColumns: Record<PrerequisiteKind, ListColumn<CatalogueRow>[]> = {
  CLAIM_ELIGIBILITY: [
    col('contract', '客户合同', true),
    col('version', '目录版本', true),
    col('coveredKinds', '覆盖索赔种类', true),
    col('approvedBy', '批准人', true),
  ],
  CLAIM_AUTHORIZATION: [
    col('customer', '客户账户', true),
    col('version', '目录版本', true),
    col('applicants', '获授权申请人'),
    col('approvedBy', '批准人', true),
  ],
};

// 授权名单为空是「此账户当前不授权任何人代提」的显式决定(0018),与「还没登记」
// (整行不在列表里)不是一回事:后者页面上根本没有这一行,前者必须说成一个决定。
// 显示成「—」或空白会让人去补一份已经作出的决定,那份空名单正是拒绝代提的依据。
function applicantsCell(applicants: string[]): string {
  if (applicants.length === 0) return '显式空名单:不授权任何申请人代提';
  return applicants.join('、');
}

function rowsOf(body: PrerequisiteListBody): CatalogueRow[] {
  switch (body.kind) {
    case 'CLAIM_ELIGIBILITY':
      return body.catalogues.map((record) => ({
        key: `eligibility:${record.contract}@${record.version}`,
        values: {
          contract: record.contract,
          version: record.version,
          // 覆盖种类是开放引用集,原词联排;登记入口要求至少一项,空集不会到场。
          coveredKinds: record.coveredKinds.join('、'),
          approvedBy: record.approvedBy,
        },
      }));
    case 'CLAIM_AUTHORIZATION':
      return body.catalogues.map((record) => ({
        key: `authorization:${record.customer}@${record.version}`,
        values: {
          customer: record.customer,
          version: record.version,
          applicants: applicantsCell(record.applicants),
          approvedBy: record.approvedBy,
        },
      }));
  }
}

function ClaimPrerequisitesTable() {
  const [kind, setKind] = useState<PrerequisiteKind>('CLAIM_ELIGIBILITY');
  const [search, setSearch] = useState('');
  const [reloadKey, setReloadKey] = useState(0);
  const [loaded, setLoaded] = useState<{
    kind: PrerequisiteKind;
    answer: ApiResult<PrerequisiteListBody>;
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

  return (
    <ListPageTemplate<CatalogueRow>
      title={info.title}
      description={`${info.owner}——索赔资格与索赔授权两册索赔前置。资格按客户合同版本列出覆盖的索赔种类;授权按客户账户列出获准代提的申请人,空名单是「不授权任何人代提」的显式决定而非缺登记,种类与申请人是开放引用按原词直示`}
      search={{
        value: search,
        onChange: setSearch,
        placeholder: '搜索合同、客户账户或申请人',
      }}
      filters={
        <>
          {prerequisiteKinds.map((candidate) => (
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
      filterSummary={
        body ? `${visibilityCatalogueKindLabels[kind]} ${rows.length} 份` : undefined
      }
      columns={kindColumns[kind]}
      rows={visibleRows}
      rowKey={(row) => row.key}
      viewState={catalogueViewState(answer, rows.length, retry, {
        module: info,
        endpoint: `GET /visibility-catalogues?kind=${kind}`,
        emptyTitle: `当前租户尚无${visibilityCatalogueKindLabels[kind]}`,
        emptyDescription: '读取入口已配置,但该目录为空;页面不会生成默认目录。',
      })}
    />
  );
}

// 登记签装本页读签的同两册,判据同判断规则页。
const registrationTargets: RegistrationTarget[] = prerequisiteKinds.map((candidate) => ({
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
 * 索赔前置两册:逐册查阅,外加登记签(ADR-0085,票 admin-write-faces/02 切片 02d)。
 *
 * 两册登的都是租户的配置而不是某一笔索赔的事实:资格声明说的是一份合同责任范围覆盖哪些
 * 索赔种类,授权名单说的是这个账户声明了谁可以代提——某一笔够不够资格、由谁提得成,是
 * 案上判断,由索赔用例按这两册核出,不在本签。换名单走版本链,没有撤销命令,所以这里同样
 * 只有登记一个动作。
 */
export function ClaimPrerequisitesPage() {
  return (
    <div className="flex-1 flex flex-col overflow-hidden bg-idpxyz-editor">
      <Tabs defaultValue="catalogue" className="flex-1 flex flex-col overflow-hidden gap-0">
        <TabsList className="px-4 shrink-0">
          <TabsTrigger value="catalogue">索赔前置</TabsTrigger>
          <TabsTrigger value="register">登记索赔前置</TabsTrigger>
        </TabsList>
        <TabsContent
          value="catalogue"
          className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden"
        >
          <ClaimPrerequisitesTable />
        </TabsContent>
        <TabsContent
          value="register"
          className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden"
        >
          <MultiRegistrationPanel
            moduleId="claim-prerequisites"
            targets={registrationTargets}
            problemNote={problemNote}
          />
        </TabsContent>
      </Tabs>
    </div>
  );
}
