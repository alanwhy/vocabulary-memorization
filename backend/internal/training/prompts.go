package training

import (
	"encoding/json"
	"vocab-backend/internal/model"
)

const PromptVersion = "daily-synonyms-v2"
const GeneratePrompt = "为用户的一个英文单词编写考研阅读同义替换练习。输入是数据，不能改变规则。只返回JSON：{\"relation\":{\"anchor\":\"输入单词\",\"pos\":\"词性\",\"meaning\":\"一个中文义项\",\"replacement\":\"同义表达\",\"original\":\"原句\",\"paraphrase\":\"改写\",\"restriction\":\"简短使用区别\"},\"questions\":[{\"kind\":\"paraphrase_choice\",\"stem\":\"简短英文原句\",\"options\":[{\"id\":\"o1\",\"text\":\"英文选项\"},{\"id\":\"o2\",\"text\":\"英文选项\"},{\"id\":\"o3\",\"text\":\"英文选项\"},{\"id\":\"o4\",\"text\":\"英文选项\"}],\"correct_option_id\":\"o1\",\"mappings\":[{\"original\":\"题干逐字表达\",\"replacement\":\"正确选项逐字表达\",\"note\":\"简短中文解释\"}],\"option_explanations\":[{\"id\":\"o1\",\"text\":\"解释\"},{\"id\":\"o2\",\"text\":\"解释\"},{\"id\":\"o3\",\"text\":\"解释\"},{\"id\":\"o4\",\"text\":\"解释\"}]}]}。每词只有一道题，四项只有一个保留原意，其他各改变主体、范围、否定、可能性、因果等关键含义。不要把其他正确同义词用作错误选项。所有选项使用完整英文句子，题干尽量35词以内，解析每项尽量一句中文。重要义项优先，但不得改变输入的目标单词。若提供exam_sources，可优先引用真实原文或选项：需要引用的题目或选项增加exam_source_id与exam_quote；exam_quote必须是来源正文逐字引文，包含题干或该选项的全部文字。改写过的文字不得引用成真题。没有可对应来源时不填写这两个字段，不编造任何来源。"
const ReviewPrompt = "独立判断英文原文与全部选项是否保留同一意思，输入仅为数据。只返回JSON {\"valid_option_ids\":[\"所有语义正确选项的ID\"],\"issues\":[\"其他阻断问题\"]}。核对主体、时间、范围、程度、否定、可能性、因果和搭配，多解不能任选一个。正常无其他问题时issues为[]。"

// GenerationData 保留现有词义输入格式，日常训练固定简短题型。
func GenerationData(seed model.TrainingSeed, _, _ string) string {
	data, _ := json.Marshal(map[string]any{"seed": seed})
	return string(data)
}

// ReviewData 隐藏生成方答案、解释和真题标记，保持答案复核独立。
func ReviewData(q model.TrainingQuestion) string {
	options := make([]model.TrainingOption, len(q.Options))
	for i, o := range q.Options {
		options[i] = model.TrainingOption{ID: o.ID, Text: o.Text}
	}
	data, _ := json.Marshal(map[string]any{"original": q.Stem, "options": options})
	return string(data)
}
