package gpa

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMenuNameAndBufferedKeys(t *testing.T) {
	for _, tc := range []struct{ name, keys, want string }{
		{"name", "plus\r/q\r", "plus"},
		{"arrows", "\x1b[B\r/q\r", "biz2"},
		{"backspace", "biz9\x7f1\r/q\r", "biz1"},
		{"invalid", "missing\r/q\r", "plus"},
		{"number", "2\r/q\r", "biz2"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := testEnv(t)
			var out bytes.Buffer
			menuLoop(s, strings.NewReader(tc.keys), &out, true, func(f func()) { f() })
			if s.Current() != tc.want {
				t.Fatalf("got %s want %s", s.Current(), tc.want)
			}
		})
	}
}

func TestMenuPendingRetainsOriginalOperation(t *testing.T) {
	s := testEnv(t)
	t.Setenv("GPA_FAKE_CLI", "running")
	r := UseAccount(s, "biz1", "cli", false, false, false, true)
	if r.Status != "pending" || !pendingNeedsCLI(r) {
		t.Fatalf("%+v", r)
	}
	retried := ApplyOperation(s, r.OperationID, false, true)
	if retried.Status != "pending" || retried.OperationID != r.OperationID || len(s.ListOperations()) != 1 {
		t.Fatalf("%+v", retried)
	}
	t.Setenv("GPA_FAKE_CLI", "")
	applied := ApplyOperation(s, r.OperationID, false, true)
	op, _ := s.GetOperation(r.OperationID)
	if applied.Status != "completed" || op.Status != "applied" || len(s.ListOperations()) != 1 {
		t.Fatalf("%+v %+v", applied, op)
	}
}

func TestMenuDeferAndCLIHint(t *testing.T) {
	s := testEnv(t)
	t.Setenv("GPA_FAKE_CLI", "running")
	var out bytes.Buffer
	menuLine(s, strings.NewReader("biz1\ny\nn\ny\n/q\n"), &out)
	if strings.Contains(out.String(), "Y 确认重启 App") {
		t.Fatal("offered restart for busy CLI")
	}
	for _, message := range []string{"请先关闭对应 Codex", "已暂缓", "没有待处理操作"} {
		if !strings.Contains(out.String(), message) {
			t.Fatalf("missing %s", message)
		}
	}
	if len(s.ListOperations()) != 1 || s.Current() != "plus" {
		t.Fatal("defer/rejected confirmation changed state")
	}
}

func TestMenuAddCancelAndValidation(t *testing.T) {
	s := testEnv(t)
	var out bytes.Buffer
	menuLine(s, strings.NewReader("/a\n\n/a\n../bad\n/q\n"), &out)
	for _, message := range []string{"新账号别名", "已取消添加账号", "添加失败：invalid name"} {
		if !strings.Contains(out.String(), message) {
			t.Fatalf("missing %s", message)
		}
	}
}

func TestBadArgumentsDoNotCreateStore(t *testing.T) {
	for _, argv := range [][]string{
		{"use", "plus", "--targte", "app"}, {"use", "plus", "--target"},
		{"use", "plus", "--target="}, {"use", "plus", "--target", "--force"},
		{"use", "plus", "extra"}, {"operation", "apply", "id", "--froce"},
		{"login", "x", "--froce"}, {"save", "x", "y"}, {"status", "--unknown"},
		{"--store"}, {"migrate", "--from"},
	} {
		t.Run(strings.Join(argv, " "), func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "store")
			var out bytes.Buffer
			code := Run(append([]string{"--store", root, "--json"}, argv...), nilReader{}, &out, &out)
			if code == 0 {
				t.Fatalf("accepted %v", argv)
			}
			if _, err := os.Stat(root); !os.IsNotExist(err) {
				t.Fatal("invalid command created store")
			}
			if !strings.Contains(out.String(), `"status": "failed"`) {
				t.Fatal(out.String())
			}
		})
	}
}
