-- name: CreateUser :one
INSERT INTO users(id, created_at, updated_at, email, hashed_password) 
VALUES (
  $1,
  $2,
  $3,
  $4,
  $5
) RETURNING *;

-- name: DeleteAll :exec
DELETE FROM users;

-- name: GetUserByEmail :one
SELECT * FROM users WHERE email = $1;

-- name: GetUserById :one
SELECT * FROM users WHERE id = $1;

-- name: UpdateUsrEmailPwd :one
UPDATE users SET updated_at = $2, email = $3, hashed_password = $4 WHERE id = $1 RETURNING *;

-- name: UpdateUsrChirpyRed :one
UPDATE users SET updated_at = $2, is_chirpy_red = $3 WHERE id = $1 RETURNING *;
