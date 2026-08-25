// 本目录 fetch 出口:合规规则库查阅(GET /customs-compliance-rules,ADR-0077、
// 票 master-data-wiring/04)。两本册子按 ?registry= 分派。传输与五格判读收敛在
// 共享 catalogue-api,本文件只保留本上下文的类型与查询函数。

import { exchangeMasterData, type ApiResult } from '../catalogue-api';

export type { ApiResult } from '../catalogue-api';

export type ComplianceRegistry = 'case-requirement' | 'interpretation';

export interface CaseRequirementRuleRecord {
  jurisdiction: string;
  direction: string;
  procedure: string;
  required: boolean;
  basis: string;
}

export interface InterpretationRuleRecord {
  layer: string;
  jurisdiction: string;
  rule: string;
  appliesFrom: string;
  appliesUntil?: string;
}

export interface CaseRequirementListResponseBody {
  outcome: 'CASE_REQUIREMENT_RULES_LISTED';
  rules: CaseRequirementRuleRecord[];
}

export interface InterpretationListResponseBody {
  outcome: 'INTERPRETATION_RULES_LISTED';
  rules: InterpretationRuleRecord[];
}

export type ComplianceRulesListResponseBody =
  | CaseRequirementListResponseBody
  | InterpretationListResponseBody;

export function listComplianceRules(
  registry: ComplianceRegistry,
): Promise<ApiResult<ComplianceRulesListResponseBody>> {
  return exchangeMasterData<ComplianceRulesListResponseBody>(
    `/customs-compliance-rules?registry=${encodeURIComponent(registry)}`,
  );
}
