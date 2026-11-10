# Clash 配置管理器

把多个订阅里的节点整理到一起，再按自己的分流规则生成订阅。适用于 Mihomo / OpenClash，也能向 v2rayNG 等客户端提供节点订阅。

后端使用 Go，前端使用原生 HTML、CSS 和 JavaScript。页面和编辑器资源都嵌入可执行文件，数据保存在 SQLite，不需要单独部署前端。

## 能做什么

- 从订阅链接或 YAML 导入节点，按来源分组管理，也可以手动添加、修改节点。
- 为不同用途维护规则方案，细分 Google Play、Google、AI、社交、流媒体等海外服务；局域网保留直连，国内分流按订阅选择。
- 使用远程规则集或手写规则，调整顺序、域名例外和广告拦截。
- 从选中的节点生成独立订阅，自定义或随机生成地址后缀，显示二维码。
- 定时更新来源，并将节点、规则方案的变更同步到关联订阅。
- 在订阅中使用可视化设置或 YAML 编辑器，保留单个订阅的自定义内容。

这是配置管理工具，不代替路由器或客户端转发代理流量。Mihomo 在服务端用于检查生成的配置。

## 本地启动

需要 Go 1.26，以及支持所用协议的 [Mihomo](https://github.com/MetaCubeX/mihomo)。

```sh
cp config.example.yaml config.yaml
```

编辑 `config.yaml`，填写管理员密码，并确认内核路径：

```yaml
listen: 0.0.0.0:8787
database: data/clash.db
mihomo: /usr/local/bin/mihomo
admin-password: "请改成自己的密码"
```

密码至少 5 字节。随后运行：

```sh
go run ./cmd/app
```

打开 `http://127.0.0.1:8787`。首次启动会创建数据库。管理员登录状态会保留，重启服务不需要重新登录；更改密码后旧会话失效。

运行配置只有上面四项，不读取环境变量。可以用 `-config /path/to/config.yaml` 指定配置文件；相对数据库路径和带目录的相对内核路径以配置文件所在目录为基准。

## 使用流程

1. 在「配置文件」导入来源订阅。默认只采纳节点及其前置依赖，分流规则由系统生成。
2. 在「节点管理」展开来源，勾选需要合并的节点，点击生成订阅地址。
3. 选择规则方案、规则集和海外分流组，保存订阅。
4. 在客户端添加生成的链接。以后可在「订阅管理」编辑设置或完整 YAML，地址保持不变。

「路由策略」中的规则集、分流规则和策略组属于规则方案。订阅启用跟随方案时，方案的修改会同步过去；订阅中单独修改过的部分会尽量保留。关闭跟随后，可以独立维护该订阅。

### 节点如何同步

全选一个来源时，订阅跟随该来源新增、删除节点；只选部分节点时，按原节点名称跟踪。节点改名视为删除旧节点、新增节点。手动修改的节点字段会保留，包含全部节点的策略组会跟随新增节点，手动挑选成员的组只保留仍存在的成员。

删除来源不会删除已生成的订阅。更新失败或关联节点全部消失时，保留最近可用内容。服务端完成更新后，客户端仍需要刷新订阅才能拿到新配置。

### 客户端格式

同一地址仅按请求头中的 User-Agent 识别客户端：

- Clash、Mihomo、Stash：完整 Clash YAML。
- v2rayN、v2rayNG、Shadowrocket、NekoBox 等已识别客户端：Base64 节点链接。
- 未识别客户端及普通浏览器：默认 Clash YAML。浏览器直接显示文本。

通用节点链接不包含 Clash 分流规则。链式节点或无法转换的节点会跳过，数量放在 `X-Subscription-Skipped` 响应头；没有可转换节点时返回 422。具体协议能否使用，还取决于客户端内核。

## 规则来源

默认远程规则主要来自 [MetaCubeX/meta-rules-dat](https://github.com/MetaCubeX/meta-rules-dat) 和 [Loyalsoldier/clash-rules](https://github.com/Loyalsoldier/clash-rules)。来源地址、更新间隔和分流策略可以在后台修改，也可以新增文本规则集。

初始化定义随程序发布；后台保存后的方案、节点和订阅放在数据库中。升级程序不会直接用默认值覆盖用户编辑的内容。旧规则的校验摘要仅用于识别、迁移旧版本数据，详见 [初始化说明](internal/config/builtin/README.md)。

规则命中只代表选择了对应出口，不能保证目标服务的账号地区、出口 IP 或设备网络一定可用。

## 一键部署

在服务器执行，无需提前下载源码：

```sh
curl -fsSL https://raw.githubusercontent.com/mrlihx/clash-config-manager/main/deploy/install.sh | sudo bash -s -- --remote
```

默认安装到 `/opt/clash-config-manager`，只下载部署文件和预构建 Docker 镜像，Linux 上安装缺少的 Docker，通过 `http://服务器IP:8787` 访问。默认密码 `admin123`，后台右上角可修改密码，左侧“备份管理”可生成、上传和恢复数据库备份。重复部署保留已有密码和数据。服务器不需要 Git、Go、Node.js 或编译环境，需能访问 GitHub、GHCR 并开放 8787 端口。

## 构建和部署

```sh
go build -trimpath -ldflags='-s -w' -o bin/clash-config-manager ./cmd/app
./bin/clash-config-manager -config config.yaml
```

HTML、CSS、JavaScript 和 YAML 编辑器已全部内嵌。部署时带上可执行程序、`config.yaml` 和 Mihomo 即可；迁移现有数据还需要数据库。服务器不需要 Node.js 或 `web/` 目录。

公网部署、HTTPS、Docker Compose、systemd 和备份步骤见 [部署说明](deploy/README.md)。Docker 镜像已包含 Mihomo；直接运行二进制时需自行安装。

## 备份与在线更新

「备份管理」显示所有已生成和上传的备份，可下载、删除或导入恢复。上传支持本系统导出的 `.db` 文件，最大 128 MB；上传不会立即改变数据。恢复会覆盖全部节点、规则方案和订阅，自动保存一份恢复前备份，并要求重新登录；`config.yaml` 中的管理员密码不变。文件保存在 `data/backups/`，Docker 中随数据卷持久保存。

网站底部显示程序版本，点击「检查更新」可从 GitHub Releases 更新可执行程序。更新前自动备份数据库，校验 SHA-256 和 CPU 架构，替换后自动重启；配置、订阅地址和数据保持不变。更新文件保存在 `data/bin/`，容器重启后仍然使用更新后的程序。

旧 Docker 镜像需要先通过部署脚本升级一次，获得支持在线更新的启动器。之后更新管理程序无需重新部署 Docker；Mihomo、系统依赖和基础镜像仍通过 Docker 镜像更新。开发启动方式不支持在线更新。

## 镜像发布

GitHub Actions 在 main 更新或推送 `v*` 标签时检查代码、构建并验证运行镜像，然后发布 `linux/amd64` 与 `linux/arm64` 到 `ghcr.io/mrlihx/clash-config-manager`。main 对应 `latest`，同时保留提交 SHA 标签；版本标签保留原名称。

首次发布后，需要在 GitHub 的 Package settings 中将镜像可见性设为 Public，其他用户才能免登录拉取。Linux 可执行程序及 `SHA256SUMS` 同时发布到 [Releases](https://github.com/mrlihx/clash-config-manager/releases)。发布流程见 [publish.yml](.github/workflows/publish.yml)。

## 项目结构

```text
cmd/app/          程序入口
internal/config/  配置解析、节点转换、规则生成和校验
internal/server/  HTTP 接口、登录、订阅发布和同步
internal/store/   SQLite 存储和迁移
web/html/         HTML 页面
web/css/          CSS 样式
web/js/           JavaScript 与编辑器产物
web/js/build/     编辑器构建入口，不嵌入程序
web/licenses/     前端第三方许可
deploy/          部署示例和说明
config.example.yaml
```

运行数据统一放在配置文件旁的 `data/`：`clash.db` 保存业务数据，`cache.db` 和 `rules/` 是 Mihomo 缓存。启动时会迁移旧根目录缓存，已有目标文件不会被覆盖。

## 开发

```sh
go test ./...
go vet ./...
```

修改普通页面或脚本后，重新编译 Go 程序即可。只有更新 CodeMirror 编辑器依赖时才需要 Node.js：

```sh
npm ci
npm run build:editor
```

编辑器产物和第三方许可文件需要一起提交。其他开发约定见 [CONTRIBUTING.md](CONTRIBUTING.md)。

## 开源与授权

项目自身代码采用 [MIT](LICENSE)。第三方编辑器、规则数据和独立分发的 Mihomo 保留各自许可，见 [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md)。

`config.yaml`、数据库、缓存和本地构建产物已加入忽略列表。提交问题或配置示例前，请删除节点密码、订阅地址和其他私人信息。

### 外部规则与订阅地址设置

订阅保留外部规则集引用，不再把远程规则展开进同一个文件。规则地址、格式、更新间隔和分流顺序保持不变，客户端按各规则集的 interval 下载更新；手写规则仍保留原有内容。

「订阅管理 → 订阅地址」统一设置订阅名称、客户端更新间隔、地址后缀和外部资源加速。默认更新间隔 24 小时，可设置 1—720 小时；名称和间隔通过响应头提供给支持的客户端，不能强制开启 APP 的自动更新开关。

外部资源加速默认关闭。启用后可填写 HTTPS 加速前缀，仅在当前订阅输出中转换公开 GitHub 规则文件的 URL；平台保存的原始地址不变，关闭即可恢复。带查询参数或自定义请求头的规则不交给公共加速服务，节点地址、私人节点订阅和其他站点不受影响。第三方服务的可用性与速度需在实际客户端网络上验证。

平台的规则缓存和更新时间展示仍保留，缓存放在 `data/resources/`。订阅客户端直接获取外部规则，不依赖平台先下载完全部规则。

程序发布和镜像发布分别由 `release.yml` 与 `publish.yml` 处理。提交信息包含 `[release-only]` 时只发布可执行程序，不构建或推送 Docker 镜像，适用于已有在线更新能力的部署。

新建订阅的规则集选项默认全部不勾选，包括国内域名、国内 IP、海外域名、GFW、海外顶级域名和广告集合。海外服务分流组默认选中 Claude、Gemini、OpenAI、Google Play、YouTube、Google 服务、X/Twitter、Netflix、TikTok；编辑已有订阅保留其实际选择。未命中专属规则的流量由末尾 MATCH 策略处理。
