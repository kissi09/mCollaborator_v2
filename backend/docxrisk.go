package main

// docxrisk.go rewrites section 1.3, Risk Assessment and Impact on Business.
//
// The template's six paragraphs were written about a web and fintech pentest:
// OWASP Top 10, customer account takeover, fraudulent refunds, "moving from
// public access" to privileged accounts, and a claim that the named findings
// prove the systems go unpatched. Put next to a network architecture finding
// such as "Flat network", every one of those sentences is false. Here each
// paragraph is rebuilt from the areas that actually have findings and from how
// severe those findings are; a paragraph with nothing true left to say is
// removed rather than printed.

import (
	"regexp"
	"sort"
	"strings"
)

// areaRisk is how section 1.3 speaks about one area's findings.
type areaRisk struct {
	// Theme completes "the key areas of concern are ...".
	Theme string
	// Cause completes "implies that ..." for a finding in this area.
	Cause string
	// Impact completes "expose the business to a severe risk of ...".
	Impact string
}

var areaRisks = map[string]areaRisk{
	"IPT": {
		Theme:  "weaknesses that allow an attacker to move laterally inside the network",
		Cause:  "internal systems have not been consistently hardened",
		Impact: "compromise of internal servers and the data they hold",
	},
	"EPT": {
		Theme:  "services exposed to the Internet",
		Cause:  "the exposure of services to the Internet is not tightly controlled",
		Impact: "intrusion from the Internet through the organization's public-facing services",
	},
	"IPTC": {
		Theme:  "misconfigured cloud resources",
		Cause:  "cloud resources have not been configured to a secure baseline",
		Impact: "unauthorized access to cloud-hosted workloads and data",
	},
	"WPT": {
		Theme:  "common web application vulnerabilities such as those in the OWASP Top 10",
		Cause:  "secure coding practices are not consistently applied",
		Impact: "customer account takeover, manipulation of application data and exposure of sensitive customer information",
	},
	"ASA": {
		Theme:  "weak authentication, authorization and input validation on the APIs",
		Cause:  "security controls are not consistently enforced on the APIs",
		Impact: "unauthorized access to the data and business functions exposed through the APIs",
	},
	"MPT": {
		Theme:  "insecure data storage and communication in the mobile applications",
		Cause:  "the mobile applications have not been built and tested against a mobile security standard",
		Impact: "exposure of user data held on mobile devices and abuse of the back-end services the applications rely on",
	},
	"SCR": {
		Theme:  "insecure coding practices in the application source code",
		Cause:  "secure coding practices are not consistently applied during development",
		Impact: "exploitation of flaws built into the applications themselves, leading to unauthorized access to their data and functions",
	},
	"ADT": {
		Theme:  "privilege escalation paths in Active Directory",
		Cause:  "privileged access in the directory is not tightly governed",
		Impact: "takeover of privileged domain accounts and, through them, the wider IT estate",
	},
	"WNA": {
		Theme:  "weak wireless encryption and authentication",
		Cause:  "the wireless networks have not been configured to current security standards",
		Impact: "unauthorized access to the corporate network from within radio range",
	},
	"CFG": {
		Theme:  "insecure network device configurations",
		Cause:  "the network devices have not been hardened against a security baseline",
		Impact: "unauthorized administrative access to network devices and interception of management traffic",
	},
	"NAR": {
		Theme:  "weaknesses in network design and segmentation",
		Cause:  "security was not fully considered in the design of the network",
		Impact: "an attack spreading unhindered across the network and outages of critical services",
	},
}

// patchFindingRe recognises a finding about missing updates by its title. Only
// such a finding supports "the systems are not regularly updated with essential
// security patches" - the template said it of whatever three findings came next.
var patchFindingRe = regexp.MustCompile(`(?i)end[ -]of[ -]life|\bEOL\b|end of support|outdated|out[ -]of[ -]date|unsupported|obsolete|missing (security )?(patch|update)|unpatched|CVE-\d`)

// Openings of the six template paragraphs of section 1.3.
const (
	riskOverviewOpening  = "The critical issues in the applications pose"
	riskCauseOpening     = "The presence of vulnerabilities"
	riskImpactOpening    = "The vulnerabilities identified across the"
	riskPracticalOpening = "In practical terms, an attacker"
	riskChainOpening     = "Because several of the highlighted weaknesses"
	riskOverallOpening   = "Overall, the tested systems present"
)

