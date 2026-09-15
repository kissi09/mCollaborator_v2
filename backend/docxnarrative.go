package main

// docxnarrative.go rewrites the chapter 1 prose that the template words for one
// kind of engagement. The template's introduction describes "internal systems
// and external applications" tested "onsite ... and remotely", and its summary
// praises "the external-facing assets" despite "a minor TLS configuration
// issue" - true of the engagement the template was written from, and plainly
// wrong for a configuration review or a network architecture review. Each of
// those paragraphs is rebuilt here from the areas actually selected and, for
// the posture paragraph, from how badly each area fared.
//
// Paragraphs are found by the opening words the template gives them. A template
// edited so those words change is left exactly as written rather than guessed at.

import (
	"strings"
)

// areaNarrative is how the prose refers to one assessment area.
type areaNarrative struct {
	// Assessed is what the introduction says was assessed:
	// "Cyberteq has performed a security assessment on <Company> <Assessed>".
	Assessed string
	// Activity is the area as a scope item: "Activities pertaining to <Activity>".
	Activity string
	// Where the work happens: "onsite" or "remote" for testing, "review" for an
	// area that examines material rather than attacking a live system.
	Mode string
	// Target is the area as the object of the onsite/remote sentence, or for a
	// review, what was reviewed.
	Target string
	// Subject opens the posture sentence; Plural picks "is" or "are".
	Subject string
	Plural  bool
	// Secure is the posture sentence for an area with no issue above low.
	Secure string
	// Concern is the posture sentence for an area with a high or critical issue.
	Concern string
}

var areaNarratives = map[string]areaNarrative{
	"IPT": {
		Assessed: "internal network infrastructure", Activity: "internal server infrastructure",
		Mode: "onsite", Target: "the internal assets",
		Subject: "the internal network",
		Secure:  "The internal network appears to be well secured overall, with services appropriately hardened and patched and little opportunity for an attacker with internal access to move laterally.",
		Concern: "The internal network requires attention: the weaknesses identified there would allow an attacker who gains a foothold inside the network to escalate privileges and move laterally.",
	},
	"EPT": {
		Assessed: "external infrastructure", Activity: "external infrastructure exposed to the Internet",
		Mode: "remote", Target: "the external infrastructure",
		Subject: "the external-facing assets", Plural: true,
		Secure:  "The external-facing assets appear to be well secured overall, reflecting strong attention to network hygiene and exposure control, with most services appropriately hardened and not publicly exploitable.",
		Concern: "The external-facing assets expose weaknesses that can be reached directly from the Internet, which considerably raises the likelihood of their exploitation.",
	},
	"IPTC": {
		Assessed: "cloud infrastructure", Activity: "cloud infrastructure",
		Mode: "remote", Target: "the cloud environment",
		Subject: "the cloud environment",
		Secure:  "The cloud environment appears to be well configured overall, with access to cloud resources appropriately restricted and services hardened.",
		Concern: "The cloud environment contains weaknesses in how its resources are configured and exposed that could allow unauthorised access to cloud-hosted workloads and data.",
	},
	"WPT": {
		Assessed: "web applications", Activity: "web applications",
		Mode: "remote", Target: "the web applications",
		Subject: "the web applications", Plural: true,
		Secure:  "The web applications appear to be well secured overall, with common vulnerability classes such as those in the OWASP Top 10 appropriately mitigated.",
		Concern: "The web applications contain weaknesses that could be exploited to compromise user accounts, application data or the servers behind them.",
	},
	"ASA": {
		Assessed: "APIs", Activity: "application programming interfaces (APIs)",
		Mode: "remote", Target: "the APIs",
		Subject: "the APIs", Plural: true,
		Secure:  "The APIs appear to enforce authentication, authorisation and input validation appropriately.",
		Concern: "The APIs contain weaknesses in how requests are authenticated, authorised or validated, which could expose the data and functions behind them.",
	},
	"ADT": {
		Assessed: "Active Directory environment", Activity: "the Active Directory environment",
		Mode: "onsite", Target: "the Active Directory environment",
		Subject: "the Active Directory environment",
		Secure:  "The Active Directory environment appears to be well hardened overall, with privileged accounts and delegation appropriately controlled.",
		Concern: "The Active Directory environment contains weaknesses that could allow an attacker to escalate from a standard domain account towards control of the domain.",
	},
	"WNA": {
		Assessed: "wireless networks", Activity: "wireless networks",
		Mode: "onsite", Target: "the wireless networks",
		Subject: "the wireless networks", Plural: true,
		Secure:  "The wireless networks appear to be well secured overall, with strong encryption and authentication in place and adequate separation from the corporate network.",
		Concern: "The wireless networks contain weaknesses that could allow an attacker within radio range to gain access to the network or intercept its traffic.",
	},
	"CFG": {
		Assessed: "network device configurations", Activity: "the configuration of network and security devices",
		Mode: "review", Target: "the configuration files of the in-scope network and security devices",
		Subject: "the reviewed device configurations", Plural: true,
		Secure:  "The reviewed device configurations appear to be well hardened overall, with management access, authentication and logging largely aligned with security best practice.",
		Concern: "The reviewed device configurations depart from security hardening best practice in ways that weaken the protection of device management, authentication or the visibility of activity on the devices.",
	},
	"NAR": {
		Assessed: "network architecture", Activity: "the network architecture",
		Mode: "review", Target: "the design of the network architecture",
		Subject: "the network architecture",
		Secure:  "The network architecture appears to be soundly designed overall, with segmentation, access control and resilience taken into account.",
		Concern: "The network architecture has design weaknesses that could allow a compromise in one part of the network to spread to others, or reduce the resilience of critical services.",
	},
}

