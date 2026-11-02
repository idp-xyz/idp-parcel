import { moduleInfoById } from '../../navigation';
import { RuleBookPage } from './RuleBookPage';
import { judgmentKinds } from './rule-books';

const info = moduleInfoById['tracking-judgment-rules'];

/**
 * 追踪规则：三本判断规则册（里程碑映射、分诊规则、冲突信号规则）——动态归成哪个状态、异常信号怎么分流、互相矛盾的动态记成
 * 什么。三册同为「事实 / 信号 → 结论」的判断规则；通知与披露是对外口径、索赔资格与授权是索赔前置，各归各页。
 */
export function TrackingJudgmentRulesPage() {
  return (
    <RuleBookPage
      moduleId="tracking-judgment-rules"
      info={info}
      kinds={judgmentKinds}
      description="包裹动态怎么归成「已揽收」「已签收」这类状态、异常信号怎么处理，由这里的规则决定；新规则只管以后的判断。"
      registerLabel="新增规则"
    />
  );
}
