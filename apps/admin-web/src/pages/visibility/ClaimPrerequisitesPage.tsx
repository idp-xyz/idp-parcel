import { useEffect, useState } from 'react';
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@idpxyz/ui-primitives';
import { moduleInfoById } from '../../navigation';
import { ListPageTemplate, type ListColumn } from '../../templates';
import { MultiRegistrationPanel, type RegistrationTarget } from '../../components/registration';
import { TechnicalDetails } from '../../components/TechnicalDetails';
import { shortPrincipalRef } from '../../components/principal-ref';
import { labelFor } from '../../vocabulary/status';
import { objectNouns } from '../../vocabulary/objects';
import { matchesSearch } from '../party/list-order';
import { useMayWriteRegistry } from '../use-operator-grants';
import { TwoLine, technicalRows } from '../settlement/ops-ui';
import { namedRef, usePartyNames, type PartyNames } from '../settlement/party-names';
import {
  listVisibilityCatalogues,
  registerVisibilityCatalogue,
  visibilityRegistrationEndpoints,
  type ApiResult,
  type ClaimAuthorizationRecord,
  type ClaimEligibilityRecord,
} from './catalogue-api';
import {
  claimPrerequisiteKindLabels,
  claimPrerequisiteOutcomeLabels,
  claimPrerequisiteRefusalLabels,
  claimPrerequisiteRegistrationTitles,
  claimPrerequisiteSnapshotHints,
  exceptionProblemNote,
  type ClaimPrerequisiteKind,
} from './case-presentation';
import { exceptionViewState } from './exception-view-state';

// 页面标题与主责出处的唯一来源是 navigation 的 moduleInfoById，只读引用，不抄第二份。
const info = moduleInfoById['claim-prerequisites'];

// 索赔资格与授权：两本册同答「谁、就什么可以提索赔」——可索赔范围按客户合同列出能索赔的类型，代提授权按客户列出谁能
// 代客户提。改版后两册各一签、设定一签；列只放合同 / 客户、索赔类型、可代提的人与批准人，版本号与原码进技术详情。
// 某一笔索赔够不够资格、由谁提得成是案上判断，在「索赔与追偿」页，不在这里——这里是规则，不是案件。
//
// 两册登的是租户自己的规则：册上没有某份合同或某个客户，就是还没设定，页面不补默认值。

const tabContent = 'flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden';

function useCatalogue<Body>(kind: ClaimPrerequisiteKind) {
  const [answer, setAnswer] = useState<ApiResult<Body> | null>(null);
  const [reloadToken, setReloadToken] = useState(0);
  useEffect(() => {
    let cancelled = false;
    setAnswer(null);
    void listVisibilityCatalogues(kind).then((result) => {
      if (!cancelled) setAnswer(result as unknown as ApiResult<Body>);
    });
    return () => {
      cancelled = true;
    };
  }, [kind, reloadToken]);
  return { answer, retry: () => setReloadToken((token) => token + 1) };
}

const claimKindsText = (kinds: readonly string[]) => kinds.map((kind) => labelFor(kind, 'claim-kind')).join('、');

// 授权名单为空是「这个客户不允许任何人代提」的明确决定（0018），与「还没设定」（整行不在列表里）不是一回事：
// 显示成「—」或空白会让人以为没填、去补一份已经作出的决定。
function applicantsText(applicants: readonly string[]): string {
  return applicants.length === 0 ? '不允许任何人代提' : applicants.join('、');
}

function technicalColumn<Row>(rowsOf: (row: Row) => { label: string; value: string | undefined }[]): ListColumn<Row> {
  return {
    id: 'technical',
    header: '',
    className: 'align-top',
    render: (row) => <TechnicalDetails rows={technicalRows(rowsOf(row))} className="max-w-md open:min-w-[16rem]" />,
  };
}

const approverColumn = <Row extends { approvedBy: string }>(): ListColumn<Row> => ({
  id: 'approved-by',
  header: '批准人',
  className: 'whitespace-nowrap font-mono type-body leading-4',
  render: (row) => <span title={row.approvedBy}>{shortPrincipalRef(row.approvedBy)}</span>,
});

// —— 可索赔范围签 ——

const eligibilityColumns: ListColumn<ClaimEligibilityRecord>[] = [
  { id: 'contract', header: objectNouns.contract, className: 'font-mono type-body leading-4 text-idpxyz-textBright', render: (row) => row.contract },
  { id: 'kinds', header: '可索赔的类型', render: (row) => claimKindsText(row.coveredKinds) },
  approverColumn<ClaimEligibilityRecord>(),
  technicalColumn((row) => [
    { label: '版本', value: row.version },
    { label: '类型码', value: row.coveredKinds.join('、') },
    { label: '批准人', value: row.approvedBy },
  ]),
];

function EligibilityTable() {
  const [search, setSearch] = useState('');
  const { answer, retry } = useCatalogue<{ catalogues: ClaimEligibilityRecord[] }>('CLAIM_ELIGIBILITY');
  const rows = answer?.kind === 'outcome' ? answer.body.catalogues : [];
  const visible = rows.filter((row) => matchesSearch(search, [row.contract, row.version, claimKindsText(row.coveredKinds)]));
  return (
    <ListPageTemplate<ClaimEligibilityRecord>
      title={claimPrerequisiteKindLabels.CLAIM_ELIGIBILITY}
      description="每份客户合同可以索赔哪些类型。没有列在这里的合同还没有设定。"
      search={{ value: search, onChange: setSearch, placeholder: '搜合同、索赔类型…' }}
      filterSummary={answer?.kind === 'outcome' ? `共 ${rows.length} 份合同` : undefined}
      columns={eligibilityColumns}
      rows={visible}
      rowKey={(row) => `${row.contract}@${row.version}`}
      emptyRowsNote="没有匹配的合同"
      viewState={exceptionViewState(answer, rows.length, retry, {
        module: info,
        endpoint: 'GET /visibility-catalogues?kind=CLAIM_ELIGIBILITY',
        subject: '可索赔范围',
        emptyTitle: '还没有设定任何合同的可索赔范围',
        emptyDescription: '在「新增设定」签里为客户合同设定可以索赔的类型；系统不会替你预置。',
      })}
    />
  );
}

