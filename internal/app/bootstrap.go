package app

import (
	"github.com/canhta/gistclaw/internal/conversations"
	"github.com/canhta/gistclaw/internal/memory"
	"github.com/canhta/gistclaw/internal/model"
	"github.com/canhta/gistclaw/internal/replay"
	"github.com/canhta/gistclaw/internal/runtime"
	"github.com/canhta/gistclaw/internal/store"
	"github.com/canhta/gistclaw/internal/tools"
)

type App struct {
	db      *store.DB
	runtime *runtime.Runtime
	replay  *replay.Service
}

func Bootstrap(cfg Config) (*App, error) {
	db, err := storeWiring(cfg)
	if err != nil {
		return nil, err
	}

	convStore := conversations.NewConversationStore(db)
	mem := memory.NewStore(db, convStore)
	reg := tools.NewRegistry()
	sink := &model.NoopEventSink{}
	rt := runtimeWiring(cfg, db, convStore, reg, mem, sink)
	rp := replayWiring(db)

	return &App{
		db:      db,
		runtime: rt,
		replay:  rp,
	}, nil
}

func storeWiring(cfg Config) (*store.DB, error) {
	db, err := store.Open(cfg.DatabasePath)
	if err != nil {
		return nil, err
	}
	if err := store.Migrate(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

func runtimeWiring(
	_ Config,
	db *store.DB,
	cs *conversations.ConversationStore,
	reg *tools.Registry,
	mem *memory.Store,
	sink model.RunEventSink,
) *runtime.Runtime {
	prov := runtime.NewMockProvider(nil, nil)
	return runtime.New(db, cs, reg, mem, prov, sink)
}

func replayWiring(db *store.DB) *replay.Service {
	return replay.NewService(db)
}
