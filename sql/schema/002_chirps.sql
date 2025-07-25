-- +goose up
CREATE TABLE chirps(
  id UUID PRIMARY KEY,
  created_at TIMESTAMP NOT NULL,
  updated_at TIMESTAMP NOT NULL,
  body TEXT NOT NULL UNIQUE,
  user_id UUID NOT NULL REFERENCES users ON DELETE CASCADE,
  CONSTRAINT fk_users FOREIGN KEY (user_id) REFERENCES users(id)
);

-- +goose down
DROP TABLE chirps;
