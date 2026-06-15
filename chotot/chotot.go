// Package chotot is the library behind the chotot command: the HTTP client,
// request shaping, and typed data models for Chợ Tốt (chotot.com), Vietnam's
// largest classifieds marketplace.
//
// The client talks to gateway.chotot.com over plain HTTPS, shapes each API
// response into clean Go records, and paces requests so a busy session stays
// polite. No API key or account is required for the public listing endpoints.
package chotot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// GatewayBase is the REST gateway every request is built from.
const GatewayBase = "https://gateway.chotot.com"

// ListingBase is the canonical web URL prefix for a single ad.
const ListingBase = "https://www.chotot.com"

// Host is the display host claimed by the domain driver.
const Host = "www.chotot.com"

// DefaultUserAgent is the UA sent with every request.
const DefaultUserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.0.0 Safari/537.36"

// ErrNotFound is returned when the API reports no ad for the requested id.
var ErrNotFound = errors.New("chotot: not found")

// Config holds the constructor parameters for a Client.
type Config struct {
	// BaseURL overrides the gateway host (used in tests).
	BaseURL   string
	UserAgent string
	Rate      time.Duration
	Timeout   time.Duration
	Retries   int
}

// DefaultConfig returns sensible production defaults.
func DefaultConfig() Config {
	return Config{
		UserAgent: DefaultUserAgent,
		Rate:      300 * time.Millisecond,
		Retries:   3,
		Timeout:   30 * time.Second,
	}
}

// Client talks to the Chợ Tốt public REST API.
type Client struct {
	cfg  Config
	http *http.Client
	mu   sync.Mutex
	last time.Time
}

// NewClient returns a Client with DefaultConfig settings.
func NewClient() *Client { return NewClientWithConfig(DefaultConfig()) }

// NewClientWithConfig returns a Client built from cfg.
func NewClientWithConfig(cfg Config) *Client {
	return &Client{
		cfg:  cfg,
		http: &http.Client{Timeout: cfg.Timeout},
	}
}

// gateway returns the base URL for the REST gateway. When cfg.BaseURL is set
// (tests) it overrides the production host.
func (c *Client) gateway() string {
	if c.cfg.BaseURL != "" {
		return strings.TrimRight(c.cfg.BaseURL, "/")
	}
	return GatewayBase
}

// Get fetches rawURL and returns the full response body.
func (c *Client) Get(ctx context.Context, rawURL string) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt <= c.cfg.Retries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff(attempt)):
			}
		}
		body, retry, err := c.do(ctx, rawURL)
		if err == nil {
			return body, nil
		}
		lastErr = err
		if !retry {
			return nil, err
		}
	}
	return nil, fmt.Errorf("get %s: %w", rawURL, lastErr)
}

func (c *Client) do(ctx context.Context, rawURL string) (body []byte, retry bool, err error) {
	c.pace()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("User-Agent", c.cfg.UserAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, true, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		return nil, false, ErrNotFound
	}
	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
		return nil, true, fmt.Errorf("http %d", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("http %d", resp.StatusCode)
	}

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, true, err
	}
	return b, false, nil
}

func (c *Client) pace() {
	if c.cfg.Rate <= 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if wait := c.cfg.Rate - time.Since(c.last); wait > 0 {
		time.Sleep(wait)
	}
	c.last = time.Now()
}

func backoff(attempt int) time.Duration {
	d := time.Duration(attempt) * 500 * time.Millisecond
	if d > 5*time.Second {
		d = 5 * time.Second
	}
	return d
}

// listingURL returns the canonical web URL for a numeric list_id.
func listingURL(listID int64) string {
	return fmt.Sprintf("%s/%d.htm", ListingBase, listID)
}

// unixToRFC3339 converts a Unix timestamp (int64) to an RFC 3339 string.
func unixToRFC3339(ts int64) string {
	if ts == 0 {
		return ""
	}
	return time.Unix(ts, 0).UTC().Format(time.RFC3339)
}

// ---- wire types (unexported, used only for JSON decoding) ----

type wireListingList struct {
	Ads   []wireListing `json:"ads"`
	Total int           `json:"total"`
}

type wireListingDetail struct {
	Ad wireAdDetail `json:"ad"`
}

