package server

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"sync"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	v1 "github.com/soasurs/koda/gen/koda/v1"
)

var (
	errRunManagerClosed  = errors.New("run manager is shutting down")
	errRunAlreadyActive  = errors.New("session already has an active run")
	errRunRequestChanged = errors.New("client request ID was reused with different input")
)

const maxRetainedRuns = 1024

type runAdmission struct {
	sessionID       string
	clientRequestID string
	fingerprint     [sha256.Size]byte
}

type interactionResolution struct {
	managed  bool
	accepted bool
}

type runManager struct {
	mu              sync.Mutex
	ctx             context.Context
	cancel          context.CancelFunc
	accepting       bool
	runsByID        map[string]*managedRun
	activeBySession map[string]*managedRun
	byRequest       map[string]*managedRun
	terminalOrder   []*managedRun
	wg              sync.WaitGroup
	newID           func() (string, error)
}

type managedRun struct {
	id              string
	sessionID       string
	clientRequestID string
	requestKey      string
	fingerprint     [sha256.Size]byte
	admittedAt      int64

	ctx    context.Context
	cancel context.CancelFunc

	mu          sync.Mutex
	state       v1.RunState
	turnID      string
	finishedAt  int64
	sequence    int64
	frames      []*v1.RunResponse
	approvals   map[string]*v1.ToolApproval
	questions   map[string]*v1.QuestionPrompt
	notify      chan struct{}
	done        chan struct{}
	terminalErr error
	watchers    int
}

func newRunManager(newID func() (string, error)) *runManager {
	ctx, cancel := context.WithCancel(context.Background())
	return &runManager{
		ctx:             ctx,
		cancel:          cancel,
		accepting:       true,
		runsByID:        make(map[string]*managedRun),
		activeBySession: make(map[string]*managedRun),
		byRequest:       make(map[string]*managedRun),
		newID:           newID,
	}
}

func (m *runManager) admit(admission runAdmission, execute func(context.Context, *managedRun) error) (*managedRun, bool, error) {
	if execute == nil {
		return nil, false, errors.New("run manager: execute function must not be nil")
	}
	requestKey := admission.sessionID + "\x00" + admission.clientRequestID
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.accepting {
		return nil, false, errRunManagerClosed
	}
	if existing := m.byRequest[requestKey]; existing != nil {
		if existing.fingerprint != admission.fingerprint {
			return nil, false, fmt.Errorf("%w: %q", errRunRequestChanged, admission.clientRequestID)
		}
		return existing, false, nil
	}
	if existing := m.activeBySession[admission.sessionID]; existing != nil {
		return nil, false, fmt.Errorf("%w: %q", errRunAlreadyActive, existing.id)
	}
	id, err := m.newID()
	if err != nil {
		return nil, false, fmt.Errorf("generate run ID: %w", err)
	}
	ctx, cancel := context.WithCancel(m.ctx)
	run := &managedRun{
		id:              id,
		sessionID:       admission.sessionID,
		clientRequestID: admission.clientRequestID,
		requestKey:      requestKey,
		fingerprint:     admission.fingerprint,
		admittedAt:      time.Now().UnixMilli(),
		ctx:             ctx,
		cancel:          cancel,
		state:           v1.RunState_RUN_STATE_ADMITTED,
		approvals:       make(map[string]*v1.ToolApproval),
		questions:       make(map[string]*v1.QuestionPrompt),
		notify:          make(chan struct{}),
		done:            make(chan struct{}),
	}
	started := new(v1.RunResponse)
	started.SetStarted(v1.RunStarted_builder{
		ClientRequestId: new(admission.clientRequestID),
		SessionId:       new(admission.sessionID),
		AdmittedAt:      new(run.admittedAt),
	}.Build())
	run.appendLocked(started)

	m.runsByID[id] = run
	m.activeBySession[admission.sessionID] = run
	m.byRequest[requestKey] = run
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		err := execute(ctx, run)
		if err != nil {
			run.terminate(err)
		} else {
			run.ensureSucceeded()
		}
		m.complete(run)
	}()
	return run, true, nil
}

