package tools

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/soasurs/adk/tool"
)

func TestAskQuestionsReturnsFrontendAnswers(t *testing.T) {
	workspace := t.TempDir()
	var received QuestionRequest
	questioner := questionerFunc(func(_ context.Context, request QuestionRequest) (QuestionResolution, error) {
		received = request
		return QuestionResolution{Answers: []QuestionAnswer{{
			QuestionID:        "database",
			SelectedOptionIDs: []string{"sqlite"},
		}}}, nil
	})
	tools, err := NewReadOnly(Config{Workdir: workspace, Questioner: questioner})
	if err != nil {
		t.Fatalf("NewReadOnly() error = %v", err)
	}
	candidate := toolByName(t, tools, "ask_questions")
	input := AskQuestionsInput{Questions: []Question{{
		ID:     "database",
		Header: "Storage",
		Prompt: "Which database should be used?",
		Options: []QuestionOption{
			{ID: "sqlite", Label: "SQLite"},
			{ID: "postgres", Label: "PostgreSQL"},
		},
	}}}
	arguments, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	result, err := candidate.Run(t.Context(), tool.Call{ID: "call-1", Name: "ask_questions", Arguments: arguments})
	if err != nil {
		t.Fatalf("ask_questions.Run() error = %v", err)
	}
	var output AskQuestionsOutput
	if err := json.Unmarshal(result.StructuredContent, &output); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if received.ToolCallID != "call-1" || len(received.Questions) != 1 ||
		len(output.Answers) != 1 || output.Answers[0].SelectedOptionIDs[0] != "sqlite" {
		t.Fatalf("request = %+v, output = %+v", received, output)
	}
	// The tool owns independent copies on both sides of the Questioner boundary.
	received.Questions[0].Options[0].Label = "changed"
	output.Answers[0].SelectedOptionIDs[0] = "changed"
	if input.Questions[0].Options[0].Label != "SQLite" || string(result.StructuredContent) == "" {
		t.Fatal("ask_questions did not preserve independent values")
	}
}

func TestAskQuestionsHandlesCancellationAndUnavailableFrontend(t *testing.T) {
	workspace := t.TempDir()
	input := AskQuestionsInput{Questions: []Question{{
		ID:            "name",
		Prompt:        "What name should be used?",
		AllowFreeform: true,
	}}}
	tools, err := NewReadOnly(Config{Workdir: workspace})
	if err != nil {
		t.Fatalf("NewReadOnly() error = %v", err)
	}
	callToolError(t, toolByName(t, tools, "ask_questions"), input)

	tools, err = NewReadOnly(Config{
		Workdir: workspace,
		Questioner: questionerFunc(func(context.Context, QuestionRequest) (QuestionResolution, error) {
			return QuestionResolution{Canceled: true}, nil
		}),
	})
	if err != nil {
		t.Fatalf("NewReadOnly(canceled) error = %v", err)
	}
	callToolError(t, toolByName(t, tools, "ask_questions"), input)
}

func TestAskQuestionsValidatesInputAndResolution(t *testing.T) {
	valid := Question{
		ID:     "choice",
		Prompt: "Choose one",
		Options: []QuestionOption{
			{ID: "a", Label: "A"},
			{ID: "b", Label: "B"},
		},
	}
	invalidQuestions := [][]Question{
		nil,
		{{ID: "", Prompt: "Missing ID", AllowFreeform: true}},
		{{ID: " q ", Prompt: "Whitespace ID", AllowFreeform: true}},
		{{ID: "q", Prompt: "", AllowFreeform: true}},
		{{ID: "q", Prompt: "No input"}},
		{{ID: "q", Prompt: "One option", Options: []QuestionOption{{ID: "a", Label: "A"}}}},
		{{ID: "q", Prompt: "Duplicate", Options: []QuestionOption{{ID: "a", Label: "A"}, {ID: "a", Label: "Again"}}}},
		{{ID: "q", Prompt: "Whitespace option", Options: []QuestionOption{{ID: " a ", Label: "A"}, {ID: "b", Label: "B"}}}},
		{{ID: "q", Prompt: "Empty label", Options: []QuestionOption{{ID: "a", Label: ""}, {ID: "b", Label: "B"}}}},
		{valid, valid},
		{valid, valid, valid, valid},
	}
	for index, questions := range invalidQuestions {
		if err := ValidateQuestions(questions); err == nil {
			t.Fatalf("ValidateQuestions(case %d) error = nil", index)
		}
	}
	if err := ValidateQuestions([]Question{valid}); err != nil {
		t.Fatalf("ValidateQuestions(valid) error = %v", err)
	}

	invalidAnswers := [][]QuestionAnswer{
		nil,
		{{QuestionID: "missing", SelectedOptionIDs: []string{"a"}}},
		{{QuestionID: "choice", SelectedOptionIDs: []string{"missing"}}},
		{{QuestionID: "choice", SelectedOptionIDs: []string{"a", "b"}}},
		{{QuestionID: "choice", Freeform: "custom"}},
		{{QuestionID: "choice"}},
	}
	for index, answers := range invalidAnswers {
		if err := ValidateQuestionAnswers([]Question{valid}, answers); err == nil {
			t.Fatalf("ValidateQuestionAnswers(case %d) error = nil", index)
		}
	}
	if err := ValidateQuestionAnswers([]Question{valid}, []QuestionAnswer{{QuestionID: "choice", SelectedOptionIDs: []string{"a"}}}); err != nil {
		t.Fatalf("ValidateQuestionAnswers(valid) error = %v", err)
	}
}

func TestAskQuestionsPropagatesQuestionerAndContractErrors(t *testing.T) {
	workspace := t.TempDir()
	backendErr := errors.New("stream closed")
	tools, err := NewReadOnly(Config{
		Workdir: workspace,
		Questioner: questionerFunc(func(context.Context, QuestionRequest) (QuestionResolution, error) {
			return QuestionResolution{}, backendErr
		}),
	})
	if err != nil {
		t.Fatalf("NewReadOnly() error = %v", err)
	}
	input := AskQuestionsInput{Questions: []Question{{ID: "name", Prompt: "Name?", AllowFreeform: true}}}
	arguments, _ := json.Marshal(input)
	_, err = toolByName(t, tools, "ask_questions").Run(t.Context(), tool.Call{ID: "call-1", Arguments: arguments})
	if !errors.Is(err, backendErr) {
		t.Fatalf("ask_questions backend error = %v, want %v", err, backendErr)
	}

	tools, err = NewReadOnly(Config{
		Workdir: workspace,
		Questioner: questionerFunc(func(context.Context, QuestionRequest) (QuestionResolution, error) {
			return QuestionResolution{Answers: []QuestionAnswer{{QuestionID: "name"}}}, nil
		}),
	})
	if err != nil {
		t.Fatalf("NewReadOnly(invalid resolution) error = %v", err)
	}
	_, err = toolByName(t, tools, "ask_questions").Run(t.Context(), tool.Call{ID: "call-1", Arguments: arguments})
	if err == nil {
		t.Fatal("ask_questions invalid resolution error = nil")
	}
	var handled *tool.HandledError
	if errors.As(err, &handled) {
		t.Fatalf("ask_questions invalid resolution = handled error %v, want terminal error", err)
	}
}

type questionerFunc func(context.Context, QuestionRequest) (QuestionResolution, error)

func (f questionerFunc) Ask(ctx context.Context, request QuestionRequest) (QuestionResolution, error) {
	return f(ctx, request)
}