type wireListing struct {
	ListID      int64    `json:"list_id"`
	Subject     string   `json:"subject"`
	Price       int64    `json:"price"`
	PriceString string   `json:"price_string"`
	AreaName    string   `json:"area_name"`
	RegionName  string   `json:"region_name"`
	Category    int      `json:"category"`
	CategoryName string  `json:"category_name"`
	AccountName string   `json:"account_name"`
	AccountID   int64    `json:"account_id"`
	Type        string   `json:"type"`
	Image       string   `json:"image"`
	Images      []string `json:"images"`
	Date        int64    `json:"date"`
	AdLabel     string   `json:"ad_label"`
	IsPro       int      `json:"is_pro"`
	IsVerified  int      `json:"is_verified"`
}

type wireAdDetail struct {
	ListID       int64    `json:"list_id"`
	Subject      string   `json:"subject"`
	Body         string   `json:"body"`
	Price        int64    `json:"price"`
	PriceString  string   `json:"price_string"`
	AreaName     string   `json:"area_name"`
	RegionName   string   `json:"region_name"`
	Category     int      `json:"category"`
	CategoryName string   `json:"category_name"`
	AccountName  string   `json:"account_name"`
	AccountID    int64    `json:"account_id"`
	Type         string   `json:"type"`
	Images       []string `json:"images"`
	Date         int64    `json:"date"`
	ExpiredDate  int64    `json:"expired_date"`
	Params       []struct {
		Label string `json:"label"`
		Value string `json:"value"`
	} `json:"params"`
	IsPro      int    `json:"is_pro"`
	IsVerified int    `json:"is_verified"`
	Phone      string `json:"phone"`
}

type wireCategoryList struct {
	Cats []wireCategory `json:"cats"`
}

type wireCategory struct {
	CategoryID   int    `json:"category_id"`
	CategoryName string `json:"category_name"`
	ParentID     int    `json:"parent_id"`
	ParentName   string `json:"parent_name"`
}

type wireRegionList struct {
	Regions []wireRegion `json:"regions"`
}

type wireRegion struct {
	RegionV2   int    `json:"region_v2"`
	RegionName string `json:"region_name"`
}

// ---- public record types ----

// Listing is one classified ad on Chợ Tốt.
type Listing struct {
	ID         string   `json:"id"                         kit:"id" table:"id"`
	Title      string   `json:"title"                      table:"title"`
	Price      int64    `json:"price,omitempty"`
	PriceStr   string   `json:"price_string,omitempty"     table:"price"`
	Category   string   `json:"category,omitempty"         table:"category"`
	Region     string   `json:"region,omitempty"           table:"region"`
	Area       string   `json:"area,omitempty"             table:"area"`
	SellerName string   `json:"seller_name,omitempty"      table:"seller"`
	IsPro      bool     `json:"is_pro,omitempty"`
	IsVerified bool     `json:"is_verified,omitempty"`
	Condition  string   `json:"condition,omitempty"`
	Body       string   `json:"body,omitempty"             kit:"body" table:"-"`
	Images     []string `json:"images,omitempty"           table:"-"`
	Params     []Param  `json:"params,omitempty"           table:"-"`
	Phone      string   `json:"phone,omitempty"            table:"-"`
	ViewCount  int      `json:"view_count,omitempty"`
	PostedAt   string   `json:"posted_at,omitempty"`
	URL        string   `json:"url,omitempty"              table:"url,url"`
}

// Param is a structured attribute on a listing (e.g. brand, year, condition).
type Param struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

// Category is a Chợ Tốt listing category.
type Category struct {
	ID       string `json:"id"                kit:"id"`
	Name     string `json:"name"`
	ParentID string `json:"parent_id,omitempty"`
	Parent   string `json:"parent,omitempty"`
}

// Region is a Vietnamese province or city served by Chợ Tốt.
type Region struct {
	ID   string `json:"id"  kit:"id"`
	Name string `json:"name"`
}

// ---- API methods ----

// SearchListings searches for listings matching query. page is 1-indexed.
func (c *Client) SearchListings(ctx context.Context, query string, page, limit int) ([]*Listing, error) {
	if limit <= 0 {
		limit = 20
	}
	if page <= 0 {
		page = 1
	}
	v := url.Values{}
	v.Set("keywords", query)
	v.Set("page", strconv.Itoa(page))
	v.Set("limit", strconv.Itoa(limit))
	return c.fetchListings(ctx, v)
}

// BrowseListings lists recent ads, optionally filtered by numeric category ID
// and/or region ID. Passing 0 for either omits that filter.
func (c *Client) BrowseListings(ctx context.Context, categoryID, regionID, page, limit int) ([]*Listing, error) {
	if limit <= 0 {
		limit = 20
	}
	if page <= 0 {
		page = 1
	}
	v := url.Values{}
	if categoryID > 0 {
		v.Set("cg", strconv.Itoa(categoryID))
	}
	if regionID > 0 {
		v.Set("region_v2", strconv.Itoa(regionID))
	}
	v.Set("page", strconv.Itoa(page))
	v.Set("limit", strconv.Itoa(limit))
	return c.fetchListings(ctx, v)
}

