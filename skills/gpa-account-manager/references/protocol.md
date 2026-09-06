# GPA Agent 协议

脚本：`scripts/gpa_agent.py`。stdout 只有一个 JSON 文档。

## 命令

| 命令 | 作用 |
|---|---|
| `status` | 账号、目标、服务状态 |
| `accounts` | 账号列表 |
| `preview --account REF --target TARGET` | 生成切换计划 |
| `switch --plan-id ID --request-id REQ` | 按计划提交 |
| `operation --id ID` | 查询操作 |
| `wait --id ID --timeout N` | 有界等待，不取消 |
| `retry --id ID` | 同一编号重试 |
| `open-ui` | 返回管理页地址 |
| `diagnose` | 脱敏诊断 |
| `rename` / `archive` / `unarchive` | 账号管理 |
| `login-start` / `login-status` / `login-cancel` | 隔离登录 |

目标必须显式给出。`desktop` 是桌面端。`all` 只有用户明确要求时才用。

## 退出码

| 码 | 意义 |
|---|---|
| 0 | 查询成功或操作 succeeded |
| 1 | failed / 恢复失败 |
| 2 | queued / running / waiting_user |
| 3 | blocked |
| 4 | 参数或账号不明确 |
| 5 | 服务或传输不可用 |

查询成功时 JSON `status=ok`。操作结果看 `status` 字段，不要只看退出码。

## 错误码

- `APP_RESTART_REQUIRED`：打开网页确认重启，不要结束当前会话
- `CLI_IN_USE`：请用户关闭对应 CLI 后 `retry`
- `QUERY_FAILED`：重新探测，不要 force
- `CREDENTIAL_CONFLICT` / `PLAN_STALE`：重新 preview
- `TRANSPORT_UNAVAILABLE`：说明需要先打开 GPA Manager，不要写 auth.json
