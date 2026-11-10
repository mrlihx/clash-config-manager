# 第三方组件与规则来源

项目自身代码采用 MIT。以下组件、工具和远程规则按各自许可提供，不因本项目采用 MIT 而改变。

## 页面和编辑器

- 其中 Tailwind CSS 的许可保留在 [LICENSES/TailwindCSS.txt](LICENSES/TailwindCSS.txt)。
- CodeMirror、Lezer 及编辑器打包依赖的许可保留在 [web/licenses/editor-vendor.LICENSE.txt](web/licenses/editor-vendor.LICENSE.txt)。更新编辑器产物时应保留该文件。

Go 依赖及版本列于 `go.mod` 和 `go.sum`，依赖本身保留上游版权和许可。

## 远程规则

默认配置引用 [MetaCubeX/meta-rules-dat](https://github.com/MetaCubeX/meta-rules-dat) 和 [Loyalsoldier/clash-rules](https://github.com/Loyalsoldier/clash-rules)。两者仓库的许可为 GPL-3.0，文本副本见 [LICENSES/GPL-3.0.txt](LICENSES/GPL-3.0.txt)，具体数据来源以各上游说明为准。

本仓库保留来源地址和初始化定义，不再分发旧服务规则快照。用户运行时下载的规则和导入的数据不属于本项目的 MIT 授权范围；重新分发时应查看相应来源的要求。

## Mihomo

[Mihomo](https://github.com/MetaCubeX/mihomo) 是独立的 GPL-3.0 程序，由本工具通过命令行调用。Dockerfile 默认从官方 Release 下载 v1.19.33，也允许构建时指定其他版本。

容器中的 `/usr/share/licenses/mihomo/` 保存该版本的 LICENSE 和源码地址。默认版本源码为 [v1.19.33](https://github.com/MetaCubeX/mihomo/tree/v1.19.33)。修改版本时，二进制、许可和源码地址随构建参数一起更新。自行发布镜像或二进制包时，应提供与所分发版本对应的源码获取方式及许可文件。
