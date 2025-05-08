# 简介

将 WireGuard 的 conn.StdBind 替换为 WebRTC 实现, 更多细节看 [bind_test.go](./bind/bind_test.go)

# 架构

先用 WebSocket 建立连接, 等 WebRTC 连接成功再切换过去
