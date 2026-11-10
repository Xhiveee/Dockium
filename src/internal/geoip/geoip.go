package geoip

import (
	"context"
	"io"
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"time"
)

const LocalNetwork = "Локальная сеть"

var (
	client = &http.Client{Timeout: 3 * time.Second}
	mu     sync.Mutex
	cache  = map[string]cachedCountry{}
)

type cachedCountry struct {
	value     string
	expiresAt time.Time
}

func parseAddr(raw string) (netip.Addr, bool) {
	addr, err := netip.ParseAddr(strings.TrimSpace(raw))
	if err != nil {
		return netip.Addr{}, false
	}
	return addr.Unmap(), true
}

func IsLocal(raw string) bool {
	if strings.EqualFold(strings.TrimSpace(raw), "localhost") {
		return true
	}
	addr, ok := parseAddr(raw)
	if !ok {
		return false
	}
	return addr.IsLoopback() || addr.IsPrivate() || addr.IsLinkLocalUnicast() || addr.IsUnspecified()
}

func cached(raw string) (string, bool) {
	mu.Lock()
	defer mu.Unlock()
	entry, ok := cache[raw]
	if !ok || time.Now().After(entry.expiresAt) {
		return "", false
	}
	return entry.value, true
}

func remember(raw, value string, ttl time.Duration) {
	mu.Lock()
	defer mu.Unlock()
	cache[raw] = cachedCountry{value: value, expiresAt: time.Now().Add(ttl)}
}

func Classify(raw string) (country string, resolved bool) {
	if strings.TrimSpace(raw) == "" {
		return "", true
	}
	if IsLocal(raw) {
		return LocalNetwork, true
	}
	if _, ok := parseAddr(raw); !ok {
		return "", true
	}
	return cached(raw)
}

func CountryForIP(ctx context.Context, raw string) string {
	country, resolved := Classify(raw)
	if resolved {
		return country
	}
	addr, ok := parseAddr(raw)
	if !ok {
		remember(raw, "", 10*time.Minute)
		return ""
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://ipapi.co/"+addr.String()+"/country_name/", nil)
	if err != nil {
		return ""
	}
	response, err := client.Do(request)
	if err != nil {
		remember(raw, "", 10*time.Minute)
		return ""
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		remember(raw, "", 10*time.Minute)
		return ""
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 256))
	if err != nil {
		return ""
	}
	country = strings.TrimSpace(string(body))
	if strings.Contains(country, "{") {
		country = ""
	}
	ttl := 24 * time.Hour
	if country == "" {
		ttl = 10 * time.Minute
	}
	remember(raw, country, ttl)
	return country
}
