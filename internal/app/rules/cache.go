package rules

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/tidwall/jsonc"
)

func ParseRemoteRuleContent(entry CacheEntry) ([]string, error) {
	if len(entry.Body) > MaxContentSize {
		return nil, fmt.Errorf("rule content exceeds 8 MB")
	}
	contentType := strings.ToLower(strings.TrimSpace(strings.Split(entry.ContentType, ";")[0]))
	prefix := bytes.TrimSpace(entry.Body)
	if len(prefix) > 32 {
		prefix = prefix[:32]
	}
	prefix = bytes.ToLower(prefix)
	if contentType == "text/html" || bytes.HasPrefix(prefix, []byte("<!doctype html")) || bytes.HasPrefix(prefix, []byte("<html")) {
		return nil, fmt.Errorf("received HTML instead of rules")
	}
	if strings.HasSuffix(contentType, "json") {
		var content []string
		if err := json.Unmarshal(jsonc.ToJSON(entry.Body), &content); err != nil {
			return nil, err
		}
		if content == nil {
			return nil, fmt.Errorf("expected a JSON array")
		}
		return content, nil
	}
	return strings.Split(string(entry.Body), "\n"), nil
}

func CacheFilename(directory string, source Source) string {
	return filepath.Join(directory, fmt.Sprintf("%x.json", sha256.Sum256([]byte(fmt.Sprintf("%t\n%s", source.IP, source.ID)))))
}

func ReadCache(directory string, source Source) (CacheEntry, error) {
	var entry CacheEntry
	file, err := os.Open(CacheFilename(directory, source))
	if err != nil {
		return entry, err
	}
	defer file.Close()
	// JSON 中的 Body 使用 base64; 元数据额外预留 64 KB.
	limit := int64((MaxContentSize+2)/3*4 + 65536)
	encoded, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return entry, err
	}
	if int64(len(encoded)) > limit {
		return entry, fmt.Errorf("rule cache exceeds size limit")
	}
	if err := json.Unmarshal(encoded, &entry); err != nil {
		return entry, err
	}
	if entry.Version != 1 || entry.URL != source.ID || entry.IP != source.IP || entry.Body == nil || entry.UpdatedAt <= 0 {
		return entry, fmt.Errorf("invalid rule cache metadata")
	}
	return entry, nil
}

func WriteCache(directory string, source Source, entry CacheEntry) error {
	if directory == "" {
		return nil
	}
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return err
	}
	file, err := os.CreateTemp(directory, ".rules-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	defer file.Close()
	if err := json.NewEncoder(file).Encode(entry); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), CacheFilename(directory, source))
}
