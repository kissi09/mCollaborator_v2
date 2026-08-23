package main

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"io"
	"strings"
	"testing"
)

// closureConfig is one engagement whose findings span two areas and three
// severities, with screenshots on some findings and not others.
func closureConfig(t *testing.T) ReportConfig {
	t.Helper()
	shot, err := base64.StdEncoding.DecodeString(
		strings.TrimPrefix(tinyPNGDataURI(), "data:image/png;base64,"))
	if err != nil {
		t.Fatal(err)
	}
	config := sampleConfig()
	config.CompanyName = "Northwind Bank"
	config.ReportDate = "23rd August 2026"
	config.Areas = []ReportArea{{Code: "IPT", Scope: "10.0.0.0/24"}, {Code: "WPT", Scope: "portal"}}
	config.Findings = []ReportFinding{
		{Title: "Redis server unprotected", Severity: "critical", Description: "No authentication on Redis.",
			Impact: "Full read access to the cache.", Recommendation: "Enable requirepass.",
			RecommendationHeader: "Enable Redis authentication", Area: "IPT", AffectedSystem: "10.0.0.12 (6379)",
			POCImages: []POCImage{{Data: shot, Filename: "redis.png"}}},
		{Title: "SQL injection in login", Severity: "critical", Description: "Unparameterised query.",
			Recommendation: "Parameterise queries.", RecommendationHeader: "Parameterise queries",
			Area: "WPT", AffectedSystem: "portal.test",
			POCImages: []POCImage{{Data: shot, Filename: "a.png"}, {Data: shot, Filename: "b.png"}}},
		{Title: "Verbose server banner", Severity: "low", Description: "Version disclosed.",
			Recommendation: "Suppress banners.", RecommendationHeader: "Suppress banners",
			Area: "WPT", AffectedSystem: "portal.test"},
	}
	return config
}

// deckParts unzips a generated deck.
func deckParts(t *testing.T, deck []byte) map[string]string {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(deck), int64(len(deck)))
	if err != nil {
		t.Fatalf("generated deck is not a valid package: %v", err)
	}
	out := map[string]string{}
	seen := map[string]bool{}
	for _, f := range zr.File {
		if seen[f.Name] {
			t.Errorf("part %s appears twice in the package", f.Name)
		}
		seen[f.Name] = true
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		b, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatal(err)
		}
		out[f.Name] = string(b)
	}
	return out
}

// TestClosureDeckStructure pins what the generated package has to agree on. A
// slide that exists as a part but is missing from any one of the slide list,
// the presentation relationships or the content types opens as a corrupt file.
func TestClosureDeckStructure(t *testing.T) {
	deck, _, err := buildClosureDeck(closureConfig(t))
	if err != nil {
		t.Fatalf("buildClosureDeck: %v", err)
	}
	parts := deckParts(t, deck)

	pres := parts["ppt/presentation.xml"]
	rels := parts["ppt/_rels/presentation.xml.rels"]
	types := parts["[Content_Types].xml"]

	var slideParts []string
	for name := range parts {
		if strings.HasPrefix(name, "ppt/slides/slide") && strings.HasSuffix(name, ".xml") {
			slideParts = append(slideParts, name)
		}
	}
	if len(slideParts) == 0 {
		t.Fatal("the deck has no slides")
	}
	for _, name := range slideParts {
		target := strings.TrimPrefix(name, "ppt/")
		if !strings.Contains(rels, `Target="`+target+`"`) {
			t.Errorf("%s has no relationship from the presentation", name)
		}
		if !strings.Contains(types, `PartName="/`+name+`"`) {
			t.Errorf("%s is not declared in [Content_Types].xml", name)
		}
	}
	// Every relationship the slide list names must still exist.
	for _, m := range sldIdEntryRe.FindAllStringSubmatch(pres, -1) {
		if !strings.Contains(rels, `Id="`+m[1]+`"`) {
			t.Errorf("the slide list points at %s, which has no relationship", m[1])
		}
	}
	// The template's own repeated slides must be gone: they hold placeholders.
	for _, gone := range []string{issuesTemplatePart, scenarioTemplatePart} {
		if _, still := parts[gone]; still {
			t.Errorf("%s is still in the deck; it is a template, not a slide", gone)
		}
	}
}

