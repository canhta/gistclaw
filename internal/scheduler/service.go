package scheduler

import (
	"context"
	"fmt"
	"time"

	"github.com/canhta/gistclaw/internal/model"
)

type ConversationKey struct {
	ConnectorID string
	AccountID   string
	ExternalID  string
	ThreadID    string
}

type DispatchCommand struct {
	ConversationKey ConversationKey
	FrontAgentID    string
	Body            string
	SourceMessageID string
	WorkspaceRoot   string
}

type Dispatcher interface {
	DispatchScheduled(ctx context.Context, cmd DispatchCommand) (model.Run, error)
}

type Service struct {
	store               *Store
	dispatcher          Dispatcher
	clock               func() time.Time
	wakeInterval        time.Duration
	dispatchGracePeriod time.Duration
	dueLimit            int
}

func NewService(store *Store, dispatcher Dispatcher) *Service {
	return &Service{
		store:               store,
		dispatcher:          dispatcher,
		clock:               func() time.Time { return time.Now().UTC() },
		wakeInterval:        30 * time.Second,
		dispatchGracePeriod: 30 * time.Second,
		dueLimit:            100,
	}
}

func (s *Service) CreateSchedule(ctx context.Context, in CreateScheduleInput) (Schedule, error) {
	return s.store.CreateSchedule(ctx, in)
}

func (s *Service) UpdateSchedule(ctx context.Context, scheduleID string, patch UpdateScheduleInput) (Schedule, error) {
	return s.store.UpdateSchedule(ctx, scheduleID, patch)
}

func (s *Service) LoadSchedule(ctx context.Context, scheduleID string) (Schedule, error) {
	return s.store.LoadSchedule(ctx, scheduleID)
}

func (s *Service) ListSchedules(ctx context.Context) ([]Schedule, error) {
	return s.store.ListSchedules(ctx)
}

func (s *Service) EnableSchedule(ctx context.Context, scheduleID string) (Schedule, error) {
	return s.store.SetScheduleEnabled(ctx, scheduleID, true, s.clock().UTC())
}

func (s *Service) DisableSchedule(ctx context.Context, scheduleID string) (Schedule, error) {
	return s.store.SetScheduleEnabled(ctx, scheduleID, false, s.clock().UTC())
}

func (s *Service) DeleteSchedule(ctx context.Context, scheduleID string) error {
	return s.store.DeleteSchedule(ctx, scheduleID)
}

func (s *Service) RunNow(ctx context.Context, scheduleID string) (*ClaimedOccurrence, error) {
	now := s.clock().UTC()
	claimed, err := s.store.ClaimManualOccurrence(ctx, scheduleID, now)
	if err != nil {
		return nil, err
	}
	if claimed == nil {
		return nil, nil
	}
	if err := s.dispatchClaimedOccurrence(ctx, *claimed, now); err != nil {
		return nil, err
	}
	return claimed, nil
}

func (s *Service) Start(ctx context.Context) error {
	if s.wakeInterval <= 0 {
		s.wakeInterval = 30 * time.Second
	}

	ticker := time.NewTicker(s.wakeInterval)
	defer ticker.Stop()

	for {
		if err := s.RunOnce(ctx); err != nil {
			return err
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (s *Service) Repair(ctx context.Context) error {
	now := s.clock().UTC()

	if err := s.reconcileOpenOccurrences(ctx, now); err != nil {
		return err
	}
	if err := s.store.RepairMissingNextRunAt(ctx, now); err != nil {
		return err
	}
	return nil
}

func (s *Service) RunOnce(ctx context.Context) error {
	now := s.clock().UTC()

	if err := s.reconcileOpenOccurrences(ctx, now); err != nil {
		return err
	}
	if err := s.store.RepairMissingNextRunAt(ctx, now); err != nil {
		return err
	}

	dueSchedules, err := s.store.ListDueSchedules(ctx, now, s.dueLimit)
	if err != nil {
		return err
	}

	for _, schedule := range dueSchedules {
		claimed, err := s.store.ClaimDueOccurrence(ctx, schedule.ID, now)
		if err != nil {
			return err
		}
		if claimed == nil {
			continue
		}
		if err := s.dispatchClaimedOccurrence(ctx, *claimed, now); err != nil {
			return err
		}
	}

	return nil
}

func (s *Service) reconcileOpenOccurrences(ctx context.Context, now time.Time) error {
	occurrences, err := s.store.ListOpenOccurrences(ctx)
	if err != nil {
		return err
	}

	for _, occurrence := range occurrences {
		if occurrence.RunID == "" {
			recovered, found, err := s.store.RecoverRunFromReceipt(ctx, occurrence)
			if err != nil {
				return err
			}
			if found {
				if err := s.store.MarkOccurrenceAccepted(ctx, occurrence.ID, recovered, now); err != nil {
					return err
				}
				if err := s.store.SyncOccurrenceWithRun(ctx, occurrence.ID, recovered, now); err != nil {
					return err
				}
				continue
			}

			if occurrence.Status == OccurrenceDispatching && occurrence.CreatedAt.Before(now.Add(-s.dispatchGracePeriod)) {
				if err := s.store.MarkOccurrenceFailed(ctx, occurrence.ID, "dispatch receipt not found", now); err != nil {
					return err
				}
			}
			continue
		}

		run, err := s.store.LoadRun(ctx, occurrence.RunID)
		if err != nil {
			return err
		}
		if err := s.store.SyncOccurrenceWithRun(ctx, occurrence.ID, run, now); err != nil {
			return err
		}
	}

	return nil
}

func (s *Service) dispatchClaimedOccurrence(ctx context.Context, claimed ClaimedOccurrence, now time.Time) error {
	if s.dispatcher == nil {
		return fmt.Errorf("scheduler: dispatcher is nil")
	}

	run, err := s.dispatcher.DispatchScheduled(ctx, DispatchCommand{
		ConversationKey: ConversationKey{
			ConnectorID: "schedule",
			AccountID:   "local",
			ExternalID:  "job:" + claimed.Schedule.ID,
			ThreadID:    claimed.Occurrence.ThreadID,
		},
		FrontAgentID:    "assistant",
		Body:            claimed.Schedule.Objective,
		SourceMessageID: claimed.Occurrence.ID,
		WorkspaceRoot:   claimed.Schedule.WorkspaceRoot,
	})
	if err == nil {
		if err := s.store.MarkOccurrenceAccepted(ctx, claimed.Occurrence.ID, run, now); err != nil {
			return err
		}
		return nil
	}

	recovered, found, recoverErr := s.store.RecoverRunFromReceipt(ctx, claimed.Occurrence)
	if recoverErr != nil {
		return recoverErr
	}
	if found {
		if err := s.store.MarkOccurrenceAccepted(ctx, claimed.Occurrence.ID, recovered, now); err != nil {
			return err
		}
		if err := s.store.SyncOccurrenceWithRun(ctx, claimed.Occurrence.ID, recovered, now); err != nil {
			return err
		}
		return nil
	}

	if err := s.store.MarkOccurrenceFailed(ctx, claimed.Occurrence.ID, err.Error(), now); err != nil {
		return err
	}
	return nil
}
