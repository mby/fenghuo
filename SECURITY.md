# Security Policy / 安全策略

## Reporting a vulnerability

Please **do not** open a public issue. Report privately through
[GitHub Security Advisories](https://github.com/mby/fenghuo/security/advisories/new).
Include the affected version, a description, and steps to reproduce.

You can expect an acknowledgement within 7 days. Fixes are released as a new
patch version and disclosed in an advisory once users have had time to upgrade.

## Supported versions

Only the latest release receives security fixes.

## Scope

Fenghuo's core promise is that **a compromised server cannot run anything on
monitored machines**, because the agent has no code path to receive commands.
Any way to make an agent execute code, read files, change its configuration or
open a listening socket is treated as critical.

See [docs/security.md](docs/security.md) for the threat model.

---

## 报告漏洞

请**不要**提交公开 Issue，通过 [GitHub Security Advisories](https://github.com/mby/fenghuo/security/advisories/new)
私下报告，并附上受影响版本、问题描述和复现步骤。

我们会在 7 天内确认收到。修复会以新的补丁版本发布，并在用户有时间升级后公开说明。

## 支持的版本

只有最新版本会获得安全修复。

## 范围

烽火的核心承诺是：**即使服务端被攻破，也无法在被监控的机器上执行任何操作**，因为 Agent
根本没有接收指令的代码路径。任何能让 Agent 执行代码、读取文件、修改配置或监听端口的问题都视为严重漏洞。

威胁模型见 [docs/security.zh-CN.md](docs/security.zh-CN.md)。
