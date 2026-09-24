-- 受控通道命令集合放进第二批的「阶段评审」一词（票 demo-intake-admission-paused/01）：
-- `parcel-governance-register` 开 stage-review 子命令，留痕表要记得下它。
--
-- 按 0006 头注的约定走新迁移放宽，不改 0006：约束名不变，集合只增不改，已落的行全部仍满足。
-- 第一道镜像仍在领域（domain.ChannelCommand）；新增命令先改领域封闭集，再走一份这样的迁移。
-- 接管仍未开，不在集合内。
ALTER TABLE pilot_governance.channel_execution
    DROP CONSTRAINT channel_execution_command_closed;

ALTER TABLE pilot_governance.channel_execution
    ADD CONSTRAINT channel_execution_command_closed
        CHECK (command IN ('authority-interval', 'suspend', 'resume', 'stage-review'));
