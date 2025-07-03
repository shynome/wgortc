# 简介

将 WireGuard 的 conn.StdBind 替换为 WebRTC 实现, 更多细节看 [bind_test.go](./bind/bind_test.go)

# 架构

先用 WebSocket 建立连接, 等 WebRTC 连接成功再切换过去

# 双重许可

如果你希望作品可以闭源发布, 可以向我购买闭源许可. (基于此贡献代码需要签署 CLA 允许我商业化售卖闭源许可)

当然如果作品是开源的, 只要遵守 GPL3.0 许可, 将你的代码开放给软件使用者即可.
GPL3.0 不会传染到服务端, 你可以随意魔改服务端
