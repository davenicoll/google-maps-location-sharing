package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
)

type Location struct {
	Name      string  `json:"name"`
	ShortName string  `json:"short_name"`
	GoogleID  string  `json:"google_id"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Accuracy  int     `json:"accuracy_meters"`
	Address   string  `json:"address"`
	Timestamp int64   `json:"timestamp_ms"`
	Time      string  `json:"time"`
	Battery   int     `json:"battery_percent"`
	Charging  bool    `json:"charging"`
	PhotoURL  string  `json:"photo_url,omitempty"`
}

type StoredCookie struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Domain string `json:"domain"`
	Secure bool   `json:"secure"`
}

func main() {
	harFile := flag.String("import-har", "", "import cookies from a HAR file")
	cookieFile := flag.String("cookies", "", "cookie store path (default: ~/.config/googlelocationsharing/cookies.json)")
	flag.Parse()

	cookiePath := *cookieFile
	if cookiePath == "" {
		home, _ := os.UserHomeDir()
		cookiePath = filepath.Join(home, ".config", "googlelocationsharing", "cookies.json")
	}

	if *harFile != "" {
		if err := importHAR(*harFile, cookiePath); err != nil {
			fmt.Fprintf(os.Stderr, "error importing HAR: %v\n", err)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "cookies imported to %s\n", cookiePath)
		return
	}

	locations, err := fetchLocations(cookiePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	out, _ := json.MarshalIndent(locations, "", "  ")
	fmt.Println(string(out))
}

// importHAR extracts Google cookies from a HAR file and saves them.
func importHAR(harPath, cookiePath string) error {
	data, err := os.ReadFile(harPath)
	if err != nil {
		return fmt.Errorf("reading HAR: %w", err)
	}

	var har struct {
		Log struct {
			Entries []struct {
				Request struct {
					URL     string `json:"url"`
					Cookies []struct {
						Name   string `json:"name"`
						Value  string `json:"value"`
						Domain string `json:"domain"`
					} `json:"cookies"`
				} `json:"request"`
			} `json:"entries"`
		} `json:"log"`
	}
	if err := json.Unmarshal(data, &har); err != nil {
		return fmt.Errorf("parsing HAR: %w", err)
	}

	seen := make(map[string]bool)
	var cookies []StoredCookie
	for _, entry := range har.Log.Entries {
		if !strings.Contains(entry.Request.URL, "google.com") {
			continue
		}
		for _, c := range entry.Request.Cookies {
			if seen[c.Name] || strings.HasPrefix(c.Name, "__Host-") {
				continue
			}
			seen[c.Name] = true
			domain := c.Domain
			if domain == "" {
				domain = ".google.com"
			}
			cookies = append(cookies, StoredCookie{
				Name:   c.Name,
				Value:  c.Value,
				Domain: domain,
				Secure: strings.HasPrefix(c.Name, "__Secure-"),
			})
		}
	}

	if len(cookies) == 0 {
		return fmt.Errorf("no Google cookies found in HAR file")
	}

	os.MkdirAll(filepath.Dir(cookiePath), 0700)
	out, _ := json.MarshalIndent(cookies, "", "  ")
	if err := os.WriteFile(cookiePath, out, 0600); err != nil {
		return fmt.Errorf("writing cookies: %w", err)
	}

	fmt.Fprintf(os.Stderr, "extracted %d cookies\n", len(cookies))
	return nil
}

func loadCookies(cookiePath string) ([]StoredCookie, error) {
	data, err := os.ReadFile(cookiePath)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w (run --import-har first)", cookiePath, err)
	}
	var cookies []StoredCookie
	if err := json.Unmarshal(data, &cookies); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", cookiePath, err)
	}
	return cookies, nil
}

// fetchLocations launches headless Chrome, injects cookies, and fetches location data.
func fetchLocations(cookiePath string) ([]Location, error) {
	cookies, err := loadCookies(cookiePath)
	if err != nil {
		return nil, err
	}

	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("no-first-run", true),
		chromedp.Flag("disable-default-apps", true),
		chromedp.Flag("no-sandbox", true),
		chromedp.Headless,
	)

	allocCtx, allocCancel := chromedp.NewExecAllocator(context.Background(), opts...)
	defer allocCancel()
	ctx, ctxCancel := chromedp.NewContext(allocCtx)
	defer ctxCancel()

	// Build CDP cookie params
	var params []*network.CookieParam
	for _, c := range cookies {
		params = append(params, &network.CookieParam{
			Name:   c.Name,
			Value:  c.Value,
			Domain: c.Domain,
			Path:   "/",
			Secure: c.Secure,
			URL:    "https://www.google.com",
		})
	}

	// Navigate to Google Maps with injected cookies
	err = chromedp.Run(ctx,
		chromedp.Navigate("about:blank"),
		chromedp.ActionFunc(func(ctx context.Context) error {
			return network.SetCookies(params).Do(ctx)
		}),
		chromedp.Navigate("https://www.google.com/maps"),
		chromedp.WaitReady("body"),
		chromedp.Sleep(2*time.Second),
	)
	if err != nil {
		return nil, fmt.Errorf("navigating to maps: %w", err)
	}

	// Check if we're logged in
	var tokenCheck string
	chromedp.Run(ctx,
		chromedp.EvaluateAsDevTools(`
			(() => {
				const scripts = document.querySelectorAll('script');
				for (const s of scripts) {
					const m = s.textContent.match(/"SNlM0e":"([^"]+)"/);
					if (m) return m[1];
				}
				return '';
			})()
		`, &tokenCheck),
	)
	if tokenCheck == "" {
		return nil, fmt.Errorf("not logged in or session expired (import a fresh HAR with --import-har)")
	}

	// Execute the location fetch from within the browser context
	err = chromedp.Run(ctx,
		chromedp.Evaluate(fetchJS, nil),
	)
	if err != nil {
		return nil, fmt.Errorf("fetching locations: %w", err)
	}

	var resultStr string
	err = chromedp.Run(ctx,
		chromedp.PollFunction(`() => window.__glsResult !== undefined`, nil,
			chromedp.WithPollingTimeout(15*time.Second),
		),
		chromedp.EvaluateAsDevTools(`window.__glsResult`, &resultStr),
	)
	if err != nil {
		return nil, fmt.Errorf("waiting for location data: %w", err)
	}
	if resultStr == "" {
		return nil, fmt.Errorf("empty response")
	}

	return parseResponse([]byte(resultStr))
}

const fetchJS = `
(async () => {
	delete window.__glsResult;

	// Extract XSRF token (SNlM0e)
	let xsrf = '';
	for (const s of document.querySelectorAll('script')) {
		const m = s.textContent.match(/"SNlM0e":"([^"]+)"/);
		if (m) { xsrf = m[1]; break; }
	}
	if (!xsrf && window.WIZ_global_data && window.WIZ_global_data.SNlM0e) {
		xsrf = window.WIZ_global_data.SNlM0e;
	}
	if (!xsrf) {
		window.__glsResult = JSON.stringify({error: 'XSRF token not found'});
		return;
	}

	// Extract page session ID (kEI)
	let kei = '';
	for (const s of document.querySelectorAll('script')) {
		const m = s.textContent.match(/kEI='([^']+)'/);
		if (m) { kei = m[1]; break; }
	}
	if (!kei) {
		window.__glsResult = JSON.stringify({error: 'session ID (kEI) not found'});
		return;
	}

	// Build the f.req payload matching the actual browser format
	const innerPayload = JSON.stringify([1, [kei, null, null, null, null, null, 81, null, null, null, null, null, null, null, 312733]]);
	const freqPayload = JSON.stringify([[["/MapsLocationSharingService.GetState", innerPayload, null, "generic"]]]);

	const payload = new URLSearchParams();
	payload.set('f.req', freqPayload);
	payload.set('at', xsrf);

	const resp = await fetch('https://www.google.com/maps/_/MapsWizUi/data/batchexecute?rpcids=Qt4aBe&hl=en&authuser=0&rt=c', {
		method: 'POST',
		headers: {
			'Content-Type': 'application/x-www-form-urlencoded;charset=utf-8',
			'X-Same-Domain': '1',
		},
		body: payload.toString(),
		credentials: 'include',
	});

	window.__glsResult = await resp.text();
})()`

func parseResponse(body []byte) ([]Location, error) {
	text := string(body)

	if strings.HasPrefix(text, `{"error":`) {
		return nil, fmt.Errorf("%s", text)
	}

	lines := strings.Split(text, "\n")

	var dataLine string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, `[["wrb.fr"`) {
			dataLine = line
			break
		}
	}
	if dataLine == "" {
		return nil, fmt.Errorf("no location data found in response (body starts with: %s)", text[:min(200, len(text))])
	}

	var outer [][]interface{}
	if err := json.Unmarshal([]byte(dataLine), &outer); err != nil {
		return nil, fmt.Errorf("parsing outer JSON: %w", err)
	}
	if len(outer) == 0 || len(outer[0]) < 3 {
		return nil, fmt.Errorf("unexpected response structure")
	}

	innerStr, ok := outer[0][2].(string)
	if !ok {
		return nil, fmt.Errorf("inner payload is not a string")
	}

	var inner []interface{}
	if err := json.Unmarshal([]byte(innerStr), &inner); err != nil {
		return nil, fmt.Errorf("parsing inner JSON: %w", err)
	}

	if len(inner) == 0 || inner[0] == nil {
		return nil, fmt.Errorf("no location data (session may be expired)")
	}

	peopleRaw, ok := inner[0].([]interface{})
	if !ok {
		return nil, fmt.Errorf("unexpected people data type")
	}

	var locations []Location
	for _, pRaw := range peopleRaw {
		person, ok := pRaw.([]interface{})
		if !ok || len(person) < 7 {
			continue
		}
		loc := parsePerson(person)
		if loc != nil {
			locations = append(locations, *loc)
		}
	}

	if len(inner) > 9 && inner[9] != nil {
		selfData, ok := inner[9].([]interface{})
		if ok {
			loc := parseSelfLocation(selfData)
			if loc != nil {
				locations = append(locations, *loc)
			}
		}
	}

	return locations, nil
}

func parsePerson(person []interface{}) *Location {
	info := toSlice(person[0])
	locData := toSlice(person[1])
	profile := toSlice(person[6])

	if info == nil || locData == nil {
		return nil
	}

	loc := &Location{
		GoogleID: toString(info, 0),
		Name:     toString(info, 3),
	}

	if profile != nil && len(profile) > 3 {
		loc.ShortName = toString(profile, 3)
		loc.PhotoURL = toString(profile, 1)
	}

	coords := toSlice(locData[1])
	if coords != nil && len(coords) >= 3 {
		loc.Longitude = toFloat(coords, 1)
		loc.Latitude = toFloat(coords, 2)
	}

	loc.Timestamp = toInt64(locData, 2)
	loc.Accuracy = int(toInt64(locData, 3))
	loc.Address = toString(locData, 4)
	loc.Time = time.UnixMilli(loc.Timestamp).Format(time.RFC3339)

	if len(person) > 13 && person[13] != nil {
		battery := toSlice(person[13])
		if battery != nil && len(battery) >= 2 {
			loc.Charging = toBool(battery, 0)
			loc.Battery = int(toInt64(battery, 1))
		}
	}

	return loc
}

func parseSelfLocation(selfData []interface{}) *Location {
	if len(selfData) < 2 || selfData[1] == nil {
		return nil
	}

	locBlock := toSlice(selfData[1])
	if locBlock == nil {
		return nil
	}

	loc := &Location{
		Name:      "Self",
		ShortName: "Self",
	}

	coords := toSlice(locBlock[1])
	if coords != nil && len(coords) >= 3 {
		loc.Longitude = toFloat(coords, 1)
		loc.Latitude = toFloat(coords, 2)
	}

	loc.Timestamp = toInt64(locBlock, 2)
	loc.Accuracy = int(toInt64(locBlock, 3))
	loc.Address = toString(locBlock, 4)
	loc.Time = time.UnixMilli(loc.Timestamp).Format(time.RFC3339)

	return loc
}

func toSlice(v interface{}) []interface{} {
	if v == nil {
		return nil
	}
	s, ok := v.([]interface{})
	if !ok {
		return nil
	}
	return s
}

func toString(arr []interface{}, idx int) string {
	if idx >= len(arr) || arr[idx] == nil {
		return ""
	}
	s, ok := arr[idx].(string)
	if !ok {
		return ""
	}
	return s
}

func toFloat(arr []interface{}, idx int) float64 {
	if idx >= len(arr) || arr[idx] == nil {
		return 0
	}
	f, ok := arr[idx].(float64)
	if !ok {
		return 0
	}
	return f
}

func toInt64(arr []interface{}, idx int) int64 {
	if idx >= len(arr) || arr[idx] == nil {
		return 0
	}
	f, ok := arr[idx].(float64)
	if !ok {
		return 0
	}
	return int64(f)
}

func toBool(arr []interface{}, idx int) bool {
	if idx >= len(arr) || arr[idx] == nil {
		return false
	}
	b, ok := arr[idx].(bool)
	if ok {
		return b
	}
	f, ok := arr[idx].(float64)
	if ok {
		return f != 0
	}
	return false
}
