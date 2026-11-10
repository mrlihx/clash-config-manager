# 生产部署

推荐 Docker Compose：一次构建包含管理程序、前端和 Mihomo。Caddy 对外提供 HTTPS，SQLite、登录会话和规则方案放在持久卷中。应用所有运行设置仍来自根目录 YAML，没有环境变量配置。

## 一键部署

在服务器执行，无需提前下载源码：

```sh
curl -fsSL https://raw.githubusercontent.com/mrlihx/clash-config-manager/main/deploy/install.sh | sudo bash -s -- --remote
```

脚本安装到 `/opt/clash-config-manager`，自动拉取 GitHub 的 main 分支；缺少 Git 时通过系统包管理器安装，缺少 Docker 时调用官方安装脚本，然后构建并启动服务。默认访问 `http://服务器IP:8787`，密码为 `admin123`，可在后台右上角修改；修改后需要重新登录。服务器需开放 8787 端口。重复执行保留已有配置和数据。

日后更新使用同一命令，使用 Git 快进更新，不覆盖本地代码改动；如存在冲突，部署会停止并显示 Git 错误。配置文件和数据库保留。此方式使用 `compose.quick.yaml`，无需配置域名；需要 HTTPS 时使用下面的 Caddy 部署方式，两种方式选择一种启动即可。

后台“导出备份”生成当前 SQLite 的一致性副本，不需要停止服务。备份包含节点凭据和登录会话，请妥善保管；启动配置文件需单独保留。

## Docker Compose

适用于 Linux amd64 / arm64 服务器，需要已安装 Docker Engine 和 Compose 插件。将项目源码上传服务器，不要上传本机的 `config.yaml` 或包含凭据的数据库到公开仓库。

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
   docker compose up -d --build
   docker compose ps
   docker compose logs --tail=100 manager
   ```

4. 使用 `https://你的域名` 登录；生成的订阅链接、二维码会自动使用该域名。

Mihomo 在构建阶段从官方 Release 下载，默认固定 v1.19.33。无需在宿主机单独安装或运行 Mihomo，也不需要 privileged、host 网络或 TUN 权限。管理程序只调用 `mihomo -t` 校验配置，不承担用户代理流量。

升级内核可在构建时指定版本（构建参数不是运行环境配置）：

```sh
docker compose build --build-arg MIHOMO_VERSION=v1.19.33 manager
docker compose up -d
```

更新项目代码后重新运行 `docker compose up -d --build`。普通重建、重启和 `docker compose down` 保留数据；不要使用 `down -v`，它会删除持久卷。

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
