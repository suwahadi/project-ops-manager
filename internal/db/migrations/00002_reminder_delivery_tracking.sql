-- +goose Up
-- +goose StatementBegin
ALTER TABLE reminder_logs ADD COLUMN updated_at timestamptz NOT NULL DEFAULT now();
CREATE INDEX reminder_logs_pending_idx ON reminder_logs (next_attempt_at) WHERE status = 'pending';
CREATE INDEX reminder_logs_asset_idx ON reminder_logs (asset_id, created_at DESC);
CREATE INDEX reminder_logs_updated_idx ON reminder_logs (updated_at DESC) WHERE status <> 'pending';

CREATE TABLE job_runs (
  id bigserial PRIMARY KEY,
  job text NOT NULL,
  started_at timestamptz NOT NULL,
  finished_at timestamptz,
  status text NOT NULL CHECK (status IN ('running','success','failed')),
  summary text NOT NULL DEFAULT '',
  error text
);
CREATE INDEX job_runs_job_idx ON job_runs (job, started_at DESC);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE job_runs;
DROP INDEX reminder_logs_updated_idx;
DROP INDEX reminder_logs_asset_idx;
DROP INDEX reminder_logs_pending_idx;
ALTER TABLE reminder_logs DROP COLUMN updated_at;
-- +goose StatementEnd
