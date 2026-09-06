package gpa

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

func Main(argv []string) int {
	return Run(argv, os.Stdin, os.Stdout, os.Stderr)
}

func Run(argv []string, stdin io.Reader, stdout, stderr io.Writer) int {
	asJSON, storePath, args, err := peelGlobal(argv)
	if err != nil {
		return emitErr(asJSON, stdout, stderr, err)
	}

	cmd := ""
	if len(args) > 0 {
		cmd = args[0]
		args = args[1:]
	}

	if err := validateArgs(cmd, args); err != nil {
		return emitErr(asJSON, stdout, stderr, err)
	}
	cfg := LoadConfig(storePath)
	store := OpenStore(cfg)
	if err := store.Ensure(); err != nil {
		return emitErr(asJSON, stdout, stderr, err)
	}

	printJSON := func(v any) {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(v)
	}

	switch cmd {
	case "", "ui":
		return cmdUI(store, args, asJSON, stdin, stdout, stderr)
	case "list":
		payload := StatusPayload(store)
		if asJSON {
			printJSON(payload)
		} else {
			printListHuman(payload, stdout)
		}
		return 0
	case "status":
		payload := StatusPayload(store)
		if asJSON {
			printJSON(payload)
		} else {
			printStatusHuman(payload, stdout)
		}
		return 0
	case "doctor":
		payload := DoctorPayload(store)
		if asJSON {
			printJSON(payload)
		} else {
			printDoctorHuman(payload, stdout)
		}
		return 0
	case "migrate":
		return cmdMigrate(store, args, asJSON, stdout, stderr)
	case "login", "capture":
		return cmdLogin(store, args, asJSON, stdout, stderr)
	case "use", "switch":
		return cmdUse(store, args, asJSON, stdin, stdout, stderr)
	case "save":
		return cmdSave(store, args, asJSON, stdout, stderr)
	case "restore":
		return cmdRestore(store, args, asJSON, stdin, stdout, stderr)
	case "install":
		return cmdInstall(store, args, asJSON, stdout, stderr)
	case "operation":
		return cmdOperation(store, args, asJSON, stdin, stdout, stderr)
	case "codex":
		return cmdCodex(store, args, stdout, stderr)
	case "version", "--version":
		fmt.Fprintln(stdout, "gpa 0.2.0")
		return 0
	case "help", "-h", "--help":
		printHelp(stdout)
		return 0
	default:
		fmt.Fprintf(stderr, "gpa: unknown command %s\n", cmd)
		printHelp(stdout)
		return 1
	}
}

func peelGlobal(argv []string) (asJSON bool, store string, rest []string, err error) {
	for _, a := range argv {
		if a == "--json" {
			asJSON = true
		}
	}
	for i := 0; i < len(argv); i++ {
		a := argv[i]
		switch {
		case a == "--json":
			asJSON = true
		case a == "--store":
			if i+1 >= len(argv) || strings.HasPrefix(argv[i+1], "-") || argv[i+1] == "" {
				err = fail("--store requires a value")
				return
			}
			i++
			store = argv[i]
		case strings.HasPrefix(a, "--store="):
			store = strings.TrimPrefix(a, "--store=")
			if store == "" {
				err = fail("--store requires a value")
				return
			}
		case a == "--version":
			rest = append(rest, "version")
		default:
			rest = append(rest, a)
		}
	}
	return
}

func emitErr(asJSON bool, stdout, stderr io.Writer, err error) int {
	code := 1
	if e, ok := err.(*Error); ok {
		code = e.Code
	}
	if asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(map[string]any{"status": "failed", "error": err.Error()})
	} else {
		fmt.Fprintln(stderr, "gpa:", err)
	}
	return code
}

func cmdUI(store *Store, args []string, asJSON bool, stdin io.Reader, stdout, stderr io.Writer) int {
	newWindow := false
	for _, a := range args {
		if a == "--new-window" {
			newWindow = true
		}
	}
	if newWindow {
		if err := openNewWindow(); err != nil {
			return emitErr(asJSON, stdout, stderr, err)
		}
		if asJSON {
			enc := json.NewEncoder(stdout)
			enc.SetIndent("", "  ")
			_ = enc.Encode(map[string]any{"status": "completed", "next": "independent GPA window"})
		} else {
			fmt.Fprintln(stdout, "opened GPA in a new window")
		}
		return 0
	}
	if asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(StatusPayload(store))
		return 0
	}
	return RunMenu(store, stdin, stdout)
}

