package bootstrap

import (
	"database/sql"
	"strings"

	"sealsuite-operation/internal/config"
	appdb "sealsuite-operation/internal/db"
	sqliteRepo "sealsuite-operation/internal/repository/sqlite"
	"sealsuite-operation/internal/sealsuite"
	"sealsuite-operation/internal/service"
)

type App struct {
	DB       *sql.DB
	Services *service.Services
	Client   *sealsuite.Client
}

func Build(cfg *config.Config) (*App, error) {
	dbPath := strings.TrimSpace(cfg.Database.Path)
	if dbPath == "" {
		dbPath = "./data/app.db"
	}

	db, err := appdb.OpenSQLite(dbPath)
	if err != nil {
		return nil, err
	}
	if err := appdb.Migrate(db); err != nil {
		_ = db.Close()
		return nil, err
	}

	client := sealsuite.NewClient(&cfg.SealSuite)
	client.SetMockMode(cfg.SealSuite.MockMode)

	services := service.NewServices(
		sqliteRepo.NewTaskDraftRepository(db),
		sqliteRepo.NewScheduleRepository(db),
		sqliteRepo.NewComplexTaskRepository(db),
		sqliteRepo.NewExternalIPSyncTaskRepository(db),
	)

	return &App{
		DB:       db,
		Services: services,
		Client:   client,
	}, nil
}
