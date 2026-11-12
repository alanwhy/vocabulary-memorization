package training

import (
	"testing"
	"vocab-backend/internal/model"
)

// sampleQuestion provides a coherent fixture; tests mutate semantic-contract boundaries.
func sampleQuestion() model.TrainingQuestion {
	return model.TrainingQuestion{Kind: "paraphrase_choice", Stem: "The report explains why sales fell.", Options: []model.TrainingOption{{ID: "o1", Text: "The report accounts for the decline in sales."}, {ID: "o2", Text: "The report causes sales to fall."}, {ID: "o3", Text: "Sales will rise."}, {ID: "o4", Text: "Sales stayed unchanged."}}, CorrectOptionID: "o1", Mappings: []model.TrainingMapping{{Original: "explains", Replacement: "accounts for", Note: "解释原因"}}, Explanations: []model.TrainingExplanation{{ID: "o1", Text: "保留解释关系"}, {ID: "o2", Text: "解释改成造成"}, {ID: "o3", Text: "下降改成上涨"}, {ID: "o4", Text: "下降改成不变"}}}
}

// TestValidationRejectsBrokenEvidence prevents incorrect answers and invented mappings entering the bank.
func TestValidationRejectsBrokenEvidence(t *testing.T) {
	if err := ValidateQuestion(sampleQuestion(), "basic"); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*model.TrainingQuestion){func(q *model.TrainingQuestion) { q.CorrectOptionID = "missing" }, func(q *model.TrainingQuestion) { q.Options[1].Text = q.Options[0].Text }, func(q *model.TrainingQuestion) { q.Mappings[0].Original = "invented evidence" }, func(q *model.TrainingQuestion) { q.Mappings[0].Replacement = "not in answer" }, func(q *model.TrainingQuestion) { q.Explanations = q.Explanations[:3] }} {
		q := sampleQuestion()
		change(&q)
		if ValidateQuestion(q, "basic") == nil {
			t.Fatal("invalid question accepted")
		}
	}
}

// TestReviewRequiresOneMatchingAnswer rejects multiple plausible options and reviewer disagreements.
func TestReviewRequiresOneMatchingAnswer(t *testing.T) {
	for _, review := range []model.TrainingReview{{ValidOptionIDs: []string{"o1", "o2"}}, {ValidOptionIDs: []string{"o2"}}, {ValidOptionIDs: []string{"o1"}, Issues: []string{"meaning changed"}}} {
		if AcceptReview(sampleQuestion(), review) == nil {
			t.Fatal("invalid review accepted")
		}
	}
	if err := AcceptReview(sampleQuestion(), model.TrainingReview{ValidOptionIDs: []string{"o1"}, Issues: []string{}}); err != nil {
		t.Fatal(err)
	}
}
