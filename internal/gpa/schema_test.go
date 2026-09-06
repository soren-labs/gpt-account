package gpa

import "testing"

func TestMaskEmail(t *testing.T) {
	if MaskEmail("alice@example.com") != "a***@example.com" {
		t.Fatal(MaskEmail("alice@example.com"))
	}
}

func TestStableAccountIDStable(t *testing.T) {
	id := Identity{UserID: "u1", WorkspaceID: "w1"}
	if StableAccountID(id, "plus") != StableAccountID(id, "other") {
		t.Fatal("id should follow seat, not slot")
	}
	if StableAccountID(id, "plus") == StableAccountID(Identity{UserID: "u2", WorkspaceID: "w1"}, "plus") {
		t.Fatal("different seats")
	}
}