func (m *runManager) complete(run *managedRun) {
	m.mu.Lock()
	if m.activeBySession[run.sessionID] == run {
		delete(m.activeBySession, run.sessionID)
	}
	m.terminalOrder = append(m.terminalOrder, run)
	for len(m.terminalOrder) > maxRetainedRuns {
		expired := m.terminalOrder[0]
		m.terminalOrder = m.terminalOrder[1:]
		delete(m.runsByID, expired.id)
		delete(m.byRequest, expired.requestKey)
	}
	m.mu.Unlock()
	close(run.done)
	run.compactJournalIfIdle()
}

func (m *runManager) get(id string) *managedRun {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.runsByID[id]
}

func (m *runManager) active(sessionID string) *managedRun {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.activeBySession[sessionID]
}

func (m *runManager) resolveApproval(id string) interactionResolution {
	m.mu.Lock()
	runs := make([]*managedRun, 0, len(m.runsByID))
	for _, run := range m.runsByID {
		runs = append(runs, run)
	}
	m.mu.Unlock()
	for _, run := range runs {
		run.mu.Lock()
		_, pending := run.approvals[id]
		if pending && run.ctx.Err() == nil && !terminalRunState(run.state) {
			delete(run.approvals, id)
			resolved := new(v1.RunResponse)
			resolved.SetInteractionResolved(v1.RunInteractionResolved_builder{ApprovalId: new(id)}.Build())
			run.appendLocked(resolved)
			run.mu.Unlock()
			return interactionResolution{managed: true, accepted: true}
		}
		run.mu.Unlock()
		if pending {
			return interactionResolution{managed: true}
		}
	}
	return interactionResolution{}
}

func (m *runManager) resolveQuestion(id string) interactionResolution {
	m.mu.Lock()
	runs := make([]*managedRun, 0, len(m.runsByID))
	for _, run := range m.runsByID {
		runs = append(runs, run)
	}
	m.mu.Unlock()
	for _, run := range runs {
		run.mu.Lock()
		_, pending := run.questions[id]
		if pending && run.ctx.Err() == nil && !terminalRunState(run.state) {
			delete(run.questions, id)
			resolved := new(v1.RunResponse)
			resolved.SetInteractionResolved(v1.RunInteractionResolved_builder{QuestionPromptId: new(id)}.Build())
			run.appendLocked(resolved)
			run.mu.Unlock()
			return interactionResolution{managed: true, accepted: true}
		}
		run.mu.Unlock()
		if pending {
			return interactionResolution{managed: true}
		}
	}
	return interactionResolution{}
}

