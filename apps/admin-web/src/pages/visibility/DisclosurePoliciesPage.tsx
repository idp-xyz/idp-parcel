import { moduleInfoById } from '../../navigation';
import { RuleBookPage } from './RuleBookPage';
import { disclosureKinds } from './rule-books';

const info = moduleInfoById['disclosure-policies'];

/**
 * 通知与披露：三本对外口径册（通知策略、披露策略、异常披露规则）——多久之内通知客户、客户能看到什么、哪些异常要告诉客户。
 * 披露策略（0012）与异常披露规则（0023）是相邻的两本册，不是一册的两个名字，签名各说各的事。
 */
export function DisclosurePoliciesPage() {
  return (
    <RuleBookPage
      moduleId="disclosure-policies"
      info={info}
      kinds={disclosureKinds}
      description="客户能看到哪些物流信息、遇到异常要不要告诉客户、多久之内必须通知到，由这里的规则决定。"
      registerLabel="新增规则"
    />
  );
}
