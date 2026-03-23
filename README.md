# Cofood API

一个基于 Go + Gin 的轻量食物搜索 API 服务。

## 功能

- 流式读取 `food-table.jsonl`，将食物条目导入 SQLite
- 结构化保存食物主信息、别名、营养项
- 支持 SiliconFlow `Qwen/Qwen3-Embedding-8B` 生成向量并持久化
- 提供名字搜索和向量搜索接口
- 对单 IP 做多时间窗限流

## 运行

1. 复制环境文件

```bash
cp .env.example .env
```

2. 如需向量功能，填写 `.env` 中的 `SILICONFLOW_API_KEY`

3. 启动服务

```bash
go run .
```

## API

### 健康检查

```http
GET /healthz
```

### 名字搜索

```http
GET /api/v1/search/name?q=葡萄酒
```

### 向量搜索

```http
GET /api/v1/search/vector?q=适合做沙拉的鱼
```

## 限流

同一 IP：

- 1 秒最多 20 次
- 1 分钟最多 120 次
- 1 小时最多 1200 次

任一窗口超限都会返回 `429 Too Many Requests`