func (m *runManager) shutdown(ctx context.Context) error {
	m.mu.Lock()
	if m.accepting {
		m.accepting = false
		m.cancel()
	}
	m.mu.Unlock()
	done := make(chan struct{})
	go func() {
		m.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (r *managedRun) setState(state v1.RunState) {
	r.mu.Lock()
	if !terminalRunState(r.state) {
		r.state = state
	}
	r.mu.Unlock()
}

func (r *managedRun) setTurnID(turnID string) {
	if turnID == "" {
		return
	}
	r.mu.Lock()
	r.turnID = turnID
	r.mu.Unlock()
}

func (r *managedRun) requestCancel() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if terminalRunState(r.state) || r.state == v1.RunState_RUN_STATE_FINALIZING {
		return
	}
	r.cancel()
}

func (r *managedRun) publish(response *v1.RunResponse) error {
	if response == nil {
		return errors.New("run manager: response must not be nil")
	}
	if err := r.ctx.Err(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if terminalRunState(r.state) {
		return errors.New("run manager: run already terminated")
	}
	if approval := response.GetApproval(); approval != nil {
		r.approvals[approval.GetId()] = proto.Clone(approval).(*v1.ToolApproval)
	}
	if question := response.GetQuestionPrompt(); question != nil {
		r.questions[question.GetId()] = proto.Clone(question).(*v1.QuestionPrompt)
	}
	if completed := response.GetCompleted(); completed != nil {
		r.turnID = completed.GetTurnId()
		r.state = v1.RunState_RUN_STATE_SUCCEEDED
		r.finishedAt = time.Now().UnixMilli()
		r.approvals = make(map[string]*v1.ToolApproval)
		r.questions = make(map[string]*v1.QuestionPrompt)
	}
	r.appendLocked(response)
	return nil
}

func (r *managedRun) appendLocked(response *v1.RunResponse) {
	r.sequence++
	owned := proto.Clone(response).(*v1.RunResponse)
	owned.SetRunId(r.id)
	owned.SetSequence(r.sequence)
	r.frames = append(r.frames, owned)
	close(r.notify)
	r.notify = make(chan struct{})
}

func (r *managedRun) terminate(err error) {
	r.mu.Lock()
	if terminalRunState(r.state) {
		r.mu.Unlock()
		return
	}
	state := v1.RunState_RUN_STATE_FAILED
	reason := v1.TurnReason_TURN_REASON_AGENT_ERROR
	code := connect.CodeOf(err).String()
	message := "Run failed"
	if errors.Is(err, context.Canceled) || connect.CodeOf(err) == connect.CodeCanceled {
		state = v1.RunState_RUN_STATE_CANCELED
		reason = v1.TurnReason_TURN_REASON_CANCELED
		message = "Run canceled"
	} else if errors.Is(err, context.DeadlineExceeded) || connect.CodeOf(err) == connect.CodeDeadlineExceeded {
		reason = v1.TurnReason_TURN_REASON_DEADLINE_EXCEEDED
	}
	r.state = state
	r.finishedAt = time.Now().UnixMilli()
	r.terminalErr = err
	r.approvals = make(map[string]*v1.ToolApproval)
	r.questions = make(map[string]*v1.QuestionPrompt)
	response := new(v1.RunResponse)
	response.SetTerminated(v1.RunTerminated_builder{
		State:  state.Enum(),
		Reason: reason.Enum(),
		Failure: v1.TurnFailure_builder{
			Code:    new(code),
			Message: new(message),
		}.Build(),
		TurnId:     new(r.turnID),
		FinishedAt: new(r.finishedAt),
	}.Build())
	r.appendLocked(response)
	r.mu.Unlock()
}

func (r *managedRun) ensureSucceeded() {
	r.mu.Lock()
	if !terminalRunState(r.state) {
		r.state = v1.RunState_RUN_STATE_SUCCEEDED
		r.finishedAt = time.Now().UnixMilli()
		close(r.notify)
		r.notify = make(chan struct{})
	}
	r.mu.Unlock()
}

func (r *managedRun) snapshot() *v1.RunSnapshot {
	r.mu.Lock()
	defer r.mu.Unlock()
	approvals := make([]*v1.ToolApproval, 0, len(r.approvals))
	for _, approval := range r.approvals {
		approvals = append(approvals, proto.Clone(approval).(*v1.ToolApproval))
	}
	questions := make([]*v1.QuestionPrompt, 0, len(r.questions))
	for _, question := range r.questions {
		questions = append(questions, proto.Clone(question).(*v1.QuestionPrompt))
	}
	return v1.RunSnapshot_builder{
		RunId:           new(r.id),
		ClientRequestId: new(r.clientRequestID),
		SessionId:       new(r.sessionID),
		State:           r.state.Enum(),
		TurnId:          new(r.turnID),
		AdmittedAt:      new(r.admittedAt),
		FinishedAt:      new(r.finishedAt),
		LastSequence:    new(r.sequence),
		Approvals:       approvals,
		QuestionPrompts: questions,
	}.Build()
}

func (r *managedRun) watch(ctx context.Context, afterSequence int64, send func(*v1.RunResponse) error) error {
	if afterSequence < 0 {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("after sequence must not be negative"))
	}
	r.mu.Lock()
	r.watchers++
	r.mu.Unlock()
	defer func() {
		r.mu.Lock()
		r.watchers--
		r.compactJournalLocked()
		r.mu.Unlock()
	}()
	for {
		r.mu.Lock()
		frames := make([]*v1.RunResponse, 0)
		for _, frame := range r.frames {
			if frame.GetSequence() > afterSequence {
				frames = append(frames, proto.Clone(frame).(*v1.RunResponse))
			}
		}
		notify := r.notify
		terminal := terminalRunState(r.state)
		r.mu.Unlock()
		for _, frame := range frames {
			if err := send(frame); err != nil {
				return err
			}
			afterSequence = frame.GetSequence()
		}
		if terminal {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-r.done:
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-notify:
		}
	}
}

func (r *managedRun) compactJournalIfIdle() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.compactJournalLocked()
}