// —— 代提授权签 ——

function authorizationColumns(names: PartyNames): ListColumn<ClaimAuthorizationRecord>[] {
  return [
    {
      id: 'customer',
      header: objectNouns.customer,
      className: 'whitespace-nowrap',
      render: (row) => {
        const ref = namedRef(names.account(row.customer), row.customer);
        return <TwoLine primary={ref.primary} secondary={ref.secondary} />;
      },
    },
    {
      id: 'applicants',
      header: '可以代客户提索赔的人',
      render: (row) =>
        row.applicants.length === 0 ? (
          <span className="text-idpxyz-textBright">{applicantsText(row.applicants)}</span>
        ) : (
          <span className="font-mono type-body leading-4">{applicantsText(row.applicants)}</span>
        ),
    },
    approverColumn<ClaimAuthorizationRecord>(),
    technicalColumn((row) => [
      { label: '版本', value: row.version },
      { label: '客户编码', value: row.customer },
      { label: '批准人', value: row.approvedBy },
    ]),
  ];
}

function AuthorizationTable({ names }: { names: PartyNames }) {
  const [search, setSearch] = useState('');
  const { answer, retry } = useCatalogue<{ catalogues: ClaimAuthorizationRecord[] }>('CLAIM_AUTHORIZATION');
  const rows = answer?.kind === 'outcome' ? answer.body.catalogues : [];
  const visible = rows.filter((row) =>
    matchesSearch(search, [row.customer, names.account(row.customer), row.version, ...row.applicants]),
  );
  return (
    <ListPageTemplate<ClaimAuthorizationRecord>
      title={claimPrerequisiteKindLabels.CLAIM_AUTHORIZATION}
      description="每个客户允许谁代他提出索赔。「不允许任何人代提」是一个明确的决定，不是没填。"
      search={{ value: search, onChange: setSearch, placeholder: '搜客户、申请人…' }}
      filterSummary={answer?.kind === 'outcome' ? `共 ${rows.length} 个客户` : undefined}
      columns={authorizationColumns(names)}
      rows={visible}
      rowKey={(row) => `${row.customer}@${row.version}`}
      emptyRowsNote="没有匹配的客户"
      viewState={exceptionViewState(answer, rows.length, retry, {
        module: info,
        endpoint: 'GET /visibility-catalogues?kind=CLAIM_AUTHORIZATION',
        subject: '代提索赔授权',
        emptyTitle: '还没有设定任何客户的代提授权',
        emptyDescription: '在「新增设定」签里为客户设定谁可以代他提索赔；系统不会替你预置。',
      })}
    />
  );
}

// —— 设定签：两册各一个登记口 ——

const prerequisiteKinds: ClaimPrerequisiteKind[] = ['CLAIM_ELIGIBILITY', 'CLAIM_AUTHORIZATION'];

const registrationTargets: RegistrationTarget[] = prerequisiteKinds.map((kind) => ({
  id: kind,
  label: claimPrerequisiteKindLabels[kind],
  title: claimPrerequisiteRegistrationTitles[kind],
  endpoint: `POST ${visibilityRegistrationEndpoints[kind]}`,
  snapshotHint: claimPrerequisiteSnapshotHints[kind],
  submit: (snapshot) => registerVisibilityCatalogue(kind, snapshot),
  outcomeLabels: claimPrerequisiteOutcomeLabels,
  refusalReasonLabels: claimPrerequisiteRefusalLabels,
}));

/**
 * 索赔资格与授权：两签查阅，外加设定签（ADR-0085）。换名单走新版本，没有撤销，所以设定签只有新增一个动作。
 * 设定签按授予收起（ADR-0315）：明确知道操作者没有登记册配置写授予时不摆。
 */
export function ClaimPrerequisitesPage() {
  const names = usePartyNames();
  const mayWrite = useMayWriteRegistry();
  return (
    <div className="flex-1 flex flex-col overflow-hidden bg-idpxyz-editor">
      <Tabs defaultValue="eligibility" className="flex-1 flex flex-col overflow-hidden gap-0">
        <TabsList className="px-4 shrink-0">
          <TabsTrigger value="eligibility">{claimPrerequisiteKindLabels.CLAIM_ELIGIBILITY}</TabsTrigger>
          <TabsTrigger value="authorization">{claimPrerequisiteKindLabels.CLAIM_AUTHORIZATION}</TabsTrigger>
          {mayWrite ? <TabsTrigger value="register">新增设定</TabsTrigger> : null}
        </TabsList>
        <TabsContent value="eligibility" className={tabContent}>
          <EligibilityTable />
        </TabsContent>
        <TabsContent value="authorization" className={tabContent}>
          <AuthorizationTable names={names} />
        </TabsContent>
        {mayWrite ? (
          <TabsContent value="register" className={tabContent}>
            <MultiRegistrationPanel moduleId="claim-prerequisites" targets={registrationTargets} problemNote={exceptionProblemNote} />
          </TabsContent>
        ) : null}
      </Tabs>
    </div>
  );
}
