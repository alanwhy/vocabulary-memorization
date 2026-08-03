package main

import (
	"embed"
	"encoding/json"
	"log"
	"strings"
)

//go:embed dict/words.json
var dictFS embed.FS

type dictEntry struct {
	Word        string `json:"word"`
	Translation string `json:"translation"`
	Pos         string `json:"pos"`
}

var builtinDict map[string]dictEntry

// loadBuiltinDict 启动时把内置词典加载进内存，作为翻译的第一优先级来源
func loadBuiltinDict() {
	data, err := dictFS.ReadFile("dict/words.json")
	if err != nil {
		log.Printf("加载内置词典失败: %v", err)
		builtinDict = map[string]dictEntry{}
		return
	}
	var entries []dictEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		log.Printf("解析内置词典失败: %v", err)
		builtinDict = map[string]dictEntry{}
		return
	}
	builtinDict = make(map[string]dictEntry, len(entries))
	for _, e := range entries {
		key := strings.ToLower(strings.TrimSpace(e.Word))
		builtinDict[key] = e
	}
	log.Printf("内置词典加载完成，共 %d 个词条", len(builtinDict))
}

func lookupBuiltinDict(wordKey string) (dictEntry, bool) {
	e, ok := builtinDict[wordKey]
	return e, ok
}
