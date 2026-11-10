package geoip

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"time"
)

const LocalNetwork = "Локальная сеть"

type Country struct {
	Name string
	Code string
}

var (
	client = &http.Client{Timeout: 3 * time.Second}
	mu     sync.Mutex
	cache  = map[string]cachedCountry{}
)

type cachedCountry struct {
	value     Country
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

func cached(raw string) (Country, bool) {
	mu.Lock()
	defer mu.Unlock()
	entry, ok := cache[raw]
	if !ok || time.Now().After(entry.expiresAt) {
		return Country{}, false
	}
	return entry.value, true
}

func remember(raw string, value Country, ttl time.Duration) {
	mu.Lock()
	defer mu.Unlock()
	cache[raw] = cachedCountry{value: value, expiresAt: time.Now().Add(ttl)}
}

func Classify(raw string) (country Country, resolved bool) {
	if strings.TrimSpace(raw) == "" {
		return Country{}, true
	}
	if IsLocal(raw) {
		return Country{Name: LocalNetwork}, true
	}
	if _, ok := parseAddr(raw); !ok {
		return Country{}, true
	}
	return cached(raw)
}

func CountryForIP(ctx context.Context, raw string) Country {
	country, resolved := Classify(raw)
	if resolved {
		return country
	}
	addr, ok := parseAddr(raw)
	if !ok {
		remember(raw, Country{}, 10*time.Minute)
		return Country{}
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://ipapi.co/"+addr.String()+"/json/", nil)
	if err != nil {
		return Country{}
	}
	response, err := client.Do(request)
	if err != nil {
		remember(raw, Country{}, 10*time.Minute)
		return Country{}
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		remember(raw, Country{}, 10*time.Minute)
		return Country{}
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 8192))
	if err != nil {
		return Country{}
	}
	var payload struct {
		Name  string `json:"country_name"`
		Code  string `json:"country_code"`
		Error bool   `json:"error"`
	}
	if err := json.Unmarshal(body, &payload); err != nil || payload.Error {
		remember(raw, Country{}, 10*time.Minute)
		return Country{}
	}
	country = Country{
		Name: strings.TrimSpace(payload.Name),
		Code: strings.ToUpper(strings.TrimSpace(payload.Code)),
	}
	ttl := 24 * time.Hour
	if country.Name == "" {
		ttl = 10 * time.Minute
	}
	remember(raw, country, ttl)
	return country
}
