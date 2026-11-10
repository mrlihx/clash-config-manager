# 生产部署

推荐 Docker Compose：直接拉取包含管理程序、前端和 Mihomo 的预构建镜像。Caddy 对外提供 HTTPS，SQLite、登录会话和规则方案放在持久卷中。应用所有运行设置仍来自根目录 YAML，没有环境变量配置。

## 一键部署

在服务器执行，无需提前下载源码：

```sh
curl -fsSL https://raw.githubusercontent.com/mrlihx/clash-config-manager/main/deploy/install.sh | sudo bash -s -- --remote
```

脚本安装到 `/opt/clash-config-manager`，只从 GitHub 下载 Compose 和示例配置文件；缺少 Docker 时调用官方安装脚本。随后从 GHCR 拉取预构建镜像并启动，不下载源码、不编译，也无需安装 Git、Go 或 Node.js。默认访问 `http://服务器IP:8787`，密码为 `admin123`，可在后台右上角修改；修改后需要重新登录。服务器需开放 8787 端口。重复执行保留已有配置和数据。

日后更新执行同一命令，拉取最新镜像并重建容器。配置文件、数据库和原有 `clash-config-manager_manager-data` 数据卷保留，旧版源码构建部署也可沿用。此方式使用 `compose.quick.yaml`，无需配置域名；需要 HTTPS 时使用下面的 Caddy 部署方式，两种方式选择一种启动即可。

后台“备份管理”支持生成、上传、下载和恢复 SQLite 备份，不需要停止服务；恢复前自动备份，恢复后重新登录，管理员密码保持不变。备份包含节点凭据和登录会话，请妥善保管；启动配置文件需单独保留。

## Docker Compose

适用于 Linux amd64 / arm64 服务器，需要已安装 Docker Engine 和 Compose 插件。准备 `compose.yaml`、`config.example.yaml` 和 `deploy/Caddyfile` 即可，无需项目源码。不要把本机配置或数据库上传到公开仓库。

1. 准备配置并编辑管理员密码：

   ```sh
   cp config.example.yaml config.production.yaml
   nano config.production.yaml
   sudo chgrp 10001 config.production.yaml
   chmod 660 config.production.yaml
   ```

   容器以 UID/GID 10001 运行。配置文件允许容器组读写，用于保存后台修改的密码；数据库和规则缓存使用命名卷，不需要手工处理目录权限。

2. 将域名解析到服务器，在 `deploy/Caddyfile` 把 `clash.example.com` 换为实际域名。服务器允许访问 80/443 端口。已有 Nginx/Caddy 占用端口时，移除 compose 中的 caddy 服务，将现有反向代理指向 manager 服务或将管理端口仅映射到 `127.0.0.1:8787`。

3. 启动：

   ```sh
   docker compose pull
   docker compose up -d --wait
   docker compose ps
   docker compose logs --tail=100 manager
   ```

4. 使用 `https://你的域名` 登录；生成的订阅链接、二维码会自动使用该域名。

镜像由 GitHub Actions 构建，已包含 Mihomo。无需在服务器单独安装内核，也不需要 privileged、host 网络或 TUN 权限。管理程序只调用 `mihomo -t` 校验配置，不承担用户代理流量。

更新镜像：

```sh
docker compose pull
docker compose up -d --wait
```

普通重建、重启和 `docker compose down` 保留数据；不要使用 `down -v`，它会删除持久卷。如需固定版本，可将 Compose 中的 `latest` 改为已发布的版本标签或 SHA 标签。

### 备份与迁移

数据库包含节点凭据、规则方案、订阅来源关系和登录会话。使用停机复制以保证一致性，不要直接复制正在写入的 SQLite 文件。

备份：

```sh
mkdir -p backup
docker compose stop manager
docker compose cp manager:/app/data/clash.db ./backup/clash.db
docker compose start manager
chmod 600 ./backup/clash.db
```

从本机迁移已有数据库到已创建的容器：先停止本机服务并复制其 `data/clash.db`，上传服务器后执行：

```sh
docker compose stop manager
docker compose cp ./backup/clash.db manager:/app/data/clash.db
docker compose run --rm --user root --entrypoint sh manager -c 'chown 10001:10001 /app/data/clash.db && chmod 600 /app/data/clash.db'
docker compose start manager
```

服务器使用与旧环境一致的管理员密码时，旧会话可在同一访问域名下继续有效；更换域名通常需要重新登录。规则缓存可以重新下载，不是恢复所必需的数据。

## 二进制 + systemd

不使用 Docker 时，服务器需另外安装对应架构的 Mihomo，并在 `config.yaml` 填写其路径。前端已嵌入 Go 二进制，服务器不需要 Node.js。

