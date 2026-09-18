package app

import "vocab-backend/internal/model"

// 这些别名让应用层沿用原有业务命名，同时确保领域数据与规则只有 model 一份实现。
// 同包白盒测试也因此无需为了分包重构而改写全部测试数据构造代码。
type (
	Sense         = model.Sense
	Word          = model.Word
	User          = model.User
	UserWithStats = model.UserWithStats
	WordStats     = model.WordStats

	// 旧白盒测试使用包内名称构造查询结果；别名不复制领域类型定义。
	dictionaryEntry = model.DictionaryEntry
	vocabularyItem  = model.VocabularyItem
)

func mergeSensesByPos(senses []Sense) []Sense {
	return model.MergeSensesByPos(senses)
}

func sensesEnriched(senses []Sense) bool {
	return model.SensesEnriched(senses)
}

func sensesNeedEnrichment(senses []Sense) bool {
	return model.SensesNeedEnrichment(senses)
}

func normalizeGlosses(glosses []string) []string {
	return model.NormalizeGlosses(glosses)
}

func applySRSScheduling(intervalDays int, easeFactor float64, rating string) (int, float64) {
	return model.ApplySRSScheduling(intervalDays, easeFactor, rating)
}
