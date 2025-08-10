-- name: CreateRefreshToken :one
INSERT INTO refresh_tokens(
  token, created_at, updated_at, user_id, expires_at, revoked_at
) VALUES (
  $1,
  $2,
  $3,
  $4,
  $5,
  NULL
) RETURNING *;

-- name: ResetRefreshTokens :exec
DELETE FROM refresh_tokens;

-- name: GetRefreshToken :one
SELECT * FROM refresh_tokens WHERE token=$1;

-- name: RevokeRefreshToken :exec
UPDATE refresh_tokens SET updated_at=$1, revoked_at=$2 WHERE token=$3;

