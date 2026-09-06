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