// Template openings of the paragraphs rewritten here.
const (
	introOpening    = "Cyberteq has performed a security assessment on"
	scopeOpening    = "Finding security flaws in the corresponding networks"
	reconOpening    = "During an initial phase of the vulnerability assessment"
	analysisOpening = "The discovered issues were thoroughly analyzed"
	postureOpening  = "The external-facing assets appear to be well secured overall"
	closingOpening  = "While the assessment identified several commendable security practices"
)

// renderEngagementNarrative rewrites the introduction and executive summary
// paragraphs for the selected areas. [Company Name] and the assessment period
// are left as placeholders for applyScalarPlaceholders to fill.
func renderEngagementNarrative(doc string, config ReportConfig, findings []numberedFinding) string {
	var areas []string
	for _, code := range selectedAreaCodes(config) {
		if _, ok := areaNarratives[code]; ok {
			areas = append(areas, code)
		}
	}
	if len(areas) == 0 {
		return doc
	}

	worst := map[string]int{}
	for _, f := range findings {
		r := severityRank(f.Severity)
		if cur, ok := worst[f.Area.Code]; !ok || r < cur {
			worst[f.Area.Code] = r
		}
	}

	intro := introParagraph(areas)
	scope := scopeParagraph(areas)
	recon, analysis := reviewMethodParagraphs(areas)
	posture, anyConcern, anySound := postureParagraph(areas, worst)
	closing := closingParagraph(anyConcern, anySound, len(findings))
	risk := buildRiskSection(areas, findings)

	// An empty replacement for a section 1.3 paragraph removes it.
	decide := func(text string) (string, bool) {
		text = strings.TrimSpace(text)
		switch {
		case strings.HasPrefix(text, riskOverviewOpening):
			return risk.Overview, true
		case strings.HasPrefix(text, riskCauseOpening):
			return risk.Cause, true
		case strings.HasPrefix(text, riskImpactOpening):
			return risk.Impact, true
		case strings.HasPrefix(text, riskPracticalOpening):
			return risk.Practical, true
		case strings.HasPrefix(text, riskChainOpening):
			return risk.Chain, true
		case strings.HasPrefix(text, riskOverallOpening):
			return risk.Overall, true
		case strings.HasPrefix(text, introOpening):
			return intro, true
		case strings.HasPrefix(text, scopeOpening):
			return scope, true
		case recon != "" && strings.HasPrefix(text, reconOpening):
			return recon, true
		case analysis != "" && strings.HasPrefix(text, analysisOpening):
			return analysis, true
		case strings.HasPrefix(text, postureOpening):
			return posture, true
		case strings.HasPrefix(text, closingOpening):
			return closing, true
		}
		return "", false
	}

	var b strings.Builder
	prev := 0
	for _, p := range childElems(doc, "w:p") {
		para := doc[p.Start:p.End]
		text, ok := decide(elemText(para))
		if !ok {
			continue
		}
		b.WriteString(doc[prev:p.Start])
		if text == "" {
			// Nothing true left to say. Section properties are the one thing a
			// removed paragraph must not take with it.
			b.WriteString(sectPrCarriers(para))
		} else {
			b.WriteString(setParaTextKeepingCompanyBookmark(para, text))
		}
		prev = p.End
	}
	if prev == 0 {
		return doc
	}
	b.WriteString(doc[prev:])
	return b.String()
}

