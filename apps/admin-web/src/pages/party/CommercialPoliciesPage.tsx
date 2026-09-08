import { useEffect, useState } from 'react';
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@idpxyz/ui-primitives';
import { moduleInfoById } from '../../navigation';
import { ListPageTemplate } from '../../templates';
import { RegistrationPanel } from '../../components/registration';
import type { ApiResult } from '../catalogue-api';
import { catalogueViewState } from '../catalogue-view';
import {
  commercialRegistrationEndpoints,
  declarationLandingLabels,
  listCommercialPolicies,
  publicationOutcomeLabels,
  registerCommercial,
  type CommercialPolicyKind,
  type CommercialPolicyListResponseBody,
} from './api';
import { CreditPolicyPublicationForm } from './CreditPolicyPublicationForm';
import { AuthorizationRulePublicationForm } from './AuthorizationRulePublicationForm';
import { SettlementPolicyPublicationForm } from './SettlementPolicyPublicationForm';
import { PricePolicyPublicationForm } from './PricePolicyPublicationForm';
import { AcceptanceRulePackagePublicationForm } from './AcceptanceRulePackagePublicationForm';
import { PreAcceptanceFinancialControlPolicyPublicationForm } from './PreAcceptanceFinancialControlPolicyPublicationForm';
import { kindColumns, rowsOf, type PolicyRow } from './policy-rows';
import {
  commercialPolicyKinds,
  policyKindLabels,
  policyKindSources,
  problemNote,
  registrationSnapshotHints,
  registrationTitles,
} from './presentation';

const info = moduleInfoById['commercial-policies'];

/**
 * 发布签发布落定后对读面的一次通知：切到那一册并重读。`token` 每次递增，同一册连发两次也各刷一次；
 * 为 null 即从未发布过。
 */
interface PublishedNotice {
  token: number;
  kind: CommercialPolicyKind;
}

const chipClass = (active: boolean) =>
  `px-2.5 py-1 text-[12px] rounded border ${
    active
      ? 'border-idpxyz-accent text-idpxyz-accent'
      : 'border-idpxyz-border text-idpxyz-textMuted hover:bg-idpxyz-hover'
  }`;

// 各政策册独立请求、独立列形;同页切换不把各类对象折成一份「大配置」。行与列的转写在
// policy-rows.ts,本文件只管取数、切册与渲染。
function PolicyRegisters({ published }: { published: PublishedNotice | null }) {
  const [kind, setKind] = useState<CommercialPolicyKind>('ACCEPTANCE_RULE_PACKAGE');
  const [search, setSearch] = useState('');
  const [reloadKey, setReloadKey] = useState(0);
  const [loaded, setLoaded] = useState<{
    kind: CommercialPolicyKind;
    answer: ApiResult<CommercialPolicyListResponseBody>;
  } | null>(null);

  // 发布签落定一版，读面切到那一册并重读：完成判据是「结果在同册立刻可见」，不让操作者自己去点刷新。
  useEffect(() => {
    if (published === null) return;
    setKind(published.kind);
    setReloadKey((value) => value + 1);
  }, [published]);

  useEffect(() => {
    let cancelled = false;
    void listCommercialPolicies(kind).then((answer) => {
      if (!cancelled) setLoaded({ kind, answer });
    });
    return () => {
      cancelled = true;
    };
  }, [kind, reloadKey]);

  const answer = loaded?.kind === kind ? loaded.answer : null;
  const rows = answer?.kind === 'outcome' ? rowsOf(answer.body) : [];
  const needle = search.trim().toLowerCase();
  const visibleRows = needle
    ? rows.filter((row) =>
        Object.values(row.values).some((value) => value.toLowerCase().includes(needle)),
      )
    : rows;
  const retry = () => setReloadKey((value) => value + 1);

  return (
    <ListPageTemplate<PolicyRow>
      title={info.title}
      description={`${info.owner}——各本政策册分别查阅,重叠候选仍是适用冲突而非「同时生效」。接单规则包一栏另列挂在同一版本上的收寄资格与终局规则声明,授权规则一栏按请求方逐格列出取消授权,信用政策一栏的额度按金额或比例恰一上列;「接受前财务控制」与「接受前财务控制策略」是两本册:前者列合同的「要不要」声明,后者列策略版本的「控制怎么做」正文`}
      search={{
        value: search,
        onChange: setSearch,
        placeholder: '搜索政策对象、范围或引用',
      }}
      filters={
        <>
          {commercialPolicyKinds.map((candidate) => (
            <button
              key={candidate}
              type="button"
              className={chipClass(candidate === kind)}
              onClick={() => setKind(candidate)}
            >
              {policyKindLabels[candidate]}
            </button>
          ))}
          {/* 册名旁写清「谁喂它」：册与发布口对象类别是两条轴，不写这句操作者会照 chip 抄词进发布快照。 */}
          <p className="basis-full text-[11px] text-idpxyz-textMuted">{policyKindSources[kind]}</p>
        </>
      }
      filterSummary={
        // 计数只在拿到业务答案后显示,未配置态不报「0 条」(与 pricing 两页同一守卫)。
        answer?.kind === 'outcome' ? `${policyKindLabels[kind]} ${rows.length} 条` : undefined
      }
      columns={kindColumns[kind]}
      rows={visibleRows}
      rowKey={(row) => row.key}
      viewState={catalogueViewState(answer, rows.length, retry, {
        module: info,
        endpoint: `GET /commercial-policies?kind=${kind}`,
        emptyTitle: `当前租户尚无${policyKindLabels[kind]}`,
        emptyDescription: '读取入口已配置,但该政策册为空;页面不会生成默认政策。',
      })}
    />
  );
}

