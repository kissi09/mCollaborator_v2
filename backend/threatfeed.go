package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

type ThreatItem struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Severity    string `json:"severity"`
	Source      string `json:"source"`
	URL         string `json:"url"`
	Category    string `json:"category"`
	PublishedAt string `json:"published_at"`
	Tags        string `json:"tags"`
}

type ThreatFeed struct {
	Items     []ThreatItem `json:"items"`
	FetchedAt string       `json:"fetched_at"`
}

var (
	cachedFeed    *ThreatFeed
	cacheTime     time.Time
	cacheMutex    sync.Mutex
	cacheDuration = 5 * time.Minute
)

func HandleThreatFeed(w http.ResponseWriter, r *http.Request) {
	cacheMutex.Lock()
	if cachedFeed != nil && time.Since(cacheTime) < cacheDuration {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(cachedFeed)
		cacheMutex.Unlock()
		return
	}
	cacheMutex.Unlock()

	type fetchResult struct {
		items []ThreatItem
		err   error
	}

	const apiTimeout = 15 * time.Second
	client := &http.Client{Timeout: apiTimeout}
	since := time.Now().Add(-72 * time.Hour).UTC().Format("2006-01-02T15:04:05.000")
	nvdURL := fmt.Sprintf("https://services.nvd.nist.gov/rest/json/cves/2.0?resultsPerPage=20&pubStartDate=%s", since)

	type apiJob struct {
		name string
		url  string
		fn   func([]byte) []ThreatItem
	}

	jobs := []apiJob{
		{
			name: "CISA KEV",
			url:  "https://www.cisa.gov/sites/default/files/feeds/known_exploited_vulnerabilities.json",
			fn:   ParseCISAKEV,
		},
		{
			name: "URLhaus",
			url:  "https://urlhaus-api.abuse.ch/v1/urls/recent/",
			fn:   ParseURLhaus,
		},
		{
			name: "NVD",
			url:  nvdURL,
			fn:   ParseNVD,
		},
	}

	resultCh := make(chan fetchResult, len(jobs))
	var wg sync.WaitGroup

	for _, job := range jobs {
		wg.Add(1)
		go func(j apiJob) {
			defer wg.Done()
			req, err := http.NewRequest("GET", j.url, nil)
			if err != nil {
				resultCh <- fetchResult{err: fmt.Errorf("%s: %w", j.name, err)}
				return
			}
			req.Header.Set("User-Agent", "mCollaborator-ThreatFeed/1.0")
			resp, err := client.Do(req)
			if err != nil {
				resultCh <- fetchResult{err: fmt.Errorf("%s: %w", j.name, err)}
				return
			}
			defer resp.Body.Close()
			body, err := io.ReadAll(io.LimitReader(resp.Body, 5*1024*1024))
			if err != nil {
				resultCh <- fetchResult{err: fmt.Errorf("%s: %w", j.name, err)}
				return
			}
			items := j.fn(body)
			resultCh <- fetchResult{items: items}
		}(job)
	}

	go func() {
		wg.Wait()
		close(resultCh)
	}()

	var allItems []ThreatItem
	for res := range resultCh {
		if res.err != nil {
			continue
		}
		allItems = append(allItems, res.items...)
	}

	feed := &ThreatFeed{
		Items:     allItems,
		FetchedAt: time.Now().UTC().Format(time.RFC3339),
	}

	cacheMutex.Lock()
	cachedFeed = feed
	cacheTime = time.Now()
	cacheMutex.Unlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(feed)
}

type cisaVulnerability struct {
	CVEID                 string   `json:"cveID"`
	VulnerabilityName     string   `json:"vulnerabilityName"`
	DateAdded             string   `json:"dateAdded"`
	ShortDescription      string   `json:"shortDescription"`
	RequiredAction        string   `json:"requiredAction"`
	DueDate               string   `json:"dueDate"`
	KnownRansomwareUse    string   `json:"knownRansomwareUse"`
	Notes                 string   `json:"notes"`
}

type cisaKEVResponse struct {
	Title        string              `json:"title"`
	Count        int                 `json:"count"`
	Vulnerabilities []cisaVulnerability `json:"vulnerabilities"`
}

