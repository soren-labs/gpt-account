# 迁移说明

工作区基线：从 `9b19781` 上的未提交 Go CLI 与 Astra 菜单修复继续，没有 reset 或删除已有改动。

## 复用

- `internal/gpa` 身份识别、账号库、切换写入、WSL 适配器、锁。
- 旧槽位名 `plus` / `biz1` / `biz2` 保留为别名。
- 账号库仍在 `%LOCALAPPDATA%\gpa`（WSL 为 `/mnt/c/Users/<user>/AppData/Local/gpa`）。

## 变更

- 新增稳定 `account_id`、`display_name`、`archived`、`schema_version=2`。升级前备份 `state.json`。
- 旧操作 `applied` 读作 `succeeded`，编号不变。
- 人类入口改为 `gpa-manager` / `GPA Manager.exe` 打开本地网页。
- Agent 入口为 `agent/gpa_agent.py` 与 `skills/gpa-account-manager`。
- 旧 `gpa` 终端菜单仍可编译，不再作为产品入口。
- 旧 Python `src/gpa` 与旧账号库不会自动删除。

## 只读检查

实现与验收使用临时账号库和伪造 JWT。没有复制 `.codex` 会话，没有覆盖当前 ChatGPT 登录。
