package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/soasurs/adk/tool"
)

const maxQuestions = 3

var (
	// ErrQuestionInteractionUnavailable indicates that an ask_questions call
	// has no frontend interaction attached to its active Run.
	ErrQuestionInteractionUnavailable = errors.New("question interaction is unavailable")
)

// Questioner publishes one batch of questions and waits for frontend-authored
// answers. Implementations must observe context cancellation and clean up any
// pending prompt.
type Questioner interface {
	Ask(context.Context, QuestionRequest) (QuestionResolution, error)
}

// QuestionRequest is one ask_questions invocation awaiting frontend input.
type QuestionRequest struct {
	ToolCallID string     `json:"tool_call_id"`
	Questions  []Question `json:"questions"`
}

// QuestionResolution is the frontend resolution of a QuestionRequest.
type QuestionResolution struct {
	Answers  []QuestionAnswer `json:"answers,omitempty"`
	Canceled bool             `json:"canceled,omitempty"`
}

// AskQuestionsInput contains one to three questions for the frontend.
type AskQuestionsInput struct {
	Questions []Question `json:"questions" jsonschema:"One to three questions that require user input"`
}

// Question is one frontend input requested by the model.
type Question struct {
	ID            string           `json:"id" jsonschema:"Unique question ID within this tool call"`
	Header        string           `json:"header,omitempty" jsonschema:"Short display header"`
	Prompt        string           `json:"prompt" jsonschema:"Question shown to the user"`
	Options       []QuestionOption `json:"options,omitempty" jsonschema:"Two or three selectable options"`
	Multiple      bool             `json:"multiple,omitempty" jsonschema:"Allow selecting multiple options"`
	AllowFreeform bool             `json:"allow_freeform,omitempty" jsonschema:"Allow a free-form answer"`
}

// QuestionOption is one frontend-selectable answer.
type QuestionOption struct {
	ID          string `json:"id" jsonschema:"Unique option ID within its question"`
	Label       string `json:"label" jsonschema:"Short option label"`
	Description string `json:"description,omitempty" jsonschema:"Explanation of the option's impact"`
}

// QuestionAnswer is one frontend-authored answer returned to the model.
type QuestionAnswer struct {
	QuestionID        string   `json:"question_id"`
	SelectedOptionIDs []string `json:"selected_option_ids,omitempty"`
	Freeform          string   `json:"freeform,omitempty"`
}

// AskQuestionsOutput is the successful structured result of ask_questions.
type AskQuestionsOutput struct {
	Answers []QuestionAnswer `json:"answers"`
}

type askQuestionsTool struct {
	definition tool.Definition
	questioner Questioner
}

func (s service) newAskQuestionsTool() (tool.Tool, error) {
	inputSchema, err := jsonschema.ForType(reflect.TypeFor[AskQuestionsInput](), &jsonschema.ForOptions{})
	if err != nil {
		return nil, fmt.Errorf("ask_questions: build input schema: %w", err)
	}
	outputSchema, err := jsonschema.ForType(reflect.TypeFor[AskQuestionsOutput](), &jsonschema.ForOptions{})
	if err != nil {
		return nil, fmt.Errorf("ask_questions: build output schema: %w", err)
	}
	return &askQuestionsTool{
		definition: tool.Definition{
			Name: "ask_questions",
			Description: "Ask the user one to three focused questions and wait for frontend-authored answers. " +
				"Use it when a plan or implementation depends on a user decision. Call it alone when later tool calls depend on the answers.",
			InputSchema:  inputSchema,
			OutputSchema: outputSchema,
		},
		questioner: s.questioner,
	}, nil
}

func (t *askQuestionsTool) Definition() tool.Definition {
	return t.definition
}

func (t *askQuestionsTool) Run(ctx context.Context, call tool.Call) (*tool.Result, error) {
	var input AskQuestionsInput
	if err := json.Unmarshal(call.Arguments, &input); err != nil {
		return nil, tool.NewHandledError(fmt.Sprintf("error: parse arguments: %s", err))
	}
	if err := validateQuestions(input.Questions); err != nil {
		return nil, tool.NewHandledError(err.Error())
	}
	if t.questioner == nil {
		return nil, tool.NewHandledError("question interaction is unavailable")
	}
	resolution, err := t.questioner.Ask(ctx, QuestionRequest{
		ToolCallID: call.ID,
		Questions:  cloneQuestions(input.Questions),
	})
	if err != nil {
		if errors.Is(err, ErrQuestionInteractionUnavailable) {
			return nil, tool.NewHandledError("question interaction is unavailable")
		}
		return nil, fmt.Errorf("ask questions: %w", err)
	}
	if resolution.Canceled {
		return nil, tool.NewHandledError("user canceled the question prompt")
	}
	if err := validateAnswers(input.Questions, resolution.Answers); err != nil {
		return nil, fmt.Errorf("ask questions: invalid resolution: %w", err)
	}
	output := AskQuestionsOutput{Answers: orderAnswers(input.Questions, resolution.Answers)}
	raw, err := json.Marshal(output)
	if err != nil {
		return nil, fmt.Errorf("ask questions: marshal result: %w", err)
	}
	return &tool.Result{Content: string(raw), StructuredContent: raw}, nil
}