func ParseCISAKEV(data []byte) []ThreatItem {
	var resp cisaKEVResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil
	}
	items := make([]ThreatItem, 0, len(resp.Vulnerabilities))
	for _, v := range resp.Vulnerabilities {
		severity := "HIGH"
		if v.KnownRansomwareUse == "Confirmed" {
			severity = "CRITICAL"
		}
		item := ThreatItem{
			ID:          v.CVEID,
			Title:       v.VulnerabilityName,
			Description: v.ShortDescription,
			Severity:    severity,
			Source:      "CISA KEV",
			URL:         "https://nvd.nist.gov/vuln/detail/" + v.CVEID,
			Category:    "Vulnerability",
			PublishedAt: v.DateAdded,
			Tags:        fmt.Sprintf("ransomware:%s", v.KnownRansomwareUse),
		}
		items = append(items, item)
	}
	return items
}

type urlhausURL struct {
	ID         string   `json:"id"`
	URL        string   `json:"url"`
	URLStatus  string   `json:"urlstatus"`
	DateAdded  string   `json:"date_added"`
	Threat     string   `json:"threat"`
	Tags       []string `json:"tags"`
	Reporter   string   `json:"reporter"`
}

type urlhausResponse struct {
	Urls []urlhausURL `json:"urls"`
}

func ParseURLhaus(data []byte) []ThreatItem {
	var resp urlhausResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil
	}
	items := make([]ThreatItem, 0, len(resp.Urls))
	for _, u := range resp.Urls {
		tags := ""
		if len(u.Tags) > 0 {
			for i, t := range u.Tags {
				if i > 0 {
					tags += ","
				}
				tags += t
			}
		}
		severity := "MEDIUM"
		if u.Threat == "malware_download" {
			severity = "HIGH"
		}
		item := ThreatItem{
			ID:          u.ID,
			Title:       fmt.Sprintf("Malicious URL: %s", u.URL),
			Description: fmt.Sprintf("URL status: %s, reported by: %s", u.URLStatus, u.Reporter),
			Severity:    severity,
			Source:      "URLhaus",
			URL:         u.URL,
			Category:    "Malicious URL",
			PublishedAt: u.DateAdded,
			Tags:        tags,
		}
		items = append(items, item)
	}
	return items
}

type nvdMetric struct {
	CVSSMetricV31 []struct {
		CVSSData struct {
			BaseScore float64 `json:"baseScore"`
			Severity  string  `json:"baseSeverity"`
		} `json:"cvssData"`
	} `json:"cvssMetricV31"`
}

type nvdDescription struct {
	Lang  string `json:"lang"`
	Value string `json:"value"`
}

type nvdCVEItem struct {
	CVE struct {
		ID               string        `json:"id"`
		Descriptions     []nvdDescription `json:"descriptions"`
		Published        string        `json:"published"`
		LastModified     string        `json:"lastModified"`
		Metrics          nvdMetric     `json:"metrics"`
	} `json:"cve"`
}

type nvdResponse struct {
	ResultsPerPage int          `json:"resultsPerPage"`
	TotalResults   int          `json:"totalResults"`
	Results        []nvdCVEItem `json:"vulnerabilities"`
}

func ParseNVD(data []byte) []ThreatItem {
	var resp nvdResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil
	}
	items := make([]ThreatItem, 0, len(resp.Results))
	for _, r := range resp.Results {
		desc := ""
		for _, d := range r.CVE.Descriptions {
			if d.Lang == "en" {
				desc = d.Value
				break
			}
		}
		if desc == "" && len(r.CVE.Descriptions) > 0 {
			desc = r.CVE.Descriptions[0].Value
		}

		severity := "UNKNOWN"
		if len(r.CVE.Metrics.CVSSMetricV31) > 0 {
			severity = r.CVE.Metrics.CVSSMetricV31[0].CVSSData.Severity
		}

		item := ThreatItem{
			ID:          r.CVE.ID,
			Title:       fmt.Sprintf("CVE: %s", r.CVE.ID),
			Description: desc,
			Severity:    severity,
			Source:      "NVD",
			URL:         "https://nvd.nist.gov/vuln/detail/" + r.CVE.ID,
			Category:    "Vulnerability",
			PublishedAt: r.CVE.Published,
			Tags:        "",
		}
		items = append(items, item)
	}
	return items
}