func cmdMigrate(store *Store, args []string, asJSON bool, stdout, stderr io.Writer) int {
	source := ""
	force := false
	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--force":
			force = true
		case args[i] == "--from" && i+1 < len(args):
			i++
			source = args[i]
		case strings.HasPrefix(args[i], "--from="):
			source = strings.TrimPrefix(args[i], "--from=")
		}
	}
	if source == "" {
		legacy := DiscoverLegacy()
		if len(legacy) == 0 {
			return emitErr(asJSON, stdout, stderr, fail("no legacy store found; pass --from PATH"))
		}
		source = asString(legacy[0]["path"])
	}
	lock, err := AcquireLock(store.LockPath())
	if err != nil {
		return emitErr(asJSON, stdout, stderr, err)
	}
	defer lock.Release()
	res, err := MigrateFrom(store, source, force)
	if err != nil {
		return emitErr(asJSON, stdout, stderr, err)
	}
	if asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(res)
	} else {
		fmt.Fprintf(stdout, "imported %s\n", joinAny(res["imported"]))
		if s := joinAny(res["skipped"]); s != "" && s != "-" {
			fmt.Fprintf(stdout, "skipped %s\n", s)
		}
		fmt.Fprintf(stdout, "current %s\n", dash(asString(res["current"])))
	}
	return 0
}

func cmdLogin(store *Store, args []string, asJSON bool, stdout, stderr io.Writer) int {
	force := false
	var name string
	for _, a := range args {
		if a == "--force" {
			force = true
			continue
		}
		if !strings.HasPrefix(a, "-") && name == "" {
			name = a
		}
	}
	if name == "" {
		return emitErr(asJSON, stdout, stderr, fail("gpa login NAME"))
	}
	fmt.Fprintln(stderr, "starting isolated login for slot", name)
	fmt.Fprintln(stderr, "use a private/incognito window; do not click Logout in ChatGPT.exe")
	res, err := LoginAccount(store, name, force, nil)
	if err != nil {
		return emitErr(asJSON, stdout, stderr, err)
	}
	if asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(res)
	} else {
		fmt.Fprintf(stdout, "captured %s  %s  %s\n", asString(res["slot"]), asString(res["email"]), asString(res["plan"]))
		fmt.Fprintf(stdout, "switch later with: gpa use %s\n", asString(res["slot"]))
	}
	return 0
}

func parseUseFlags(args []string) (name, target string, force, open, dry bool, restErr error) {
	target = "all"
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--force":
			force = true
		case a == "--open":
			open = true
		case a == "--dry-run":
			dry = true
		case a == "--no-restart":
			// kept for old habit: do not start app
		case a == "--target":
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "-") {
				restErr = fail("--target requires a value")
				return
			}
			i++
			target = args[i]
		case strings.HasPrefix(a, "--target="):
			target = strings.TrimPrefix(a, "--target=")
		case !strings.HasPrefix(a, "-") && name == "":
			name = a
		default:
			restErr = fail("unexpected argument " + a)
			return
		}
	}
	if target == "" {
		restErr = fail("--target requires a value")
		return
	}
	if name == "" {
		restErr = fail("gpa use NAME")
	}
	return
}

func cmdUse(store *Store, args []string, asJSON bool, stdin io.Reader, stdout, stderr io.Writer) int {
	name, target, force, open, dry, err := parseUseFlags(args)
	if err != nil {
		return emitErr(asJSON, stdout, stderr, err)
	}
	interactive := false
	if f, ok := stdin.(*os.File); ok && isTTY(f) {
		if o, ok := stdout.(*os.File); ok && isTTY(o) {
			interactive = true
		}
	}
	lock, err := AcquireLock(store.LockPath())
	if err != nil {
		return emitErr(asJSON, stdout, stderr, err)
	}
	defer lock.Release()
	res := UseAccount(store, name, target, force, open, dry, interactive)
	if asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(res)
	} else {
		printResultHuman(res, stdout)
	}
	return res.ExitCode()
}

func cmdSave(store *Store, args []string, asJSON bool, stdout, stderr io.Writer) int {
	force := false
	name := ""
	for _, a := range args {
		if a == "--force" {
			force = true
			continue
		}
		if !strings.HasPrefix(a, "-") && name == "" {
			name = a
		}
	}
	lock, err := AcquireLock(store.LockPath())
	if err != nil {
		return emitErr(asJSON, stdout, stderr, err)
	}
	defer lock.Release()
	meta, err := SaveLive(store, name, force)
	if err != nil {
		return emitErr(asJSON, stdout, stderr, err)
	}
	if asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(meta)
	} else {
		fmt.Fprintf(stdout, "saved %s  %s  %s\n", asString(meta["name"]), asString(meta["email"]), asString(meta["plan"]))
	}
	return 0
}

func cmdRestore(store *Store, args []string, asJSON bool, stdin io.Reader, stdout, stderr io.Writer) int {
	name := store.Current()
	if name == "" {
		return emitErr(asJSON, stdout, stderr, fail("no current slot; gpa save or gpa migrate first"))
	}
	return cmdUse(store, append([]string{name}, args...), asJSON, stdin, stdout, stderr)
}

