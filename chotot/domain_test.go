package chotot

import (
	"testing"

	"github.com/tamnd/any-cli/kit"
)

func TestDomainInfo(t *testing.T) {
	info := Domain{}.Info()
	if info.Scheme != "chotot" {
		t.Errorf("Scheme = %q, want chotot", info.Scheme)
	}
	found := false
	for _, h := range info.Hosts {
		if h == Host {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Hosts = %v, want to contain %s", info.Hosts, Host)
	}
	if info.Identity.Binary != "chotot" {
		t.Errorf("Identity.Binary = %q, want chotot", info.Identity.Binary)
	}
}

func TestClassify(t *testing.T) {
	cases := []struct{ in, typ, id string }{
		{"12345678", "listing", "12345678"},
		{"https://www.chotot.com/12345678.htm", "listing", "12345678"},
		{"/12345678.htm", "listing", "12345678"},
	}
	for _, tc := range cases {
		typ, id, err := Domain{}.Classify(tc.in)
		if err != nil || typ != tc.typ || id != tc.id {
			t.Errorf("Classify(%q) = (%q, %q, %v), want (%q, %q, nil)",
				tc.in, typ, id, err, tc.typ, tc.id)
		}
	}
}

func TestClassifyInvalid(t *testing.T) {
	_, _, err := Domain{}.Classify("not-a-number")
	if err == nil {
		t.Error("expected error for non-numeric input, got nil")
	}
}

func TestLocate(t *testing.T) {
	got, err := Domain{}.Locate("listing", "12345678")
	want := "https://www.chotot.com/12345678.htm"
	if err != nil || got != want {
		t.Errorf("Locate = (%q, %v), want (%q, nil)", got, err, want)
	}
}

func TestLocateUnknownType(t *testing.T) {
	_, err := Domain{}.Locate("unknown", "x")
	if err == nil {
		t.Error("expected error for unknown type, got nil")
	}
}

func TestHostWiring(t *testing.T) {
	h, err := kit.Open()
	if err != nil {
		t.Fatal(err)
	}

	l := &Listing{
		ID:       "12345678",
		Title:    "Honda Wave Alpha 2020",
		URL:      "https://www.chotot.com/12345678.htm",
		PriceStr: "15.000.000 đ",
	}
	u, err := h.Mint(l)
	if err != nil {
		t.Fatalf("Mint: %v", err)
	}
	want := "chotot://listing/12345678"
	if u.String() != want {
		t.Errorf("Mint = %q, want %q", u.String(), want)
	}

	got, err := h.ResolveOn("chotot", "99999999")
	if err != nil || got.String() != "chotot://listing/99999999" {
		t.Errorf("ResolveOn = (%q, %v), want chotot://listing/99999999", got.String(), err)
	}
}
