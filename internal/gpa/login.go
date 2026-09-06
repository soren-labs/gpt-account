package gpa

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
)

func LoginAccount(store *Store, name string, force bool, runner func(argv []string, env []string, cwd string) int) (map[string]any, error) {
	if !ValidSlotName(name) {
		return nil, fail("invalid name " + strconv.Quote(name) + "; use letters, digits, . _ -")
	}
	inboxRoot := filepath.Join(store.Root, "_inbox")
	if err := os.MkdirAll(inboxRoot, 0o700); err != nil {
		return nil, fail(err.Error())
	}
	inbox, err := os.MkdirTemp(inboxRoot, name+"-")
	if err != nil {
		return nil, fail(err.Error())
	}
	defer os.RemoveAll(inbox)

	var env []string
	for _, e := range os.Environ() {
		if len(e) >= 15 && e[:15] == "OPENAI_API_KEY=" {
			continue
		}
		if len(e) >= 11 && e[:11] == "CODEX_HOME=" {
			continue
		}
		env = append(env, e)
	}
	env = append(env, "CODEX_HOME="+inbox)

	codex := "codex"
	if runner == nil {
		if p, err := exec.LookPath("codex"); err == nil {
			codex = p
		} else {
			return nil, fail("codex CLI not on PATH")
		}
	}
	argv := []string{codex, "login", "--device-auth"}
	code := 1
	if runner != nil {
		code = runner(argv, env, inbox)
	} else {
		cmd := exec.Command(argv[0], argv[1:]...)
		cmd.Env = env
		cmd.Dir = inbox
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			if ee, ok := err.(*exec.ExitError); ok {
				code = ee.ExitCode()
			} else {
				return nil, fail("login failed; live App login is unchanged (" + err.Error() + ")")
			}
		} else {
			code = 0
		}
	}
	captured := filepath.Join(inbox, "auth.json")
	if code != 0 || !exists(captured) {
		return nil, fail("login failed; live App login is unchanged (exit " + strconv.Itoa(code) + ")")
	}
	auth, err := readJSON(captured)
	if err != nil {
		return nil, fail(err.Error())
	}
	id := InspectAuth(auth)
	if !IsChatGPTBundle(id) {
		return nil, fail("isolated login did not produce a ChatGPT token bundle")
	}
	if existing := store.FindByIdentity(id); existing != "" && existing != name {
		return nil, fail("that login is already saved as " + existing)
	}
	dest, _ := store.SlotDir(name)
	slotAuth := filepath.Join(dest, "auth.json")
	if exists(slotAuth) {
		old, _ := readJSON(slotAuth)
		oldID := InspectAuth(old)
		if !SameSeat(oldID, id) && !force {
			who := oldID.Email
			if who == "" {
				who = oldID.UserID
			}
			return nil, fail("slot " + name + " already holds " + who + "; use another name or --force")
		}
	}
	meta, err := store.Put(name, auth, captured, true)
	if err != nil {
		return nil, err
	}
	store.AppendLog("login", map[string]any{"slot": name, "email": id.Email, "plan": id.Plan})
	return map[string]any{
		"slot":          name,
		"email":         id.Email,
		"plan":          id.Plan,
		"user_id":       id.UserID,
		"workspace_id":  id.WorkspaceID,
		"cred_version":  credVersion(meta),
	}, nil
}

