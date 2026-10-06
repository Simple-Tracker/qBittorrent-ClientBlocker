package webui

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"
)

const webUILogCapacity = 1000
const webUILogMessageBytes = 4096

type WebUILogEntry struct {
	ID        string `json:"id"`
	Timestamp int64  `json:"timestamp"`
	Level     string `json:"level"`
	Module    string `json:"module"`
	Message   string `json:"message"`
	Truncated bool   `json:"truncated"`
}
type WebUILogPage struct {
	Items      []WebUILogEntry `json:"items"`
	NextCursor string          `json:"next_cursor"`
	Reset      bool            `json:"reset"`
	HasMore    bool            `json:"has_more"`
}
type webUILogCursor struct {
	Epoch         string
	Sequence      uint64
	Level, Module string
}

var webUILogMutex sync.Mutex
var webUILogEpoch = strconv.FormatInt(now().UnixNano(), 36)
var webUILogSequence uint64
var webUILogEntries [webUILogCapacity]WebUILogEntry

func AppendWebUILog(level, module, message string) {
	truncated := len(message) > webUILogMessageBytes
	if truncated {
		message = message[:webUILogMessageBytes]
		for !utf8.ValidString(message) && len(message) > 0 {
			message = message[:len(message)-1]
		}
	}
	if len(module) > 256 {
		module = module[:256]
	}
	webUILogMutex.Lock()
	defer webUILogMutex.Unlock()
	webUILogSequence++
	webUILogEntries[(webUILogSequence-1)%webUILogCapacity] = WebUILogEntry{ID: strconv.FormatUint(webUILogSequence, 10), Timestamp: now().Unix(), Level: level, Module: module, Message: message, Truncated: truncated}
}
func WebUI_GetStructuredLogs(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	level, module := q.Get("level"), q.Get("module")
	if level != "" && level != "info" && level != "debug" && level != "error" || len(module) > 256 {
		WriteWebUIAPIError(w, 400, "invalid_query")
		return
	}
	limit := 100
	if raw := q.Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 200 {
			WriteWebUIAPIError(w, 400, "invalid_query")
			return
		}
		limit = n
	}
	cursor := webUILogCursor{}
	if raw := q.Get("after"); raw != "" {
		if len(raw) > 2048 {
			WriteWebUIAPIError(w, 400, "invalid_cursor")
			return
		}
		data, err := base64.RawURLEncoding.DecodeString(raw)
		if err != nil || json.Unmarshal(data, &cursor) != nil || cursor.Epoch == "" {
			WriteWebUIAPIError(w, 400, "invalid_cursor")
			return
		}
	}
	webUILogMutex.Lock()
	oldest := uint64(1)
	if webUILogSequence >= webUILogCapacity {
		oldest = webUILogSequence - webUILogCapacity + 1
	}
	reset := cursor.Epoch != webUILogEpoch || cursor.Sequence < oldest-1 || cursor.Sequence > webUILogSequence || cursor.Level != level || cursor.Module != module
	if reset {
		cursor.Sequence = oldest - 1
	}
	page := WebUILogPage{Items: []WebUILogEntry{}, Reset: reset}
	for cursor.Sequence < webUILogSequence && len(page.Items) < limit {
		cursor.Sequence++
		entry := webUILogEntries[(cursor.Sequence-1)%webUILogCapacity]
		if level != "" && entry.Level != level || module != "" && entry.Module != module {
			continue
		}
		page.Items = append(page.Items, entry)
	}
	page.HasMore = cursor.Sequence < webUILogSequence
	cursor.Epoch, cursor.Level, cursor.Module = webUILogEpoch, level, module
	data, _ := json.Marshal(cursor)
	page.NextCursor = base64.RawURLEncoding.EncodeToString(data)
	webUILogMutex.Unlock()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(page)
}

func LogMessageLevel(module string) string {
	if strings.HasPrefix(module, "Debug") {
		return "debug"
	}
	return "info"
}
