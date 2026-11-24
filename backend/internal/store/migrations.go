package store

type migration struct {
	version    int
	name       string
	statements []string
}

var migrations = []migration{
	{
		version: 1,
		name:    "initial_records",
		statements: []string{
			`CREATE TABLE IF NOT EXISTS experiments (
				id TEXT PRIMARY KEY,
				created_at TEXT NOT NULL,
				updated_at TEXT NOT NULL,
				name TEXT NOT NULL,
				config_json TEXT NOT NULL,
				results_json TEXT NOT NULL,
				status TEXT NOT NULL,
				core_version TEXT NOT NULL,
				seed INTEGER,
				error_message TEXT
			)`,
			`CREATE TABLE IF NOT EXISTS simulations (
				id TEXT PRIMARY KEY,
				experiment_id TEXT REFERENCES experiments(id) ON DELETE SET NULL,
				created_at TEXT NOT NULL,
				updated_at TEXT NOT NULL,
				model_json TEXT NOT NULL,
				initial_distribution_json TEXT NOT NULL,
				algorithm TEXT NOT NULL,
				seed INTEGER,
				core_version TEXT NOT NULL,
				trace_json TEXT NOT NULL,
				result_json TEXT NOT NULL,
				status TEXT NOT NULL,
				equilibrium_status TEXT NOT NULL,
				optimum_status TEXT NOT NULL,
				poa_status TEXT NOT NULL,
				error_message TEXT
			)`,
		},
	},
	{
		version: 2,
		name:    "history_indexes",
		statements: []string{
			`CREATE INDEX IF NOT EXISTS idx_experiments_created_at ON experiments(created_at DESC, id DESC)`,
			`CREATE INDEX IF NOT EXISTS idx_simulations_created_at ON simulations(created_at DESC, id DESC)`,
			`CREATE INDEX IF NOT EXISTS idx_simulations_experiment_id ON simulations(experiment_id, created_at DESC, id DESC)`,
		},
	},
}