// setParaTextKeepingCompanyBookmark is setParaText for a paragraph that may hold
// the ClientCompanyName bookmark. The introduction does: every page footer
// prints the client's name through a REF field pointing at it, and the PDF
// export refreshes fields, so rebuilding the paragraph without it would blank
// the name in every footer. The bookmark is put back around [Company Name].
func setParaTextKeepingCompanyBookmark(para, text string) string {
	m := clientCompanyBookmarkRe.FindStringSubmatch(para)
	at := strings.Index(text, "[Company Name]")
	if m == nil || at < 0 {
		return setParaText(para, text)
	}
	start, end := m[0], `<w:bookmarkEnd w:id="`+m[1]+`"/>`
	if !strings.Contains(para, end) {
		return setParaText(para, text)
	}
	gt := strings.Index(para, ">")
	if gt < 0 {
		return para
	}
	rPr := paraFirstRPr(para)
	before, after := text[:at], text[at+len("[Company Name]"):]
	return para[:gt+1] + paraPPr(para) +
		runsForText(rPr, before) + start + runsForText(rPr, "[Company Name]") + end + runsForText(rPr, after) +
		`</w:p>`
}

func introParagraph(areas []string) string {
	var assessed, onsite, remote, reviewed []string
	for _, code := range areas {
		n := areaNarratives[code]
		assessed = append(assessed, n.Assessed)
		switch n.Mode {
		case "onsite":
			onsite = append(onsite, n.Target)
		case "remote":
			remote = append(remote, n.Target)
		default:
			reviewed = append(reviewed, n.Target)
		}
	}

	s := introOpening + " [Company Name] " + joinWithAnd(assessed) + ". "

	const vapt = "Vulnerability assessment and penetration testing (VAPT) activities were conducted "
	switch {
	case len(onsite) > 0 && len(remote) > 0:
		s += vapt + "both onsite for " + joinWithAnd(onsite) + " and remotely for " + joinWithAnd(remote) + ". "
	case len(onsite) > 0:
		s += vapt + "onsite for " + joinWithAnd(onsite) + ". "
	case len(remote) > 0:
		s += vapt + "remotely for " + joinWithAnd(remote) + ". "
	}
	if len(reviewed) > 0 {
		if len(onsite)+len(remote) > 0 {
			s += "A review of " + joinWithAnd(reviewed) + " was also carried out. "
		} else {
			s += "The assessment was conducted as a review of " + joinWithAnd(reviewed) + ". "
		}
	}
	// The template's own sentence, unterminated as the template has it.
	return s + "The security assessment was carried out during the period from [Assessment Start – Assessment End]"
}

