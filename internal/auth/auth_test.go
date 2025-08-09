package auth

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestValidateJWT(t *testing.T) {
	userID := uuid.New()
	validtoken, _ := MakeJWT(userID, "theonering", time.Hour)

	tests := []struct {
		name        string
		tokenString string
		tokenSecret string
		wantUserID  uuid.UUID
		wantErr     bool
	}{
		{
			name:        "Valid token",
			tokenString: validtoken,
			tokenSecret: "theonering",
			wantUserID:  userID,
			wantErr:     false,
		},
		// {
		// 	name:        "Invalid token",
		// 	tokenString: "invalid.token.string",
		// 	tokenSecret: "theonering",
		// 	wantUserID:  uuid.Nil,
		// 	wantErr:     true,
		// },
		// {
		// 	name:        "Wrong secret",
		// 	tokenString: validtoken,
		// 	tokenSecret: "wrong_secret",
		// 	wantUserID:  uuid.Nil,
		// 	wantErr:     true,
		// },
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotUserID, err := ValidateJWT(tt.tokenString, tt.tokenSecret)
			if err != nil {
				t.Errorf("ValidateJWT error = %v, want err %v", err, tt.wantErr)
				return
			}
			if gotUserID != tt.wantUserID {
				t.Errorf("ValidateJWT gotUserID = %v, want %v", gotUserID, tt.wantUserID)
			}
		})
	}
}

