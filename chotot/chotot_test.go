package chotot

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func newTestClient(srv *httptest.Server) *Client {
	cfg := DefaultConfig()
	cfg.BaseURL = srv.URL
	cfg.Rate = 0
	cfg.Retries = 0
	return NewClientWithConfig(cfg)
}

func sampleListingListJSON(n int) string {
	type ad struct {
		ListID       int64  `json:"list_id"`
		Subject      string `json:"subject"`
		Price        int64  `json:"price"`
		PriceString  string `json:"price_string"`
		CategoryName string `json:"category_name"`
		RegionName   string `json:"region_name"`
		AreaName     string `json:"area_name"`
		AccountName  string `json:"account_name"`
		AccountID    int64  `json:"account_id"`
		Date         int64  `json:"date"`
	}
	var ads []ad
	for i := 0; i < n; i++ {
		ads = append(ads, ad{
			ListID:       int64(10000000 + i),
			Subject:      "Listing " + string(rune('A'+i)),
			Price:        int64(5000000 * (i + 1)),
			PriceString:  "5.000.000 đ",
			CategoryName: "Xe máy",
			RegionName:   "Hồ Chí Minh",
			AreaName:     "Quận 1",
			AccountName:  "Seller " + string(rune('A'+i)),
			Date:         1700000000,
		})
	}
	b, _ := json.Marshal(map[string]any{"ads": ads, "total": n})
	return string(b)
}

func sampleListingDetailJSON(listID int64) string {
	detail := map[string]any{
		"ad": map[string]any{
			"list_id":       listID,
			"subject":       "Honda Wave Alpha 2020",
			"body":          "Xe còn mới, ít đi",
			"price":         15000000,
			"price_string":  "15.000.000 đ",
			"category_name": "Xe máy",
			"region_name":   "Hà Nội",
			"area_name":     "Cầu Giấy",
			"account_name":  "Nguyen Van A",
			"account_id":    98765,
			"date":          1700000000,
			"params": []map[string]string{
				{"label": "Hãng xe", "value": "Honda"},
				{"label": "Năm sản xuất", "value": "2020"},
			},
			"is_pro":      0,
			"is_verified": 1,
		},
	}
	b, _ := json.Marshal(detail)
	return string(b)
}

func sampleCategoryListJSON() string {
	cats := map[string]any{
		"cats": []map[string]any{
			{"category_id": 2, "category_name": "Xe cộ", "parent_id": 0},
			{"category_id": 20, "category_name": "Xe máy", "parent_id": 2},
			{"category_id": 21, "category_name": "Ô tô", "parent_id": 2},
		},
	}
	b, _ := json.Marshal(cats)
	return string(b)
}

func sampleRegionListJSON() string {
	regions := map[string]any{
		"regions": []map[string]any{
			{"region_v2": 13, "region_name": "Hồ Chí Minh"},
			{"region_v2": 2, "region_name": "Hà Nội"},
		},
	}
	b, _ := json.Marshal(regions)
	return string(b)
}

func TestGet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") == "" {
			t.Error("request carried no User-Agent")
		}
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := newTestClient(srv)
	body, err := c.Get(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != `{}` {
		t.Errorf("body = %q, want {}", body)
	}
}

func TestGetRetriesOn503(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if hits < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	cfg := DefaultConfig()
	cfg.BaseURL = srv.URL
	cfg.Rate = 0
	cfg.Retries = 5
	c := NewClientWithConfig(cfg)

	start := time.Now()
	body, err := c.Get(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != `{}` {
		t.Errorf("body = %q after retries", body)
	}
	if hits != 3 {
		t.Errorf("server saw %d hits, want 3", hits)
	}
	if time.Since(start) < 500*time.Millisecond {
		t.Error("retries did not back off")
	}
}

func TestGet404ReturnsNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c := newTestClient(srv)
	_, err := c.Get(context.Background(), srv.URL)
	if err != ErrNotFound {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestBrowseListings(t *testing.T) {
	listJSON := sampleListingListJSON(3)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(listJSON))
	}))
	defer srv.Close()

	c := newTestClient(srv)
	listings, err := c.BrowseListings(context.Background(), 0, 0, 1, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(listings) != 3 {
		t.Fatalf("got %d listings, want 3", len(listings))
	}
	if listings[0].ID != "10000000" {
		t.Errorf("listings[0].ID = %q, want 10000000", listings[0].ID)
	}
	if listings[0].Category != "Xe máy" {
		t.Errorf("Category = %q", listings[0].Category)
	}
}

func TestGetListing(t *testing.T) {
	detailJSON := sampleListingDetailJSON(12345678)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(detailJSON))
	}))
	defer srv.Close()

	c := newTestClient(srv)
	l, err := c.GetListing(context.Background(), "12345678")
	if err != nil {
		t.Fatal(err)
	}
	if l.ID != "12345678" {
		t.Errorf("ID = %q, want 12345678", l.ID)
	}
	if l.Title != "Honda Wave Alpha 2020" {
		t.Errorf("Title = %q", l.Title)
	}
	if l.Body != "Xe còn mới, ít đi" {
		t.Errorf("Body = %q", l.Body)
	}
	if len(l.Params) != 2 {
		t.Errorf("Params len = %d, want 2", len(l.Params))
	}
	if !l.IsVerified {
		t.Error("IsVerified = false, want true")
	}
}

func TestGetCategories(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(sampleCategoryListJSON()))
	}))
	defer srv.Close()

	c := newTestClient(srv)
	cats, err := c.GetCategories(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(cats) != 3 {
		t.Fatalf("got %d categories, want 3", len(cats))
	}
	if cats[1].Name != "Xe máy" {
		t.Errorf("cats[1].Name = %q, want Xe máy", cats[1].Name)
	}
}

func TestGetRegions(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(sampleRegionListJSON()))
	}))
	defer srv.Close()

	c := newTestClient(srv)
	regions, err := c.GetRegions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(regions) != 2 {
		t.Fatalf("got %d regions, want 2", len(regions))
	}
	if regions[0].Name != "Hồ Chí Minh" {
		t.Errorf("regions[0].Name = %q", regions[0].Name)
	}
}

func TestListingURL(t *testing.T) {
	got := listingURL(12345678)
	want := "https://www.chotot.com/12345678.htm"
	if got != want {
		t.Errorf("listingURL = %q, want %q", got, want)
	}
}
