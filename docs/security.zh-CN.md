# 安全模型

[English](security.md)

## 烽火要解决的问题

2026 年，一款被广泛使用的开源探针遭到大规模利用：攻击者先拿下面板（例如利用路径穿越读到 JWT 密钥伪造
登录，或利用权限校验缺失让低权限用户创建命令任务），再用面板自带的"在 Agent 上执行命令"功能，
一次性在所有被监控的服务器上植入后门。Agent 以 root 运行，连接又是出站方向，防火墙无能为力。

逐个修补漏洞解决不了设计问题：只要 Agent 会执行面板下发的内容，面板就是打开所有机器的一把钥匙。

## 设计

**Agent 无法被控制。** 它只是一个循环：读取 `/proc`，向 `/api/v1/report` POST JSON，丢弃响应体。
协议中不存在从服务端到 Agent 的消息类型。Agent 从响应中唯一获取的是成功状态码：`204`（有人在查看面板）
或 `202`（没人查看），并据此在本地配置的两个间隔 `-interval` 和 `-idle-interval` 之间切换。服务端无法
指定任何间隔数值，服务端被攻破时，利用这个信号最多只能让 Agent 一直按正常间隔上报。配置只来自本机 systemd 环境文件：`/etc/fenghuo/agent.env`，以及可选的 TCP 探测文件 `/etc/fenghuo/probes.env`。
后台探测模板仅生成由管理员在本机执行的命令。测试 `cmd/fenghuo-agent/deps_test.go` 会在以下情况让构建失败：Agent 依赖了第三方包，依赖了
`os/exec`、`plugin`、`net/rpc` 等包，或代码中调用了 `net.Listen`、`ListenAndServe`。

**Agent 上没有值得窃取的东西。** 它以 systemd `DynamicUser` 运行：capability 为空，
`ProtectSystem=strict`、`NoNewPrivileges`、`PrivateDevices`、`MemoryDenyWriteExecute`，系统调用限制在
`@system-service`，内存上限 64 MB。它不监听端口，不写磁盘。Token 由 systemd 从仅 root 可读的 `0600`
文件中读取，不出现在命令行里。

**管理入口是隔离的。** 服务端有两个监听端口。公开端口（`127.0.0.1:8080`，放在你的 TLS 代理后面）
只提供上报接口和只读状态 API，管理 API 在这里根本没有注册。管理 API 位于第二个监听端口
`127.0.0.1:8081`，通过 SSH 隧道访问。

**服务端的纵深防御：**

- 单一管理员账号，密码使用 argon2id 哈希（19 MiB，t=2）。
- 可选的 TOTP 两步验证，带防重放。
- 登录限流：每个 IP 每 15 分钟最多失败 10 次，全局最多 100 次。
- 会话存在服务端，只保存 SHA-256 哈希，通过 `HttpOnly`、`SameSite=Strict` 的 Cookie 下发（HTTPS 下带
  `Secure`）。不使用 JWT，也就没有可泄露的签名密钥。修改密码会注销所有会话。
- 使用 Go 的 `CrossOriginProtection` 拒绝跨站的写操作请求。
- 严格的 CSP（`script-src 'self'`、`frame-ancestors 'none'`），以及 `nosniff`、`Referrer-Policy: no-referrer`。
- 前端嵌入在二进制中，从内存提供，没有可穿越的文件路径。
- 所有请求体都有大小限制，并拒绝未知字段；上报数据会做范围校验。
- 节点 Token 是 256 位随机值，只保存哈希，只显示一次。
- 公开 API 从不返回 IP、Token 和私有备注；隐藏的节点只对已登录的管理员可见。
- 发送通知时会在 DNS 解析后拒绝内网、回环、链路本地和 CGNAT 地址，防止 SSRF 和 DNS 重绑定。
- 服务端以非特权用户 `fenghuo` 运行并使用加固的 systemd 配置，或以非 root 的 distroless 容器运行。

**最小化供应链。** Agent 只依赖 Go 标准库。服务端额外依赖 `modernc.org/sqlite`（纯 Go）和
`golang.org/x/crypto`。前端依赖只在构建时需要；pnpm 配置为只安装发布满一天的版本，并且不执行任何
安装脚本。

## 被攻破后的影响范围

| 被攻破的部分 | 攻击者能做到 | 攻击者做不到 |
| --- | --- | --- |
| 管理员账号或整个服务端 | 查看、伪造监控数据，删除节点，发送通知 | 在 Agent 上执行任何操作、读取 Agent 上的文件、修改 Agent 配置 |
| 某个节点的 Token | 为该节点提交伪造数据 | 读取任何数据，影响其他节点 |
| 网络链路（未使用 TLS） | 读取监控数据和 Token | 控制 Agent；这也是 Agent 拒绝明文 `http://` 的原因 |
| 某台 Agent 主机 | 读取该主机的 Token | 访问管理 API 或其他节点 |

服务端被完全攻破时，最坏结果是看到一个被篡改的面板。恢复方法是重装服务端并重置各节点 Token。

## 加固清单

- [ ] 修改初始管理员密码，并开启两步验证。
- [ ] 让 `-admin-listen` 保持在回环地址或内网地址上。
- [ ] 公开端口只通过 HTTPS 提供服务。
- [ ] 把 `-trusted-proxies` 设置为恰好是你的代理地址。
- [ ] 不需要公开状态页时将其关闭。
- [ ] 及时更新服务端。Go 标准库编译在二进制中，安全修复只能通过新版本获得。
- [ ] 主机被入侵或下线后，重置该节点的 Token。

## 校验发布文件

安装脚本会用同一版本的 `checksums.txt` 校验每个压缩包。手动校验：

```bash
sha256sum -c checksums.txt --ignore-missing
```

每个发布压缩包和容器镜像都附带 GitHub 构建来源证明（build provenance），可以证明它由本仓库的发布
工作流构建：

```bash
gh attestation verify fenghuo-agent_linux_amd64.tar.gz --repo mby/fenghuo
gh attestation verify oci://ghcr.io/mby/fenghuo:latest --repo mby/fenghuo
```

## 报告漏洞

参见 [SECURITY.md](../SECURITY.md)。
