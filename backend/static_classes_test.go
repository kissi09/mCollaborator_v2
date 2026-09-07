package main

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// A class the markup uses and the stylesheet does not define fails silently:
// the element simply gets no padding, no margin, no alignment, and the page
// looks like nobody spaced it. That is exactly what happened to the dashboard -
// mb-1, mb-3, mt-4, pb-2, p-3, items-start and text-center were all in the
// markup and none of them existed, so every project card sat flush against the
// next with its own lines jammed together.
//
// This is the check that would have caught it.

// classAttrRe pulls the value out of every class="..." in the markup.
var classAttrRe = regexp.MustCompile(`class="([^"]*)"`)

// A real class name. Anything else in a class attribute is a piece of the
// template expression that builds it - `${sev === 'critical' ? 'a' : 'b'}` -
// and is not a name to look up.
var classNameRe = regexp.MustCompile(`^[a-z][a-z0-9]*(-[a-z0-9]+)*$`)

// cssSelectorRe finds the class selectors the stylesheet defines.
var cssSelectorRe = regexp.MustCompile(`\.([a-zA-Z][\w-]*)`)

// classesUsedByJS are hooks the JavaScript looks elements up by rather than
// styles them with, so the stylesheet is not expected to carry them.
var classesUsedByJS = map[string]bool{
	"finding-card": true,
}

func TestEveryClassInTheMarkupIsStyled(t *testing.T) {
	root := filepath.Join("static")

	cssBody, err := os.ReadFile(filepath.Join(root, "css", "app.css"))
	if err != nil {
		t.Fatalf("read app.css: %v", err)
	}
	defined := map[string]bool{}
	for _, m := range cssSelectorRe.FindAllStringSubmatch(string(cssBody), -1) {
		defined[m[1]] = true
	}

	sources := []string{
		filepath.Join(root, "index.html"),
		filepath.Join(root, "js", "pages.js"),
		filepath.Join(root, "js", "state.js"),
	}

	missing := map[string][]string{}
	for _, src := range sources {
		body, err := os.ReadFile(src)
		if err != nil {
			t.Fatalf("read %s: %v", src, err)
		}
		for _, m := range classAttrRe.FindAllStringSubmatch(string(body), -1) {
			for _, name := range strings.Fields(m[1]) {
				if !classNameRe.MatchString(name) || defined[name] || classesUsedByJS[name] {
					continue
				}
				missing[name] = append(missing[name], filepath.Base(src))
			}
		}
	}

	if len(missing) == 0 {
		return
	}
	var names []string
	for name := range missing {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		t.Errorf("class %q is used in the markup but app.css never defines it, so it does nothing (%s)",
			name, strings.Join(uniqueStrings(missing[name]), ", "))
	}
}

func uniqueStrings(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

// The class guard above only sees class names written as literals. A pill whose
// class is built from data - class="status-pill ${e.status}" - is invisible to
// it, and that is exactly how a project's status renders. The New Project modal
// offered planning, in_progress and review; app.css defined only in_progress,
// so two of the three statuses a user can choose drew a pill with no
// background, no colour and no border.
func TestEveryProjectStatusHasAPill(t *testing.T) {
	pages, err := os.ReadFile(filepath.Join("static", "js", "pages.js"))
	if err != nil {
		t.Fatalf("read pages.js: %v", err)
	}
	css, err := os.ReadFile(filepath.Join("static", "css", "app.css"))
	if err != nil {
		t.Fatalf("read app.css: %v", err)
	}

	// The statuses the New Project modal actually offers.
	body := string(pages)
	start := strings.Index(body, `id="ne-status"`)
	if start < 0 {
		t.Fatal("could not find the status select in the New Project modal")
	}
	end := strings.Index(body[start:], "</select>")
	if end < 0 {
		t.Fatal("status select is not closed")
	}
	block := body[start : start+end]

	var statuses []string
	for _, part := range strings.Split(block, `value="`)[1:] {
		if q := strings.Index(part, `"`); q >= 0 {
			statuses = append(statuses, part[:q])
		}
	}
	if len(statuses) == 0 {
		t.Fatal("no statuses found in the modal")
	}

	for _, st := range statuses {
		if !strings.Contains(string(css), ".status-pill."+st+" ") {
			t.Errorf("status %q can be chosen in the New Project modal but app.css has no .status-pill.%s rule - the pill renders unstyled", st, st)
		}
	}

	// A project can also be marked completed, which the dashboard shows.
	for _, st := range []string{"completed", "closed"} {
		if !strings.Contains(string(css), ".status-pill."+st+" ") {
			t.Errorf("app.css has no .status-pill.%s rule", st)
		}
	}
}

// Severity chips on a project card are built the same way, from the finding's
// own severity, so they are equally invisible to the literal-class guard.
func TestEverySeverityHasABadge(t *testing.T) {
	css, err := os.ReadFile(filepath.Join("static", "css", "app.css"))
	if err != nil {
		t.Fatalf("read app.css: %v", err)
	}
	for _, sev := range []string{"critical", "high", "medium", "low", "info"} {
		if !strings.Contains(string(css), ".badge-severity."+sev+" ") {
			t.Errorf("app.css has no .badge-severity.%s rule - the chip renders unstyled", sev)
		}
	}
}

// A status pill is written as class="status-pill ${e.status}" and a severity
// chip as class="badge-severity ${sev}", so the bare word becomes a class in
// its own right. If the stylesheet also defines a rule for that bare word,
// every pill silently inherits it.
//
// That is not hypothetical. The finding-import board's page container was
// called .review - "display:flex; height: calc(100vh - var(--topbar-height))" -
// and a project whose status is review therefore rendered its pill as a column
// the full height of the window, which pushed the rest of the card off the
// screen. It looked like a spacing bug and was a name collision.
func TestNoBareStatusOrSeverityClassInTheStylesheet(t *testing.T) {
	css, err := os.ReadFile(filepath.Join("static", "css", "app.css"))
	if err != nil {
		t.Fatalf("read app.css: %v", err)
	}

	// Words that reach the markup as a standalone class.
	reserved := []string{
		// engagement and finding statuses
		"planning", "in_progress", "review", "completed", "closed", "open",
		"draft", "mitigated", "verified",
		// severities
		"critical", "high", "medium", "low", "info",
	}

	for _, line := range strings.Split(string(css), "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, ".") {
			continue
		}
		// The selector is everything before the declaration block.
		sel, _, found := strings.Cut(trimmed, "{")
		if !found {
			continue
		}
		sel = strings.TrimSpace(sel)
		for _, word := range reserved {
			// A rule for the bare word alone, e.g. ".review" or ".review, .x".
			for _, part := range strings.Split(sel, ",") {
				if strings.TrimSpace(part) == "."+word {
					t.Errorf("app.css defines a bare %q rule. That word is emitted as a standalone class "+
						"by a status pill or severity chip, so every one of them would inherit this rule. "+
						"Give the page class a name of its own, e.g. .import-%s", "."+word, word)
				}
			}
		}
	}
}