// TestClosureDeckUsesTheEngagement checks the deck says what the engagement
// says, and that no placeholder survived into a delivered file.
func TestClosureDeckUsesTheEngagement(t *testing.T) {
	deck, notes, err := buildClosureDeck(closureConfig(t))
	if err != nil {
		t.Fatalf("buildClosureDeck: %v", err)
	}
	parts := deckParts(t, deck)

	var text strings.Builder
	for name, body := range parts {
		if strings.HasPrefix(name, "ppt/slides/slide") {
			text.WriteString(body)
		}
	}
	all := text.String()

	for _, want := range []string{"Northwind Bank", "Redis server unprotected", "SQL injection in login", "10.0.0.12 (6379)"} {
		if !strings.Contains(all, want) {
			t.Errorf("the deck never mentions %q", want)
		}
	}
	// A placeholder reaching a client is the failure this whole path exists to
	// avoid, so none may survive.
	for _, leftover := range []string{"[Company Name]", "[Issue]", "[Rating]", "[Recommendation]", "[Vulnerability ID]", "[Affected Host]", "[Finding]", "[Scope]"} {
		if strings.Contains(all, leftover) {
			t.Errorf("placeholder %s survived into the generated deck", leftover)
		}
	}
	// The finding with no evidence is reported rather than silently skipped.
	if len(notes.FindingsWithoutProof) != 1 || notes.FindingsWithoutProof[0] != "Verbose server banner" {
		t.Errorf("findings without proof = %v, want just the banner finding", notes.FindingsWithoutProof)
	}
}

// TestClosureDeckEmbedsEveryScreenshot pins the image path: one scenario slide
// per screenshot, each pointing at its own media part.
func TestClosureDeckEmbedsEveryScreenshot(t *testing.T) {
	deck, _, err := buildClosureDeck(closureConfig(t))
	if err != nil {
		t.Fatalf("buildClosureDeck: %v", err)
	}
	parts := deckParts(t, deck)

	media := 0
	for name := range parts {
		if strings.HasPrefix(name, "ppt/media/poc") {
			media++
		}
	}
	// Three screenshots across the config: one on Redis, two on the injection.
	if media != 3 {
		t.Errorf("embedded %d screenshots, want 3", media)
	}

	scenarios := 0
	for name, body := range parts {
		if !strings.HasPrefix(name, "ppt/slides/slide") || !strings.Contains(body, "Vulnerability Scenario") {
			continue
		}
		scenarios++
		rels := parts[relsNameFor(name)]
		if !strings.Contains(rels, "../media/poc") {
			t.Errorf("%s shows a scenario but references no screenshot", name)
		}
	}
	if scenarios != 3 {
		t.Errorf("produced %d scenario slides, want one per screenshot (3)", scenarios)
	}
}

// TestClosureDeckGroupsIssuesByAreaAndSeverity guards the slide titles. Grouping
// purely by count titles a slide "IPT/WPT Issues - Critical/Low/High Level",
// which names everything and says nothing.
func TestClosureDeckGroupsIssuesByAreaAndSeverity(t *testing.T) {
	groups := chunkFindings(buildNumberedFindings(closureConfig(t)), issuesPerSlide)
	if len(groups) < 2 {
		t.Fatalf("expected the findings to split across slides, got %d group(s)", len(groups))
	}
	for i, g := range groups {
		title := issuesTitle(g, i+1, len(groups))
		if strings.Count(title, "/") > 1 {
			t.Errorf("slide %d is titled %q; a slide should hold one area and one severity", i+1, title)
		}
	}
}
