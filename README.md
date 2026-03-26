# Cofood

Cofood 是一个基于 Go + Gin 的食物营养搜索服务，内置 SQLite 存储、关键词检索、向量检索，以及一个可直接访问的轻量 Web 界面。

项目启动后会自动导入 `food-table.jsonl` 中的食物数据；如果配置了 SiliconFlow API Key，还可以在启动阶段批量生成 embedding，支持语义搜索。

![Cofood screenshot](docs/image.png)

## Features

- 自动导入 `food-table.jsonl` 到 SQLite，并结构化保存食物、别名和营养项
- 支持名称精确匹配和关键词搜索
- 支持基于 embedding 的语义搜索
- 首页直接提供单页查询界面，无需额外前端构建
- 内置多时间窗限流，保护公开接口

## Tech Stack

- Go
- Gin
- SQLite
- SiliconFlow Embeddings
- 原生 HTML / CSS / JavaScript

## Quick Start

1. 复制环境变量模板

```bash
cp .env.example .env
```

2. 按需编辑 `.env`

- 只体验关键词搜索时，可直接使用默认配置
- 需要语义搜索时，设置 `SILICONFLOW_API_KEY`
- 需要启动时自动补全 embedding 时，设置 `AUTO_EMBED_ON_STARTUP=true`

3. 启动服务

```bash
go run .
```

4. 打开浏览器访问

```text
http://localhost:8080
```

## Configuration

主要环境变量如下：

| Name | Default | Description |
| --- | --- | --- |
| `APP_HOST` | `0.0.0.0` | 服务监听地址 |
| `APP_PORT` | `8080` | 服务端口 |
| `DATABASE_PATH` | `data/cofood.db` | SQLite 数据文件路径 |
| `DATA_FILE_PATH` | `food-table.jsonl` | 食物数据源 |
| `SILICONFLOW_API_KEY` | empty | SiliconFlow API Key |
| `EMBEDDING_MODEL` | `Qwen/Qwen3-Embedding-8B` | embedding 模型 |
| `AUTO_EMBED_ON_STARTUP` | `false` | 启动时是否自动补全 embedding |
| `EMBEDDING_BATCH_SIZE` | `16` | embedding 批处理大小 |

完整示例见 [`.env.example`](.env.example)。

## API

### Health Check

```http
GET /healthz
```

### Name Search

```http
GET /api/v1/search/name?q=葡萄酒
```

### Vector Search

```http
GET /api/v1/search/vector?q=适合做沙拉的鱼
```

当未配置 embedding 或尚未加载向量索引时，向量接口仍会返回精确匹配结果，并在响应中标记 `embedding_loaded=false`。

## Rate Limit

同一 IP 的默认限制：

- 1 秒最多 20 次请求
- 1 分钟最多 120 次请求
- 1 小时最多 1200 次请求

任一窗口超限都会返回 `429 Too Many Requests`。

## Project Structure

```text
.
|-- internal/
|   |-- api/          HTTP 路由与处理器
|   |-- database/     SQLite 访问与 schema
|   |-- embedding/    SiliconFlow embedding 客户端
|   |-- importer/     JSONL 导入与 embedding 回填
|   |-- search/       搜索服务
|   `-- vector/       内存向量索引
|-- web/              单页查询界面
|-- docs/             README 资源
`-- food-table.jsonl  示例数据
```
