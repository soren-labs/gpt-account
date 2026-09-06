---
name: gpa-account-manager
description: 使用 GPA 管理本机已保存的 Codex/ChatGPT 订阅登录，包括查看账号、预览和切换本地客户端、导入账号及协助更新授权。适用于用户要求 GPA 换号或本地账号管理；不用于 API key 管理、远程 VPS 部署或一般 Codex 设置。
---

# GPA 本地账号管理

使用本技能目录下的 `scripts/gpa_agent.py`，不要操作终端菜单、模拟网页点击或自行复制 auth.json。

查询账号和客户端使用 `status` / `accounts`。用户要求切换时，先解析账号和目标；沿用会话已确定的选择，存在歧义才询问。先 `preview`，根据返回计划执行 `switch`，保留 request_id 和 operation_id。

需要等待时使用 `operation` 或有界 `wait`。网络超时后先查询同一请求，不创建另一个切换。`succeeded` 才表示该操作完成；报告服务返回的验证层次，不把本地凭据匹配说成在线请求成功。

`waiting_user` 时遵循 next_action：需要重启 App 就打开对应管理页面并告诉用户；CLI 正在使用时说明要关闭哪个客户端。`blocked` 时说明 reason_code，不增加 force、不清空文件、不执行 Logout。参数不明确和服务不可用的处理见 `references/protocol.md`。

添加或更新账号时启动隔离登录任务，把官方授权交给用户。不要把设备码或认证资料写入普通日志。改名、归档和导入只在用户请求这些动作时执行。

脚本路径必须从本技能所在目录解析，不假设当前工作目录。脚本可启动已配置后台；如果缺少安装配置，返回安装位置和明确说明，不搜索并尝试运行不明可执行文件。
