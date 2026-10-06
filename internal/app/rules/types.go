package rules

const MaxContentSize = 8 * 1024 * 1024

type Source struct {
	IP   bool
	Kind string
	ID   string
}

// 内容和 HTTP 校验器放在同一个文件, 防止 304 对应的规则内容丢失.
type CacheEntry struct {
	Version      int    `json:"version"`
	URL          string `json:"url"`
	IP           bool   `json:"ip"`
	ContentType  string `json:"contentType"`
	Body         []byte `json:"body"`
	ETag         string `json:"etag"`
	LastModified string `json:"lastModified"`
	UpdatedAt    int64  `json:"updatedAt"`
}
