package gpa

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode"

	"golang.org/x/term"
)

func isTTY(f *os.File) bool {
	return term.IsTerminal(int(f.Fd()))
}

func printMenu(w io.Writer, store *Store, selected int, last Result) {
	payload := StatusPayload(store)
	accounts, _ := payload["accounts"].([]map[string]any)
	if accounts == nil {
		if raw, ok := payload["accounts"].([]any); ok {
			for _, item := range raw {
				if m := asMap(item); m != nil {
					accounts = append(accounts, m)
				}
			}
		}
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "账号                   套餐        使用位置")
	for i, acct := range accounts {
		mark := " "
		marks, _ := acct["marks"].([]any)
		for _, m := range marks {
			if asString(m) == "live" || asString(m) == "current" {
				mark = "●"
			}
		}
		if selected == i {
			mark = ">"
		}
		usedLabels := asStringSlice(acct["used_on"])
		where := strings.Join(usedLabels, " · ")
		fmt.Fprintf(w, "%s %d. %-20s %-11s %s\n", mark, i+1, asString(acct["name"]), asString(acct["plan_label"]), where)
	}
	fmt.Fprintln(w)

	if last.Status != "" {
		fmt.Fprintf(w, "上次结果：%s", last.Status)
		if last.Error != "" {
			fmt.Fprintf(w, "  %s", last.Error)
		}
		if last.OperationID != "" {
			fmt.Fprintf(w, "  操作 %s", last.OperationID)
		}
		fmt.Fprintln(w)
		for _, c := range last.Clients {
			fmt.Fprintf(w, "  %s  %s  %s\n", c.Label, c.Status, c.Detail)
		}
	}
	fmt.Fprintln(w, "↑↓ 选择账号；或输入编号/完整名称，Enter 确认")
	if last.Status == "pending" {
		if pendingNeedsCLI(last) {
			fmt.Fprintln(w, "请先关闭对应 Codex；R 重试   N 稍后（输入后回车）")
		} else {
			fmt.Fprintln(w, "Y 确认重启 App   N 稍后（输入后回车）")
		}
	}
	fmt.Fprintln(w, "/a 添加账号   /d 诊断   /t 切换范围   /q 退出（输入后回车）")
}

func RunMenu(store *Store, in io.Reader, out io.Writer) int {
	if f, ok := in.(*os.File); ok && !isTTY(f) {
		return printNoTTY(store, out)
	}
	if f, ok := out.(*os.File); ok && !isTTY(f) {
		return printNoTTY(store, out)
	}
	if f, ok := in.(*os.File); ok && isTTY(f) {
		fd := int(f.Fd())
		if output, ok := out.(*os.File); ok {
			restore, err := prepareTerminal(output)
			if err != nil {
				return menuLine(store, in, out)
			}
			defer restore()
		}
		old, err := term.MakeRaw(fd)
		if err == nil {
			defer term.Restore(fd, old)
			fmt.Fprint(out, "\x1b[?1049h")
			defer fmt.Fprint(out, "\x1b[?1049l")
			return menuLoop(store, in, crlfWriter{out}, true, func(run func()) {
				term.Restore(fd, old)
				run()
				_, _ = term.MakeRaw(fd)
			})
		}
	}
	return menuLine(store, in, out)
}

type crlfWriter struct{ io.Writer }

func (w crlfWriter) Write(p []byte) (int, error) {
	_, err := io.WriteString(w.Writer, strings.ReplaceAll(string(p), "\n", "\r\n"))
	if err != nil {
		return 0, err
	}
	return len(p), nil
}

func printNoTTY(store *Store, out io.Writer) int {
	payload := StatusPayload(store)
	fmt.Fprintln(out, "gpa: 无交互终端，未打开菜单。")
	printStatusHuman(payload, out)
	fmt.Fprintln(out, "人类: gpa ui --new-window")
	fmt.Fprintln(out, "Agent: gpa status --json   或   gpa use NAME --json")
	return 0
}

func joinAny(v any) string {
	switch t := v.(type) {
	case []string:
		return strings.Join(t, ", ")
	case []any:
		var s []string
		for _, x := range t {
			s = append(s, asString(x))
		}
		return strings.Join(s, ", ")
	}
	return asString(v)
}

// Commands are submitted with Enter so account names such as plus, demo and
// admin are never interpreted as single-letter shortcuts. /name always means a command.
func menuLine(store *Store, in io.Reader, out io.Writer) int {
	return menuLoop(store, in, out, false, func(run func()) { run() })
}

func pendingNeedsCLI(r Result) bool {
	for _, c := range r.Clients {
		if c.Kind == "cli" && c.Process != ProcNone && c.Process != ProcIdle {
			return true
		}
	}
	return false
}