在开发机交叉编译，例如 Linux amd64：

```sh
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags='-s -w' -o bin/clash-config-manager-linux-amd64 ./cmd/app
```

服务器目录建议：

```text
/opt/clash-config-manager/
  config.yaml
  bin/clash-config-manager
  bin/mihomo
  data/clash.db
  data/rules/
```

先将 `config.production.yaml` 的 `mihomo` 改为 `/opt/clash-config-manager/bin/mihomo`；使用本机 HTTPS 反向代理时，将监听地址改为 `127.0.0.1:8787`。然后创建服务用户、安装二进制与配置：

```sh
sudo useradd --system --home /opt/clash-config-manager --shell /usr/sbin/nologin clash-manager
sudo mkdir -p /opt/clash-config-manager/bin /opt/clash-config-manager/data /opt/clash-config-manager/data/rules
sudo install -m 755 bin/clash-config-manager-linux-amd64 /opt/clash-config-manager/bin/clash-config-manager
sudo install -m 755 mihomo /opt/clash-config-manager/bin/mihomo
sudo install -m 640 -o clash-manager -g clash-manager config.production.yaml /opt/clash-config-manager/config.yaml
sudo chown -R clash-manager:clash-manager /opt/clash-config-manager
sudo install -m 644 deploy/clash-config-manager.service /etc/systemd/system/clash-config-manager.service
sudo systemctl daemon-reload
sudo systemctl enable --now clash-config-manager
```

反向代理应保留原始 Host 并传递 X-Forwarded-Proto。订阅地址使用浏览器访问后台时的域名。

## 规则在哪里

- `internal/config/builtin/*.yaml` 及 Go 中的规则目录提供默认设置、规则来源和生成逻辑，随二进制发布。
- 后台修改后的规则方案、手写规则、节点和订阅覆盖保存在 `data/clash.db`，重启或升级不会用代码默认值重新覆盖。
- URL 规则集的实际域名/IP 内容由 Mihomo 按 interval 下载更新；手写规则集直接嵌入生成配置。
- 生产迁移要同时保留启动 YAML 和 SQLite 数据；仅上传二进制会得到一个新的空系统。

订阅在浏览器中直接按纯文本阅读，不再触发附件下载；客户端仍根据 User-Agent 获取 Clash YAML 或通用节点订阅。

Mihomo 的工作目录为配置文件所在目录下的 `data/`，因此运行缓存为 `data/cache.db`，默认规则缓存为 `data/rules/`。启动时会迁移旧根目录缓存（目标不存在时）；本地文件规则的相对路径同样以 `data/` 为基准。Docker 中这些缓存与 SQLite 一起由 manager-data 卷持久化。

## 网站底部在线更新

新版镜像通过启动器运行管理程序，支持从网站底部检查更新并安装 GitHub Releases 中的可执行文件。旧镜像第一次启用时先运行部署脚本更新镜像，之后管理程序升级无需重新创建容器。

启动器在 `data/bin/clash-config-manager` 存在时运行该更新程序，因此 Docker 重启不会回到旧版。更新前会生成数据库备份，下载通过 SHA-256 和 ELF 架构检查后才替换，并等待当前请求、后台任务和数据库正常关闭后重启。更新失败不会替换现有程序。

在线更新不会更新 Mihomo 或基础镜像。需要回到镜像自带程序时，停止服务并删除数据卷中的 `data/bin/clash-config-manager`，然后启动容器；业务数据和备份不受影响。

## 维护者发布镜像

`.github/workflows/publish.yml` 在 main 更新、`v*` 标签或手动触发时执行：代码检查、实际容器启动及改密/备份验证、发布 Linux 可执行程序及校验文件、验证在线更新和容器重启后的版本、多架构构建、推送 GHCR。使用仓库自动提供的 `GITHUB_TOKEN`，无需另外配置镜像密码。

首次发布的 GitHub Package 默认可能是 Private。维护者需到 `https://github.com/users/mrlihx/packages/container/clash-config-manager/settings` 将 Visibility 改成 Public；否则其他人的 `docker pull` 会提示 denied。

镜像包含 amd64、arm64，Docker 按服务器架构自动选择。服务器部署过程不执行 Dockerfile。维护者需要自行构建镜像时可运行：

```sh
docker build -t clash-config-manager:local --build-arg GOPROXY=https://proxy.golang.org,direct .
```

一键部署仍需要网络能访问 raw.githubusercontent.com、ghcr.io；首次安装 Docker 还需要其官方安装源。无法访问镜像仓库时请先处理服务器网络。
