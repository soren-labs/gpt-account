# 测试报告

日期：2026-09-06

## 已验证

- `go test ./...`：账号 ID、预览/幂等切换、查询失败阻断、Agent 不能确认重启、HTTP Origin/CSRF、列表不含 token、登录隔离/取消/身份冲突、事务 journal 恢复、已在位账号在 App 运行时判定为 noop。
- `go test -race ./...`：通过；并发预览/提交和异步登录没有数据竞争。
- 既有 `internal/gpa` 隔离测试（伪造 JWT、临时目录）。
- 演示服务首页 HTML、session、desktop 目标下列出三个席位、切换 biz2 返回 succeeded；列表响应不含 token。
- 使用 `agent-browser` 实际验证页面加载、无错误覆盖层、账号切换、刷新后会话、添加演示账号、最近操作、归档与恢复。
- Agent 脚本在 `GPA_FORCE_LOOPBACK` 或本机 loopback 可达时查询同一后台；后台未启动时能从可信路径自动拉起并完成健康握手。
- `scripts/test-web/test_agent.py` 使用临时端口，避免与本机其他 localhost 服务冲突。
- Skill 包脚本与 `agent/gpa_agent.py` 发布前按字节复制。

### 真实环境（WSL Ubuntu-24.04 + Windows ChatGPT App，2026-09-06）

后台以非 demo 模式跑在 `%LOCALAPPDATA%\gpa`（三个真实账号 biz1/biz2/plus），ChatGPT.exe 全程处于运行状态。

- Agent：`status` / `accounts` / `diagnose` 连到同一后台，输出脱敏（`email_hint`），无 token。
- Agent：`preview biz1 --target desktop` → `waiting_user / APP_RESTART_REQUIRED`，`switch` 返回 `next_action: open_ui`，Agent 侧 `confirm` 被拒绝；WSL CLI 目标在本机 Codex 运行时返回 `CLI_IN_USE`。
- Agent：`preview plus --target desktop`（已在位）→ `noop`，`switch` 直接 `succeeded`，不要求重启、不写文件。
- 人类：`open-ui` bootstrap 一次性、复用返回 401；无 CSRF 的 POST 返回 403；外域 Origin 返回 403。
- 人类：以 UI 会话预览 → 提交 → 确认重启，plus → biz1 → plus 往返切号成功。每次 ChatGPT.exe 被结束并重新拉起，`written` 只含 `C:\Users\<user>\.codex\auth.json`，WSL 侧 `auth.json` 未被改动，`state.json` 与 `current_label` 同步更新。

### 本次修复

- WSL 下 Agent 脚本无条件走 `--agent-stdio` 桥接，并把 Windows 风格 store 路径传给 Linux 版 `gpa-manager`，导致在当前目录字面创建 `C:\Users\...` 空 store 并拉起第二个后台，`accounts` 返回空。现在 loopback 可达时直接复用现有后台；桥接时按目标可执行文件类型选择路径写法；Linux/WSL 二进制收到 `C:\...` 也会转换到 `/mnt/<drive>/...`。
- 目标座位已是该账号时，App 运行中会被判成 `APP_RESTART_REQUIRED` 而不是 `noop`，人类点「切换」到当前账号会被无意义地要求重启。现在无冲突且探测未失败时直接 `noop`。

## 未当作已验证

- Windows 原生双击 `GPA Manager.exe`（已交叉编译，未在独立 Windows 会话点击）。
- WSL → Windows `--agent-stdio` 在真实已安装 Windows exe 上的联通（本次后台由 Linux 二进制承载，桥接分支未走到）。
- 真实 Codex 设备码登录；隔离登录进程和设备码解析使用 fake process fixture 验证。
- 当前会话凭据的在线有效性（`verification.online` 始终为 `not_checked`）。
- `all` 目标下 WSL CLI 的真实写入（本机 Codex 正在运行，未关闭）。

## 需要用户安排窗口后才做

真实设备码授权、`all` 目标真实切号、Windows exe 桥接实机联通。没有这些窗口时不能声称已经完成这些验证。