func menuLoop(store *Store, in io.Reader, out io.Writer, raw bool, cooked func(func())) int {
	reader := bufio.NewReader(in)
	selected, scope := 0, 0
	targets, labels := []string{"all"}, []string{"全部本地客户端"}
	for _, c := range store.LoadClients() {
		targets = append(targets, c.ID)
		labels = append(labels, c.Label)
	}
	var last Result
	var notice string
	var input []rune
	mode := "menu"
	for {
		if raw {
			fmt.Fprint(out, "\x1b[H\x1b[2J")
		}
		names := store.Names()
		if selected >= len(names) {
			selected = 0
		}
		if len(names) == 0 {
			fmt.Fprintln(out, "欢迎使用 GPA：还没有保存的账号。")
			fmt.Fprintln(out, "[1] 导入已有账号    [2] 添加账号    /q 退出（输入后回车）")
		} else {
			printMenu(out, store, selected, last)
			fmt.Fprintln(out, "切换范围："+labels[scope])
			_, shared, _ := ResolveTargets(store.LoadClients(), targets[scope])
			for _, note := range shared {
				fmt.Fprintln(out, note)
			}
		}
		if notice != "" {
			fmt.Fprintln(out, notice)
		}
		prompt := "编号/名称/命令："
		if mode == "add" {
			prompt = "新账号别名（空输入取消）："
		}
		fmt.Fprint(out, prompt+string(input))
		var line string
		if raw {
			ch, _, err := reader.ReadRune()
			if err != nil || ch == 3 || ch == 4 {
				return 0
			}
			switch ch {
			case 27:
				// Escape sequences may arrive one byte at a time. Consume the entire
				// sequence instead of dropping the remainder of a batched read.
				next, _, err := reader.ReadRune()
				if err != nil {
					return 0
				}
				if next == '[' || next == 'O' {
					for {
						key, _, err := reader.ReadRune()
						if err != nil {
							return 0
						}
						if key >= '@' && key <= '~' {
							if mode == "menu" {
								if key == 'A' && selected > 0 {
									selected--
								}
								if key == 'B' && selected+1 < len(names) {
									selected++
								}
								if key == 'A' || key == 'B' {
									input = nil
								}
							}
							break
						}
					}
				}
				continue
			case 127, 8:
				if len(input) > 0 {
					input = input[:len(input)-1]
				}
				continue
			case 21:
				input = nil
				continue
			case '\r', '\n':
				line = strings.TrimSpace(string(input))
				input = nil
			default:
				if unicode.IsPrint(ch) {
					input = append(input, ch)
				}
				continue
			}
			fmt.Fprintln(out)
		} else {
			value, err := reader.ReadString('\n')
			if err != nil && value == "" {
				return 0
			}
			line = strings.TrimSpace(value)
		}
		if mode == "add" {
			mode = "menu"
			if line == "" {
				notice = "已取消添加账号"
				continue
			}
			cooked(func() {
				result, err := LoginAccount(store, line, false, nil)
				if err != nil {
					notice = "添加失败：" + err.Error()
				} else {
					notice = "已添加账号：" + asString(result["slot"])
				}
			})
			continue
		}
		// Exact account aliases take precedence over unprefixed shortcuts.
		command := strings.ToLower(line)
		for _, name := range names {
			if line == name {
				command = ""
				break
			}
		}
		command = strings.TrimPrefix(command, "/")
		switch command {
		case "q":
			return 0
		case "a":
			mode = "add"
			notice = ""
			continue
		case "d":
			var b strings.Builder
			printDoctorHuman(DoctorPayload(store), &b)
			notice = b.String()
			continue
		case "t":
			scope = (scope + 1) % len(targets)
			notice = "已选择范围：" + labels[scope]
			continue
		case "n":
			if last.Status == "pending" {
				notice = "已暂缓，操作 " + last.OperationID + " 已保留，可稍后执行 gpa operation apply " + last.OperationID
				last = Result{}
			} else {
				notice = "没有待处理操作"
			}
			continue
		case "y", "r":
			if last.Status != "pending" || last.OperationID == "" {
				notice = "没有待处理操作"
				continue
			}
			if command == "y" && pendingNeedsCLI(last) {
				notice = "请先关闭对应 Codex，然后输入 R 回车重试"
				continue
			}
			last = ApplyOperation(store, last.OperationID, command == "y", true)
			notice = ""
			continue
		}
		if len(names) == 0 {
			switch line {
			case "2":
				mode = "add"
				notice = ""
			case "1":
				legacy := DiscoverLegacy()
				if len(legacy) == 0 {
					notice = "没有可导入的账号"
					continue
				}
				lock, err := AcquireLock(store.LockPath())
				if err != nil {
					notice = err.Error()
					continue
				}
				result, err := MigrateFrom(store, asString(legacy[0]["path"]), false)
				lock.Release()
				if err != nil {
					notice = err.Error()
				} else {
					notice = "已导入：" + joinAny(result["imported"])
				}
			default:
				notice = "请输入 1、2 或 /q"
			}
			continue
		}
		name := ""
		var err error
		if line == "" {
			name = names[selected]
		} else {
			name, err = pickName(store, line)
		}
		if err != nil {
			notice = err.Error()
			continue
		}
		lock, err := AcquireLock(store.LockPath())
		if err != nil {
			notice = err.Error()
			continue
		}
		last = UseAccount(store, name, targets[scope], false, false, false, true)
		lock.Release()
		notice = ""
	}
}

func pickName(store *Store, raw string) (string, error) {
	names := store.Names()
	if raw == "" {
		return "", fail("cancelled")
	}
	if len(raw) > 0 && raw[0] >= '1' && raw[0] <= '9' && len(raw) < 4 {
		n := 0
		for _, ch := range raw {
			if ch < '0' || ch > '9' {
				n = -1
				break
			}
			n = n*10 + int(ch-'0')
		}
		if n >= 1 && n <= len(names) {
			return names[n-1], nil
		}
	}
	for _, n := range names {
		if n == raw {
			return n, nil
		}
	}
	return "", fail("no slot " + raw)
}
