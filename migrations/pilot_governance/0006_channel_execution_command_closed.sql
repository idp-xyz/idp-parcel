-- 受控通道命令集合在库内的第二道镜像（票 pilot-governance-context-gaps/01 裁决）。
--
-- 集合归产品定：`parcel-governance-register` 按批开放子命令，留痕记的是「哪一个受控命令被执行过」。
-- 第一道在领域（domain.ChannelCommand，enum 门禁守「新增取值必须同时补 String()」），本 CHECK 是
-- 第二道——换一个写入方、或直接写表，也造不出集合外的行。字面值取 CLI 子命令的拼法，与 0004
-- 以来已落的行同一套字。第二批子命令（阶段评审、接管）进来时放宽本约束走新迁移，不改本文件。
--
-- outcome 列刻意不收紧：它的取值来自应用层各结果枚举的 String()，那个集合在应用层由穷举 switch
-- 守着；镜像进 CHECK 会让每一种新答案都牵动一份留痕表迁移，而留痕表只该随「有哪些受控命令」变。
ALTER TABLE pilot_governance.channel_execution
    ADD CONSTRAINT channel_execution_command_closed
        CHECK (command IN ('authority-interval', 'suspend', 'resume'));