// riskSection holds the rebuilt paragraphs. An empty field removes that
// paragraph from the report.
type riskSection struct {
	Overview, Cause, Impact, Practical, Chain, Overall string
}

func buildRiskSection(areas []string, findings []numberedFinding) riskSection {
	sorted := make([]numberedFinding, len(findings))
	copy(sorted, findings)
	sort.SliceStable(sorted, func(i, j int) bool {
		return severityRank(sorted[i].Severity) < severityRank(sorted[j].Severity)
	})

	worst := 5
	if len(sorted) > 0 {
		worst = severityRank(sorted[0].Severity)
	}
	serious := 0 // medium or above
	for _, f := range sorted {
		if severityRank(f.Severity) <= 2 {
			serious++
		}
	}

	// Areas with findings, in template order.
	has := map[string]bool{}
	for _, f := range findings {
		has[f.Area.Code] = true
	}
	var hit []string
	for _, code := range areas {
		if has[code] {
			hit = append(hit, code)
		}
	}

	var r riskSection
	r.Overall = riskOverall(areas, worst)
	if len(sorted) == 0 {
		return r
	}
	r.Overview = riskOverview(hit, worst)
	r.Cause = riskCause(sorted)
	r.Impact = riskImpact(hit, worst)
	if worst <= 1 {
		r.Practical = riskPractical(hit, sorted)
	}
	if serious >= 2 {
		r.Chain = "Because several of the highlighted weaknesses can be chained together, a single successful attack could quickly escalate into a full compromise of critical systems and business operations."
	}
	return r
}

func riskOverview(hit []string, worst int) string {
	var s string
	switch worst {
	case 0:
		s = "The critical issues identified pose a significant risk to [Company Name]’s security posture. "
	case 1:
		s = "The high-risk issues identified pose a significant risk to [Company Name]’s security posture. "
	case 2:
		s = "The issues identified pose a moderate risk to [Company Name]’s security posture. "
	default:
		s = "The issues identified pose a limited risk to [Company Name]’s security posture. "
	}

	var themes []string
	for _, code := range hit {
		themes = append(themes, areaRisks[code].Theme)
	}
	if len(hit) == 1 {
		s += "The key area of concern is " + themes[0] + ". "
	} else {
		var spans []string
		for _, code := range hit {
			spans = append(spans, areaNarratives[code].Assessed)
		}
		s += "These vulnerabilities span the " + joinWithAnd(spans) +
			", and the key areas of concern are " + joinPhrases(themes) + ". "
	}

	switch worst {
	case 0:
		return s + "Their exploitation could have severe repercussions. Therefore, it is crucial that the critical issues are mitigated as soon as possible."
	case 1:
		return s + "Their exploitation could have severe repercussions. Therefore, it is crucial that these issues are mitigated as soon as possible."
	case 2:
		return s + "They should be mitigated in a planned manner before they can be combined with other weaknesses."
	default:
		return s + "They should be addressed as part of routine maintenance."
	}
}

// riskCause names the findings the template's placeholders pointed at - the
// third to fifth most severe, the first two having been named in 1.2 - and says
// only what those findings actually show.
func riskCause(sorted []numberedFinding) string {
	named := sorted
	if len(sorted) >= 3 {
		named = sorted[2:]
	}
	if len(named) > 3 {
		named = named[:3]
	}

	var titles, causes []string
	seen := map[string]bool{}
	patched := false
	for _, f := range named {
		titles = append(titles, strings.TrimSpace(f.Title))
		if patchFindingRe.MatchString(f.Title) {
			patched = true
			continue
		}
		if c := areaRisks[f.Area.Code].Cause; c != "" && !seen[c] {
			seen[c] = true
			causes = append(causes, c)
		}
	}
	if patched {
		causes = append([]string{"the systems are not regularly updated with essential security patches"}, causes...)
	}
	for i := range causes {
		causes[i] = "that " + causes[i]
	}

	subject := "vulnerabilities " + joinWithAnd(titles)
	if len(titles) == 1 {
		subject = "the vulnerability " + titles[0]
	}
	// "The presence of ..." takes "implies" however many findings it names.
	return "The presence of " + subject + " implies " + joinWithAnd(causes) +
		". This exposes the affected systems to a heightened risk of exploitation by potential attackers. " +
		"The impact after a successful attack can lead to data breaches, service disruptions, and reputational damage."
}

