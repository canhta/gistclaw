package model

import (
	"context"
	"reflect"
	"testing"
)

func TestProviderError_ImplementsError(t *testing.T) {
	var err error = &ProviderError{
		Code:    ErrRateLimit,
		Message: "too many requests",
	}

	if got := err.Error(); got != "rate_limit: too many requests" {
		t.Fatalf("expected %q, got %q", "rate_limit: too many requests", got)
	}
}

func TestNoopEventSink_AlwaysSucceeds(t *testing.T) {
	sink := &NoopEventSink{}
	err := sink.Emit(context.Background(), "run-123", ReplayDelta{
		RunID: "run-123",
		Kind:  "test",
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestAllProviderCodes_Defined(t *testing.T) {
	codes := []ProviderErrorCode{
		ErrRateLimit,
		ErrContextWindowExceeded,
		ErrModelRefusal,
		ErrProviderTimeout,
		ErrMalformedResponse,
	}

	seen := make(map[ProviderErrorCode]bool)
	for _, code := range codes {
		if code == "" {
			t.Fatal("provider error code must not be empty")
		}
		if seen[code] {
			t.Fatalf("duplicate provider error code: %s", code)
		}
		seen[code] = true
	}
	if len(seen) != 5 {
		t.Fatalf("expected 5 distinct codes, got %d", len(seen))
	}
}

func TestRunStatus_AllDefined(t *testing.T) {
	statuses := []RunStatus{
		RunStatusPending,
		RunStatusActive,
		RunStatusNeedsApproval,
		RunStatusCompleted,
		RunStatusInterrupted,
		RunStatusFailed,
	}

	for _, status := range statuses {
		if status == "" {
			t.Fatal("RunStatus must not be empty")
		}
	}
}

func TestNoopEventSink_ImplementsInterface(t *testing.T) {
	var _ RunEventSink = &NoopEventSink{}
}

func TestSessionMessageKindsRemainStable(t *testing.T) {
	got := []SessionMessageKind{
		MessageUser,
		MessageAssistant,
		MessageSpawn,
		MessageAnnounce,
		MessageSteer,
		MessageAgentSend,
	}
	want := []SessionMessageKind{
		"user",
		"assistant",
		"spawn",
		"announce",
		"steer",
		"agent_send",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected message kinds: %#v", got)
	}
}

func TestAgentProfile_ExposesAdaptivePolicyFields(t *testing.T) {
	agentType := reflect.TypeOf(AgentProfile{})
	assertHasField(
		t,
		agentType,
		"BaseProfile",
		"ToolFamilies",
		"AllowTools",
		"DenyTools",
		"DelegationKinds",
		"SpecialistSummaryVisibility",
	)
	assertOmitsField(t, agentType, "Capabilities", "ToolProfile", "CanSpawn")
}

func TestToolSpec_ExposesFamily(t *testing.T) {
	specType := reflect.TypeOf(ToolSpec{})
	assertHasField(t, specType, "Family")
}

func TestAdaptivePolicyValidators(t *testing.T) {
	t.Run("base profile", func(t *testing.T) {
		if !IsValidBaseProfile("operator") {
			t.Fatal("expected operator to be a valid base profile")
		}
		if IsValidBaseProfile("specialist") {
			t.Fatal("expected specialist base profile to be rejected")
		}
		if IsValidBaseProfile("unknown") {
			t.Fatal("expected unknown base profile to be rejected")
		}
	})

	t.Run("tool family", func(t *testing.T) {
		if !IsValidToolFamily("connector_capability") {
			t.Fatal("expected connector_capability to be a valid tool family")
		}
		if IsValidToolFamily("unexpected_tooling") {
			t.Fatal("expected unexpected_tooling tool family to be rejected")
		}
	})

	t.Run("delegation kind", func(t *testing.T) {
		if !IsValidDelegationKind("research") {
			t.Fatal("expected research to be a valid delegation kind")
		}
		if IsValidDelegationKind("fanout") {
			t.Fatal("expected fanout delegation kind to be rejected")
		}
	})

	t.Run("specialist summary visibility", func(t *testing.T) {
		if !IsValidSpecialistSummaryVisibility("basic") {
			t.Fatal("expected basic to be a valid specialist summary visibility")
		}
		if IsValidSpecialistSummaryVisibility("verbose") {
			t.Fatal("expected verbose specialist summary visibility to be rejected")
		}
	})
}

func TestHostExecutionTypes_ReplaceWorkspaceFields(t *testing.T) {
	projectType := reflect.TypeOf(Project{})
	assertHasField(t, projectType, "PrimaryPath", "RootsJSON", "PolicyJSON")
	assertOmitsField(t, projectType, "WorkspaceRoot")

	runType := reflect.TypeOf(Run{})
	assertHasField(t, runType, "CWD", "AuthorityJSON")
	assertOmitsField(t, runType, "WorkspaceRoot")

	approvalRequestType := reflect.TypeOf(ApprovalRequest{})
	assertHasField(t, approvalRequestType, "BindingJSON")
	assertOmitsField(t, approvalRequestType, "TargetPath")

	approvalTicketType := reflect.TypeOf(ApprovalTicket{})
	assertHasField(t, approvalTicketType, "BindingJSON")
	assertOmitsField(t, approvalTicketType, "TargetPath")
}

func assertHasField(t *testing.T, typ reflect.Type, fields ...string) {
	t.Helper()
	for _, field := range fields {
		if _, ok := typ.FieldByName(field); !ok {
			t.Fatalf("expected %s to expose field %q", typ.Name(), field)
		}
	}
}

func assertOmitsField(t *testing.T, typ reflect.Type, fields ...string) {
	t.Helper()
	for _, field := range fields {
		if _, ok := typ.FieldByName(field); ok {
			t.Fatalf("expected %s to omit field %q", typ.Name(), field)
		}
	}
}
