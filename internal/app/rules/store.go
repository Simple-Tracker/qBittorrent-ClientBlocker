package rules

import (
	"fmt"
	"sync"
	"time"

	"github.com/dlclark/regexp2"
)

// Services 只提供规则编译需要的日志和文案.
type Services struct {
	Log      func(string, string, bool, ...any)
	LogError func(string, string, bool, ...any)
	Text     func(string) string
}

// Store 管理编译后的规则和各来源的所有权. 扫描器可并发读取编译结果;
// 更新来源应通过 SetRuleContent 或 PublishRuleSource 完成.
type Store struct {
	BlockList   sync.Map
	IPBlockList sync.Map
	mutex       sync.Mutex
	sources     map[Source]map[string]interface{}
	services    Services
}

func NewStore(services Services) *Store {
	if services.Log == nil {
		services.Log = func(string, string, bool, ...any) {}
	}
	if services.LogError == nil {
		services.LogError = func(string, string, bool, ...any) {}
	}
	if services.Text == nil {
		services.Text = func(key string) string { return key }
	}
	return &Store{sources: make(map[Source]map[string]interface{}), services: services}
}

// Reset 在配置重载期间重建来源, 不替换正在被扫描器读取的 sync.Map.
func (s *Store) Reset() {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.sources = make(map[Source]map[string]interface{})
	for _, compiled := range []*sync.Map{&s.BlockList, &s.IPBlockList} {
		compiled.Range(func(key, value any) bool { compiled.Delete(key); return true })
	}
}

func (s *Store) CompileRuleContent(content []string, source Source, strict bool) (map[string]interface{}, error) {
	rules := make(map[string]interface{})
	compiled := &s.BlockList
	module := "SetBlockListFromContent_Compile"
	if source.IP {
		compiled, module = &s.IPBlockList, "SetIPBlockListFromContent_Compile"
	}
	for index, line := range content {
		line = StrTrim(ProcessRemark(line))
		if line == "" {
			s.services.LogError("Debug-"+module, s.services.Text("Error-Debug-EmptyLineWithSource"), false, index, source.ID)
			continue
		}
		if _, exists := rules[line]; exists {
			continue
		}
		if existing, ok := compiled.Load(line); ok {
			rules[line] = existing
			continue
		}
		s.services.Log("Debug-"+module, ":%d %s (Source: %s)", false, index, line, source.ID)
		var value interface{}
		var err error
		if source.IP {
			network := ParseIPCIDR(line)
			if network == nil {
				err = fmt.Errorf("invalid IP or CIDR")
			} else {
				value = network
			}
		} else {
			var reg *regexp2.Regexp
			reg, err = regexp2.Compile("(?i)"+line, 0)
			if err == nil {
				reg.MatchTimeout = 50 * time.Millisecond
				value = reg
			}
		}
		if err != nil {
			s.services.LogError(module, s.services.Text("Error-"+module), true, index, line, source.ID)
			if strict {
				return nil, fmt.Errorf("invalid rule at line %d: %w", index+1, err)
			}
			continue
		}
		rules[line] = value
	}
	return rules, nil
}

func (s *Store) PublishRuleSource(source Source, rules map[string]interface{}) int {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	compiled := &s.BlockList
	if source.IP {
		compiled = &s.IPBlockList
	}
	previous := s.sources[source]
	s.sources[source] = rules
	count := 0
	for key, value := range rules {
		if _, loaded := compiled.LoadOrStore(key, value); !loaded {
			count++
		}
	}
	for key := range previous {
		if _, exists := rules[key]; exists {
			continue
		}
		retained := false
		for other, contents := range s.sources {
			if other.IP == source.IP {
				if _, exists := contents[key]; exists {
					retained = true
					break
				}
			}
		}
		if !retained {
			compiled.Delete(key)
		}
	}
	return count
}

func (s *Store) SetRuleContent(content []string, source Source) int {
	rules, _ := s.CompileRuleContent(content, source, false)
	return s.PublishRuleSource(source, rules)
}