func (c *Client) fetchListings(ctx context.Context, v url.Values) ([]*Listing, error) {
	rawURL := c.gateway() + "/v2/public/ad/listing?" + v.Encode()
	body, err := c.Get(ctx, rawURL)
	if err != nil {
		return nil, err
	}
	var w wireListingList
	if err := json.Unmarshal(body, &w); err != nil {
		return nil, fmt.Errorf("chotot: decode listings: %w", err)
	}
	out := make([]*Listing, 0, len(w.Ads))
	for _, a := range w.Ads {
		out = append(out, convertListing(a))
	}
	return out, nil
}

func convertListing(a wireListing) *Listing {
	imgs := a.Images
	if len(imgs) == 0 && a.Image != "" {
		imgs = []string{a.Image}
	}
	return &Listing{
		ID:         strconv.FormatInt(a.ListID, 10),
		Title:      a.Subject,
		Price:      a.Price,
		PriceStr:   a.PriceString,
		Category:   a.CategoryName,
		Region:     a.RegionName,
		Area:       a.AreaName,
		SellerName: a.AccountName,
		IsPro:      a.IsPro != 0,
		IsVerified: a.IsVerified != 0,
		Condition:  a.AdLabel,
		Images:     imgs,
		PostedAt:   unixToRFC3339(a.Date),
		URL:        listingURL(a.ListID),
	}
}

// GetListing fetches a single ad by its numeric list ID.
func (c *Client) GetListing(ctx context.Context, listID string) (*Listing, error) {
	rawURL := c.gateway() + "/v2/public/ad/detail/" + listID
	body, err := c.Get(ctx, rawURL)
	if err != nil {
		return nil, err
	}
	var w wireListingDetail
	if err := json.Unmarshal(body, &w); err != nil {
		return nil, fmt.Errorf("chotot: decode detail: %w", err)
	}
	a := w.Ad
	id, _ := strconv.ParseInt(listID, 10, 64)
	params := make([]Param, 0, len(a.Params))
	for _, p := range a.Params {
		params = append(params, Param{Label: p.Label, Value: p.Value})
	}
	return &Listing{
		ID:         strconv.FormatInt(a.ListID, 10),
		Title:      a.Subject,
		Price:      a.Price,
		PriceStr:   a.PriceString,
		Category:   a.CategoryName,
		Region:     a.RegionName,
		Area:       a.AreaName,
		SellerName: a.AccountName,
		IsPro:      a.IsPro != 0,
		IsVerified: a.IsVerified != 0,
		Body:       a.Body,
		Images:     a.Images,
		Params:     params,
		Phone:      a.Phone,
		PostedAt:   unixToRFC3339(a.Date),
		URL:        listingURL(id),
	}, nil
}

// GetCategories returns all Chợ Tốt listing categories.
func (c *Client) GetCategories(ctx context.Context) ([]*Category, error) {
	rawURL := c.gateway() + "/v2/public/category"
	body, err := c.Get(ctx, rawURL)
	if err != nil {
		return nil, err
	}
	var w wireCategoryList
	if err := json.Unmarshal(body, &w); err != nil {
		return nil, fmt.Errorf("chotot: decode categories: %w", err)
	}
	out := make([]*Category, 0, len(w.Cats))
	for _, cat := range w.Cats {
		out = append(out, &Category{
			ID:       strconv.Itoa(cat.CategoryID),
			Name:     cat.CategoryName,
			ParentID: strconv.Itoa(cat.ParentID),
			Parent:   cat.ParentName,
		})
	}
	return out, nil
}

// GetRegions returns all Chợ Tốt regions (provinces/cities).
func (c *Client) GetRegions(ctx context.Context) ([]*Region, error) {
	rawURL := c.gateway() + "/v2/public/region"
	body, err := c.Get(ctx, rawURL)
	if err != nil {
		return nil, err
	}
	var w wireRegionList
	if err := json.Unmarshal(body, &w); err != nil {
		return nil, fmt.Errorf("chotot: decode regions: %w", err)
	}
	out := make([]*Region, 0, len(w.Regions))
	for _, r := range w.Regions {
		out = append(out, &Region{
			ID:   strconv.Itoa(r.RegionV2),
			Name: r.RegionName,
		})
	}
	return out, nil
}
