package server

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	v1 "github.com/soasurs/koda/gen/koda/v1"
	"github.com/soasurs/koda/internal/tools"
)

var (
	// ErrQuestionPromptNotFound indicates that a pending prompt expired, was
	// canceled with its Run, or was already resolved.
	ErrQuestionPromptNotFound = errors.New("question prompt not found")
)

// QuestionBroker coordinates in-process, run-scoped ask_questions prompts.
// Submitted answers are validated before a prompt is removed, allowing a
// frontend to correct an invalid response and retry.
type QuestionBroker struct {
	mu       sync.Mutex
	pending  map[string]*pendingQuestion
	resolved func(string) interactionResolution
}

type pendingQuestion struct {
	prompt     *v1.QuestionPrompt
	resolution chan questionResolution
}

type questionResolution struct {
	answers  *v1.QuestionAnswers
	canceled bool
}

// NewQuestionBroker constructs an empty QuestionBroker.
func NewQuestionBroker() *QuestionBroker {
	return &QuestionBroker{pending: make(map[string]*pendingQuestion)}
}

// Await publishes prompt and blocks until valid answers, explicit frontend
// cancellation, context cancellation, or a publish error.
func (b *QuestionBroker) Await(ctx context.Context, prompt *v1.QuestionPrompt, publish func(*v1.QuestionPrompt) error) (*v1.QuestionAnswers, bool, error) {
	if ctx == nil {
		return nil, false, errors.New("question broker: context must not be nil")
	}
	if prompt == nil {
		return nil, false, errors.New("question broker: prompt must not be nil")
	}
	id := strings.TrimSpace(prompt.GetId())
	if id == "" {
		return nil, false, errors.New("question broker: prompt ID must not be empty")
	}
	if publish == nil {
		return nil, false, errors.New("question broker: publish function must not be nil")
	}
	questions := questionsFromProto(prompt.GetQuestions())
	if err := tools.ValidateQuestions(questions); err != nil {
		return nil, false, fmt.Errorf("question broker: validate prompt: %w", err)
	}
	ownedPrompt := proto.Clone(prompt).(*v1.QuestionPrompt)
	pending := &pendingQuestion{
		prompt:     ownedPrompt,
		resolution: make(chan questionResolution, 1),
	}
	b.mu.Lock()
	if _, exists := b.pending[id]; exists {
		b.mu.Unlock()
		return nil, false, fmt.Errorf("question broker: duplicate prompt ID %q", id)
	}
	b.pending[id] = pending
	b.mu.Unlock()

	if err := publish(proto.Clone(ownedPrompt).(*v1.QuestionPrompt)); err != nil {
		b.removeQuestion(id, pending)
		return nil, false, err
	}
	select {
	case resolution := <-pending.resolution:
		if resolution.answers == nil {
			return nil, resolution.canceled, nil
		}
		return proto.Clone(resolution.answers).(*v1.QuestionAnswers), resolution.canceled, nil
	case <-ctx.Done():
		b.mu.Lock()
		if b.pending[id] == pending {
			delete(b.pending, id)
			b.mu.Unlock()
			return nil, false, ctx.Err()
		}
		b.mu.Unlock()
		resolution := <-pending.resolution
		if resolution.answers == nil {
			return nil, resolution.canceled, nil
		}
		return proto.Clone(resolution.answers).(*v1.QuestionAnswers), resolution.canceled, nil
	}
}

// ResolveAnswers validates and records answers for one pending prompt.
func (b *QuestionBroker) ResolveAnswers(id string, answers *v1.QuestionAnswers) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return errors.New("question prompt ID must not be empty")
	}
	if answers == nil {
		return errors.New("question answers must not be nil")
	}
	b.mu.Lock()
	pending, exists := b.pending[id]
	if !exists {
		b.mu.Unlock()
		return fmt.Errorf("resolve question prompt %q: %w", id, ErrQuestionPromptNotFound)
	}
	if err := tools.ValidateQuestionAnswers(questionsFromProto(pending.prompt.GetQuestions()), answersFromProto(answers.GetAnswers())); err != nil {
		b.mu.Unlock()
		return fmt.Errorf("resolve question prompt %q: %w", id, err)
	}
	if b.resolved != nil {
		resolution := b.resolved(id)
		if resolution.managed && !resolution.accepted {
			b.mu.Unlock()
			return fmt.Errorf("resolve question prompt %q: %w", id, ErrQuestionPromptNotFound)
		}
	}
	pending.resolution <- questionResolution{answers: proto.Clone(answers).(*v1.QuestionAnswers)}
	delete(b.pending, id)
	b.mu.Unlock()
	return nil
}

