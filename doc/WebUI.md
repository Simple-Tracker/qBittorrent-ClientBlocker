# WebUI

WebUI 是 qBittorrent-ClientBlocker 内置的只读监控页面, 并通过 RESTful API 获取运行状态/封禁记录/内存日志.

## 启用

在 `config.json` 中设置:

```JSON
{
	"webUI": true,
	"webUIListen": "127.0.0.1:7222",
	"webUIUsername": "",
	"webUIPassword": ""
}
```

当 `webUIUsername` 非空时, 页面及 API 使用 HTTP Basic Auth. 用户名为空时不启用认证, `webUIPassword` 会被忽略.

## API

API 当前仅支持 `GET`, 响应为 JSON, 并携带 `Cache-Control: no-store`. 示例中的时间为 Unix 秒, 流量单位为字节.

### 运行状态

请求:

```HTTP
GET /api/v1/status
```

响应示例:

```JSON
{
	"client_state": "reachable",
	"last_client_success": 1789257600,
	"last_client_failure": 0,
	"scan_state": "succeeded",
	"last_scan_started": 1789257600,
	"last_scan_success": 1789257601,
	"submission_state": "retrying",
	"last_submission_success": 1789257500,
	"last_submission_failure": 1789257601,
	"pending": true,
	"pending_ips": 12,
	"next_retry": 1789257605
}
```

- `client_state`: 最近一次下载客户端请求结果
- `scan_state`: 完整扫描状态
- `submission_state`: 名单提交状态. 时间为 0 表示尚无记录.
- `pending_ips`: 待同步 IP 数.

### 封禁列表

请求:

```HTTP
GET /api/v1/bans?query=192.0.2&module=CheckPeer&sort=-timestamp&limit=50
```

支持以下参数:

- `query`: 搜索 IP, 端口, 模块, 原因, 客户端名称和 Peer ID; 不区分大小写, 最多 256 字节.
- `module`, `reason`: 精确匹配, 最多 256 字节.
- `from`, `to`: 按封禁记录时间筛选, 使用 Unix 秒并包含边界; 0 表示不限制.
- `sort`: 可选 `timestamp`, `-timestamp`, `ip`, `-ip`, `uploaded`, `-uploaded`; `-` 表示降序.
- `limit`: 每页数量, 范围 1 至 200, 默认 50.
- `cursor`: 上一页响应中的 `next_cursor`. 使用游标时, 其他参数必须保持不变.

响应示例:

```JSON
{
	"items": [{
		"ip": "192.0.2.1",
		"timestamp": 1789257600,
		"module": "CheckPeer",
		"reason": "Bad-Client_Normal",
		"ports": ["6881"],
		"id": "-EXAMPLE-",
		"client": "ExampleClient",
		"downloaded": 1024,
		"uploaded": 2048
	}],
	"total": 125,
	"filtered_total": 12,
	"offset": 0,
	"next_cursor": "OPAQUE_CURSOR",
	"snapshot_at": 1789257600,
	"modules": ["CheckPeer"],
	"reasons": ["Bad-Client_Normal"]
}
```

封禁列表最多缓存 30 秒, 翻页时使用同一份数据. `next_cursor` 为空为最后一页. 缓存过期时返回 `409 snapshot_expired`, 客户端应丢弃游标并从第一页重新请求.

### Peer 详情

请求:

```HTTP
GET /api/v1/bans/192.0.2.1
```

IPv6 地址需要进行 URL 编码:

```HTTP
GET /api/v1/bans/2001%3Adb8%3A%3A1
```

响应示例:

```JSON
{
	"ip": "192.0.2.1",
	"timestamp": 1789257600,
	"module": "CheckPeer",
	"reason": "Bad-Client_Normal",
	"ports": ["6881"],
	"id": "-EXAMPLE-",
	"client": "ExampleClient",
	"downloaded": 1024,
	"uploaded": 2048
}
```

Peer 详情返回最新数据, 不使用列表缓存. 记录不存在时返回 404; IP 格式无效时返回 400.

### 结构化日志

首次请求:

```HTTP
GET /api/v1/logs?level=error&limit=100
```

后续请求将上一响应的 `next_cursor` 作为 `after` 原样传回:

```HTTP
GET /api/v1/logs?level=error&limit=100&after=OPAQUE_CURSOR
```

支持以下参数:

- `after`: 上一响应中的 `next_cursor`.
- `level`: 可选 `info`, `error`, `debug`; 为空时返回全部级别.
- `module`: 模块名精确匹配, 最多 256 字节.
- `limit`: 每次返回数量, 范围 1 至 200, 默认 100.

响应示例:

```JSON
{
	"items": [{
		"id": "42",
		"timestamp": 1789257600,
		"level": "error",
		"module": "FetchTorrents",
		"message": "请求时发生了错误",
		"truncated": false
	}],
	"next_cursor": "OPAQUE_CURSOR",
	"reset": true,
	"has_more": false
}
```

日志最多保留 1000 条, 每条消息最多 4 KB. `reset=true` 时, 客户端应先清空本地日志视图, 再追加 `items`. 即使 `items` 为空, 也应保存新的 `next_cursor`. `has_more=true` 时可继续请求下一页. 程序重启, 筛选条件变化或请求的日志已被清理时会触发重置.

## 错误响应

错误响应示例:

```JSON
{
	"error": {
		"code": "invalid_query",
		"message": "Bad Request"
	}
}
```

| HTTP 状态 | code | 含义 |
| --- | --- | --- |
| 400 | `invalid_query` | 查询参数, 范围, 排序或 `limit` 无效 |
| 400 | `invalid_cursor` | 游标格式错误或与当前查询条件不匹配 |
| 400 | `invalid_ip` | Peer 详情路径中的 IP 无效 |
| 401 | `unauthorized` | Basic Auth 缺失或错误 |
| 404 | `not_found` | API 路径或 Peer 记录不存在 |
| 405 | `method_not_allowed` | 请求方法不是 GET |
| 409 | `snapshot_expired` | 封禁列表缓存已过期, 应从第一页重新请求 |
