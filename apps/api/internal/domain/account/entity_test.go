package domainaccount

import (
	"errors"
	"testing"
)

func TestAuthenticateRejectsDisabledAndUnknownStatus(t *testing.T) {
	user, err := New("alice", "secret", "Alice")
	if err != nil {
		t.Fatal(err)
	}
	if err := user.Authenticate("secret"); err != nil {
		t.Fatal(err)
	}
	for _, status := range []int{StatusDisabled, -1, 2} {
		restored := RestoreUser(user.ID, user.Account, user.Password, user.Nickname, "", "", status, user.Role)
		if restored.Status != status {
			t.Fatalf("status %d rewritten to %d", status, restored.Status)
		}
		if err := restored.Authenticate("secret"); !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("disabled login error: %v", err)
		}
	}
}