func scopeParagraph(areas []string) string {
	var activities []string
	testing := false
	for _, code := range areas {
		n := areaNarratives[code]
		activities = append(activities, n.Activity)
		if n.Mode != "review" {
			testing = true
		}
	}
	s := "Finding security flaws in the corresponding networks was the work's goal. Activities pertaining to " +
		joinWithAnd(activities)
	if testing {
		return s + ", and penetration testing carried out in a way that mimicked the methods of malicious hackers were all included in the scope."
	}
	return s + " were included in the scope."
}

// reviewMethodParagraphs replaces the reconnaissance and exploitation account
// when nothing was tested - there were no scans and no exploitation attempts
// in a review. With any testing area selected the template's account stands,
// and both results are empty.
func reviewMethodParagraphs(areas []string) (string, string) {
	var material, standard []string
	for _, code := range areas {
		switch code {
		case "CFG":
			material = append(material, "the configuration files of the in-scope devices")
			standard = append(standard, "security hardening best practice")
		case "NAR":
			material = append(material, "the network design documentation")
			standard = append(standard, "secure network design principles")
		default:
			return "", ""
		}
	}
	recon := "During an initial phase of the review, " + joinWithAnd(material) +
		" were collected. These were then examined against " + joinWithAnd(standard) +
		", and every deviation identified was verified and recorded for a comprehensive summarization with this report."
	analysis := "The discovered issues were thoroughly analyzed to establish their effect on the security of the environment. " +
		"Security issues can arise from configuration errors, unsupported software and weaknesses in network design, " +
		"and many of them result from a lack of security hardening processes for devices and services."
	return recon, analysis
}

// postureParagraph gives each selected area one sentence matching its worst
// finding. It reports whether any area had a high or critical issue, and
// whether any area came through with nothing above medium.
func postureParagraph(areas []string, worst map[string]int) (string, bool, bool) {
	var sentences []string
	anyConcern, anySound := false, false
	for _, code := range areas {
		n := areaNarratives[code]
		rank, has := worst[code]
		switch {
		case !has:
			sentences = append(sentences, n.Secure)
			anySound = true
		case rank <= 1: // critical or high
			sentences = append(sentences, n.Concern)
			anyConcern = true
		case rank == 2: // medium
			verb := "is"
			if n.Plural {
				verb = "are"
			}
			sentences = append(sentences, capitalizeFirst(n.Subject)+" "+verb+
				" reasonably well secured, although the medium-risk issues identified there should be addressed before they can be combined with other weaknesses.")
			anySound = true
		default: // low or informational only
			sentences = append(sentences, n.Secure+
				" Only low-risk issues were identified there, and they do not materially undermine the overall security posture.")
			anySound = true
		}
	}
	return strings.Join(sentences, " "), anyConcern, anySound
}

func closingParagraph(anyConcern, anySound bool, findings int) string {
	switch {
	case anyConcern && anySound:
		return "While the assessment identified several commendable security practices in place, a number of significant vulnerabilities were also uncovered which, if left unaddressed, could expose the organization to unnecessary risk. We strongly recommend that these findings be prioritized to further reduce the overall attack surface."
	case anyConcern:
		return "The assessment uncovered a number of significant vulnerabilities which, if left unaddressed, could expose the organization to unnecessary risk. We strongly recommend that these findings be prioritized to reduce the overall attack surface."
	case findings > 0:
		return "While the assessment identified several commendable security practices in place, the issues that were uncovered should still be addressed to further reduce the overall attack surface."
	default:
		return "The assessment identified several commendable security practices in place. We recommend that these controls are maintained and reviewed regularly to keep the attack surface low."
	}
}

// joinWithAnd lists items as "a", "a and b" or "a, b and c".
func joinWithAnd(items []string) string {
	switch len(items) {
	case 0:
		return ""
	case 1:
		return items[0]
	}
	return strings.Join(items[:len(items)-1], ", ") + " and " + items[len(items)-1]
}

func capitalizeFirst(s string) string {
	if s == "" {
		return s
	}
	r := []rune(s)
	return strings.ToUpper(string(r[0])) + string(r[1:])
}
