package main

import "testing"

// TestParseSensesEnrichment 覆盖解析 6 个强化字段，以及空数组/词级字段的序列化结果
func TestParseSensesEnrichment(t *testing.T) {
	content := `[{"pos":"n.","translation":"苹果；苹果树","phonetic":"/ˈæp(ə)l/","example":"She bit into an apple.","example_translation":"她咬了一口苹果。","root":"","affix":"","synonyms":["pome（梨果）"],"antonyms":[],"lookalikes":["ample（充足的）"]}]`
	senses, err := parseSenses(content)
	if err != nil {
		t.Fatalf("parseSenses 返回错误: %v", err)
	}
	if len(senses) != 1 {
		t.Fatalf("期望 1 条，got %d", len(senses))
	}
	s := senses[0]
	if s.Pos != "n." || s.Translation != "苹果；苹果树" {
		t.Fatalf("pos/translation 解析错误: %+v", s)
	}
	if s.Phonetic != "/ˈæp(ə)l/" {
		t.Fatalf("Phonetic = %q", s.Phonetic)
	}
	if s.Example != "She bit into an apple." {
		t.Fatalf("Example = %q", s.Example)
	}
	if s.ExampleTranslation != "她咬了一口苹果。" {
		t.Fatalf("ExampleTranslation = %q", s.ExampleTranslation)
	}
	if len(s.Synonyms) != 1 || s.Synonyms[0] != "pome（梨果）" {
		t.Fatalf("Synonyms = %v", s.Synonyms)
	}
	if len(s.Antonyms) != 0 {
		t.Fatalf("Antonyms 期望为空，got %v", s.Antonyms)
	}
	if len(s.Lookalikes) != 1 || s.Lookalikes[0] != "ample（充足的）" {
		t.Fatalf("Lookalikes = %v", s.Lookalikes)
	}
}

// TestParseSensesEnrichmentMarkdown 覆盖模型偶发多包一层 markdown 代码块时的容错，
// 以及词根词缀的解析。
func TestParseSensesEnrichmentMarkdown(t *testing.T) {
	content := "```json\n[{\"pos\":\"n.\",\"translation\":\"机缘巧合\",\"phonetic\":\"/ˌserənˈdɪpəti/\",\"example\":\"Finding that book was pure serendipity.\",\"example_translation\":\"找到那本书纯属机缘巧合。\",\"root\":\"serendip\",\"affix\":\"-ity\",\"synonyms\":[\"chance（机会）\"],\"antonyms\":[]}]\n```"
	senses, err := parseSenses(content)
	if err != nil {
		t.Fatalf("parseSenses 返回错误: %v", err)
	}
	if len(senses) != 1 || senses[0].Root != "serendip" || senses[0].Affix != "-ity" {
		t.Fatalf("词根词缀解析错误: %+v", senses)
	}
	if senses[0].ExampleTranslation != "找到那本书纯属机缘巧合。" {
		t.Fatalf("ExampleTranslation = %q", senses[0].ExampleTranslation)
	}
}

// TestParseSensesSkipsMissingEnrichment 覆盖旧 prompt 返回（只有 pos/translation）仍能解析，
// 强化字段回退为空——这是历史数据兼容的关键。
func TestParseSensesSkipsMissingEnrichment(t *testing.T) {
	content := `[{"pos":"n.","translation":"苹果"}]`
	senses, err := parseSenses(content)
	if err != nil {
		t.Fatalf("parseSenses 返回错误: %v", err)
	}
	if len(senses) != 1 {
		t.Fatalf("期望 1 条，got %d", len(senses))
	}
	if senses[0].Phonetic != "" || senses[0].Example != "" {
		t.Fatalf("旧格式应无强化字段，got %+v", senses[0])
	}
}
