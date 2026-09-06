# 测试报告

日期：2026-09-06

## 已验证

- `go test ./...`：账号 ID、预览/幂等切换、查询失败阻断、Agent 不能确认重启、HTTP Origin/CSRF、列表不含 token。
- 既有 `internal/gpa` 隔离测试（伪造 JWT、临时目录）。
- 演示服务首页 HTML、session、desktop 目标下列出三个席位、切换 biz2 返回 succeeded；列表响应不含 token。当前环境没有可用的浏览器自动化，页面点击用同源 HTTP 流程代替。
- Agent 脚本在 `GPA_FORCE_LOOPBACK` 或本机 loopback 可达时查询同一后台。
- Skill 包脚本与 `agent/gpa_agent.py` 发布前按字节复制。

## 未当作已验证

- Windows 原生双击 `GPA Manager.exe`（已交叉编译，未在独立 Windows 会话点击）。
- WSL → Windows `--agent-stdio` 在真实已安装 exe 上的联通。
- 真实 Codex 设备码登录。
- 真实 ChatGPT App 重启与真实切号。
- 当前会话凭据的在线有效性。

## 需要用户安排窗口后才做

真实授权、真实 App 重启、真实切号。没有这些窗口时不能声称已经完成在线验证。