// ResolveCanceled records an explicit frontend cancellation for one prompt.
func (b *QuestionBroker) ResolveCanceled(id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return errors.New("question prompt ID must not be empty")
	}
	b.mu.Lock()
	pending, exists := b.pending[id]
	if exists {
		if b.resolved != nil {
			resolution := b.resolved(id)
			if resolution.managed && !resolution.accepted {
				b.mu.Unlock()
				return fmt.Errorf("resolve question prompt %q: %w", id, ErrQuestionPromptNotFound)
			}
		}
		pending.resolution <- questionResolution{canceled: true}
		delete(b.pending, id)
	}
	b.mu.Unlock()
	if !exists {
		return fmt.Errorf("resolve question prompt %q: %w", id, ErrQuestionPromptNotFound)
	}
	return nil
}

func (b *QuestionBroker) removeQuestion(id string, pending *pendingQuestion) {
	b.mu.Lock()
	if b.pending[id] == pending {
		delete(b.pending, id)
	}
	b.mu.Unlock()
}

// SubmitQuestionAnswers resolves one blocked ask_questions tool call.
func (h *Handler) SubmitQuestionAnswers(ctx context.Context, request *v1.SubmitQuestionAnswersRequest) (*v1.SubmitQuestionAnswersResponse, error) {
	if request == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("submit question answers request must not be nil"))
	}
	if err := ctx.Err(); err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, connect.NewError(connect.CodeDeadlineExceeded, err)
		}
		return nil, connect.NewError(connect.CodeCanceled, err)
	}
	var err error
	switch request.WhichResolution() {
	case v1.SubmitQuestionAnswersRequest_Answers_case:
		err = h.questions.ResolveAnswers(request.GetPromptId(), request.GetAnswers())
	case v1.SubmitQuestionAnswersRequest_Canceled_case:
		if !request.GetCanceled() {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("canceled resolution must be true"))
		}
		err = h.questions.ResolveCanceled(request.GetPromptId())
	default:
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("question resolution must be set"))
	}
	if err != nil {
		if errors.Is(err, ErrQuestionPromptNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, err)
		}
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	return v1.SubmitQuestionAnswersResponse_builder{}.Build(), nil
}

func questionsFromProto(questions []*v1.Question) []tools.Question {
	result := make([]tools.Question, len(questions))
	for index, question := range questions {
		if question == nil {
			continue
		}
		result[index] = tools.Question{
			ID:            question.GetId(),
			Header:        question.GetHeader(),
			Prompt:        question.GetPrompt(),
			Multiple:      question.GetMultiple(),
			AllowFreeform: question.GetAllowFreeform(),
			Options:       make([]tools.QuestionOption, len(question.GetOptions())),
		}
		for optionIndex, option := range question.GetOptions() {
			if option == nil {
				continue
			}
			result[index].Options[optionIndex] = tools.QuestionOption{
				ID:          option.GetId(),
				Label:       option.GetLabel(),
				Description: option.GetDescription(),
			}
		}
	}
	return result
}

func answersFromProto(answers []*v1.QuestionAnswer) []tools.QuestionAnswer {
	result := make([]tools.QuestionAnswer, len(answers))
	for index, answer := range answers {
		if answer == nil {
			continue
		}
		result[index] = tools.QuestionAnswer{
			QuestionID:        answer.GetQuestionId(),
			SelectedOptionIDs: append([]string(nil), answer.GetSelectedOptionIds()...),
			Freeform:          answer.GetFreeform(),
		}
	}
	return result
}
