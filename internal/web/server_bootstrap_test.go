package web

import (
	"path/filepath"
	"testing"

	"sealsuite-operation/internal/config"
	appdb "sealsuite-operation/internal/db"
	sqliteRepo "sealsuite-operation/internal/repository/sqlite"
	"sealsuite-operation/internal/runner"
	"sealsuite-operation/internal/sealsuite"
	"sealsuite-operation/internal/service"
)

func TestNewServerWithServicesUsesInjectedServices(t *testing.T) {
	withTempWorkingDir(t, func(dir string) {
		db, err := appdb.OpenSQLite(filepath.Join(dir, "app.db"))
		if err != nil {
			t.Fatalf("OpenSQLite err=%v", err)
		}
		defer db.Close()

		if err := appdb.Migrate(db); err != nil {
			t.Fatalf("Migrate err=%v", err)
		}

		services := service.NewServices(
			sqliteRepo.NewTaskDraftRepository(db),
			sqliteRepo.NewScheduleRepository(db),
			sqliteRepo.NewComplexTaskRepository(db),
			sqliteRepo.NewExternalIPSyncTaskRepository(db),
		)

		cfg := &config.Config{
			Server:   config.ServerConfig{Bind: "127.0.0.1", Port: 0, Mode: "debug"},
			Database: config.DatabaseConfig{Path: "/proc/invalid/app.db"},
			SealSuite: config.SealSuiteConfig{
				BaseURL:  "http://example.com",
				Timeout:  1,
				MockMode: true,
			},
		}
		client := sealsuite.NewClient(&cfg.SealSuite)
		client.SetMockMode(true)

		r := runner.New(cfg, client)
		srv, err := NewServerWithServices(cfg, r, services)
		if err != nil {
			t.Fatalf("NewServerWithServices err=%v", err)
		}
		if srv == nil || srv.Handler == nil {
			t.Fatalf("expected http server with handler")
		}
	})
}
