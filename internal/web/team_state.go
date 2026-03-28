package web

import (
	"context"
	"fmt"

	"github.com/canhta/gistclaw/internal/teams"
)

type teamState struct {
	Config          teams.Config
	ActiveProfile   string
	Profiles        []teams.Profile
	ProfileSavePath string
}

func (s *Server) loadTeamState(ctx context.Context) (teamState, error) {
	if s.rt == nil {
		return teamState{}, fmt.Errorf("runtime: team dir not configured")
	}
	cfg, err := s.rt.TeamConfig(ctx)
	if err != nil {
		return teamState{}, err
	}
	activeProfile, err := s.rt.ActiveTeamProfile(ctx)
	if err != nil {
		return teamState{}, err
	}
	profiles, err := s.rt.ListTeamProfiles(ctx)
	if err != nil {
		return teamState{}, err
	}
	savePath, err := s.rt.TeamConfigPath(ctx)
	if err != nil {
		return teamState{}, err
	}
	return teamState{
		Config:          cfg,
		ActiveProfile:   activeProfile,
		Profiles:        profiles,
		ProfileSavePath: savePath,
	}, nil
}

func (s *Server) loadTeamConfig(ctx context.Context) (teams.Config, error) {
	state, err := s.loadTeamState(ctx)
	if err != nil {
		return teams.Config{}, err
	}
	return state.Config, nil
}
