package store

import "testing"

func TestResumeEventFailureDoesNotStrandQueuedRun(t *testing.T) {
	db, runs, _, conversation := runPreparationEnv(t)
	prepared, err := runs.PrepareConversationRun(t.Context(), PrepareConversationRunInput{UserID: "u", ConversationID: conversation.ID, ModelProviderID: "model", Content: "hello"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runs.UpdateRun(t.Context(), "u", prepared.Run.ID, UpdateAgentRunInput{Status: AgentRunFailed}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TRIGGER reject_resume BEFORE INSERT ON agent_events WHEN NEW.type = 'run_resumed' BEGIN SELECT RAISE(ABORT, 'injected'); END`); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runs.ResumeRunWithEvent(t.Context(), "u", prepared.Run.ID); err == nil {
		t.Fatal("expected resume marker failure")
	}
	run, err := runs.GetRun(t.Context(), "u", prepared.Run.ID)
	if err != nil || run.Status != AgentRunFailed {
		t.Fatalf("stranded run: %+v %v", run, err)
	}
	if _, err := db.Exec(`DROP TRIGGER reject_resume`); err != nil {
		t.Fatal(err)
	}
	run, event, err := runs.ResumeRunWithEvent(t.Context(), "u", run.ID)
	if err != nil || run.Status != AgentRunQueued || event.Type != "run_resumed" || event.Seq != 3 {
		t.Fatalf("resume contract: %+v %+v %v", run, event, err)
	}
	if _, _, err := runs.ResumeRunWithEvent(t.Context(), "u", run.ID); err == nil {
		t.Fatal("concurrent resume accepted twice")
	}
}
