# gpt-account (`gpa`)

本地切换 ChatGPT / Codex 官方登录。每个账号只做一次设备码授权，之后切号只换 `auth.json`，不再走网页登录。

这是 Astra 规划里的 **第一阶段：本地 CLI**。VPS 部署和跨机器凭据接力还没做。

## 安装

```bash
pip install -e '.[dev]'
```

会提供两个命令：`gpa` 和 `gpt-account`。

## 从现有槽位导入

如果本机已经有 `~/.local/share/gpt-accounts`（之前的原型脚本）：

```bash
gpa migrate
gpa list
```

默认写入 `~/.local/share/gpa`，不会改旧目录里的文件。

## 日常用法

```bash
gpa                 # 终端里选账号
gpa use plus        # 写入 Windows + WSL 的 auth.json，并重启 ChatGPT.exe
gpa use biz1 --no-restart
gpa status
gpa list            # 只读，不会回写令牌
gpa save            # 把当前 live 登录存进对应槽
gpa login biz3      # 独立目录跑 codex login --device-auth，不动正在用的号
```

切号顺序：停掉 ChatGPT → 把 live 里刚轮转过的令牌收回原槽 → 写入目标账号 → 启动 → 核对两边 `auth.json` 身份一致。

同一 Business workspace 的两个席位按 `user_id + workspace_id` 区分，不会再被 workspace UUID 误判成同一个人。

## 不要做的事

不要在 ChatGPT App 里点 Logout。官方登出会清掉桌面会话，还可能作废 refresh token。要换号就 `gpa use`。

会话目录不按账号拆。三个官方号共用同一份 Codex 历史。

## 环境变量

| 变量 | 作用 |
|---|---|
| `GPA_STORE` | 账号库目录，默认 `~/.local/share/gpa` |
| `GPA_CODEX_HOME` | WSL / Linux 的 Codex home |
| `GPA_WINDOWS_CODEX` | ChatGPT.exe 读的 Windows Codex home |
| `GPA_CHATGPT` | `auto` 或 `off`（测试用，不杀进程） |

## 开发

```bash
python -m pytest
```