func validateQuestions(questions []Question) error {
	if len(questions) == 0 || len(questions) > maxQuestions {
		return fmt.Errorf("questions must contain between 1 and %d items", maxQuestions)
	}
	questionIDs := make(map[string]struct{}, len(questions))
	for _, question := range questions {
		id := strings.TrimSpace(question.ID)
		if id == "" {
			return errors.New("question ID must not be empty")
		}
		if id != question.ID {
			return fmt.Errorf("question ID %q must not contain surrounding whitespace", question.ID)
		}
		if _, exists := questionIDs[id]; exists {
			return fmt.Errorf("duplicate question ID %q", id)
		}
		questionIDs[id] = struct{}{}
		if strings.TrimSpace(question.Prompt) == "" {
			return fmt.Errorf("question %q prompt must not be empty", id)
		}
		if len(question.Options) == 1 || len(question.Options) > 3 {
			return fmt.Errorf("question %q options must be empty or contain 2 to 3 items", id)
		}
		if len(question.Options) == 0 && !question.AllowFreeform {
			return fmt.Errorf("question %q must provide options or allow free-form input", id)
		}
		if question.Multiple && len(question.Options) < 2 {
			return fmt.Errorf("question %q cannot enable multiple selection without options", id)
		}
		optionIDs := make(map[string]struct{}, len(question.Options))
		for _, option := range question.Options {
			optionID := strings.TrimSpace(option.ID)
			if optionID == "" {
				return fmt.Errorf("question %q option ID must not be empty", id)
			}
			if optionID != option.ID {
				return fmt.Errorf("question %q option ID %q must not contain surrounding whitespace", id, option.ID)
			}
			if _, exists := optionIDs[optionID]; exists {
				return fmt.Errorf("question %q has duplicate option ID %q", id, optionID)
			}
			optionIDs[optionID] = struct{}{}
			if strings.TrimSpace(option.Label) == "" {
				return fmt.Errorf("question %q option %q label must not be empty", id, optionID)
			}
		}
	}
	return nil
}

// ValidateQuestions validates an ask_questions input independently of tool
// execution. Interaction adapters use it before publishing a frontend prompt.
func ValidateQuestions(questions []Question) error {
	return validateQuestions(questions)
}

func validateAnswers(questions []Question, answers []QuestionAnswer) error {
	if len(answers) != len(questions) {
		return errors.New("answers must contain exactly one item for every question")
	}
	byID := make(map[string]Question, len(questions))
	for _, question := range questions {
		byID[question.ID] = question
	}
	answered := make(map[string]struct{}, len(answers))
	for _, answer := range answers {
		question, exists := byID[answer.QuestionID]
		if !exists {
			return fmt.Errorf("answer references unknown question %q", answer.QuestionID)
		}
		if _, exists := answered[answer.QuestionID]; exists {
			return fmt.Errorf("duplicate answer for question %q", answer.QuestionID)
		}
		answered[answer.QuestionID] = struct{}{}
		if !question.Multiple && len(answer.SelectedOptionIDs) > 1 {
			return fmt.Errorf("question %q allows only one selected option", answer.QuestionID)
		}
		validOptions := make(map[string]struct{}, len(question.Options))
		for _, option := range question.Options {
			validOptions[option.ID] = struct{}{}
		}
		selected := make(map[string]struct{}, len(answer.SelectedOptionIDs))
		for _, optionID := range answer.SelectedOptionIDs {
			if _, exists := validOptions[optionID]; !exists {
				return fmt.Errorf("question %q answer selects unknown option %q", answer.QuestionID, optionID)
			}
			if _, exists := selected[optionID]; exists {
				return fmt.Errorf("question %q answer selects option %q more than once", answer.QuestionID, optionID)
			}
			selected[optionID] = struct{}{}
		}
		freeform := strings.TrimSpace(answer.Freeform)
		if freeform != "" && !question.AllowFreeform {
			return fmt.Errorf("question %q does not allow free-form input", answer.QuestionID)
		}
		if len(answer.SelectedOptionIDs) == 0 && freeform == "" {
			return fmt.Errorf("question %q answer must select an option or provide free-form input", answer.QuestionID)
		}
	}
	return nil
}

// ValidateQuestionAnswers validates a completed frontend resolution against
// the questions that were originally published.
func ValidateQuestionAnswers(questions []Question, answers []QuestionAnswer) error {
	return validateAnswers(questions, answers)
}

func cloneQuestions(questions []Question) []Question {
	result := make([]Question, len(questions))
	for index, question := range questions {
		result[index] = question
		result[index].Options = append([]QuestionOption(nil), question.Options...)
	}
	return result
}

func orderAnswers(questions []Question, answers []QuestionAnswer) []QuestionAnswer {
	byID := make(map[string]QuestionAnswer, len(answers))
	for _, answer := range answers {
		byID[answer.QuestionID] = answer
	}
	result := make([]QuestionAnswer, len(questions))
	for index, question := range questions {
		result[index] = byID[question.ID]
		result[index].SelectedOptionIDs = append([]string(nil), result[index].SelectedOptionIDs...)
	}
	return result
}

var _ tool.Tool = (*askQuestionsTool)(nil)
