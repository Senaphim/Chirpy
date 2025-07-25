-- name: CreateChirp :one
INSERT INTO chirps(id, created_at, updated_at, body, user_id) 
VALUES (
  $1,
  $2,
  $3,
  $4,
  $5
) RETURNING *;

-- name: ResetChirps :exec
DELETE FROM chirps;

-- name: AllChirps :many
SELECT * FROM chirps ORDER BY updated_at ASC;
