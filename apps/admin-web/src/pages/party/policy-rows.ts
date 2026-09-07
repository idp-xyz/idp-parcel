// 商业政策页的行转写：把各政策册的响应体转成列表模板的行与列。抽出 .tsx 是为了让它在
// Node 里跑得起来（票 admin-web-audit-followups/02）；这里没有 React，只有形状与词表。

import type { ListColumn } from '../../templates';
import { formatInstant, formatRange } from '../catalogue-view';
import type {
  AuthorizationRuleRecord,
  CommercialPolicyKind,
  CommercialPolicyListResponseBody,
  CreditPolicyRecord,
  PreAcceptanceFinancialControlPolicyRecord,
} from './api';
import {
  commercialDirectionLabels,
  commercialStatusLabels,
  controlFailureDispositionLabels,
  controlKindLabels,
  controlRequirementLabels,
  finalOutcomeLabels,
  intakeSourceLabels,
  jointPassConditionLabels,
  labelOf,
  settlementMethodLabels,
} from './presentation';

export interface PolicyRow {
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

// 按 kind 换列(MCP-3 裁决⑦):各册子的行形状互不相同,列向各随其册。种类命名
// 册子而非商业对象类别;授权规则、信用政策与接受前财务控制策略几格的名字恰好也是对象类别,
// 不是例外——那几册上列的对象就是那类版本自己(后端 kind 封闭集注释同一句)。
export const kindColumns: Record<CommercialPolicyKind, ListColumn<PolicyRow>[]> = {
  ACCEPTANCE_RULE_PACKAGE: [
    col('identity', '规则包 / 版本', true),
    col('serviceProduct', '服务产品', true),
    col('contract', '客户合同', true),
    col('legalEntity', '责任法人', true),
    col('scope', '适用范围', true),
    col('rules', '组装规则(类别:引用)', true),
    col('allowedIntakeSources', '允许收寄来源'),
    col('intakeQualifications', '收寄硬资格', true),
    col('finalRules', '终局规则(结果:分类)', true),
    col('effective', '有效区间', true),
    col('declaredAt', '声明时间', true),
  ],
  PRE_ACCEPTANCE_CONTROL: [
    col('contract', '客户合同 / 版本', true),
    col('requirement', '控制要求'),
    col('notApplicableBasis', '不适用依据', true),
    col('declaredAt', '声明时间', true),
  ],
  PRICE_POLICY: [
    col('identity', '政策对象 / 版本', true),
    col('direction', '政策方向'),
    col('planRef', '定价方案引用', true),
    col('planDirection', '方案方向'),
    col('bindingConversion', '绑定转换', true),
    col('policyScope', '适用范围', true),
    col('effective', '有效区间', true),
    col('registeredAt', '登记时间', true),
  ],
  SETTLEMENT_POLICY: [
    col('identity', '政策对象 / 版本', true),
    col('method', '结算方式'),
    col('legalEntity', '责任法人', true),
    col('counterparty', '相对方', true),
    col('contractLabel', '合同标签', true),
    col('chargeScope', '费用范围', true),
    col('currency', '币种', true),
    col('effective', '有效区间', true),
    col('registeredAt', '登记时间', true),
  ],
  AS_OF_POLICY: [
    col('package', '规则包 / 版本', true),
    col('judgmentType', '判断类型', true),
    col('semanticsRef', '时点语义引用', true),
    col('policyVersion', '时点政策版本', true),
    col('declaredAt', '声明时间', true),
  ],
  // 两个请求方各占一列,不并成「取消授权」一栏:「客户可取消」与「运营可取消」是两条
  // 独立授权,合成一栏读不出哪一方缺席。
  AUTHORIZATION_RULE: [
    col('identity', '授权规则 / 版本', true),
    col('scope', '适用范围', true),
    col('status', '生命周期状态'),
    col('customerCancellation', '客户取消授权'),
    col('operationsCancellation', '运营取消授权'),
    col('effective', '有效区间', true),
    col('publishedAt', '发布时间', true),
  ],
  // 列名取 CONTEXT 原句「信用政策……按责任法人、业务角色、费用类型、金额或比例形成版本」;
  // 金额与比例合成一栏「额度」,因为后端契约是两键恰一在场——分成两列会让每行必有一格空着,
  // 读的人分不出那是「这一格不适用」还是「缺了」。
  CREDIT_POLICY: [
    col('identity', '政策对象 / 版本', true),
    col('legalEntity', '责任法人', true),
    col('authorityLevel', '授权层级', true),
    col('chargeType', '费用类型', true),
    col('limit', '额度', true),
    col('effective', '有效区间', true),
    col('registeredAt', '登记时间', true),
  ],
  // 控制项合成一栏而不是按种类拆列:一版策略里同一种控制可以在多个费用范围上各成一行,
  // 拆成「预付冻结 / 信用校验」两列会把范围与顺序压扁;顺序是正文的一部分,合栏里逐项带上。
  PRE_ACCEPTANCE_FINANCIAL_CONTROL_POLICY: [
    col('identity', '策略对象 / 版本', true),
    col('scope', '适用范围', true),
    col('status', '生命周期状态'),
    col('contentRegistered', '正文'),
    col('jointPassCondition', '共同通过条件'),
    col('controls', '控制项(顺序. 种类@费用范围 → 失败处置;责任)', true),
    col('effective', '有效区间', true),
    col('publishedAt', '发布时间', true),
  ],
};

// 正文在场与否由服务端的显式布尔说,页面不拿 content 的有无去推:布尔为真而 content 节缺了是响应
// 不合契约,点名而不是折成「—」——那会让一次坏响应长得像一格正常的空(判据同 creditLimitCell)。
function contentRegisteredCell(record: PreAcceptanceFinancialControlPolicyRecord): string {
  if (!record.contentRegistered) return '未登记';
  if (!record.content) return '正文缺失(响应不合契约)';
  return `已登记(${formatInstant(record.content.registeredAt)})`;
}

// 控制项三态:未登记 / 已登记且至少一项 / 已登记却零项。第三态按领域规矩不该出现(有父行而零子行
// 是坏数据,内容读口会拒),所以那句写成「已登记,正文为空」而不是一句无害的空话——它是一份坏数据
// 的如实呈现,不该读起来像正常态。未登记那态正是票 admin-write-faces/06 立票时看不见的那一格。
function controlsCell(record: PreAcceptanceFinancialControlPolicyRecord): string {
  if (!record.contentRegistered) return '未登记正文';
  if (!record.content) return '正文缺失(响应不合契约)';
  if (record.content.controls.length === 0) return '已登记,正文为空';
  return record.content.controls
    .map(
      (item) =>
        `${item.order}. ${labelOf(controlKindLabels, item.control)}@${item.chargeScope} → ` +
        `${labelOf(controlFailureDispositionLabels, item.onFailure)};责任:${item.responsibility}`,
    )
    .join(' | ');
}

// 额度三态:金额(含 0)、比例、两键都缺。零金额是登记方说出的「授予零信用」,与缺席相反;
// 两键都缺按契约不该出现,点名而不是折成「—」——那会让一次坏响应长得像一格正常的空。
function creditLimitCell(record: CreditPolicyRecord): string {
  if (record.limitMinor !== undefined) return `金额 ${record.limitMinor}（最小货币单位）`;
  if (record.limitRatioBasisPoints !== undefined) {
    return `比例 ${record.limitRatioBasisPoints / 100}%`;
  }
  return '额度缺失（响应不合契约）';
}

// 两族阶段内容声明的三栏各有自己的「空」。「未声明」与「声明了但为空」在数组长度上
// 撞成同一签名,靠服务端给的 *Declared 布尔分——两者的恢复动作相反(前者去登记声明,
// 后者无事可做)。
//
// 允许来源与终局结果两栏的「已声明却为空」按领域规矩根本不该出现(两处都要求至少
// 一行),所以那句写成「已声明,正文为空」而不是一句无害的空话:它是一份坏数据的
// 如实呈现,不该读起来像正常态。资格引用那栏不同——显式声明「无硬资格」是合法的。
function declaredList(declared: boolean, values: string[], emptyNote: string): string {
  if (!declared) return '未声明';
  if (values.length === 0) return emptyNote;
  return values.join('、');
}

// 取消授权按请求方逐格作答,三态各有各的说法。中间那态最容易写错:目录在场而这一方
// 没有行,是这份目录说出的真话——该请求方不许取消——不是配置缺件。把它显示成空白或
// 「未声明」会让人去补一份已经写好的目录,而那份目录正是拒绝的依据。
export function cancellationCell(record: AuthorizationRuleRecord, party: string): string {
  if (!record.cancellationAuthorityDeclared) return '未声明';
  const declaration = record.cancellationAuthorities.find((entry) => entry.party === party);
  return declaration ? `允许:${declaration.ruleReference}` : '不许取消';
}

// 响应体按 kind 判别(api.ts 的联合),各分支读各自的行形;判断类型与绑定转换是
// 开放引用集,按原词展示不配词表。
export function rowsOf(body: CommercialPolicyListResponseBody): PolicyRow[] {
  switch (body.kind) {
    case 'ACCEPTANCE_RULE_PACKAGE':
      return body.policies.map((record) => ({
        key: `package:${record.objectId}@${record.version}`,
        values: {
          identity: `${record.objectId}@${record.version}`,
          serviceProduct: record.serviceProduct,
          contract: record.contract,
          legalEntity: record.legalEntity,
          scope: record.scope,
          rules: record.rules
            .map((rule) => `${rule.category}:${rule.reference}`)
            .join('、'),
          allowedIntakeSources: declaredList(
            record.intakeQualificationDeclared,
            record.allowedIntakeSources.map((source) => labelOf(intakeSourceLabels, source)),
            '已声明,正文为空',
          ),
          intakeQualifications: declaredList(
            record.intakeQualificationDeclared,
            record.intakeQualificationRefs,
            '已声明,无硬资格',
          ),
          finalRules: declaredList(
            record.finalRulesDeclared,
            record.finalRules.map(
              (final) => `${labelOf(finalOutcomeLabels, final.outcome)}:${final.finalKind}`,
            ),
            '已声明,正文为空',
          ),
          effective: formatRange(record.effectiveStartsAt, record.effectiveEndsAt),
          declaredAt: formatInstant(record.declaredAt),
        },
      }));
    case 'PRE_ACCEPTANCE_CONTROL':
      return body.policies.map((record) => ({
        key: `control:${record.contractObjectId}@${record.contractVersion}`,
        values: {
          contract: `${record.contractObjectId}@${record.contractVersion}`,
          requirement: labelOf(controlRequirementLabels, record.requirement),
          notApplicableBasis: record.notApplicableBasis ?? '—',
          declaredAt: formatInstant(record.declaredAt),
        },
      }));
    case 'PRICE_POLICY':
      return body.policies.map((record) => ({
        key: `price:${record.objectId}@${record.version}`,
        values: {
          identity: `${record.objectId}@${record.version}`,
          direction: labelOf(commercialDirectionLabels, record.direction),
          planRef: record.planRef,
          planDirection: labelOf(commercialDirectionLabels, record.planDirection),
          bindingConversion: record.bindingConversion,
          policyScope: record.policyScope,
          effective: formatRange(record.effectiveStartsAt, record.effectiveEndsAt),
          registeredAt: formatInstant(record.registeredAt),
        },
      }));
    case 'SETTLEMENT_POLICY':
      return body.policies.map((record) => ({
        key: `settlement:${record.objectId}@${record.version}`,
        values: {
          identity: `${record.objectId}@${record.version}`,
          method: labelOf(settlementMethodLabels, record.method),
          legalEntity: record.legalEntity,
          counterparty: record.counterparty,
          contractLabel: record.contractLabel,
          chargeScope: record.chargeScope,
          currency: record.currency,
          effective: formatRange(record.effectiveStartsAt, record.effectiveEndsAt),
          registeredAt: formatInstant(record.registeredAt),
        },
      }));
    case 'AS_OF_POLICY':
      return body.policies.map((record) => ({
        key: `as-of:${record.rulePackageObjectId}@${record.rulePackageVersion}:${record.judgmentType}`,
        values: {
          package: `${record.rulePackageObjectId}@${record.rulePackageVersion}`,
          judgmentType: record.judgmentType,
          semanticsRef: record.semanticsRef,
          policyVersion: record.policyVersion,
          declaredAt: formatInstant(record.declaredAt),
        },
      }));
    case 'AUTHORIZATION_RULE':
      return body.policies.map((record) => ({
        key: `authz:${record.objectId}@${record.version}`,
        values: {
          identity: `${record.objectId}@${record.version}`,
          scope: record.scope,
          status: labelOf(commercialStatusLabels, record.status),
          customerCancellation: cancellationCell(record, 'CUSTOMER'),
          operationsCancellation: cancellationCell(record, 'OPERATIONS'),
          effective: formatRange(record.effectiveStartsAt, record.effectiveEndsAt),
          publishedAt: formatInstant(record.publishedAt),
        },
      }));
    case 'CREDIT_POLICY':
      return body.policies.map((record) => ({
        key: `credit:${record.objectId}@${record.version}`,
        values: {
          identity: `${record.objectId}@${record.version}`,
          legalEntity: record.legalEntity,
          authorityLevel: record.authorityLevel,
          chargeType: record.chargeType,
          limit: creditLimitCell(record),
          effective: formatRange(record.effectiveStartsAt, record.effectiveEndsAt),
          registeredAt: formatInstant(record.registeredAt),
        },
      }));
    case 'PRE_ACCEPTANCE_FINANCIAL_CONTROL_POLICY':
      return body.policies.map((record) => ({
        key: `control-policy:${record.objectId}@${record.version}`,
        values: {
          identity: `${record.objectId}@${record.version}`,
          scope: record.scope,
          status: labelOf(commercialStatusLabels, record.status),
          contentRegistered: contentRegisteredCell(record),
          jointPassCondition: record.content
            ? labelOf(jointPassConditionLabels, record.content.jointPassCondition)
            : '—',
          controls: controlsCell(record),
          effective: formatRange(record.effectiveStartsAt, record.effectiveEndsAt),
          publishedAt: formatInstant(record.publishedAt),
        },
      }));
  }
}
