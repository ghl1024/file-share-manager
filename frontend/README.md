# File Share Manager Frontend

Vue 3 + Vite + Element Plus + Pinia 前端，服务于工作空间、目录浏览、分片上传、版本下载和审计查询。

## 运行

```bash
npm install
npm run dev
```

默认开发端口是 `39000`，应用基址为 `/fileshare/`。如果端口被占用，Vite 会自动选择下一个可用端口。开发代理将 `/api/fileshare/v1` 转发到 `http://localhost:29000`。

## 构建

```bash
npm run build
```

生产产物输出到 `dist/`，由 Nginx 以 `/fileshare/` 提供。后端 API 统一使用 `/api/fileshare/v1/` 前缀；Axios 客户端会将非认证请求自动放到 `/management` 命名空间。

## 前端约定

- 所有请求通过 `src/utils/request.js`，成功响应读取 `data`，分页列表读取 `data.list`、`data.total`、`data.page` 和 `data.page_size`。
- 表单通过后端统一校验，按钮权限只负责改善界面体验，服务端仍然是最终安全边界。
- 列表页面复用 `src/composables/usePagination.js`，表格分页使用 Element Plus 的统一分页控件。
- 会话使用 HttpOnly Cookie，前端不读取或持久化 JWT。
- 文件上传使用初始化、分片、状态、完成四步协议，页面支持暂停、继续和取消当前上传。
