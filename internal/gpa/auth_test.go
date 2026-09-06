package gpa

import "testing"

func TestInspectReadsPlanAndUser(t *testing.T) {
	id := InspectAuth(FakeAuth("a@x.com", "user-a", "plus", "ws-1", ""))
	if id.Email != "a@x.com" || id.UserID != "user-a" || id.Plan != "plus" || id.WorkspaceID != "ws-1" {
		t.Fatalf("%+v", id)
	}
	if !IsChatGPTBundle(id) {
		t.Fatal("expected bundle")
	}
}

func TestSameWorkspaceDifferentSeats(t *testing.T) {
	a := InspectAuth(FakeAuth("a@co.com", "u1", "team", "ws", ""))
	b := InspectAuth(FakeAuth("b@co.com", "u2", "team", "ws", ""))
	if SameSeat(a, b) {
		t.Fatal("different seats")
	}
}

func TestSameUserDifferentWorkspaces(t *testing.T) {
	a := InspectAuth(FakeAuth("a@x.com", "u1", "plus", "ws-a", ""))
	b := InspectAuth(FakeAuth("a@x.com", "u1", "team", "ws-b", ""))
	if SameSeat(a, b) {
		t.Fatal("different workspaces")
	}
}

func TestSameSeatMatches(t *testing.T) {
	a := InspectAuth(FakeAuth("a@x.com", "u1", "team", "ws", ""))
	b := InspectAuth(FakeAuth("a@x.com", "u1", "team", "ws", ""))
	if !SameSeat(a, b) {
		t.Fatal("same seat")
	}
}

func TestUnusableWithoutRefresh(t *testing.T) {
	auth := FakeAuth("a@x.com", "u1", "plus", "ws", "")
	tokens := asMap(auth["tokens"])
	tokens["refresh_token"] = ""
	if IsChatGPTBundle(InspectAuth(auth)) {
		t.Fatal("empty refresh")
	}
}

func TestDefaultSlotName(t *testing.T) {
	id := InspectAuth(FakeAuth("a@x.com", "u1", "plus", "ws", ""))
	if DefaultSlotName(id, map[string]bool{}) != "plus" {
		t.Fatal("want plus")
	}
	if DefaultSlotName(id, map[string]bool{"plus": true}) != "plus-a" {
		t.Fatal(DefaultSlotName(id, map[string]bool{"plus": true}))
	}
}

func TestValidSlotNameRejectsPath(t *testing.T) {
	if ValidSlotName("/tmp/keep-me") {
		t.Fatal("path accepted")
	}
	if ValidSlotName("..") {
		t.Fatal("dotdot accepted")
	}
	if !ValidSlotName("biz-1") {
		t.Fatal("biz-1 rejected")
	}
}
