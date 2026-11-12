// 同义替换内容只用于当日内存练习，不对应数据库实体。
package model

type TrainingCitation struct {
	Title string `json:"title"`
	URL   string `json:"url"`
	Year  string `json:"year"`
	Quote string `json:"quote"`
}
type TrainingOption struct {
	ID           string            `json:"id"`
	Text         string            `json:"text"`
	ExamSourceID string            `json:"exam_source_id,omitempty"`
	ExamQuote    string            `json:"exam_quote,omitempty"`
	Exam         *TrainingCitation `json:"exam,omitempty"`
}
type TrainingMapping struct {
	Original    string `json:"original"`
	Replacement string `json:"replacement"`
	Note        string `json:"note"`
}
type TrainingExplanation struct {
	ID       string `json:"id"`
	Text     string `json:"text"`
	Category string `json:"category,omitempty"`
}
type TrainingQuestion struct {
	Kind            string                `json:"kind"`
	Stem            string                `json:"stem"`
	Options         []TrainingOption      `json:"options"`
	CorrectOptionID string                `json:"correct_option_id"`
	Mappings        []TrainingMapping     `json:"mappings"`
	Explanations    []TrainingExplanation `json:"option_explanations"`
	ExamSourceID    string                `json:"exam_source_id,omitempty"`
	ExamQuote       string                `json:"exam_quote,omitempty"`
	Exam            *TrainingCitation     `json:"exam,omitempty"`
}
type TrainingContent struct {
	Anchor      string `json:"anchor"`
	Pos         string `json:"pos"`
	Meaning     string `json:"meaning"`
	Replacement string `json:"replacement"`
	Original    string `json:"original"`
	Paraphrase  string `json:"paraphrase"`
	Restriction string `json:"restriction"`
}
type TrainingBundle struct {
	SearchStatus string             `json:"search_status,omitempty"`
	Relation     TrainingContent    `json:"relation"`
	Questions    []TrainingQuestion `json:"questions"`
}
type TrainingReview struct {
	ValidOptionIDs []string `json:"valid_option_ids"`
	Issues         []string `json:"issues"`
}
type TrainingSeed struct {
	Word             string   `json:"word"`
	Senses           []Sense  `json:"senses,omitempty"`
	ImportantGlosses []string `json:"important_glosses,omitempty"`
}