func (r *managedRun) compactJournalLocked() {
	if r.watchers != 0 || !terminalRunState(r.state) || len(r.frames) <= 2 {
		return
	}
	r.frames = []*v1.RunResponse{r.frames[0], r.frames[len(r.frames)-1]}
}

func (r *managedRun) executionError() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.terminalErr
}

func terminalRunState(state v1.RunState) bool {
	switch state {
	case v1.RunState_RUN_STATE_SUCCEEDED, v1.RunState_RUN_STATE_FAILED, v1.RunState_RUN_STATE_CANCELED:
		return true
	default:
		return false
	}
}

// WatchRun attaches one transport stream to a server-owned Run.
func (h *Handler) WatchRun(ctx context.Context, request *v1.WatchRunRequest, stream *connect.ServerStream[v1.WatchRunResponse]) error {
	if request == nil {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("watch run request must not be nil"))
	}
	run := h.runs.get(request.GetRunId())
	if run == nil {
		return connect.NewError(connect.CodeNotFound, errors.New("run not found"))
	}
	return run.watch(ctx, request.GetAfterSequence(), func(frame *v1.RunResponse) error {
		return stream.Send(v1.WatchRunResponse_builder{Frame: frame}.Build())
	})
}

// GetRun returns the recoverable state of an active or recent Run.
func (h *Handler) GetRun(ctx context.Context, request *v1.GetRunRequest) (*v1.GetRunResponse, error) {
	if request == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("get run request must not be nil"))
	}
	if err := ctx.Err(); err != nil {
		return nil, runtimeError(err)
	}
	run := h.runs.get(request.GetRunId())
	if run == nil {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("run not found"))
	}
	return v1.GetRunResponse_builder{Run: run.snapshot()}.Build(), nil
}

// GetActiveRun returns the Run currently occupying a session, if one exists.
func (h *Handler) GetActiveRun(ctx context.Context, request *v1.GetActiveRunRequest) (*v1.GetActiveRunResponse, error) {
	if request == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("get active run request must not be nil"))
	}
	id, err := sessionIDFromRequest(request.GetSessionId())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	if _, err := h.store.GetSession(ctx, id); err != nil {
		return nil, sessionError(err)
	}
	run := h.runs.active(id)
	if run == nil {
		return v1.GetActiveRunResponse_builder{}.Build(), nil
	}
	return v1.GetActiveRunResponse_builder{Run: run.snapshot()}.Build(), nil
}

// CancelRun explicitly requests cancellation without depending on a watcher.
func (h *Handler) CancelRun(ctx context.Context, request *v1.CancelRunRequest) (*v1.CancelRunResponse, error) {
	if request == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("cancel run request must not be nil"))
	}
	if err := ctx.Err(); err != nil {
		return nil, runtimeError(err)
	}
	run := h.runs.get(request.GetRunId())
	if run == nil {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("run not found"))
	}
	run.requestCancel()
	return v1.CancelRunResponse_builder{Run: run.snapshot()}.Build(), nil
}