func riskImpact(hit []string, worst int) string {
	level := "limited"
	switch {
	case worst <= 1:
		level = "severe"
	case worst == 2:
		level = "moderate"
	}
	var impacts []string
	for _, code := range hit {
		impacts = append(impacts, areaRisks[code].Impact)
	}
	return "The vulnerabilities identified across the [Company Name] environment expose the business to a " +
		level + " risk of " + joinPhrases(impacts) + "."
}

// joinPhrases is joinWithAnd for phrases that may carry an "and" of their own,
// where "a and b and c and d" would leave the reader to find the seams.
func joinPhrases(items []string) string {
	for _, it := range items {
		if !strings.Contains(it, " and ") {
			continue
		}
		switch len(items) {
		case 1:
			return items[0]
		case 2:
			return items[0] + ", as well as " + items[1]
		}
		return strings.Join(items[:len(items)-1], "; ") + "; and " + items[len(items)-1]
	}
	return joinWithAnd(items)
}

// riskPractical describes, for the areas holding a high or critical finding,
// where an attacker would start and what they would reach from there.
func riskPractical(hit []string, sorted []numberedFinding) string {
	grave := map[string]bool{}
	for _, f := range sorted {
		if severityRank(f.Severity) <= 1 {
			grave[f.Area.Code] = true
		}
	}
	external, internal, review, code := false, false, false, false
	for _, c := range hit {
		if !grave[c] {
			continue
		}
		switch {
		case c == "SCR":
			code = true
		case areaNarratives[c].Mode == "remote":
			external = true
		case areaNarratives[c].Mode == "onsite":
			internal = true
		default:
			review = true
		}
	}

	var clauses []string
	if external {
		clauses = append(clauses, "an attacker would not need sophisticated methods to move from public access to controlling privileged accounts, altering records, and abusing core business processes")
	}
	if internal {
		clauses = append(clauses, "an attacker who gains a foothold inside the network would not need sophisticated methods to reach privileged accounts and critical servers")
	}
	if review {
		clauses = append(clauses, "the weaknesses identified would make it considerably easier for an attacker who reaches the network to take control of network devices and move between parts of the network")
	}
	if code {
		clauses = append(clauses, "the flaws identified in the source code are present in every deployment of the applications, giving an attacker who can reach them a direct route to their data and functions")
	}
	if len(clauses) == 0 {
		return ""
	}
	outcome := "disruption of business operations, regulatory consequences, reputational damage, and serious erosion of customer confidence"
	if external {
		outcome = "direct financial loss, " + outcome
	}
	s := "In practical terms, " + clauses[0] + "."
	for _, c := range clauses[1:] {
		s += " Likewise, " + c + "."
	}
	if len(clauses) == 1 {
		return s + " This could result in " + outcome + "."
	}
	return s + " Any of these could result in " + outcome + "."
}

func riskOverall(areas []string, worst int) string {
	var assessed []string
	allRemote, allReview := true, true
	for _, code := range areas {
		n := areaNarratives[code]
		assessed = append(assessed, n.Assessed)
		if n.Mode != "remote" {
			allRemote = false
		}
		if n.Mode != "review" {
			allReview = false
		}
	}
	subject := "the tested systems present"
	if allReview {
		subject = "the reviewed environment presents"
	}
	surface := "a low-risk attack surface"
	switch worst {
	case 0:
		surface = "a high-risk attack surface with critical vulnerabilities"
	case 1:
		surface = "a high-risk attack surface with high-risk vulnerabilities"
	case 2:
		surface = "a moderate-risk attack surface"
	}
	actors := "internal and external threat actors"
	if allRemote {
		actors = "external threat actors"
	}
	s := "Overall, " + subject + " " + surface + " within the [Company Name] " + joinWithAnd(assessed) + "."
	if worst > 4 {
		return s + " Maintaining the controls currently in place will keep the number of attack vectors available to " + actors + " low."
	}
	return s + " If remediated, the findings will further improve security by lowering the risk and reducing the number of attack vectors for " + actors + "."
}
