# 参与开发

提交问题时，说明复现步骤、运行方式、客户端和 Mihomo 版本。如果涉及配置，请用虚构节点替换真实凭据，不要上传数据库。

修改尽量围绕一个问题展开，沿用现有目录和页面样式。外部输入在接口或解析边界校验，内部避免重复判断；单次使用的逻辑不必提前抽象。复杂的同步、兼容和迁移逻辑可加简短中文注释。

提交前运行：

```sh
go test ./...
go vet ./...
for file in web/js/*.js web/js/build/*.js; do node --check "$file"; done
go build ./cmd/app
```

测试使用临时数据库，不要拿日常使用的数据库做测试。修改同步或迁移逻辑时，补充能覆盖用户自定义内容的测试。

前端由 Go 内嵌，修改后需要重新构建。`web/js/build/editor-vendor-entry.js` 是编辑器构建入口；更新依赖后运行 `npm ci && npm run build:editor`，同时保留产物中的许可说明。普通页面修改不需要构建编辑器。

发布时仅携带示例配置，不携带本地 `config.yaml`、`data/`、`node_modules/` 或 IDE 文件。重新分发第三方程序和规则数据时，保留其许可与来源说明。
