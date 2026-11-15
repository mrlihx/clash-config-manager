# 初始化定义

`defaults.yaml` 保存端口、DNS 等基础默认值；`providers.yaml` 定义通用远程规则来源。两者编译时嵌入程序。

新方案通过远程地址获取规则，内容按 interval 更新。后台保存的修改放在数据库，不依赖这里的文件。

旧版本曾内嵌 2026-11-08 的服务规则快照。如今只在 `legacy_snapshots.go` 保留其 payload 的 JSON SHA-256，供 `UpgradeRoutingDefaults` 识别：只有内容与原版完全一致才改成远程规则集，用户改过的规则继续保留。程序不再携带完整旧规则数据。
