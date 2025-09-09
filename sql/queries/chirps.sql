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

-- name: GetChirpsByAuthor :many
SELECT * FROM chirps WHERE user_id=$1;

-- name: GetChirpById :one
SELECT * FROM chirps WHERE id=$1;

-- name: DeleteChirpById :exec
DELETE FROM chirps WHERE id=$1;
