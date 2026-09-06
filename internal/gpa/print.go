package gpa

import (
	"fmt"
	"io"
	"strings"
)

func printListHuman(payload map[string]any, w io.Writer) {
	accounts := asMaps(payload["accounts"])
	if len(accounts) == 0 {
		fmt.Fprintln(w, "no saved accounts")
		return
	}
	for _, acct := range accounts {
		marks := asStringSlice(acct["marks"])
		extra := ""
		if len(marks) > 0 {
			extra = "  [" + strings.Join(marks, ", ") + "]"
		}
		fmt.Fprintf(w, "%-16s  %-32s  %s%s\n", asString(acct["name"]), dash(asString(acct["email"])), dash(asString(acct["plan"])), extra)
	}
}

func printStatusHuman(payload map[string]any, w io.Writer) {
	lives := asMaps(payload["lives"])
	if len(lives) == 0 {
		fmt.Fprintln(w, "live: logged out")
	}
	for _, live := range lives {
		email := dash(asString(live["email"]))
		plan := dash(asString(live["plan"]))
		fmt.Fprintf(w, "%s: %s  %s\n", asString(live["label"]), email, plan)
		fmt.Fprintf(w, "path: %s\n", asString(live["path"]))
	}
	cur := asString(payload["current"])
	if cur == "" {
		cur = "-"
	}
	fmt.Fprintf(w, "store current: %s\n", cur)
	fmt.Fprintf(w, "store: %s\n", asString(payload["store"]))
	var names []string
	for _, a := range asMaps(payload["accounts"]) {
		names = append(names, asString(a["name"]))
	}
	if len(names) == 0 {
		fmt.Fprintln(w, "slots: -")
	} else {
		fmt.Fprintf(w, "slots: %s\n", strings.Join(names, ", "))
	}
}

func printDoctorHuman(payload map[string]any, w io.Writer) {
	printStatusHuman(payload, w)
	fmt.Fprintln(w)
	fmt.Fprintln(w, "客户端")
	for _, row := range asMaps(payload["client_states"]) {
		shared := asStringSlice(row["shared"])
		share := ""
		if len(shared) > 0 {
			share = "  共用: " + strings.Join(shared, ", ")
		}
		fmt.Fprintf(w, "  %s  %s  %s  %s%s\n", asString(row["label"]), asString(row["email"]), asString(row["process"]), asString(row["path"]), share)
		if d := asString(row["detail"]); d != "" {
			fmt.Fprintf(w, "    %s\n", d)
		}
	}
	sugs := asStringSlice(payload["suggestions"])
	if len(sugs) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "建议")
		for _, s := range sugs {
			fmt.Fprintf(w, "  - %s\n", s)
		}
	}
}

func printResultHuman(r Result, w io.Writer) {
	fmt.Fprintf(w, "%s  %s  %s  %s\n", r.Status, r.Account, r.Email, r.Plan)
	for _, p := range r.Written {
		fmt.Fprintln(w, "wrote", p)
	}
	if len(r.Adopted) > 0 {
		fmt.Fprintln(w, "kept rotated tokens in", strings.Join(r.Adopted, ", "))
	}
	for _, n := range r.Shared {
		fmt.Fprintln(w, "note:", n)
	}
	for _, c := range r.Clients {
		if c.Detail != "" {
			fmt.Fprintf(w, "  %s  %s  %s\n", c.Label, c.Status, c.Detail)
		}
	}
	if r.Error != "" {
		fmt.Fprintln(w, r.Error)
	}
	if r.Next != "" {
		fmt.Fprintln(w, "next:", r.Next)
	}
}

func dash(s string) string {
	if s == "" {
		return "?"
	}
	return s
}

func asMaps(v any) []map[string]any {
	switch t := v.(type) {
	case []map[string]any:
		return t
	case []any:
		var out []map[string]any
		for _, item := range t {
			if m := asMap(item); m != nil {
				out = append(out, m)
			}
		}
		return out
	}
	return nil
}

func asStringSlice(v any) []string {
	switch t := v.(type) {
	case []string:
		return t
	case []any:
		var out []string
		for _, x := range t {
			if s := asString(x); s != "" {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}
