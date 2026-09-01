# 贡献者指南

感谢你参与 File Share Manager。提交 Issue 或 Pull Request 前，请先搜索是否已有相同问题，并说明受影响的组件、版本和部署方式。

## 开发环境

- Go 1.26.5，以 `server/go.mod` 为准。
- Node.js 22 和 npm，前端依赖锁定在 `frontend/package-lock.json`。
- MySQL 8.3+；也可以使用根目录的 Docker Compose 启动本地依赖。
- Docker 及 Docker Compose，用于验证生产镜像和完整部署。

安装依赖：

```bash
cd server && go mod download
cd ../frontend && npm ci
```

## 本地验证

提交前至少运行：

```bash
make test
make vet
make swag

cd server
go build ./...

cd ../frontend
npm run build:prod
```

修改 Go 代码后应对本次涉及的 `.go` 文件运行 `gofmt -w`。新增代码还应尽量通过对应包的 `go vet` 检查；如果命中仓库已有告警，请在 Pull Request 中说明。

修改 API 路由、请求参数或响应结构时，必须同步更新 Handler 的 Swagger 注释，运行 `make swag`，并提交 `server/docs` 下的三个生成文件。路由测试会校验每个 `/api/fileshare/v1` operation 都存在于生成文档中。

涉及数据库模型时，必须同时提供可审阅的增量迁移，并更新迁移相关文档。不要修改已发布迁移的内容。

## 分支与提交

建议使用以下分支前缀：

| 前缀 | 用途 | 示例 |
| --- | --- | --- |
| `feature/` | 新功能 | `feature/batch-download` |
| `fix/` | 缺陷修复 | `fix/share-expiry` |
| `docs/` | 文档 | `docs/deploy-guide` |
| `refactor/` | 重构 | `refactor/storage-engine` |

提交信息建议遵循 Conventional Commits：

```text
feat(server): 支持批量下载 ZIP 打包
fix(frontend): 修复文件列表分页条件
docs(deploy): 补充生产发布检查项
```

## Pull Request 检查项

- 说明问题背景、实现方案、兼容性和验证结果。
- 只包含同一目的的改动，避免夹带无关格式化或重构。
- 新行为应补充测试；接口、配置或数据库变化应同步更新文档。
- 不得提交密码、Token、私钥、生产地址或真实业务数据。
- 确认 GitHub 和 CNB 的 CI 检查通过后再合并。
