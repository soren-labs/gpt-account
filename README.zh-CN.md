# gpt-account (GPA)

[English](README.md) · **简体中文**

在一台机器上管理多个 ChatGPT / Codex 官方订阅登录，切换账号时不必重复走浏览器授权。

人类通过本地网页操作；Agent 通过同一个后台上的 Python 脚本和 Skill 操作。两者看到的是同一份账号库、同一组操作记录。

> 这是本机管理工具。VPS、跨机器接力和云同步尚未实现。**不要在 ChatGPT App 里点 Logout**，那会使保存的凭据失效。

## 功能

- 保存多个官方登录的凭据，一键在它们之间切换
- 切换目标可选：桌面端（ChatGPT App + Windows CLI 共用一份登录）、WSL 发行版 CLI、本机全部
- 切换前先预览：会写哪些文件、哪些客户端正在运行、是否需要重启 App
- App 正在运行时，网页会明确询问「重启并切换到哪个账号」；Agent 不能替用户确认重启
- CLI 正在使用时阻止写入，等用户关闭后重试
- 操作幂等，有编号可查；后台中断后能恢复
- 网页与 Agent 输出都不包含 token；邮箱只显示脱敏后的提示
- 网页支持英文 / 中文，默认跟随浏览器语言，可手动切换

## 人类使用

### Windows

解压发布包后双击 `GPA Manager.exe`。不需要安装 Python、Node 或 Go。浏览器会自动打开管理页。

首页只有这几件事：

1. **切换到哪里**：默认桌面端。「本机全部」需要自己选。
2. **账号列表**：每行一个「切换」按钮；当前已在位的账号显示「当前凭据」。
3. **添加账号**：启动一次官方设备码登录，把授权交给官方页面完成。
4. **最近操作 / 设置**：查看操作记录、导入已有账号、归档账号、诊断信息。

没有阻塞时点一次即可。需要重启 App 时，页面会写明「重启并切换到哪个账号」，确认后由后台结束并重新拉起 ChatGPT.exe。Windows App 和 Windows CLI 共用登录，页面会说明二者会一起更新。

### 从源码运行

```bash
export PATH="$HOME/.local/go/bin:$PATH"
go run ./cmd/gpa-manager                                  # 真实账号库
go run ./cmd/gpa-manager --demo --listen 127.0.0.1:18765  # 隔离演示数据
```

演示模式使用临时目录里的伪造账号，不会碰真实 `auth.json`，也不会重启 ChatGPT。

## Agent 使用

Skill 位于 `skills/gpa-account-manager/`，脚本为 `agent/gpa_agent.py`（Skill 包内是按字节一致的副本）。脚本只输出一个 JSON 文档，不含 token。

```bash
python3 agent/gpa_agent.py status
python3 agent/gpa_agent.py accounts
python3 agent/gpa_agent.py preview --account biz1 --target desktop
python3 agent/gpa_agent.py switch --plan-id PLAN_ID --request-id REQUEST_ID
python3 agent/gpa_agent.py operation --id OP_ID
```

流程约定：

- 先 `preview` 拿到计划，再按计划 `switch`，保留 `request_id` 和 `operation_id`
- `succeeded` 才表示完成；`waiting_user` 表示需要用户到网页确认（如重启 App）或关闭 CLI，不要结束当前会话
- `blocked` 时说明 `reason_code`，不要 force、不要清空文件、不要执行 Logout
- 后台未启动时脚本会从可信路径自动拉起；已有后台在 loopback 可达时直接复用

完整命令、退出码和错误码见 `skills/gpa-account-manager/references/protocol.md`。

## 存储位置

| 平台 | 账号库 |
|---|---|
| Windows / WSL | `%LOCALAPPDATA%\gpa`（WSL 下为 `/mnt/c/Users/<user>/AppData/Local/gpa`） |
| 其他 Linux | `~/.local/share/gpa` |

Windows 与 WSL 共享同一份账号库，因此网页和 Agent 无论从哪一侧启动，看到的都是同一组账号和操作。可用 `GPA_STORE` 覆盖。

## 开发

```bash
export PATH="$HOME/.local/go/bin:$PATH"
go test ./...
go test -race ./...
make test          # 同时校验 Skill 包脚本与 agent/gpa_agent.py 一致
make build         # 输出 Linux 与 Windows 二进制到 dist/
```

测试使用伪造 JWT 和临时目录，不碰真实凭据，也不重启 ChatGPT。真实账号与真实 App 重启的验证记录见 `docs/TEST-REPORT.md`。

仓库结构：

- `cmd/gpa-manager`：本地网页后台与 Agent 桥接入口
- `cmd/gpa`：旧终端 CLI，仍可编译，不再作为产品入口
- `internal/app`：账号、切换计划、操作、登录任务
- `internal/httpapi`：本地 HTTP API（仅 127.0.0.1，会话 + CSRF + Origin 检查）
- `internal/gpa`：凭据解析、客户端发现、进程探测、事务写入
- `web/`：管理页静态资源，随二进制嵌入
- `agent/`、`skills/`：Agent 脚本与 Skill 包
- `src/gpa`、`tests/`：更早的 Python 原型

## 许可

MIT，见 `LICENSE`。
