# gpt-account (`GPA`)

人类双击打开本地网页管理 ChatGPT / Codex 账号。Agent 用同一个后台上的 Python 脚本和 Skill 操作。不要在 App 里点 Logout。

这是本机管理。VPS、跨机器接力和云同步还没做。

## 人类

解压 Windows 包后双击 `GPA Manager.exe`。不需要安装 Python、Node 或 Go。

首页只有：

- 切换到哪里，默认桌面端
- 账号列表和「切换」
- 添加账号
- 最近操作、设置

没有阻塞时点一次即可。需要重启 App 时，页面会写明「重启并切换到哪个账号」。Windows App 和 Windows CLI 如果共用登录，页面会说明会一起更新。「本机全部」必须自己选。

从源码启动演示数据：

```bash
export PATH="$HOME/.local/go/bin:$PATH"
go run ./cmd/gpa-manager --demo --listen 127.0.0.1:18765
```

## Agent

```bash
python3 agent/gpa_agent.py status
python3 agent/gpa_agent.py accounts
python3 agent/gpa_agent.py preview --account biz1 --target desktop
python3 agent/gpa_agent.py switch --plan-id PLAN_ID --request-id REQUEST_ID
python3 agent/gpa_agent.py operation --id OP_ID
```

脚本只输出 JSON。Skill 在 `skills/gpa-account-manager/`。需要重启正在运行的 App 时，脚本返回 `waiting_user`，请到网页确认，不要结束当前会话。

## 开发

```bash
export PATH="$HOME/.local/go/bin:$PATH"
go test ./...
./scripts/package-web.sh
```

测试用伪造 JWT 和临时目录，不碰真实 `auth.json`，也不重启 ChatGPT。

旧终端 `gpa` 仍可编译，不再作为产品入口。`src/gpa` 是更早的 Python 原型。