func cmdOperation(store *Store, args []string, asJSON bool, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		return emitErr(asJSON, stdout, stderr, fail("gpa operation status|apply|list ID"))
	}
	switch args[0] {
	case "list":
		ops := store.ListOperations()
		if asJSON {
			enc := json.NewEncoder(stdout)
			enc.SetIndent("", "  ")
			_ = enc.Encode(ops)
			return 0
		}
		if len(ops) == 0 {
			fmt.Fprintln(stdout, "no operations")
			return 0
		}
		for _, op := range ops {
			fmt.Fprintf(stdout, "%s  %s  %s  %s  %s\n", op.ID, op.Status, op.Kind, op.Account, op.Reason)
		}
		return 0
	case "status":
		if len(args) < 2 {
			return emitErr(asJSON, stdout, stderr, fail("gpa operation status ID"))
		}
		op, err := store.GetOperation(args[1])
		if err != nil {
			return emitErr(asJSON, stdout, stderr, err)
		}
		if asJSON {
			enc := json.NewEncoder(stdout)
			enc.SetIndent("", "  ")
			_ = enc.Encode(op)
		} else {
			fmt.Fprintf(stdout, "%s  %s  %s  %s\n", op.ID, op.Status, op.Account, op.Reason)
			if op.NextHint() != "" {
				fmt.Fprintln(stdout, "next:", op.NextHint())
			}
		}
		return 0
	case "apply":
		if len(args) < 2 {
			return emitErr(asJSON, stdout, stderr, fail("gpa operation apply ID"))
		}
		force := false
		for _, a := range args[2:] {
			if a == "--force" {
				force = true
			}
		}
		for _, a := range args[2:] {
			if a != "--force" {
				return emitErr(asJSON, stdout, stderr, fail("unexpected argument "+a))
			}
		}
		res := ApplyOperation(store, args[1], force, false)

		if asJSON {
			enc := json.NewEncoder(stdout)
			enc.SetIndent("", "  ")
			_ = enc.Encode(res)
		} else {
			printResultHuman(res, stdout)
		}
		return res.ExitCode()
	default:
		return emitErr(asJSON, stdout, stderr, fail("gpa operation status|apply|list"))
	}
}

func (op Operation) NextHint() string {
	if op.Status == "pending" {
		return "gpa operation apply " + op.ID + " --force"
	}
	return ""
}

func orDefault(s, d string) string {
	if s == "" {
		return d
	}
	return s
}

func printHelp(w io.Writer) {
	fmt.Fprint(w, `gpa — 本地切换 ChatGPT / Codex 官方登录

用法:
  gpa                     交互菜单（无 TTY 时打印状态）
  gpa ui --new-window     给人类开独立窗口
  gpa list
  gpa status
  gpa doctor
  gpa use NAME [--target app|cli|wsl:DISTRO] [--force] [--open] [--dry-run]
  gpa login NAME
  gpa migrate [--from PATH]
  gpa save [NAME]
  gpa operation status ID
  gpa operation apply ID [--force]
  gpa codex [--account NAME]
  gpa install

Agent:
  gpa status --json
  gpa use plus --dry-run --json
  gpa use plus --json
`)
}

// Validate the complete command before creating a store or touching any client.
func validateArgs(cmd string, args []string) error {
	if cmd == "codex" {
		return nil
	} // Codex owns its own flags.
	flags := map[string]bool{} // true means a value is required
	min, max := 0, 0
	switch cmd {
	case "", "ui":
		flags["--new-window"] = false
	case "list", "status", "doctor", "version", "help", "-h", "--help":
	case "use", "switch":
		_, _, _, _, _, err := parseUseFlags(args)
		return err
	case "restore":
		_, _, _, _, _, err := parseUseFlags(append([]string{"current"}, args...))
		return err
	case "login", "capture":
		min, max = 1, 1
		flags["--force"] = false
	case "save":
		max = 1
		flags["--force"] = false
	case "migrate":
		flags["--from"] = true
		flags["--force"] = false
	case "install":
		flags["--windows-bin"] = true
		flags["--linux-bin"] = true
	case "operation":
		if len(args) == 0 {
			return fail("gpa operation status|apply|list ID")
		}
		switch args[0] {
		case "list":
		case "status":
			min, max = 1, 1
		case "apply":
			min, max = 1, 1
			flags["--force"] = false
		default:
			return fail("unknown operation " + args[0])
		}
		args = args[1:]
	default:
		return fail("unknown command " + cmd)
	}
	count := 0
	for i := 0; i < len(args); i++ {
		a := args[i]
		if !strings.HasPrefix(a, "-") {
			count++
			continue
		}
		key, value, eq := strings.Cut(a, "=")
		needsValue, ok := flags[key]
		if !ok {
			return fail("unknown option " + key)
		}
		if !needsValue {
			if eq {
				return fail("unexpected value for " + key)
			}
			continue
		}
		if !eq {
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "-") {
				return fail(key + " requires a value")
			}
			i++
			value = args[i]
		}
		if value == "" {
			return fail(key + " requires a value")
		}
	}
	if count < min || count > max {
		return fail("invalid arguments for " + cmd + "; see gpa help")
	}
	return nil
}