/**
 * 商业规则与策略（party-commercial）：各本政策册，外加发布口的受控镜像签。
 *
 * 发布签摆在这里而不摆在别处、也不另开专页，是票 admin-write-faces/03 的裁决：九类发布
 * 对象里六类的结果显示在本页的册里，本页是「写签跟着读签走」能走到的最大一页；专页
 * 会是九类结果一个都不在场的纯写页。同屏两套词（册名 vs 对象类别）曾是不摆的理由，
 * 处置不是把词对齐——它们是两条分类轴（见 policyKindSources 的注释）——而是把对应关系
 * 写在签上和册名旁，让页面教的是真规则。
 *
 * 这一签按 ADR-0101 决定一是受控批量口的在线镜像，不是运营配置员的主路径，所以列在
 * 最后一签而不是首签；各册的逐字段表单按决定八逐册另裁另建。
 *
 * 「发布信用政策版本」是决定八下本页第一张逐字段表单签（票 admin-write-faces/16，公共半边的首例）：
 * 表单 → 预览摘要 → 存为待批准 → 批准 → 发布，结果在信用政策册立刻可见——发布落定后读面切到那一册
 * 并重读。摆在册签之后、镜像签之前：它是主路径，镜像签是高级口。其余各册的签由各自子票逐册接。
 */
export function CommercialPoliciesPage() {
  const [published, setPublished] = useState<PublishedNotice | null>(null);
  const notePublished = (kind: CommercialPolicyKind) =>
    setPublished((current) => ({ token: (current?.token ?? 0) + 1, kind }));

  return (
    <div className="flex-1 flex flex-col overflow-hidden bg-idpxyz-editor">
      <Tabs defaultValue="registers" className="flex-1 flex flex-col overflow-hidden gap-0">
        <TabsList className="px-4 shrink-0">
          <TabsTrigger value="registers">政策册</TabsTrigger>
          <TabsTrigger value="publish-credit-policy">发布信用政策版本</TabsTrigger>
          <TabsTrigger value="publish-authorization-rule">发布授权规则版本</TabsTrigger>
          <TabsTrigger value="publish-settlement-policy">发布结算政策版本</TabsTrigger>
          <TabsTrigger value="publish-price-policy">发布价格政策版本</TabsTrigger>
          <TabsTrigger value="publish-acceptance-rule-package">发布规则包版本</TabsTrigger>
          <TabsTrigger value="publish-pre-acceptance-financial-control-policy">发布接受前财务控制策略版本</TabsTrigger>
          <TabsTrigger value="publish">受控发布（JSON 镜像）</TabsTrigger>
        </TabsList>
        <TabsContent
          value="registers"
          className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden"
        >
          <PolicyRegisters published={published} />
        </TabsContent>
        <TabsContent
          value="publish-credit-policy"
          className="flex-1 flex flex-col overflow-auto data-[state=inactive]:hidden"
        >
          <CreditPolicyPublicationForm onPublished={() => notePublished('CREDIT_POLICY')} />
        </TabsContent>
        <TabsContent
          value="publish-authorization-rule"
          className="flex-1 flex flex-col overflow-auto data-[state=inactive]:hidden"
        >
          <AuthorizationRulePublicationForm onPublished={() => notePublished('AUTHORIZATION_RULE')} />
        </TabsContent>
        <TabsContent
          value="publish-settlement-policy"
          className="flex-1 flex flex-col overflow-auto data-[state=inactive]:hidden"
        >
          <SettlementPolicyPublicationForm onPublished={() => notePublished('SETTLEMENT_POLICY')} />
        </TabsContent>
        <TabsContent
          value="publish-price-policy"
          className="flex-1 flex flex-col overflow-auto data-[state=inactive]:hidden"
        >
          {/* 票 admin-write-faces/14：发布类别 PRICE_RULE 的结果显示在册名 PRICE_POLICY 那本册（两条分类轴），落定后切到它并重读。 */}
          <PricePolicyPublicationForm onPublished={() => notePublished('PRICE_POLICY')} />
        </TabsContent>
        <TabsContent
          value="publish-acceptance-rule-package"
          className="flex-1 flex flex-col overflow-auto data-[state=inactive]:hidden"
        >
          <AcceptanceRulePackagePublicationForm onPublished={() => notePublished('ACCEPTANCE_RULE_PACKAGE')} />
        </TabsContent>
        <TabsContent
          value="publish-pre-acceptance-financial-control-policy"
          className="flex-1 flex flex-col overflow-auto data-[state=inactive]:hidden"
        >
          <PreAcceptanceFinancialControlPolicyPublicationForm
            onPublished={() => notePublished('PRE_ACCEPTANCE_FINANCIAL_CONTROL_POLICY')}
          />
        </TabsContent>
        <TabsContent
          value="publish"
          className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden"
        >
          <RegistrationPanel
            moduleId="commercial-policies"
            title={registrationTitles.publication}
            endpoint={`POST ${commercialRegistrationEndpoints.publication}`}
            snapshotHint={registrationSnapshotHints.publication}
            submit={(snapshot) => registerCommercial('publication', snapshot)}
            outcomeLabels={publicationOutcomeLabels}
            declarationLandingLabels={declarationLandingLabels}
            problemNote={problemNote}
          />
        </TabsContent>
      </Tabs>
    </div>
  );
}
