-- 受控通道命令集合放进「接管」一词（ADR-0154）：
-- `parcel-governance-register` 开 takeover 子命令，留痕表要记得下它。
--
-- 按 0006 头注的约定走新迁移放宽，不改 0006 / 0007：约束名不变，集合只增不改，已落的行全部仍满足。
-- 第一道镜像仍在领域（domain.ChannelCommand）。
ALTER TABLE pilot_governance.channel_execution
    DROP CONSTRAINT channel_execution_command_closed;

ALTER TABLE pilot_governance.channel_execution
    ADD CONSTRAINT channel_execution_command_closed
        CHECK (command IN ('authority-interval', 'suspend', 'resume', 'stage-review', 'takeover'));
