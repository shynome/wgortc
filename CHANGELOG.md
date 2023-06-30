# Changelog

## [0.0.11] - 20230630

### Improve

- 添加 `NewSettingEngine` 便于第三方使用时进行过滤网卡和 IP 过滤之类的操作

## [0.0.10] - 20230630

### Fix

- 当 port 为 0 时现在所有 ip 都会监听同一个随机端口, 而不是每个地址不同端口
- 当监听某些 IP 地址失败时跳过这些地址而不是报错
- UDPMux now is working as expect

## [0.0.9]

### Fix

- 当 pc 断开连接时, 直接 pc.Close() 关闭 dc, 使得后续连接可以重连

## [0.0.8]

### Fix

- GetSelectedCandidatePair also maybe return nil, add a check

## [0.0.7]

### Change

- DstToString now export webrtc remote pair ip:port

## [0.0.6]

### Fix

- ep.dc maybe is nil, now have a check

## [0.0.5]

### Change

- 现在直接使用协程发送信息, 不再等待信息是否发送完成, 更符合 udp 特性, 管发不管送达

## [0.0.4]

### Fix

- 连接方现在使用 ice servers
- 连接方现在提供自身的连接信息给对等点了

## [0.0.3]

### Change

- 不再超时断开 webrtc 链接

## [0.0.2]

- [x] endpoint return a fake addr for compat wg show

## [0.0.1]

- [x] close PeerConnection if it long time no packet send
- [x] webrtc peer connection connect only when wireguard has reponsed
