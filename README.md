# Google Maps Location Sharing

A Go CLI tool that fetches shared location data from Google Maps. There is no official API for this, so it uses headless Chrome to make authenticated requests against Google's internal `MapsLocationSharingService`.

## How it works

1. You export a HAR file from your browser while viewing Google Maps
2. The tool extracts your Google session cookies from the HAR
3. On each run, it launches headless Chromium, injects the cookies, and calls the location sharing endpoint
4. Results are printed as JSON to stdout

## Prerequisites

- Go 1.21+
- Chromium or Google Chrome installed

## Install

```bash
go install github.com/davenicoll/google-maps-location-sharing@latest
```

Or build from source:

```bash
git clone git@github.com:davenicoll/google-maps-location-sharing.git
cd google-maps-location-sharing
go build -o googlelocationsharing .
```

## Usage

### 1. Export cookies

Open Google Maps in your browser, open DevTools (F12), go to the Network tab, refresh the page, then right-click and **Save all as HAR**.

### 2. Import the HAR

```bash
./googlelocationsharing --import-har /path/to/capture.har
```

This extracts session cookies to `~/.config/googlelocationsharing/cookies.json`.

### 3. Fetch locations

```bash
./googlelocationsharing
```

Output:

```json
[
  {
    "name": "Jane Doe",
    "short_name": "Jane",
    "google_id": "123456789",
    "latitude": 49.2065,
    "longitude": -122.558,
    "accuracy_meters": 12,
    "address": "123 Main St, Vancouver, BC, Canada",
    "timestamp_ms": 1775700685494,
    "time": "2026-04-08T19:11:25-07:00",
    "battery_percent": 50,
    "charging": false
  }
]
```

### Options

| Flag | Description |
|------|-------------|
| `--import-har <file>` | Import cookies from a HAR file |
| `--cookies <path>` | Custom cookie store path |

## Cookie expiry

Session cookies typically last weeks to months. When they expire, the tool will report a session error — just export a new HAR and re-import.

## License

MIT
